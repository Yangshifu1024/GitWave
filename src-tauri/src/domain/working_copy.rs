//! Working copy domain types — file status and staged/unstaged changes.

use serde::{Deserialize, Serialize};

/// File status kind (mirrors `git status --porcelain` letter codes).
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum FileStatusKind {
    Modified,
    Added,
    Deleted,
    Untracked,
    Renamed,
    Copied,
}

/// One path change in the working copy (staged or unstaged).
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct FileChange {
    pub path: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub old_path: Option<String>,
    pub kind: FileStatusKind,
    pub staged: bool,
    pub additions: u32,
    pub deletions: u32,
}

/// Snapshot of the active repo working tree + index.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct WorkingCopy {
    pub repo_id: String,
    pub branch: String,
    pub upstream: Option<String>,
    pub sha: String,
    pub ahead: u32,
    pub behind: u32,
    pub files: Vec<FileChange>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn file_status_kind_snake_case() {
        let json = serde_json::to_string(&FileStatusKind::Untracked).unwrap();
        assert_eq!(json, "\"untracked\"");
    }
}
