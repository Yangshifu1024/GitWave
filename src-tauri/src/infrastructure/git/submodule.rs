//! Submodule management (S1 / roadmap v0.1; full support in v0.2):
//! list, init, update (recursive), add, deinit.

use git2::Repository;
use serde::Serialize;
use std::path::Path;

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
    /// Whether the checked-out HEAD matches the sha recorded in the parent
    /// index (the gitlink) — false means the submodule needs an update.
    pub in_sync: bool,
}

/// All `.gitmodules` entries with their state. Read-only — the previous
/// `sm.init(false)` probe silently re-registered deinit'ed submodules.
pub fn list_submodules(repo: &Repository) -> Result<Vec<SubmoduleInfo>> {
    let mut out = Vec::new();
    for sm in repo.submodules().map_err(map_git_err)? {
        let name = sm.name().unwrap_or("").to_string();
        // Initialized = the repo's local config carries the submodule's
        // settings (`git submodule init` / `add` writes them there).
        let initialized = {
            let cfg = repo.config().map_err(map_git_err)?;
            cfg.get_string(&format!("submodule.{name}.url")).is_ok()
        };
        let head_sha = sm
            .open()
            .ok()
            .and_then(|sub_repo| sub_repo.head().ok()?.target().map(|o| o.to_string()));
        // The gitlink recorded in the parent index is what a checkout of
        // the parent expects the submodule HEAD to be.
        let index_sha = repo.index().ok().and_then(|index| {
            index
                .get_path(Path::new(sm.path()), 0)
                .map(|entry| entry.id.to_string())
        });
        let in_sync = match (&head_sha, index_sha) {
            (Some(head), Some(recorded)) => *head == recorded,
            _ => false,
        };
        out.push(SubmoduleInfo {
            name,
            path: sm.path().to_string_lossy().into_owned(),
            url: sm.url().map(str::to_string),
            initialized,
            head_sha,
            in_sync,
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
/// (`git submodule update --init`); with `recursive`, repeat for every
/// nested submodule of the freshly checked-out worktree.
pub fn update_submodule(repo: &Repository, name: &str, recursive: bool) -> Result<()> {
    let mut sm = find(repo, name)?;
    let mut opts = git2::SubmoduleUpdateOptions::new();
    let mut checkout = git2::build::CheckoutBuilder::new();
    checkout.allow_conflicts(true);
    opts.checkout(checkout).allow_fetch(true);
    sm.update(true, Some(&mut opts)).map_err(map_git_err)?;
    if recursive {
        // Open the submodule's worktree and update whatever it declares.
        let sub_repo = sm.open().map_err(map_git_err)?;
        let nested: Vec<String> = sub_repo
            .submodules()
            .map_err(map_git_err)?
            .iter_mut()
            .map(|s| s.name().unwrap_or("").to_string())
            .collect();
        for nested_name in nested {
            update_submodule(&sub_repo, &nested_name, true)?;
        }
    }
    Ok(())
}

/// Add a submodule at `path` cloning from `url`, staging the gitlink and
/// `.gitmodules` entry (`git submodule add` semantics). Changes are
/// staged, NOT committed — the user commits them through the normal flow.
pub fn add_submodule(repo: &Repository, url: &str, path: &str) -> Result<()> {
    if url.trim().is_empty() || path.trim().is_empty() {
        return Err(AppError::Protocol(
            "submodule URL and path are required".into(),
        ));
    }
    if path.contains("..") || Path::new(path).is_absolute() {
        return Err(AppError::Protocol(
            "submodule path must be a relative path inside the repository".into(),
        ));
    }
    // git2 splits `git submodule add` into setup → clone → finalize.
    let mut sm = repo
        .submodule(url, Path::new(path), true)
        .map_err(|e| AppError::Unknown(format!("submodule add: {e}")))?;
    let mut opts = git2::SubmoduleUpdateOptions::new();
    let mut checkout = git2::build::CheckoutBuilder::new();
    checkout.allow_conflicts(true);
    opts.checkout(checkout);
    sm.clone(Some(&mut opts))
        .map_err(|e| AppError::Unknown(format!("submodule clone: {e}")))?;
    sm.add_to_index(true).map_err(map_git_err)?;
    sm.add_finalize().map_err(map_git_err)?;
    Ok(())
}

/// Unregister a submodule from `.git/config` (`git submodule deinit`).
/// Milder than git's version: the submodule worktree is left untouched
/// (nothing is deleted); the entry stays in `.gitmodules` and the index.
pub fn deinit_submodule(repo: &Repository, name: &str) -> Result<()> {
    // Validate the submodule exists (name or path lookup).
    find(repo, name)?;
    // Remove through the repo's live config so its cache stays consistent —
    // editing the file behind the repo's back is invisible to lookups.
    let mut config = repo.config().map_err(map_git_err)?;
    // Collect keys in a scoped block: entries borrows config until dropped.
    let mut keys: Vec<String> = Vec::new();
    {
        let mut entries = config
            .entries(Some(&format!("submodule\\.{name}\\.")))
            .map_err(map_git_err)?;
        // ConfigEntries has a custom `next()` rather than the Iterator trait.
        while let Some(entry) = entries.next() {
            if let Ok(entry) = entry {
                if let Some(key) = entry.name() {
                    keys.push(key.to_string());
                }
            }
        }
    }
    if keys.is_empty() {
        return Err(AppError::Protocol(format!(
            "submodule not initialized: {name}"
        )));
    }
    for key in keys {
        config.remove(&key).map_err(map_git_err)?;
    }
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
        let err = update_submodule(&repo, "nope", false).unwrap_err();
        assert_eq!(err.category(), "Protocol");
        cleanup(&path);
    }

    #[test]
    fn add_then_deinit_roundtrip() {
        let (child_path, _child) = build_linear_repo(1);
        let (parent_path, repo) = build_linear_repo(1);
        let url = child_path.to_str().expect("utf8 temp path");

        add_submodule(&repo, url, "libs/child").expect("add");
        let subs = list_submodules(&repo).expect("list");
        assert_eq!(subs.len(), 1, "one submodule after add: {subs:?}");
        assert_eq!(subs[0].path, "libs/child");
        assert!(subs[0].initialized, "submodule_add registers and inits");
        assert!(subs[0].head_sha.is_some(), "worktree was cloned");

        deinit_submodule(&repo, "libs/child").expect("deinit");
        let subs = list_submodules(&repo).expect("list after deinit");
        assert!(!subs[0].initialized, "deinit unregisters from .git/config");
        // Config-only deinit leaves the worktree in place, so the checkout
        // stays in sync with the index — nothing is deleted.
        assert!(subs[0].in_sync);

        // Bad paths are rejected before touching git.
        assert!(add_submodule(&repo, url, "../escape").is_err());
        assert!(add_submodule(&repo, url, "").is_err());

        cleanup(&parent_path);
        cleanup(&child_path);
    }
}
