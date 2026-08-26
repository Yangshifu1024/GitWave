//! SQLite-backed persistence.
//!
//! See `docs/tech/tech-selection/00-overview.md` §本地存储 for the full
//! table schema.

pub mod migrations;
pub mod sqlite;
pub mod workspace_repo;

pub use sqlite::{open, state_dir};
pub use workspace_repo::{SqliteWorkspaceRepo, WorkspaceRepository};
