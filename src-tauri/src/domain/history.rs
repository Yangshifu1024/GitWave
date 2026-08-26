//! History graph domain types — commit summaries, details, and file
//! change summary. See `docs/tasks/feat-history-graph/plan.md` step 1.

use serde::{Deserialize, Serialize};

/// Status of a file relative to its parent commit. Mirrors the labels
/// used in `git status --porcelain`.
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum FileStatus {
    Added,     // A
    Modified,  // M
    Deleted,   // D
    Renamed,   // R
    Copied,    // C
    Untracked, // ?
}

/// One file changed in a commit. Returned as part of `CommitDetails`
/// and (with reduced detail) as part of `FileSummary` listings.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct FileSummary {
    pub path: String,
    #[serde(default)]
    pub old_path: Option<String>, // present for Renamed
    pub kind: FileStatus,
    pub additions: u32,
    pub deletions: u32,
}

/// Kind of a named pointer decorating a commit in the History graph.
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum CommitRefKind {
    LocalBranch,
    RemoteBranch,
    Tag,
    Head,
}

/// A branch / tag / HEAD label attached to a commit tip.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct CommitRef {
    /// Short display name (`main`, `origin/main`, `v1.0.0`, `HEAD`).
    pub name: String,
    pub kind: CommitRefKind,
}

/// Lightweight commit summary used to render the commit graph. Lane is
/// assigned by `infrastructure::git::history::commit_log` so the
/// frontend can position this commit in a vertical column.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct CommitSummary {
    pub sha: String,
    pub author: String,
    pub author_email: String,
    /// Unix epoch seconds.
    pub time: i64,
    /// First line of the commit message (subject).
    pub message_summary: String,
    /// Graph lane index (0-based) assigned by the walker.
    pub lane: u32,
    /// SHAs of parent commits. Most commits have 0 or 1 parent; merge
    /// commits have 2+. Defaults to empty when omitted from serialized
    /// input (e.g. for root commits or partial server output).
    #[serde(default)]
    pub parents: Vec<String>,
    /// Branch / tag / HEAD decorations pointing at this commit.
    #[serde(default)]
    pub refs: Vec<CommitRef>,
}

/// Full commit details loaded on demand when a commit is selected.
/// Includes the full message body and the list of changed files.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct CommitDetails {
    pub sha: String,
    pub author: String,
    pub author_email: String,
    pub time: i64,
    /// Full commit message (subject + body, joined by \n\n).
    pub message_full: String,
    pub parents: Vec<String>,
    pub files: Vec<FileSummary>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn file_status_serializes_snake_case() {
        assert_eq!(
            serde_json::to_string(&FileStatus::Added).unwrap(),
            "\"added\""
        );
        assert_eq!(
            serde_json::to_string(&FileStatus::Modified).unwrap(),
            "\"modified\""
        );
        assert_eq!(
            serde_json::to_string(&FileStatus::Deleted).unwrap(),
            "\"deleted\""
        );
        assert_eq!(
            serde_json::to_string(&FileStatus::Renamed).unwrap(),
            "\"renamed\""
        );
        assert_eq!(
            serde_json::to_string(&FileStatus::Copied).unwrap(),
            "\"copied\""
        );
        assert_eq!(
            serde_json::to_string(&FileStatus::Untracked).unwrap(),
            "\"untracked\""
        );
    }

    #[test]
    fn commit_summary_roundtrip() {
        let c = CommitSummary {
            sha: "abc123".into(),
            author: "Alice".into(),
            author_email: "alice@example.com".into(),
            time: 1_700_000_000,
            message_summary: "feat: add thing".into(),
            lane: 0,
            parents: vec!["def456".into()],
            refs: vec![CommitRef {
                name: "main".into(),
                kind: CommitRefKind::LocalBranch,
            }],
        };
        let json = serde_json::to_string(&c).unwrap();
        let back: CommitSummary = serde_json::from_str(&json).unwrap();
        assert_eq!(c, back);
    }

    #[test]
    fn commit_summary_default_parents_and_refs_empty() {
        let json = r#"{
            "sha": "abc",
            "author": "x",
            "author_email": "x@y",
            "time": 0,
            "message_summary": "root",
            "lane": 0
        }"#;
        let c: CommitSummary = serde_json::from_str(json).unwrap();
        assert!(c.parents.is_empty());
        assert!(c.refs.is_empty());
    }

    #[test]
    fn commit_details_roundtrip() {
        let d = CommitDetails {
            sha: "abc".into(),
            author: "Bob".into(),
            author_email: "b@x".into(),
            time: 1_700_000_000,
            message_full: "feat: subject\n\nbody paragraph".into(),
            parents: vec!["p1".into(), "p2".into()],
            files: vec![FileSummary {
                path: "src/lib.rs".into(),
                old_path: None,
                kind: FileStatus::Modified,
                additions: 3,
                deletions: 1,
            }],
        };
        let json = serde_json::to_string(&d).unwrap();
        let back: CommitDetails = serde_json::from_str(&json).unwrap();
        assert_eq!(d, back);
    }
}
