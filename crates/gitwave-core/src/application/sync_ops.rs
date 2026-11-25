//! Cancel registry and timeout budget for network sync operations
//! (fetch / pull / push / clone / delete-remote-branch / submodule update
//! / submodule add).
//!
//! libgit2 has no socket timeouts: a hung transport blocks its thread
//! forever, which used to pin the UI at "Fetching…" and — via the
//! per-workspace fetch lock — starve every later sync. [`run_sync_op`]
//! therefore races each blocking operation against [`SYNC_OP_TIMEOUT`] and
//! keeps this per-workspace registry of cancel flags so the UI can abort an
//! in-flight operation (`cancel_workspace_ops`).
//!
//! A set flag is observed at the next libgit2 checkpoint — the
//! transfer-progress callback (returning `false` aborts the transfer) or the
//! `git credential fill` wait — which also releases the workspace fetch
//! lock for the next operation. A transport with zero responsiveness
//! (e.g. a dead TCP connect) reaches no checkpoint; the thread then lingers
//! until app restart, but the operation itself still returns promptly with
//! `git.sync_timeout`.

use std::collections::HashMap;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex, OnceLock};
use std::time::Duration;

use super::use_cases::AppContext;
use crate::domain::error::AppError;
use crate::domain::error_codes as codes;

/// Wall-clock budget for one network sync operation, enforced at the
/// command layer. Generous on purpose: incremental syncs finish far below
/// it, and the cost of exceeding it is aborting a legitimately slow
/// transfer (the user can retry).
pub const SYNC_OP_TIMEOUT: Duration = Duration::from_secs(180);

/// Wall-clock budget for a whole-workspace fetch sweep (auto-refresh), which
/// fetches every repo serially. Larger than [`SYNC_OP_TIMEOUT`] so a handful
/// of repos can each consume a normal fetch budget; the timeout still bounds
/// a swept workspace whose transports hang.
pub const WORKSPACE_FETCH_TIMEOUT: Duration = Duration::from_secs(300);

type Flag = Arc<AtomicBool>;

fn registry() -> &'static Mutex<HashMap<String, Vec<Flag>>> {
    static REGISTRY: OnceLock<Mutex<HashMap<String, Vec<Flag>>>> = OnceLock::new();
    REGISTRY.get_or_init(|| Mutex::new(HashMap::new()))
}

/// Unregisters its flag on drop. The guard lives inside the blocking
/// closure, so a registry entry exactly matches the operation's real
/// lifetime — including operations whose command already returned after a
/// timeout while the closure is still winding down.
pub struct CancelGuard {
    workspace_id: String,
    flag: Flag,
}

impl Drop for CancelGuard {
    fn drop(&mut self) {
        let mut registry = match registry().lock() {
            Ok(guard) => guard,
            Err(poisoned) => poisoned.into_inner(),
        };
        if let Some(flags) = registry.get_mut(&self.workspace_id) {
            flags.retain(|f| !Arc::ptr_eq(f, &self.flag));
            if flags.is_empty() {
                registry.remove(&self.workspace_id);
            }
        }
    }
}

/// Register `flag` as an in-flight operation of `workspace_id`. The caller
/// moves the returned guard into the blocking closure; the entry stays
/// visible to [`cancel_workspace_ops`] until that closure finishes.
pub fn register(workspace_id: &str, flag: &Flag) -> CancelGuard {
    let mut registry = match registry().lock() {
        Ok(guard) => guard,
        Err(poisoned) => poisoned.into_inner(),
    };
    registry
        .entry(workspace_id.to_string())
        .or_default()
        .push(Arc::clone(flag));
    CancelGuard {
        workspace_id: workspace_id.to_string(),
        flag: Arc::clone(flag),
    }
}

/// Flag every in-flight sync operation of `workspace_id` as cancelled and
/// report whether one existed. The operation then fails at its next
/// checkpoint with `git.sync_cancelled`; losing the race against an
/// operation that already finished is harmless.
pub fn cancel_workspace_ops(workspace_id: &str) -> bool {
    let registry = match registry().lock() {
        Ok(guard) => guard,
        Err(poisoned) => poisoned.into_inner(),
    };
    let Some(flags) = registry.get(workspace_id) else {
        return false;
    };
    if flags.is_empty() {
        return false;
    }
    for flag in flags {
        flag.store(true, Ordering::Relaxed);
    }
    true
}

/// Error for a sync operation that exceeded [`SYNC_OP_TIMEOUT`].
pub fn timeout_error(operation: &str) -> AppError {
    AppError::network_with(
        codes::git::SYNC_TIMEOUT,
        format!("{operation} timed out after {}s", SYNC_OP_TIMEOUT.as_secs()),
        &[("seconds", SYNC_OP_TIMEOUT.as_secs().to_string())],
    )
}

/// Run a network sync operation (fetch / pull / push / clone /
/// delete-remote-branch / submodule update / submodule add)
/// on the blocking pool behind its workspace's cancel flag and a total
/// timeout, so a hung libgit2 transport can never pin the UI at "Fetching…"
/// forever. On timeout the flag is set — the blocking task aborts at its
/// next libgit2 checkpoint (transfer-progress callback or credential fill
/// wait) and thereby releases the workspace fetch lock — while the caller
/// reports `git.sync_timeout` immediately. The same flag backs
/// `cancel_workspace_ops`.
pub async fn run_sync_op<T, F>(
    workspace_id: &str,
    operation: &'static str,
    join_error_code: &'static str,
    timeout: Duration,
    ctx: &AppContext,
    op: F,
) -> Result<T, AppError>
where
    T: Send + 'static,
    F: FnOnce(&AppContext, Arc<AtomicBool>) -> Result<T, AppError> + Send + 'static,
{
    let workspaces = Arc::clone(&ctx.workspaces);
    let app_settings = Arc::clone(&ctx.app_settings);
    let flag = Arc::new(AtomicBool::new(false));
    let inner = Arc::clone(&flag);
    // The guard lives inside the closure so the registry entry matches the
    // operation's real lifetime — a timed-out caller returns early while
    // its closure may still be winding down.
    let guard = register(workspace_id, &flag);
    let handle = tokio::task::spawn_blocking(move || {
        let _guard = guard;
        let local_ctx = AppContext::new(workspaces, app_settings);
        op(&local_ctx, inner)
    });
    match tokio::time::timeout(timeout, handle).await {
        // JoinHandle settles to Result<Result<T, AppError>, JoinError>:
        // map the join failure, then flatten the operation's own result.
        Ok(result) => result
            .map_err(|e| {
                AppError::unknown_with(
                    join_error_code,
                    format!("{operation} task join: {e}"),
                    &[("error", e.to_string())],
                )
            })
            .and_then(std::convert::identity),
        Err(_) => {
            flag.store(true, Ordering::Relaxed);
            Err(timeout_error(operation))
        }
    }
}

/// Run one blocking local Git read on the blocking pool.
pub async fn local_git_task<T: Send + 'static>(
    job: impl FnOnce() -> Result<T, AppError> + Send + 'static,
) -> Result<T, AppError> {
    tokio::task::spawn_blocking(job).await.map_err(|e| {
        AppError::unknown_with(
            codes::git::GIT_ERROR,
            "local Git task failed",
            &[("error", e.to_string())],
        )
    })?
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::persistence::{
        migrations, SqliteAppSettingsRepo, SqliteWorkspaceRepo,
    };
    use rusqlite::Connection;
    use std::sync::Mutex;

    fn fresh_ctx() -> AppContext {
        let conn = Connection::open_in_memory().expect("in-memory sqlite");
        migrations::apply(&conn).expect("migrations");
        let app_settings = SqliteAppSettingsRepo::open_in_memory().expect("in-memory app settings");
        AppContext::new(
            Arc::new(Mutex::new(SqliteWorkspaceRepo::new(conn))),
            Arc::new(Mutex::new(app_settings)),
        )
    }

    #[test]
    fn cancel_flips_registered_flags_only() {
        let flag = Flag::default();
        let other = Flag::default();
        let _guard = register("ws-cancel-a", &flag);
        let _guard_b = register("ws-cancel-b", &other);

        assert!(cancel_workspace_ops("ws-cancel-a"), "an op was in flight");
        assert!(flag.load(Ordering::Relaxed), "its flag must flip");
        assert!(!other.load(Ordering::Relaxed), "other workspaces stay");

        assert!(!cancel_workspace_ops("ws-cancel-none"), "no op registered");
    }

    #[test]
    fn dropped_guard_unregisters_its_flag() {
        let flag = Flag::default();
        let guard = register("ws-cancel-c", &flag);
        drop(guard);
        assert!(
            !cancel_workspace_ops("ws-cancel-c"),
            "dropped guard must remove the flag"
        );
    }

    #[test]
    fn timeout_error_is_network_with_seconds_param() {
        let err = timeout_error("fetch");
        assert_eq!(err.category(), "Network");
        assert_eq!(err.code(), codes::git::SYNC_TIMEOUT);
    }

    #[tokio::test]
    async fn run_sync_op_returns_the_operation_result() {
        let ctx = fresh_ctx();
        let result = run_sync_op(
            "ws-run-ok",
            "fetch",
            codes::git::GIT_ERROR,
            SYNC_OP_TIMEOUT,
            &ctx,
            |_: &AppContext, cancel: Flag| -> Result<bool, AppError> {
                Ok(cancel.load(Ordering::Relaxed))
            },
        )
        .await;
        assert!(
            !result.expect("operation result"),
            "a fresh flag is not cancelled"
        );
        assert!(
            !cancel_workspace_ops("ws-run-ok"),
            "the finished operation unregistered its flag"
        );
    }

    #[tokio::test]
    async fn run_sync_op_timeout_flags_the_operation_so_it_unwinds() {
        let ctx = fresh_ctx();
        let (unwound, waited) = std::sync::mpsc::channel();
        let err = run_sync_op(
            "ws-run-timeout",
            "fetch",
            codes::git::GIT_ERROR,
            Duration::from_millis(20),
            &ctx,
            move |_: &AppContext, cancel: Flag| -> Result<(), AppError> {
                while !cancel.load(Ordering::Relaxed) {
                    std::thread::sleep(Duration::from_millis(2));
                }
                let _ = unwound.send(());
                Err(timeout_error("fetch"))
            },
        )
        .await
        .expect_err("a hung operation must time out");
        assert_eq!(err.code(), codes::git::SYNC_TIMEOUT);
        waited
            .recv_timeout(Duration::from_secs(5))
            .expect("the timeout sets the flag the operation waits on");
        // The operation signals just before it returns, and its flag is
        // unregistered only once the blocking closure has finished.
        let deadline = std::time::Instant::now() + Duration::from_secs(5);
        while cancel_workspace_ops("ws-run-timeout") {
            assert!(
                std::time::Instant::now() < deadline,
                "the unwound operation unregistered its flag"
            );
            std::thread::sleep(Duration::from_millis(2));
        }
    }

    #[tokio::test]
    async fn run_sync_op_reports_a_panicking_operation_as_a_join_error() {
        let ctx = fresh_ctx();
        let err = run_sync_op(
            "ws-run-panic",
            "fetch",
            codes::git::GIT_ERROR,
            SYNC_OP_TIMEOUT,
            &ctx,
            |_: &AppContext, _: Flag| -> Result<bool, AppError> { panic!("boom") },
        )
        .await
        .expect_err("a panicking operation must not hang its caller");
        assert_eq!(err.code(), codes::git::GIT_ERROR);
    }

    #[tokio::test]
    async fn local_git_task_runs_its_job_and_surfaces_a_panic_as_an_error() {
        assert_eq!(local_git_task(|| Ok(7)).await.expect("job result"), 7);
        let err = local_git_task::<bool>(|| panic!("boom"))
            .await
            .expect_err("a panicking job must not hang its caller");
        assert_eq!(err.code(), codes::git::GIT_ERROR);
    }
}
