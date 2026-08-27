//! GitWave library crate entry point.
//!
//! Architecture follows `docs/tech/architecture/00-overview.md`:
//! presentation (WebView) → IPC command → application → domain ← infrastructure.
//!
//! Concrete use cases live in `application::use_cases`. The Tauri commands
//! below are thin wrappers that resolve state and forward to use cases.

#![allow(dead_code)]
#![allow(unused_imports)]

mod application;
mod domain;
mod infrastructure;

use std::sync::{Arc, Mutex};

use application::{
    abort_interactive_rebase_pause, abort_merge, add_local_repo, add_ssh_key, add_worktree,
    apply_stash, checkout_branch, clear_ai_api_key, clone_repo, commit,
    continue_interactive_rebase, create_branch, create_workspace, delete_branch, delete_ssh_key,
    delete_workspace, drop_stash, execute_interactive_rebase, explain_conflict, fetch,
    generate_commit_message, get_ahead_behind, get_ai_key_status, get_blame, get_branches,
    get_commit_diff, get_commit_log, get_conflict_sides, get_file_diff, get_stash_diff,
    get_workdir_diff, get_working_copy, get_workspace, init_repo, interactive_rebase_paused,
    list_conflicts, list_repos, list_ssh_keys, list_stashes, list_workspaces, list_worktrees,
    merge_branch, merge_in_progress, plan_interactive_rebase, pop_stash, probe_ollama, pull, push,
    rebase_branch, relink_repo, remove_repo, remove_worktree, rename_workspace, resolve_conflict,
    save_stash, set_active_repo, set_ai_api_key, stage_all, stage_files, test_ssh_connection,
    unstage_files, update_workspace_settings, AheadBehind, AiKeyStatus, AppContext,
};
use domain::blame::BlameLine;
use domain::branch::BranchInfo;
use domain::diff::FileDiff;
use domain::error::AppError;
use domain::history::CommitSummary;
use domain::stash::StashEntry;
use domain::working_copy::WorkingCopy;
use domain::workspace::{RepoRef, Workspace, WorkspaceSettings, WorkspaceSummary};
use domain::worktree::WorktreeInfo;
use infrastructure::git::conflict::{ConflictFile, ConflictSides};
use infrastructure::git::diff::DiffSummary;
use infrastructure::git::interactive_rebase::{InteractiveRebaseResult, InteractiveRebaseTodo};
use infrastructure::git::merge::MergeResult;
use infrastructure::git::rebase::RebaseResult;
use infrastructure::observability::tracing::init as init_tracing;
use infrastructure::persistence::{migrations, open as open_state, SqliteWorkspaceRepo};
use infrastructure::ssh::keys::{SshKey, SshTestResult};
use std::fmt::Display;
use tauri::WebviewWindow;
use tauri_plugin_decoration::WebviewWindowExt;
use tracing::info;

// ─── App meta ─────────────────────────────────────────────────────────────

/// Returns the running app version.
#[tauri::command]
fn get_app_version() -> String {
    env!("CARGO_PKG_VERSION").to_string()
}

// ─── Workspace commands (Sprint 1) ───────────────────────────────────────

#[tauri::command]
fn cmd_list_workspaces(
    ctx: tauri::State<'_, AppContext>,
) -> Result<Vec<WorkspaceSummary>, AppError> {
    list_workspaces(&ctx)
}

#[tauri::command]
fn cmd_create_workspace(
    ctx: tauri::State<'_, AppContext>,
    name: String,
) -> Result<Workspace, AppError> {
    create_workspace(&ctx, name)
}

#[tauri::command]
fn cmd_rename_workspace(
    ctx: tauri::State<'_, AppContext>,
    id: String,
    new_name: String,
) -> Result<(), AppError> {
    rename_workspace(&ctx, id, new_name)
}

#[tauri::command]
fn cmd_delete_workspace(ctx: tauri::State<'_, AppContext>, id: String) -> Result<(), AppError> {
    delete_workspace(&ctx, id)
}

#[tauri::command]
fn cmd_set_active_repo(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    repo_id: Option<String>,
) -> Result<(), AppError> {
    set_active_repo(&ctx, workspace_id, repo_id)
}

#[tauri::command]
fn cmd_get_workspace(ctx: tauri::State<'_, AppContext>, id: String) -> Result<Workspace, AppError> {
    get_workspace(&ctx, id)
}

#[tauri::command]
fn cmd_update_workspace_settings(
    ctx: tauri::State<'_, AppContext>,
    id: String,
    settings: WorkspaceSettings,
) -> Result<(), AppError> {
    update_workspace_settings(&ctx, id, settings)
}

#[tauri::command]
fn cmd_set_ai_api_key(
    workspace_id: String,
    provider: String,
    api_key: String,
) -> Result<(), AppError> {
    set_ai_api_key(workspace_id, provider, api_key)
}

#[tauri::command]
fn cmd_clear_ai_api_key(workspace_id: String, provider: String) -> Result<(), AppError> {
    clear_ai_api_key(workspace_id, provider)
}

#[tauri::command]
fn cmd_get_ai_key_status(workspace_id: String, provider: String) -> Result<AiKeyStatus, AppError> {
    get_ai_key_status(workspace_id, provider)
}

#[tauri::command]
async fn cmd_probe_ollama(base_url: Option<String>) -> Result<Vec<String>, AppError> {
    probe_ollama(base_url).await
}

#[tauri::command]
async fn cmd_generate_commit_message(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<String, AppError> {
    generate_commit_message(&ctx, workspace_id).await
}

// ─── Repo commands (Sprint 2) ────────────────────────────────────────────

#[tauri::command]
fn cmd_init_repo(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    path: String,
) -> Result<RepoRef, AppError> {
    init_repo(&ctx, workspace_id, path)
}

#[tauri::command]
async fn cmd_clone_repo(
    app: tauri::AppHandle,
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    url: String,
    dest_path: String,
    replace_dest: Option<bool>,
) -> Result<RepoRef, AppError> {
    use infrastructure::git::repo_adapter::CloneProgress;
    use tauri::Emitter;

    let replace = replace_dest.unwrap_or(false);
    let app_emit = app.clone();
    let on_progress: Option<Box<dyn Fn(CloneProgress) + Send>> =
        Some(Box::new(move |p: CloneProgress| {
            let _ = app_emit.emit("clone-progress", &p);
        }));

    let workspaces = Arc::clone(&ctx.workspaces);
    let result = tauri::async_runtime::spawn_blocking(move || {
        let local_ctx = AppContext::new(workspaces);
        clone_repo(
            &local_ctx,
            workspace_id,
            url,
            dest_path,
            replace,
            on_progress,
        )
    })
    .await
    .map_err(|e| AppError::Unknown(format!("clone task join: {e}")))?;

    result
}

#[tauri::command]
fn cmd_add_local_repo(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    path: String,
) -> Result<RepoRef, AppError> {
    add_local_repo(&ctx, workspace_id, path)
}

#[tauri::command]
fn cmd_remove_repo(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    repo_id: String,
) -> Result<(), AppError> {
    remove_repo(&ctx, workspace_id, repo_id)
}

#[tauri::command]
fn cmd_relink_repo(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    repo_id: String,
    new_path: String,
) -> Result<(), AppError> {
    relink_repo(&ctx, workspace_id, repo_id, new_path)
}

#[tauri::command]
fn cmd_list_repos(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<Vec<RepoRef>, AppError> {
    list_repos(&ctx, workspace_id)
}

// ─── SSH commands (Sprint 2) ──────────────────────────────────────────────

#[tauri::command]
fn cmd_list_ssh_keys() -> Result<Vec<SshKey>, AppError> {
    list_ssh_keys()
}

#[tauri::command]
fn cmd_add_ssh_key(path: String) -> Result<(), AppError> {
    add_ssh_key(path)
}

#[tauri::command]
fn cmd_delete_ssh_key(path: String) -> Result<(), AppError> {
    delete_ssh_key(path)
}

#[tauri::command]
fn cmd_test_ssh_connection(host: String, user: String) -> Result<SshTestResult, AppError> {
    test_ssh_connection(host, user)
}

// ─── History / Diff / Blame commands (Sprint 3) ───────────────────────────

#[tauri::command]
async fn cmd_get_commit_log(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    max: u32,
) -> Result<Vec<CommitSummary>, AppError> {
    get_commit_log(&ctx, &workspace_id, max)
}

#[tauri::command]
async fn cmd_get_workdir_diff(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<DiffSummary, AppError> {
    get_workdir_diff(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_get_commit_diff(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    commit_oid: String,
) -> Result<DiffSummary, AppError> {
    get_commit_diff(&ctx, &workspace_id, &commit_oid)
}

#[tauri::command]
async fn cmd_get_file_diff(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    from_oid: String,
    to_oid: String,
) -> Result<Vec<FileDiff>, AppError> {
    get_file_diff(&ctx, &workspace_id, &from_oid, &to_oid)
}

#[tauri::command]
async fn cmd_get_blame(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    path: String,
) -> Result<Vec<BlameLine>, AppError> {
    get_blame(&ctx, &workspace_id, &path)
}

// ─── Branch commands (Sprint 3) ───────────────────────────────────────────

#[tauri::command]
async fn cmd_get_branches(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<Vec<BranchInfo>, AppError> {
    get_branches(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_create_branch(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
    from_sha: String,
) -> Result<BranchInfo, AppError> {
    create_branch(&ctx, &workspace_id, &name, &from_sha)
}

#[tauri::command]
async fn cmd_delete_branch(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
) -> Result<(), AppError> {
    delete_branch(&ctx, &workspace_id, &name)
}

#[tauri::command]
async fn cmd_checkout_branch(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
    force: Option<bool>,
) -> Result<(), AppError> {
    checkout_branch(&ctx, &workspace_id, &name, force.unwrap_or(false))
}

#[tauri::command]
async fn cmd_get_ahead_behind(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    branch_name: String,
) -> Result<AheadBehind, AppError> {
    get_ahead_behind(&ctx, &workspace_id, &branch_name)
}

#[tauri::command]
async fn cmd_merge_branch(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    branch_name: String,
) -> Result<MergeResult, AppError> {
    merge_branch(&ctx, &workspace_id, &branch_name)
}

#[tauri::command]
async fn cmd_list_conflicts(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<Vec<ConflictFile>, AppError> {
    list_conflicts(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_get_conflict_sides(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    path: String,
) -> Result<ConflictSides, AppError> {
    get_conflict_sides(&ctx, &workspace_id, path)
}

#[tauri::command]
async fn cmd_resolve_conflict(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    path: String,
    content: String,
) -> Result<(), AppError> {
    resolve_conflict(&ctx, &workspace_id, path, content)
}

#[tauri::command]
async fn cmd_abort_merge(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<(), AppError> {
    abort_merge(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_merge_in_progress(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<bool, AppError> {
    merge_in_progress(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_explain_conflict(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    path: String,
) -> Result<String, AppError> {
    explain_conflict(&ctx, workspace_id, path).await
}

#[tauri::command]
async fn cmd_rebase_branch(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    upstream: String,
) -> Result<RebaseResult, AppError> {
    rebase_branch(&ctx, &workspace_id, &upstream)
}

#[tauri::command]
async fn cmd_plan_interactive_rebase(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    upstream: String,
) -> Result<Vec<InteractiveRebaseTodo>, AppError> {
    plan_interactive_rebase(&ctx, &workspace_id, &upstream)
}

#[tauri::command]
async fn cmd_execute_interactive_rebase(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    upstream: String,
    todos: Vec<InteractiveRebaseTodo>,
) -> Result<InteractiveRebaseResult, AppError> {
    execute_interactive_rebase(&ctx, &workspace_id, &upstream, todos)
}

#[tauri::command]
async fn cmd_continue_interactive_rebase(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<InteractiveRebaseResult, AppError> {
    continue_interactive_rebase(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_abort_interactive_rebase_pause(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<(), AppError> {
    abort_interactive_rebase_pause(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_interactive_rebase_paused(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<bool, AppError> {
    interactive_rebase_paused(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_get_working_copy(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<WorkingCopy, AppError> {
    get_working_copy(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_stage_files(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    paths: Vec<String>,
) -> Result<(), AppError> {
    stage_files(&ctx, &workspace_id, paths)
}

#[tauri::command]
async fn cmd_unstage_files(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    paths: Vec<String>,
) -> Result<(), AppError> {
    unstage_files(&ctx, &workspace_id, paths)
}

#[tauri::command]
async fn cmd_stage_all(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<(), AppError> {
    stage_all(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_commit(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    message: String,
) -> Result<String, AppError> {
    commit(&ctx, &workspace_id, message)
}

#[tauri::command]
async fn cmd_fetch(
    app: tauri::AppHandle,
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    remote: Option<String>,
) -> Result<(), AppError> {
    use application::use_cases::fetch;
    use infrastructure::git::remote::SyncProgress;
    use std::sync::Arc;
    use tauri::Emitter;

    let app_emit = app.clone();
    let on_progress: Option<Box<dyn Fn(SyncProgress) + Send>> =
        Some(Box::new(move |p: SyncProgress| {
            let _ = app_emit.emit("sync-progress", &p);
        }));

    let workspaces = Arc::clone(&ctx.workspaces);
    tauri::async_runtime::spawn_blocking(move || {
        let local_ctx = AppContext::new(workspaces);
        fetch(&local_ctx, &workspace_id, remote, on_progress)
    })
    .await
    .map_err(|e| AppError::Unknown(format!("fetch task join: {e}")))?
}

#[tauri::command]
async fn cmd_pull(
    app: tauri::AppHandle,
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    remote: Option<String>,
) -> Result<(), AppError> {
    use application::use_cases::pull;
    use infrastructure::git::remote::SyncProgress;
    use std::sync::Arc;
    use tauri::Emitter;

    let app_emit = app.clone();
    let on_progress: Option<Box<dyn Fn(SyncProgress) + Send>> =
        Some(Box::new(move |p: SyncProgress| {
            let _ = app_emit.emit("sync-progress", &p);
        }));

    let workspaces = Arc::clone(&ctx.workspaces);
    tauri::async_runtime::spawn_blocking(move || {
        let local_ctx = AppContext::new(workspaces);
        pull(&local_ctx, &workspace_id, remote, on_progress)
    })
    .await
    .map_err(|e| AppError::Unknown(format!("pull task join: {e}")))?
}

#[tauri::command]
async fn cmd_push(
    app: tauri::AppHandle,
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    remote: Option<String>,
) -> Result<(), AppError> {
    use application::use_cases::push;
    use infrastructure::git::remote::SyncProgress;
    use std::sync::Arc;
    use tauri::Emitter;

    let app_emit = app.clone();
    let on_progress: Option<Box<dyn Fn(SyncProgress) + Send>> =
        Some(Box::new(move |p: SyncProgress| {
            let _ = app_emit.emit("sync-progress", &p);
        }));

    let workspaces = Arc::clone(&ctx.workspaces);
    tauri::async_runtime::spawn_blocking(move || {
        let local_ctx = AppContext::new(workspaces);
        push(&local_ctx, &workspace_id, remote, on_progress)
    })
    .await
    .map_err(|e| AppError::Unknown(format!("push task join: {e}")))?
}

#[tauri::command]
async fn cmd_list_stashes(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<Vec<StashEntry>, AppError> {
    list_stashes(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_save_stash(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    message: Option<String>,
) -> Result<String, AppError> {
    save_stash(&ctx, &workspace_id, message)
}

#[tauri::command]
async fn cmd_apply_stash(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    index: u32,
) -> Result<(), AppError> {
    apply_stash(&ctx, &workspace_id, index)
}

#[tauri::command]
async fn cmd_pop_stash(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    index: u32,
) -> Result<(), AppError> {
    pop_stash(&ctx, &workspace_id, index)
}

#[tauri::command]
async fn cmd_drop_stash(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    index: u32,
) -> Result<(), AppError> {
    drop_stash(&ctx, &workspace_id, index)
}

#[tauri::command]
async fn cmd_get_stash_diff(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    oid: String,
) -> Result<DiffSummary, AppError> {
    get_stash_diff(&ctx, &workspace_id, &oid)
}

#[tauri::command]
async fn cmd_list_worktrees(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<Vec<WorktreeInfo>, AppError> {
    list_worktrees(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_add_worktree(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
    path: String,
    branch: String,
    create_branch: bool,
) -> Result<WorktreeInfo, AppError> {
    add_worktree(&ctx, &workspace_id, name, path, branch, create_branch)
}

#[tauri::command]
async fn cmd_remove_worktree(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
) -> Result<(), AppError> {
    remove_worktree(&ctx, &workspace_id, name)
}

// ─── Window decoration (tauri-plugin-decoration v3) ───────────────────────

async fn restore_and_show(
    window: &WebviewWindow,
    activation_error: impl Display,
) -> Result<&'static str, String> {
    let restore_error = window.restore_decoration().await.err();
    let show_error = window.show().err();

    match (restore_error, show_error) {
        (None, None) => Ok("native"),
        (Some(restore), None) => Err(format!(
            "custom decoration failed ({activation_error}); native restoration failed ({restore}), but the window was revealed"
        )),
        (None, Some(show)) => Err(format!(
            "custom decoration failed ({activation_error}); native fallback could not be revealed ({show})"
        )),
        (Some(restore), Some(show)) => Err(format!(
            "custom decoration failed ({activation_error}); native restoration failed ({restore}); revealing the fallback also failed ({show})"
        )),
    }
}

#[tauri::command]
async fn activate_and_show(window: WebviewWindow) -> Result<&'static str, String> {
    if let Err(error) = window.activate_decoration().await {
        return restore_and_show(&window, error).await;
    }

    #[cfg(target_os = "macos")]
    if let Err(error) = window.set_traffic_lights_inset(14.0, 14.0).await {
        return restore_and_show(&window, error).await;
    }

    match window.show() {
        Ok(()) => Ok("custom"),
        Err(error) => restore_and_show(&window, error).await,
    }
}

// ─── App startup ──────────────────────────────────────────────────────────

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let _guard = init_tracing();
    info!("GitWave starting (version {})", env!("CARGO_PKG_VERSION"));

    let ctx = match open_state() {
        Ok(conn) => AppContext::new(Arc::new(Mutex::new(SqliteWorkspaceRepo::new(conn)))),
        Err(e) => {
            tracing::error!("failed to open SQLite state, falling back to in-memory: {e}");
            let conn = rusqlite::Connection::open_in_memory().expect("in-memory fallback");
            migrations::apply(&conn).expect("fallback migrations");
            AppContext::new(Arc::new(Mutex::new(SqliteWorkspaceRepo::new(conn))))
        }
    };

    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_decoration::init())
        .manage(ctx)
        .invoke_handler(tauri::generate_handler![
            activate_and_show,
            get_app_version,
            cmd_list_workspaces,
            cmd_create_workspace,
            cmd_rename_workspace,
            cmd_delete_workspace,
            cmd_set_active_repo,
            cmd_get_workspace,
            cmd_update_workspace_settings,
            cmd_set_ai_api_key,
            cmd_clear_ai_api_key,
            cmd_get_ai_key_status,
            cmd_probe_ollama,
            cmd_generate_commit_message,
            cmd_init_repo,
            cmd_clone_repo,
            cmd_add_local_repo,
            cmd_remove_repo,
            cmd_relink_repo,
            cmd_list_repos,
            cmd_list_ssh_keys,
            cmd_add_ssh_key,
            cmd_delete_ssh_key,
            cmd_test_ssh_connection,
            cmd_get_commit_log,
            cmd_get_workdir_diff,
            cmd_get_commit_diff,
            cmd_get_file_diff,
            cmd_get_blame,
            cmd_get_branches,
            cmd_create_branch,
            cmd_delete_branch,
            cmd_checkout_branch,
            cmd_get_ahead_behind,
            cmd_merge_branch,
            cmd_list_conflicts,
            cmd_get_conflict_sides,
            cmd_resolve_conflict,
            cmd_abort_merge,
            cmd_merge_in_progress,
            cmd_explain_conflict,
            cmd_rebase_branch,
            cmd_plan_interactive_rebase,
            cmd_execute_interactive_rebase,
            cmd_continue_interactive_rebase,
            cmd_abort_interactive_rebase_pause,
            cmd_interactive_rebase_paused,
            cmd_get_working_copy,
            cmd_stage_files,
            cmd_unstage_files,
            cmd_stage_all,
            cmd_commit,
            cmd_fetch,
            cmd_pull,
            cmd_push,
            cmd_list_stashes,
            cmd_save_stash,
            cmd_apply_stash,
            cmd_pop_stash,
            cmd_drop_stash,
            cmd_get_stash_diff,
            cmd_list_worktrees,
            cmd_add_worktree,
            cmd_remove_worktree,
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
