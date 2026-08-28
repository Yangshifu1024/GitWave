//! Branch merge operations. See
//! `docs/tasks/feat-history-graph/plan.md` step 4.

use git2::Repository;

use crate::domain::error::{AppError, Result};
use crate::infrastructure::git::git2_adapter::commit_signature;

#[derive(Debug, Clone, Copy, PartialEq, Eq, serde::Serialize)]
#[serde(rename_all = "snake_case")]
pub enum MergeKind {
    /// HEAD was moved directly to the target branch tip. Index and working
    /// tree are refreshed to match.
    FastForward,
    /// HEAD already contains the target tip (equal to it or ahead of it).
    /// Nothing to do — HEAD is left untouched.
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

/// How a merge would play out — computed without touching HEAD, index or
/// working tree (see [`merge_preview`]).
#[derive(Debug, Clone, serde::Serialize)]
pub struct MergePreview {
    /// Target tip is HEAD itself or an ancestor of it — nothing to merge.
    pub up_to_date: bool,
    /// Merge would fast-forward (HEAD has none of the target's commits).
    pub fast_forward: bool,
    /// Paths that would conflict per a dry-run tree merge (empty when
    /// clean or up to date).
    pub conflicts: Vec<String>,
}

/// Dry-run a merge for the confirmation dialog: no ref, index or working
/// tree is touched.
pub fn merge_preview(repo: &Repository, branch_name: &str) -> Result<MergePreview> {
    let (our_oid, their_oid) = resolve_tips(repo, branch_name)?;
    let (ahead, behind) = repo
        .graph_ahead_behind(our_oid, their_oid)
        .map_err(map_git_err)?;
    let up_to_date = behind == 0;
    let fast_forward = !up_to_date && ahead == 0;
    let conflicts = if up_to_date {
        Vec::new()
    } else {
        let ours = repo.find_commit(our_oid).map_err(map_git_err)?;
        let theirs = repo.find_commit(their_oid).map_err(map_git_err)?;
        let mut opts = git2::MergeOptions::new();
        opts.find_renames(true);
        let index = repo
            .merge_commits(&ours, &theirs, Some(&opts))
            .map_err(map_git_err)?;
        index_conflicts(&index)?
    };
    Ok(MergePreview {
        up_to_date,
        fast_forward,
        conflicts,
    })
}

fn resolve_tips(repo: &Repository, branch_name: &str) -> Result<(git2::Oid, git2::Oid)> {
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
    Ok((our_oid, their_oid))
}

fn index_conflicts(index: &git2::Index) -> Result<Vec<String>> {
    Ok(index
        .conflicts()
        .map_err(map_git_err)?
        .filter_map(|c| {
            let ic = c.ok()?;
            ic.our
                .or(ic.their)
                .or(ic.ancestor)
                .map(|e| String::from_utf8_lossy(&e.path).into_owned())
        })
        .collect())
}

/// Merge `branch_name` into the current HEAD.
///
/// On FastForward: HEAD is moved to the target branch tip; index and
/// working tree are refreshed to match.
///
/// On ThreeWay without conflicts: a merge commit is created with both
/// branches as parents. On conflicts: index + MERGE_HEAD are written,
/// no commit is created; paths are listed in `MergeResult::conflicts`
/// for the conflict UI to resolve.
///
/// `no_ff` forces a real merge commit even when a fast-forward would be
/// possible (the result tree equals HEAD's, both tips become parents).
///
/// Returns AlreadyUpToDate if HEAD already contains the target.
pub fn merge_branch(repo: &Repository, branch_name: &str, no_ff: bool) -> Result<MergeResult> {
    let (our_oid, their_oid) = resolve_tips(repo, branch_name)?;

    let (ahead, behind) = repo
        .graph_ahead_behind(our_oid, their_oid)
        .map_err(map_git_err)?;
    if behind == 0 {
        // The target adds nothing: it is HEAD itself or an ancestor of it.
        // This MUST be a no-op — "fast-forwarding" here would rewind the
        // branch and silently drop local commits (the Aug 2026 data-loss
        // incident: merging an already-merged branch reset main by two
        // commits).
        return Ok(MergeResult {
            kind: MergeKind::AlreadyUpToDate,
            conflicts: Vec::new(),
            new_head: our_oid.to_string(),
        });
    }
    if ahead == 0 {
        if !no_ff {
            // True fast-forward: HEAD has none of the target's commits, so
            // the branch can move straight to their tip. Refresh index +
            // working tree too — a ref-only move would leave the checkout
            // stale.
            repo.head()
                .map_err(map_git_err)?
                .set_target(their_oid, "gitwave: merge")
                .map_err(map_git_err)?;
            repo.checkout_head(Some(git2::build::CheckoutBuilder::new().force()))
                .map_err(map_git_err)?;
            return Ok(MergeResult {
                kind: MergeKind::FastForward,
                conflicts: Vec::new(),
                new_head: their_oid.to_string(),
            });
        }
        // --no-ff on a fast-forwardable merge: record a real merge commit.
        // HEAD is an ancestor of the target tip, so the merged result IS
        // the target tree — taking OUR tree here would drop the whole
        // branch (Aug 2026: merging a hot branch produced an empty merge
        // commit and lost 42 files). Refresh the checkout to match.
        let sig = commit_signature(repo)?;
        let our_commit = repo.find_commit(our_oid).map_err(map_git_err)?;
        let their_commit = repo.find_commit(their_oid).map_err(map_git_err)?;
        let new_head_oid = repo
            .commit(
                Some("HEAD"),
                &sig,
                &sig,
                &format!("merge {branch_name}"),
                &their_commit.tree().map_err(map_git_err)?,
                &[&our_commit, &their_commit],
            )
            .map_err(map_git_err)?;
        repo.checkout_head(Some(git2::build::CheckoutBuilder::new().force()))
            .map_err(map_git_err)?;
        return Ok(MergeResult {
            kind: MergeKind::ThreeWay,
            conflicts: Vec::new(),
            new_head: new_head_oid.to_string(),
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
    let conflicts = index_conflicts(&index)?;

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
    let sig = commit_signature(repo)?;
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
    use crate::infrastructure::git::test_helpers::{
        build_linear_repo, init_empty_repo, make_commit, write_and_stage,
    };
    use std::fs;

    fn cleanup(path: &std::path::Path) {
        let _ = fs::remove_dir_all(path);
    }

    #[test]
    fn merge_already_up_to_date() {
        let (path, repo) = build_linear_repo(3);
        // main is at i=2; asking to merge main into main is
        // "already up to date"
        let res = merge_branch(&repo, "main", false).unwrap();
        assert_eq!(res.kind, MergeKind::AlreadyUpToDate);
        assert!(res.conflicts.is_empty());
        cleanup(&path);
    }

    #[test]
    fn merge_ancestor_branch_is_up_to_date_and_keeps_head() {
        // Regression for the Aug 2026 data-loss bug: merging a branch whose
        // tip is already an ancestor of HEAD used to REWIND main to that
        // ancestor, dropping every commit in between.
        let (path, repo) = build_linear_repo(3);
        let main_tip = repo.head().unwrap().peel_to_commit().unwrap().id();
        let i1 = repo.find_commit(main_tip).unwrap().parent(0).unwrap().id();
        repo.branch("feature", &repo.find_commit(i1).unwrap(), false)
            .unwrap();

        let res = merge_branch(&repo, "feature", false).unwrap();

        assert_eq!(res.kind, MergeKind::AlreadyUpToDate);
        assert_eq!(res.new_head, main_tip.to_string());
        assert_eq!(
            repo.head().unwrap().peel_to_commit().unwrap().id(),
            main_tip,
            "HEAD must not move when the target adds nothing"
        );
        cleanup(&path);
    }

    #[test]
    fn merge_no_ff_creates_merge_commit_when_ff_possible() {
        let (path, repo) = build_linear_repo(3);
        repo.config()
            .unwrap()
            .set_str("user.name", "Merge Tester")
            .unwrap();
        repo.config()
            .unwrap()
            .set_str("user.email", "merge@example.com")
            .unwrap();
        let main_tip = repo.head().unwrap().peel_to_commit().unwrap().id();
        let i1 = repo.find_commit(main_tip).unwrap().parent(0).unwrap().id();
        repo.branch("old", &repo.find_commit(i1).unwrap(), false)
            .unwrap();
        repo.set_head("refs/heads/old").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::new().force()))
            .unwrap();

        let res = merge_branch(&repo, "main", true).unwrap();

        let head = repo.head().unwrap().peel_to_commit().unwrap();
        assert_eq!(res.kind, MergeKind::ThreeWay);
        assert_eq!(head.parent_count(), 2, "no-ff must create a merge commit");
        assert_eq!(head.parent(1).unwrap().id(), main_tip);
        // Regression for the Aug 2026 empty-merge data loss: merging a
        // fast-forwardable branch with --no-ff must bring the target's
        // tree into HEAD (HEAD is the target's ancestor).
        assert_eq!(
            head.tree_id(),
            repo.find_commit(main_tip).unwrap().tree_id(),
            "no-ff over a ff-able merge must carry the target tip's tree"
        );
        assert!(
            repo.workdir().unwrap().join("file2.txt").exists(),
            "checkout must be refreshed to the merged tree"
        );
        assert_eq!(
            head.author().name().unwrap(),
            "Merge Tester",
            "merge commit must use the user's configured identity"
        );
        cleanup(&path);
    }

    #[test]
    fn commit_signature_prefers_repo_config() {
        let (path, repo) = build_linear_repo(3);
        repo.config()
            .unwrap()
            .set_str("user.name", "Alice")
            .unwrap();
        repo.config()
            .unwrap()
            .set_str("user.email", "alice@example.com")
            .unwrap();
        let sig = commit_signature(&repo).unwrap();
        assert_eq!(sig.name().unwrap(), "Alice");
        assert_eq!(sig.email().unwrap(), "alice@example.com");
        cleanup(&path);
    }

    #[test]
    fn merge_preview_flags_up_to_date_ff_and_conflicts() {
        let sig = git2::Signature::now("Test", "test@local").unwrap();
        let (path, repo) = build_linear_repo(3);
        let main_tip = repo.head().unwrap().peel_to_commit().unwrap().id();
        let i1 = repo.find_commit(main_tip).unwrap().parent(0).unwrap().id();

        // Ancestor branch: up to date, no conflicts.
        repo.branch("ancestor", &repo.find_commit(i1).unwrap(), false)
            .unwrap();
        let p = merge_preview(&repo, "ancestor").unwrap();
        assert!(p.up_to_date && !p.fast_forward && p.conflicts.is_empty());

        // HEAD behind the target with no own commits: clean fast-forward.
        repo.branch("old", &repo.find_commit(i1).unwrap(), false)
            .unwrap();
        repo.set_head("refs/heads/old").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::new().force()))
            .unwrap();
        let p = merge_preview(&repo, "main").unwrap();
        assert!(p.fast_forward && !p.up_to_date && p.conflicts.is_empty());

        // Diverged edits to the same file: conflict flagged before merging.
        repo.set_head("refs/heads/main").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::new().force()))
            .unwrap();
        let tree = write_and_stage(&repo, "file1.txt", "main-edit\n");
        make_commit(&repo, &sig, "main edit", tree, &[main_tip]);
        repo.branch("side", &repo.find_commit(i1).unwrap(), false)
            .unwrap();
        repo.set_head("refs/heads/side").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::new().force()))
            .unwrap();
        let tree = write_and_stage(&repo, "file1.txt", "side-edit\n");
        make_commit(&repo, &sig, "side edit", tree, &[i1]);

        // Back on main: merging the diverged side branch must flag the
        // conflict without touching anything.
        repo.set_head("refs/heads/main").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::new().force()))
            .unwrap();
        let p = merge_preview(&repo, "side").unwrap();
        assert!(!p.up_to_date && !p.fast_forward);
        assert!(
            p.conflicts.iter().any(|c| c == "file1.txt"),
            "expected file1.txt conflict, got {:?}",
            p.conflicts
        );
        cleanup(&path);
    }

    #[test]
    fn merge_fast_forward_moves_head_and_worktree() {
        // HEAD sits on an ancestor branch; merging the branch ahead of it
        // must move HEAD forward and refresh the working tree.
        let (path, repo) = build_linear_repo(3);
        let main_tip = repo.head().unwrap().peel_to_commit().unwrap().id();
        let i1 = repo.find_commit(main_tip).unwrap().parent(0).unwrap().id();
        repo.branch("old", &repo.find_commit(i1).unwrap(), false)
            .unwrap();
        repo.set_head("refs/heads/old").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::new().force()))
            .unwrap();

        let res = merge_branch(&repo, "main", false).unwrap();

        assert_eq!(res.kind, MergeKind::FastForward);
        assert_eq!(res.new_head, main_tip.to_string());
        assert_eq!(
            repo.head().unwrap().peel_to_commit().unwrap().id(),
            main_tip,
            "fast-forward must advance HEAD to the target tip"
        );
        // The refreshed worktree reflects the new HEAD's file set.
        assert!(repo.workdir().unwrap().join("file2.txt").exists());
        cleanup(&path);
    }

    #[test]
    #[ignore = "empty-repo workdir/init issue pending; see test_helpers::init_empty_repo"]
    fn merge_empty_repo_errors() {
        let (path, repo) = init_empty_repo();
        let err = merge_branch(&repo, "main", false).unwrap_err();
        assert_eq!(err.category(), "Protocol");
        cleanup(&path);
    }
}
