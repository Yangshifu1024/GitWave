//! libgit2 adapter for read and write git operations.
//!
//! git2-rs wraps libgit2; vendored build (no system dependency required).
//! Hooks are not auto-executed — by design, see
//! `docs/tech/tech-selection/00-overview.md` §Git 后端.

pub mod blame;
pub mod branch;
pub mod conflict;
pub mod credentials;
pub mod diff;
pub mod git2_adapter;
pub mod history;
pub mod merge;
pub mod rebase;
pub mod remote;
pub mod repo_adapter;
pub mod stash;
pub mod working_copy;
pub mod worktree;

#[cfg(test)]
mod test_helpers;
