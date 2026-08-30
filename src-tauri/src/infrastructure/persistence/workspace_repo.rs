//! Workspace persistence — `WorkspaceRepository` trait and SQLite impl.
//!
//! Repos live in a child table; workspaces reference them via `Workspace.repos`
//! populated on read. `RepoStatus::Missing` is the run-time marker for
//! repos whose `path` no longer points at a valid working tree; the
//! relink endpoint flips it back to `Active`.

use rusqlite::{params, Connection};

use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;
use crate::domain::workspace::{RepoRef, Workspace, WorkspaceSettings, WorkspaceSummary};

/// Storage abstraction for Workspaces. `Send` only — `Sync` is intentionally
/// NOT required because rusqlite's `Connection` is `!Sync`.
pub trait WorkspaceRepository: Send {
    fn create(&self, workspace: &Workspace) -> Result<()>;
    fn get(&self, id: &str) -> Result<Option<Workspace>>;
    fn list_summaries(&self) -> Result<Vec<WorkspaceSummary>>;
    fn rename(&self, id: &str, new_name: &str) -> Result<()>;
    fn delete(&self, id: &str) -> Result<()>;
    fn set_active_repo(&self, workspace_id: &str, repo_id: Option<&str>) -> Result<()>;
    fn update_settings(&self, id: &str, settings: &WorkspaceSettings) -> Result<()>;

    // Repo CRUD (Sprint 2)
    fn add_repo(&self, repo: &RepoRef) -> Result<()>;
    fn remove_repo(&self, workspace_id: &str, repo_id: &str) -> Result<()>;
    fn list_repos(&self, workspace_id: &str) -> Result<Vec<RepoRef>>;
    fn mark_repo_missing(&self, workspace_id: &str, repo_id: &str) -> Result<()>;
    fn relink_repo(&self, workspace_id: &str, repo_id: &str, new_path: &str) -> Result<()>;
    /// Persist tab order: `repo_ids` must list EVERY repo of the workspace
    /// exactly once, in the desired order (F005).
    fn reorder_repos(&self, workspace_id: &str, repo_ids: &[String]) -> Result<()>;
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
            let mut ws = row_to_workspace(row)?;
            ws.repos = self.list_repos(&ws.id)?;
            Ok(Some(ws))
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
            return Err(AppError::protocol_with(
                codes::infra::WORKSPACE_NOT_FOUND,
                format!("workspace not found: {id}"),
                &[("id", id.to_string())],
            ));
        }
        Ok(())
    }

    fn delete(&self, id: &str) -> Result<()> {
        let affected = self
            .conn
            .execute("DELETE FROM workspaces WHERE id = ?1", [id])
            .map_err(map_sqlite_err)?;
        if affected == 0 {
            return Err(AppError::protocol_with(
                codes::infra::WORKSPACE_NOT_FOUND,
                format!("workspace not found: {id}"),
                &[("id", id.to_string())],
            ));
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
            return Err(AppError::protocol_with(
                codes::infra::WORKSPACE_NOT_FOUND,
                format!("workspace not found: {workspace_id}"),
                &[("id", workspace_id.to_string())],
            ));
        }
        Ok(())
    }

    fn update_settings(&self, id: &str, settings: &WorkspaceSettings) -> Result<()> {
        let settings_json = serde_json::to_string(settings).map_err(map_serde_err)?;
        let affected = self
            .conn
            .execute(
                "UPDATE workspaces SET settings_json = ?1, updated_at = ?2 WHERE id = ?3",
                params![settings_json, now_unix(), id],
            )
            .map_err(map_sqlite_err)?;
        if affected == 0 {
            return Err(AppError::protocol_with(
                codes::infra::WORKSPACE_NOT_FOUND,
                format!("workspace not found: {id}"),
                &[("id", id.to_string())],
            ));
        }
        Ok(())
    }

    // ─── Repo CRUD ────────────────────────────────────────────────────────

    fn add_repo(&self, repo: &RepoRef) -> Result<()> {
        let settings_override_json = repo
            .settings_override
            .as_ref()
            .map(serde_json::to_string)
            .transpose()
            .map_err(map_serde_err)?;
        let status_str = repo_status_to_db(repo.status);
        self.conn
            .execute(
                "INSERT INTO repos \
                 (id, workspace_id, path, nickname, settings_override_json, status, missing_at, added_at, position) \
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, \
                         (SELECT COALESCE(MAX(position), -1) + 1 FROM repos WHERE workspace_id = ?2))",
                params![
                        repo.id,
                        repo.workspace_id,
                        repo.path,
                        repo.nickname,
                        settings_override_json,
                        status_str,
                        repo.missing_since,
                        repo.added_at,
                    ],
            )
            .map_err(map_sqlite_err)?;
        Ok(())
    }

    fn remove_repo(&self, workspace_id: &str, repo_id: &str) -> Result<()> {
        let affected = self
            .conn
            .execute(
                "DELETE FROM repos WHERE id = ?1 AND workspace_id = ?2",
                params![repo_id, workspace_id],
            )
            .map_err(map_sqlite_err)?;
        if affected == 0 {
            return Err(AppError::protocol_with(
                codes::infra::REPO_NOT_FOUND_IN_WS,
                format!("repo not found: {repo_id} in workspace {workspace_id}"),
                &[
                    ("repo_id", repo_id.to_string()),
                    ("workspace_id", workspace_id.to_string()),
                ],
            ));
        }
        Ok(())
    }

    fn list_repos(&self, workspace_id: &str) -> Result<Vec<RepoRef>> {
        let mut stmt = self
            .conn
            .prepare(
                "SELECT id, workspace_id, path, nickname, settings_override_json, \
                        status, missing_at, added_at \
                 FROM repos WHERE workspace_id = ?1 ORDER BY position, added_at",
            )
            .map_err(map_sqlite_err)?;
        let rows = stmt
            .query_map([workspace_id], row_to_repo)
            .map_err(map_sqlite_err)?
            .filter_map(std::result::Result::ok)
            .collect();
        Ok(rows)
    }

    fn mark_repo_missing(&self, workspace_id: &str, repo_id: &str) -> Result<()> {
        let affected = self
            .conn
            .execute(
                "UPDATE repos SET status = 'missing', missing_at = ?1 \
                 WHERE id = ?2 AND workspace_id = ?3",
                params![now_unix(), repo_id, workspace_id],
            )
            .map_err(map_sqlite_err)?;
        if affected == 0 {
            return Err(AppError::protocol_with(
                codes::infra::REPO_NOT_FOUND,
                format!("repo not found: {repo_id}"),
                &[("repo_id", repo_id.to_string())],
            ));
        }
        Ok(())
    }

    fn relink_repo(&self, workspace_id: &str, repo_id: &str, new_path: &str) -> Result<()> {
        let affected = self
            .conn
            .execute(
                "UPDATE repos SET path = ?1, status = 'active', missing_at = NULL \
                 WHERE id = ?2 AND workspace_id = ?3",
                params![new_path, repo_id, workspace_id],
            )
            .map_err(map_sqlite_err)?;
        if affected == 0 {
            return Err(AppError::protocol_with(
                codes::infra::REPO_NOT_FOUND,
                format!("repo not found: {repo_id}"),
                &[("repo_id", repo_id.to_string())],
            ));
        }
        Ok(())
    }

    fn reorder_repos(&self, workspace_id: &str, repo_ids: &[String]) -> Result<()> {
        let current = self.list_repos(workspace_id)?;
        // The order payload must be a permutation of the workspace's repos —
        // anything else (stale frontend list, cross-workspace id) would
        // silently drop or shift rows.
        if current.len() != repo_ids.len()
            || !current
                .iter()
                .all(|r| repo_ids.iter().any(|id| id == &r.id))
        {
            return Err(AppError::protocol_with(
                codes::infra::REORDER_MISMATCH,
                format!("reorder ids do not match repos of workspace {workspace_id}"),
                &[("workspace_id", workspace_id.to_string())],
            ));
        }
        let tx = self.conn.unchecked_transaction().map_err(map_sqlite_err)?;
        for (idx, repo_id) in repo_ids.iter().enumerate() {
            let affected = tx
                .execute(
                    "UPDATE repos SET position = ?1 WHERE id = ?2 AND workspace_id = ?3",
                    params![idx as i64, repo_id, workspace_id],
                )
                .map_err(map_sqlite_err)?;
            if affected == 0 {
                return Err(AppError::protocol_with(
                    codes::infra::REPO_NOT_FOUND,
                    format!("repo not found: {repo_id}"),
                    &[("repo_id", repo_id.to_string())],
                ));
            }
        }
        tx.commit().map_err(map_sqlite_err)?;
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

    Ok(Workspace {
        id,
        name,
        repos: Vec::new(), // populated by SqliteWorkspaceRepo::get via list_repos
        settings,
        last_active_repo_id,
        created_at,
        updated_at,
    })
}

fn row_to_repo(row: &rusqlite::Row<'_>) -> rusqlite::Result<RepoRef> {
    let id: String = row.get(0)?;
    let workspace_id: String = row.get(1)?;
    let path: String = row.get(2)?;
    let nickname: Option<String> = row.get(3)?;
    let settings_override_json: Option<String> = row.get(4)?;
    let status: String = row.get(5)?;
    let missing_at: Option<i64> = row.get(6)?;
    let added_at: i64 = row.get(7)?;

    let status = match status.as_str() {
        "missing" => crate::domain::workspace::RepoStatus::Missing,
        _ => crate::domain::workspace::RepoStatus::Active,
    };

    let settings_override = settings_override_json
        .map(|s| serde_json::from_str(&s))
        .transpose()
        .map_err(serde_to_sqlite)?;

    Ok(RepoRef {
        id,
        workspace_id,
        path,
        nickname,
        settings_override,
        status,
        missing_since: missing_at,
        added_at,
    })
}

fn serde_to_sqlite(e: serde_json::Error) -> rusqlite::Error {
    rusqlite::Error::FromSqlConversionFailure(0, rusqlite::types::Type::Text, Box::new(e))
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

fn map_serde_err(e: serde_json::Error) -> AppError {
    AppError::unknown_with(
        codes::infra::SERDE_ERROR,
        format!("serde: {e}"),
        &[("error", e.to_string())],
    )
}

fn repo_status_to_db(status: crate::domain::workspace::RepoStatus) -> &'static str {
    use crate::domain::workspace::RepoStatus;
    match status {
        RepoStatus::Active => "active",
        RepoStatus::Missing => "missing",
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::workspace::RepoStatus;
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

    fn sample_repo(ws_id: &str, id: &str, path: &str) -> RepoRef {
        RepoRef {
            id: id.into(),
            workspace_id: ws_id.into(),
            path: path.into(),
            nickname: None,
            settings_override: None,
            status: RepoStatus::Active,
            missing_since: None,
            added_at: 1000,
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
        repo.add_repo(&sample_repo("ws-1", "r-1", "/tmp")).unwrap();
        repo.delete("ws-1").expect("delete");
        let count: i64 = repo
            .conn
            .query_row("SELECT COUNT(*) FROM repos", [], |r| r.get(0))
            .unwrap();
        assert_eq!(count, 0);
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

    // ─── Repo CRUD tests ────────────────────────────────────────────────

    #[test]
    fn add_repo_then_list_repos() {
        let repo = fresh_repo();
        repo.create(&sample_ws("ws-1", "Default")).unwrap();
        repo.add_repo(&sample_repo("ws-1", "r-1", "/tmp/x"))
            .unwrap();
        repo.add_repo(&sample_repo("ws-1", "r-2", "/tmp/y"))
            .unwrap();

        let repos = repo.list_repos("ws-1").unwrap();
        assert_eq!(repos.len(), 2);
        assert_eq!(repos[0].id, "r-1");
        assert_eq!(repos[1].id, "r-2");
        assert_eq!(repos[0].workspace_id, "ws-1");
        assert_eq!(repos[0].path, "/tmp/x");
    }

    #[test]
    fn get_workspace_populates_repos() {
        let repo = fresh_repo();
        repo.create(&sample_ws("ws-1", "Default")).unwrap();
        repo.add_repo(&sample_repo("ws-1", "r-1", "/tmp")).unwrap();
        let ws = repo.get("ws-1").unwrap().unwrap();
        assert_eq!(ws.repos.len(), 1);
        assert_eq!(ws.repos[0].id, "r-1");
    }

    #[test]
    fn remove_repo_deletes_row() {
        let repo = fresh_repo();
        repo.create(&sample_ws("ws-1", "Default")).unwrap();
        repo.add_repo(&sample_repo("ws-1", "r-1", "/tmp")).unwrap();
        repo.remove_repo("ws-1", "r-1").expect("remove");
        assert_eq!(repo.list_repos("ws-1").unwrap().len(), 0);
    }

    #[test]
    fn remove_repo_wrong_workspace_errors() {
        let repo = fresh_repo();
        repo.create(&sample_ws("ws-1", "Default")).unwrap();
        repo.create(&sample_ws("ws-2", "Other")).unwrap();
        repo.add_repo(&sample_repo("ws-1", "r-1", "/tmp")).unwrap();
        let err = repo.remove_repo("ws-2", "r-1").expect_err("should error");
        assert_eq!(err.category(), "Protocol");
        assert!(repo.list_repos("ws-1").unwrap().len() == 1);
    }

    #[test]
    fn mark_repo_missing_flips_status() {
        let repo = fresh_repo();
        repo.create(&sample_ws("ws-1", "Default")).unwrap();
        repo.add_repo(&sample_repo("ws-1", "r-1", "/tmp")).unwrap();

        repo.mark_repo_missing("ws-1", "r-1").unwrap();
        let ws = repo.get("ws-1").unwrap().unwrap();
        let r = &ws.repos[0];
        assert_eq!(r.status, RepoStatus::Missing);
        assert!(r.missing_since.is_some());
    }

    #[test]
    fn relink_repo_restores_active_status() {
        let repo = fresh_repo();
        repo.create(&sample_ws("ws-1", "Default")).unwrap();
        repo.add_repo(&sample_repo("ws-1", "r-1", "/old")).unwrap();
        repo.mark_repo_missing("ws-1", "r-1").unwrap();

        repo.relink_repo("ws-1", "r-1", "/new").unwrap();
        let r = &repo.list_repos("ws-1").unwrap()[0];
        assert_eq!(r.path, "/new");
        assert_eq!(r.status, RepoStatus::Active);
        assert!(r.missing_since.is_none());
    }

    #[test]
    fn add_repo_for_missing_workspace_violates_fk() {
        let repo = fresh_repo();
        let r = sample_repo("nonexistent", "r-1", "/tmp");
        let err = repo.add_repo(&r).expect_err("FK should fail");
        // rusqlite returns Generic/Constraint error; we surface as Unknown.
        assert_eq!(err.category(), "Unknown");
    }

    // ─── Reorder tests (F005) ───────────────────────────────────────────

    #[test]
    fn reorder_repos_persists_new_order() {
        let repo = fresh_repo();
        repo.create(&sample_ws("ws-1", "Default")).unwrap();
        for id in ["r-1", "r-2", "r-3"] {
            repo.add_repo(&sample_repo("ws-1", id, "/tmp")).unwrap();
        }

        repo.reorder_repos("ws-1", &["r-3".into(), "r-1".into(), "r-2".into()])
            .expect("reorder");

        let ids: Vec<String> = repo
            .list_repos("ws-1")
            .unwrap()
            .into_iter()
            .map(|r| r.id)
            .collect();
        assert_eq!(ids, vec!["r-3", "r-1", "r-2"]);
    }

    #[test]
    fn reorder_repos_rejects_incomplete_or_foreign_ids() {
        let repo = fresh_repo();
        repo.create(&sample_ws("ws-1", "Default")).unwrap();
        repo.add_repo(&sample_repo("ws-1", "r-1", "/tmp/x"))
            .unwrap();
        repo.add_repo(&sample_repo("ws-1", "r-2", "/tmp/y"))
            .unwrap();

        // Missing id — would silently drop a repo from the order.
        let err = repo
            .reorder_repos("ws-1", &["r-1".into()])
            .expect_err("incomplete ids");
        assert_eq!(err.category(), "Protocol");

        // Foreign / unknown id.
        let err = repo
            .reorder_repos("ws-1", &["r-1".into(), "r-9".into()])
            .expect_err("foreign id");
        assert_eq!(err.category(), "Protocol");

        // Neither call may touch stored positions.
        let ids: Vec<String> = repo
            .list_repos("ws-1")
            .unwrap()
            .into_iter()
            .map(|r| r.id)
            .collect();
        assert_eq!(ids, vec!["r-1", "r-2"]);
    }

    #[test]
    fn add_repo_after_reorder_appends_to_end() {
        let repo = fresh_repo();
        repo.create(&sample_ws("ws-1", "Default")).unwrap();
        repo.add_repo(&sample_repo("ws-1", "r-1", "/tmp/x"))
            .unwrap();
        repo.add_repo(&sample_repo("ws-1", "r-2", "/tmp/y"))
            .unwrap();
        repo.reorder_repos("ws-1", &["r-2".into(), "r-1".into()])
            .unwrap();

        repo.add_repo(&sample_repo("ws-1", "r-3", "/tmp/z"))
            .unwrap();
        let ids: Vec<String> = repo
            .list_repos("ws-1")
            .unwrap()
            .into_iter()
            .map(|r| r.id)
            .collect();
        assert_eq!(ids, vec!["r-2", "r-1", "r-3"]);
    }
}
