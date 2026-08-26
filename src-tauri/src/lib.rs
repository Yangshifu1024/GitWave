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
    add_local_repo, add_ssh_key, clone_repo, create_workspace, delete_ssh_key, delete_workspace,
    init_repo, list_repos, list_ssh_keys, list_workspaces, relink_repo, remove_repo,
    rename_workspace, set_active_repo, test_ssh_connection, AppContext,
};
use domain::error::AppError;
use domain::workspace::{RepoRef, Workspace, WorkspaceSummary};
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

// ─── SSH commands (Sprint 2) — no state ──────────────────────────────────

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
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
