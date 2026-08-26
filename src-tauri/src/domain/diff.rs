//! File diff domain types. See
//! `docs/tasks/feat-history-graph/plan.md` step 1.

use serde::{Deserialize, Serialize};

/// Per-line classification in a diff hunk.
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "snake_case")]
pub enum DiffLineKind {
    Added,   // +
    Removed, // -
    Context, // ' '
}

/// A single line in a diff hunk.
/// - `old_line_no` is `None` for `Added` lines.
/// - `new_line_no` is `None` for `Removed` lines.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct DiffLine {
    pub kind: DiffLineKind,
    pub content: String,
    pub old_line_no: Option<u32>,
    pub new_line_no: Option<u32>,
}

/// One contiguous changed region in a file diff.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct DiffHunk {
    pub old_start: u32,
    pub old_lines: u32,
    pub new_start: u32,
    pub new_lines: u32,
    pub lines: Vec<DiffLine>,
}

/// Full diff for one file between two revisions (commit vs parent,
/// HEAD vs working tree, etc.).
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct FileDiff {
    pub path: String,
    pub old_sha: Option<String>,
    pub new_sha: Option<String>,
    pub additions: u32,
    pub deletions: u32,
    pub hunks: Vec<DiffHunk>,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn diff_line_kind_serializes() {
        assert_eq!(
            serde_json::to_string(&DiffLineKind::Added).unwrap(),
            "\"added\""
        );
        assert_eq!(
            serde_json::to_string(&DiffLineKind::Removed).unwrap(),
            "\"removed\""
        );
        assert_eq!(
            serde_json::to_string(&DiffLineKind::Context).unwrap(),
            "\"context\""
        );
    }

    #[test]
    fn diff_line_added_has_no_old_line_no() {
        let line = DiffLine {
            kind: DiffLineKind::Added,
            content: "new line".into(),
            old_line_no: None,
            new_line_no: Some(42),
        };
        let json = serde_json::to_string(&line).unwrap();
        let back: DiffLine = serde_json::from_str(&json).unwrap();
        assert_eq!(line, back);
    }

    #[test]
    fn diff_line_removed_has_no_new_line_no() {
        let line = DiffLine {
            kind: DiffLineKind::Removed,
            content: "old line".into(),
            old_line_no: Some(7),
            new_line_no: None,
        };
        let json = serde_json::to_string(&line).unwrap();
        let back: DiffLine = serde_json::from_str(&json).unwrap();
        assert_eq!(line, back);
    }

    #[test]
    fn file_diff_roundtrip() {
        let d = FileDiff {
            path: "src/lib.rs".into(),
            old_sha: Some("a".into()),
            new_sha: Some("b".into()),
            additions: 3,
            deletions: 1,
            hunks: vec![DiffHunk {
                old_start: 1,
                old_lines: 4,
                new_start: 1,
                new_lines: 6,
                lines: vec![
                    DiffLine {
                        kind: DiffLineKind::Context,
                        content: "fn main() {".into(),
                        old_line_no: Some(1),
                        new_line_no: Some(1),
                    },
                    DiffLine {
                        kind: DiffLineKind::Added,
                        content: "    println!(\"hi\");".into(),
                        old_line_no: None,
                        new_line_no: Some(2),
                    },
                ],
            }],
        };
        let json = serde_json::to_string(&d).unwrap();
        let back: FileDiff = serde_json::from_str(&json).unwrap();
        assert_eq!(d, back);
    }
}
