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
pub mod health;
pub mod history;
pub mod interactive_rebase;
pub mod merge;
pub mod rebase;
pub mod reflog;
pub mod remote;
pub mod repo_adapter;
pub mod revert;
pub mod stash;
pub mod submodule;
pub mod tag;
pub mod working_copy;
pub mod worktree;

#[cfg(test)]
mod test_helpers;
