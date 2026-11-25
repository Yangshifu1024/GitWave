//! Worktree domain types.

use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct WorktreeInfo {
    pub name: String,
    pub path: String,
    pub is_main: bool,
    pub is_locked: bool,
    pub branch: Option<String>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn worktree_info_serde() {
        let w = WorktreeInfo {
            name: "feature".into(),
            path: "/tmp/wt".into(),
            is_main: false,
            is_locked: false,
            branch: Some("feature".into()),
        };
        let json = serde_json::to_string(&w).unwrap();
        assert!(json.contains("feature"));
    }
}
