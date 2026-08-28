//! Reflog domain types — entries of `HEAD` / branch movement history.

use serde::{Deserialize, Serialize};

/// One reflog entry (who moved the reference, from where, to where).
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct ReflogEntry {
    /// Previous target; `None` for the zero OID (branch/HEAD creation).
    pub old_sha: Option<String>,
    /// New target after the move.
    pub new_sha: String,
    /// Human-readable reason (`commit`, `checkout`, `reset`, …).
    pub message: Option<String>,
    pub committer: String,
    /// Unix epoch seconds.
    pub time: i64,
}
