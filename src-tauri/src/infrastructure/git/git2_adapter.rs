//! libgit2 adapter — open local repository, read HEAD, build signatures.
//!
//! Used by Sprint 1's Workspace CRUD to verify that a repo path is a valid
//! git working tree before adding it.

use std::path::Path;

use git2::{Repository, Signature};

use crate::domain::error::{AppError, Result};

/// Open a local repository at `path`.
pub fn open_local(path: &Path) -> Result<Repository> {
    Repository::open(path).map_err(|e| match e.code() {
        git2::ErrorCode::NotFound => {
            AppError::Protocol(format!("not a git repository: {}", path.display()))
        }
        _ => AppError::Unknown(format!("git open {}: {e}", path.display())),
    })
}

/// A lightweight read of HEAD commit — `(sha, summary)`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct HeadSummary {
    pub sha: String,
    pub summary: String,
}

/// Read HEAD commit summary from an open repository.
pub fn head(repo: &Repository) -> Result<HeadSummary> {
    let head = repo.head().map_err(map_git_err)?;
    let commit = head.peel_to_commit().map_err(map_git_err)?;
    Ok(HeadSummary {
        sha: commit.id().to_string(),
        summary: commit.summary().unwrap_or("").to_string(),
    })
}

/// Build a default signature for environments without git config set.
pub fn default_signature() -> Result<Signature<'static>> {
    Signature::now("GitWave", "noreply@gitwave.local").map_err(map_git_err)
}

fn map_git_err(e: git2::Error) -> AppError {
    AppError::Unknown(format!("git: {e}"))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    #[test]
    fn open_local_repo_and_read_head() {
        let tmp = std::env::temp_dir().join(format!("gitwave-git2-test-{}", std::process::id()));
        let _ = fs::remove_dir_all(&tmp);
        fs::create_dir_all(&tmp).expect("create tmp dir");

        // Init repo via libgit2 directly (no subprocess).
        let repo = Repository::init(&tmp).expect("init repo");
        let sig = default_signature().expect("signature");

        std::fs::write(tmp.join("README.md"), "hello\n").expect("write readme");
        let mut index = repo.index().expect("index");
        index
            .add_path(std::path::Path::new("README.md"))
            .expect("add path");
        index.write().expect("write index");
        let tree_id = index.write_tree().expect("write tree");
        let tree = repo.find_tree(tree_id).expect("find tree");
        repo.commit(Some("HEAD"), &sig, &sig, "initial commit", &tree, &[])
            .expect("commit");

        let repo = open_local(&tmp).expect("open local");
        let h = head(&repo).expect("head");
        assert_eq!(h.summary, "initial commit");
        assert_eq!(h.sha.len(), 40, "git sha is 40 hex chars");

        fs::remove_dir_all(&tmp).ok();
    }

    #[test]
    fn open_non_repo_errors_protocol() {
        let tmp = std::env::temp_dir().join(format!("gitwave-git2-nonrepo-{}", std::process::id()));
        let _ = fs::remove_dir_all(&tmp);
        fs::create_dir_all(&tmp).expect("create tmp dir");
        // No init — directory exists but isn't a git repo.

        let err = open_local(&tmp).err().expect("should fail");
        assert_eq!(err.category(), "Protocol");
    }
}
