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
use std::process::{Command, Stdio};

use git2::{Cred, CredentialType, RemoteCallbacks};

/// Pluggable credential strategy for libgit2 fetch/clone.
pub trait CredentialProvider: Send + Sync {
    /// Build a fresh `RemoteCallbacks` configured with this provider.
    /// Call once per fetch/clone.
    fn callbacks(&self) -> RemoteCallbacks<'_>;
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

/// HTTPS credential: query the system `git credential fill` for
/// user/password. The helper may itself prompt the user; we surface the
/// resulting credentials to libgit2.
pub struct GitCredentialHelper {
    url: String,
}

impl GitCredentialHelper {
    #[must_use]
    pub fn new(url: String) -> Self {
        Self { url }
    }

    fn build_callbacks(&self) -> RemoteCallbacks<'_> {
        let url = self.url.clone();
        let mut cb = RemoteCallbacks::new();
        cb.credentials(move |_url, username_from_url, allowed_types| {
            if allowed_types.contains(CredentialType::SSH_KEY) {
                let user = username_from_url.unwrap_or("git");
                if let Ok(c) = Cred::ssh_key_from_agent(user) {
                    return Ok(c);
                }
            }
            if allowed_types.contains(CredentialType::USER_PASS_PLAINTEXT) {
                if let Some((user, pass)) = query_helper(&url) {
                    return Cred::userpass_plaintext(&user, &pass);
                }
            }
            if allowed_types.contains(CredentialType::USERNAME) {
                let user = username_from_url.unwrap_or("anonymous").to_string();
                return Cred::username(&user);
            }
            Err(git2::Error::from_str("no credentials available"))
        });
        cb
    }
}

impl CredentialProvider for GitCredentialHelper {
    fn callbacks(&self) -> RemoteCallbacks<'_> {
        self.build_callbacks()
    }
}

/// Invoke `git credential fill` for the URL and return parsed user/pass.
/// Returns `None` if the helper is not configured / fails.
fn query_helper(url: &str) -> Option<(String, String)> {
    let mut child = Command::new("git")
        .args(["credential", "fill"])
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::null())
        .spawn()
        .ok()?;

    if let Some(stdin) = child.stdin.as_mut() {
        let input = format!("url={url}\n\n");
        stdin.write_all(input.as_bytes()).ok()?;
    }

    let output = child.wait_with_output().ok()?;
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

#[cfg(test)]
mod tests {
    use super::*;

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
        let result = query_helper("https://this-domain-does-not-exist.invalid/repo.git");
        assert!(result.is_none());
    }
}
