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
    add_local_repo, add_ssh_key, checkout_branch, clone_repo, commit, create_branch,
    create_workspace, delete_branch, delete_ssh_key, delete_workspace, get_ahead_behind, get_blame,
    get_branches, get_commit_diff, get_commit_log, get_file_diff, get_workdir_diff,
    get_working_copy, init_repo, list_repos, list_ssh_keys, list_workspaces, merge_branch,
    rebase_branch, relink_repo, remove_repo, rename_workspace, set_active_repo, stage_all,
    stage_files, test_ssh_connection, unstage_files, AheadBehind, AppContext,
};
use domain::blame::BlameLine;
use domain::branch::BranchInfo;
use domain::diff::FileDiff;
use domain::error::AppError;
use domain::history::CommitSummary;
use domain::working_copy::WorkingCopy;
use domain::workspace::{RepoRef, Workspace, WorkspaceSummary};
use infrastructure::git::diff::DiffSummary;
use infrastructure::git::merge::MergeResult;
use infrastructure::git::rebase::RebaseResult;
use infrastructure::observability::tracing::init as init_tracing;
use infrastructure::persistence::{migrations, open as open_state, SqliteWorkspaceRepo};
use infrastructure::ssh::keys::{SshKey, SshTestResult};
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
fn cmd_clone_repo(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    url: String,
    dest_path: String,
) -> Result<RepoRef, AppError> {
    clone_repo(&ctx, workspace_id, url, dest_path)
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
) -> Result<(), AppError> {
    checkout_branch(&ctx, &workspace_id, &name)
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
async fn cmd_rebase_branch(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    upstream: String,
) -> Result<RebaseResult, AppError> {
    rebase_branch(&ctx, &workspace_id, &upstream)
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
        .manage(ctx)
        .invoke_handler(tauri::generate_handler![
            get_app_version,
            cmd_list_workspaces,
            cmd_create_workspace,
            cmd_rename_workspace,
            cmd_delete_workspace,
            cmd_set_active_repo,
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
            cmd_rebase_branch,
            cmd_get_working_copy,
            cmd_stage_files,
            cmd_unstage_files,
            cmd_stage_all,
            cmd_commit,
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
