//! Submodule management (S1 / roadmap v0.1): list, init, update.

use git2::Repository;
use serde::Serialize;

use crate::domain::error::{AppError, Result};

fn map_git_err(e: git2::Error) -> AppError {
    AppError::Unknown(format!("git: {e}"))
}

/// One submodule of the active repository.
#[derive(Debug, Clone, Serialize)]
pub struct SubmoduleInfo {
    pub name: String,
    /// Path relative to the repo root.
    pub path: String,
    pub url: Option<String>,
    /// Whether the submodule is registered in `.git/config` (init'ed).
    pub initialized: bool,
    /// Checked-out HEAD sha, `None` when the submodule worktree is absent.
    pub head_sha: Option<String>,
}

/// All `.gitmodules` entries with their state.
pub fn list_submodules(repo: &Repository) -> Result<Vec<SubmoduleInfo>> {
    let mut out = Vec::new();
    for mut sm in repo.submodules().map_err(map_git_err)? {
        let initialized = sm.init(false).is_ok() && sm.url().is_some();
        let head_sha = sm
            .open()
            .ok()
            .and_then(|sub_repo| sub_repo.head().ok()?.target().map(|o| o.to_string()));
        out.push(SubmoduleInfo {
            name: sm.name().unwrap_or("").to_string(),
            path: sm.path().to_string_lossy().into_owned(),
            url: sm.url().map(str::to_string),
            initialized,
            head_sha,
        });
    }
    Ok(out)
}

fn find<'repo>(repo: &'repo Repository, name: &str) -> Result<git2::Submodule<'repo>> {
    repo.find_submodule(name)
        .map_err(|_| AppError::Protocol(format!("submodule not found: {name}")))
}

/// Register a submodule in `.git/config` (`git submodule init`).
pub fn init_submodule(repo: &Repository, name: &str) -> Result<()> {
    find(repo, name)?.init(true).map_err(map_git_err)?;
    Ok(())
}

/// Clone / fetch / checkout the submodule worktree
/// (`git submodule update --init`; fetch allowed like git's default).
pub fn update_submodule(repo: &Repository, name: &str) -> Result<()> {
    let mut sm = find(repo, name)?;
    let mut opts = git2::SubmoduleUpdateOptions::new();
    let mut checkout = git2::build::CheckoutBuilder::new();
    checkout.allow_conflicts(true);
    opts.checkout(checkout).allow_fetch(true);
    sm.update(true, Some(&mut opts)).map_err(map_git_err)?;
    Ok(())
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
    fn no_submodules_yields_empty_list() {
        let (path, repo) = build_linear_repo(1);
        let subs = list_submodules(&repo).unwrap();
        assert!(subs.is_empty());
        cleanup(&path);
    }

    #[test]
    fn init_and_update_missing_submodule_errors() {
        let (path, repo) = build_linear_repo(1);
        let err = init_submodule(&repo, "nope").unwrap_err();
        assert_eq!(err.category(), "Protocol");
        let err = update_submodule(&repo, "nope").unwrap_err();
        assert_eq!(err.category(), "Protocol");
        cleanup(&path);
    }
}
