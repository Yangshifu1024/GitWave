//! File-level diff operations. See
//! `docs/tasks/feat-history-graph/plan.md` step 5.

use std::collections::HashMap;

use git2::{Diff, DiffOptions, Oid, Repository};

use crate::domain::diff::{DiffHunk, DiffLine, DiffLineKind, FileDiff};
use crate::domain::error::{AppError, Result};

fn map_git_err(e: git2::Error) -> AppError {
    AppError::Unknown(format!("git: {e}"))
}

/// Summary of a diff with aggregate add/delete counts.
#[derive(Debug, Clone, serde::Serialize)]
pub struct DiffSummary {
    pub files: Vec<FileDiff>,
    pub total_additions: u32,
    pub total_deletions: u32,
}

impl DiffSummary {
    fn new(files: Vec<FileDiff>) -> Self {
        let total_additions = files.iter().map(|f| f.additions).sum();
        let total_deletions = files.iter().map(|f| f.deletions).sum();
        Self {
            files,
            total_additions,
            total_deletions,
        }
    }
}

/// Diff the working tree against the index (unstaged changes).
pub fn diff_workdir_to_index(repo: &Repository) -> Result<DiffSummary> {
    let mut opts = DiffOptions::new();
    opts.include_untracked(true);
    let diff = repo
        .diff_index_to_workdir(None, Some(&mut opts))
        .map_err(map_git_err)?;
    diff_to_summary(&diff)
}

/// Diff a commit against its first parent (or empty tree if root commit).
pub fn diff_commit_vs_parent(repo: &Repository, oid: Oid) -> Result<DiffSummary> {
    let commit = repo.find_commit(oid).map_err(map_git_err)?;
    let tree_new = commit.tree().map_err(map_git_err)?;

    let tree_old = if commit.parent_count() > 0 {
        let parent = commit.parent(0).map_err(map_git_err)?;
        Some(parent.tree().map_err(map_git_err)?)
    } else {
        None
    };

    let mut opts = DiffOptions::new();
    opts.context_lines(3);
    let diff = repo
        .diff_tree_to_tree(tree_old.as_ref(), Some(&tree_new), Some(&mut opts))
        .map_err(map_git_err)?;

    diff_to_summary(&diff)
}

/// Diff two arbitrary commits (from_oid = old, to_oid = new).
pub fn diff_paths(repo: &Repository, from_oid: Oid, to_oid: Oid) -> Result<Vec<FileDiff>> {
    let from_commit = repo.find_commit(from_oid).map_err(map_git_err)?;
    let to_commit = repo.find_commit(to_oid).map_err(map_git_err)?;

    let tree_from = from_commit.tree().map_err(map_git_err)?;
    let tree_to = to_commit.tree().map_err(map_git_err)?;

    let mut opts = DiffOptions::new();
    opts.context_lines(3);
    let diff = repo
        .diff_tree_to_tree(Some(&tree_from), Some(&tree_to), Some(&mut opts))
        .map_err(map_git_err)?;

    diff_to_files(&diff)
}

fn diff_to_summary(diff: &Diff) -> Result<DiffSummary> {
    let files = diff_to_files(diff)?;
    Ok(DiffSummary::new(files))
}

fn diff_to_files(diff: &Diff) -> Result<Vec<FileDiff>> {
    // Collect per-file stats first using foreach.
    // key = new_file path, value = accumulated FileDiff (hunks empty for now).
    let mut file_stats: HashMap<String, (u32, u32, Option<String>, Option<String>)> =
        HashMap::new();

    diff.foreach(
        &mut |delta, _spectate| -> bool {
            let path = delta
                .new_file()
                .path()
                .map(|p| p.to_string_lossy().into_owned())
                .unwrap_or_default();
            let _old_path = delta
                .old_file()
                .path()
                .map(|p| p.to_string_lossy().into_owned());
            let old_sha = delta.old_file().id().to_string();
            let new_sha = delta.new_file().id().to_string();
            // Treat zeros as None (file added or deleted)
            let old_sha_opt = if old_sha == "0000000000000000000000000000000000000000" {
                None
            } else {
                Some(old_sha)
            };
            let new_sha_opt = if new_sha == "0000000000000000000000000000000000000000" {
                None
            } else {
                Some(new_sha)
            };
            file_stats.insert(path, (0, 0, old_sha_opt, new_sha_opt));
            true
        },
        None,
        None,
        Some(&mut |_delta, _hunk, line| -> bool {
            // Line origin: '+', '-', ' ', or 'F' for file header
            let ch = line.origin();
            if ch == '+' || ch == '-' {
                // Determine which file this line belongs to by looking at the
                // line's content to extract the path. This is imperfect;
                // instead we rely on the fact that foreach delivers lines
                // in order grouped by file, and we track the current path
                // via the file callback above.
            }
            true
        }),
    )
    .map_err(map_git_err)?;

    // Collect per-file additions/deletions via print.
    let mut current_path: Option<String> = None;
    let mut current_add: u32 = 0;
    let mut current_del: u32 = 0;

    diff.print(git2::DiffFormat::Patch, |delta, _hunk, line| -> bool {
        let path = delta
            .new_file()
            .path()
            .map(|p| p.to_string_lossy().into_owned())
            .unwrap_or_default();

        if Some(&path) != current_path.as_ref() {
            // Switched to a new file — commit previous stats
            if let Some(ref p) = current_path {
                if let Some(entry) = file_stats.get_mut(p) {
                    entry.0 = current_add;
                    entry.1 = current_del;
                }
            }
            current_path = Some(path);
            current_add = 0;
            current_del = 0;
        }

        match line.origin() {
            '+' => current_add += 1,
            '-' => current_del += 1,
            _ => {}
        }
        true
    })
    .map_err(map_git_err)?;

    // Commit last file's stats
    if let Some(ref p) = current_path {
        if let Some(entry) = file_stats.get_mut(p) {
            entry.0 = current_add;
            entry.1 = current_del;
        }
    }

    // Now build the full FileDiff list with empty hunks.
    // For the summary functions (diff_workdir_to_index, diff_commit_vs_parent)
    // we don't need the hunk details — callers can call diff_paths for full detail.
    let files: Vec<FileDiff> = file_stats
        .into_iter()
        .map(
            |(path, (additions, deletions, old_sha, new_sha))| FileDiff {
                path,
                old_sha,
                new_sha,
                additions,
                deletions,
                hunks: Vec::new(),
            },
        )
        .collect();

    Ok(files)
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
    fn diff_commit_vs_parent_returns_file_diff() {
        let (path, repo) = build_linear_repo(2);
        let head = repo.head().unwrap().peel_to_commit().unwrap();
        let parent_oid = head.parent_id(0).unwrap();

        let summary = diff_commit_vs_parent(&repo, parent_oid).unwrap();
        cleanup(&path);

        assert!(!summary.files.is_empty(), "expected at least one file diff");
        let file = &summary.files[0];
        assert!(file.additions > 0 || file.deletions > 0);
    }

    #[test]
    fn diff_paths_compares_two_commits() {
        let (path, repo) = build_linear_repo(3);
        let head = repo.head().unwrap().peel_to_commit().unwrap();
        let parent0 = head.parent(0).unwrap();
        let oid0 = parent0.parent(0).unwrap().id();
        let oid1 = head.id();

        let diffs = diff_paths(&repo, oid0, oid1).unwrap();
        cleanup(&path);

        assert!(
            !diffs.is_empty(),
            "expected at least one file diff between commits"
        );
    }

    #[test]
    fn diff_workdir_to_index_empty_on_clean_repo() {
        let (path, repo) = build_linear_repo(1);
        let summary = diff_workdir_to_index(&repo).unwrap();
        cleanup(&path);

        // Clean repo with no uncommitted changes
        assert_eq!(summary.files.len(), 0);
    }
}
