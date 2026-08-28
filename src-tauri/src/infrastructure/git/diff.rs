//! File-level diff operations. See
//! `docs/tasks/feat-history-graph/plan.md` step 5.

use git2::{Diff, DiffDelta, DiffOptions, Oid, Repository};

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

    pub fn merge(self, other: Self) -> Self {
        let mut files = self.files;
        files.extend(other.files);
        Self::new(files)
    }

    pub fn mark_staged(mut self, staged: bool) -> Self {
        for file in &mut self.files {
            file.staged = Some(staged);
        }
        self
    }
}

/// Diff the working tree against the index (unstaged changes).
pub fn diff_workdir_to_index(repo: &Repository) -> Result<DiffSummary> {
    let mut opts = DiffOptions::new();
    opts.include_untracked(true)
        .recurse_untracked_dirs(true)
        .show_untracked_content(true)
        .context_lines(3);
    let diff = repo
        .diff_index_to_workdir(None, Some(&mut opts))
        .map_err(map_git_err)?;
    diff_to_summary(&diff)
}

/// Diff the index (staged) against HEAD tree.
pub fn diff_index_to_head(repo: &Repository) -> Result<DiffSummary> {
    let head_tree = match repo.head() {
        Ok(h) => Some(h.peel_to_tree().map_err(map_git_err)?),
        Err(e) if e.code() == git2::ErrorCode::UnbornBranch => None,
        Err(e) => return Err(map_git_err(e)),
    };
    let mut opts = DiffOptions::new();
    opts.context_lines(3);
    let diff = repo
        .diff_tree_to_index(head_tree.as_ref(), None, Some(&mut opts))
        .map_err(map_git_err)?;
    diff_to_summary(&diff)
}

/// Same diff as [`diff_index_to_head`] but with per-file hunks, for
/// consumers that need the actual patch text (AI commit-message prompt).
pub fn diff_index_to_head_files(repo: &Repository) -> Result<Vec<FileDiff>> {
    let head_tree = match repo.head() {
        Ok(h) => Some(h.peel_to_tree().map_err(map_git_err)?),
        Err(e) if e.code() == git2::ErrorCode::UnbornBranch => None,
        Err(e) => return Err(map_git_err(e)),
    };
    let mut opts = DiffOptions::new();
    opts.context_lines(3);
    let diff = repo
        .diff_tree_to_index(head_tree.as_ref(), None, Some(&mut opts))
        .map_err(map_git_err)?;
    diff_to_files(&diff)
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

/// Same diff as [`diff_commit_vs_parent`] but with per-file hunks, for
/// consumers that need the actual patch text (AI explain prompts).
pub fn diff_commit_vs_parent_files(repo: &Repository, oid: Oid) -> Result<Vec<FileDiff>> {
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

    diff_to_files(&diff)
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

fn path_from_delta(delta: &DiffDelta<'_>) -> String {
    delta
        .new_file()
        .path()
        .or_else(|| delta.old_file().path())
        .map(|p| p.to_string_lossy().into_owned())
        .unwrap_or_default()
}

fn sha_opt(id: git2::Oid) -> Option<String> {
    let s = id.to_string();
    if s.chars().all(|c| c == '0') {
        None
    } else {
        Some(s)
    }
}

/// Walk the diff and build per-file `FileDiff` values **including hunks**.
fn diff_to_files(diff: &Diff) -> Result<Vec<FileDiff>> {
    // `Diff::foreach` takes four callbacks that all need to mutate the same
    // accumulator; RefCell lets them share it without overlapping &mut borrows.
    use std::cell::RefCell;

    let files: RefCell<Vec<FileDiff>> = RefCell::new(Vec::new());

    diff.foreach(
        &mut |delta, _progress| -> bool {
            let path = path_from_delta(&delta);
            let old_sha = sha_opt(delta.old_file().id());
            let new_sha = sha_opt(delta.new_file().id());
            files.borrow_mut().push(FileDiff {
                path,
                old_sha,
                new_sha,
                additions: 0,
                deletions: 0,
                hunks: Vec::new(),
                staged: None,
            });
            true
        },
        None,
        Some(&mut |_delta, hunk| -> bool {
            if let Some(file) = files.borrow_mut().last_mut() {
                file.hunks.push(DiffHunk {
                    old_start: hunk.old_start(),
                    old_lines: hunk.old_lines(),
                    new_start: hunk.new_start(),
                    new_lines: hunk.new_lines(),
                    lines: Vec::new(),
                });
            }
            true
        }),
        Some(&mut |_delta, _hunk, line| -> bool {
            let mut files = files.borrow_mut();
            let Some(file) = files.last_mut() else {
                return true;
            };
            let content = std::str::from_utf8(line.content())
                .unwrap_or("")
                .trim_end_matches(['\r', '\n'])
                .to_string();

            let (kind, old_line_no, new_line_no) = match line.origin() {
                '+' => {
                    file.additions += 1;
                    (
                        DiffLineKind::Added,
                        None,
                        Some(line.new_lineno().unwrap_or(0)),
                    )
                }
                '-' => {
                    file.deletions += 1;
                    (
                        DiffLineKind::Removed,
                        Some(line.old_lineno().unwrap_or(0)),
                        None,
                    )
                }
                ' ' => (DiffLineKind::Context, line.old_lineno(), line.new_lineno()),
                _ => return true,
            };

            if let Some(hunk) = file.hunks.last_mut() {
                hunk.lines.push(DiffLine {
                    kind,
                    content,
                    old_line_no,
                    new_line_no,
                });
            }
            true
        }),
    )
    .map_err(map_git_err)?;

    Ok(files.into_inner())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::diff::FileDiff;
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
    fn diff_commit_vs_parent_includes_hunks() {
        let (path, repo) = build_linear_repo(2);
        let head = repo.head().unwrap().peel_to_commit().unwrap();

        let summary = diff_commit_vs_parent(&repo, head.id()).unwrap();
        cleanup(&path);

        assert!(!summary.files.is_empty());
        let file = &summary.files[0];
        assert!(
            !file.hunks.is_empty(),
            "expected hunk detail, got empty hunks"
        );
        assert!(
            file.hunks.iter().any(|h| !h.lines.is_empty()),
            "expected hunk lines"
        );
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
        assert!(
            diffs.iter().any(|f| !f.hunks.is_empty()),
            "expected hunks between commits"
        );
    }

    #[test]
    fn diff_workdir_to_index_empty_on_clean_repo() {
        let (path, repo) = build_linear_repo(1);
        let summary = diff_workdir_to_index(&repo).unwrap();
        cleanup(&path);

        assert_eq!(summary.files.len(), 0);
    }

    #[test]
    fn untracked_file_diff_includes_full_content() {
        let (path, repo) = build_linear_repo(1);
        fs::write(path.join("new.txt"), "alpha\nbeta\ngamma\n").unwrap();

        let summary = diff_workdir_to_index(&repo).unwrap();
        cleanup(&path);

        let file = summary
            .files
            .iter()
            .find(|f| f.path == "new.txt")
            .expect("untracked new.txt should appear in workdir diff");
        assert_eq!(file.additions, 3, "expected every new line counted");
        assert_eq!(file.deletions, 0);
        let lines: Vec<&str> = file
            .hunks
            .iter()
            .flat_map(|h| h.lines.iter())
            .map(|l| l.content.as_str())
            .collect();
        assert_eq!(lines, ["alpha", "beta", "gamma"]);
    }

    #[test]
    fn staged_new_file_diff_includes_full_content() {
        let (path, repo) = build_linear_repo(1);
        fs::write(path.join("added.rs"), "fn main() {}\n").unwrap();
        {
            let mut index = repo.index().unwrap();
            index.add_path(std::path::Path::new("added.rs")).unwrap();
            index.write().unwrap();
        }

        let summary = diff_index_to_head(&repo).unwrap();
        cleanup(&path);

        let file = summary
            .files
            .iter()
            .find(|f| f.path == "added.rs")
            .expect("staged added.rs should appear in index diff");
        assert_eq!(file.additions, 1);
        let lines: Vec<&str> = file
            .hunks
            .iter()
            .flat_map(|h| h.lines.iter())
            .map(|l| l.content.as_str())
            .collect();
        assert_eq!(lines, ["fn main() {}"]);
    }

    #[test]
    fn untracked_file_in_new_directory_includes_full_content() {
        let (path, repo) = build_linear_repo(1);
        fs::create_dir_all(path.join("src")).unwrap();
        fs::write(path.join("src").join("lib.rs"), "pub fn f() {}\n").unwrap();

        let summary = diff_workdir_to_index(&repo).unwrap();
        cleanup(&path);

        let file = summary
            .files
            .iter()
            .find(|f| f.path.replace('\\', "/") == "src/lib.rs");
        if file.is_none() {
            panic!(
                "expected src/lib.rs, got {:?}",
                summary
                    .files
                    .iter()
                    .map(|f| f.path.as_str())
                    .collect::<Vec<_>>()
            );
        }
        let file = file.unwrap();
        assert_eq!(file.additions, 1);
        let lines: Vec<&str> = file
            .hunks
            .iter()
            .flat_map(|h| h.lines.iter())
            .map(|l| l.content.as_str())
            .collect();
        assert_eq!(lines, ["pub fn f() {}"]);
    }

    #[test]
    fn diff_summary_merge_concatenates_files_and_totals() {
        let file = |path: &str, additions: u32, deletions: u32| FileDiff {
            path: path.into(),
            old_sha: None,
            new_sha: None,
            additions,
            deletions,
            hunks: vec![],
            staged: None,
        };
        let merged = DiffSummary::new(vec![file("a.ts", 3, 1)])
            .merge(DiffSummary::new(vec![file("b.ts", 2, 4)]));
        assert_eq!(merged.files.len(), 2);
        assert_eq!(merged.total_additions, 5);
        assert_eq!(merged.total_deletions, 5);
    }

    #[test]
    fn mark_staged_tags_both_sides_when_same_path_merges() {
        let file = |path: &str, additions: u32| FileDiff {
            path: path.into(),
            old_sha: None,
            new_sha: None,
            additions,
            deletions: 0,
            hunks: vec![],
            staged: None,
        };
        let merged = DiffSummary::new(vec![file("a.ts", 4)])
            .mark_staged(true)
            .merge(DiffSummary::new(vec![file("a.ts", 1)]).mark_staged(false));
        assert_eq!(merged.files.len(), 2);
        assert_eq!(merged.files[0].staged, Some(true));
        assert_eq!(merged.files[1].staged, Some(false));
        assert_eq!(merged.total_additions, 5);
    }
}
