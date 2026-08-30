//! Revert and cherry-pick single commits (PM 1.1 "Core Git Operations").
//!
//! Both ops refuse a dirty index/worktree so user staging work can never be
//! silently mixed into the result, and both clean libgit2's persistent state
//! on conflict (the app's conflict flow is merge-based; standalone
//! revert/cherry-pick conflicts surface as a plain error listing paths).

use git2::Repository;

use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;
use crate::infrastructure::git::git2_adapter::commit_signature;

fn map_git_err(e: git2::Error) -> AppError {
    AppError::unknown_with(
        codes::git::GIT_ERROR,
        format!("git: {e}"),
        &[("error", e.to_string())],
    )
}

fn parse_oid(oid_str: &str) -> Result<git2::Oid> {
    git2::Oid::from_str(oid_str).map_err(|e| {
        AppError::protocol_with(
            codes::git::INVALID_OID,
            format!("invalid oid: {e}"),
            &[("error", e.to_string())],
        )
    })
}

/// Refuse when the index or worktree carries any change (incl. untracked).
fn ensure_clean(repo: &Repository) -> Result<()> {
    let mut opts = git2::StatusOptions::new();
    opts.include_untracked(true).recurse_untracked_dirs(true);
    let statuses = repo.statuses(Some(&mut opts)).map_err(map_git_err)?;
    if !statuses.is_empty() {
        return Err(AppError::protocol(
            codes::git::DIRTY_WORKTREE,
            "working copy is not clean — commit or stash your changes first",
        ));
    }
    Ok(())
}

fn index_conflicts(index: &git2::Index) -> Result<Vec<String>> {
    let mut out = Vec::new();
    for c in index.conflicts().map_err(map_git_err)? {
        let ic = c.map_err(map_git_err)?;
        if let Some(e) = ic.our.or(ic.their).or(ic.ancestor) {
            out.push(String::from_utf8_lossy(&e.path).into_owned());
        }
    }
    Ok(out)
}

/// Revert `oid`: apply its inverse onto HEAD and create a commit
/// `Revert "<subject>"` (single-parent commits only for v0.1).
///
/// git2 0.20's `Repository::revert` proved a no-op on the index/worktree, so
/// the inverse is built here as a 3-way tree merge: ancestor = the commit's
/// tree, ours = HEAD's tree, theirs = parent's tree. Everything stays in
/// memory until the commit, so conflicts need no persistent-state cleanup.
pub fn revert_commit(repo: &Repository, oid_str: &str) -> Result<String> {
    ensure_clean(repo)?;
    let oid = parse_oid(oid_str)?;
    let commit = repo.find_commit(oid).map_err(|_| {
        AppError::protocol_with(
            codes::git::COMMIT_NOT_FOUND,
            format!("commit not found: {oid_str}"),
            &[("oid", oid_str.to_string())],
        )
    })?;
    if commit.parent_count() > 1 {
        return Err(AppError::protocol(
            codes::git::REVERT_MERGE_COMMIT,
            "reverting a merge commit is not supported (use an explicit reverse merge)",
        ));
    }
    let head = repo
        .head()
        .map_err(map_git_err)?
        .peel_to_commit()
        .map_err(map_git_err)?;

    let parent_tree = commit
        .parent(0)
        .map_err(map_git_err)?
        .tree()
        .map_err(map_git_err)?;
    let mut merged = repo
        .merge_trees(
            &commit.tree().map_err(map_git_err)?,
            &head.tree().map_err(map_git_err)?,
            &parent_tree,
            None,
        )
        .map_err(map_git_err)?;
    let conflicts = index_conflicts(&merged)?;
    if !conflicts.is_empty() {
        return Err(AppError::protocol_with(
            codes::git::REVERT_CONFLICTS,
            format!("revert hit conflicts in: {}", conflicts.join(", ")),
            &[("conflicts", conflicts.join(", "))],
        ));
    }

    let tree_oid = merged.write_tree_to(repo).map_err(map_git_err)?;
    let tree = repo.find_tree(tree_oid).map_err(map_git_err)?;
    let subject = commit.summary().unwrap_or("");
    let message = format!("Revert \"{subject}\"\n\nThis reverts commit {oid}.");
    let sig = commit_signature(repo)?;
    let new_oid = repo
        .commit(Some("HEAD"), &sig, &sig, &message, &tree, &[&head])
        .map_err(map_git_err)?;
    // Refresh index + worktree to the reverted tree (same pattern as the
    // merge fast-forward path).
    repo.checkout_head(Some(git2::build::CheckoutBuilder::new().force()))
        .map_err(map_git_err)?;
    Ok(new_oid.to_string())
}

/// Cherry-pick `oid` onto HEAD: applies to index + worktree and creates a
/// commit preserving the original author, message and a picked-from trailer.
pub fn cherry_pick_commit(repo: &Repository, oid_str: &str) -> Result<String> {
    ensure_clean(repo)?;
    let oid = parse_oid(oid_str)?;
    let commit = repo.find_commit(oid).map_err(|_| {
        AppError::protocol_with(
            codes::git::COMMIT_NOT_FOUND,
            format!("commit not found: {oid_str}"),
            &[("oid", oid_str.to_string())],
        )
    })?;

    repo.cherrypick(&commit, None).map_err(map_git_err)?;
    let on_disk = repo.index().map_err(map_git_err)?;
    let conflicts = index_conflicts(&on_disk)?;
    if !conflicts.is_empty() {
        let _ = repo.cleanup_state();
        return Err(AppError::protocol_with(
            codes::git::CHERRY_PICK_CONFLICTS,
            format!("cherry-pick hit conflicts in: {}", conflicts.join(", ")),
            &[("conflicts", conflicts.join(", "))],
        ));
    }

    let mut index = repo.index().map_err(map_git_err)?;
    let tree_oid = index.write_tree().map_err(map_git_err)?;
    let head = repo
        .head()
        .map_err(map_git_err)?
        .peel_to_commit()
        .map_err(map_git_err)?;
    // Refuse empty picks: the commit is already contained in HEAD (e.g. it
    // arrived via a merge). Git stops here with "nothing to commit" — so do
    // we, instead of silently creating a no-change commit.
    if tree_oid == head.tree_id() {
        let _ = repo.cleanup_state();
        return Err(AppError::protocol(
            codes::git::CHERRY_PICK_NO_CHANGES,
            "cherry-pick produced no changes — this commit is already contained in the current branch",
        ));
    }
    let tree = repo.find_tree(tree_oid).map_err(map_git_err)?;
    let message = format!(
        "{}\n\n(cherry picked from commit {})",
        commit.message().unwrap_or("").trim_end(),
        oid
    );
    let sig = commit_signature(repo)?;
    let new_oid = repo
        .commit(
            Some("HEAD"),
            &commit.author(),
            &sig,
            &message,
            &tree,
            &[&head],
        )
        .map_err(map_git_err)?;
    let _ = repo.cleanup_state();
    Ok(new_oid.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::{build_linear_repo, write_and_stage};
    use std::fs;

    #[test]
    fn revert_creates_inverse_commit() {
        let (path, repo) = build_linear_repo(3);
        let tip = repo.head().unwrap().peel_to_commit().unwrap();
        let tip_sha = tip.id().to_string();
        // Revert tip: file2.txt was added by it, so it must disappear.
        let sha = revert_commit(&repo, &tip_sha).unwrap();
        let head = repo.head().unwrap().peel_to_commit().unwrap();
        assert_ne!(head.id().to_string(), tip_sha);
        assert!(head.summary().unwrap().starts_with("Revert \"commit 2\""));
        assert!(head.message().unwrap().contains(&tip_sha));
        assert!(
            !repo.workdir().unwrap().join("file2.txt").exists(),
            "reverted file must leave the worktree"
        );
        assert_eq!(head.parent(0).unwrap().id(), tip.id());
        let _ = sha;
        cleanup(&path);
    }

    #[test]
    fn revert_merge_commit_errors() {
        // build a merge via two branches, then try reverting it.
        let (path, repo) = build_linear_repo(2);
        let base = repo.head().unwrap().peel_to_commit().unwrap().id();
        repo.branch("side", &repo.find_commit(base).unwrap(), false)
            .unwrap();
        let sig = git2::Signature::now("T", "t@l").unwrap();
        repo.set_head("refs/heads/side").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::new().force()))
            .unwrap();
        let tree = write_and_stage(&repo, "side.txt", "side\n");
        let side = {
            let parent = repo.head().unwrap().peel_to_commit().unwrap();
            repo.commit(
                Some("HEAD"),
                &sig,
                &sig,
                "side commit",
                &repo.find_tree(tree).unwrap(),
                &[&parent],
            )
            .unwrap()
        };
        repo.set_head("refs/heads/main").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::new().force()))
            .unwrap();
        let side_commit = repo.find_commit(side).unwrap();
        let our = repo.head().unwrap().peel_to_commit().unwrap();
        let merged = {
            let mut index = repo.merge_commits(&our, &side_commit, None).unwrap();
            let tree_oid = index.write_tree_to(&repo).unwrap();
            repo.commit(
                Some("HEAD"),
                &sig,
                &sig,
                "merge side",
                &repo.find_tree(tree_oid).unwrap(),
                &[&our, &side_commit],
            )
            .unwrap()
        };
        let err = revert_commit(&repo, &merged.to_string()).unwrap_err();
        assert_eq!(err.category(), "Protocol");
        cleanup(&path);
    }

    #[test]
    fn cherry_pick_preserves_author_and_trailer() {
        let (path, repo) = build_linear_repo(2);
        let base = repo.head().unwrap().peel_to_commit().unwrap().id();
        repo.branch("side", &repo.find_commit(base).unwrap(), false)
            .unwrap();
        let sig = git2::Signature::now("Original Author", "orig@example.com").unwrap();
        repo.set_head("refs/heads/side").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::new().force()))
            .unwrap();
        let tree = write_and_stage(&repo, "picked.txt", "picked\n");
        let picked = {
            let parent = repo.head().unwrap().peel_to_commit().unwrap();
            repo.commit(
                Some("HEAD"),
                &sig,
                &sig,
                "pick me",
                &repo.find_tree(tree).unwrap(),
                &[&parent],
            )
            .unwrap()
        };
        repo.set_head("refs/heads/main").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::new().force()))
            .unwrap();

        let sha = cherry_pick_commit(&repo, &picked.to_string()).unwrap();
        let head = repo.head().unwrap().peel_to_commit().unwrap();
        assert_eq!(head.id().to_string(), sha);
        assert_eq!(head.summary(), Some("pick me"));
        assert_eq!(head.author().name().unwrap(), "Original Author");
        assert!(head.message().unwrap().contains("(cherry picked from"));
        assert!(repo.workdir().unwrap().join("picked.txt").exists());
        cleanup(&path);
    }

    #[test]
    fn refuse_when_worktree_dirty() {
        let (path, repo) = build_linear_repo(2);
        let tip = repo.head().unwrap().peel_to_commit().unwrap().id();
        fs::write(repo.workdir().unwrap().join("file1.txt"), "dirty\n").unwrap();
        let err = cherry_pick_commit(&repo, &tip.to_string()).unwrap_err();
        assert_eq!(err.category(), "Protocol");
        cleanup(&path);
    }

    fn cleanup(path: &std::path::Path) {
        let _ = fs::remove_dir_all(path);
    }
}

#[cfg(test)]
mod empty_pick_tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::{build_linear_repo, write_and_stage};
    use std::fs;

    #[test]
    fn cherry_pick_already_contained_refuses() {
        let (path, repo) = build_linear_repo(2);
        let base = repo.head().unwrap().peel_to_commit().unwrap().id();
        repo.branch("side", &repo.find_commit(base).unwrap(), false)
            .unwrap();
        let sig = git2::Signature::now("T", "t@l").unwrap();
        repo.set_head("refs/heads/side").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::new().force()))
            .unwrap();
        let tree = write_and_stage(&repo, "side.txt", "side\n");
        let picked = {
            let parent = repo.head().unwrap().peel_to_commit().unwrap();
            repo.commit(
                Some("HEAD"),
                &sig,
                &sig,
                "side work",
                &repo.find_tree(tree).unwrap(),
                &[&parent],
            )
            .unwrap()
        };
        // Merge side into main: the commit is now contained in HEAD.
        repo.set_head("refs/heads/main").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::new().force()))
            .unwrap();
        let our = repo.head().unwrap().peel_to_commit().unwrap();
        let side_commit = repo.find_commit(picked).unwrap();
        let mut index = repo.merge_commits(&our, &side_commit, None).unwrap();
        let tree_oid = index.write_tree_to(&repo).unwrap();
        repo.commit(
            Some("HEAD"),
            &sig,
            &sig,
            "merge side",
            &repo.find_tree(tree_oid).unwrap(),
            &[&our, &side_commit],
        )
        .unwrap();
        // Refresh on-disk index + worktree so the clean-check passes (the
        // in-memory merge index never touched the disk).
        repo.checkout_head(Some(git2::build::CheckoutBuilder::new().force()))
            .unwrap();

        let err = cherry_pick_commit(&repo, &picked.to_string()).unwrap_err();
        assert_eq!(err.category(), "Protocol");
        let msg = match &err {
            AppError::Protocol { message: m, .. } => m.clone(),
            other => other.to_string(),
        };
        assert!(
            msg.contains("already contained"),
            "expected empty-pick message, got: {msg}"
        );
        fs::remove_dir_all(&path).ok();
    }
}
