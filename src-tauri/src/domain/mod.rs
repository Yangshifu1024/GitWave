//! Domain layer — pure business models and invariants.
//!
//! Zero external dependencies (no DB, no network, no IPC). Everything here
//! is `Serialize`/`Deserialize` so it can cross the Tauri IPC boundary.

pub mod blame;
pub mod branch;
pub mod diff;
pub mod error;
pub mod history;
pub mod workspace;

pub use blame::BlameLine;
pub use branch::{BranchInfo, BranchKind};
pub use diff::{DiffHunk, DiffLine, DiffLineKind, FileDiff};
pub use error::{AppError, Result};
pub use history::{
    CommitDetails, CommitRef, CommitRefKind, CommitSummary, FileStatus, FileSummary,
};
pub use workspace::{
    PromptTemplates, RepoRef, RepoStatus, RepoSummary, Workspace, WorkspaceSettings,
    WorkspaceSummary,
};
