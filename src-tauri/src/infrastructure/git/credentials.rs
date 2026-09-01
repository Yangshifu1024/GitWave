//! Credential callbacks for libgit2 clone / fetch operations.
//!
//! Two strategies:
//! - `SshAgentCredential`: SSH key from ssh-agent (libgit2 default).
//! - `GitCredentialHelper`: HTTPS user/password via the system
//!   `git credential fill`.
//!
//! Both produce a `RemoteCallbacks` that can be plugged into
//! `git2::FetchOptions::remote_callbacks`.

use std::io::Write;
use std::process::Stdio;
use std::sync::atomic::AtomicBool;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use git2::{Cred, CredentialType, RemoteCallbacks};

use crate::domain::error::{AppError, Result};
use crate::infrastructure::process::{hidden_command, wait_timeout, wait_with_output_timeout};

/// Upper bound for one `git credential fill`. A GUI helper (GCM) may hold
/// the process open while its dialog waits for the user; without a cap, a
/// dismissed or forgotten prompt hangs the whole fetch forever.
const CREDENTIAL_FILL_TIMEOUT: Duration = Duration::from_secs(120);

/// Upper bound for `git credential approve|reject` — helpers only store and
/// return, so anything this slow is stuck.
const CREDENTIAL_NOTIFY_TIMEOUT: Duration = Duration::from_secs(15);

/// Pluggable credential strategy for libgit2 fetch/clone.
pub trait CredentialProvider: Send + Sync {
    /// Build a fresh `RemoteCallbacks` configured with this provider.
    /// Call once per fetch/clone. libgit2 invokes the callbacks serially
    /// and the caller runs `approve`/`reject` only after the operation
    /// returned, so implementations may lock across the whole helper call.
    fn callbacks(&self) -> RemoteCallbacks<'_>;
    /// Tell the system helper the remote accepted the credentials handed out
    /// by [`Self::callbacks`] (`git credential approve`), so it can store
    /// them for the next operation. No-op for helpers without state.
    fn approve(&self) {}
    /// Tell the system helper the remote refused those credentials
    /// (`git credential reject`), so a stale stored credential doesn't
    /// shadow the next prompt. No-op for helpers without state.
    fn reject(&self) {}
}

/// Drive a libgit2 remote operation through the credential protocol:
/// success approves the credentials it used (the helper persists them),
/// an `Auth` failure rejects them (a stale stored credential must not
/// shadow the next prompt), anything else leaves the storage untouched —
/// the outcome said nothing about the credentials. A proxy 407 can also
/// surface as `Auth`; rejecting then is harmless, the next fill just
/// re-prompts.
pub(crate) fn run_with_credentials<T>(
    provider: &dyn CredentialProvider,
    operation: impl FnOnce() -> std::result::Result<T, git2::Error>,
    auth_error: impl FnOnce(&git2::Error) -> AppError,
    other_error: impl FnOnce(&git2::Error) -> AppError,
) -> Result<T> {
    match operation() {
        Ok(value) => {
            provider.approve();
            Ok(value)
        }
        Err(e) if e.code() == git2::ErrorCode::Auth => {
            provider.reject();
            Err(auth_error(&e))
        }
        Err(e) => Err(other_error(&e)),
    }
}

// SSH agent

/// SSH credential: use ssh-agent to provide the key. libgit2 talks to
/// `SSH_AUTH_SOCK` directly.
pub struct SshAgentCredential;

impl SshAgentCredential {
    #[must_use]
    pub fn new() -> Self {
        Self
    }

    fn build_callbacks(&self) -> RemoteCallbacks<'_> {
        let mut cb = RemoteCallbacks::new();
        cb.credentials(|_url, username_from_url, allowed_types| {
            if allowed_types.contains(CredentialType::SSH_KEY) {
                let user = username_from_url.unwrap_or("git");
                Cred::ssh_key_from_agent(user)
            } else {
                Err(git2::Error::from_str(
                    "ssh-agent: no SSH credential type allowed",
                ))
            }
        });
        cb
    }
}

impl Default for SshAgentCredential {
    fn default() -> Self {
        Self::new()
    }
}

impl CredentialProvider for SshAgentCredential {
    fn callbacks(&self) -> RemoteCallbacks<'_> {
        self.build_callbacks()
    }
}

// HTTPS via git credential fill

/// Auth-classified error for the libgit2 credentials callback. The HTTP
/// transports propagate callback errors verbatim, so an `Error::from_str`
/// would surface as `GenericError` and hide the auth failure from the
/// caller's reject / credential-error wiring.
fn auth_error(msg: &str) -> git2::Error {
    git2::Error::new(git2::ErrorCode::Auth, git2::ErrorClass::Http, msg)
}

/// One `git credential fill` per libgit2 operation. libgit2 re-invokes the
/// credentials callback after the remote rejects an answer (401 retry); a
/// second helper query would re-prompt for credentials already proven wrong
/// (or re-show a dialog the user just cancelled), so only the first
/// invocation reaches the helper.
#[derive(Default)]
struct FillOnce {
    queried: bool,
    answer: Option<(String, String)>,
}

/// Outcome of a gated helper query.
enum Fill {
    /// First query; the helper returned this username/password.
    Answer((String, String)),
    /// First query; the helper had nothing (not configured or cancelled).
    Empty,
    /// The gate already ran once this operation — fail instead of prompting.
    AlreadyQueried,
}

impl FillOnce {
    fn new() -> Self {
        Self::default()
    }

    fn take(&mut self, fill: impl FnOnce(&str) -> Option<(String, String)>, url: &str) -> Fill {
        // Once the helper answered, that answer is THE credential for the
        // whole operation: replay it for follow-up auth rounds and for the
        // push retry ladder instead of re-querying (which would re-prompt).
        if let Some((user, pass)) = &self.answer {
            return Fill::Answer((user.clone(), pass.clone()));
        }
        if self.queried {
            return Fill::AlreadyQueried;
        }
        self.queried = true;
        match fill(url) {
            Some(creds) => {
                self.answer = Some(creds.clone());
                Fill::Answer(creds)
            }
            None => Fill::Empty,
        }
    }
}

/// HTTPS credential: query the system `git credential fill` for
/// user/password. The helper may itself prompt the user; we surface the
/// resulting credentials to libgit2.
pub struct GitCredentialHelper {
    url: String,
    fill: Arc<Mutex<FillOnce>>,
    cancel: Option<Arc<AtomicBool>>,
}

impl GitCredentialHelper {
    #[must_use]
    pub fn new(url: String) -> Self {
        Self {
            url,
            fill: Arc::new(Mutex::new(FillOnce::new())),
            cancel: None,
        }
    }

    /// Share the operation's cancel flag: an aborted operation also stops
    /// waiting on a helper prompt instead of blocking until the fill
    /// timeout.
    #[must_use]
    pub fn with_cancel(mut self, cancel: Option<Arc<AtomicBool>>) -> Self {
        self.cancel = cancel;
        self
    }

    /// Credentials the helper actually supplied this operation; `approve`
    /// and `reject` must not speak for a prompt the user cancelled.
    fn helper_answer(&self) -> Option<(String, String)> {
        self.fill.lock().ok()?.answer.clone()
    }

    fn build_callbacks(&self) -> RemoteCallbacks<'_> {
        let url = self.url.clone();
        let gate = Arc::clone(&self.fill);
        let cancel = self.cancel.clone();
        let mut cb = RemoteCallbacks::new();
        cb.credentials(move |_url, username_from_url, allowed_types| {
            if allowed_types.contains(CredentialType::SSH_KEY) {
                let user = username_from_url.unwrap_or("git");
                if let Ok(c) = Cred::ssh_key_from_agent(user) {
                    return Ok(c);
                }
            }
            if allowed_types.contains(CredentialType::USER_PASS_PLAINTEXT) {
                let Ok(mut gate) = gate.lock() else {
                    return Err(auth_error("credential fill state poisoned"));
                };
                match gate.take(|url| query_helper(url, cancel.as_deref()), &url) {
                    Fill::Answer((user, pass)) => {
                        return Cred::userpass_plaintext(&user, &pass);
                    }
                    // The remote rejected the first answer; asking the
                    // helper again would pop the same prompt.
                    Fill::AlreadyQueried => {
                        return Err(auth_error("credentials rejected by the remote"));
                    }
                    // Cancelled or absent helper: the HTTPS transports never
                    // ask for a bare USERNAME (only the ssh one does), so
                    // surface the auth failure instead of retrying.
                    Fill::Empty => {
                        return Err(auth_error("no credentials available"));
                    }
                }
            }
            if allowed_types.contains(CredentialType::USERNAME) {
                let user = username_from_url.unwrap_or("anonymous").to_string();
                return Cred::username(&user);
            }
            Err(auth_error("no credentials available"))
        });
        cb
    }
}

impl CredentialProvider for GitCredentialHelper {
    fn callbacks(&self) -> RemoteCallbacks<'_> {
        self.build_callbacks()
    }

    fn approve(&self) {
        if let Some((user, pass)) = self.helper_answer() {
            notify_helper("approve", &self.url, &user, &pass);
        }
    }

    fn reject(&self) {
        if let Some((user, pass)) = self.helper_answer() {
            notify_helper("reject", &self.url, &user, &pass);
        }
    }
}

/// Serialize a `git credential` request for the helper's stdin, or `None`
/// when a value would break the protocol: it has no escaping, so a newline
/// in a value would inject fake keys (the same limitation the git CLI
/// has). Values come from the helper's own line-based output, so the
/// check is defense in depth.
fn credential_request(url: &str, user: Option<&str>, pass: Option<&str>) -> Option<String> {
    if [url, user.unwrap_or(""), pass.unwrap_or("")]
        .iter()
        .any(|value| value.contains(['\n', '\r']))
    {
        return None;
    }
    let mut request = format!("url={url}\n");
    if let Some(user) = user {
        request.push_str(&format!("username={user}\n"));
    }
    if let Some(pass) = pass {
        request.push_str(&format!("password={pass}\n"));
    }
    request.push('\n');
    Some(request)
}

/// Invoke `git credential fill` for the URL and return parsed user/pass.
/// Returns `None` if the helper is not configured / fails / does not answer
/// within [`CREDENTIAL_FILL_TIMEOUT`] (a GUI prompt nobody answers) or the
/// operation was cancelled.
fn query_helper(url: &str, cancel: Option<&AtomicBool>) -> Option<(String, String)> {
    let mut child = hidden_command("git")
        .args(["credential", "fill"])
        // The GUI helper (GCM) is controlled by its own settings; this only
        // stops git's terminal fallback from hanging on a prompt the piped
        // stdin can never answer.
        .env("GIT_TERMINAL_PROMPT", "0")
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::null())
        .spawn()
        .ok()?;

    if let Some(stdin) = child.stdin.as_mut() {
        let input = credential_request(url, None, None)?;
        stdin.write_all(input.as_bytes()).ok()?;
    }

    let output = match wait_with_output_timeout(child, CREDENTIAL_FILL_TIMEOUT, cancel) {
        Ok(Some(output)) => output,
        Ok(None) => {
            tracing::warn!(
                "git credential fill gave no answer within {}s (helper prompt unanswered or operation cancelled)",
                CREDENTIAL_FILL_TIMEOUT.as_secs()
            );
            return None;
        }
        Err(e) => {
            tracing::warn!("git credential fill wait failed: {e}");
            return None;
        }
    };
    if !output.status.success() {
        return None;
    }

    let stdout = String::from_utf8_lossy(&output.stdout);
    let mut user: Option<String> = None;
    let mut pass: Option<String> = None;
    for line in stdout.lines() {
        if let Some((k, v)) = line.split_once('=') {
            match k {
                "username" => user = Some(v.to_string()),
                "password" => pass = Some(v.to_string()),
                _ => {}
            }
        }
    }

    match (user, pass) {
        (Some(u), Some(p)) => Some((u, p)),
        (Some(u), None) => Some((u, String::new())),
        _ => None,
    }
}

/// Run `git credential approve|reject` for a credential the remote just
/// accepted or refused. `fill` alone never stores anything — without this
/// return trip a helper-prompted credential is lost and the next operation
/// prompts again. Best effort: the operation's own result already stands,
/// but failures are logged — a silent one degrades back to prompting on
/// every operation.
fn notify_helper(action: &str, url: &str, user: &str, pass: &str) {
    let Some(input) = credential_request(url, Some(user), Some(pass)) else {
        tracing::warn!("git credential {action} skipped: value would break the protocol");
        return;
    };
    let mut child = match hidden_command("git")
        .args(["credential", action])
        .env("GIT_TERMINAL_PROMPT", "0")
        .stdin(Stdio::piped())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
    {
        Ok(child) => child,
        Err(e) => {
            tracing::warn!("git credential {action} failed to spawn: {e}");
            return;
        }
    };
    if let Some(stdin) = child.stdin.as_mut() {
        if let Err(e) = stdin.write_all(input.as_bytes()) {
            tracing::warn!("git credential {action} stdin write failed: {e}");
        }
    }
    match wait_timeout(&mut child, CREDENTIAL_NOTIFY_TIMEOUT) {
        Ok(Some(status)) if !status.success() => {
            tracing::warn!("git credential {action} exited with {status}");
        }
        Ok(Some(_)) => {}
        Ok(None) => {
            tracing::warn!(
                "git credential {action} gave no answer within {}s — killed",
                CREDENTIAL_NOTIFY_TIMEOUT.as_secs()
            );
        }
        Err(e) => tracing::warn!("git credential {action} wait failed: {e}"),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicU32, Ordering};

    #[test]
    fn ssh_callback_constructs_without_panic() {
        let provider = SshAgentCredential::new();
        let cb = provider.callbacks();
        drop(cb);
    }

    #[test]
    fn https_callback_constructs_without_panic() {
        let provider = GitCredentialHelper::new("https://example.com/repo.git".into());
        let cb = provider.callbacks();
        drop(cb);
    }

    #[test]
    fn query_helper_returns_none_for_unconfigured_helper() {
        let result = query_helper("https://this-domain-does-not-exist.invalid/repo.git", None);
        assert!(result.is_none());
    }

    #[test]
    fn auth_error_carries_auth_code_for_reject_wiring() {
        // libgit2's transports propagate callback errors verbatim; only an
        // Auth code reaches the caller's reject()/credential-error branch.
        let err = auth_error("credentials rejected by the remote");
        assert_eq!(err.code(), git2::ErrorCode::Auth);
    }

    #[test]
    fn credential_request_matches_git_credential_protocol() {
        assert_eq!(
            credential_request("https://example.com/repo.git", None, None),
            Some("url=https://example.com/repo.git\n\n".to_string()),
            "fill sends only the url, blank-line terminated"
        );
        assert_eq!(
            credential_request("https://example.com/repo.git", Some("u"), Some("p")),
            Some("url=https://example.com/repo.git\nusername=u\npassword=p\n\n".to_string()),
            "approve/reject echo the full description"
        );
        assert_eq!(
            credential_request("https://example.com/repo.git", Some("u"), Some("")),
            Some("url=https://example.com/repo.git\nusername=u\npassword=\n\n".to_string()),
            "an empty password still emits its key"
        );
    }

    #[test]
    fn credential_request_rejects_control_characters() {
        // A newline in a value would inject fake keys into the protocol.
        assert!(credential_request("https://a.b\n/x", None, None).is_none());
        assert!(credential_request("https://a.b/x", Some("u\r"), Some("p")).is_none());
        assert!(credential_request("https://a.b/x", Some("u"), Some("p\nq")).is_none());
    }

    struct RecordingProvider {
        approved: AtomicU32,
        rejected: AtomicU32,
    }

    impl CredentialProvider for RecordingProvider {
        fn callbacks(&self) -> RemoteCallbacks<'_> {
            RemoteCallbacks::new()
        }
        fn approve(&self) {
            self.approved.fetch_add(1, Ordering::SeqCst);
        }
        fn reject(&self) {
            self.rejected.fetch_add(1, Ordering::SeqCst);
        }
    }

    fn unexpected_error(_: &git2::Error) -> AppError {
        panic!("error builder must not run for this outcome")
    }

    #[test]
    fn run_with_credentials_wires_the_provider_lifecycle() {
        use crate::domain::error_codes as codes;
        let provider = RecordingProvider {
            approved: AtomicU32::new(0),
            rejected: AtomicU32::new(0),
        };

        // Success approves and passes the value through.
        let value: u8 =
            run_with_credentials(&provider, || Ok(7), unexpected_error, unexpected_error).unwrap();
        assert_eq!(value, 7);

        // An auth failure rejects and maps through the auth builder.
        let err = run_with_credentials(
            &provider,
            || Err::<(), _>(auth_error("401")),
            |e| AppError::credential(codes::git::FETCH_AUTH_FAILED, format!("auth: {e}")),
            unexpected_error,
        )
        .unwrap_err();
        assert_eq!(err.category(), "Credential");

        // Any other failure touches neither approve nor reject.
        run_with_credentials(
            &provider,
            || Err::<(), _>(git2::Error::from_str("network down")),
            unexpected_error,
            |e| AppError::network(codes::git::FETCH_FAILED, format!("net: {e}")),
        )
        .unwrap_err();

        assert_eq!(provider.approved.load(Ordering::SeqCst), 1);
        assert_eq!(provider.rejected.load(Ordering::SeqCst), 1);
    }

    #[test]
    fn fill_once_queries_helper_only_on_first_take() {
        let mut gate = FillOnce::new();
        let mut calls = 0;

        match gate.take(
            |url: &str| {
                calls += 1;
                assert!(url.starts_with("https://"));
                Some(("user".to_string(), "pass".to_string()))
            },
            "https://example.com/repo.git",
        ) {
            Fill::Answer((user, pass)) => {
                assert_eq!((user.as_str(), pass.as_str()), ("user", "pass"));
            }
            _ => panic!("first take must return the helper answer"),
        }

        // Follow-up auth rounds (401 replay, push retry ladder) reuse the
        // stored answer without querying (and re-prompting) the helper.
        let retry = gate.take(
            |_: &str| -> Option<(String, String)> {
                calls += 1;
                Some(("never".to_string(), "never".to_string()))
            },
            "https://example.com/repo.git",
        );
        assert!(matches!(retry, Fill::Answer((ref u, ref p)) if u == "user" && p == "pass"));
        assert_eq!(calls, 1, "the helper must be queried at most once");
        assert_eq!(
            gate.answer.as_ref().map(|(u, _)| u.as_str()),
            Some("user"),
            "the answer stays available for approve/reject"
        );
    }

    #[test]
    fn fill_once_latches_even_when_helper_returns_nothing() {
        // A cancelled prompt must not re-show within the same operation.
        let mut gate = FillOnce::new();
        assert!(matches!(
            gate.take(
                |_: &str| -> Option<(String, String)> { None },
                "https://example.com/repo.git"
            ),
            Fill::Empty
        ));
        assert!(matches!(
            gate.take(
                |_: &str| -> Option<(String, String)> { Some(("u".to_string(), "p".to_string())) },
                "https://example.com/repo.git"
            ),
            Fill::AlreadyQueried
        ));
        assert!(gate.answer.is_none());
    }

    #[test]
    fn approve_and_reject_are_noop_without_a_helper_answer() {
        // A cancelled prompt means we never speak for the helper's storage.
        let provider = GitCredentialHelper::new("https://example.com/repo.git".into());
        assert!(provider.helper_answer().is_none());
        provider.approve();
        provider.reject();
    }
}
