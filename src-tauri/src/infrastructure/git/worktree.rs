//! Worktree list / add / remove via libgit2.

use std::path::Path;

use git2::{Repository, WorktreeAddOptions, WorktreePruneOptions};

use crate::domain::error::{AppError, Result};
use crate::domain::worktree::WorktreeInfo;

fn map_git_err(e: git2::Error) -> AppError {
    AppError::Unknown(format!("git: {e}"))
}

pub fn list_worktrees(repo: &Repository) -> Result<Vec<WorktreeInfo>> {
    let mut out = Vec::new();

    // Main worktree
    let main_path = repo
        .workdir()
        .map(|p| p.to_string_lossy().into_owned())
        .unwrap_or_else(|| repo.path().to_string_lossy().into_owned());
    let main_branch = repo
        .head()
        .ok()
        .and_then(|h| h.shorthand().map(str::to_string));
    out.push(WorktreeInfo {
        name: "(main)".into(),
        path: main_path,
        is_main: true,
        is_locked: false,
        branch: main_branch,
    });

    let names = repo.worktrees().map_err(map_git_err)?;
    for name in names.iter().flatten() {
        let wt = repo.find_worktree(name).map_err(map_git_err)?;
        let path = wt.path().to_string_lossy().into_owned();
        let is_locked = wt.is_locked().is_ok_and(|status| !matches!(status, git2::WorktreeLockStatus::Unlocked));
        // Open the linked repo to read its HEAD branch when possible.
        let branch = Repository::open(wt.path()).ok().and_then(|r| {
            r.head()
                .ok()
                .and_then(|h| h.shorthand().map(str::to_string))
        });
        out.push(WorktreeInfo {
            name: name.to_string(),
            path,
            is_main: false,
            is_locked,
            branch,
        });
    }
    Ok(out)
}

/// Create a new worktree at `path` checked out to `branch`.
/// If `create_branch` is true, create `branch` from HEAD first.
pub fn add_worktree(
    repo: &Repository,
    name: &str,
    path: &Path,
    branch: &str,
    create_branch: bool,
) -> Result<WorktreeInfo> {
    if create_branch {
        let commit = repo
            .head()
            .map_err(map_git_err)?
            .peel_to_commit()
            .map_err(map_git_err)?;
        repo.branch(branch, &commit, false).map_err(map_git_err)?;
    }

    let mut opts = WorktreeAddOptions::new();
    // Reference the branch for checkout
    let reference = repo
        .find_reference(&format!("refs/heads/{branch}"))
        .map_err(map_git_err)?;
    opts.reference(Some(&reference));

    let wt = repo.worktree(name, path, Some(&opts)).map_err(map_git_err)?;
    Ok(WorktreeInfo {
        name: name.to_string(),
        path: wt.path().to_string_lossy().into_owned(),
        is_main: false,
        is_locked: false,
        branch: Some(branch.to_string()),
    })
}

pub fn remove_worktree(repo: &Repository, name: &str) -> Result<()> {
    if name == "(main)" {
        return Err(AppError::Protocol("cannot remove the main worktree".into()));
    }
    let wt = repo.find_worktree(name).map_err(map_git_err)?;
    let mut opts = WorktreePruneOptions::new();
    opts.valid(true).locked(true).working_tree(true);
    wt.prune(Some(&mut opts)).map_err(map_git_err)?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::build_linear_repo;
    use std::fs;

    #[test]
    fn list_includes_main() {
        let (path, repo) = build_linear_repo(1);
        let list = list_worktrees(&repo).unwrap();
        let _ = fs::remove_dir_all(&path);
        assert!(list.iter().any(|w| w.is_main));
    }

    #[test]
    fn add_and_remove_worktree() {
        let (path, repo) = build_linear_repo(2);
        let wt_path = path.parent().unwrap().join(format!(
            "gitwave-wt-{}",
            std::process::id()
        ));
        let _ = fs::remove_dir_all(&wt_path);

        let info = add_worktree(&repo, "feat-wt", &wt_path, "feat-wt", true).unwrap();
        assert!(!info.is_main);
        assert!(wt_path.exists());

        let list = list_worktrees(&repo).unwrap();
        assert!(list.iter().any(|w| w.name == "feat-wt"));

        remove_worktree(&repo, "feat-wt").unwrap();
        let list = list_worktrees(&repo).unwrap();
        assert!(!list.iter().any(|w| w.name == "feat-wt"));

        let _ = fs::remove_dir_all(&path);
        let _ = fs::remove_dir_all(&wt_path);
    }
}
