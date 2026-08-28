//! Repo health metrics (M3). Deterministic, libgit2 + filesystem only —
//! the AI summary is layered on top in the use case, never required for
//! the numbers themselves.

use std::fs;

use git2::{Repository, Time};
use serde::Serialize;

use crate::domain::error::{AppError, Result};

fn map_git_err(e: git2::Error) -> AppError {
    AppError::Unknown(format!("git: {e}"))
}

const STALE_DAYS: i64 = 30;
const LARGE_FILE_BYTES: u64 = 1024 * 1024;

#[derive(Debug, Clone, Serialize)]
pub struct LargeFile {
    pub path: String,
    pub size_bytes: u64,
}

#[derive(Debug, Clone, Serialize)]
pub struct HealthReport {
    /// Commits on the current branch that its upstream does not have.
    /// `None` when the branch has no upstream configured.
    pub unpushed: Option<u32>,
    /// Names of leftover operation state files (merge / revert /
    /// cherry-pick in progress) — usually a sign of an abandoned operation.
    pub conflict_residue: Vec<String>,
    pub dirty_files: u32,
    /// Local branches (excluding the current one) whose tip is older than
    /// [`STALE_DAYS`].
    pub stale_branches: Vec<String>,
    pub large_files: Vec<LargeFile>,
    pub branch_count: u32,
    pub tag_count: u32,
}

fn state_file_exists(repo: &Repository, name: &str) -> Option<String> {
    if repo.path().join(name).exists() {
        Some(name.to_string())
    } else {
        None
    }
}

/// Largest workdir files above [`LARGE_FILE_BYTES`], skipping `.git`,
/// newest irrelevant — deterministic path order then size sort.
fn large_files(repo: &Repository, limit: usize) -> Vec<LargeFile> {
    let Some(workdir) = repo.workdir() else {
        return Vec::new();
    };
    let mut found: Vec<LargeFile> = Vec::new();
    let mut stack = vec![workdir.to_path_buf()];
    while let Some(dir) = stack.pop() {
        let Ok(entries) = fs::read_dir(&dir) else {
            continue;
        };
        for entry in entries.flatten() {
            let path = entry.path();
            let Ok(meta) = entry.metadata() else {
                continue;
            };
            if meta.is_dir() {
                if path.file_name().is_some_and(|n| n == ".git") {
                    continue;
                }
                stack.push(path);
            } else if meta.is_file() && meta.len() >= LARGE_FILE_BYTES {
                found.push(LargeFile {
                    path: path
                        .strip_prefix(workdir)
                        .unwrap_or(&path)
                        .to_string_lossy()
                        .into_owned(),
                    size_bytes: meta.len(),
                });
            }
        }
    }
    found.sort_by(|a, b| b.size_bytes.cmp(&a.size_bytes));
    found.truncate(limit);
    found
}

/// Collect the full health report for the active repository.
pub fn collect_health(repo: &Repository) -> Result<HealthReport> {
    let now = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or(0);
    let stale_cutoff = now - STALE_DAYS * 24 * 3600;

    // Conflict residue: operation state files in .git.
    let conflict_residue: Vec<String> = ["MERGE_HEAD", "REVERT_HEAD", "CHERRY_PICK_HEAD"]
        .iter()
        .filter_map(|f| state_file_exists(repo, f))
        .collect();

    // Dirty working copy (incl. untracked).
    let mut status_opts = git2::StatusOptions::new();
    status_opts
        .include_untracked(true)
        .recurse_untracked_dirs(true);
    let dirty_files = repo
        .statuses(Some(&mut status_opts))
        .map_err(map_git_err)?
        .len() as u32;

    // Branches: count, stale (tip older than cutoff), current ahead count.
    let head_ref = repo.head().ok();
    let current_name = head_ref
        .as_ref()
        .and_then(|h| h.shorthand().map(str::to_string));
    let mut stale_branches: Vec<String> = Vec::new();
    let mut branch_count = 0u32;
    let mut unpushed: Option<u32> = None;

    let branches = repo
        .branches(Some(git2::BranchType::Local))
        .map_err(map_git_err)?;
    for item in branches {
        let (branch, _) = item.map_err(map_git_err)?;
        let Some(name) = branch.name().map_err(map_git_err)? else {
            continue;
        };
        branch_count += 1;
        let is_current = branch.is_head();
        let Some(oid) = branch.get().target() else {
            continue;
        };
        let commit = repo.find_commit(oid).map_err(map_git_err)?;
        if is_current {
            if let Ok(upstream) = branch.upstream() {
                if let (Some(a), Some(b)) = (branch.get().target(), upstream.get().target()) {
                    if let Ok((ahead, _)) = repo.graph_ahead_behind(a, b) {
                        unpushed = Some(ahead as u32);
                    }
                }
            }
            continue;
        }
        if commit.time() < Time::new(stale_cutoff, 0) {
            stale_branches.push(name.to_string());
        }
    }

    let tag_count = repo.tag_names(None).iter().flatten().count() as u32;

    Ok(HealthReport {
        unpushed,
        conflict_residue,
        dirty_files,
        stale_branches,
        large_files: large_files(repo, 5),
        branch_count,
        tag_count,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::{build_linear_repo, write_and_stage};
    use std::fs;

    fn cleanup(path: &std::path::Path) {
        let _ = fs::remove_dir_all(path);
    }

    #[test]
    fn fresh_repo_is_healthy() {
        let (path, repo) = build_linear_repo(2);
        let report = collect_health(&repo).unwrap();
        assert!(report.conflict_residue.is_empty());
        assert_eq!(report.dirty_files, 0);
        assert!(report.stale_branches.is_empty());
        assert_eq!(report.branch_count, 1);
        cleanup(&path);
    }

    #[test]
    fn detects_dirty_untracked_and_large_files() {
        let (path, repo) = build_linear_repo(1);
        fs::write(repo.workdir().unwrap().join("untracked.txt"), "x\n").unwrap();
        let big = vec![b'a'; (1024 * 1024 * 2) as usize];
        fs::write(repo.workdir().unwrap().join("big.bin"), &big).unwrap();

        let report = collect_health(&repo).unwrap();
        assert!(report.dirty_files >= 2, "untracked + big file");
        assert_eq!(report.large_files.len(), 1);
        assert_eq!(report.large_files[0].path, "big.bin");
        assert!(report.large_files[0].size_bytes >= 1024 * 1024);
        cleanup(&path);
    }

    #[test]
    fn detects_merge_residue_and_stale_branch() {
        let (path, repo) = build_linear_repo(2);
        let parent = repo.head().unwrap().peel_to_commit().unwrap();

        // Stale branch: tip committed 60 days ago (backdated signature).
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_secs() as i64;
        let sig_old =
            git2::Signature::new("T", "t@l", &Time::new(now - 60 * 24 * 3600, 0)).unwrap();
        let tree = write_and_stage(&repo, "old.txt", "old\n");
        let stale_commit = repo
            .commit(
                None,
                &sig_old,
                &sig_old,
                "old work",
                &repo.find_tree(tree).unwrap(),
                &[&parent],
            )
            .unwrap();
        repo.branch("old-stuff", &repo.find_commit(stale_commit).unwrap(), false)
            .unwrap();

        // Residue: simulate an abandoned merge by writing MERGE_HEAD.
        let base = parent.id();
        fs::write(repo.path().join("MERGE_HEAD"), format!("{base}\n")).unwrap();

        let report = collect_health(&repo).unwrap();
        assert_eq!(report.conflict_residue, vec!["MERGE_HEAD".to_string()]);
        assert!(
            report.stale_branches.contains(&"old-stuff".to_string()),
            "stale branch should be flagged: {:?}",
            report.stale_branches
        );
        cleanup(&path);
    }
}
