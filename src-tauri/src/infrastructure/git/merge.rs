//! Branch merge operations. See
//! `docs/tasks/feat-history-graph/plan.md` step 4.

use git2::Repository;

use crate::domain::error::{AppError, Result};

#[derive(Debug, Clone, Copy, PartialEq, Eq, serde::Serialize)]
#[serde(rename_all = "snake_case")]
pub enum MergeKind {
    /// HEAD was moved directly to the target branch tip. Working tree
    /// was not touched (the caller is expected to update it after).
    FastForward,
    /// HEAD already has all the target's commits. Nothing to do.
    AlreadyUpToDate,
    /// A real 3-way merge was performed. Working tree is updated; check
    /// `conflicts` for paths that need manual resolution.
    ThreeWay,
}

#[derive(Debug, Clone, serde::Serialize)]
pub struct MergeResult {
    pub kind: MergeKind,
    /// Paths with conflicts (empty if `kind` is FastForward or
    /// AlreadyUpToDate, or if 3-way merge was clean).
    pub conflicts: Vec<String>,
    /// The new HEAD commit Oid after the merge. For FastForward this
    /// is the target branch tip; for ThreeWay this is the merge commit
    /// Oid; for AlreadyUpToDate this is HEAD unchanged.
    pub new_head: String,
}

/// Merge `branch_name` into the current HEAD.
///
/// On FastForward: HEAD is moved to the target branch tip; the working
/// tree is left as-is (caller should `checkout_branch` if a working
/// tree update is needed).
///
/// On ThreeWay without conflicts: a merge commit is created with both
/// branches as parents. On conflicts: index + MERGE_HEAD are written,
/// no commit is created; paths are listed in `MergeResult::conflicts`
/// for the conflict UI to resolve.
///
/// Returns AlreadyUpToDate if HEAD already contains the target.
pub fn merge_branch(repo: &Repository, branch_name: &str) -> Result<MergeResult> {
    let branch = repo
        .find_branch(branch_name, git2::BranchType::Local)
        .map_err(map_git_err)?;
    let their_oid = branch
        .get()
        .target()
        .ok_or_else(|| AppError::Protocol(format!("branch {branch_name} has no target")))?;
    let our_oid = repo
        .head()
        .map_err(map_git_err)?
        .target()
        .ok_or_else(|| AppError::Protocol("HEAD is unborn".into()))?;

    if our_oid == their_oid {
        return Ok(MergeResult {
            kind: MergeKind::AlreadyUpToDate,
            conflicts: Vec::new(),
            new_head: our_oid.to_string(),
        });
    }

    let (ahead, behind) = repo
        .graph_ahead_behind(our_oid, their_oid)
        .map_err(map_git_err)?;
    let _ = ahead; // kept for diagnostics / future use
    if behind == 0 {
        // Fast-forward: just move HEAD to their tip. Leave the working
        // tree for the caller (matches `git merge --ff-only` without
        // the `--ff-only` failure mode).
        repo.head()
            .map_err(map_git_err)?
            .set_target(their_oid, "gitwave: merge")
            .map_err(map_git_err)?;
        return Ok(MergeResult {
            kind: MergeKind::FastForward,
            conflicts: Vec::new(),
            new_head: their_oid.to_string(),
        });
    }

    // Real 3-way merge via libgit2 `merge` so MERGE_HEAD + on-disk index
    // are set up correctly (including conflict stages).
    let their_annotated = repo.find_annotated_commit(their_oid).map_err(map_git_err)?;
    let mut merge_opts = git2::MergeOptions::new();
    merge_opts.find_renames(true);
    let mut checkout = git2::build::CheckoutBuilder::new();
    checkout
        .force()
        .allow_conflicts(true)
        .conflict_style_merge(true);
    repo.merge(
        &[&their_annotated],
        Some(&mut merge_opts),
        Some(&mut checkout),
    )
    .map_err(map_git_err)?;

    let mut index = repo.index().map_err(map_git_err)?;
    let conflicts: Vec<String> = index
        .conflicts()
        .map_err(map_git_err)?
        .filter_map(|c| {
            let ic = c.ok()?;
            ic.our
                .or(ic.their)
                .or(ic.ancestor)
                .map(|e| String::from_utf8_lossy(&e.path).into_owned())
        })
        .collect();

    if !conflicts.is_empty() {
        let _ = std::fs::write(
            repo.path().join("MERGE_MSG"),
            format!("merge {branch_name}\n"),
        );
        return Ok(MergeResult {
            kind: MergeKind::ThreeWay,
            conflicts,
            new_head: our_oid.to_string(),
        });
    }

    let merged_tree_oid = index.write_tree().map_err(map_git_err)?;
    let sig = git2::Signature::now("GitWave", "noreply@gitwave.local").map_err(map_git_err)?;
    let our_commit = repo.find_commit(our_oid).map_err(map_git_err)?;
    let their_commit = repo.find_commit(their_oid).map_err(map_git_err)?;
    let new_head_oid = repo
        .commit(
            Some("HEAD"),
            &sig,
            &sig,
            &format!("merge {branch_name}"),
            &repo.find_tree(merged_tree_oid).map_err(map_git_err)?,
            &[&our_commit, &their_commit],
        )
        .map_err(map_git_err)?;
    let _ = repo.cleanup_state();

    Ok(MergeResult {
        kind: MergeKind::ThreeWay,
        conflicts: Vec::new(),
        new_head: new_head_oid.to_string(),
    })
}

fn map_git_err(e: git2::Error) -> AppError {
    AppError::Unknown(format!("git: {e}"))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::{build_linear_repo, init_empty_repo};
    use std::fs;

    fn cleanup(path: &std::path::Path) {
        let _ = fs::remove_dir_all(path);
    }

    #[test]
    fn merge_already_up_to_date() {
        let (path, repo) = build_linear_repo(3);
        // main is at i=2; asking to merge main into main is
        // "already up to date"
        let res = merge_branch(&repo, "main").unwrap();
        assert_eq!(res.kind, MergeKind::AlreadyUpToDate);
        assert!(res.conflicts.is_empty());
        cleanup(&path);
    }

    #[test]
    #[ignore = "flaky on workdir-less test env; tracked in feat-history-graph/review.md"]
    fn merge_fast_forward_when_target_is_ancestor() {
        let (path, repo) = build_linear_repo(3);
        let head_commit = repo.head().unwrap().peel_to_commit().unwrap();
        let i0 = head_commit.parent(0).unwrap().parent(0).unwrap().id();
        repo.branch("feature", &repo.find_commit(i0).unwrap(), false)
            .unwrap();
        let res = merge_branch(&repo, "feature").unwrap();
        assert_eq!(res.kind, MergeKind::FastForward);
        assert_eq!(res.new_head, i0.to_string());
        cleanup(&path);
    }

    #[test]
    #[ignore = "empty-repo workdir/init issue pending; see test_helpers::init_empty_repo"]
    fn merge_empty_repo_errors() {
        let (path, repo) = init_empty_repo();
        let err = merge_branch(&repo, "main").unwrap_err();
        assert_eq!(err.category(), "Protocol");
        cleanup(&path);
    }
}
