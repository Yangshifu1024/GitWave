//! SQLite adapter — connection, WAL setup, and migration wiring.
//!
//! Uses bundled SQLite (no system dependency). Migration SQL is embedded
//! via `migrations.rs`; see that file for the migration list.

use std::path::{Path, PathBuf};

use rusqlite::Connection;

use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;

/// Resolve the platform-specific state directory for GitWave.
///
/// macOS: `~/Library/Application Support/GitWave`
/// Linux: `$XDG_DATA_HOME/GitWave` (or `~/.local/share/GitWave`)
/// Windows: `%APPDATA%/GitWave`
pub fn state_dir() -> Result<PathBuf> {
    let base = dirs::data_dir().ok_or_else(|| {
        AppError::permission(
            codes::infra::DATA_DIR_RESOLVE,
            "could not resolve user data directory",
        )
    })?;
    let dir = base.join("GitWave");
    ensure_dir(&dir)?;
    Ok(dir)
}

fn ensure_dir(dir: &Path) -> Result<()> {
    std::fs::create_dir_all(dir).map_err(|e| {
        AppError::permission_with(
            codes::infra::DATA_DIR_CREATE,
            format!("create {}: {e}", dir.display()),
            &[("dir", dir.display().to_string()), ("error", e.to_string())],
        )
    })
}

/// Open the SQLite database at `<state_dir>/state.db`, configuring WAL
/// journal mode and foreign keys. Migrations are applied automatically.
pub fn open() -> Result<Connection> {
    let path = state_dir()?.join("state.db");
    let conn = Connection::open(&path).map_err(map_sqlite_err)?;
    // WAL allows multiple readers + one writer concurrently — important
    // for multi-Workspace scenarios where the SQLite connection may be
    // accessed from async tokio tasks.
    conn.pragma_update(None, "journal_mode", "WAL")
        .map_err(map_sqlite_err)?;
    conn.pragma_update(None, "foreign_keys", "ON")
        .map_err(map_sqlite_err)?;
    // Two connections share state.db (workspace repo + F013 app settings
    // repo); a 5s busy wait turns rare write collisions into a short pause
    // instead of an immediate SQLITE_BUSY error.
    conn.busy_timeout(std::time::Duration::from_secs(5))
        .map_err(map_sqlite_err)?;
    super::migrations::apply(&conn)?;
    Ok(conn)
}

fn map_sqlite_err(e: rusqlite::Error) -> AppError {
    AppError::unknown_with(
        codes::infra::SQLITE_ERROR,
        format!("sqlite: {e}"),
        &[("error", e.to_string())],
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn in_memory_connection_works() {
        let conn = Connection::open_in_memory().expect("in-memory sqlite");
        let v: String = conn
            .query_row("SELECT sqlite_version()", [], |r| r.get(0))
            .expect("query");
        assert!(!v.is_empty());
    }

    #[test]
    fn state_dir_resolves_under_home() {
        let dir = state_dir().expect("state dir");
        assert!(dir.ends_with("GitWave"));
        assert!(dir.exists(), "state dir should exist after resolve");
    }
}
