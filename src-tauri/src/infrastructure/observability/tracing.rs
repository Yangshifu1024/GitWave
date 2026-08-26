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
            let file_appender = tracing_appender::rolling::daily(&dir, &file_name);
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn init_is_idempotent() {
        // First call initializes; second call must not panic.
        init();
        init();
    }
}
