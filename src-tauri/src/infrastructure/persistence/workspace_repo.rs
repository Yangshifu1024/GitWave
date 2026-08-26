//! Workspace persistence — `WorkspaceRepository` trait and SQLite impl.

use rusqlite::{params, Connection};

use crate::domain::error::{AppError, Result};
use crate::domain::workspace::{RepoRef, Workspace, WorkspaceSettings, WorkspaceSummary};

/// Storage abstraction for Workspaces. The trait stays async-friendly even
/// though Sprint 1 uses a synchronous SQLite impl, so a future tokio-based
/// adapter can swap in without touching call sites.
///
/// `Send` only — `Sync` is intentionally NOT required because rusqlite's
/// `Connection` is `!Sync` (its internal statement cache uses `RefCell`).
/// Sprint 1 callers access the repo from a single task; if we ever need
/// cross-task sharing, wrap the repo in `Mutex` and add `Sync` here.
pub trait WorkspaceRepository: Send {
    fn create(&self, workspace: &Workspace) -> Result<()>;
    fn get(&self, id: &str) -> Result<Option<Workspace>>;
    fn list_summaries(&self) -> Result<Vec<WorkspaceSummary>>;
    fn rename(&self, id: &str, new_name: &str) -> Result<()>;
    fn delete(&self, id: &str) -> Result<()>;
    fn set_active_repo(&self, workspace_id: &str, repo_id: Option<&str>) -> Result<()>;
}

/// SQLite-backed `WorkspaceRepository`.
pub struct SqliteWorkspaceRepo {
    conn: Connection,
}

impl SqliteWorkspaceRepo {
    #[must_use]
    pub fn new(conn: Connection) -> Self {
        Self { conn }
    }
}

impl WorkspaceRepository for SqliteWorkspaceRepo {
    fn create(&self, workspace: &Workspace) -> Result<()> {
        let settings_json = serde_json::to_string(&workspace.settings).map_err(map_serde_err)?;
        self.conn
            .execute(
                "INSERT INTO workspaces (id, name, settings_json, last_active_repo_id, created_at, updated_at) \
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6)",
                params![
                    workspace.id,
                    workspace.name,
                    settings_json,
                    workspace.last_active_repo_id,
                    workspace.created_at,
                    workspace.updated_at,
                ],
            )
            .map_err(map_sqlite_err)?;
        Ok(())
    }

    fn get(&self, id: &str) -> Result<Option<Workspace>> {
        let mut stmt = self
            .conn
            .prepare(
                "SELECT id, name, settings_json, last_active_repo_id, created_at, updated_at \
                 FROM workspaces WHERE id = ?1",
            )
            .map_err(map_sqlite_err)?;
        let mut rows = stmt.query([id]).map_err(map_sqlite_err)?;
        if let Some(row) = rows.next().map_err(map_sqlite_err)? {
            Ok(Some(row_to_workspace(row)?))
        } else {
            Ok(None)
        }
    }

    fn list_summaries(&self) -> Result<Vec<WorkspaceSummary>> {
        let mut stmt = self
            .conn
            .prepare(
                "SELECT id, name, last_active_repo_id, updated_at \
                 FROM workspaces ORDER BY updated_at DESC",
            )
            .map_err(map_sqlite_err)?;
        let rows = stmt
            .query_map([], |row| {
                Ok(WorkspaceSummary {
                    id: row.get(0)?,
                    name: row.get(1)?,
                    last_active_repo_id: row.get(2)?,
                    updated_at: row.get(3)?,
                })
            })
            .map_err(map_sqlite_err)?
            .filter_map(std::result::Result::ok)
            .collect();
        Ok(rows)
    }

    fn rename(&self, id: &str, new_name: &str) -> Result<()> {
        let affected = self
            .conn
            .execute(
                "UPDATE workspaces SET name = ?1, updated_at = ?2 WHERE id = ?3",
                params![new_name, now_unix(), id],
            )
            .map_err(map_sqlite_err)?;
        if affected == 0 {
            return Err(AppError::Protocol(format!("workspace not found: {id}")));
        }
        Ok(())
    }

    fn delete(&self, id: &str) -> Result<()> {
        let affected = self
            .conn
            .execute("DELETE FROM workspaces WHERE id = ?1", [id])
            .map_err(map_sqlite_err)?;
        if affected == 0 {
            return Err(AppError::Protocol(format!("workspace not found: {id}")));
        }
        Ok(())
    }

    fn set_active_repo(&self, workspace_id: &str, repo_id: Option<&str>) -> Result<()> {
        let affected = self
            .conn
            .execute(
                "UPDATE workspaces SET last_active_repo_id = ?1, updated_at = ?2 WHERE id = ?3",
                params![repo_id, now_unix(), workspace_id],
            )
            .map_err(map_sqlite_err)?;
        if affected == 0 {
            return Err(AppError::Protocol(format!(
                "workspace not found: {workspace_id}"
            )));
        }
        Ok(())
    }
}

fn row_to_workspace(row: &rusqlite::Row<'_>) -> Result<Workspace> {
    let id: String = row.get(0).map_err(map_sqlite_err)?;
    let name: String = row.get(1).map_err(map_sqlite_err)?;
    let settings_json: String = row.get(2).map_err(map_sqlite_err)?;
    let last_active_repo_id: Option<String> = row.get(3).map_err(map_sqlite_err)?;
    let created_at: i64 = row.get(4).map_err(map_sqlite_err)?;
    let updated_at: i64 = row.get(5).map_err(map_sqlite_err)?;

    let settings: WorkspaceSettings =
        serde_json::from_str(&settings_json).map_err(map_serde_err)?;

    // Sprint 1 doesn't persist repos yet (W2). Repos list starts empty
    // until the repo-add API lands.
    let repos: Vec<RepoRef> = Vec::new();

    Ok(Workspace {
        id,
        name,
        repos,
        settings,
        last_active_repo_id,
        created_at,
        updated_at,
    })
}

fn now_unix() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or(0)
}

fn map_sqlite_err(e: rusqlite::Error) -> AppError {
    AppError::Unknown(format!("sqlite: {e}"))
}

fn map_serde_err(e: serde_json::Error) -> AppError {
    AppError::Unknown(format!("serde: {e}"))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::persistence::migrations;

    fn fresh_repo() -> SqliteWorkspaceRepo {
        let conn = Connection::open_in_memory().expect("in-memory sqlite");
        migrations::apply(&conn).expect("migrations");
        SqliteWorkspaceRepo::new(conn)
    }

    fn sample_ws(id: &str, name: &str) -> Workspace {
        Workspace {
            id: id.into(),
            name: name.into(),
            repos: vec![],
            settings: WorkspaceSettings::default(),
            last_active_repo_id: None,
            created_at: 1000,
            updated_at: 1000,
        }
    }

    #[test]
    fn create_then_get_roundtrip() {
        let repo = fresh_repo();
        let ws = sample_ws("ws-1", "Default");
        repo.create(&ws).expect("create");
        let loaded = repo.get("ws-1").expect("get").expect("found");
        assert_eq!(loaded.id, "ws-1");
        assert_eq!(loaded.name, "Default");
        assert_eq!(loaded.created_at, 1000);
        assert_eq!(loaded.updated_at, 1000);
    }

    #[test]
    fn list_summaries_orders_by_updated_at_desc() {
        let repo = fresh_repo();
        repo.create(&sample_ws("a", "A")).unwrap();
        repo.create(&sample_ws("b", "B")).unwrap();
        repo.rename("a", "A renamed").unwrap();
        let list = repo.list_summaries().expect("list");
        // 'a' was just renamed -> updated_at = now() > 'b' (1000)
        assert_eq!(list[0].id, "a");
        assert_eq!(list[1].id, "b");
    }

    #[test]
    fn rename_missing_workspace_errors_protocol() {
        let repo = fresh_repo();
        let err = repo.rename("nope", "x").expect_err("should error");
        assert_eq!(err.category(), "Protocol");
    }

    #[test]
    fn delete_workspace_cascades_repos() {
        let repo = fresh_repo();
        repo.create(&sample_ws("ws-1", "Default")).unwrap();
        // Insert a repo row directly (no repo API yet) to verify cascade.
        repo.conn
            .execute(
                "INSERT INTO repos (id, workspace_id, path, added_at) \
                 VALUES ('r-1', 'ws-1', '/tmp', 1000)",
                [],
            )
            .unwrap();
        repo.delete("ws-1").expect("delete");
        let count: i64 = repo
            .conn
            .query_row("SELECT COUNT(*) FROM repos", [], |r| r.get(0))
            .unwrap();
        assert_eq!(count, 0, "ON DELETE CASCADE should remove child repos");
    }

    #[test]
    fn set_active_repo_updates_field() {
        let repo = fresh_repo();
        repo.create(&sample_ws("ws-1", "Default")).unwrap();
        repo.set_active_repo("ws-1", Some("r-1")).unwrap();
        let ws = repo.get("ws-1").unwrap().unwrap();
        assert_eq!(ws.last_active_repo_id.as_deref(), Some("r-1"));
        repo.set_active_repo("ws-1", None).unwrap();
        let ws = repo.get("ws-1").unwrap().unwrap();
        assert_eq!(ws.last_active_repo_id, None);
    }

    #[test]
    fn workspace_summary_omits_settings() {
        let repo = fresh_repo();
        repo.create(&sample_ws("ws-1", "Default")).unwrap();
        let summaries = repo.list_summaries().unwrap();
        assert_eq!(summaries.len(), 1);
        assert_eq!(summaries[0].id, "ws-1");
        assert_eq!(summaries[0].name, "Default");
    }
}
