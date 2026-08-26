//! SQLite adapter — connection, WAL setup, migration runner.
//!
//! Uses bundled SQLite (no system dependency). Future migration files will
//! live in `migrations/NNNN-name.sql` and be embedded via `include_str!`.

use std::path::{Path, PathBuf};

use rusqlite::Connection;

use crate::domain::error::{AppError, Result};

/// Resolve the platform-specific state directory for GitWave.
///
/// macOS: `~/Library/Application Support/GitWave`
/// Linux: `$XDG_DATA_HOME/GitWave` (or `~/.local/share/GitWave`)
/// Windows: `%APPDATA%/GitWave`
pub fn state_dir() -> Result<PathBuf> {
    let base = dirs::data_dir()
        .ok_or_else(|| AppError::Permission("could not resolve user data directory".into()))?;
    let dir = base.join("GitWave");
    ensure_dir(&dir)?;
    Ok(dir)
}

fn ensure_dir(dir: &Path) -> Result<()> {
    std::fs::create_dir_all(dir)
        .map_err(|e| AppError::Permission(format!("create {}: {e}", dir.display())))
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
    migrate(&conn)?;
    Ok(conn)
}

/// Apply pending migrations. Sprint 0 stub: no migrations yet. Sprint 1
/// will introduce the `workspaces` and `repos` tables.
fn migrate(_conn: &Connection) -> Result<()> {
    Ok(())
}

fn map_sqlite_err(e: rusqlite::Error) -> AppError {
    AppError::Unknown(format!("sqlite: {e}"))
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
    fn migrate_is_idempotent_on_empty_db() {
        let conn = Connection::open_in_memory().expect("in-memory sqlite");
        migrate(&conn).expect("first migrate");
        migrate(&conn).expect("second migrate");
    }

    #[test]
    fn state_dir_resolves_under_home() {
        let dir = state_dir().expect("state dir");
        assert!(dir.ends_with("GitWave"));
        assert!(dir.exists(), "state dir should exist after resolve");
    }
}
