//! Shared worktree safety guards: dirty-state checks and path containment.
//!
//! Extracted from `revert.rs::ensure_clean` / `working_copy.rs` so every
//! destructive operation (merge, rebase, conflict resolve, staging) enforces
//! the same bar instead of each module rolling its own.

use std::path::Path;

use git2::{Repository, Status};

use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;

fn map_git_err(e: git2::Error) -> AppError {
    AppError::unknown_with(
        codes::git::GIT_ERROR,
        format!("git: {e}"),
        &[("error", e.to_string())],
    )
}

/// Refuse when the index or worktree carries any change (incl. untracked).
///
/// Destructive ops must call this before touching HEAD / index / worktree so
/// user staging work can never be silently mixed into the result.
pub fn ensure_clean(repo: &Repository) -> Result<()> {
    let mut opts = git2::StatusOptions::new();
    opts.include_untracked(true).recurse_untracked_dirs(true);
    let statuses = repo.statuses(Some(&mut opts)).map_err(map_git_err)?;
    if !statuses.is_empty() {
        return Err(AppError::protocol(
            codes::git::DIRTY_WORKTREE,
            "working copy is not clean — commit or stash your changes first",
        ));
    }
    Ok(())
}

/// Classify worktree dirt into staged / unstaged / untracked buckets
/// (ignored files excluded, same as [`ensure_clean`]).
pub fn classify_dirty(repo: &Repository) -> Result<(bool, bool, bool)> {
    const INDEX_BITS: Status = Status::INDEX_NEW
        .union(Status::INDEX_MODIFIED)
        .union(Status::INDEX_DELETED)
        .union(Status::INDEX_RENAMED)
        .union(Status::INDEX_TYPECHANGE);
    const WORKTREE_BITS: Status = Status::WT_MODIFIED
        .union(Status::WT_DELETED)
        .union(Status::WT_RENAMED)
        .union(Status::WT_TYPECHANGE)
        .union(Status::CONFLICTED);

    let mut opts = git2::StatusOptions::new();
    opts.include_untracked(true).recurse_untracked_dirs(true);
    let statuses = repo.statuses(Some(&mut opts)).map_err(map_git_err)?;
    let mut staged = false;
    let mut unstaged = false;
    let mut untracked = false;
    for entry in statuses.iter() {
        let st = entry.status();
        if st.contains(Status::WT_NEW) {
            untracked = true;
        }
        if st.intersects(INDEX_BITS) {
            staged = true;
        }
        if st.intersects(WORKTREE_BITS) {
            unstaged = true;
        }
    }
    Ok((staged, unstaged, untracked))
}

/// Reject anything that could leave the worktree: absolute paths replace the
/// base in `PathBuf::join`, and libgit2 treats `..` differently than the
/// filesystem does, so both layers are checked explicitly.
pub fn ensure_path_in_workdir(workdir: &Path, path: &str) -> Result<()> {
    reject_escaping_syntax(path)?;
    if !workdir.join(path).starts_with(workdir) {
        return Err(escapes_error(path));
    }
    Ok(())
}

/// Syntax-only half of [`ensure_path_in_workdir`] for call sites without a
/// guaranteed workdir (e.g. staging into a bare repo's index).
pub fn reject_escaping_syntax(path: &str) -> Result<()> {
    // Note: a leading `/` is rejected explicitly because Windows
    // `Path::is_absolute` does not treat `/abs/path` as absolute, while
    // repo-relative paths never legitimately start with one.
    if path.is_empty()
        || path.starts_with('/')
        || Path::new(path).is_absolute()
        || path.split(['/', '\\']).any(|seg| seg == "..")
    {
        return Err(escapes_error(path));
    }
    Ok(())
}

fn escapes_error(path: &str) -> AppError {
    AppError::protocol_with(
        codes::git::PATH_ESCAPES_WORKTREE,
        format!("path escapes worktree: {path}"),
        &[("path", path.to_string())],
    )
}
