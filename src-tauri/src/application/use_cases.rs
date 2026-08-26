//! Application use cases — workspace, repo, and SSH operations.
//!
//! Pure functions taking `&AppContext` (when they touch the DB) or no
//! argument at all (stateless SSH subprocess wrappers). No Tauri dependency
//! here — easy to unit test.

use std::path::PathBuf;
use std::sync::{Arc, Mutex};

use crate::domain::error::{AppError, Result};
use crate::domain::workspace::{
    RepoRef, RepoStatus, Workspace, WorkspaceSettings, WorkspaceSummary,
};
use crate::infrastructure::persistence::workspace_repo::WorkspaceRepository;
use crate::infrastructure::persistence::SqliteWorkspaceRepo;
use crate::infrastructure::ssh::keys::{SshKey, SshTestResult};

/// Application context — bundles infrastructure adapters and exposes use
/// cases. Held by Tauri as managed state.
///
/// The repository is wrapped in `Arc<Mutex<…>>` so that `AppContext: Send +
/// Sync` is satisfied for `tauri::manage` even though `rusqlite::Connection`
/// is `!Sync`. Lock contention is acceptable for Sprint 1-2's single-task
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

fn new_workspace_id() -> String {
    use std::time::{SystemTime, UNIX_EPOCH};
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_nanos())
        .unwrap_or(0);
    format!("ws-{nanos:x}")
}

fn new_repo_id() -> String {
    use std::time::{SystemTime, UNIX_EPOCH};
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_nanos())
        .unwrap_or(0);
    format!("r-{nanos:x}")
}

fn now_unix() -> i64 {
    use std::time::{SystemTime, UNIX_EPOCH};
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or(0)
}

// ─── Workspace use cases (Sprint 1) ───────────────────────────────────────

/// Create a new Workspace with the given name. Trims whitespace; rejects
/// empty names.
pub fn create_workspace(ctx: &AppContext, name: String) -> Result<Workspace> {
    let trimmed = name.trim();
    if trimmed.is_empty() {
        return Err(AppError::Protocol("workspace name cannot be empty".into()));
    }
    let now = now_unix();
    let ws = Workspace {
        id: new_workspace_id(),
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

/// Rename a workspace.
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

// ─── Repo use cases (Sprint 2) ───────────────────────────────────────────

/// Initialize a new git repo at `path` and attach it to the workspace.
/// Does NOT create an initial commit (P1: 永不自动 commit).
pub fn init_repo(ctx: &AppContext, workspace_id: String, path: String) -> Result<RepoRef> {
    let p = PathBuf::from(&path);
    crate::infrastructure::git::repo_adapter::init(&p)?;

    let repo = RepoRef {
        id: new_repo_id(),
        workspace_id,
        path,
        nickname: None,
        settings_override: None,
        status: RepoStatus::Active,
        missing_since: None,
        added_at: now_unix(),
    };
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .add_repo(&repo)?;
    Ok(repo)
}

/// Clone a remote repository and attach it to the workspace. Routes SSH
/// (ssh:// or git@) vs HTTPS based on URL scheme.
pub fn clone_repo(
    ctx: &AppContext,
    workspace_id: String,
    url: String,
    dest_path: String,
) -> Result<RepoRef> {
    let dest = PathBuf::from(&dest_path);
    if url.starts_with("ssh://") || url.starts_with("git@") {
        crate::infrastructure::git::repo_adapter::clone_ssh(&url, &dest)?;
    } else {
        crate::infrastructure::git::repo_adapter::clone_https(&url, &dest)?;
    }

    let repo = RepoRef {
        id: new_repo_id(),
        workspace_id,
        path: dest_path,
        nickname: None,
        settings_override: None,
        status: RepoStatus::Active,
        missing_since: None,
        added_at: now_unix(),
    };
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .add_repo(&repo)?;
    Ok(repo)
}

/// Attach an existing local git repository (by path) to the workspace.
/// Verifies the path is a valid git working tree.
pub fn add_local_repo(ctx: &AppContext, workspace_id: String, path: String) -> Result<RepoRef> {
    let p = PathBuf::from(&path);
    // open_local returns AppError::Protocol if path isn't a git repo.
    let _repo = crate::infrastructure::git::git2_adapter::open_local(&p)?;

    let repo = RepoRef {
        id: new_repo_id(),
        workspace_id,
        path,
        nickname: None,
        settings_override: None,
        status: RepoStatus::Active,
        missing_since: None,
        added_at: now_unix(),
    };
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .add_repo(&repo)?;
    Ok(repo)
}

/// Remove a repo from the workspace. Does NOT delete the local directory.
pub fn remove_repo(ctx: &AppContext, workspace_id: String, repo_id: String) -> Result<()> {
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .remove_repo(&workspace_id, &repo_id)
}

/// Re-attach a repo whose path has changed. Verifies the new path is a
/// valid git repo and flips status back to Active.
pub fn relink_repo(
    ctx: &AppContext,
    workspace_id: String,
    repo_id: String,
    new_path: String,
) -> Result<()> {
    let p = PathBuf::from(&new_path);
    let _repo = crate::infrastructure::git::git2_adapter::open_local(&p)?;
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .relink_repo(&workspace_id, &repo_id, &new_path)
}

/// List repos for a workspace, ordered by `added_at`.
pub fn list_repos(ctx: &AppContext, workspace_id: String) -> Result<Vec<RepoRef>> {
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .list_repos(&workspace_id)
}

// ─── SSH use cases (Sprint 2) — stateless subprocess wrappers ────────────

pub fn list_ssh_keys() -> Result<Vec<SshKey>> {
    crate::infrastructure::ssh::keys::list_loaded()
}

pub fn add_ssh_key(path: String) -> Result<()> {
    let p = PathBuf::from(path);
    crate::infrastructure::ssh::keys::add(&p)
}

pub fn delete_ssh_key(path: String) -> Result<()> {
    let p = PathBuf::from(path);
    crate::infrastructure::ssh::keys::delete(&p)
}

pub fn test_ssh_connection(host: String, user: String) -> Result<SshTestResult> {
    crate::infrastructure::ssh::keys::test_connection(&host, &user)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::persistence::migrations;
    use crate::infrastructure::persistence::workspace_repo::WorkspaceRepository;
    use rusqlite::Connection;
    use std::fs;

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

    // ─── Repo use case tests ─────────────────────────────────────────────

    #[test]
    fn init_repo_creates_and_persists() {
        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).unwrap();
        let tmp = std::env::temp_dir().join(format!("gitwave-uc-init-{}", std::process::id()));
        let _ = fs::remove_dir_all(&tmp);

        let repo =
            init_repo(&ctx, ws.id.clone(), tmp.to_string_lossy().to_string()).expect("init_repo");
        assert_eq!(repo.workspace_id, ws.id);
        assert!(tmp.join(".git").exists());

        let list = list_repos(&ctx, ws.id).expect("list_repos");
        assert_eq!(list.len(), 1);
        assert_eq!(list[0].id, repo.id);

        fs::remove_dir_all(&tmp).ok();
    }

    #[test]
    fn add_local_repo_requires_valid_git_path() {
        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).unwrap();
        let tmp = std::env::temp_dir().join(format!("gitwave-uc-local-{}", std::process::id()));
        let _ = fs::remove_dir_all(&tmp);
        fs::create_dir_all(&tmp).unwrap();

        let err = add_local_repo(&ctx, ws.id.clone(), tmp.to_string_lossy().to_string())
            .expect_err("not a repo");
        assert_eq!(err.category(), "Protocol");

        fs::remove_dir_all(&tmp).ok();
    }

    #[test]
    fn add_local_repo_then_remove() {
        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).unwrap();
        let tmp = std::env::temp_dir().join(format!("gitwave-uc-addremove-{}", std::process::id()));
        let _ = fs::remove_dir_all(&tmp);
        fs::create_dir_all(&tmp).unwrap();
        crate::infrastructure::git::repo_adapter::init(&tmp).expect("init");

        let repo = add_local_repo(&ctx, ws.id.clone(), tmp.to_string_lossy().to_string())
            .expect("add_local_repo");
        assert_eq!(list_repos(&ctx, ws.id.clone()).unwrap().len(), 1);

        remove_repo(&ctx, ws.id.clone(), repo.id).expect("remove_repo");
        assert_eq!(list_repos(&ctx, ws.id).unwrap().len(), 0);

        fs::remove_dir_all(&tmp).ok();
    }

    #[test]
    fn relink_repo_updates_path() {
        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).unwrap();
        let old = std::env::temp_dir().join(format!("gitwave-uc-old-{}", std::process::id()));
        let new_p = std::env::temp_dir().join(format!("gitwave-uc-new-{}", std::process::id()));
        let _ = fs::remove_dir_all(&old);
        let _ = fs::remove_dir_all(&new_p);
        fs::create_dir_all(&old).unwrap();
        crate::infrastructure::git::repo_adapter::init(&old).expect("init old");
        fs::create_dir_all(&new_p).unwrap();
        crate::infrastructure::git::repo_adapter::init(&new_p).expect("init new");

        let repo = add_local_repo(&ctx, ws.id.clone(), old.to_string_lossy().to_string())
            .expect("add_local_repo");
        relink_repo(
            &ctx,
            ws.id.clone(),
            repo.id.clone(),
            new_p.to_string_lossy().to_string(),
        )
        .expect("relink");

        let list = list_repos(&ctx, ws.id).unwrap();
        assert_eq!(list[0].path, new_p.to_string_lossy().to_string());

        fs::remove_dir_all(&old).ok();
        fs::remove_dir_all(&new_p).ok();
    }
}
