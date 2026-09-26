//! Git hooks domain types.

use serde::{Deserialize, Serialize};

/// One known git hook in the effective Git hooks directory.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct HookInfo {
    pub name: String,
    /// Actual location after resolving core.hooksPath and linked worktrees.
    pub actual_path: String,
    /// The hook file exists (not just a `.sample`).
    pub exists: bool,
    /// Executable bit set (always false on Windows — no exec bit there).
    pub executable: bool,
}
