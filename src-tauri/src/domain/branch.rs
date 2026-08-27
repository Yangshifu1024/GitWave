//! Git branch metadata. See
//! `docs/tasks/feat-history-graph/plan.md` step 1.

use serde::{Deserialize, Serialize};

/// Whether a branch is local or remote.
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum BranchKind {
    Local,
    Remote,
}

/// Summary of a git branch. Returned by `cmd_list_branches`.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct BranchInfo {
    pub name: String,
    pub kind: BranchKind,
    pub is_current: bool,
    /// Upstream tracking branch, e.g. `origin/main`. `None` for local
    /// branches without upstream, or for remote branches themselves.
    pub upstream: Option<String>,
    /// Commits in this branch that are not in the upstream.
    pub ahead: u32,
    /// Commits in the upstream that are not in this branch.
    pub behind: u32,
    /// SHA of the tip commit.
    pub last_commit_sha: String,
    /// Unix timestamp (seconds) of the tip commit. `0` if the tip is missing.
    pub last_commit_time: i64,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn branch_kind_serializes() {
        assert_eq!(
            serde_json::to_string(&BranchKind::Local).unwrap(),
            "\"local\""
        );
        assert_eq!(
            serde_json::to_string(&BranchKind::Remote).unwrap(),
            "\"remote\""
        );
    }

    #[test]
    fn branch_info_roundtrip() {
        let b = BranchInfo {
            name: "main".into(),
            kind: BranchKind::Local,
            is_current: true,
            upstream: Some("origin/main".into()),
            ahead: 2,
            behind: 0,
            last_commit_sha: "abc123".into(),
            last_commit_time: 1_700_000_000,
        };
        let json = serde_json::to_string(&b).unwrap();
        let back: BranchInfo = serde_json::from_str(&json).unwrap();
        assert_eq!(b, back);
    }

    #[test]
    fn branch_info_no_upstream() {
        let b = BranchInfo {
            name: "feature/x".into(),
            kind: BranchKind::Local,
            is_current: false,
            upstream: None,
            ahead: 0,
            behind: 0,
            last_commit_sha: "deadbeef".into(),
            last_commit_time: 1_700_000_100,
        };
        let json = serde_json::to_string(&b).unwrap();
        let back: BranchInfo = serde_json::from_str(&json).unwrap();
        assert_eq!(b, back);
        assert!(back.upstream.is_none());
    }
}
