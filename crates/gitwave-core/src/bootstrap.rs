//! Process-wide startup that every presentation layer performs before its
//! first use case: the state stores, libgit2 owner validation and the proxy
//! environment bridge.

use std::sync::{Arc, Mutex};

use crate::application::use_cases::{get_proxy_settings, AppContext};
use crate::infrastructure::persistence::{
    migrations, open as open_state, SqliteAppSettingsRepo, SqliteWorkspaceRepo,
};
use crate::infrastructure::proxy;

/// libgit2's owner check (the safe.directory equivalent) rejects repos owned
/// by another local account — e.g. an elevated clone leaves the dir owned by
/// BUILTIN\Administrators. Root repo paths always come from the user's
/// explicit folder pick; derived paths (submodules, worktrees) share the
/// parent repo's trust decision. libgit2 doesn't auto-run hooks, so disable
/// the check process-wide. The check is cross-platform (uid on POSIX, SID on
/// Windows); the git2 wrapper always returns Ok, so failure is not
/// observable.
pub fn disable_git_owner_validation() {
    let _ = unsafe { git2::opts::set_verify_owner_validation(false) };
    tracing::info!("libgit2 owner validation disabled");
}

/// Opens the app-level settings and the workspace state. Each falls back to
/// an in-memory store when its on-disk database is unusable, so the UI stays
/// fully functional.
#[must_use]
pub fn open_app_context() -> AppContext {
    let app_settings = match SqliteAppSettingsRepo::open() {
        Ok(repo) => Arc::new(Mutex::new(repo)),
        Err(e) => {
            tracing::error!("failed to open app settings store, falling back to in-memory: {e}");
            Arc::new(Mutex::new(
                SqliteAppSettingsRepo::open_in_memory().expect("in-memory app settings store"),
            ))
        }
    };

    match open_state() {
        Ok(conn) => AppContext::new(
            Arc::new(Mutex::new(SqliteWorkspaceRepo::new(conn))),
            Arc::clone(&app_settings),
        ),
        Err(e) => {
            tracing::error!("failed to open SQLite state, falling back to in-memory: {e}");
            let conn = rusqlite::Connection::open_in_memory().expect("in-memory fallback");
            migrations::apply(&conn).expect("fallback migrations");
            AppContext::new(
                Arc::new(Mutex::new(SqliteWorkspaceRepo::new(conn))),
                app_settings,
            )
        }
    }
}

/// Bridges the configured proxy into the process environment before anything
/// can construct a network client (AI singletons are lazy, libgit2 reads env
/// per operation). Only the proxy defaults off when this fails.
pub fn bridge_proxy_to_env(ctx: &AppContext) {
    if let Err(e) = get_proxy_settings(ctx).map(|s| proxy::apply_to_env(&s)) {
        tracing::warn!("failed to read proxy settings, env bridge skipped: {e}");
    }
}
