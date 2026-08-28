//! Git LFS domain types.

use serde::{Deserialize, Serialize};

/// Snapshot of the active repository's Git LFS state for the LFS panel.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct LfsStatus {
    /// A working `git lfs` binary is on PATH.
    pub available: bool,
    /// This repository's local config wires the LFS filters.
    pub installed: bool,
    /// Patterns tracked via `.gitattributes` (`filter=lfs`).
    pub patterns: Vec<String>,
}
