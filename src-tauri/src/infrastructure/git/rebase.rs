//! Non-interactive rebase. See
//! `docs/tasks/feat-history-graph/plan.md` step 5.
//!
//! Interactive rebase (with edit / reword / squash) is deferred to Sprint 5.

use git2::Repository;

use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;
use crate::infrastructure::git::git2_adapter::commit_signature;

#[derive(Debug, Clone, Copy, PartialEq, Eq, serde::Serialize)]
#[serde(rename_all = "snake_case")]
pub enum RebaseKind {
    /// Nothing to do: HEAD already contains everything in `upstream`
    /// (same commit, or upstream is an ancestor of HEAD).
    AlreadyUpToDate,
    /// HEAD was strictly behind upstream, so the branch fast-forwarded.
    FastForward,
    /// HEAD was rewritten onto upstream; no conflicts.
    Clean,
    /// Rebase was aborted because at least one commit conflicted.
    Conflicts,
}

#[derive(Debug, Clone, serde::Serialize)]
pub struct RebaseResult {
    pub kind: RebaseKind,
    /// Paths with conflicts (empty unless `kind` is Conflicts).
    pub conflicts: Vec<String>,
    /// The new HEAD commit Oid after a Clean rebase or a FastForward.
    /// None for the other two variants.
    pub new_head: Option<String>,
}

/// Rebase current HEAD onto `upstream` (a branch name, tag, or sha).
///
/// On AlreadyUpToDate: HEAD already contains everything in `upstream`,
/// so nothing happens.
///
/// On FastForward: HEAD had no commits not in `upstream`, so there was
/// nothing to rewrite; the current branch was fast-forwarded to
/// `upstream` instead (what `git rebase` does). Callers must have
/// refused a dirty worktree — the landing force-checks out.
///
/// On Clean: each commit in HEAD that is not in `upstream` is
/// cherry-picked (rewritten) onto `upstream`. The result is a new
/// linear history.
///
/// On Conflicts: a conflict was encountered and the rebase was
/// aborted. HEAD is back where it was before the rebase started. The
/// `conflicts` list contains the first file that conflicted.
pub fn rebase_branch(repo: &Repository, upstream: &str) -> Result<RebaseResult> {
    let our_oid = repo
        .head()
        .map_err(map_git_err)?
        .target()
        .ok_or_else(|| AppError::protocol(codes::git::HEAD_UNBORN, "HEAD is unborn"))?;
    let upstream_obj = repo.revparse_single(upstream).map_err(map_git_err)?;
    // `Object::from` doesn't exist on Object; peel to the commit form
    // and grab the Oid.
    let upstream_oid = upstream_obj.peel(git2::ObjectType::Commit)?.id();

    let (ahead, behind) = repo
        .graph_ahead_behind(our_oid, upstream_oid)
        .map_err(map_git_err)?;
    if behind == 0 {
        // HEAD contains every commit of upstream (same commit, or
        // upstream is an ancestor of HEAD): nothing to rebase.
        return Ok(RebaseResult {
            kind: RebaseKind::AlreadyUpToDate,
            conflicts: Vec::new(),
            new_head: None,
        });
    }
    if ahead == 0 {
        // HEAD is strictly behind upstream: a rebase has no commits to
        // rewrite, and `git rebase` fast-forwards. Land upstream on the
        // current branch here; callers refuse a dirty worktree before
        // this, so the force checkout inside cannot drop changes.
        finalize_rebase(repo, &upstream_oid.to_string())?;
        return Ok(RebaseResult {
            kind: RebaseKind::FastForward,
            conflicts: Vec::new(),
            new_head: Some(upstream_oid.to_string()),
        });
    }

    let sig = commit_signature(repo)?;
    let mut opts = git2::RebaseOptions::new();
    opts.inmemory(true); // Don't touch the workdir; let caller decide
                         // `Rebase::rebase` takes `Option<AnnotatedCommit>` not `Option<Oid>`.
    let our_annotated = repo.find_annotated_commit(our_oid).map_err(map_git_err)?;
    let upstream_annotated = repo
        .find_annotated_commit(upstream_oid)
        .map_err(map_git_err)?;
    let mut rebase = repo
        .rebase(
            Some(&our_annotated),
            Some(&upstream_annotated),
            None,
            Some(&mut opts),
        )
        .map_err(map_git_err)?;

    let mut last_new_head: Option<String> = None;
    let mut conflicts: Vec<String> = Vec::new();

    while let Some(op_result) = rebase.next() {
        match op_result {
            Ok(_op) => {
                // Try to commit this pick. If this fails, the rebase has
                // a conflict; collect the first path and abort.
                match rebase.commit(None, &sig, None) {
                    Ok(oid) => last_new_head = Some(oid.to_string()),
                    Err(e) => {
                        // Conflict; try to extract the conflicted path
                        // from the current index.
                        let wd = repo.workdir().ok_or_else(|| {
                            AppError::protocol(codes::git::BARE_REPO, "repo has no workdir")
                        })?;
                        let idx = git2::Repository::open(wd)
                            .map_err(map_git_err)?
                            .index()
                            .map_err(map_git_err)?;
                        let cit = idx.conflicts().map_err(map_git_err)?;
                        for c in cit {
                            let ic = c.map_err(map_git_err)?;
                            if let Some(e) = ic.our.or(ic.their).or(ic.ancestor) {
                                conflicts.push(String::from_utf8_lossy(&e.path).into_owned());
                                break;
                            }
                        }
                        let _ = rebase.abort();
                        // `e` is the conflict error; surface a generic msg
                        // since the precise file may already be in `conflicts`.
                        let _ = e;
                        return Ok(RebaseResult {
                            kind: RebaseKind::Conflicts,
                            conflicts,
                            new_head: None,
                        });
                    }
                }
            }
            Err(e) => {
                // The rebase itself errored (e.g. cannot apply patch).
                conflicts.clear();
                conflicts.push(format!("rebase operation: {e}"));
                let _ = rebase.abort();
                return Ok(RebaseResult {
                    kind: RebaseKind::Conflicts,
                    conflicts,
                    new_head: None,
                });
            }
        }
    }

    // rebase.next() returned None: all picks applied cleanly.
    rebase.finish(Some(&sig)).map_err(map_git_err)?;
    Ok(RebaseResult {
        kind: RebaseKind::Clean,
        conflicts: Vec::new(),
        new_head: last_new_head,
    })
}

/// Point the current branch at `oid` and check the result out.
///
/// [`rebase_branch`] runs in memory (refs, index and workdir are never
/// touched), so a `Clean` result only exists as rewritten commits until the
/// caller finalizes with this — same landing sequence as pull's
/// fast-forward block.
pub fn finalize_rebase(repo: &Repository, oid: &str) -> Result<()> {
    let oid = git2::Oid::from_str(oid).map_err(map_git_err)?;
    let head = repo.head().map_err(map_git_err)?;
    if !head.is_branch() {
        return Err(AppError::protocol(
            codes::git::FINALIZE_DETACHED_HEAD,
            "cannot finalize rebase on detached HEAD",
        ));
    }
    let refname = head
        .name()
        .ok_or_else(|| AppError::protocol(codes::git::REF_NO_NAME, "branch ref has no name"))?
        .to_string();
    let mut reference = repo.find_reference(&refname).map_err(map_git_err)?;
    reference.set_target(oid, "rebase").map_err(map_git_err)?;
    repo.set_head(&refname).map_err(map_git_err)?;
    repo.checkout_head(Some(git2::build::CheckoutBuilder::default().force()))
        .map_err(map_git_err)?;
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
    fn rebase_already_up_to_date() {
        let (path, repo) = build_linear_repo(3);
        // Try to rebase onto a branch that is an ancestor of HEAD -> already
        // up to date. We don't have such a branch; instead, rebase onto
        // HEAD itself -> already up to date.
        let res = rebase_branch(&repo, "HEAD").unwrap();
        assert_eq!(res.kind, RebaseKind::AlreadyUpToDate);
        cleanup(&path);
    }

    #[test]
    fn rebase_onto_ancestor_is_up_to_date() {
        // Rebase onto a branch that is an ancestor of HEAD: HEAD fully
        // contains it (behind == 0), so nothing happens.
        let (path, repo) = build_linear_repo(3);
        let first = repo
            .revparse_single("main~2")
            .unwrap()
            .peel_to_commit()
            .unwrap();
        repo.branch("feature", &first, false).unwrap();
        let res = rebase_branch(&repo, "feature").unwrap();
        assert_eq!(res.kind, RebaseKind::AlreadyUpToDate);
        cleanup(&path);
    }

    #[test]
    fn rebase_strictly_behind_fast_forwards() {
        // HEAD on a branch strictly behind `main`: a rebase has no picks,
        // so it must fast-forward and land (branch ref, HEAD, workdir),
        // not report a silent "already up to date".
        let (path, repo) = build_linear_repo(3);
        let first = repo
            .revparse_single("main~2")
            .unwrap()
            .peel_to_commit()
            .unwrap();
        repo.branch("behind", &first, false).unwrap();
        repo.set_head("refs/heads/behind").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::default().force()))
            .unwrap();
        let main_tip = repo
            .revparse_single("main")
            .unwrap()
            .peel_to_commit()
            .unwrap()
            .id()
            .to_string();
        assert!(
            !path.join("file2.txt").exists(),
            "precondition: behind workdir"
        );

        let res = rebase_branch(&repo, "main").unwrap();
        assert_eq!(res.kind, RebaseKind::FastForward);
        assert_eq!(res.new_head.as_deref(), Some(main_tip.as_str()));

        assert_eq!(
            repo.head()
                .unwrap()
                .peel_to_commit()
                .unwrap()
                .id()
                .to_string(),
            main_tip,
            "HEAD must move to upstream"
        );
        assert_eq!(
            repo.find_reference("refs/heads/behind")
                .unwrap()
                .target()
                .unwrap()
                .to_string(),
            main_tip,
            "the current branch ref must fast-forward"
        );
        assert_eq!(
            fs::read_to_string(path.join("file2.txt")).unwrap(),
            "v2\n",
            "the fast-forwarded content must be checked out"
        );
        cleanup(&path);
    }

    #[test]
    fn finalize_rebase_moves_branch_and_checks_out() {
        use crate::infrastructure::git::test_helpers::{make_commit, write_and_stage};

        // Diverged: `old` gains its own commit after branching, so rebasing
        // main onto old truly rewrites main's tip in memory (a strictly
        // linear "rebase onto parent" would just be up to date). finalize
        // must land the rewritten commit on the branch.
        let (path, repo) = build_linear_repo(2);
        let first = repo
            .revparse_single("main~1")
            .unwrap()
            .peel_to_commit()
            .unwrap();
        repo.branch("old", &first, false).unwrap();

        repo.set_head("refs/heads/old").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::default().force()))
            .unwrap();
        let sig = git2::Signature::now("Test", "test@local").unwrap();
        let tree = write_and_stage(&repo, "file_old.txt", "old\n");
        make_commit(&repo, &sig, "commit old", tree, &[first.id()]);

        repo.set_head("refs/heads/main").unwrap();
        repo.checkout_head(Some(git2::build::CheckoutBuilder::default().force()))
            .unwrap();
        let main_tip = repo
            .revparse_single("main")
            .unwrap()
            .peel_to_commit()
            .unwrap()
            .id()
            .to_string();

        let res = rebase_branch(&repo, "old").unwrap();
        assert_eq!(res.kind, RebaseKind::Clean);
        let new_head = res.new_head.clone().unwrap();
        assert_ne!(new_head, main_tip, "the rewrite must produce a new commit");

        finalize_rebase(&repo, &new_head).unwrap();

        assert_eq!(
            repo.head()
                .unwrap()
                .peel_to_commit()
                .unwrap()
                .id()
                .to_string(),
            new_head,
            "HEAD must point at the rewritten commit"
        );
        assert_eq!(
            repo.find_reference("refs/heads/main")
                .unwrap()
                .target()
                .unwrap()
                .to_string(),
            new_head,
            "the branch ref must move to the rewritten commit"
        );
        assert_eq!(
            fs::read_to_string(path.join("file1.txt")).unwrap(),
            "v1\n",
            "the rewritten commit's changes must be checked out"
        );
        assert_eq!(
            fs::read_to_string(path.join("file_old.txt")).unwrap(),
            "old\n",
            "the new base's content must be checked out"
        );
        cleanup(&path);
    }
}
