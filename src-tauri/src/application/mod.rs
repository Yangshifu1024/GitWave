//! Application layer — orchestrates domain operations, enforces use cases.
//!
//! See `docs/tech/architecture/00-overview.md` for the layer responsibilities.
//! Use cases depend only on `domain` + `infrastructure` types; no Tauri or
//! framework imports.

pub mod use_cases;

pub use use_cases::{
    create_workspace, delete_workspace, list_workspaces, rename_workspace, set_active_repo,
    AppContext,
};
