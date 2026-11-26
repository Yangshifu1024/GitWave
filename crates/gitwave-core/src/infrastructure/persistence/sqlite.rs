//! SQLite adapter — connection, WAL setup, and migration wiring.
//!
//! Uses bundled SQLite (no system dependency). Migration SQL is embedded
//! via `migrations.rs`; see that file for the migration list.

use std::path::{Path, PathBuf};

use rusqlite::Connection;

use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;

/// The file name of the state database, inside the state directory.
pub const STATE_DB_FILE: &str = "state.db";

/// Where GitWave keeps its state, created when it does not exist yet.
///
/// macOS: `~/Library/Application Support/GitWave`
/// Linux: `$XDG_DATA_HOME/GitWave` (or `~/.local/share/GitWave`)
/// Windows: `%APPDATA%/GitWave`
///
/// This is the released application's directory.
///
/// # The rewrite under development does not call this
///
/// It opens the released application's own database, and runs this
/// version's migrations on it: as soon as the two differ, the released
/// version is left with a database it does not understand. The development
/// build opens a copy instead, see [`crate::infrastructure::persistence::snapshot`].
/// Opening the original comes with the release pipeline, which decides how
/// the upgrade goes.
pub fn state_dir() -> Result<PathBuf> {
    let dir = state_dir_path()?;
    ensure_dir(&dir)?;
    Ok(dir)
}

/// Where GitWave keeps its state, without touching the file system.
pub fn state_dir_path() -> Result<PathBuf> {
    let base = dirs::data_dir().ok_or_else(|| {
        AppError::permission(
            codes::infra::DATA_DIR_RESOLVE,
            "could not resolve user data directory",
        )
    })?;
    Ok(base.join("GitWave"))
}

pub(super) fn ensure_dir(dir: &Path) -> Result<()> {
    std::fs::create_dir_all(dir).map_err(|e| {
        AppError::permission_with(
            codes::infra::DATA_DIR_CREATE,
            format!("create {}: {e}", dir.display()),
            &[("dir", dir.display().to_string()), ("error", e.to_string())],
        )
    })
}

/// Open the SQLite database in the user's state directory. See [`open_in`].
///
/// # The rewrite under development does not call this
///
/// It opens the released application's own database, and runs this
/// version's migrations on it: as soon as the two differ, the released
/// version is left with a database it does not understand. The development
/// build opens a copy instead, see [`crate::infrastructure::persistence::snapshot`].
/// Opening the original comes with the release pipeline, which decides how
/// the upgrade goes.
pub fn open() -> Result<Connection> {
    open_in(&state_dir()?)
}

/// Open the SQLite database `state.db` in `dir`, creating the directory and
/// the database when they are missing, and configuring WAL journal mode and
/// foreign keys. Migrations are applied automatically.
pub fn open_in(dir: &Path) -> Result<Connection> {
    ensure_dir(dir)?;
    let path = dir.join(STATE_DB_FILE);
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
    lock_down_db_files(&path);
    Ok(conn)
}

/// Restrict `state.db` (+ `-wal` / `-shm` / `-journal` sidecars, when
/// present) to owner-only access on Unix — the DB stores tokens. Best
/// effort: failures only warn, the DB works fine with looser permissions.
#[cfg(unix)]
pub(super) fn lock_down_db_files(path: &Path) {
    use std::os::unix::fs::PermissionsExt;
    let mut targets = vec![path.to_path_buf()];
    for suffix in ["-wal", "-shm", "-journal"] {
        let mut s = path.as_os_str().to_owned();
        s.push(suffix);
        targets.push(PathBuf::from(s));
    }
    for target in targets {
        if !target.exists() {
            continue;
        }
        if let Err(e) = std::fs::set_permissions(&target, std::fs::Permissions::from_mode(0o600)) {
            tracing::warn!(path = %target.display(), error = %e, "chmod 0600 failed");
        }
    }
}

/// Windows keeps default ACLs (no Unix permission bits). The copy of the
/// state database calls this too, so both versions are visible to it.
#[cfg(not(unix))]
pub(super) fn lock_down_db_files(_path: &Path) {}

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

    /// Plan-promised regression for `lock_down_db_files` (Unix only: the
    /// function itself is `#[cfg(unix)]`; Windows keeps default ACLs).
    #[cfg(unix)]
    #[test]
    fn lock_down_sets_owner_only_permissions() {
        use std::os::unix::fs::PermissionsExt;
        let dir = std::env::temp_dir().join(format!(
            "gitwave-db-perm-test-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_millis()
        ));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        let db = dir.join("state.db");
        let wal = dir.join("state.db-wal");
        std::fs::write(&db, b"x").unwrap();
        std::fs::write(&wal, b"x").unwrap();
        for p in [&db, &wal] {
            std::fs::set_permissions(p, std::fs::Permissions::from_mode(0o644)).unwrap();
        }

        lock_down_db_files(&db);

        for p in [&db, &wal] {
            let mode = std::fs::metadata(p).unwrap().permissions().mode() & 0o777;
            assert_eq!(mode, 0o600, "{p:?} must be owner-only after lock down");
        }

        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn state_dir_resolves_under_home_without_creating_it() {
        // Only the path: creating the directory is left to whoever opens it.
        let dir = state_dir_path().expect("state dir");
        assert!(dir.ends_with("GitWave"));
        assert_eq!(state_dir_path().expect("state dir again"), dir);
    }

    #[test]
    fn open_in_creates_the_directory_and_migrates_the_database() {
        let tmp = tempfile::tempdir().expect("a directory of the test's own");
        let dir = tmp.path().join("state");

        let conn = open_in(&dir).expect("open in a new directory");

        let tables: i64 = conn
            .query_row(
                "SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = 'workspaces'",
                [],
                |row| row.get(0),
            )
            .expect("count tables");
        assert_eq!(tables, 1, "the migrations ran on the new database");
        assert!(dir.join(STATE_DB_FILE).exists());
    }
}
