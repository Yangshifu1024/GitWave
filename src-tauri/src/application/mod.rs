//! Application layer — orchestrates domain operations, enforces use cases.
//!
//! See `docs/tech/architecture/00-overview.md` for the layer responsibilities.

pub mod use_cases;

pub use use_cases::{
    abort_interactive_rebase_pause, abort_merge, add_local_repo, add_ssh_key, add_worktree,
    apply_stash, checkout_branch, cherry_pick_commit, clear_ai_api_key, clone_repo, commit,
    continue_interactive_rebase, create_branch, create_tag, create_workspace, delete_branch,
    delete_remote_branch, delete_ssh_key, delete_tag, delete_workspace, discard_changes,
    drop_stash, execute_interactive_rebase, explain_conflict, export_workspace, fetch,
    generate_commit_message, get_ahead_behind, get_ai_key_status, get_blame, get_branches,
    get_commit_details, get_commit_diff, get_commit_log, get_conflict_sides, get_file_diff,
    get_gitignore, get_repo_ai_rules, get_stash_diff, get_workdir_diff, get_working_copy,
    get_workspace, ignore_path, import_workspace, init_repo, init_submodule,
    interactive_rebase_paused, list_conflicts, list_repos, list_ssh_keys, list_stashes,
    list_submodules, list_tags, list_workspaces, list_worktrees, merge_branch, merge_in_progress,
    merge_preview, plan_interactive_rebase, pop_stash, probe_ollama, pull, push, rebase_branch,
    relink_repo, remove_repo, remove_worktree, rename_workspace, resolve_conflict, revert_commit,
    save_stash, set_active_repo, set_ai_api_key, stage_all, stage_files, test_ssh_connection,
    unstage_files, update_submodule, update_workspace_settings, write_gitignore, AheadBehind,
    AiGenerateOutcome, AiKeyStatus, AppContext,
};
