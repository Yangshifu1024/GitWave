//! Application layer — orchestrates domain operations, enforces use cases.
//!
//! See `docs/tech/architecture/00-overview.md` for the layer responsibilities.

pub mod use_cases;

pub use use_cases::{
    add_local_repo, add_ssh_key, apply_stash, checkout_branch, clone_repo, commit, create_branch,
    create_workspace, delete_branch, delete_ssh_key, delete_workspace, drop_stash, fetch,
    get_ahead_behind, get_blame, get_branches, get_commit_diff, get_commit_log, get_file_diff,
    get_stash_diff, get_workdir_diff, get_working_copy, init_repo, list_repos, list_ssh_keys,
    list_stashes, list_workspaces, merge_branch, pop_stash, pull, push, rebase_branch, relink_repo,
    remove_repo, rename_workspace, save_stash, set_active_repo, stage_all, stage_files,
    test_ssh_connection, unstage_files, AheadBehind, AppContext,
};
