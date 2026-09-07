//! Credential callbacks for libgit2 clone / fetch operations.
//!!
//!! Three storage layers cooperate (see docs/tasks/fix-auth-credential-not-persisted/):
//!! - `SshAgentCredential`: SSH key from ssh-agent (libgit2 default).
//!! - `GitCredentialHelper`: HTTPS user/password via the system
//!!   `git credential fill`, with the app keyring (`CredentialVault`) as a
//!!   fallback when the helper has nothing.
//!! - `InlineCredentialProvider`: credentials typed in the F012 auth prompt,
//!!   remembered into both the helper (approve) and the app keyring.
//!!
//!! All providers produce a `RemoteCallbacks` that can be plugged into
//!! `git2::FetchOptions::remote_callbacks`.

use std::io::Write;
use std::process::{Command, Stdio};
use std::sync::atomic::AtomicBool;
use std::sync::{Arc, LazyLock, Mutex};
use std::time::Duration;

use git2::{Cred, CredentialType, RemoteCallbacks};

use crate::domain::error::{AppError, Result};
use crate::infrastructure::ai::secrets::entry_in_service;
use crate::infrastructure::process::{hidden_command, wait_timeout, wait_with_output_timeout};

/// How a `git credential approve` round trip ended. `ExitFailed` also covers
/// the exit-0-but-did-not-store case (observed with GCM on Windows): the
/// helper cannot prove it stored anything, so callers must treat the helper
/// as unreliable and lean on the vault fallback.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum HelperNotifyOutcome {
    /// The helper process confirmed the store.
    Stored,
    /// The helper process could not be spawned.
    SpawnFailed,
    /// The helper ran but exited non-zero.
    ExitFailed,
    /// The helper did not answer within [`CREDENTIAL_NOTIFY_TIMEOUT`].
    TimedOut,
}

impl HelperNotifyOutcome {
    #[must_use]
    pub fn stored(self) -> bool {
        matches!(self, Self::Stored)
    }
}

/// Result of one credential persistence attempt, consumed by the UI through
/// the `credential-storage` event so a silent persistence failure can never
/// degrade back into an unexplained prompt loop.
#[derive(Debug, Clone, serde::Serialize)]
#[serde(rename_all = "camelCase", tag = "status")]
pub enum CredentialStorageOutcome {
    /// The system helper confirmed the credential.
    Stored,
    /// The helper did not confirm, but the app keyring now holds the
    /// credential — GitWave will reuse it silently.
    Fallback,
    /// Neither storage accepted the credential; the next operation will
    /// prompt again.
    Failed,
}

/// Snapshot of the last persistence attempt, drained by the command layer
/// after the sync operation returns. A slot — not a queue — because the
/// status area shows only the last message anyway.
#[derive(Debug, Clone, Default)]
pub struct CredentialStorageSlot(Arc<Mutex<Option<CredentialStorageOutcome>>>);

impl CredentialStorageSlot {
    #[must_use]
    pub fn new() -> Self {
        Self(Arc::new(Mutex::new(None)))
    }

    fn record(&self, outcome: CredentialStorageOutcome) {
        if let Ok(mut slot) = self.0.lock() {
            *slot = Some(outcome);
        }
    }

    /// Take (and clear) the recorded outcome, if any. Empty means the
    /// operation neither approved nor rejected a credential.
    #[must_use]
    pub fn take(&self) -> Option<CredentialStorageOutcome> {
        let mut slot = self.0.lock().ok()?;
        slot.take()
    }
}

/// Storage for remembered HTTPS credentials, shared with nothing — entries
/// live under the app-owned `gitwave.remote` keyring service, so the system
/// credential helper stays the source of truth and the vault only catches
/// the cases the helper silently drops.
pub(crate) trait CredentialVault: Send + Sync {
    fn store(&self, host: &str, username: &str, password: &str) -> std::result::Result<(), String>;
    fn load(&self, host: &str) -> std::result::Result<Option<(String, String)>, String>;
    fn erase(&self, host: &str) -> std::result::Result<(), String>;
}

/// OS keychain-backed vault (Windows Credential Manager / macOS Keychain /
/// Secret Service). One entry per host, keyed `https/<host>`; the payload is
/// the username and password on separate lines.
struct KeyringVault;

impl CredentialVault for KeyringVault {
    fn store(&self, host: &str, username: &str, password: &str) -> std::result::Result<(), String> {
        entry(host)?
            .set_password(&encode_secret(username, password))
            .map_err(|e| format!("keychain set: {e}"))
    }

    fn load(&self, host: &str) -> std::result::Result<Option<(String, String)>, String> {
        match entry(host) {
            Ok(e) => match e.get_password() {
                Ok(payload) => decode_secret(&payload)
                    .map(Some)
                    .map_err(|e| format!("keychain payload: {e}")),
                Err(keyring::Error::NoEntry) => Ok(None),
                Err(e) => Err(format!("keychain get: {e}")),
            },
            Err(e) => Err(e),
        }
    }

    fn erase(&self, host: &str) -> std::result::Result<(), String> {
        match entry(host) {
            Ok(e) => match e.delete_credential() {
                Ok(()) => Ok(()),
                Err(keyring::Error::NoEntry) => Ok(()),
                Err(e) => Err(format!("keychain delete: {e}")),
            },
            Err(e) => Err(e),
        }
    }
}

/// Keyring entry for one host under the app-owned remote-credential service.
/// The `https/` prefix namespaces the account; hosts are scheme-agnostic
/// here because vault keys only carry host+port (git credential protocol
/// scope is effectively per-host for the app's HTTPS remotes).
fn entry(host: &str) -> std::result::Result<keyring::Entry, String> {
    entry_in_service(
        crate::infrastructure::ai::secrets::SERVICE_REMOTE,
        &format!("https/{host}"),
    )
    .map_err(|e| e.message())
}

/// Two-line `username\npassword` payload; rejects values that would break
/// the line-based split (the same protocol limitation the git credential
/// helper itself has).
fn encode_secret(username: &str, password: &str) -> String {
    format!("{username}\n{password}")
}

fn decode_secret(payload: &str) -> std::result::Result<(String, String), String> {
    let (username, password) = payload
        .split_once('\n')
        .ok_or_else(|| "payload missing separator".to_string())?;
    if password.contains('\n') {
        return Err("payload has extra lines".to_string());
    }
    Ok((username.to_string(), password.to_string()))
}

/// Storage host key for a remote URL: strip scheme and userinfo, keep host
/// and port, lowercase, drop any path. Credentials are host-scoped (the git
/// credential protocol treats them that way too). scp-style remotes
/// (`git@host:path`) separate host from path with a colon, not a slash.
pub(crate) fn vault_host(url: &str) -> Option<String> {
    let (rest, scp_style) = match url.split_once("://") {
        Some((_, rest)) => (rest, false),
        None => (url, true),
    };
    let rest = rest.split_once('@').map_or(rest, |(_, host)| host);
    let host = if scp_style {
        rest.split(':').next().unwrap_or(rest)
    } else {
        rest.split('/').next().unwrap_or(rest)
    };
    let host = host.trim_end_matches('.');
    if host.is_empty() {
        None
    } else {
        Some(host.to_ascii_lowercase())
    }
}

/// The process-wide vault. `set_vault` aims it at different storage (tests
/// inject an in-memory double); `None` restores the real keyring.
static VAULT: Mutex<Option<&'static dyn CredentialVault>> = Mutex::new(None);

static REAL_VAULT: KeyringVault = KeyringVault;

fn vault() -> &'static dyn CredentialVault {
    VAULT
        .lock()
        .ok()
        .and_then(|guard| *guard)
        .unwrap_or(&REAL_VAULT)
}

#[cfg(test)]
fn set_vault(vault: Option<&'static dyn CredentialVault>) {
    *VAULT.lock().expect("vault mutex poisoned") = vault;
}

/// Where persistence results are reported for the current operation; the
/// command layer drains it into the `credential-storage` event.
static STORAGE_SLOT: LazyLock<CredentialStorageSlot> = LazyLock::new(CredentialStorageSlot::new);
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
    /// First query; neither the helper nor the app keyring had anything
    /// (not configured, cancelled, or simply never saved).
    Empty,
    /// The gate already ran once this operation — fail instead of prompting.
    AlreadyQueried,
}

impl FillOnce {
    fn new() -> Self {
        Self::default()
    }

    fn take(
        &mut self,
        fill: impl FnOnce(&str) -> Option<(String, String)>,
        vault_lookup: impl FnOnce(&str) -> Option<(String, String)>,
        url: &str,
    ) -> Fill {
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
        // Helper first, app keyring second: a credential the helper silently
        // failed to store (the fix-auth-credential-not-persisted case) still
        // lives in the vault and must keep working.
        match fill(url).or_else(|| vault_lookup(url)) {
            Some(creds) => {
                self.answer = Some(creds.clone());
                Fill::Answer(creds)
            }
            None => Fill::Empty,
        }
    }
}

/// HTTPS credential: query the system `git credential fill` for
/// user/password. Helper interactivity is disabled — a GUI helper answers
/// only from its stored credentials or fails fast — and an empty answer
/// falls through to the app keyring; the F012 in-app prompt is the only
/// credential UX.
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
                match gate.take(
                    |url| query_helper(url, cancel.as_deref()),
                    remember_vault,
                    &url,
                ) {
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
            remember_credential(&self.url, &user, &pass);
        }
    }

    fn reject(&self) {
        if let Some((user, pass)) = self.helper_answer() {
            notify_helper("reject", &self.url, &user, &pass);
            // A refused credential must not keep coming back from the vault:
            // erase the fallback entry too, or the next operation would
            // replay the stale secret forever (the helper's own storage is
            // the helper's business — ours is the vault).
            erase_vault_entry(&self.url);
        }
    }
}

/// Erase the vault fallback entry for a remote's host.
fn erase_vault_entry(url: &str) {
    if let Some(host) = vault_host(url) {
        if let Err(e) = vault().erase(&host) {
            tracing::warn!("app keyring erase after reject failed: {e}");
        }
    }
}

/// User-supplied credentials entered in-app (F012): used verbatim for one
/// operation; `remember` persists them through the system helper on
/// `approve` so later operations fill from storage again.
#[derive(Clone, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct InlineAuth {
    pub username: String,
    pub password: String,
    pub remember: bool,
}

/// Debug never prints the password — one stray `{auth:?}` in a log line
/// must not leak the token.
impl std::fmt::Debug for InlineAuth {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("InlineAuth")
            .field("username", &self.username)
            .field("password", &"***")
            .field("remember", &self.remember)
            .finish()
    }
}

/// HTTPS credential straight from the app's auth prompt — no helper query,
/// no prompt loop. `reject` is a no-op by design: the credential was typed
/// this session and is only stored on `approve`, so there is nothing to
/// erase and a stored entry for another account must not be touched.
pub struct InlineCredentialProvider {
    url: String,
    username: String,
    password: String,
    remember: bool,
}

impl InlineCredentialProvider {
    pub fn new(url: String, username: String, password: String, remember: bool) -> Self {
        Self {
            url,
            username,
            password,
            remember,
        }
    }

    fn build_callbacks(&self) -> RemoteCallbacks<'_> {
        let username = self.username.clone();
        let password = self.password.clone();
        let mut cb = RemoteCallbacks::new();
        cb.credentials(move |_url, username_from_url, allowed_types| {
            if allowed_types.contains(CredentialType::USERNAME) {
                let user = if username.is_empty() {
                    username_from_url.unwrap_or("anonymous").to_string()
                } else {
                    username.clone()
                };
                return Cred::username(&user);
            }
            if allowed_types.contains(CredentialType::USER_PASS_PLAINTEXT) {
                return Cred::userpass_plaintext(&username, &password);
            }
            Err(auth_error(
                "inline credentials only cover HTTPS user/pass auth",
            ))
        });
        cb
    }
}

impl CredentialProvider for InlineCredentialProvider {
    fn callbacks(&self) -> RemoteCallbacks<'_> {
        self.build_callbacks()
    }

    fn approve(&self) {
        if self.remember {
            remember_credential(&self.url, &self.username, &self.password);
        }
    }

    fn reject(&self) {}
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

/// Build the `git credential …` subprocess for the helper protocol. Every
/// interactive channel is dead by construction:
///
/// - `GIT_TERMINAL_PROMPT=0` plus the two askpass vars pointed at a
///   nonexistent program kill git's own prompt chain
///   (GIT_ASKPASS → core.askpass → SSH_ASKPASS → terminal) — cancelling a
///   native askpass box yields an empty answer anyway; the app keyring
///   fallback and the in-app F012 prompt are the UX.
/// - `GCM_INTERACTIVE=never` and `-c credential.interactive=never` (the
///   latter travels down to helper subprocesses through git's config
///   propagation) tell a GUI helper like GCM to *fail immediately* when it
///   has no stored credential, instead of opening its own dialog and
///   holding the fill open for up to [`CREDENTIAL_FILL_TIMEOUT`]. Cached
///   credentials — including GCM OAuth tokens — are still returned
///   silently: `never` only refuses *interaction*. The empty answer falls
///   through to the vault lookup and, if that misses, the F012 prompt.
///
/// Shared by `fill` and `approve`/`reject` so the two call sites cannot
/// drift apart (approve/reject never prompt today, but the same guarantee
/// is cheap to enforce).
fn helper_command(args: &[&str]) -> Command {
    let mut cmd = hidden_command("git");
    cmd.args(["-c", "credential.interactive=never"]);
    cmd.args(args);
    cmd.env("GIT_TERMINAL_PROMPT", "0")
        .env("GCM_INTERACTIVE", "never")
        .env("GIT_ASKPASS", "gitwave-disabled-askpass")
        .env("SSH_ASKPASS", "gitwave-disabled-askpass");
    cmd
}

/// Invoke `git credential fill` for the URL and return parsed user/pass.
/// Returns `None` if the helper is not configured / fails / does not answer
/// within [`CREDENTIAL_FILL_TIMEOUT`] or the operation was cancelled. With
/// helper interactivity disabled ([`helper_command`]) the realistic timeout
/// case is a helper that ignores the `interactive` opt-out; a conforming
/// GCM fails within milliseconds instead of holding the dialog open.
fn query_helper(url: &str, cancel: Option<&AtomicBool>) -> Option<(String, String)> {
    let mut child = helper_command(&["credential", "fill"])
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

/// Helper had nothing: fall back to the app keyring so a remembered
/// credential keeps working when the system helper silently drops storage.
fn remember_vault(url: &str) -> Option<(String, String)> {
    let host = vault_host(url)?;
    match vault().load(&host) {
        Ok(Some(creds)) => {
            tracing::info!("credential helper empty; using app keyring fallback for {host}");
            Some(creds)
        }
        Ok(None) => None,
        Err(e) => {
            tracing::warn!("app keyring fallback read failed for {host}: {e}");
            None
        }
    }
}

/// Persist a credential the remote just accepted: the system helper first
/// (`git credential approve`), the app keyring always. The helper path is
/// unreliable in the wild — GCM on Windows has been observed exiting 0
/// without storing anything — so the vault write is not conditional on the
/// helper's answer, and the difference is only reported to the UI. A
/// no-op-keyring write (the vault already holds exactly this credential)
/// keeps per-fetch approves from churning the OS keychain.
fn remember_credential(url: &str, user: &str, pass: &str) {
    let helper = notify_helper("approve", url, user, pass);
    let vault_stored = store_in_vault(url, user, pass);
    STORAGE_SLOT.record(map_storage_outcome(helper, vault_stored));
}

/// The UI-facing classification: the helper's word only counts when it says
/// it stored; otherwise the vault decides between graceful fallback and a
/// visible failure.
fn map_storage_outcome(
    helper: HelperNotifyOutcome,
    vault_stored: bool,
) -> CredentialStorageOutcome {
    match (helper.stored(), vault_stored) {
        (true, _) => CredentialStorageOutcome::Stored,
        (false, true) => CredentialStorageOutcome::Fallback,
        (false, false) => CredentialStorageOutcome::Failed,
    }
}

/// Take (and clear) the outcome recorded by the last sync operation so the
/// command layer can surface it in the UI. Empty when the operation never
/// approved a credential.
pub fn drain_storage_outcome() -> Option<CredentialStorageOutcome> {
    STORAGE_SLOT.take()
}

/// Write the credential into the app keyring unless it is already there
/// (per-fetch approves must not churn the OS keychain). `false` on failure
/// or for values that could never be read back (a newline would break the
/// line-based payload — storing them would silently disable the fallback,
/// exactly the failure mode this module exists to kill).
fn store_in_vault(url: &str, user: &str, pass: &str) -> bool {
    if user.contains(['\n', '\r']) || pass.contains(['\n', '\r']) {
        tracing::warn!("app keyring store skipped: value would break the payload format");
        return false;
    }
    let Some(host) = vault_host(url) else {
        return false;
    };
    let v = vault();
    let already = matches!(
        v.load(&host),
        Ok(Some((ref u, ref p))) if u == user && p == pass
    );
    if already {
        return true;
    }
    match v.store(&host, user, pass) {
        Ok(()) => true,
        Err(e) => {
            tracing::warn!("app keyring store failed for {host}: {e}");
            false
        }
    }
}

/// Run `git credential approve|reject` for a credential the remote just
/// accepted or refused. `fill` alone never stores anything — without this
/// return trip a helper-prompted credential is lost and the next operation
/// prompts again. Best effort: the operation's own result already stands,
/// failures are logged and classified — a silent one used to degrade back
/// to prompting on every operation.
fn notify_helper(action: &str, url: &str, user: &str, pass: &str) -> HelperNotifyOutcome {
    let Some(input) = credential_request(url, Some(user), Some(pass)) else {
        tracing::warn!("git credential {action} skipped: value would break the protocol");
        return HelperNotifyOutcome::SpawnFailed;
    };
    let mut child = match helper_command(&["credential", action])
        .stdin(Stdio::piped())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn()
    {
        Ok(child) => child,
        Err(e) => {
            tracing::warn!("git credential {action} failed to spawn: {e}");
            return HelperNotifyOutcome::SpawnFailed;
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
            HelperNotifyOutcome::ExitFailed
        }
        // The process saying yes is still no proof it stored anything (the
        // GCM case) — callers must keep the vault fallback warm regardless.
        Ok(Some(_)) => HelperNotifyOutcome::Stored,
        Ok(None) => {
            tracing::warn!(
                "git credential {action} gave no answer within {}s — killed",
                CREDENTIAL_NOTIFY_TIMEOUT.as_secs()
            );
            HelperNotifyOutcome::TimedOut
        }
        Err(e) => {
            tracing::warn!("git credential {action} wait failed: {e}");
            HelperNotifyOutcome::SpawnFailed
        }
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
    fn helper_command_disables_every_interactive_channel() {
        // The GCM external dialog leak (fix-credential-dialog-convergence):
        // the helper subprocess must refuse interaction from every side —
        // git's own terminal/askpass chain AND the helper's own GUI (GCM),
        // which otherwise opens its dialog whenever it has no stored
        // credential and holds the fill open for the full timeout.
        let cmd = helper_command(&["credential", "fill"]);
        let args: Vec<String> = cmd
            .get_args()
            .map(|a| a.to_string_lossy().into_owned())
            .collect();
        assert_eq!(
            args,
            ["-c", "credential.interactive=never", "credential", "fill"],
            "`-c` must precede the subcommand so git parses it as config"
        );

        let env_val = |key: &str| {
            cmd.get_envs()
                .find(|(k, _)| *k == std::ffi::OsStr::new(key))
                .and_then(|(_, v)| v.map(|v| v.to_string_lossy().into_owned()))
        };
        assert_eq!(env_val("GCM_INTERACTIVE").as_deref(), Some("never"));
        assert_eq!(env_val("GIT_TERMINAL_PROMPT").as_deref(), Some("0"));
        assert_eq!(
            env_val("GIT_ASKPASS").as_deref(),
            Some("gitwave-disabled-askpass")
        );
        assert_eq!(
            env_val("SSH_ASKPASS").as_deref(),
            Some("gitwave-disabled-askpass")
        );
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
            |_: &str| -> Option<(String, String)> {
                panic!("vault must not run while the helper answers")
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
            |_: &str| -> Option<(String, String)> {
                panic!("vault must not run once an answer exists")
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
    fn fill_once_falls_back_to_the_vault_when_the_helper_is_empty() {
        let mut gate = FillOnce::new();
        match gate.take(
            |_: &str| -> Option<(String, String)> { None },
            |url: &str| {
                assert_eq!(url, "https://example.com/repo.git");
                Some(("vault-user".to_string(), "vault-pass".to_string()))
            },
            "https://example.com/repo.git",
        ) {
            Fill::Answer((user, pass)) => {
                assert_eq!((user.as_str(), pass.as_str()), ("vault-user", "vault-pass"));
            }
            _ => panic!("the vault fallback must satisfy the query"),
        }
        // The vault answer becomes THE answer for the operation.
        assert!(matches!(
            gate.answer.as_ref().map(|(u, _)| u.as_str()),
            Some("vault-user")
        ));
    }

    #[test]
    fn fill_once_reports_empty_when_neither_storage_answers() {
        let mut gate = FillOnce::new();
        assert!(matches!(
            gate.take(
                |_: &str| -> Option<(String, String)> { None },
                |_: &str| -> Option<(String, String)> { None },
                "https://example.com/repo.git"
            ),
            Fill::Empty
        ));
        assert!(gate.answer.is_none());
    }

    #[test]
    fn fill_once_latches_even_when_helper_returns_nothing() {
        // A cancelled prompt must not re-show within the same operation.
        let mut gate = FillOnce::new();
        assert!(matches!(
            gate.take(
                |_: &str| -> Option<(String, String)> { None },
                |_: &str| -> Option<(String, String)> { None },
                "https://example.com/repo.git"
            ),
            Fill::Empty
        ));
        assert!(matches!(
            gate.take(
                |_: &str| -> Option<(String, String)> { Some(("u".to_string(), "p".to_string())) },
                |_: &str| -> Option<(String, String)> { Some(("v".to_string(), "p".to_string())) },
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

    // ─── fix-auth-credential-not-persisted ─────────────────────────────────────

    use std::collections::HashMap;

    /// In-memory vault double; `fail_store` / `fail_load` simulate an
    /// unreachable keychain so the fallback and failure paths stay testable
    /// without touching the real OS storage.
    struct MockVault {
        entries: Mutex<HashMap<String, (String, String)>>,
        fail_store: bool,
        fail_load: bool,
        store_calls: AtomicU32,
    }

    impl MockVault {
        fn new(fail_store: bool, fail_load: bool) -> Self {
            Self {
                entries: Mutex::new(HashMap::new()),
                fail_store,
                fail_load,
                store_calls: AtomicU32::new(0),
            }
        }
    }

    impl CredentialVault for MockVault {
        fn store(
            &self,
            host: &str,
            username: &str,
            password: &str,
        ) -> std::result::Result<(), String> {
            self.store_calls.fetch_add(1, Ordering::SeqCst);
            if self.fail_store {
                return Err("mock store failure".to_string());
            }
            self.entries.lock().unwrap().insert(
                host.to_string(),
                (username.to_string(), password.to_string()),
            );
            Ok(())
        }

        fn load(&self, host: &str) -> std::result::Result<Option<(String, String)>, String> {
            if self.fail_load {
                return Err("mock load failure".to_string());
            }
            Ok(self.entries.lock().unwrap().get(host).cloned())
        }

        fn erase(&self, host: &str) -> std::result::Result<(), String> {
            self.entries.lock().unwrap().remove(host);
            Ok(())
        }
    }

    /// The vault is a process-global: vault-touching tests serialize on this
    /// lock, and the guard restores the real keyring even on panic.
    static VAULT_TEST_LOCK: Mutex<()> = Mutex::new(());

    struct VaultGuard;

    impl Drop for VaultGuard {
        fn drop(&mut self) {
            set_vault(None);
        }
    }

    #[test]
    fn vault_host_normalizes_remote_urls() {
        assert_eq!(
            vault_host("https://github.com/owner/repo.git").as_deref(),
            Some("github.com")
        );
        assert_eq!(
            vault_host("https://user@github.com/owner/repo.git").as_deref(),
            Some("github.com")
        );
        assert_eq!(
            vault_host("https://user:pass@GitHub.COM:8443/r.git").as_deref(),
            Some("github.com:8443")
        );
        assert_eq!(
            vault_host("HTTPS://Example.COM/").as_deref(),
            Some("example.com")
        );
        // scp-style: the colon after the host is the path separator.
        assert_eq!(
            vault_host("git@github.com:owner/repo.git").as_deref(),
            Some("github.com")
        );
        assert_eq!(
            vault_host("ssh://git@host.example/path").as_deref(),
            Some("host.example")
        );
        assert_eq!(vault_host("https://"), None);
    }

    #[test]
    fn secret_payload_roundtrip_and_rejects_malformed_values() {
        assert_eq!(encode_secret("u", "p"), "u\np");
        assert_eq!(
            decode_secret("u\np").unwrap(),
            ("u".to_string(), "p".to_string())
        );
        assert!(decode_secret("u").is_err(), "missing separator");
        assert!(decode_secret("u\np\nq").is_err(), "extra lines");
    }

    #[test]
    fn storage_outcome_maps_helper_and_vault_results() {
        assert!(matches!(
            map_storage_outcome(HelperNotifyOutcome::Stored, false),
            CredentialStorageOutcome::Stored
        ));
        assert!(matches!(
            map_storage_outcome(HelperNotifyOutcome::ExitFailed, true),
            CredentialStorageOutcome::Fallback
        ));
        assert!(matches!(
            map_storage_outcome(HelperNotifyOutcome::TimedOut, true),
            CredentialStorageOutcome::Fallback
        ));
        assert!(matches!(
            map_storage_outcome(HelperNotifyOutcome::SpawnFailed, false),
            CredentialStorageOutcome::Failed
        ));
    }

    #[test]
    fn store_in_vault_writes_skips_identical_rewrites_and_overwrites_changes() {
        let _serialization = VAULT_TEST_LOCK.lock().unwrap();
        let mock: &'static MockVault = Box::leak(Box::new(MockVault::new(false, false)));
        set_vault(Some(mock));
        let _guard = VaultGuard;
        let url = "https://github.com/owner/repo.git";

        assert!(store_in_vault(url, "u", "p"));
        assert_eq!(mock.store_calls.load(Ordering::SeqCst), 1);
        assert_eq!(
            mock.entries.lock().unwrap().get("github.com").cloned(),
            Some(("u".to_string(), "p".to_string()))
        );

        // The same credential again: no second keychain write.
        assert!(store_in_vault(url, "u", "p"));
        assert_eq!(mock.store_calls.load(Ordering::SeqCst), 1);

        // A rotated password must overwrite.
        assert!(store_in_vault(url, "u", "p2"));
        assert_eq!(mock.store_calls.load(Ordering::SeqCst), 2);
        assert_eq!(
            mock.entries.lock().unwrap().get("github.com").cloned(),
            Some(("u".to_string(), "p2".to_string()))
        );
    }

    #[test]
    fn store_in_vault_reports_failure_instead_of_panicking() {
        let _serialization = VAULT_TEST_LOCK.lock().unwrap();
        let mock: &'static MockVault = Box::leak(Box::new(MockVault::new(true, false)));
        set_vault(Some(mock));
        let _guard = VaultGuard;
        assert!(!store_in_vault("https://github.com/x.git", "u", "p"));
    }

    #[test]
    fn remember_vault_serves_the_fallback_entry_per_host() {
        let _serialization = VAULT_TEST_LOCK.lock().unwrap();
        let mock: &'static MockVault = Box::leak(Box::new(MockVault::new(false, false)));
        set_vault(Some(mock));
        let _guard = VaultGuard;
        mock.entries
            .lock()
            .unwrap()
            .insert("github.com".to_string(), ("u".to_string(), "p".to_string()));

        assert_eq!(
            remember_vault("https://github.com/owner/repo.git"),
            Some(("u".to_string(), "p".to_string()))
        );
        // A different host has no entry.
        assert_eq!(remember_vault("https://gitlab.com/x.git"), None);
        // A broken keychain must not fabricate credentials.
        let broken: &'static MockVault = Box::leak(Box::new(MockVault::new(false, true)));
        set_vault(Some(broken));
        assert_eq!(remember_vault("https://github.com/owner/repo.git"), None);
    }

    #[test]
    fn store_in_vault_rejects_line_breaking_values() {
        let _serialization = VAULT_TEST_LOCK.lock().unwrap();
        let mock: &'static MockVault = Box::leak(Box::new(MockVault::new(false, false)));
        set_vault(Some(mock));
        let _guard = VaultGuard;
        // Such a payload could never be read back — storing it would
        // silently disable the fallback.
        assert!(!store_in_vault("https://github.com/x.git", "u", "p\nq"));
        assert!(!store_in_vault("https://github.com/x.git", "u\r", "p"));
        assert!(mock.entries.lock().unwrap().is_empty());
    }

    #[test]
    fn helper_reject_also_erases_the_vault_fallback() {
        let _serialization = VAULT_TEST_LOCK.lock().unwrap();
        let mock: &'static MockVault = Box::leak(Box::new(MockVault::new(false, false)));
        set_vault(Some(mock));
        let _guard = VaultGuard;
        mock.entries.lock().unwrap().insert(
            "github.com".to_string(),
            ("stale".to_string(), "old".to_string()),
        );

        // The 401 path: the helper answered, the remote refused — the vault
        // entry must not come back on the next fill.
        let provider = GitCredentialHelper::new("https://github.com/owner/repo.git".into());
        {
            let mut gate = provider.fill.lock().unwrap();
            gate.answer = Some(("stale".to_string(), "old".to_string()));
        }
        provider.reject();
        assert!(!mock.entries.lock().unwrap().contains_key("github.com"));
    }

    #[test]
    fn storage_slot_take_clears_so_outcomes_do_not_leak_across_operations() {
        let slot = CredentialStorageSlot::new();
        assert!(slot.take().is_none(), "fresh slot is empty");
        slot.record(CredentialStorageOutcome::Failed);
        assert!(matches!(
            slot.take(),
            Some(CredentialStorageOutcome::Failed)
        ));
        // The recorded outcome must be consumed exactly once; a sticky slot
        // would replay stale outcomes on every later sync operation.
        assert!(slot.take().is_none());
    }
}
