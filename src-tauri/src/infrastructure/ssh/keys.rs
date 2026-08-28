//! SSH key management — wrappers around `ssh-add` / `ssh-keygen` / `ssh -T`.
//!
//! Backed by the system OpenSSH client. No vendored crypto.

use std::path::{Path, PathBuf};
use std::process::Stdio;

use serde::Serialize;

use crate::domain::error::{AppError, Result};
use crate::infrastructure::process::hidden_command;

/// One SSH key currently loaded in the agent.
#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub struct SshKey {
    pub path: PathBuf,
    pub fingerprint: String,
    pub comment: String,
}

/// Outcome of an SSH connectivity probe (e.g. `ssh -T git@github.com`).
#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub struct SshTestResult {
    pub host: String,
    pub user: String,
    pub success: bool,
    pub message: String,
}

/// List keys currently loaded in ssh-agent (`ssh-add -l`).
/// Returns empty Vec if no agent or no keys — never errors.
pub fn list_loaded() -> Result<Vec<SshKey>> {
    let output = hidden_command("ssh-add")
        .arg("-l")
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .output()
        .map_err(|e| AppError::Unknown(format!("ssh-add: {e}")))?;

    if !output.status.success() {
        // ssh-add without agent or with no keys exits 1. Treat as empty.
        return Ok(Vec::new());
    }

    let stdout = String::from_utf8_lossy(&output.stdout);
    let mut keys = Vec::new();
    for line in stdout.lines() {
        // Format: "<bits> <fingerprint> <comment...> (<key-type>)"
        // Comment often contains the path: "/Users/x/.ssh/id_ed25519 (ED25519)"
        let parts: Vec<&str> = line.split_whitespace().collect();
        if parts.len() < 4 {
            continue;
        }
        let fingerprint = parts[1].to_string();
        let comment = parts[2..parts.len() - 1].join(" ");
        let path = PathBuf::from(&comment);
        keys.push(SshKey {
            path,
            fingerprint,
            comment,
        });
    }
    Ok(keys)
}

/// Add a key to the ssh-agent. Passphrase-protected keys cannot be added
/// from the GUI — `ssh-add` needs an interactive prompt and this spawn has
/// no controlling tty; that case gets an explicit, actionable error.
pub fn add(path: &Path) -> Result<()> {
    let output = hidden_command("ssh-add")
        .arg(path)
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .output()
        .map_err(|e| AppError::Unknown(format!("ssh-add: {e}")))?;
    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr).to_lowercase();
        if stderr.contains("incorrect passphrase")
            || stderr.contains("passphrase")
            || stderr.contains("permission denied")
        {
            return Err(AppError::Protocol(
                "this key is protected by a passphrase, which cannot be entered here. \
                 Load it once from a terminal (ssh-add <key>) so the agent caches it \
                 decrypted, or use a key without a passphrase"
                    .into(),
            ));
        }
        if stderr.contains("could not open a connection") || stderr.contains("no agent") {
            return Err(AppError::Protocol(
                "ssh-agent is not running — start it (on Windows: the \"OpenSSH Agent\" \
                 service; on macOS/Linux: eval $(ssh-agent)) and try again"
                    .into(),
            ));
        }
        return Err(AppError::Unknown(format!(
            "ssh-add failed (exit {}): {}",
            status_code_text(&output.status),
            String::from_utf8_lossy(&output.stderr).trim()
        )));
    }
    Ok(())
}

fn status_code_text(status: &std::process::ExitStatus) -> String {
    status
        .code()
        .map(|c| c.to_string())
        .unwrap_or_else(|| "signal".to_string())
}

/// Remove a key from the agent (`ssh-add -d`). The key file itself is
/// not deleted.
pub fn delete(path: &Path) -> Result<()> {
    let status = hidden_command("ssh-add")
        .arg("-d")
        .arg(path)
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::piped())
        .status()
        .map_err(|e| AppError::Unknown(format!("ssh-add: {e}")))?;
    if !status.success() {
        return Err(AppError::Unknown(format!(
            "ssh-add -d failed (exit {})",
            status.code().unwrap_or(-1)
        )));
    }
    Ok(())
}

/// Probe SSH connectivity to `user@host` via `ssh -T` in `BatchMode`.
/// GitHub-style servers return exit 1 with "successfully authenticated"
/// which we treat as success.
pub fn test_connection(host: &str, user: &str) -> Result<SshTestResult> {
    let output = hidden_command("ssh")
        .args([
            "-T",
            "-o",
            "BatchMode=yes",
            "-o",
            "StrictHostKeyChecking=accept-new",
            &format!("{user}@{host}"),
        ])
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .output()
        .map_err(|e| AppError::Unknown(format!("ssh: {e}")))?;

    let stdout = String::from_utf8_lossy(&output.stdout).into_owned();
    let stderr = String::from_utf8_lossy(&output.stderr).into_owned();
    let combined = format!("{stdout}{stderr}");
    let lower = combined.to_lowercase();

    let success = lower.contains("successfully authenticated")
        || (output.status.success() && !lower.contains("permission denied"));

    // Prefer stdout (real banner), fall back to stderr
    let message = if !stdout.is_empty() { stdout } else { stderr };

    Ok(SshTestResult {
        host: host.to_string(),
        user: user.to_string(),
        success,
        message,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn list_loaded_is_ok_even_without_agent() {
        let result = list_loaded();
        assert!(
            result.is_ok(),
            "list_loaded should be infallible on no agent"
        );
        // Result is a Vec — can be empty or contain loaded keys.
    }

    #[test]
    fn parse_ssh_add_line_shape() {
        // Manually validate the parser's expectations.
        let line = "256 SHA256:abc123def user@host (ED25519)";
        let parts: Vec<&str> = line.split_whitespace().collect();
        assert_eq!(parts.len(), 4);
        assert_eq!(parts[1], "SHA256:abc123def");
        assert_eq!(parts[2], "user@host");
        assert_eq!(parts[3], "(ED25519)");
    }

    #[test]
    fn test_connection_returns_result_for_unreachable_host() {
        // BatchMode + unreachable host should yield success=false without
        // an AppError (ssh itself runs but cannot authenticate).
        let result = test_connection("this-host-does-not-exist.invalid", "git")
            .expect("test_connection shouldn't return AppError");
        assert!(!result.success);
    }
}
