//! Git blame / file annotation. See
//! `docs/tasks/feat-history-graph/plan.md` step 5.

use std::path::Path;

use git2::{Blame, BlameOptions, Repository};

use crate::domain::blame::BlameLine;
use crate::domain::error::{AppError, Result};

fn map_git_err(e: git2::Error) -> AppError {
    AppError::Unknown(format!("git: {e}"))
}

/// Annotate each line of a file with the commit that last modified it.
///
/// `rel_path` is relative to the repository root.
pub fn blame_file(repo: &Repository, rel_path: &str) -> Result<Vec<BlameLine>> {
    let mut opts = BlameOptions::new();
    let blame = repo
        .blame_file(Path::new(rel_path), Some(&mut opts))
        .map_err(map_git_err)?;

    // Collect all hunks by iterating line numbers.
    // We use the hunk boundary approach: for each line we call get_line,
    // which returns the hunk that contains it. We track processed hunks
    // to avoid duplicates.
    let mut lines: Vec<BlameLine> = Vec::new();
    let mut line_no: usize = 1;

    while let Some(hunk) = blame.get_line(line_no) {
        let commit_oid = hunk.final_commit_id();
        let commit = repo.find_commit(commit_oid).map_err(map_git_err)?;
        let author = commit.author();
        let time = commit.time().seconds();
        let content_lines_in_hunk = hunk.lines_in_hunk();
        let start_line = hunk.final_start_line();

        // For each line in this hunk, create a BlameLine.
        // content will be filled from the working-tree file below.
        for offset in 0..content_lines_in_hunk {
            let this_line = (start_line + offset) as u32;
            lines.push(BlameLine {
                line_no: this_line,
                sha: commit_oid.to_string(),
                author: author.name().unwrap_or("").to_string(),
                author_email: author.email().unwrap_or("").to_string(),
                time,
                content: String::new(), // filled from workdir below
            });
        }

        // Advance past this hunk
        line_no += content_lines_in_hunk;
        if content_lines_in_hunk == 0 {
            break; // safety: avoid infinite loop
        }
    }

    // Fill line content from the working-tree file.
    if let Ok(head_commit) = repo.head().map_err(map_git_err)?.peel_to_commit() {
        if let Ok(tree) = head_commit.tree() {
            if let Ok(entry) = tree.get_path(Path::new(rel_path)) {
                if let Ok(blob) = repo.find_blob(entry.id()) {
                    let content = String::from_utf8_lossy(blob.content());
                    let all_lines: Vec<&str> = content.lines().collect();
                    for line in &mut lines {
                        line.content = all_lines
                            .get((line.line_no - 1) as usize)
                            .unwrap_or(&"")
                            .to_string();
                    }
                }
            }
        }
    }

    Ok(lines)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::build_linear_repo;
    use std::fs;

    fn cleanup(path: &std::path::Path) {
        let _ = fs::remove_dir_all(path);
    }

    #[test]
    fn blame_file_returns_lines_for_existing_file() {
        let (path, repo) = build_linear_repo(2);

        // blame the first file created in build_linear_repo
        let result = blame_file(&repo, "file0.txt");
        cleanup(&path);

        let lines = result.expect("blame should succeed");
        assert!(!lines.is_empty(), "expected at least one blamed line");
        assert!(lines[0].sha.len() == 40, "sha should be 40 hex chars");
    }

    #[test]
    fn blame_nonexistent_file_errors() {
        let (path, repo) = build_linear_repo(1);
        let result = blame_file(&repo, "does_not_exist.txt");
        cleanup(&path);

        assert!(result.is_err(), "blame on nonexistent file should error");
    }
}
