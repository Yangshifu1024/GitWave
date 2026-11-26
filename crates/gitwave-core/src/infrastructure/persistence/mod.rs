//! SQLite-backed persistence.
//!
//! See `docs/tech/tech-selection/00-overview.md` §本地存储 for the full
//! table schema.

pub mod app_settings_repo;
pub mod migrations;
pub mod snapshot;
pub mod sqlite;
pub mod workspace_repo;

pub use app_settings_repo::{AppSettingsRepository, SqliteAppSettingsRepo};
pub use snapshot::{refresh, taken_at, Refresh, Stamp};
pub use sqlite::{open, open_in, state_dir, state_dir_path, STATE_DB_FILE};
pub use workspace_repo::{SqliteWorkspaceRepo, WorkspaceRepository};
