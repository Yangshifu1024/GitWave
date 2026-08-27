//! Fetch / push / pull against a named remote (default `origin`).

use std::sync::{Arc, Mutex};

use git2::{AutotagOption, BranchType, FetchOptions, PushOptions, Repository, StatusOptions};

use crate::domain::error::{AppError, Result};
use crate::infrastructure::git::credentials::{
    CredentialProvider, GitCredentialHelper, SshAgentCredential,
};

#[derive(Debug, Clone, Copy, serde::Serialize, serde::Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum SyncOperation {
    Fetch,
    Pull,
    Push,
}

#[derive(Debug, Clone, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SyncProgress {
    pub operation: SyncOperation,
    pub received_objects: u64,
    pub total_objects: u64,
    pub received_bytes: u64,
}

fn map_git_err(e: git2::Error) -> AppError {
    match e.code() {
        git2::ErrorCode::Auth => AppError::Credential(format!("auth failed: {e}")),
        _ => AppError::Unknown(format!("git: {e}")),
    }
}

fn provider_for_url(url: &str) -> Arc<dyn CredentialProvider> {
    if url.starts_with("git@") || url.starts_with("ssh://") {
        Arc::new(SshAgentCredential::new())
    } else {
        Arc::new(GitCredentialHelper::new(url.to_string()))
    }
}

fn remote_url(repo: &Repository, remote_name: &str) -> Result<String> {
    let remote = repo.find_remote(remote_name).map_err(map_git_err)?;
    remote
        .url()
        .map(str::to_string)
        .ok_or_else(|| AppError::Protocol(format!("remote '{remote_name}' has no URL")))
}

fn attach_transfer_progress(
    mut callbacks: git2::RemoteCallbacks<'_>,
    operation: SyncOperation,
    on_progress: Option<Box<dyn Fn(SyncProgress) + Send>>,
) -> git2::RemoteCallbacks<'_> {
    if let Some(progress) = on_progress {
        let progress = Mutex::new(progress);
        callbacks.transfer_progress(move |stats| {
            if let Ok(guard) = progress.lock() {
                guard(SyncProgress {
                    operation,
                    received_objects: stats.received_objects() as u64,
                    total_objects: stats.total_objects() as u64,
                    received_bytes: stats.received_bytes() as u64,
                });
            }
            true
        });
    }
    callbacks
}

/// Fetch from `remote_name` (typically `origin`). Does not update the working tree.
pub fn fetch(
    repo: &Repository,
    remote_name: &str,
    operation: SyncOperation,
    on_progress: Option<Box<dyn Fn(SyncProgress) + Send>>,
) -> Result<()> {
    let url = remote_url(repo, remote_name)?;
    let creds = provider_for_url(&url);
    let mut remote = repo.find_remote(remote_name).map_err(map_git_err)?;
    let mut fo = FetchOptions::new();
    let cb = attach_transfer_progress(creds.callbacks(), operation, on_progress);
    fo.remote_callbacks(cb);
    fo.download_tags(AutotagOption::Auto);
    remote
        .fetch(&[] as &[&str], Some(&mut fo), None)
        .map_err(|e| match e.code() {
            git2::ErrorCode::Auth => AppError::Credential(format!("fetch auth: {e}")),
            _ => AppError::Network(format!("fetch failed: {e}")),
        })?;
    Ok(())
}

/// Push the current branch to `remote_name` under the same branch name.
pub fn push(
    repo: &Repository,
    remote_name: &str,
    on_progress: Option<Box<dyn Fn(SyncProgress) + Send>>,
) -> Result<()> {
    let url = remote_url(repo, remote_name)?;
    let creds = provider_for_url(&url);
    let mut remote = repo.find_remote(remote_name).map_err(map_git_err)?;
    let head = repo.head().map_err(map_git_err)?;
    if !head.is_branch() {
        return Err(AppError::Protocol("cannot push detached HEAD".into()));
    }
    let branch = head.shorthand().unwrap_or("HEAD").to_string();
    let refspec = format!("refs/heads/{branch}:refs/heads/{branch}");

    let mut po = PushOptions::new();
    let cb = attach_transfer_progress(creds.callbacks(), SyncOperation::Push, on_progress);
    po.remote_callbacks(cb);
    remote
        .push(&[refspec.as_str()], Some(&mut po))
        .map_err(|e| match e.code() {
            git2::ErrorCode::Auth => AppError::Credential(format!("push auth: {e}")),
            _ => AppError::Network(format!("push failed: {e}")),
        })?;
    Ok(())
}

/// Remote names configured on the repository.
pub fn list_remotes(repo: &Repository) -> Result<Vec<String>> {
    let remotes = repo.remotes().map_err(map_git_err)?;
    Ok(remotes.iter().flatten().map(str::to_string).collect())
}

/// Delete `branch_name` on `remote_name` by pushing a bare refspec, then
/// prune the stale local remote-tracking ref (best effort).
pub fn delete_remote_branch(repo: &Repository, remote_name: &str, branch_name: &str) -> Result<()> {
    let url = remote_url(repo, remote_name)?;
    let creds = provider_for_url(&url);
    let mut remote = repo.find_remote(remote_name).map_err(map_git_err)?;
    let refspec = format!(":refs/heads/{branch_name}");
    let mut po = PushOptions::new();
    let cb = attach_transfer_progress(creds.callbacks(), SyncOperation::Push, None);
    po.remote_callbacks(cb);
    remote
        .push(&[refspec.as_str()], Some(&mut po))
        .map_err(|e| match e.code() {
            git2::ErrorCode::Auth => AppError::Credential(format!("push auth: {e}")),
            _ => AppError::Network(format!("delete remote branch failed: {e}")),
        })?;
    if let Ok(mut tracking) =
        repo.find_reference(&format!("refs/remotes/{remote_name}/{branch_name}"))
    {
        let _ = tracking.delete();
    }
    Ok(())
}

/// Options controlling [`pull_with_options`].
#[derive(Debug, Clone, Default)]
pub struct PullOptions {
    /// Remote-tracking branch (short name, e.g. `main`) to pull from. `None`
    /// pulls the current branch's configured upstream, falling back to
    /// `<remote>/<branch>`.
    pub branch: Option<String>,
    /// Rebase local commits onto the pulled branch instead of refusing on
    /// divergence.
    pub rebase: bool,
    /// Stash local (including untracked) changes before pulling and reapply
    /// them afterwards.
    pub stash: bool,
}

/// Whether the worktree (including untracked files) has any entry.
fn worktree_is_dirty(repo: &Repository) -> Result<bool> {
    let mut opts = StatusOptions::new();
    opts.include_untracked(true).recurse_untracked_dirs(true);
    let statuses = repo.statuses(Some(&mut opts)).map_err(map_git_err)?;
    Ok(!statuses.is_empty())
}

/// Fetch then integrate a remote branch into the current branch.
///
/// `opts.branch` selects the remote-tracking branch; divergence either
/// rebases (`opts.rebase`) or errors. With `opts.stash`, local changes are
/// stashed before the pull and reapplied after; a failed reapply keeps the
/// stash and says so.
pub fn pull_with_options(
    repo: &mut Repository,
    remote_name: &str,
    opts: PullOptions,
    on_progress: Option<Box<dyn Fn(SyncProgress) + Send>>,
) -> Result<()> {
    // Newest stash entry is index 0.
    let mut stashed = false;
    if opts.stash && worktree_is_dirty(repo)? {
        crate::infrastructure::git::stash::save_stash(repo, Some("pull: auto stash"))?;
        stashed = true;
    }

    match pull_integrate(repo, remote_name, &opts, on_progress) {
        Ok(()) => {
            if stashed {
                crate::infrastructure::git::stash::pop_stash(repo, 0).map_err(|e| {
                    AppError::Unknown(format!(
                        "pull completed; stash re-apply failed, the stash was kept: {e}"
                    ))
                })?;
            }
            Ok(())
        }
        Err(e) => {
            if stashed {
                // Best-effort restore so a failed pull doesn't swallow changes.
                if crate::infrastructure::git::stash::pop_stash(repo, 0).is_err() {
                    return Err(AppError::Unknown(format!(
                        "{e}; stash re-apply also failed, the stash was kept"
                    )));
                }
            }
            Err(e)
        }
    }
}

fn pull_integrate(
    repo: &Repository,
    remote_name: &str,
    opts: &PullOptions,
    on_progress: Option<Box<dyn Fn(SyncProgress) + Send>>,
) -> Result<()> {
    fetch(repo, remote_name, SyncOperation::Pull, on_progress)?;

    let head = repo.head().map_err(map_git_err)?;
    if !head.is_branch() {
        return Err(AppError::Protocol("cannot pull with detached HEAD".into()));
    }
    let local_name = head.shorthand().unwrap_or("HEAD").to_string();

    let target_ref = match &opts.branch {
        Some(branch) => format!("{remote_name}/{branch}"),
        None => {
            let local_branch = repo
                .find_branch(&local_name, BranchType::Local)
                .map_err(map_git_err)?;
            let upstream = local_branch
                .upstream()
                .ok()
                .and_then(|u| u.name().ok().flatten().map(str::to_string));
            upstream.unwrap_or_else(|| format!("{remote_name}/{local_name}"))
        }
    };

    let their_oid = repo
        .revparse_single(&target_ref)
        .and_then(|obj| obj.peel(git2::ObjectType::Commit))
        .map(|commit| commit.id())
        .map_err(|e| AppError::Protocol(format!("cannot resolve '{target_ref}': {e}")))?;
    let annotated = repo.find_annotated_commit(their_oid).map_err(map_git_err)?;

    let (analysis, _) = repo.merge_analysis(&[&annotated]).map_err(map_git_err)?;

    if analysis.is_up_to_date() {
        return Ok(());
    }

    if opts.rebase {
        let result = crate::infrastructure::git::rebase::rebase_branch(repo, &target_ref)?;
        if result.kind == crate::infrastructure::git::rebase::RebaseKind::Conflicts {
            return Err(AppError::VersionConflict(
                "pull --rebase hit conflicts; local commits were left untouched".into(),
            ));
        }
        return Ok(());
    }

    if analysis.is_fast_forward() {
        let refname = format!("refs/heads/{local_name}");
        let mut reference = repo.find_reference(&refname).map_err(map_git_err)?;
        reference
            .set_target(their_oid, "pull: fast-forward")
            .map_err(map_git_err)?;
        repo.set_head(&refname).map_err(map_git_err)?;
        repo.checkout_head(Some(git2::build::CheckoutBuilder::default().force()))
            .map_err(map_git_err)?;
        return Ok(());
    }

    if analysis.is_normal() {
        return Err(AppError::VersionConflict(
            "pull would require a merge; enable Rebase or use Merge from Branches".into(),
        ));
    }

    Err(AppError::Protocol("pull: unexpected merge analysis".into()))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::git::test_helpers::build_linear_repo;
    use std::fs;

    #[test]
    fn fetch_missing_remote_errors() {
        let (path, repo) = build_linear_repo(1);
        let err = fetch(&repo, "origin", SyncOperation::Fetch, None).expect_err("no origin");
        let _ = fs::remove_dir_all(&path);
        assert_eq!(err.category(), "Unknown");
    }
}
