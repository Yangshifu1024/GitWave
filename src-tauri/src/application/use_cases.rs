//! Application use cases for workspace operations.
//!
//! Pure functions taking `&AppContext`, returning `Result<T>` from the
//! domain layer. No Tauri dependency here — easy to unit test.

use std::sync::{Arc, Mutex};

use crate::domain::error::{AppError, Result};
use crate::domain::workspace::{Workspace, WorkspaceSettings, WorkspaceSummary};
use crate::infrastructure::persistence::workspace_repo::WorkspaceRepository;
use crate::infrastructure::persistence::SqliteWorkspaceRepo;

/// Application context — bundles infrastructure adapters and exposes use
/// cases. Held by Tauri as managed state.
///
/// The repository is wrapped in `Arc<Mutex<…>>` so that `AppContext: Send +
/// Sync` is satisfied for `tauri::manage` even though `rusqlite::Connection`
/// is `!Sync`. Lock contention is acceptable for Sprint 1's single-task
/// call sites; revisit if benchmarks show contention.
pub struct AppContext {
    pub workspaces: Arc<Mutex<SqliteWorkspaceRepo>>,
}

impl AppContext {
    #[must_use]
    pub fn new(workspaces: Arc<Mutex<SqliteWorkspaceRepo>>) -> Self {
        Self { workspaces }
    }
}

fn new_id() -> String {
    use std::time::{SystemTime, UNIX_EPOCH};
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_nanos())
        .unwrap_or(0);
    format!("ws-{nanos:x}")
}

fn now_unix() -> i64 {
    use std::time::{SystemTime, UNIX_EPOCH};
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or(0)
}

/// Create a new Workspace with the given name. Trims whitespace; rejects
/// empty names. Returns the created Workspace including generated id and
/// timestamps.
pub fn create_workspace(ctx: &AppContext, name: String) -> Result<Workspace> {
    let trimmed = name.trim();
    if trimmed.is_empty() {
        return Err(AppError::Protocol("workspace name cannot be empty".into()));
    }
    let now = now_unix();
    let ws = Workspace {
        id: new_id(),
        name: trimmed.to_string(),
        repos: Vec::new(),
        settings: WorkspaceSettings::default(),
        last_active_repo_id: None,
        created_at: now,
        updated_at: now,
    };
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .create(&ws)?;
    Ok(ws)
}

/// List all workspaces, ordered by `updated_at` desc.
pub fn list_workspaces(ctx: &AppContext) -> Result<Vec<WorkspaceSummary>> {
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .list_summaries()
}

/// Rename a workspace. Trims and rejects empty names.
pub fn rename_workspace(ctx: &AppContext, id: String, new_name: String) -> Result<()> {
    let trimmed = new_name.trim();
    if trimmed.is_empty() {
        return Err(AppError::Protocol("workspace name cannot be empty".into()));
    }
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .rename(&id, trimmed)
}

/// Delete a workspace. Cascades to child repos.
pub fn delete_workspace(ctx: &AppContext, id: String) -> Result<()> {
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .delete(&id)
}

/// Set the active repo for a workspace (or clear it).
pub fn set_active_repo(
    ctx: &AppContext,
    workspace_id: String,
    repo_id: Option<String>,
) -> Result<()> {
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .set_active_repo(&workspace_id, repo_id.as_deref())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::persistence::migrations;
    use crate::infrastructure::persistence::workspace_repo::WorkspaceRepository;
    use rusqlite::Connection;

    fn fresh_ctx() -> AppContext {
        let conn = Connection::open_in_memory().expect("in-memory sqlite");
        migrations::apply(&conn).expect("migrations");
        AppContext::new(Arc::new(Mutex::new(SqliteWorkspaceRepo::new(conn))))
    }

    #[test]
    fn create_then_list() {
        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).expect("create");
        assert_eq!(ws.name, "Default");
        assert!(ws.id.starts_with("ws-"));
        let list = list_workspaces(&ctx).expect("list");
        assert_eq!(list.len(), 1);
        assert_eq!(list[0].id, ws.id);
    }

    #[test]
    fn empty_name_rejected() {
        let ctx = fresh_ctx();
        let err = create_workspace(&ctx, "   ".into()).expect_err("empty");
        assert_eq!(err.category(), "Protocol");
    }

    #[test]
    fn rename_then_delete() {
        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).unwrap();
        rename_workspace(&ctx, ws.id.clone(), "Renamed".into()).expect("rename");
        let ws_after = ctx.workspaces.lock().unwrap().get(&ws.id).unwrap().unwrap();
        assert_eq!(ws_after.name, "Renamed");
        delete_workspace(&ctx, ws.id.clone()).expect("delete");
        let after = ctx.workspaces.lock().unwrap().get(&ws.id).unwrap();
        assert!(after.is_none());
    }

    #[test]
    fn set_active_repo_roundtrip() {
        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).unwrap();
        set_active_repo(&ctx, ws.id.clone(), Some("r-1".into())).unwrap();
        let after = ctx.workspaces.lock().unwrap().get(&ws.id).unwrap().unwrap();
        assert_eq!(after.last_active_repo_id.as_deref(), Some("r-1"));
        set_active_repo(&ctx, ws.id.clone(), None).unwrap();
        let after = ctx.workspaces.lock().unwrap().get(&ws.id).unwrap().unwrap();
        assert_eq!(after.last_active_repo_id, None);
    }
}
