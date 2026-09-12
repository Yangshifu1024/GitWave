//! Tracing initialization — JSON output to a rolling file in the platform
//! log directory.
//!
//! macOS: `~/Library/Application Support/GitWave/logs/app.YYYY-MM-DD.log`
//! Linux: `$XDG_DATA_HOME/GitWave/logs/app.YYYY-MM-DD.log`
//! Windows: `%APPDATA%/GitWave/logs/app.YYYY-MM-DD.log`
//!
//! Falls back to stderr when the log directory cannot be created.

use std::path::PathBuf;

use tracing_appender::non_blocking::WorkerGuard;
use tracing_subscriber::{fmt, prelude::*, EnvFilter};

/// Initialize the tracing subscriber with JSON output to a rolling file.
///
/// Returns a `WorkerGuard` that must be held for the lifetime of the
/// application to ensure buffered writes are flushed. Returns `None` if
/// the log directory cannot be created (fallback to stderr-only).
pub fn init() -> Option<WorkerGuard> {
    let filter = EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info"));

    match log_target() {
        Some((dir, file_name)) => {
            if let Err(e) = std::fs::create_dir_all(&dir) {
                eprintln!("gitwave: failed to create log dir {}: {e}", dir.display());
                return init_stderr(filter);
            }
            lock_down_log_dir(&dir);
            prune_old_logs(&dir);
            let file_appender = tracing_appender::rolling::daily(&dir, &file_name);
            // chmod what exists now (old files); files the appender
            // creates later inherit the umask — see `lock_down_log_dir`.
            lock_down_log_files(&dir);
            let (non_blocking, guard) = tracing_appender::non_blocking(file_appender);
            let _ = tracing_subscriber::registry()
                .with(filter)
                .with(
                    fmt::layer()
                        .with_target(true)
                        .json()
                        .with_writer(non_blocking),
                )
                .try_init();
            Some(guard)
        }
        None => init_stderr(filter),
    }
}

fn init_stderr(filter: EnvFilter) -> Option<WorkerGuard> {
    let _ = tracing_subscriber::registry()
        .with(filter)
        .with(fmt::layer().with_target(true).json())
        .try_init();
    None
}

fn log_target() -> Option<(PathBuf, String)> {
    let base = dirs::data_dir()?;
    let dir = base.join("GitWave").join("logs");
    Some((dir, "app.log".to_string()))
}

/// Retention decision for one log file: only `app.*` files older than the
/// 30-day window (by mtime) are pruned. Pure so the policy is unit-testable
/// on every platform.
fn should_prune(file_name: &str, mtime: std::time::SystemTime, now: std::time::SystemTime) -> bool {
    const RETENTION: std::time::Duration = std::time::Duration::from_secs(30 * 24 * 60 * 60);
    if !file_name.starts_with("app.") {
        return false;
    }
    now.duration_since(mtime).is_ok_and(|age| age > RETENTION)
}

/// Delete `app.*` log files older than 30 days (by mtime). Per-file
/// failures are silently skipped — a stale log is harmless, a failed
/// init is not.
fn prune_old_logs(dir: &std::path::Path) {
    let Ok(entries) = std::fs::read_dir(dir) else {
        return;
    };
    for entry in entries.flatten() {
        if !entry.file_type().is_ok_and(|t| t.is_file()) {
            continue;
        }
        let prune = entry
            .metadata()
            .and_then(|m| m.modified())
            .map(|mtime| {
                should_prune(
                    &entry.file_name().to_string_lossy(),
                    mtime,
                    std::time::SystemTime::now(),
                )
            })
            .unwrap_or(false);
        if prune {
            let _ = std::fs::remove_file(entry.path());
        }
    }
}

/// Owner-only permissions on Unix — logs may contain tokens/paths.
/// The dir gets 0700 (0600 would break traversal); existing `app.*`
/// files get 0600. Best effort, failures ignored: logging must never
/// crash startup. Note files the appender creates *later* inherit the
/// umask (usually 0644); a `tracing_appender` wrapper would be needed to
/// enforce 0600 on those too.
#[cfg(unix)]
fn lock_down_log_dir(dir: &std::path::Path) {
    use std::os::unix::fs::PermissionsExt;
    let _ = std::fs::set_permissions(dir, std::fs::Permissions::from_mode(0o700));
}

/// chmod existing `app.*` files to 0600 (Unix only, best effort).
#[cfg(unix)]
fn lock_down_log_files(dir: &std::path::Path) {
    use std::os::unix::fs::PermissionsExt;
    let Ok(entries) = std::fs::read_dir(dir) else {
        return;
    };
    for entry in entries.flatten() {
        if !entry.file_name().to_string_lossy().starts_with("app.") {
            continue;
        }
        let _ = std::fs::set_permissions(entry.path(), std::fs::Permissions::from_mode(0o600));
    }
}

/// Windows keeps default ACLs (no Unix permission bits).
#[cfg(not(unix))]
fn lock_down_log_dir(_dir: &std::path::Path) {}

/// Windows keeps default ACLs (no Unix permission bits).
#[cfg(not(unix))]
fn lock_down_log_files(_dir: &std::path::Path) {}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn init_is_idempotent() {
        // First call initializes; second call must not panic.
        init();
        init();
    }

    fn set_file_mtime(path: &std::path::Path, mtime: std::time::SystemTime) {
        use std::fs::FileTimes;
        let f = std::fs::OpenOptions::new().append(true).open(path).unwrap();
        f.set_times(FileTimes::new().set_modified(mtime)).unwrap();
    }

    #[test]
    fn should_prune_policy_is_name_and_age_gated() {
        use std::time::Duration;
        let now = std::time::SystemTime::now();
        let month = 24 * 60 * 60;
        assert!(should_prune(
            "app.2020-01-01.log",
            now - Duration::from_secs(31 * month),
            now
        ));
        assert!(
            !should_prune("app.log", now - Duration::from_secs(60), now),
            "fresh log stays"
        );
        assert!(
            !should_prune(
                "app.2024-06-01.log",
                now - Duration::from_secs(29 * month),
                now
            ),
            "inside retention stays"
        );
        assert!(
            !should_prune("other.log", now - Duration::from_secs(365 * month), now),
            "non-app names are ignored"
        );
    }

    #[test]
    fn prune_keeps_fresh_logs_and_foreign_files() {
        let dir = std::env::temp_dir().join(format!(
            "gitwave-prune-test-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_millis()
        ));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();

        let old_path = dir.join("app.2020-01-01.log");
        let fresh_path = dir.join("app.log");
        let foreign_path = dir.join("other.log");
        for p in [&old_path, &fresh_path, &foreign_path] {
            std::fs::write(p, b"log").unwrap();
        }
        // Age the old file beyond the 30-day retention window.
        let old_stamp = std::time::UNIX_EPOCH + std::time::Duration::from_secs(31 * 24 * 60 * 60);
        set_file_mtime(&old_path, old_stamp);

        prune_old_logs(&dir);

        assert!(!old_path.exists(), "expired app log must be pruned");
        assert!(fresh_path.exists(), "current log must survive");
        assert!(foreign_path.exists(), "non-app files must survive");

        let _ = std::fs::remove_dir_all(&dir);
    }
}
