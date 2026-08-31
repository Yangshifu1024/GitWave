//! GitWave library crate entry point.
//!
//! Architecture follows `docs/tech/architecture/00-overview.md`:
//! presentation (WebView) → IPC command → application → domain ← infrastructure.
//!
//! Concrete use cases live in `application::use_cases`. The Tauri commands
//! below are thin wrappers that resolve state and forward to use cases.

#![allow(dead_code)]
#![allow(unused_imports)]

pub mod application;
pub mod domain;
pub mod infrastructure;

use std::sync::{Arc, Mutex};

use crate::domain::error_codes as codes;
use application::{
    abort_interactive_rebase_pause, abort_merge, add_local_repo, add_remote, add_ssh_key,
    add_submodule, add_worktree, ai_palette_intent, apply_stash, checkout_branch,
    cherry_pick_commit, clear_ai_api_key, clone_repo, commit, continue_interactive_rebase,
    create_branch, create_tag, create_workspace, deinit_submodule, delete_branch,
    delete_remote_branch, delete_ssh_key, delete_tag, delete_workspace, discard_changes,
    drop_stash, execute_interactive_rebase, explain_commit, explain_conflict, explain_health,
    explain_reflog, export_workspace, fetch, generate_commit_message, generate_pr_description,
    get_ahead_behind, get_ai_key_status, get_blame, get_branches, get_commit_details,
    get_commit_diff, get_commit_log, get_conflict_sides, get_file_diff, get_gitignore, get_health,
    get_hook, get_repo_ai_rules, get_stash_diff, get_workdir_diff, get_working_copy, get_workspace,
    ignore_path, import_workspace, init_repo, init_submodule, interactive_rebase_paused,
    lfs_install, lfs_status, lfs_track, lfs_untrack, list_conflicts, list_hooks, list_reflog,
    list_remote_details, list_repos, list_ssh_keys, list_stashes, list_submodules, list_tags,
    list_workspaces, list_worktrees, merge_branch, merge_in_progress, merge_preview,
    plan_interactive_rebase, pop_stash, probe_ollama, pull, push, rebase_branch, relink_repo,
    remove_remote, remove_repo, remove_worktree, rename_remote, rename_workspace, reorder_repos,
    reset_hard, resolve_conflict, revert_commit, save_hook, save_stash, set_active_repo,
    set_ai_api_key, set_remote_push_url, set_remote_url, stage_all, stage_files,
    test_ssh_connection, unstage_files, update_submodule, update_workspace_settings,
    write_gitignore, AheadBehind, AiGenerateOutcome, AiKeyStatus, AppContext, PaletteIntent,
    PrDescriptionOutcome,
};
use domain::blame::BlameLine;
use domain::branch::BranchInfo;
use domain::diff::FileDiff;
use domain::error::AppError;
use domain::history::{CommitDetails, CommitSummary};
use domain::hooks::HookInfo;
use domain::lfs::LfsStatus;
use domain::reflog::ReflogEntry;
use domain::stash::StashEntry;
use domain::working_copy::WorkingCopy;
use domain::workspace::{RepoRef, Workspace, WorkspaceSettings, WorkspaceSummary};
use domain::worktree::WorktreeInfo;
use infrastructure::git::conflict::{ConflictFile, ConflictSides};
use infrastructure::git::diff::DiffSummary;
use infrastructure::git::interactive_rebase::{InteractiveRebaseResult, InteractiveRebaseTodo};
use infrastructure::git::merge::{MergePreview, MergeResult};
use infrastructure::git::rebase::RebaseResult;
use infrastructure::observability::tracing::init as init_tracing;
use infrastructure::persistence::{migrations, open as open_state, state_dir, SqliteWorkspaceRepo};
use infrastructure::ssh::keys::{SshKey, SshTestResult};
use std::fmt::Display;
use tauri::WebviewWindow;
mod macos_window;

use tauri_plugin_decoration::WebviewWindowExt;
use tauri_plugin_opener::OpenerExt;
use tracing::info;

// ─── App meta ─────────────────────────────────────────────────────────────

/// Returns the running app version.
#[tauri::command]
fn get_app_version() -> String {
    env!("CARGO_PKG_VERSION").to_string()
}

/// Whether the app runs from an AppImage bundle (Linux). The updater plugin
/// can only self-replace an AppImage; deb/rpm installs degrade to an
/// "open releases page" flow, so the frontend needs this to pick the UI.
#[tauri::command]
fn is_appimage() -> bool {
    std::env::var("APPIMAGE").is_ok()
}

/// Opens the platform state directory (holds the SQLite database) in the OS
/// file manager. Runs on the Rust side via the opener plugin: the plugin's
/// `open_path` IPC permission has no usable default scope, so calling it
/// here bypasses the webview ACL entirely.
#[tauri::command]
fn open_data_dir(app: tauri::AppHandle) -> Result<(), String> {
    let dir = state_dir().map_err(|e| e.to_string())?;
    app.opener()
        .open_path(dir.display().to_string(), None::<&str>)
        .map_err(|e| e.to_string())
}

// ─── Workspace commands (Sprint 1) ───────────────────────────────────────

#[tauri::command]
fn cmd_list_workspaces(
    ctx: tauri::State<'_, AppContext>,
) -> Result<Vec<WorkspaceSummary>, AppError> {
    list_workspaces(&ctx)
}

#[tauri::command]
fn cmd_create_workspace(
    ctx: tauri::State<'_, AppContext>,
    name: String,
) -> Result<Workspace, AppError> {
    create_workspace(&ctx, name)
}

#[tauri::command]
fn cmd_rename_workspace(
    ctx: tauri::State<'_, AppContext>,
    id: String,
    new_name: String,
) -> Result<(), AppError> {
    rename_workspace(&ctx, id, new_name)
}

#[tauri::command]
fn cmd_delete_workspace(ctx: tauri::State<'_, AppContext>, id: String) -> Result<(), AppError> {
    delete_workspace(&ctx, id)
}

#[tauri::command]
fn cmd_set_active_repo(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    repo_id: Option<String>,
) -> Result<(), AppError> {
    set_active_repo(&ctx, workspace_id, repo_id)
}

#[tauri::command]
fn cmd_get_workspace(ctx: tauri::State<'_, AppContext>, id: String) -> Result<Workspace, AppError> {
    get_workspace(&ctx, id)
}

#[tauri::command]
fn cmd_update_workspace_settings(
    ctx: tauri::State<'_, AppContext>,
    id: String,
    settings: WorkspaceSettings,
) -> Result<(), AppError> {
    update_workspace_settings(&ctx, id, settings)
}

#[tauri::command]
fn cmd_set_ai_api_key(
    workspace_id: String,
    provider: String,
    api_key: String,
) -> Result<(), AppError> {
    set_ai_api_key(workspace_id, provider, api_key)
}

#[tauri::command]
fn cmd_clear_ai_api_key(workspace_id: String, provider: String) -> Result<(), AppError> {
    clear_ai_api_key(workspace_id, provider)
}

#[tauri::command]
fn cmd_get_ai_key_status(workspace_id: String, provider: String) -> Result<AiKeyStatus, AppError> {
    get_ai_key_status(workspace_id, provider)
}

#[tauri::command]
async fn cmd_probe_ollama(base_url: Option<String>) -> Result<Vec<String>, AppError> {
    probe_ollama(base_url).await
}

#[tauri::command]
async fn cmd_generate_commit_message(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    language: Option<String>,
) -> Result<AiGenerateOutcome, AppError> {
    generate_commit_message(&ctx, workspace_id, language).await
}

#[tauri::command]
fn cmd_get_repo_ai_rules(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<Option<String>, AppError> {
    get_repo_ai_rules(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_generate_pr_description(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    base: Option<String>,
    language: Option<String>,
) -> Result<PrDescriptionOutcome, AppError> {
    generate_pr_description(&ctx, workspace_id, base, language).await
}

#[tauri::command]
async fn cmd_explain_commit(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    sha: String,
    language: Option<String>,
) -> Result<AiGenerateOutcome, AppError> {
    explain_commit(&ctx, workspace_id, sha, language).await
}

#[tauri::command]
async fn cmd_ai_palette_intent(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    query: String,
) -> Result<PaletteIntent, AppError> {
    ai_palette_intent(&ctx, workspace_id, query).await
}

// ─── Repo commands (Sprint 2) ────────────────────────────────────────────

#[tauri::command]
fn cmd_init_repo(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    path: String,
) -> Result<RepoRef, AppError> {
    init_repo(&ctx, workspace_id, path)
}

#[tauri::command]
async fn cmd_clone_repo(
    app: tauri::AppHandle,
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    url: String,
    dest_path: String,
    replace_dest: Option<bool>,
) -> Result<RepoRef, AppError> {
    use infrastructure::git::repo_adapter::CloneProgress;
    use tauri::Emitter;

    let replace = replace_dest.unwrap_or(false);
    let app_emit = app.clone();
    let on_progress: Option<Box<dyn Fn(CloneProgress) + Send>> =
        Some(Box::new(move |p: CloneProgress| {
            let _ = app_emit.emit("clone-progress", &p);
        }));

    let workspaces = Arc::clone(&ctx.workspaces);
    let result = tauri::async_runtime::spawn_blocking(move || {
        let local_ctx = AppContext::new(workspaces);
        clone_repo(
            &local_ctx,
            workspace_id,
            url,
            dest_path,
            replace,
            on_progress,
        )
    })
    .await
    .map_err(|e| {
        AppError::unknown_with(
            codes::cmds::CLONE_TASK_JOIN,
            format!("clone task join: {e}"),
            &[("error", e.to_string())],
        )
    })?;

    result
}

#[tauri::command]
fn cmd_add_local_repo(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    path: String,
) -> Result<RepoRef, AppError> {
    add_local_repo(&ctx, workspace_id, path)
}

#[tauri::command]
fn cmd_remove_repo(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    repo_id: String,
) -> Result<(), AppError> {
    remove_repo(&ctx, workspace_id, repo_id)
}

#[tauri::command]
fn cmd_relink_repo(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    repo_id: String,
    new_path: String,
) -> Result<(), AppError> {
    relink_repo(&ctx, workspace_id, repo_id, new_path)
}

#[tauri::command]
async fn cmd_list_repos(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<Vec<RepoRef>, AppError> {
    // list_repos sweeps filesystem presence for every repo in the workspace;
    // on stalled network storage that hangs for minutes, and a sync command
    // runs on the UI thread — the whole window would freeze (workspace
    // deletion included). Keep the sweep off the main thread, like clone.
    let workspaces = Arc::clone(&ctx.workspaces);
    let result = tauri::async_runtime::spawn_blocking(move || {
        let local_ctx = AppContext::new(workspaces);
        list_repos(&local_ctx, workspace_id)
    })
    .await
    .map_err(|e| {
        AppError::unknown_with(
            codes::cmds::LIST_REPOS_TASK_JOIN,
            format!("list_repos task join: {e}"),
            &[("error", e.to_string())],
        )
    })?;

    result
}

#[tauri::command]
fn cmd_reorder_repos(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    repo_ids: Vec<String>,
) -> Result<(), AppError> {
    reorder_repos(&ctx, workspace_id, repo_ids)
}

// ─── SSH commands (Sprint 2) ──────────────────────────────────────────────

#[tauri::command]
fn cmd_list_ssh_keys() -> Result<Vec<SshKey>, AppError> {
    list_ssh_keys()
}

#[tauri::command]
fn cmd_add_ssh_key(path: String) -> Result<(), AppError> {
    add_ssh_key(path)
}

#[tauri::command]
fn cmd_delete_ssh_key(path: String) -> Result<(), AppError> {
    delete_ssh_key(path)
}

#[tauri::command]
fn cmd_test_ssh_connection(host: String, user: String) -> Result<SshTestResult, AppError> {
    test_ssh_connection(host, user)
}

// ─── History / Diff / Blame commands (Sprint 3) ───────────────────────────

#[tauri::command]
async fn cmd_get_commit_log(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    max: u32,
    filter: Option<String>,
) -> Result<Vec<CommitSummary>, AppError> {
    get_commit_log(&ctx, &workspace_id, max, filter)
}

#[tauri::command]
async fn cmd_get_workdir_diff(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<DiffSummary, AppError> {
    get_workdir_diff(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_get_commit_details(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    commit_oid: String,
) -> Result<CommitDetails, AppError> {
    get_commit_details(&ctx, &workspace_id, &commit_oid)
}

#[tauri::command]
async fn cmd_get_commit_diff(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    commit_oid: String,
) -> Result<DiffSummary, AppError> {
    get_commit_diff(&ctx, &workspace_id, &commit_oid)
}

#[tauri::command]
async fn cmd_get_file_diff(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    from_oid: String,
    to_oid: String,
) -> Result<Vec<FileDiff>, AppError> {
    get_file_diff(&ctx, &workspace_id, &from_oid, &to_oid)
}

#[tauri::command]
async fn cmd_get_blame(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    path: String,
) -> Result<Vec<BlameLine>, AppError> {
    get_blame(&ctx, &workspace_id, &path)
}

// ─── Branch commands (Sprint 3) ───────────────────────────────────────────

#[tauri::command]
async fn cmd_get_branches(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<Vec<BranchInfo>, AppError> {
    get_branches(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_create_branch(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
    from_sha: String,
) -> Result<BranchInfo, AppError> {
    create_branch(&ctx, &workspace_id, &name, &from_sha)
}

#[tauri::command]
async fn cmd_delete_branch(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
) -> Result<(), AppError> {
    delete_branch(&ctx, &workspace_id, &name)
}

#[tauri::command]
async fn cmd_delete_remote_branch(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    remote: String,
    branch: String,
) -> Result<(), AppError> {
    delete_remote_branch(&ctx, &workspace_id, &remote, &branch)
}

#[tauri::command]
async fn cmd_checkout_branch(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
    force: Option<bool>,
) -> Result<(), AppError> {
    checkout_branch(&ctx, &workspace_id, &name, force.unwrap_or(false))
}

#[tauri::command]
async fn cmd_get_ahead_behind(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    branch_name: String,
) -> Result<AheadBehind, AppError> {
    get_ahead_behind(&ctx, &workspace_id, &branch_name)
}

#[tauri::command]
async fn cmd_merge_branch(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    branch_name: String,
    no_ff: bool,
) -> Result<MergeResult, AppError> {
    merge_branch(&ctx, &workspace_id, &branch_name, no_ff)
}

#[tauri::command]
async fn cmd_merge_preview(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    branch_name: String,
) -> Result<MergePreview, AppError> {
    merge_preview(&ctx, &workspace_id, &branch_name)
}

#[tauri::command]
async fn cmd_list_conflicts(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<Vec<ConflictFile>, AppError> {
    list_conflicts(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_get_conflict_sides(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    path: String,
) -> Result<ConflictSides, AppError> {
    get_conflict_sides(&ctx, &workspace_id, path)
}

#[tauri::command]
async fn cmd_resolve_conflict(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    path: String,
    content: String,
) -> Result<(), AppError> {
    resolve_conflict(&ctx, &workspace_id, path, content)
}

#[tauri::command]
async fn cmd_abort_merge(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<(), AppError> {
    abort_merge(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_merge_in_progress(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<bool, AppError> {
    merge_in_progress(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_explain_conflict(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    path: String,
    language: Option<String>,
) -> Result<AiGenerateOutcome, AppError> {
    explain_conflict(&ctx, workspace_id, path, language).await
}

#[tauri::command]
async fn cmd_rebase_branch(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    upstream: String,
) -> Result<RebaseResult, AppError> {
    rebase_branch(&ctx, &workspace_id, &upstream)
}

#[tauri::command]
async fn cmd_revert_commit(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    commit_oid: String,
) -> Result<String, AppError> {
    revert_commit(&ctx, &workspace_id, &commit_oid)
}

#[tauri::command]
async fn cmd_cherry_pick_commit(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    commit_oid: String,
) -> Result<String, AppError> {
    cherry_pick_commit(&ctx, &workspace_id, &commit_oid)
}

#[tauri::command]
async fn cmd_list_tags(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<Vec<infrastructure::git::tag::TagInfo>, AppError> {
    list_tags(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_create_tag(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
    target_oid: Option<String>,
    message: Option<String>,
) -> Result<String, AppError> {
    create_tag(&ctx, &workspace_id, &name, target_oid, message)
}

#[tauri::command]
async fn cmd_delete_tag(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
) -> Result<(), AppError> {
    delete_tag(&ctx, &workspace_id, &name)
}

#[tauri::command]
async fn cmd_list_submodules(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<Vec<infrastructure::git::submodule::SubmoduleInfo>, AppError> {
    list_submodules(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_init_submodule(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
) -> Result<(), AppError> {
    init_submodule(&ctx, &workspace_id, &name)
}

#[tauri::command]
async fn cmd_update_submodule(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
    recursive: Option<bool>,
) -> Result<(), AppError> {
    update_submodule(&ctx, &workspace_id, &name, recursive.unwrap_or(false))
}

#[tauri::command]
async fn cmd_add_submodule(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    url: String,
    path: String,
) -> Result<(), AppError> {
    add_submodule(&ctx, &workspace_id, url, path)
}

#[tauri::command]
async fn cmd_deinit_submodule(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
) -> Result<(), AppError> {
    deinit_submodule(&ctx, &workspace_id, &name)
}

#[tauri::command]
async fn cmd_lfs_status(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<LfsStatus, AppError> {
    lfs_status(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_lfs_install(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<String, AppError> {
    lfs_install(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_lfs_track(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    pattern: String,
) -> Result<(), AppError> {
    lfs_track(&ctx, &workspace_id, pattern)
}

#[tauri::command]
async fn cmd_lfs_untrack(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    pattern: String,
) -> Result<(), AppError> {
    lfs_untrack(&ctx, &workspace_id, &pattern)
}

#[tauri::command]
async fn cmd_get_health(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<infrastructure::git::health::HealthReport, AppError> {
    get_health(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_explain_health(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    language: Option<String>,
) -> Result<String, AppError> {
    explain_health(&ctx, workspace_id, language).await
}

#[tauri::command]
async fn cmd_reset_hard(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    oid: String,
) -> Result<(), AppError> {
    reset_hard(&ctx, &workspace_id, &oid)
}

#[tauri::command]
async fn cmd_explain_reflog(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    old_oid: String,
    new_oid: String,
    action: String,
    message: String,
    language: Option<String>,
) -> Result<String, AppError> {
    explain_reflog(
        &ctx,
        workspace_id,
        old_oid,
        new_oid,
        action,
        message,
        language,
    )
    .await
}

#[tauri::command]
async fn cmd_list_remote_details(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<Vec<infrastructure::git::remote::RemoteInfo>, AppError> {
    list_remote_details(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_add_remote(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
    url: String,
) -> Result<(), AppError> {
    add_remote(&ctx, &workspace_id, &name, &url)
}

#[tauri::command]
async fn cmd_set_remote_url(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
    url: String,
) -> Result<(), AppError> {
    set_remote_url(&ctx, &workspace_id, &name, &url)
}

#[tauri::command]
async fn cmd_set_remote_push_url(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
    url: Option<String>,
) -> Result<(), AppError> {
    set_remote_push_url(&ctx, &workspace_id, &name, url)
}

#[tauri::command]
async fn cmd_rename_remote(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
    new_name: String,
) -> Result<(), AppError> {
    rename_remote(&ctx, &workspace_id, &name, &new_name)
}

#[tauri::command]
async fn cmd_remove_remote(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
) -> Result<(), AppError> {
    remove_remote(&ctx, &workspace_id, &name)
}

#[tauri::command]
async fn cmd_list_reflog(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    reference: Option<String>,
) -> Result<Vec<infrastructure::git::reflog::ReflogEntry>, AppError> {
    list_reflog(&ctx, &workspace_id, reference)
}

#[tauri::command]
async fn cmd_get_gitignore(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<String, AppError> {
    get_gitignore(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_list_hooks(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<Vec<HookInfo>, AppError> {
    list_hooks(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_get_hook(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
) -> Result<String, AppError> {
    get_hook(&ctx, &workspace_id, &name)
}

#[tauri::command]
async fn cmd_save_hook(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
    content: String,
) -> Result<(), AppError> {
    save_hook(&ctx, &workspace_id, &name, content)
}

#[tauri::command]
async fn cmd_write_gitignore(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    content: String,
) -> Result<(), AppError> {
    write_gitignore(&ctx, &workspace_id, &content)
}

#[tauri::command]
async fn cmd_export_workspace(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    dest_path: String,
) -> Result<String, AppError> {
    export_workspace(&ctx, &workspace_id, &dest_path)
}

#[tauri::command]
async fn cmd_import_workspace(
    ctx: tauri::State<'_, AppContext>,
    src_path: String,
    new_name: Option<String>,
) -> Result<WorkspaceSummary, AppError> {
    import_workspace(&ctx, &src_path, new_name)
}

#[tauri::command]
async fn cmd_plan_interactive_rebase(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    upstream: String,
) -> Result<Vec<InteractiveRebaseTodo>, AppError> {
    plan_interactive_rebase(&ctx, &workspace_id, &upstream)
}

#[tauri::command]
async fn cmd_execute_interactive_rebase(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    upstream: String,
    todos: Vec<InteractiveRebaseTodo>,
) -> Result<InteractiveRebaseResult, AppError> {
    execute_interactive_rebase(&ctx, &workspace_id, &upstream, todos)
}

#[tauri::command]
async fn cmd_continue_interactive_rebase(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<InteractiveRebaseResult, AppError> {
    continue_interactive_rebase(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_abort_interactive_rebase_pause(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<(), AppError> {
    abort_interactive_rebase_pause(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_interactive_rebase_paused(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<bool, AppError> {
    interactive_rebase_paused(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_get_working_copy(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<WorkingCopy, AppError> {
    get_working_copy(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_stage_files(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    paths: Vec<String>,
) -> Result<(), AppError> {
    stage_files(&ctx, &workspace_id, paths)
}

#[tauri::command]
async fn cmd_unstage_files(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    paths: Vec<String>,
) -> Result<(), AppError> {
    unstage_files(&ctx, &workspace_id, paths)
}

#[tauri::command]
async fn cmd_stage_all(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<(), AppError> {
    stage_all(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_commit(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    message: String,
) -> Result<String, AppError> {
    commit(&ctx, &workspace_id, message)
}

#[tauri::command]
async fn cmd_discard_changes(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    paths: Vec<String>,
) -> Result<(), AppError> {
    discard_changes(&ctx, &workspace_id, paths)
}

#[tauri::command]
async fn cmd_ignore_path(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    pattern: String,
) -> Result<(), AppError> {
    ignore_path(&ctx, &workspace_id, pattern)
}

#[tauri::command]
async fn cmd_fetch(
    app: tauri::AppHandle,
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    remote: Option<String>,
) -> Result<(), AppError> {
    use application::use_cases::fetch;
    use infrastructure::git::remote::SyncProgress;
    use std::sync::Arc;
    use tauri::Emitter;

    let app_emit = app.clone();
    let on_progress: Option<Box<dyn Fn(SyncProgress) + Send>> =
        Some(Box::new(move |p: SyncProgress| {
            let _ = app_emit.emit("sync-progress", &p);
        }));

    let workspaces = Arc::clone(&ctx.workspaces);
    tauri::async_runtime::spawn_blocking(move || {
        let local_ctx = AppContext::new(workspaces);
        fetch(&local_ctx, &workspace_id, remote, on_progress)
    })
    .await
    .map_err(|e| {
        AppError::unknown_with(
            codes::cmds::FETCH_TASK_JOIN,
            format!("fetch task join: {e}"),
            &[("error", e.to_string())],
        )
    })?
}

#[tauri::command]
async fn cmd_pull(
    app: tauri::AppHandle,
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    remote: Option<String>,
    branch: Option<String>,
    rebase: Option<bool>,
    stash: Option<bool>,
) -> Result<(), AppError> {
    use application::use_cases::pull;
    use infrastructure::git::remote::SyncProgress;
    use std::sync::Arc;
    use tauri::Emitter;

    let app_emit = app.clone();
    let on_progress: Option<Box<dyn Fn(SyncProgress) + Send>> =
        Some(Box::new(move |p: SyncProgress| {
            let _ = app_emit.emit("sync-progress", &p);
        }));

    let workspaces = Arc::clone(&ctx.workspaces);
    tauri::async_runtime::spawn_blocking(move || {
        let local_ctx = AppContext::new(workspaces);
        pull(
            &local_ctx,
            &workspace_id,
            remote,
            branch,
            rebase.unwrap_or(false),
            stash.unwrap_or(false),
            on_progress,
        )
    })
    .await
    .map_err(|e| {
        AppError::unknown_with(
            codes::cmds::PULL_TASK_JOIN,
            format!("pull task join: {e}"),
            &[("error", e.to_string())],
        )
    })?
}

#[tauri::command]
async fn cmd_list_remotes(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<Vec<String>, AppError> {
    use application::use_cases::list_remotes;
    use std::sync::Arc;

    let workspaces = Arc::clone(&ctx.workspaces);
    tauri::async_runtime::spawn_blocking(move || {
        let local_ctx = AppContext::new(workspaces);
        list_remotes(&local_ctx, &workspace_id)
    })
    .await
    .map_err(|e| {
        AppError::unknown_with(
            codes::cmds::LIST_REMOTES_TASK_JOIN,
            format!("list remotes task join: {e}"),
            &[("error", e.to_string())],
        )
    })?
}

#[tauri::command]
async fn cmd_push(
    app: tauri::AppHandle,
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    remote: Option<String>,
    tags: Option<bool>,
    force: Option<bool>,
) -> Result<(), AppError> {
    use application::use_cases::push;
    use infrastructure::git::remote::SyncProgress;
    use std::sync::Arc;
    use tauri::Emitter;

    let app_emit = app.clone();
    let on_progress: Option<Box<dyn Fn(SyncProgress) + Send>> =
        Some(Box::new(move |p: SyncProgress| {
            let _ = app_emit.emit("sync-progress", &p);
        }));

    let workspaces = Arc::clone(&ctx.workspaces);
    tauri::async_runtime::spawn_blocking(move || {
        let local_ctx = AppContext::new(workspaces);
        push(
            &local_ctx,
            &workspace_id,
            remote,
            tags.unwrap_or(false),
            force.unwrap_or(false),
            on_progress,
        )
    })
    .await
    .map_err(|e| {
        AppError::unknown_with(
            codes::cmds::PUSH_TASK_JOIN,
            format!("push task join: {e}"),
            &[("error", e.to_string())],
        )
    })?
}

#[tauri::command]
async fn cmd_list_stashes(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<Vec<StashEntry>, AppError> {
    list_stashes(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_save_stash(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    message: Option<String>,
    include_untracked: Option<bool>,
) -> Result<String, AppError> {
    save_stash(
        &ctx,
        &workspace_id,
        message,
        include_untracked.unwrap_or(true),
    )
}

#[tauri::command]
async fn cmd_apply_stash(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    index: u32,
) -> Result<(), AppError> {
    apply_stash(&ctx, &workspace_id, index)
}

#[tauri::command]
async fn cmd_pop_stash(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    index: u32,
) -> Result<(), AppError> {
    pop_stash(&ctx, &workspace_id, index)
}

#[tauri::command]
async fn cmd_drop_stash(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    index: u32,
) -> Result<(), AppError> {
    drop_stash(&ctx, &workspace_id, index)
}

#[tauri::command]
async fn cmd_get_stash_diff(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    oid: String,
) -> Result<DiffSummary, AppError> {
    get_stash_diff(&ctx, &workspace_id, &oid)
}

#[tauri::command]
async fn cmd_list_worktrees(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
) -> Result<Vec<WorktreeInfo>, AppError> {
    list_worktrees(&ctx, &workspace_id)
}

#[tauri::command]
async fn cmd_add_worktree(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
    path: String,
    branch: String,
    create_branch: bool,
    start_point: Option<String>,
) -> Result<WorktreeInfo, AppError> {
    add_worktree(
        &ctx,
        &workspace_id,
        name,
        path,
        branch,
        create_branch,
        start_point,
    )
}

#[tauri::command]
async fn cmd_remove_worktree(
    ctx: tauri::State<'_, AppContext>,
    workspace_id: String,
    name: String,
) -> Result<(), AppError> {
    remove_worktree(&ctx, &workspace_id, name)
}

// ─── Window decoration (tauri-plugin-decoration v3) ───────────────────────

async fn restore_and_show(
    window: &WebviewWindow,
    activation_error: impl Display,
) -> Result<&'static str, String> {
    let restore_error = window.restore_decoration().await.err();
    let show_error = window.show().err();

    match (restore_error, show_error) {
        (None, None) => Ok("native"),
        (Some(restore), None) => Err(format!(
            "custom decoration failed ({activation_error}); native restoration failed ({restore}), but the window was revealed"
        )),
        (None, Some(show)) => Err(format!(
            "custom decoration failed ({activation_error}); native fallback could not be revealed ({show})"
        )),
        (Some(restore), Some(show)) => Err(format!(
            "custom decoration failed ({activation_error}); native restoration failed ({restore}); revealing the fallback also failed ({show})"
        )),
    }
}

#[tauri::command]
async fn activate_and_show(window: WebviewWindow) -> Result<&'static str, String> {
    if let Err(error) = window.activate_decoration().await {
        return restore_and_show(&window, error).await;
    }

    #[cfg(target_os = "macos")]
    if let Err(error) = window.set_traffic_lights_inset(14.0, 14.0).await {
        return restore_and_show(&window, error).await;
    }

    if let Err(error) = macos_window::configure_window(&window) {
        tracing::warn!("macOS window chrome setup failed: {error}");
    }

    match window.show() {
        Ok(()) => Ok("custom"),
        Err(error) => restore_and_show(&window, error).await,
    }
}

#[tauri::command]
fn toggle_instant_zoom(window: WebviewWindow) -> Result<(), String> {
    window
        .clone()
        .run_on_main_thread(move || {
            if let Err(error) = macos_window::toggle_instant_zoom(&window) {
                tracing::warn!("instant zoom toggle failed: {error}");
            }
        })
        .map_err(|e| e.to_string())
}

/// Quit the whole application (File → Exit menu item), regardless of how the
/// platform treats plain window closes (e.g. macOS keeps apps alive).
#[tauri::command]
fn quit_app(app: tauri::AppHandle) {
    app.exit(0);
}

// ─── App startup ──────────────────────────────────────────────────────────

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let _guard = init_tracing();
    info!("GitWave starting (version {})", env!("CARGO_PKG_VERSION"));

    let ctx = match open_state() {
        Ok(conn) => AppContext::new(Arc::new(Mutex::new(SqliteWorkspaceRepo::new(conn)))),
        Err(e) => {
            tracing::error!("failed to open SQLite state, falling back to in-memory: {e}");
            let conn = rusqlite::Connection::open_in_memory().expect("in-memory fallback");
            migrations::apply(&conn).expect("fallback migrations");
            AppContext::new(Arc::new(Mutex::new(SqliteWorkspaceRepo::new(conn))))
        }
    };

    tauri::Builder::default()
        .plugin(tauri_plugin_opener::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_decoration::init())
        .plugin(tauri_plugin_updater::Builder::new().build())
        .plugin(tauri_plugin_process::init())
        .plugin(tauri_plugin_os::init())
        .manage(ctx)
        .invoke_handler(tauri::generate_handler![
            activate_and_show,
            toggle_instant_zoom,
            quit_app,
            get_app_version,
            is_appimage,
            open_data_dir,
            cmd_list_workspaces,
            cmd_create_workspace,
            cmd_rename_workspace,
            cmd_delete_workspace,
            cmd_set_active_repo,
            cmd_get_workspace,
            cmd_update_workspace_settings,
            cmd_set_ai_api_key,
            cmd_clear_ai_api_key,
            cmd_get_ai_key_status,
            cmd_probe_ollama,
            cmd_generate_commit_message,
            cmd_generate_pr_description,
            cmd_explain_commit,
            cmd_ai_palette_intent,
            cmd_get_repo_ai_rules,
            cmd_init_repo,
            cmd_clone_repo,
            cmd_add_local_repo,
            cmd_remove_repo,
            cmd_relink_repo,
            cmd_list_repos,
            cmd_reorder_repos,
            cmd_list_ssh_keys,
            cmd_add_ssh_key,
            cmd_delete_ssh_key,
            cmd_test_ssh_connection,
            cmd_get_commit_log,
            cmd_get_commit_details,
            cmd_get_workdir_diff,
            cmd_get_commit_diff,
            cmd_get_file_diff,
            cmd_get_blame,
            cmd_get_branches,
            cmd_create_branch,
            cmd_delete_branch,
            cmd_delete_remote_branch,
            cmd_checkout_branch,
            cmd_get_ahead_behind,
            cmd_merge_branch,
            cmd_merge_preview,
            cmd_list_conflicts,
            cmd_get_conflict_sides,
            cmd_resolve_conflict,
            cmd_abort_merge,
            cmd_merge_in_progress,
            cmd_explain_conflict,
            cmd_rebase_branch,
            cmd_revert_commit,
            cmd_cherry_pick_commit,
            cmd_list_tags,
            cmd_create_tag,
            cmd_delete_tag,
            cmd_list_submodules,
            cmd_init_submodule,
            cmd_update_submodule,
            cmd_add_submodule,
            cmd_deinit_submodule,
            cmd_lfs_status,
            cmd_lfs_install,
            cmd_lfs_track,
            cmd_lfs_untrack,
            cmd_get_gitignore,
            cmd_list_reflog,
            cmd_list_hooks,
            cmd_get_hook,
            cmd_save_hook,
            cmd_write_gitignore,
            cmd_export_workspace,
            cmd_import_workspace,
            cmd_plan_interactive_rebase,
            cmd_execute_interactive_rebase,
            cmd_continue_interactive_rebase,
            cmd_abort_interactive_rebase_pause,
            cmd_interactive_rebase_paused,
            cmd_get_working_copy,
            cmd_stage_files,
            cmd_unstage_files,
            cmd_stage_all,
            cmd_commit,
            cmd_discard_changes,
            cmd_ignore_path,
            cmd_fetch,
            cmd_pull,
            cmd_list_remotes,
            cmd_reset_hard,
            cmd_explain_reflog,
            cmd_get_health,
            cmd_explain_health,
            cmd_list_remote_details,
            cmd_add_remote,
            cmd_set_remote_url,
            cmd_set_remote_push_url,
            cmd_rename_remote,
            cmd_remove_remote,
            cmd_push,
            cmd_list_stashes,
            cmd_save_stash,
            cmd_apply_stash,
            cmd_pop_stash,
            cmd_drop_stash,
            cmd_get_stash_diff,
            cmd_list_worktrees,
            cmd_add_worktree,
            cmd_remove_worktree,
        ])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
