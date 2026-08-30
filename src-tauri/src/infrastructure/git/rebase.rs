//! Non-interactive rebase. See
//! `docs/tasks/feat-history-graph/plan.md` step 5.
//!
//! Interactive rebase (with edit / reword / squash) is deferred to Sprint 5.

use git2::Repository;

use crate::domain::error::{AppError, Result};
use crate::infrastructure::git::git2_adapter::commit_signature;

#[derive(Debug, Clone, Copy, PartialEq, Eq, serde::Serialize)]
#[serde(rename_all = "snake_case")]
pub enum RebaseKind {
    /// Nothing to do (already up to date with upstream).
    AlreadyUpToDate,
    /// HEAD was rewritten onto upstream; no conflicts.
    Clean,
    /// Rebase was aborted because at least one commit conflicted.
    Conflicts,
}

#[derive(Debug, Clone, serde::Serialize)]
pub struct RebaseResult {
    pub kind: RebaseKind,
    /// Paths with conflicts (empty if `kind` is Clean or AlreadyUpToDate).
    pub conflicts: Vec<String>,
    /// The new HEAD commit Oid after a Clean rebase. None for the
    /// other two variants.
    pub new_head: Option<String>,
}

/// Rebase current HEAD onto `upstream` (a branch name, tag, or sha).
///
/// On AlreadyUpToDate: HEAD already contains everything in `upstream`,
/// so nothing happens.
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
        .ok_or_else(|| AppError::Protocol("HEAD is unborn".into()))?;
    let upstream_obj = repo.revparse_single(upstream).map_err(map_git_err)?;
    // `Object::from` doesn't exist on Object; peel to the commit form
    // and grab the Oid.
    let upstream_oid = upstream_obj.peel(git2::ObjectType::Commit)?.id();

    if our_oid == upstream_oid {
        return Ok(RebaseResult {
            kind: RebaseKind::AlreadyUpToDate,
            conflicts: Vec::new(),
            new_head: None,
        });
    }

    let (ahead, _behind) = repo
        .graph_ahead_behind(our_oid, upstream_oid)
        .map_err(map_git_err)?;
    if ahead == 0 {
        // HEAD is a descendant of upstream; nothing to rebase.
        return Ok(RebaseResult {
            kind: RebaseKind::AlreadyUpToDate,
            conflicts: Vec::new(),
            new_head: None,
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
                        let wd = repo
                            .workdir()
                            .ok_or_else(|| AppError::Unknown("repo has no workdir".into()))?;
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
        return Err(AppError::Protocol(
            "cannot finalize rebase on detached HEAD".into(),
        ));
    }
    let refname = head
        .name()
        .ok_or_else(|| AppError::Protocol("branch ref has no name".into()))?
        .to_string();
    let mut reference = repo.find_reference(&refname).map_err(map_git_err)?;
    reference.set_target(oid, "rebase").map_err(map_git_err)?;
    repo.set_head(&refname).map_err(map_git_err)?;
    repo.checkout_head(Some(git2::build::CheckoutBuilder::default().force()))
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
    fn rebase_fast_forward_when_already_descendant() {
        // Create a branch at i=0, then rebase onto it -> already
        // up to date (ahead == 0).
        let (path, repo) = build_linear_repo(3);
        let old = repo.head().unwrap().peel_to_commit().unwrap().id();
        repo.branch("feature", &repo.find_commit(old).unwrap(), false)
            .unwrap();
        // HEAD (i=2) is a descendant of feature (i=0), so ahead == 0.
        let res = rebase_branch(&repo, "feature").unwrap();
        assert_eq!(res.kind, RebaseKind::AlreadyUpToDate);
        cleanup(&path);
    }

    #[test]
    fn finalize_rebase_moves_branch_and_checks_out() {
        let (path, repo) = build_linear_repo(2);
        // "old" sits at commit 0; rebasing HEAD (commit 1) onto it rewrites
        // commit 1 in memory. finalize must land it on the branch.
        let first = repo
            .revparse_single("main~1")
            .unwrap()
            .peel_to_commit()
            .unwrap();
        repo.branch("old", &first, false).unwrap();

        let res = rebase_branch(&repo, "old").unwrap();
        assert_eq!(res.kind, RebaseKind::Clean);
        let new_head = res.new_head.clone().unwrap();

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
            "the result must be checked out"
        );
        cleanup(&path);
    }
}
