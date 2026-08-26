//! GitWave library crate entry point.
//!
//! Architecture follows `docs/tech/architecture/00-overview.md`:
//! presentation (WebView) → IPC command → application → domain ← infrastructure.
//!
//! Concrete use cases (e.g., `CreateWorkspace`, `SwitchActiveRepo`) live in
//! `application::use_cases`. The Tauri commands below are thin wrappers
//! that resolve state and forward to use cases.

// Domain types and re-exports are intentionally exposed before being
// consumed; Sprint 1 use cases consume `Workspace`, `WorkspaceSummary`, and
// `AppError`. The allow attributes can be tightened once everything is wired.
#![allow(dead_code)]
#![allow(unused_imports)]

mod application;
mod domain;
mod infrastructure;

use std::sync::{Arc, Mutex};

use application::{
    create_workspace, delete_workspace, list_workspaces, rename_workspace, set_active_repo,
    AppContext,
};
use domain::{Workspace, WorkspaceSummary};
use infrastructure::observability::tracing::init as init_tracing;
use infrastructure::persistence::{migrations, open as open_state, SqliteWorkspaceRepo};
use tracing::info;

/// Returns the running app version.
#[tauri::command]
fn get_app_version() -> String {
    env!("CARGO_PKG_VERSION").to_string()
}

#[tauri::command]
fn cmd_list_workspaces(
    ctx: tauri::State<'_, AppContext>,
) -> Result<Vec<WorkspaceSummary>, domain::error::AppError> {
    list_workspaces(&ctx)
}

#[tauri::command]
fn cmd_create_workspace(
    ctx: tauri::State<'_, AppContext>,
    name: String,
) -> Result<Workspace, domain::error::AppError> {
    create_workspace(&ctx, name)
}

#[tauri::command]
fn cmd_rename_workspace(
    ctx: tauri::State<'_, AppContext>,
    id: String,
    new_name: String,
) -> Result<(), domain::error::AppError> {
    rename_workspace(&ctx, id, new_name)
}

#[tauri::command]
fn cmd_delete_workspace(
    ctx: tauri::State<'_, AppContext>,
    id: String,
) -> Result<(), domain::error::AppError> {
    delete_workspace(&ctx, id)
}

#[tauri::command]
fn cmd_set_active_repo(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    repo_id: Option<String>,
) -> Result<(), domain::error::AppError> {
    set_active_repo(&ctx, workspace_id, repo_id)
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let _guard = init_tracing();
    info!("GitWave starting (version {})", env!("CARGO_PKG_VERSION"));

    let ctx = match open_state() {
        Ok(conn) => AppContext::new(Arc::new(Mutex::new(SqliteWorkspaceRepo::new(conn)))),
        Err(e) => {
            tracing::error!("failed to open SQLite state, falling back to in-memory: {e}");
            // Sprint 1 fallback: in-memory DB. State will not persist
            // across restarts in this fallback path. v0.2 should surface
            // a proper error UI instead.
            let conn = rusqlite::Connection::open_in_memory().expect("in-memory fallback");
            migrations::apply(&conn).expect("fallback migrations");
            AppContext::new(Arc::new(Mutex::new(SqliteWorkspaceRepo::new(conn))))
        }
    };

    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .manage(ctx)
        .invoke_handler(tauri::generate_handler![
            get_app_version,
            cmd_list_workspaces,
            cmd_create_workspace,
            cmd_rename_workspace,
            cmd_delete_workspace,
            cmd_set_active_repo,
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
