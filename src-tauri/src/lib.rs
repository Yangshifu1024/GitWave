//! GitWave library crate entry point.
//!
//! Architecture follows `docs/tech/architecture/00-overview.md`:
//! presentation (WebView) → IPC command → application → domain ← infrastructure.
//!
//! Concrete use cases (e.g., `CreateWorkspace`, `SwitchActiveRepo`) land
//! in subsequent sprints.

// Domain types and re-exports are intentionally exposed before being
// consumed; Sprint 1 use cases will exercise them. Remove once the layers
// are wired end-to-end.
#![allow(dead_code)]
#![allow(unused_imports)]

mod application;
mod domain;
mod infrastructure;

use domain::Workspace;
use tracing::info;

/// Returns the running app version. Used by the frontend to verify the
/// IPC bridge is wired end-to-end during Sprint 0 smoke testing.
#[tauri::command]
fn get_app_version() -> String {
    env!("CARGO_PKG_VERSION").to_string()
}

/// Returns the list of workspaces. Sprint 0 returns an empty list; Sprint 1
/// will wire this through `application::AppContext` to the SQLite adapter.
#[tauri::command]
fn list_workspaces() -> Vec<Workspace> {
    Vec::new()
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let _guard = infrastructure::observability::tracing::init();
    info!("GitWave starting (version {})", env!("CARGO_PKG_VERSION"));

    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .invoke_handler(tauri::generate_handler![get_app_version, list_workspaces])
        .setup(|_app| {
            // Sprint 1 will hold the connection in `AppContext`. For now,
            // open once to confirm state dir + migrations work at startup.
            match infrastructure::persistence::sqlite::open() {
                Ok(_) => info!("SQLite state opened"),
                Err(e) => tracing::error!("failed to open SQLite state: {e}"),
            }
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
