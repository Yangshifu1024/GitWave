//! SSH key management — wrappers around `ssh-add` / `ssh-keygen` / `ssh -T`.
//!
//! Backed by the system OpenSSH client. No vendored crypto.

use std::path::{Path, PathBuf};
use std::process::Stdio;

use serde::Serialize;

use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;
use crate::infrastructure::process::hidden_command;

/// One SSH key currently loaded in the agent.
#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub struct SshKey {
    pub path: PathBuf,
    pub fingerprint: String,
    pub comment: String,
}

/// Snapshot of the agent: whether it is reachable at all, plus the keys
/// currently loaded. An unreachable agent (Windows: the "OpenSSH
/// Authentication Agent" service not running; Unix: no agent on
/// `SSH_AUTH_SOCK`) is a distinct state from "running but empty" — the
/// UI needs to tell the user to start the agent, not to add keys.
#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub struct SshKeyList {
    pub agent_running: bool,
    pub keys: Vec<SshKey>,
}

/// Outcome of an SSH connectivity probe (e.g. `ssh -T git@github.com`).
#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub struct SshTestResult {
    pub host: String,
    pub user: String,
    pub success: bool,
    pub message: String,
}

/// Classify ssh-add stderr (already lowercased) as "no agent reachable".
/// Covers both OpenSSH flavors:
/// - Windows (`System32\OpenSSH`): "error connecting to agent: no such file
///   or directory" when the named-pipe agent service is not running
/// - Unix: "could not open a connection to your authentication agent."
fn is_agent_unreachable(stderr_lower: &str) -> bool {
    stderr_lower.contains("could not open a connection")
        || stderr_lower.contains("no agent")
        || stderr_lower.contains("error connecting to agent")
}

/// Expand a leading `~` / `~/` / `~\` to the user's home directory
/// (`dirs::home_dir()`). Anything else — absolute or relative — is
/// returned unchanged (trimmed). Falls back to the literal path when no
/// home directory can be determined.
pub fn expand_tilde(path: &str) -> PathBuf {
    let trimmed = path.trim();
    let rest = if trimmed == "~" {
        Some(String::new())
    } else {
        trimmed
            .strip_prefix("~/")
            .or_else(|| trimmed.strip_prefix("~\\"))
            .map(|rest| rest.to_string())
    };
    if let Some(rest) = rest {
        if let Some(home) = dirs::home_dir() {
            return if rest.is_empty() {
                home
            } else {
                home.join(rest)
            };
        }
    }
    PathBuf::from(trimmed)
}

/// List keys currently loaded in ssh-agent (`ssh-add -l`). Never errors on
/// agent state — an unreachable agent yields `agent_running: false`.
pub fn list_loaded() -> Result<SshKeyList> {
    let output = hidden_command("ssh-add")
        .arg("-l")
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .output()
        .map_err(|e| {
            AppError::unknown_with(
                codes::infra::SSH_ADD_FAILED,
                format!("ssh-add: {e}"),
                &[("error", e.to_string())],
            )
        })?;

    if !output.status.success() {
        // "The agent has no identities." (exit 1) means the agent is fine;
        // anything matching the agent-unreachable strings means it is not.
        let stderr = String::from_utf8_lossy(&output.stderr).to_lowercase();
        return Ok(SshKeyList {
            agent_running: !is_agent_unreachable(&stderr),
            keys: Vec::new(),
        });
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
    Ok(SshKeyList {
        agent_running: true,
        keys,
    })
}

/// Add a key to the ssh-agent. Passphrase-protected keys cannot be added
/// from the GUI — `ssh-add` needs an interactive prompt and this spawn has
/// no controlling tty; that case gets an explicit, actionable error.
pub fn add(path: &Path) -> Result<()> {
    if path
        .extension()
        .is_some_and(|ext| ext.eq_ignore_ascii_case("pub"))
    {
        return Err(AppError::protocol(
            codes::infra::KEY_PUBLIC_FILE,
            "ssh-add loads the private key — pick the file without the .pub \
             extension (e.g. id_ed25519, not id_ed25519.pub)",
        ));
    }
    let output = hidden_command("ssh-add")
        .arg(path)
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .output()
        .map_err(|e| {
            AppError::unknown_with(
                codes::infra::SSH_ADD_FAILED,
                format!("ssh-add: {e}"),
                &[("error", e.to_string())],
            )
        })?;
    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr).to_lowercase();
        if stderr.contains("incorrect passphrase")
            || stderr.contains("passphrase")
            || stderr.contains("permission denied")
        {
            return Err(AppError::protocol(
                codes::infra::KEY_PASSPHRASE,
                "this key is protected by a passphrase, which cannot be entered here. \
                 Load it once from a terminal (ssh-add <key>) so the agent caches it \
                 decrypted, or use a key without a passphrase",
            ));
        }
        if is_agent_unreachable(&stderr) {
            return Err(AppError::protocol(
                codes::infra::AGENT_NOT_RUNNING,
                "ssh-agent is not running — start it (on Windows: the \"OpenSSH Agent\" \
                 service; on macOS/Linux: eval $(ssh-agent)) and try again",
            ));
        }
        return Err(AppError::unknown_with(
            codes::infra::SSH_ADD_EXIT,
            format!(
                "ssh-add failed (exit {}): {}",
                status_code_text(&output.status),
                String::from_utf8_lossy(&output.stderr).trim()
            ),
            &[
                ("exit", status_code_text(&output.status)),
                (
                    "stderr",
                    String::from_utf8_lossy(&output.stderr).trim().to_string(),
                ),
            ],
        ));
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
    let output = hidden_command("ssh-add")
        .arg("-d")
        .arg(path)
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::piped())
        .output()
        .map_err(|e| {
            AppError::unknown_with(
                codes::infra::SSH_ADD_FAILED,
                format!("ssh-add: {e}"),
                &[("error", e.to_string())],
            )
        })?;
    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr).to_lowercase();
        if is_agent_unreachable(&stderr) {
            return Err(AppError::protocol(
                codes::infra::AGENT_NOT_RUNNING,
                "ssh-agent is not running — start it (on Windows: the \"OpenSSH Agent\" \
                 service; on macOS/Linux: eval $(ssh-agent)) and try again",
            ));
        }
        return Err(AppError::unknown_with(
            codes::infra::SSH_DELETE_FAILED,
            format!(
                "ssh-add -d failed (exit {})",
                status_code_text(&output.status)
            ),
            &[("exit", status_code_text(&output.status))],
        ));
    }
    Ok(())
}

/// Ask Windows to enable + start the "OpenSSH Authentication Agent"
/// service. Requires elevation, so the command is handed to `Start-Process
/// -Verb RunAs`: the user sees one UAC prompt and nothing is elevated
/// silently. The fixed script takes no user input.
///
/// This returns once the UAC request has been handed to the shell — the
/// elevated process itself is not observable here, so callers should
/// re-probe the agent (`ssh-add -l`) afterwards.
#[cfg(windows)]
pub fn start_windows_agent_service() -> Result<()> {
    // `sc config ... start= auto` — the space after `=` is required by sc.
    let script = "Start-Process cmd -Verb RunAs -WindowStyle Hidden -ArgumentList \
                  '/c sc config ssh-agent start= auto & sc start ssh-agent'";
    let output = hidden_command("powershell")
        .args(["-NoProfile", "-Command", script])
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::piped())
        .output()
        .map_err(|e| {
            AppError::unknown_with(
                codes::infra::AGENT_START_FAILED,
                format!("powershell: {e}"),
                &[("error", e.to_string())],
            )
        })?;
    if !output.status.success() {
        // E.g. the user declined the UAC prompt (Start-Process throws).
        return Err(AppError::protocol(
            codes::infra::AGENT_START_FAILED,
            format!(
                "could not start the OpenSSH Agent service: {}",
                String::from_utf8_lossy(&output.stderr).trim()
            ),
        ));
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
        .map_err(|e| {
            AppError::unknown_with(
                codes::infra::SSH_SPAWN_FAILED,
                format!("ssh: {e}"),
                &[("error", e.to_string())],
            )
        })?;

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
        // Result carries agent state — either is valid on a dev machine.
    }

    #[test]
    fn agent_unreachable_windows_message() {
        assert!(is_agent_unreachable(
            "error connecting to agent: no such file or directory"
        ));
    }

    #[test]
    fn agent_unreachable_unix_message() {
        assert!(is_agent_unreachable(
            "ssh-add: could not open a connection to your authentication agent."
        ));
    }

    #[test]
    fn agent_reachable_with_no_identities() {
        assert!(!is_agent_unreachable("the agent has no identities."));
    }

    #[test]
    fn agent_unreachable_ignores_unrelated_errors() {
        assert!(!is_agent_unreachable(
            "error loading key \"foo\": invalid format"
        ));
    }

    #[test]
    fn expand_tilde_leaves_absolute_path_alone() {
        let p = if cfg!(windows) {
            "C:\\Users\\x\\.ssh\\id_rsa"
        } else {
            "/home/x/.ssh/id_rsa"
        };
        assert_eq!(expand_tilde(p), PathBuf::from(p));
    }

    #[test]
    fn expand_tilde_expands_home_prefix() {
        let home = dirs::home_dir().expect("home dir should resolve in tests");
        assert_eq!(expand_tilde("~"), home);
        assert_eq!(
            expand_tilde("~/.ssh/id_rsa"),
            home.join(".ssh").join("id_rsa")
        );
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
