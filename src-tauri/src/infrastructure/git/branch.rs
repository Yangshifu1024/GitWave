//! Branch CRUD operations. See
//! `docs/tasks/feat-history-graph/plan.md` step 3.
//!
//! `list_branches` lives in `history.rs` (kept together with the commit
//! log walker so branch listing and the commit log share the same
//! libgit2 borrow patterns).

use git2::Repository;

use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;

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
        return Err(AppError::protocol_with(
            codes::git::DELETE_CHECKED_OUT_BRANCH,
            format!("cannot delete checked-out branch: {name}"),
            &[("name", name.to_string())],
        ));
    }
    branch.into_reference().delete().map_err(map_git_err)?;
    Ok(())
}

/// Check out a local branch (updates the working tree, then HEAD).
///
/// `force = false` refuses to overwrite local changes (safe default).
/// `force = true` discards tracked and untracked worktree files — UI must confirm.
pub fn checkout_branch(repo: &Repository, name: &str, force: bool) -> Result<()> {
    let branch = repo
        .find_branch(name, git2::BranchType::Local)
        .map_err(map_git_err)?;
    let object = branch
        .get()
        .peel(git2::ObjectType::Tree)
        .map_err(map_git_err)?;
    let mut builder = git2::build::CheckoutBuilder::new();
    if force {
        builder.force();
        builder.remove_untracked(true);
    }
    repo.checkout_tree(&object, Some(&mut builder))
        .map_err(map_git_err)?;
    repo.set_head(&format!("refs/heads/{name}"))
        .map_err(map_git_err)?;
    Ok(())
}

/// Check out a commit directly (detached HEAD, updates the working tree).
///
/// `force = false` refuses to touch a dirty worktree (safe default).
/// `force = true` discards tracked and untracked worktree files — UI must
/// confirm. Any untracked file counts as dirty: deliberately conservative,
/// it matches the F004 branch-switch gate and keeps the UI's
/// stash-and-switch flow (`saveStash` includes untracked) sound. libgit2's
/// SAFE checkout strategy rejects even clean tree swaps that remove tracked
/// files, so a verified-clean worktree switches via a force checkout
/// (equivalent to `git checkout <commit>`).
pub fn checkout_commit(repo: &Repository, oid: &str, force: bool) -> Result<()> {
    let oid = git2::Oid::from_str(oid).map_err(|e| {
        AppError::protocol_with(
            codes::git::INVALID_OID,
            format!("invalid oid: {e}"),
            &[("error", e.to_string())],
        )
    })?;
    let commit = repo.find_commit(oid).map_err(|_| {
        AppError::protocol_with(
            codes::git::COMMIT_NOT_FOUND,
            format!("commit not found: {oid}"),
            &[("oid", oid.to_string())],
        )
    })?;

    if !force {
        let mut opts = git2::StatusOptions::new();
        opts.include_untracked(true).recurse_untracked_dirs(true);
        let statuses = repo.statuses(Some(&mut opts)).map_err(map_git_err)?;
        if statuses.iter().next().is_some() {
            return Err(AppError::protocol(
                codes::git::DIRTY_WORKTREE,
                "working tree is dirty — commit or stash before checking out a commit",
            ));
        }
    }

    let object = commit
        .as_object()
        .peel(git2::ObjectType::Tree)
        .map_err(map_git_err)?;
    let mut builder = git2::build::CheckoutBuilder::new();
    builder.force();
    builder.remove_untracked(true);
    repo.checkout_tree(&object, Some(&mut builder))
        .map_err(map_git_err)?;
    repo.set_head_detached(oid).map_err(map_git_err)?;
    Ok(())
}

/// Rename a local branch. HEAD stays attached when the current branch is
/// renamed — libgit2 moves the ref and the `branch.<old>.*` config section
/// (so upstream tracking follows) but does NOT update the HEAD symref.
/// Refuses to rename a branch checked out in a linked worktree: the move
/// would leave that worktree's HEAD symref dangling.
pub fn rename_branch(repo: &Repository, old_name: &str, new_name: &str, force: bool) -> Result<()> {
    for wt in crate::infrastructure::git::worktree::list_worktrees(repo)? {
        if !wt.is_main && wt.branch.as_deref() == Some(old_name) {
            return Err(AppError::protocol_with(
                codes::git::RENAME_IN_WORKTREE,
                format!(
                    "cannot rename {old_name}: checked out in worktree \"{}\"",
                    wt.name
                ),
                &[("name", old_name.to_string()), ("worktree", wt.name)],
            ));
        }
    }
    let mut branch = repo
        .find_branch(old_name, git2::BranchType::Local)
        .map_err(map_git_err)?;
    let is_head = branch.is_head();
    branch.rename(new_name, force).map_err(map_git_err)?;
    if is_head {
        repo.set_head(&format!("refs/heads/{new_name}"))
            .map_err(map_git_err)?;
    }
    Ok(())
}

/// Set (or clear, `None`) the upstream a local branch tracks. `upstream`
/// uses the remote-tracking shorthand form, e.g. `origin/main`.
pub fn set_branch_upstream(
    repo: &Repository,
    branch_name: &str,
    upstream: Option<&str>,
) -> Result<()> {
    let mut branch = repo
        .find_branch(branch_name, git2::BranchType::Local)
        .map_err(map_git_err)?;
    branch.set_upstream(upstream).map_err(map_git_err)?;
    Ok(())
}

fn map_git_err(e: git2::Error) -> AppError {
    AppError::unknown_with(
        codes::git::GIT_ERROR,
        format!("git: {e}"),
        &[("error", e.to_string())],
    )
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
    fn checkout_commit_detaches_head_and_swaps_worktree() {
        let (path, repo) = build_linear_repo(2);
        let root = repo
            .revparse_single("main~1")
            .unwrap()
            .peel_to_commit()
            .unwrap();
        let workdir = repo.workdir().unwrap();
        assert!(workdir.join("file1.txt").exists());

        checkout_commit(&repo, &root.id().to_string(), false).unwrap();

        let head = repo.head().unwrap();
        assert!(!head.is_branch(), "expected detached HEAD");
        assert_eq!(head.target(), Some(root.id()));
        assert!(!workdir.join("file1.txt").exists());
        assert_eq!(
            fs::read_to_string(workdir.join("file0.txt")).unwrap(),
            "v0\n"
        );
        cleanup(&path);
    }

    #[test]
    fn checkout_commit_invalid_oid_errors() {
        let (path, repo) = build_linear_repo(2);
        let err = checkout_commit(&repo, "0000000000000000000000000000000000000000", false);
        assert!(err.is_err());
        assert!(repo.head().unwrap().is_branch(), "HEAD must stay on main");
        cleanup(&path);
    }

    #[test]
    fn checkout_commit_dirty_worktree_needs_force() {
        let (path, repo) = build_linear_repo(2);
        let root = repo
            .revparse_single("main~1")
            .unwrap()
            .peel_to_commit()
            .unwrap();
        let tracked = repo.workdir().unwrap().join("file0.txt");
        fs::write(&tracked, "dirty\n").unwrap();

        let err = checkout_commit(&repo, &root.id().to_string(), false).unwrap_err();
        assert_eq!(err.code(), codes::git::DIRTY_WORKTREE);
        assert!(repo.head().unwrap().is_branch(), "refuse must keep HEAD");

        checkout_commit(&repo, &root.id().to_string(), true).unwrap();
        assert_eq!(fs::read_to_string(&tracked).unwrap(), "v0\n");
        assert!(!repo.head().unwrap().is_branch());
        cleanup(&path);
    }

    #[test]
    fn checkout_commit_untracked_file_counts_as_dirty() {
        let (path, repo) = build_linear_repo(2);
        let root = repo
            .revparse_single("main~1")
            .unwrap()
            .peel_to_commit()
            .unwrap();
        fs::write(repo.workdir().unwrap().join("scratch.txt"), "note\n").unwrap();

        // The UI's stash-and-switch flow relies on this: saveStash includes
        // untracked files, so the retry must see a clean worktree.
        let err = checkout_commit(&repo, &root.id().to_string(), false).unwrap_err();
        assert_eq!(err.code(), codes::git::DIRTY_WORKTREE);
        assert!(repo.head().unwrap().is_branch(), "refuse must keep HEAD");
        cleanup(&path);
    }

    #[test]
    fn checkout_commit_same_oid_detaches_without_touching_worktree() {
        let (path, repo) = build_linear_repo(2);
        let head_sha = repo
            .head()
            .unwrap()
            .peel_to_commit()
            .unwrap()
            .id()
            .to_string();
        let before = fs::read_to_string(repo.workdir().unwrap().join("file1.txt")).unwrap();

        // The UI disables this entry, but the backend follows git semantics:
        // checking out the current tip detaches at the same commit, no-op on
        // the worktree.
        checkout_commit(&repo, &head_sha, false).unwrap();
        let head = repo.head().unwrap();
        assert!(!head.is_branch());
        assert_eq!(
            fs::read_to_string(repo.workdir().unwrap().join("file1.txt")).unwrap(),
            before
        );
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

        checkout_branch(&repo, "feature", true).unwrap();

        let head = repo.head().unwrap();
        assert_eq!(head.name().unwrap(), "refs/heads/feature");
        cleanup(&path);
    }

    #[test]
    fn checkout_without_force_errors_on_uncommitted_edit() {
        let (path, repo) = build_linear_repo(1);
        let sha = repo
            .head()
            .unwrap()
            .peel_to_commit()
            .unwrap()
            .id()
            .to_string();
        create_branch(&repo, "feature", &sha, false).unwrap();
        fs::write(path.join("file0.txt"), "dirty\n").unwrap();

        let err = checkout_branch(&repo, "feature", false).unwrap_err();
        assert!(
            err.to_string().to_lowercase().contains("conflict"),
            "expected checkout conflict, got {err}"
        );
        assert_eq!(
            fs::read_to_string(path.join("file0.txt")).unwrap(),
            "dirty\n"
        );
        cleanup(&path);
    }

    #[test]
    fn checkout_force_discards_uncommitted_edit() {
        let (path, repo) = build_linear_repo(2);
        let sha = repo
            .head()
            .unwrap()
            .peel_to_commit()
            .unwrap()
            .id()
            .to_string();
        create_branch(&repo, "feature", &sha, false).unwrap();
        fs::write(path.join("file0.txt"), "dirty\n").unwrap();

        checkout_branch(&repo, "feature", true).unwrap();
        assert_ne!(
            fs::read_to_string(path.join("file0.txt")).unwrap(),
            "dirty\n"
        );
        cleanup(&path);
    }

    #[test]
    fn rename_branch_moves_the_ref() {
        let (path, repo) = build_linear_repo(2);
        let sha = repo
            .head()
            .unwrap()
            .peel_to_commit()
            .unwrap()
            .id()
            .to_string();
        create_branch(&repo, "feature", &sha, false).unwrap();

        rename_branch(&repo, "feature", "renamed", false).unwrap();

        let branches = crate::infrastructure::git::history::list_branches(&repo).unwrap();
        assert!(branches.iter().all(|b| b.name != "feature"));
        assert!(branches.iter().any(|b| b.name == "renamed"));
        cleanup(&path);
    }

    #[test]
    fn rename_current_branch_keeps_head_on_it() {
        let (path, repo) = build_linear_repo(2);
        let sha = repo
            .head()
            .unwrap()
            .peel_to_commit()
            .unwrap()
            .id()
            .to_string();
        create_branch(&repo, "feature", &sha, false).unwrap();
        // Point HEAD at the branch directly — same tip as main, so no
        // worktree change is involved (and libgit2's SAFE checkout would
        // spuriously conflict on the test fixture's racy index anyway).
        repo.set_head("refs/heads/feature").unwrap();

        rename_branch(&repo, "feature", "renamed", false).unwrap();

        let head = repo.head().unwrap();
        assert_eq!(head.shorthand(), Some("renamed"));
        cleanup(&path);
    }

    #[test]
    fn rename_existing_name_without_force_errors() {
        let (path, repo) = build_linear_repo(2);
        let sha = repo
            .head()
            .unwrap()
            .peel_to_commit()
            .unwrap()
            .id()
            .to_string();
        create_branch(&repo, "a", &sha, false).unwrap();
        create_branch(&repo, "b", &sha, false).unwrap();

        assert!(rename_branch(&repo, "a", "b", false).is_err());
        // With force the rename goes through.
        rename_branch(&repo, "a", "b", true).unwrap();
        let branches = crate::infrastructure::git::history::list_branches(&repo).unwrap();
        assert!(branches.iter().any(|b| b.name == "b"));
        assert!(branches.iter().all(|b| b.name != "a"));
        cleanup(&path);
    }

    #[test]
    fn rename_branch_refuses_worktree_occupied_branch() {
        use crate::infrastructure::git::worktree::add_worktree;

        let (path, repo) = build_linear_repo(2);
        let sha = repo
            .head()
            .unwrap()
            .peel_to_commit()
            .unwrap()
            .id()
            .to_string();
        create_branch(&repo, "feature", &sha, false).unwrap();
        let wt_path = std::env::temp_dir().join(format!("gitwave-wt-rename-{sha}"));
        add_worktree(&repo, "feature", &wt_path, "feature", false, None).unwrap();

        let err = rename_branch(&repo, "feature", "renamed", false).unwrap_err();
        assert_eq!(err.code(), codes::git::RENAME_IN_WORKTREE);
        assert!(repo.find_branch("feature", git2::BranchType::Local).is_ok());

        let _ = fs::remove_dir_all(&wt_path);
        cleanup(&path);
    }

    #[test]
    fn rename_branch_keeps_upstream_tracking() {
        let (path, repo) = build_linear_repo(2);
        let sha = repo
            .head()
            .unwrap()
            .peel_to_commit()
            .unwrap()
            .id()
            .to_string();
        create_branch(&repo, "feature", &sha, false).unwrap();
        // set_upstream resolves the remote from config, so a remote must exist.
        repo.remote("origin", "https://example.com/repo.git")
            .unwrap();
        // Stand in for a fetched remote-tracking branch.
        repo.reference(
            "refs/remotes/origin/feature",
            repo.revparse_single(&sha).unwrap().id(),
            true,
            "test: fake remote-tracking ref",
        )
        .unwrap();
        set_branch_upstream(&repo, "feature", Some("origin/feature")).unwrap();

        rename_branch(&repo, "feature", "renamed", false).unwrap();

        let renamed = repo
            .find_branch("renamed", git2::BranchType::Local)
            .unwrap();
        let upstream = renamed.upstream().unwrap();
        assert_eq!(upstream.name().unwrap(), Some("origin/feature"));
        cleanup(&path);
    }

    #[test]
    fn set_branch_upstream_sets_and_clears() {
        let (path, repo) = build_linear_repo(2);
        let sha = repo
            .head()
            .unwrap()
            .peel_to_commit()
            .unwrap()
            .id()
            .to_string();
        create_branch(&repo, "feature", &sha, false).unwrap();
        repo.remote("origin", "https://example.com/repo.git")
            .unwrap();
        repo.reference(
            "refs/remotes/origin/feature",
            repo.revparse_single(&sha).unwrap().id(),
            true,
            "test: fake remote-tracking ref",
        )
        .unwrap();

        set_branch_upstream(&repo, "feature", Some("origin/feature")).unwrap();
        let feature = repo
            .find_branch("feature", git2::BranchType::Local)
            .unwrap();
        assert_eq!(
            feature.upstream().unwrap().name().unwrap(),
            Some("origin/feature")
        );

        set_branch_upstream(&repo, "feature", None).unwrap();
        let feature = repo
            .find_branch("feature", git2::BranchType::Local)
            .unwrap();
        assert!(feature.upstream().is_err());
        cleanup(&path);
    }
}
