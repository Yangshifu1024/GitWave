//! App-level global settings persistence (F013).
//!
//! `app_settings` is a flat key → JSON-blob store for configuration that is
//! not Workspace-scoped (first use: proxy settings). It uses its OWN SQLite
//! connection to `state.db` — the workspace repo owns the primary one and
//! rusqlite connections are not shareable; WAL mode makes multi-connection
//! access safe. Migrations are idempotent, so re-running them here is a
//! no-op.

use rusqlite::{params, Connection};

use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;

/// Storage abstraction for app-level settings. Same `Send`-not-`Sync`
/// rationale as [`super::workspace_repo::WorkspaceRepository`].
pub trait AppSettingsRepository: Send {
    fn get(&self, key: &str) -> Result<Option<String>>;
    fn set(&self, key: &str, value_json: &str) -> Result<()>;
}

/// SQLite-backed `AppSettingsRepository`.
pub struct SqliteAppSettingsRepo {
    conn: Connection,
}

impl SqliteAppSettingsRepo {
    /// Open a second connection to `<state_dir>/state.db` (WAL makes this
    /// safe alongside the workspace repo's connection).
    pub fn open() -> Result<Self> {
        Ok(Self {
            conn: super::sqlite::open()?,
        })
    }

    /// In-memory store for the fallback path (disk state unavailable).
    pub fn open_in_memory() -> Result<Self> {
        let conn = Connection::open_in_memory().map_err(|e| {
            AppError::unknown_with(
                codes::infra::SQLITE_ERROR,
                format!("sqlite: {e}"),
                &[("error", e.to_string())],
            )
        })?;
        super::migrations::apply(&conn)?;
        Ok(Self { conn })
    }
}

impl AppSettingsRepository for SqliteAppSettingsRepo {
    fn get(&self, key: &str) -> Result<Option<String>> {
        let mut stmt = self
            .conn
            .prepare("SELECT value_json FROM app_settings WHERE key = ?1")
            .map_err(map_sqlite_err)?;
        let mut rows = stmt.query([key]).map_err(map_sqlite_err)?;
        match rows.next().map_err(map_sqlite_err)? {
            Some(row) => Ok(Some(row.get(0).map_err(map_sqlite_err)?)),
            None => Ok(None),
        }
    }

    fn set(&self, key: &str, value_json: &str) -> Result<()> {
        self.conn
            .execute(
                "INSERT INTO app_settings (key, value_json, updated_at) VALUES (?1, ?2, ?3) \
                 ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, \
                 updated_at = excluded.updated_at",
                params![key, value_json, now_unix()],
            )
            .map_err(map_sqlite_err)?;
        Ok(())
    }
}

fn now_unix() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or(0)
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

    fn memory_repo() -> SqliteAppSettingsRepo {
        SqliteAppSettingsRepo::open_in_memory().expect("in-memory app settings repo")
    }

    #[test]
    fn missing_key_is_none() {
        let repo = memory_repo();
        assert_eq!(repo.get("proxy").expect("get"), None);
    }

    #[test]
    fn set_then_get_roundtrips_and_overwrites() {
        let repo = memory_repo();
        repo.set("proxy", r#"{"mode":"manual"}"#).expect("set");
        repo.set("proxy", r#"{"mode":"off"}"#).expect("set again");
        assert_eq!(
            repo.get("proxy").expect("get"),
            Some(r#"{"mode":"off"}"#.to_string())
        );
    }
}
