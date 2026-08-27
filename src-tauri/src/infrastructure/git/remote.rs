//! Fetch / push / pull against a named remote (default `origin`).

use std::sync::Arc;

use git2::{AutotagOption, BranchType, FetchOptions, PushOptions, Repository};

use crate::domain::error::{AppError, Result};
use crate::infrastructure::git::credentials::{
    CredentialProvider, GitCredentialHelper, SshAgentCredential,
};

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

/// Fetch from `remote_name` (typically `origin`). Does not update the working tree.
pub fn fetch(repo: &Repository, remote_name: &str) -> Result<()> {
    let url = remote_url(repo, remote_name)?;
    let creds = provider_for_url(&url);
    let mut remote = repo.find_remote(remote_name).map_err(map_git_err)?;
    let mut fo = FetchOptions::new();
    fo.remote_callbacks(creds.callbacks());
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
pub fn push(repo: &Repository, remote_name: &str) -> Result<()> {
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
    po.remote_callbacks(creds.callbacks());
    remote
        .push(&[refspec.as_str()], Some(&mut po))
        .map_err(|e| match e.code() {
            git2::ErrorCode::Auth => AppError::Credential(format!("push auth: {e}")),
            _ => AppError::Network(format!("push failed: {e}")),
        })?;
    Ok(())
}

/// Fetch then fast-forward the current branch onto its upstream tip.
/// Divergent histories return `VersionConflict` (use Merge UI instead).
pub fn pull(repo: &Repository, remote_name: &str) -> Result<()> {
    fetch(repo, remote_name)?;

    let head = repo.head().map_err(map_git_err)?;
    if !head.is_branch() {
        return Err(AppError::Protocol("cannot pull with detached HEAD".into()));
    }
    let local_name = head.shorthand().unwrap_or("HEAD").to_string();
    let local_branch = repo
        .find_branch(&local_name, BranchType::Local)
        .map_err(map_git_err)?;

    let upstream = match local_branch.upstream() {
        Ok(u) => u,
        Err(e) if e.code() == git2::ErrorCode::NotFound => {
            let remote_ref = format!("{remote_name}/{local_name}");
            match repo.find_branch(&remote_ref, BranchType::Remote) {
                Ok(b) => b,
                Err(_) => {
                    return Err(AppError::Protocol(
                        "no upstream configured; set upstream or push first".into(),
                    ));
                }
            }
        }
        Err(e) => return Err(map_git_err(e)),
    };

    let their_oid = upstream
        .get()
        .target()
        .ok_or_else(|| AppError::Protocol("upstream has no target".into()))?;
    let annotated = repo
        .find_annotated_commit(their_oid)
        .map_err(map_git_err)?;

    let (analysis, _) = repo.merge_analysis(&[&annotated]).map_err(map_git_err)?;

    if analysis.is_up_to_date() {
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
            "pull would require a merge; use Merge from Branches or rebase".into(),
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
        let err = fetch(&repo, "origin").expect_err("no origin");
        let _ = fs::remove_dir_all(&path);
        assert_eq!(err.category(), "Unknown");
    }
}
