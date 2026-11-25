//! Domain layer — pure business models and invariants.
//!
//! Zero external dependencies (no DB, no network, no IPC). Everything here
//! is `Serialize`/`Deserialize` so it can cross the Tauri IPC boundary.

pub mod app_settings;
pub mod blame;
pub mod branch;
pub mod diff;
pub mod error;
pub mod error_codes;
pub mod history;
pub mod hooks;
pub mod lfs;
pub mod reflog;
pub mod stash;
pub mod working_copy;
pub mod workspace;
pub mod worktree;

pub use app_settings::{ProxyMode, ProxySettings};
pub use blame::BlameLine;
pub use branch::{BranchInfo, BranchKind};
pub use diff::{DiffHunk, DiffLine, DiffLineKind, FileDiff};
pub use error::{AppError, Result};
pub use history::{
    CommitDetails, CommitRef, CommitRefKind, CommitSummary, FileStatus, FileSummary, PrCommit,
};
pub use hooks::HookInfo;
pub use lfs::LfsStatus;
pub use reflog::ReflogEntry;
pub use stash::StashEntry;
pub use working_copy::{FileChange, FileStatusKind, WorkingCopy};
pub use workspace::{
    PromptTemplates, RepoRef, RepoStatus, RepoSummary, Workspace, WorkspaceSettings,
    WorkspaceSummary,
};
pub use worktree::WorktreeInfo;
