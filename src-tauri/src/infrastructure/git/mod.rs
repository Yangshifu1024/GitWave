//! libgit2 adapter for read-heavy git operations.
//!
//! git2-rs wraps libgit2; vendored build (no system dependency required).
//! Hooks are not auto-executed — by design, see
//! `docs/tech/tech-selection/00-overview.md` §Git 后端.

pub mod git2_adapter;
