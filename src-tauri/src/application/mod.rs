//! Application layer — orchestrates domain operations, enforces use cases.
//!
//! See `docs/tech/architecture/00-overview.md` for the layer responsibilities.
//! Use cases depend only on `domain` + `infrastructure` types; no Tauri or
//! framework imports.

pub mod use_cases;

pub use use_cases::{
    add_local_repo, add_ssh_key, clone_repo, create_workspace, delete_ssh_key, delete_workspace,
    init_repo, list_repos, list_ssh_keys, list_workspaces, relink_repo, remove_repo,
    rename_workspace, set_active_repo, test_ssh_connection, AppContext,
};
