//! Domain layer — pure business models and invariants.
//!
//! Zero external dependencies (no DB, no network, no IPC). Everything here
//! is `Serialize`/`Deserialize` so it can cross the Tauri IPC boundary.

pub mod error;
pub mod workspace;

pub use error::{AppError, Result};
pub use workspace::{PromptTemplates, RepoRef, Workspace, WorkspaceSettings, WorkspaceSummary};
