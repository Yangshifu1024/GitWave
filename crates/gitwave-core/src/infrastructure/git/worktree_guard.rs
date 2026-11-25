//! Shared worktree safety guards: dirty-state checks and path containment.
//!
//! Extracted from `revert.rs::ensure_clean` / `working_copy.rs` so every
//! destructive operation (merge, rebase, conflict resolve, staging) enforces
//! the same bar instead of each module rolling its own.
//!
//! Known limitations (accepted trade-offs for a local-first desktop app):
//! - TOCTOU: the status snapshot and the destructive step are separate
//!   calls; a concurrent writer can slip between them. The app assumes a
//!   single local user, so the window is acceptable — keep destructive ops
//!   immediately after the check.
//! - Symlinks: path checks are lexical. A symlink already inside the
//!   worktree pointing outside can make a contained-looking path write
//!   through to the target; treat hostile-repo symlinks as out of scope.

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
    opts.include_untracked(true)
        .recurse_untracked_dirs(true)
        // Pin the libgit2 default explicitly: submodule state (a drifted
        // submodule HEAD) must stay in the snapshot, or the guard would
        // wave through dirt a hard reset could then clobber.
        .exclude_submodules(false);
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
    opts.include_untracked(true)
        .recurse_untracked_dirs(true)
        .exclude_submodules(false);
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
    // Windows drive-relative syntax (`C:evil`): `Path::is_absolute` is false
    // for it, but `PathBuf::join` swaps the base to the drive — reject the
    // prefix at the syntax gate instead of relying on a downstream join
    // check (call sites without a workdir have none).
    let bytes = path.as_bytes();
    if bytes.len() >= 2 && bytes[0].is_ascii_alphabetic() && bytes[1] == b':' {
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
