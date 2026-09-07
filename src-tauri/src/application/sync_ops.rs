//! Cancel registry and timeout budget for network sync operations
//! (fetch / pull / push / clone / delete-remote-branch / submodule update
//! / submodule add).
//!
//! libgit2 has no socket timeouts: a hung transport blocks its thread
//! forever, which used to pin the UI at "Fetching…" and — via the
//! per-workspace fetch lock — starve every later sync. The command layer
//! therefore races each blocking operation against [`SYNC_OP_TIMEOUT`] and
//! keeps this per-workspace registry of cancel flags so the UI can abort an
//! in-flight operation (`cmd_cancel_sync`).
//!
//! A set flag is observed at the next libgit2 checkpoint — the
//! transfer-progress callback (returning `false` aborts the transfer) or the
//! `git credential fill` wait — which also releases the workspace fetch
//! lock for the next operation. A transport with zero responsiveness
//! (e.g. a dead TCP connect) reaches no checkpoint; the thread then lingers
//! until app restart, but the command itself still returns promptly with
//! `git.sync_timeout`.

use std::collections::HashMap;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex, OnceLock};
use std::time::Duration;

use crate::domain::error::AppError;
use crate::domain::error_codes as codes;

/// Wall-clock budget for one network sync operation, enforced at the
/// command layer. Generous on purpose: incremental syncs finish far below
/// it, and the cost of exceeding it is aborting a legitimately slow
/// transfer (the user can retry).
pub const SYNC_OP_TIMEOUT: Duration = Duration::from_secs(180);

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

#[cfg(test)]
mod tests {
    use super::*;

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
}
