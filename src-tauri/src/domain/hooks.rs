//! Git hooks domain types.

use serde::{Deserialize, Serialize};

/// One known git hook and whether it is present in `.git/hooks`.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct HookInfo {
    pub name: String,
    /// The hook file exists (not just a `.sample`).
    pub exists: bool,
    /// Executable bit set (always false on Windows — no exec bit there).
    pub executable: bool,
}
