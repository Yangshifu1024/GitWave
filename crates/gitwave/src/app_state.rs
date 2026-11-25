//! Application-wide state: the gitwave-core context every view shares.

use gitwave_core::application::use_cases::{AppContext, list_workspaces};
use gitwave_core::domain::workspace::WorkspaceSummary;
use gpui_kit::{App, AppContext as _, Global, Task};

use crate::runtime;

struct AppState {
    context: AppContext,
    /// Where the state lives. Shown in the status bar so that nobody takes an
    /// isolated build for one that reads the real workspaces.
    storage: &'static str,
}

impl Global for AppState {}

/// Opens the state. The application is not ready to take over the user's data,
/// so its stores live in memory and nothing on disk is read or written.
pub fn init(cx: &mut App) {
    let context = AppContext::in_memory().expect("failed to open the in-memory state");
    cx.set_global(AppState {
        context,
        storage: "In-memory state, nothing is saved",
    });
}

pub fn storage(cx: &App) -> &'static str {
    cx.global::<AppState>().storage
}

/// The workspaces, read off the main thread.
pub fn workspaces(cx: &App) -> Task<Result<Vec<WorkspaceSummary>, String>> {
    let context = cx.global::<AppState>().context.clone();
    let task = runtime::blocking(cx, move || {
        list_workspaces(&context).map_err(|error| error.message())
    });
    cx.background_spawn(async move { task.await.map_err(|join| join.to_string())? })
}
