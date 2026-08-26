//! Branch CRUD operations. See
//! `docs/tasks/feat-history-graph/plan.md` step 3.
//!
//! `list_branches` lives in `history.rs` (kept together with the commit
//! log walker so branch listing and the commit log share the same
//! libgit2 borrow patterns).

use git2::Repository;

use crate::domain::error::{AppError, Result};

/// Create a new branch pointing at `from_sha`. The branch is local and
/// not checked out — call `checkout_branch` separately if you want to
/// switch to it.
///
/// `force = true` overwrites an existing branch with the same name.
pub fn create_branch(repo: &Repository, name: &str, from_sha: &str, force: bool) -> Result<()> {
    let commit = repo
        .find_commit(git2::Oid::from_str(from_sha).map_err(map_git_err)?)
        .map_err(map_git_err)?;
    repo.branch(name, &commit, force).map_err(map_git_err)?;
    Ok(())
}

/// Delete a local branch. Errors if the branch doesn't exist or is the
/// current HEAD (refuse to delete checked-out branch).
pub fn delete_branch(repo: &Repository, name: &str) -> Result<()> {
    let branch = repo
        .find_branch(name, git2::BranchType::Local)
        .map_err(map_git_err)?;
    if branch.is_head() {
        return Err(AppError::Protocol(format!(
            "cannot delete checked-out branch: {name}"
        )));
    }
    branch.into_reference().delete().map_err(map_git_err)?;
    Ok(())
}

/// Check out a local branch (updates HEAD and the working tree).
pub fn checkout_branch(repo: &Repository, name: &str) -> Result<()> {
    let refname = format!("refs/heads/{name}");
    repo.set_head(&refname).map_err(map_git_err)?;
    repo.checkout_head(Some(
        git2::build::CheckoutBuilder::default()
            .force()
            .remove_untracked(true),
    ))
    .map_err(map_git_err)?;
    Ok(())
}

fn map_git_err(e: git2::Error) -> AppError {
    AppError::Unknown(format!("git: {e}"))
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
    fn create_branch_then_list_includes_it() {
        let (path, repo) = build_linear_repo(2);
        let sha = repo
            .head()
            .unwrap()
            .peel_to_commit()
            .unwrap()
            .id()
            .to_string();
        create_branch(&repo, "feature", &sha, false).unwrap();

        let branches = crate::infrastructure::git::history::list_branches(&repo).unwrap();
        let feature: Vec<_> = branches.iter().filter(|b| b.name == "feature").collect();
        assert_eq!(feature.len(), 1, "expected exactly one 'feature' branch");
        cleanup(&path);
    }

    #[test]
    fn create_branch_force_overwrites() {
        let (path, repo) = build_linear_repo(2);
        let sha = repo
            .head()
            .unwrap()
            .peel_to_commit()
            .unwrap()
            .id()
            .to_string();
        create_branch(&repo, "feature", &sha, false).unwrap();
        // Second call without force should fail (Exists).
        let err = create_branch(&repo, "feature", &sha, false).unwrap_err();
        assert_eq!(err.category(), "Unknown");
        // Third call with force should succeed.
        create_branch(&repo, "feature", &sha, true).unwrap();
        cleanup(&path);
    }

    #[test]
    fn delete_branch_removes_it() {
        let (path, repo) = build_linear_repo(2);
        let sha = repo
            .head()
            .unwrap()
            .peel_to_commit()
            .unwrap()
            .id()
            .to_string();
        create_branch(&repo, "feature", &sha, false).unwrap();
        delete_branch(&repo, "feature").unwrap();
        let branches = crate::infrastructure::git::history::list_branches(&repo).unwrap();
        assert!(branches.iter().all(|b| b.name != "feature"));
        cleanup(&path);
    }

    #[test]
    fn delete_current_branch_errors() {
        let (path, repo) = build_linear_repo(2);
        let err = delete_branch(&repo, "main").unwrap_err();
        assert_eq!(err.category(), "Protocol");
        assert!(err.to_string().contains("checked-out"));
        cleanup(&path);
    }

    #[test]
    fn checkout_then_head_points_to_branch() {
        let (path, repo) = build_linear_repo(2);
        let sha = repo
            .head()
            .unwrap()
            .peel_to_commit()
            .unwrap()
            .id()
            .to_string();
        create_branch(&repo, "feature", &sha, false).unwrap();

        checkout_branch(&repo, "feature").unwrap();

        let head = repo.head().unwrap();
        assert_eq!(head.name().unwrap(), "refs/heads/feature");
        cleanup(&path);
    }
}
