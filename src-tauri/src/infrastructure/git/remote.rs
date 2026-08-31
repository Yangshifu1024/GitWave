//! Fetch / push / pull against a named remote (default `origin`).

use std::sync::{Arc, Mutex};

use git2::{AutotagOption, BranchType, FetchOptions, PushOptions, Repository, StatusOptions};
use serde::Serialize;

use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;
use crate::infrastructure::git::credentials::{
    run_with_credentials, CredentialProvider, GitCredentialHelper, SshAgentCredential,
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
        git2::ErrorCode::Auth => AppError::credential_with(
            codes::git::AUTH_FAILED,
            format!("auth failed: {e}"),
            &[("error", e.to_string())],
        ),
        _ => AppError::unknown_with(
            codes::git::GIT_ERROR,
            format!("git: {e}"),
            &[("error", e.to_string())],
        ),
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
    remote.url().map(str::to_string).ok_or_else(|| {
        AppError::protocol_with(
            codes::git::REMOTE_NO_URL,
            format!("remote '{remote_name}' has no URL"),
            &[("name", remote_name.to_string())],
        )
    })
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

/// Fetch from `remote_name` (typically `origin`). Does not update the working
/// tree. Prunes the remote's own tracking refs (`refs/remotes/<name>/*`) that
/// no longer exist upstream; tags stay add-only (`AutotagOption::Auto` never
/// deletes — the shared `refs/tags/*` namespace makes per-fetch tag pruning
/// unsafe with multiple remotes, cf. git's opt-in `--prune-tags`).
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
    fo.prune(git2::FetchPrune::On);
    run_with_credentials(
        &*creds,
        || remote.fetch(&[] as &[&str], Some(&mut fo), None),
        |e| {
            AppError::credential_with(
                codes::git::FETCH_AUTH_FAILED,
                format!("fetch auth: {e}"),
                &[("error", e.to_string())],
            )
        },
        |e| {
            AppError::network_with(
                codes::git::FETCH_FAILED,
                format!("fetch failed: {e}"),
                &[("error", e.to_string())],
            )
        },
    )
}

/// Options controlling [`push_with_options`].
#[derive(Debug, Clone, Default)]
pub struct PushRequest {
    /// Push all local tags in addition to the current branch.
    pub tags: bool,
    /// Force-update the remote branch (leading `+` refspec).
    pub force: bool,
}

/// Push the current branch to `remote_name` under the same branch name.
pub fn push_with_options(
    repo: &Repository,
    remote_name: &str,
    opts: PushRequest,
    on_progress: Option<Box<dyn Fn(SyncProgress) + Send>>,
) -> Result<()> {
    let url = remote_url(repo, remote_name)?;
    let creds = provider_for_url(&url);
    let mut remote = repo.find_remote(remote_name).map_err(map_git_err)?;
    let head = repo.head().map_err(map_git_err)?;
    if !head.is_branch() {
        return Err(AppError::protocol(
            codes::git::PUSH_DETACHED_HEAD,
            "cannot push detached HEAD",
        ));
    }
    let branch = head.shorthand().unwrap_or("HEAD").to_string();
    let mut refspecs = vec![format!(
        "{}refs/heads/{branch}:refs/heads/{branch}",
        if opts.force { "+" } else { "" }
    )];
    if opts.tags {
        let tags = repo.references_glob("refs/tags/*").map_err(map_git_err)?;
        for tag in tags {
            let tag = tag.map_err(map_git_err)?;
            let Some(name) = tag.name() else { continue };
            let short = name.trim_start_matches("refs/tags/");
            refspecs.push(format!("refs/tags/{short}:refs/tags/{short}"));
        }
    }

    let mut po = PushOptions::new();
    let cb = attach_transfer_progress(creds.callbacks(), SyncOperation::Push, on_progress);
    po.remote_callbacks(cb);
    let str_refs: Vec<&str> = refspecs.iter().map(String::as_str).collect();
    run_with_credentials(
        &*creds,
        || remote.push(&str_refs, Some(&mut po)),
        |e| {
            AppError::credential_with(
                codes::git::PUSH_AUTH_FAILED,
                format!("push auth: {e}"),
                &[("error", e.to_string())],
            )
        },
        |e| {
            AppError::network_with(
                codes::git::PUSH_FAILED,
                format!("push failed: {e}"),
                &[("error", e.to_string())],
            )
        },
    )
}

/// Remote names configured on the repository.
pub fn list_remotes(repo: &Repository) -> Result<Vec<String>> {
    let remotes = repo.remotes().map_err(map_git_err)?;
    Ok(remotes.iter().flatten().map(str::to_string).collect())
}

/// One configured remote with its URLs (`git remote -v` equivalent).
#[derive(Debug, Clone, Serialize)]
pub struct RemoteInfo {
    pub name: String,
    pub fetch_url: Option<String>,
    pub push_url: Option<String>,
}

/// Configured remotes with URLs, in config order.
pub fn list_remote_details(repo: &Repository) -> Result<Vec<RemoteInfo>> {
    let names = repo.remotes().map_err(map_git_err)?;
    let mut out = Vec::new();
    for name in names.iter().flatten() {
        let remote = repo.find_remote(name).map_err(map_git_err)?;
        out.push(RemoteInfo {
            name: name.to_string(),
            fetch_url: remote.url().map(str::to_string),
            push_url: remote.pushurl().map(str::to_string),
        });
    }
    Ok(out)
}

fn validate_remote_name(name: &str) -> Result<()> {
    if name.trim().is_empty() {
        return Err(AppError::protocol(
            codes::git::REMOTE_NAME_EMPTY,
            "remote name cannot be empty",
        ));
    }
    Ok(())
}

fn validate_remote_url(url: &str) -> Result<()> {
    if url.trim().is_empty() {
        return Err(AppError::protocol(
            codes::git::REMOTE_URL_EMPTY,
            "remote URL cannot be empty",
        ));
    }
    Ok(())
}

fn remote_op_err(e: git2::Error) -> AppError {
    match e.code() {
        git2::ErrorCode::NotFound => AppError::protocol_with(
            codes::git::REMOTE_NOT_FOUND,
            format!("remote not found: {e}"),
            &[("error", e.to_string())],
        ),
        git2::ErrorCode::Exists => AppError::protocol_with(
            codes::git::REMOTE_EXISTS,
            format!("remote already exists: {e}"),
            &[("error", e.to_string())],
        ),
        _ => AppError::unknown_with(
            codes::git::REMOTE_OP_FAILED,
            format!("git remote: {e}"),
            &[("error", e.to_string())],
        ),
    }
}

/// Add a remote with a fetch URL (`git remote add`).
pub fn add_remote(repo: &Repository, name: &str, url: &str) -> Result<()> {
    validate_remote_name(name)?;
    validate_remote_url(url)?;
    if repo.find_remote(name.trim()).is_ok() {
        return Err(AppError::protocol_with(
            codes::git::REMOTE_DUPLICATE,
            format!("remote '{name}' already exists"),
            &[("name", name.to_string())],
        ));
    }
    repo.remote(name.trim(), url.trim())
        .map_err(remote_op_err)?;
    Ok(())
}

/// Update a remote's fetch URL (`git remote set-url`).
pub fn set_remote_url(repo: &Repository, name: &str, url: &str) -> Result<()> {
    validate_remote_name(name)?;
    validate_remote_url(url)?;
    repo.remote_set_url(name.trim(), url.trim())
        .map_err(remote_op_err)?;
    Ok(())
}

/// Update a remote's push URL (`git remote set-url --push`).
/// The push URL defaults to the fetch URL when unset.
pub fn set_remote_push_url(repo: &Repository, name: &str, url: Option<&str>) -> Result<()> {
    validate_remote_name(name)?;
    repo.remote_set_pushurl(name.trim(), url)
        .map_err(remote_op_err)?;
    Ok(())
}

/// Rename a remote (`git remote rename`).
pub fn rename_remote(repo: &Repository, name: &str, new_name: &str) -> Result<()> {
    validate_remote_name(name)?;
    validate_remote_name(new_name)?;
    repo.remote_rename(name.trim(), new_name.trim())
        .map_err(remote_op_err)?;
    Ok(())
}

/// Delete a remote (`git remote remove`).
pub fn remove_remote(repo: &Repository, name: &str) -> Result<()> {
    validate_remote_name(name)?;
    repo.remote_delete(name.trim()).map_err(remote_op_err)?;
    Ok(())
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
    run_with_credentials(
        &*creds,
        || remote.push(&[refspec.as_str()], Some(&mut po)),
        |e| {
            AppError::credential_with(
                codes::git::PUSH_AUTH_FAILED,
                format!("push auth: {e}"),
                &[("error", e.to_string())],
            )
        },
        |e| {
            AppError::network_with(
                codes::git::DELETE_REMOTE_BRANCH_FAILED,
                format!("delete remote branch failed: {e}"),
                &[("error", e.to_string())],
            )
        },
    )?;
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
pub(crate) fn worktree_is_dirty(repo: &Repository) -> Result<bool> {
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
        crate::infrastructure::git::stash::save_stash(repo, Some("pull: auto stash"), true)?;
        stashed = true;
    }

    match pull_integrate(repo, remote_name, &opts, on_progress) {
        Ok(()) => {
            if stashed {
                crate::infrastructure::git::stash::pop_stash(repo, 0).map_err(|e| {
                    AppError::unknown_with(
                        codes::git::STASH_REAPPLY_FAILED,
                        format!("pull completed; stash re-apply failed, the stash was kept: {e}"),
                        &[("error", e.to_string())],
                    )
                })?;
            }
            Ok(())
        }
        Err(e) => {
            if stashed {
                // Best-effort restore so a failed pull doesn't swallow changes.
                if crate::infrastructure::git::stash::pop_stash(repo, 0).is_err() {
                    return Err(AppError::unknown_with(
                        codes::git::STASH_RESTORE_FAILED,
                        format!("{e}; stash re-apply also failed, the stash was kept"),
                        &[("error", e.to_string())],
                    ));
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
        return Err(AppError::protocol(
            codes::git::PULL_DETACHED_HEAD,
            "cannot pull with detached HEAD",
        ));
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
        .map_err(|e| {
            AppError::protocol_with(
                codes::git::CANNOT_RESOLVE_REF,
                format!("cannot resolve '{target_ref}': {e}"),
                &[("ref", target_ref.clone()), ("error", e.to_string())],
            )
        })?;
    let annotated = repo.find_annotated_commit(their_oid).map_err(map_git_err)?;

    let (analysis, _) = repo.merge_analysis(&[&annotated]).map_err(map_git_err)?;

    if analysis.is_up_to_date() {
        return Ok(());
    }

    // Both landing paths (fast-forward and the rebase finalize) end in a
    // force checkout that would silently drop uncommitted changes in files
    // they touch — refuse like git does ("your local changes would be
    // overwritten"). The stash checkbox pre-cleans the worktree before we
    // get here, so this only fires when the user declined it.
    if !opts.stash && worktree_is_dirty(repo)? {
        return Err(AppError::protocol(
            codes::git::PULL_DIRTY_WORKTREE,
            "pull needs a clean worktree; check 'Stash and reapply' or commit first",
        ));
    }

    // Fast-forward before the rebase branch: `git pull --rebase`
    // fast-forwards too when local has no unique commits, and the previous
    // order let the rebase path swallow this case as a silent no-op.
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

    if opts.rebase {
        let result = crate::infrastructure::git::rebase::rebase_branch(repo, &target_ref)?;
        match result.kind {
            crate::infrastructure::git::rebase::RebaseKind::Conflicts => {
                return Err(AppError::version_conflict(
                    codes::git::PULL_REBASE_CONFLICTS,
                    "pull --rebase hit conflicts; local commits were left untouched",
                ));
            }
            crate::infrastructure::git::rebase::RebaseKind::AlreadyUpToDate => return Ok(()),
            crate::infrastructure::git::rebase::RebaseKind::FastForward => {
                // rebase_branch lands a strictly-behind result itself.
                // Unreachable in practice: the fast-forward block above
                // returns first, and this arm only exists for exhaustiveness.
                return Ok(());
            }
            crate::infrastructure::git::rebase::RebaseKind::Clean => {
                // In-memory rebase leaves refs and the workdir untouched;
                // land the rewritten head on the current branch here.
                let new_head = result.new_head.ok_or_else(|| {
                    AppError::protocol(
                        codes::git::REBASE_NO_NEW_HEAD,
                        "rebase finished without a new HEAD",
                    )
                })?;
                crate::infrastructure::git::rebase::finalize_rebase(repo, &new_head)?;
                return Ok(());
            }
        }
    }

    if analysis.is_normal() {
        return Err(AppError::version_conflict(
            codes::git::PULL_NEEDS_MERGE,
            "pull would require a merge; enable Rebase or use Merge from Branches",
        ));
    }

    Err(AppError::protocol(
        codes::git::PULL_UNEXPECTED,
        "pull: unexpected merge analysis",
    ))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn cleanup(path: &std::path::Path) {
        let _ = fs::remove_dir_all(path);
    }
    use crate::infrastructure::git::test_helpers::build_linear_repo;
    use std::fs;

    #[test]
    fn remote_crud_roundtrip() {
        let (path, repo) = crate::infrastructure::git::test_helpers::build_linear_repo(1);
        add_remote(&repo, "origin", "https://example.com/r.git").unwrap();
        assert_eq!(
            add_remote(&repo, "origin", "git@example.com:r2.git")
                .unwrap_err()
                .category(),
            "Protocol",
            "duplicate remote must be Protocol"
        );

        let details = list_remote_details(&repo).unwrap();
        assert_eq!(details.len(), 1);
        assert_eq!(details[0].name, "origin");
        assert_eq!(
            details[0].fetch_url.as_deref(),
            Some("https://example.com/r.git")
        );
        assert!(details[0].push_url.is_none());

        set_remote_url(&repo, "origin", "https://example.com/new.git").unwrap();
        set_remote_push_url(&repo, "origin", Some("git@example.com/push.git")).unwrap();
        let d = &list_remote_details(&repo).unwrap()[0];
        assert_eq!(d.fetch_url.as_deref(), Some("https://example.com/new.git"));
        assert_eq!(d.push_url.as_deref(), Some("git@example.com/push.git"));

        rename_remote(&repo, "origin", "origin2").unwrap();
        assert!(list_remote_details(&repo).unwrap()[0].name == "origin2");

        remove_remote(&repo, "origin2").unwrap();
        assert!(list_remote_details(&repo).unwrap().is_empty());
        cleanup(&path);
    }

    #[test]
    fn remote_ops_validate_and_error_protocol() {
        let (path, repo) = crate::infrastructure::git::test_helpers::build_linear_repo(1);
        assert_eq!(
            add_remote(&repo, " ", "https://x.git")
                .unwrap_err()
                .category(),
            "Protocol"
        );
        assert_eq!(
            add_remote(&repo, "r", " ").unwrap_err().category(),
            "Protocol"
        );
        assert_eq!(
            remove_remote(&repo, "nope").unwrap_err().category(),
            "Protocol"
        );
        assert_eq!(
            rename_remote(&repo, "nope", "x").unwrap_err().category(),
            "Protocol"
        );
        cleanup(&path);
    }

    #[test]
    fn fetch_missing_remote_errors() {
        let (path, repo) = build_linear_repo(1);
        let err = fetch(&repo, "origin", SyncOperation::Fetch, None).expect_err("no origin");
        let _ = fs::remove_dir_all(&path);
        assert_eq!(err.category(), "Unknown");
    }

    #[test]
    fn fetch_prunes_tracking_refs_deleted_on_remote() {
        let (server_path, local_path, server, local) = cloned_from_server();
        // A `feature` branch appears on the server and is fetched, so local
        // grows a refs/remotes/origin/feature tracking ref...
        server
            .reference(
                "refs/heads/feature",
                head_oid(&server),
                true,
                "create feature",
            )
            .unwrap();
        fetch(&local, "origin", SyncOperation::Fetch, None).unwrap();
        assert!(local.find_reference("refs/remotes/origin/feature").is_ok());

        // ...then it is deleted upstream: the next fetch must prune the
        // stale tracking ref. Namespaced refs are unaffected.
        server
            .find_reference("refs/heads/feature")
            .unwrap()
            .delete()
            .unwrap();
        fetch(&local, "origin", SyncOperation::Fetch, None).unwrap();
        assert!(
            local.find_reference("refs/remotes/origin/feature").is_err(),
            "stale tracking ref must be pruned"
        );
        assert!(local.find_reference("refs/remotes/origin/main").is_ok());

        let _ = fs::remove_dir_all(&server_path);
        let _ = fs::remove_dir_all(&local_path);
    }

    // ─── pull --rebase regressions ───────────────────────────────────────
    // The rebase branch of `pull_integrate` used to drop the in-memory
    // rebase's `new_head` and return Ok, reporting success while the
    // branch never moved.

    use crate::infrastructure::git::test_helpers::{make_commit, write_and_stage};
    use git2::Signature;

    /// Server repo at commit 0 plus a clone of it (`origin` → server path,
    /// local branch `main`). Caller removes both paths.
    fn cloned_from_server() -> (
        std::path::PathBuf,
        std::path::PathBuf,
        Repository,
        Repository,
    ) {
        let (server_path, server) = build_linear_repo(1);
        let local_path = server_path.with_extension("clone");
        let local = Repository::clone(server_path.to_str().unwrap(), &local_path).unwrap();
        // Clone doesn't copy the server's repo config, so on Windows runners
        // the global `core.autocrlf=true` would make checkouts write CRLF
        // and break the exact-content assertions below (same reason
        // test_helpers::configure_user pins it on fresh repos).
        local
            .config()
            .unwrap()
            .set_str("core.autocrlf", "false")
            .unwrap();
        // The clone's own checkout already ran under the global config and
        // left CRLF files behind — re-checkout with the pin in effect so the
        // worktree is LF and `worktree_is_dirty` sees it as clean.
        local
            .checkout_head(Some(git2::build::CheckoutBuilder::default().force()))
            .unwrap();
        (server_path, local_path, server, local)
    }

    fn sig() -> Signature<'static> {
        Signature::now("Test", "test@local").unwrap()
    }

    fn head_oid(repo: &Repository) -> git2::Oid {
        repo.head().unwrap().peel_to_commit().unwrap().id()
    }

    /// One commit on each side of the clone: local adds `local.txt`,
    /// server adds `file1.txt`.
    fn diverge(server: &Repository, local: &Repository) -> git2::Oid {
        let local_tree = write_and_stage(local, "local.txt", "local\n");
        let _local_tip = make_commit(
            local,
            &sig(),
            "local commit",
            local_tree,
            &[head_oid(local)],
        );
        let server_tree = write_and_stage(server, "file1.txt", "v1\n");
        make_commit(server, &sig(), "commit 1", server_tree, &[head_oid(server)])
    }

    fn rebase_opts(stash: bool) -> PullOptions {
        PullOptions {
            branch: Some("main".into()),
            rebase: true,
            stash,
        }
    }

    #[test]
    fn pull_rebase_fast_forwards_when_local_is_behind() {
        let (server_path, local_path, server, mut local) = cloned_from_server();
        let tree = write_and_stage(&server, "file1.txt", "v1\n");
        let server_tip = make_commit(&server, &sig(), "commit 1", tree, &[head_oid(&server)]);

        pull_with_options(&mut local, "origin", rebase_opts(false), None).unwrap();

        assert_eq!(
            head_oid(&local),
            server_tip,
            "rebase pull must fast-forward"
        );
        assert_eq!(
            fs::read_to_string(local_path.join("file1.txt")).unwrap(),
            "v1\n"
        );
        let _ = fs::remove_dir_all(&server_path);
        let _ = fs::remove_dir_all(&local_path);
    }

    #[test]
    fn pull_rebase_rewrites_diverged_local_commits() {
        let (server_path, local_path, server, mut local) = cloned_from_server();
        let _local_tip = head_oid(&local);
        let server_tip = diverge(&server, &local);

        pull_with_options(&mut local, "origin", rebase_opts(false), None).unwrap();

        let head = local.head().unwrap().peel_to_commit().unwrap();
        assert_eq!(
            head.parent(0).unwrap().id(),
            server_tip,
            "the local commit must sit on the server tip"
        );
        assert_ne!(head.id(), _local_tip, "the local commit must be rewritten");
        assert_eq!(
            fs::read_to_string(local_path.join("local.txt")).unwrap(),
            "local\n"
        );
        assert_eq!(
            fs::read_to_string(local_path.join("file1.txt")).unwrap(),
            "v1\n"
        );
        let _ = fs::remove_dir_all(&server_path);
        let _ = fs::remove_dir_all(&local_path);
    }

    #[test]
    fn pull_rebase_refuses_dirty_worktree() {
        let (server_path, local_path, server, mut local) = cloned_from_server();
        diverge(&server, &local);
        fs::write(local.workdir().unwrap().join("local.txt"), "dirty\n").unwrap();
        let before = head_oid(&local);

        let err = pull_with_options(&mut local, "origin", rebase_opts(false), None)
            .expect_err("dirty worktree must refuse");

        assert_eq!(err.category(), "Protocol");
        assert_eq!(head_oid(&local), before, "HEAD must not move");
        assert_eq!(
            fs::read_to_string(local_path.join("local.txt")).unwrap(),
            "dirty\n"
        );
        let _ = fs::remove_dir_all(&server_path);
        let _ = fs::remove_dir_all(&local_path);
    }

    #[test]
    fn pull_rebase_refuses_dirty_worktree_even_when_fast_forwardable() {
        // Regression for the guard order: a fast-forwardable pull with
        // rebase checked must refuse before its force checkout, not clobber
        // the dirty file.
        let (server_path, local_path, server, mut local) = cloned_from_server();
        let tree = write_and_stage(&server, "file1.txt", "v1\n");
        make_commit(&server, &sig(), "commit 1", tree, &[head_oid(&server)]);
        fs::write(local.workdir().unwrap().join("file0.txt"), "dirty\n").unwrap();
        let before = head_oid(&local);

        let err = pull_with_options(&mut local, "origin", rebase_opts(false), None)
            .expect_err("dirty worktree must refuse even when fast-forwardable");

        assert_eq!(err.category(), "Protocol");
        assert_eq!(head_oid(&local), before, "HEAD must not move");
        assert_eq!(
            fs::read_to_string(local_path.join("file0.txt")).unwrap(),
            "dirty\n"
        );
        let _ = fs::remove_dir_all(&server_path);
        let _ = fs::remove_dir_all(&local_path);
    }

    #[test]
    fn pull_fast_forward_refuses_dirty_worktree() {
        // Plain pull shares the guard: a fast-forward's force checkout must
        // not drop uncommitted changes either.
        let (server_path, local_path, server, mut local) = cloned_from_server();
        let tree = write_and_stage(&server, "file1.txt", "v1\n");
        make_commit(&server, &sig(), "commit 1", tree, &[head_oid(&server)]);
        fs::write(local.workdir().unwrap().join("file0.txt"), "dirty\n").unwrap();
        let before = head_oid(&local);

        let opts = PullOptions {
            branch: Some("main".into()),
            rebase: false,
            stash: false,
        };
        let err = pull_with_options(&mut local, "origin", opts, None)
            .expect_err("dirty worktree must refuse plain pull too");

        assert_eq!(err.category(), "Protocol");
        assert_eq!(head_oid(&local), before, "HEAD must not move");
        assert_eq!(
            fs::read_to_string(local_path.join("file0.txt")).unwrap(),
            "dirty\n"
        );
        let _ = fs::remove_dir_all(&server_path);
        let _ = fs::remove_dir_all(&local_path);
    }

    #[test]
    fn pull_rebase_with_stash_keeps_local_changes() {
        let (server_path, local_path, server, mut local) = cloned_from_server();
        let server_tip = diverge(&server, &local);
        fs::write(local.workdir().unwrap().join("local.txt"), "dirty\n").unwrap();

        pull_with_options(&mut local, "origin", rebase_opts(true), None).unwrap();

        assert_eq!(
            local
                .head()
                .unwrap()
                .peel_to_commit()
                .unwrap()
                .parent(0)
                .unwrap()
                .id(),
            server_tip
        );
        assert_eq!(
            fs::read_to_string(local_path.join("local.txt")).unwrap(),
            "dirty\n",
            "stashed change must be reapplied"
        );
        assert_eq!(
            fs::read_to_string(local_path.join("file1.txt")).unwrap(),
            "v1\n"
        );
        let _ = fs::remove_dir_all(&server_path);
        let _ = fs::remove_dir_all(&local_path);
    }
}
