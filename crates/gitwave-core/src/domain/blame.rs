//! `git blame` line annotation. See
//! `docs/tasks/feat-history-graph/plan.md` step 1.

use serde::{Deserialize, Serialize};

/// One line in a file's blame output.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct BlameLine {
    pub line_no: u32,
    pub sha: String,
    pub author: String,
    pub author_email: String,
    /// Unix epoch seconds of the originating commit.
    pub time: i64,
    /// Original line text, without trailing newline.
    pub content: String,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn blame_line_roundtrip() {
        let line = BlameLine {
            line_no: 42,
            sha: "abc123".into(),
            author: "Bob".into(),
            author_email: "bob@example.com".into(),
            time: 1_700_000_000,
            content: "fn main() {}".into(),
        };
        let json = serde_json::to_string(&line).unwrap();
        let back: BlameLine = serde_json::from_str(&json).unwrap();
        assert_eq!(line, back);
    }

    #[test]
    fn blame_line_unicode_content() {
        let line = BlameLine {
            line_no: 1,
            sha: "x".into(),
            author: "a".into(),
            author_email: "a@x".into(),
            time: 0,
            content: "中文 / emoji 🎉".into(),
        };
        let json = serde_json::to_string(&line).unwrap();
        let back: BlameLine = serde_json::from_str(&json).unwrap();
        assert_eq!(line, back);
    }
}
