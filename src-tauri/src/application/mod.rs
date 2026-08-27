//! Application layer — orchestrates domain operations, enforces use cases.
//!
//! See `docs/tech/architecture/00-overview.md` for the layer responsibilities.

pub mod use_cases;

pub use use_cases::{
    abort_interactive_rebase_pause, abort_merge, add_local_repo, add_ssh_key, add_worktree,
    apply_stash, checkout_branch, clear_ai_api_key, clone_repo, commit,
    continue_interactive_rebase, create_branch, create_workspace, delete_branch, delete_ssh_key,
    delete_workspace, discard_changes, drop_stash, execute_interactive_rebase, explain_conflict,
    fetch, generate_commit_message, get_ahead_behind, get_ai_key_status, get_blame, get_branches,
    get_commit_diff, get_commit_log, get_conflict_sides, get_file_diff, get_stash_diff,
    get_workdir_diff, get_working_copy, get_workspace, ignore_path, init_repo,
    interactive_rebase_paused, list_conflicts, list_repos, list_ssh_keys, list_stashes,
    list_workspaces, list_worktrees, merge_branch, merge_in_progress, plan_interactive_rebase,
    pop_stash, probe_ollama, pull, push, rebase_branch, relink_repo, remove_repo, remove_worktree,
    rename_workspace, resolve_conflict, save_stash, set_active_repo, set_ai_api_key, stage_all,
    stage_files, test_ssh_connection, unstage_files, update_workspace_settings, AheadBehind,
    AiKeyStatus, AppContext,
};
