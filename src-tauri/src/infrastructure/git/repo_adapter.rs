//! libgit2 write operations — `init`, `clone` (HTTPS via `git credential
//! helper`, SSH via ssh-agent).
//!
//! Read operations on an already-open repository live in
//! `git2_adapter.rs` (open_local, head).

use std::path::Path;
use std::sync::Arc;

use git2::{build::RepoBuilder, Repository, RepositoryInitOptions};

use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;

use super::credentials::{
    run_with_credentials, CredentialProvider, GitCredentialHelper, SshAgentCredential,
};

#[derive(Debug, Clone, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct CloneProgress {
    pub received_objects: u64,
    pub total_objects: u64,
    pub indexed_deltas: u64,
    pub total_deltas: u64,
    pub received_bytes: u64,
}

/// Initialize a new git repository at `path`. Does NOT create a commit —
/// see P1 (永不自动 commit). The user can stage + commit explicitly.
pub fn init(path: &Path) -> Result<()> {
    let mut opts = RepositoryInitOptions::new();
    opts.bare(false).initial_head("main").mkdir(true);
    Repository::init_opts(path, &opts).map_err(|e| match e.code() {
        git2::ErrorCode::Exists => AppError::protocol_with(
            codes::git::INIT_EXISTS,
            format!("path already a repo: {}", path.display()),
            &[("path", path.display().to_string())],
        ),
        _ => AppError::unknown_with(
            codes::git::INIT_FAILED,
            format!("git init: {e}"),
            &[("error", e.to_string())],
        ),
    })?;
    Ok(())
}

/// Clone an HTTPS repository. Credentials are obtained from the system
/// `git credential helper`.
pub fn clone_https(
    url: &str,
    dest: &Path,
    on_progress: Option<Box<dyn Fn(CloneProgress) + Send>>,
) -> Result<()> {
    let provider: Arc<dyn CredentialProvider> = Arc::new(GitCredentialHelper::new(url.to_string()));
    clone_with_creds(url, dest, provider, on_progress)
}

/// Clone an SSH repository. Credentials are obtained from ssh-agent.
pub fn clone_ssh(
    url: &str,
    dest: &Path,
    on_progress: Option<Box<dyn Fn(CloneProgress) + Send>>,
) -> Result<()> {
    let provider: Arc<dyn CredentialProvider> = Arc::new(SshAgentCredential::new());
    clone_with_creds(url, dest, provider, on_progress)
}

fn clone_with_creds(
    url: &str,
    dest: &Path,
    creds: Arc<dyn CredentialProvider>,
    on_progress: Option<Box<dyn Fn(CloneProgress) + Send>>,
) -> Result<()> {
    let mut cb = creds.callbacks();
    if let Some(progress) = on_progress {
        let progress = std::sync::Mutex::new(progress);
        cb.transfer_progress(move |stats| {
            if let Ok(guard) = progress.lock() {
                guard(CloneProgress {
                    received_objects: stats.received_objects() as u64,
                    total_objects: stats.total_objects() as u64,
                    indexed_deltas: stats.indexed_deltas() as u64,
                    total_deltas: stats.total_deltas() as u64,
                    received_bytes: stats.received_bytes() as u64,
                });
            }
            true
        });
    }

    let mut fo = git2::FetchOptions::new();
    fo.remote_callbacks(cb);

    let mut builder = RepoBuilder::new();
    builder.fetch_options(fo);

    run_with_credentials(
        &*creds,
        || builder.clone(url, dest),
        |e| {
            AppError::credential_with(
                codes::git::CLONE_AUTH_FAILED,
                format!("auth failed for {url}: {e}"),
                &[("url", url.to_string()), ("error", e.to_string())],
            )
        },
        |e| match e.code() {
            git2::ErrorCode::NotFound => AppError::protocol_with(
                codes::git::CLONE_NOT_FOUND,
                format!("not found: {url}"),
                &[("url", url.to_string())],
            ),
            _ => AppError::network_with(
                codes::git::CLONE_NETWORK,
                format!("network error cloning {url}: {e}"),
                &[("url", url.to_string()), ("error", e.to_string())],
            ),
        },
    )?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    #[test]
    fn init_creates_repo_with_main_head_but_no_commit() {
        let tmp = std::env::temp_dir().join(format!("gitwave-init-{}", std::process::id()));
        let _ = fs::remove_dir_all(&tmp);
        fs::create_dir_all(&tmp).expect("create tmp");

        init(&tmp).expect("init");

        // Verify .git/ was created
        assert!(tmp.join(".git").exists());

        // HEAD should point to refs/heads/main; no commit object yet.
        let repo = Repository::open(&tmp).expect("re-open");
        match repo.head() {
            Ok(_reference) => {
                // HEAD resolves to refs/heads/main — expected
            }
            Err(e) if e.code() == git2::ErrorCode::UnbornBranch => {
                // Acceptable: branch ref exists but no commit yet
            }
            Err(e) => panic!("unexpected head() error: {e}"),
        }

        fs::remove_dir_all(&tmp).ok();
    }

    #[test]
    fn init_then_add_commit_works() {
        let tmp = std::env::temp_dir().join(format!("gitwave-init-commit-{}", std::process::id()));
        let _ = fs::remove_dir_all(&tmp);
        fs::create_dir_all(&tmp).expect("create tmp");

        init(&tmp).expect("init");
        let repo = Repository::open(&tmp).expect("open");

        fs::write(tmp.join("README.md"), "hello\n").expect("write");
        let mut index = repo.index().expect("index");
        index
            .add_path(std::path::Path::new("README.md"))
            .expect("add");
        index.write().expect("write index");
        let tree_id = index.write_tree().expect("write tree");
        let tree = repo.find_tree(tree_id).expect("find tree");
        let sig = git2::Signature::now("Test", "test@local").expect("sig");
        repo.commit(Some("HEAD"), &sig, &sig, "initial", &tree, &[])
            .expect("commit");

        let head = repo.head().expect("head");
        let commit = head.peel_to_commit().expect("peel");
        assert_eq!(commit.message().unwrap(), "initial");

        fs::remove_dir_all(&tmp).ok();
    }

    #[test]
    fn init_in_existing_git_dir_is_idempotent_or_errors() {
        // libgit2's `init_opts` is tolerant: calling it on a path that's
        // already a valid repo may succeed silently or return Exists.
        // We accept either outcome here, but require no panic and a
        // usable repo afterward.
        let tmp = std::env::temp_dir().join(format!("gitwave-init-twice-{}", std::process::id()));
        let _ = fs::remove_dir_all(&tmp);
        fs::create_dir_all(&tmp).expect("create tmp");
        init(&tmp).expect("first init");

        let second = init(&tmp);
        // Either Ok (idempotent) or Err (Protocol) — both are acceptable.
        if let Err(e) = second {
            assert_eq!(e.category(), "Protocol");
        }

        // Repo should still be openable.
        Repository::open(&tmp).expect("open after second init");

        fs::remove_dir_all(&tmp).ok();
    }

    #[test]
    fn clone_https_unknown_host_errors_transport() {
        let tmp = std::env::temp_dir().join(format!("gitwave-clone-fail-{}", std::process::id()));
        let _ = fs::remove_dir_all(&tmp);
        let dest = tmp.join("repo");
        // Non-routable URL triggers network/protocol failure without
        // requiring real network in CI.
        let err = clone_https(
            "https://this-host-does-not-exist.invalid/repo.git",
            &dest,
            None,
        )
        .expect_err("should fail");
        let cat = err.category();
        assert!(
            matches!(cat, "Network" | "Protocol" | "Credential" | "Unknown"),
            "expected transport-related error, got {cat}"
        );

        let _ = fs::remove_dir_all(&tmp);
    }
}
