//! Application use cases — workspace, repo, SSH, history, diff, blame, and branch.
//!
//! See `docs/tasks/feat-history-graph/plan.md` steps 6-10.

use std::path::PathBuf;
use std::sync::{Arc, Mutex};

use crate::domain::blame::BlameLine;
use crate::domain::branch::BranchInfo;
use crate::domain::diff::FileDiff;
use crate::domain::error::{AppError, Result};
use crate::domain::history::CommitSummary;
use crate::domain::stash::StashEntry;
use crate::domain::working_copy::WorkingCopy;
use crate::domain::workspace::{
    RepoRef, RepoStatus, Workspace, WorkspaceSettings, WorkspaceSummary,
};
use crate::domain::worktree::WorktreeInfo;
use crate::infrastructure::git::blame::blame_file as infra_blame_file;
use crate::infrastructure::git::branch::{
    checkout_branch as infra_checkout_branch, create_branch as infra_create_branch,
    delete_branch as infra_delete_branch,
};
use crate::infrastructure::git::conflict::{
    abort_merge as infra_abort_merge, get_conflict_sides as infra_get_conflict_sides,
    is_merge_in_progress as infra_merge_in_progress, list_conflicts as infra_list_conflicts,
    resolve_conflict as infra_resolve_conflict, ConflictFile, ConflictSides,
};
use crate::infrastructure::git::diff::{
    diff_commit_vs_parent as infra_diff_commit_vs_parent,
    diff_index_to_head as infra_diff_index_to_head, diff_paths as infra_diff_paths,
    diff_workdir_to_index as infra_diff_workdir_to_index, DiffSummary,
};
use crate::infrastructure::git::history::{
    ahead_behind as infra_ahead_behind, commit_log as infra_commit_log,
    list_branches as infra_list_branches,
};
use crate::infrastructure::git::interactive_rebase::{
    abort_interactive_rebase_pause as infra_abort_irebase_pause,
    continue_interactive_rebase as infra_continue_irebase,
    execute_interactive_rebase as infra_execute_irebase,
    interactive_rebase_paused as infra_irebase_paused,
    plan_interactive_rebase as infra_plan_irebase, InteractiveRebaseResult, InteractiveRebaseTodo,
};
use crate::infrastructure::git::merge::{merge_branch as infra_merge_branch, MergeResult};
use crate::infrastructure::git::rebase::{rebase_branch as infra_rebase_branch, RebaseResult};
use crate::infrastructure::git::remote::{
    delete_remote_branch as infra_delete_remote_branch, fetch as infra_fetch,
    list_remotes as infra_list_remotes, pull_with_options as infra_pull_with_options,
    push_with_options as infra_push_with_options, PullOptions, PushRequest, SyncProgress,
};
use crate::infrastructure::git::stash::{
    apply_stash as infra_apply_stash, drop_stash as infra_drop_stash,
    list_stashes as infra_list_stashes, pop_stash as infra_pop_stash,
    save_stash as infra_save_stash, stash_diff as infra_stash_diff,
};
use crate::infrastructure::git::working_copy::{
    commit as infra_commit, discard_worktree_changes as infra_discard_worktree_changes,
    ignore_path as infra_ignore_path, stage_all as infra_stage_all,
    stage_paths as infra_stage_paths, status as infra_wc_status,
    unstage_paths as infra_unstage_paths,
};
use crate::infrastructure::git::worktree::{
    add_worktree as infra_add_worktree, list_worktrees as infra_list_worktrees,
    remove_worktree as infra_remove_worktree,
};
use crate::infrastructure::persistence::workspace_repo::WorkspaceRepository;
use crate::infrastructure::persistence::SqliteWorkspaceRepo;
use crate::infrastructure::ssh::keys::{SshKey, SshTestResult};

// ─── AppContext ──────────────────────────────────────────────────────────────

/// Application context — bundles infrastructure adapters and exposes use
/// cases. Held by Tauri as managed state.
pub struct AppContext {
    pub workspaces: Arc<Mutex<SqliteWorkspaceRepo>>,
}

impl AppContext {
    #[must_use]
    pub fn new(workspaces: Arc<Mutex<SqliteWorkspaceRepo>>) -> Self {
        Self { workspaces }
    }

    /// Open a Repository for the given repo path (no caching — git2::Repository
    /// is !Sync so can't be stored in shared state).
    #[allow(dead_code)]
    pub fn open_repo(&self, repo_path: &str) -> Result<git2::Repository> {
        git2::Repository::open(PathBuf::from(repo_path))
            .map_err(|e| AppError::Unknown(format!("git open: {e}")))
    }
}

// ─── Existing workspace/repo/SSH use cases (unchanged) ─────────────────────

fn new_workspace_id() -> String {
    use std::time::{SystemTime, UNIX_EPOCH};
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_nanos())
        .unwrap_or(0);
    format!("ws-{nanos:x}")
}

fn new_repo_id() -> String {
    use std::time::{SystemTime, UNIX_EPOCH};
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_nanos())
        .unwrap_or(0);
    format!("r-{nanos:x}")
}

fn now_unix() -> i64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or(0)
}

// ─── Workspace use cases (Sprint 1) ────────────────────────────────────────

pub fn create_workspace(ctx: &AppContext, name: String) -> Result<Workspace> {
    let trimmed = name.trim();
    if trimmed.is_empty() {
        return Err(AppError::Protocol("workspace name cannot be empty".into()));
    }
    let now = now_unix();
    let ws = Workspace {
        id: new_workspace_id(),
        name: trimmed.to_string(),
        repos: Vec::new(),
        settings: WorkspaceSettings::default(),
        last_active_repo_id: None,
        created_at: now,
        updated_at: now,
    };
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .create(&ws)?;
    Ok(ws)
}

pub fn list_workspaces(ctx: &AppContext) -> Result<Vec<WorkspaceSummary>> {
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .list_summaries()
}

pub fn rename_workspace(ctx: &AppContext, id: String, new_name: String) -> Result<()> {
    let trimmed = new_name.trim();
    if trimmed.is_empty() {
        return Err(AppError::Protocol("workspace name cannot be empty".into()));
    }
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .rename(&id, trimmed)
}

pub fn delete_workspace(ctx: &AppContext, id: String) -> Result<()> {
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .delete(&id)
}

pub fn get_workspace(ctx: &AppContext, id: String) -> Result<Workspace> {
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .get(&id)?
        .ok_or_else(|| AppError::Protocol(format!("workspace not found: {id}")))
}

pub fn update_workspace_settings(
    ctx: &AppContext,
    id: String,
    settings: WorkspaceSettings,
) -> Result<()> {
    let provider = settings.ai_provider.as_deref().unwrap_or("");
    if !provider.is_empty() && !matches!(provider, "openai" | "anthropic" | "ollama") {
        return Err(AppError::Protocol(format!(
            "unsupported ai_provider: {provider}"
        )));
    }
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .update_settings(&id, &settings)
}

pub fn set_active_repo(
    ctx: &AppContext,
    workspace_id: String,
    repo_id: Option<String>,
) -> Result<()> {
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .set_active_repo(&workspace_id, repo_id.as_deref())
}

// ─── Repo use cases (Sprint 2) ──────────────────────────────────────────────

pub fn init_repo(ctx: &AppContext, workspace_id: String, path: String) -> Result<RepoRef> {
    let p = PathBuf::from(&path);
    crate::infrastructure::git::repo_adapter::init(&p)?;

    let repo = RepoRef {
        id: new_repo_id(),
        workspace_id,
        path,
        nickname: None,
        settings_override: None,
        status: RepoStatus::Active,
        missing_since: None,
        added_at: now_unix(),
    };
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .add_repo(&repo)?;
    Ok(repo)
}

pub fn clone_repo(
    ctx: &AppContext,
    workspace_id: String,
    url: String,
    dest_path: String,
    replace_dest: bool,
    on_progress: Option<
        Box<dyn Fn(crate::infrastructure::git::repo_adapter::CloneProgress) + Send>,
    >,
) -> Result<RepoRef> {
    let dest = PathBuf::from(&dest_path);
    if replace_dest && dest.exists() {
        std::fs::remove_dir_all(&dest)
            .map_err(|e| AppError::Unknown(format!("failed to clear dest for retry: {e}")))?;
    }
    if url.starts_with("ssh://") || url.starts_with("git@") {
        crate::infrastructure::git::repo_adapter::clone_ssh(&url, &dest, on_progress)?;
    } else {
        crate::infrastructure::git::repo_adapter::clone_https(&url, &dest, on_progress)?;
    }

    let repo = RepoRef {
        id: new_repo_id(),
        workspace_id,
        path: dest_path,
        nickname: None,
        settings_override: None,
        status: RepoStatus::Active,
        missing_since: None,
        added_at: now_unix(),
    };
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .add_repo(&repo)?;
    Ok(repo)
}

pub fn add_local_repo(ctx: &AppContext, workspace_id: String, path: String) -> Result<RepoRef> {
    let p = PathBuf::from(&path);
    let _repo = crate::infrastructure::git::git2_adapter::open_local(&p)?;

    let repo = RepoRef {
        id: new_repo_id(),
        workspace_id,
        path,
        nickname: None,
        settings_override: None,
        status: RepoStatus::Active,
        missing_since: None,
        added_at: now_unix(),
    };
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .add_repo(&repo)?;
    Ok(repo)
}

pub fn remove_repo(ctx: &AppContext, workspace_id: String, repo_id: String) -> Result<()> {
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .remove_repo(&workspace_id, &repo_id)
}

pub fn relink_repo(
    ctx: &AppContext,
    workspace_id: String,
    repo_id: String,
    new_path: String,
) -> Result<()> {
    let p = PathBuf::from(&new_path);
    let _repo = crate::infrastructure::git::git2_adapter::open_local(&p)?;
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .relink_repo(&workspace_id, &repo_id, &new_path)
}

pub fn list_repos(ctx: &AppContext, workspace_id: String) -> Result<Vec<RepoRef>> {
    // Sweep filesystem presence so Missing badges stay accurate without a
    // separate background job (W3 acceptance: missing detection on list).
    refresh_repo_presence(ctx, &workspace_id)?;
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .list_repos(&workspace_id)
}

/// Mark Active→Missing when path is gone; Missing→Active when path is a git repo again.
fn refresh_repo_presence(ctx: &AppContext, workspace_id: &str) -> Result<()> {
    let repos = ctx
        .workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .list_repos(workspace_id)?;

    for repo in repos {
        let path = PathBuf::from(&repo.path);
        let present = crate::infrastructure::git::git2_adapter::open_local(&path).is_ok();
        match (present, repo.status) {
            (false, RepoStatus::Active) => {
                ctx.workspaces
                    .lock()
                    .expect("workspace repo mutex poisoned")
                    .mark_repo_missing(workspace_id, &repo.id)?;
            }
            (true, RepoStatus::Missing) => {
                // Same path is valid again — flip without requiring a new path from the user.
                ctx.workspaces
                    .lock()
                    .expect("workspace repo mutex poisoned")
                    .relink_repo(workspace_id, &repo.id, &repo.path)?;
            }
            _ => {}
        }
    }
    Ok(())
}

// ─── SSH use cases (Sprint 2) ───────────────────────────────────────────────

pub fn list_ssh_keys() -> Result<Vec<SshKey>> {
    crate::infrastructure::ssh::keys::list_loaded()
}

pub fn add_ssh_key(path: String) -> Result<()> {
    let p = PathBuf::from(path);
    crate::infrastructure::ssh::keys::add(&p)
}

pub fn delete_ssh_key(path: String) -> Result<()> {
    let p = PathBuf::from(path);
    crate::infrastructure::ssh::keys::delete(&p)
}

pub fn test_ssh_connection(host: String, user: String) -> Result<SshTestResult> {
    crate::infrastructure::ssh::keys::test_connection(&host, &user)
}

// ─── AI provider (Sprint 4) ─────────────────────────────────────────────────

#[derive(Debug, Clone, serde::Serialize)]
pub struct AiKeyStatus {
    pub provider: String,
    pub has_key: bool,
}

pub fn set_ai_api_key(workspace_id: String, provider: String, api_key: String) -> Result<()> {
    crate::infrastructure::ai::set_api_key(&workspace_id, &provider, &api_key)
}

pub fn clear_ai_api_key(workspace_id: String, provider: String) -> Result<()> {
    crate::infrastructure::ai::clear_api_key(&workspace_id, &provider)
}

pub fn get_ai_key_status(workspace_id: String, provider: String) -> Result<AiKeyStatus> {
    let has_key = crate::infrastructure::ai::has_api_key(&workspace_id, &provider)?;
    Ok(AiKeyStatus { provider, has_key })
}

pub async fn probe_ollama(base_url: Option<String>) -> Result<Vec<String>> {
    crate::infrastructure::ai::probe_ollama(base_url).await
}

/// Generate a commit message suggestion from staged/workdir diff + recent commits.
/// Result is always returned for the user to edit — never auto-commits (P1).
pub async fn generate_commit_message(ctx: &AppContext, workspace_id: String) -> Result<String> {
    let ws = get_workspace(ctx, workspace_id.clone())?;
    let settings = ws.settings;
    let provider = settings.ai_provider.clone().ok_or_else(|| {
        AppError::Protocol("AI provider not configured for this Workspace".into())
    })?;
    let model = settings
        .ai_model
        .clone()
        .unwrap_or_else(|| match provider.as_str() {
            "anthropic" => "claude-3-5-haiku-latest".into(),
            "ollama" => "llama3.2".into(),
            _ => "gpt-4o-mini".into(),
        });
    let api_key = if provider == "ollama" {
        None
    } else {
        crate::infrastructure::ai::get_api_key(&workspace_id, &provider)?
    };

    let repo_path = active_repo_path(ctx, &workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    let staged = infra_diff_index_to_head(&repo)?;
    let workdir = infra_diff_workdir_to_index(&repo)?;
    let recent = infra_commit_log(&repo, 8)?;

    let mut user = String::new();
    user.push_str("Recent commits (newest first):\n");
    for c in &recent {
        user.push_str(&format!("- {}\n", c.message_summary));
    }
    user.push_str("\nStaged changes:\n");
    append_diff_summary(&mut user, &staged);
    user.push_str("\nUnstaged changes:\n");
    append_diff_summary(&mut user, &workdir);

    let system = settings.prompt_templates.commit.unwrap_or_else(|| {
        "You write concise git commit messages. Output ONLY the message text. \
             Prefer conventional commits (type: summary). First line <= 72 chars. \
             Do not wrap in markdown fences."
            .into()
    });

    crate::infrastructure::ai::generate_text(crate::infrastructure::ai::AiGenerateRequest {
        provider,
        model,
        base_url: settings.ai_base_url,
        api_key,
        system,
        user,
    })
    .await
}

fn append_diff_summary(buf: &mut String, diff: &DiffSummary) {
    if diff.files.is_empty() {
        buf.push_str("(none)\n");
        return;
    }
    for f in &diff.files {
        buf.push_str(&format!(
            "- {} (+{} -{})\n",
            f.path, f.additions, f.deletions
        ));
    }
}

// ─── History use cases (Sprint 3) ───────────────────────────────────────────

/// Get the commit log for the active repo in a workspace.
pub fn get_commit_log(
    ctx: &AppContext,
    workspace_id: &str,
    max: u32,
) -> Result<Vec<CommitSummary>> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_commit_log(&repo, max)
}

/// Get the working-copy diff. Staged files are tagged `staged: true` (index vs HEAD);
/// unstaged files are tagged `staged: false` (worktree vs index). Same path can appear twice.
pub fn get_workdir_diff(ctx: &AppContext, workspace_id: &str) -> Result<DiffSummary> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    let unstaged = infra_diff_workdir_to_index(&repo)?.mark_staged(false);
    let staged = infra_diff_index_to_head(&repo)?.mark_staged(true);
    Ok(staged.merge(unstaged))
}

/// Get diff between a commit and its parent.
pub fn get_commit_diff(
    ctx: &AppContext,
    workspace_id: &str,
    commit_oid: &str,
) -> Result<DiffSummary> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    let oid = git2::Oid::from_str(commit_oid)
        .map_err(|e| AppError::Protocol(format!("invalid commit OID: {e}")))?;
    infra_diff_commit_vs_parent(&repo, oid)
}

/// Get diff between two commits for a specific file path.
pub fn get_file_diff(
    ctx: &AppContext,
    workspace_id: &str,
    from_oid: &str,
    to_oid: &str,
) -> Result<Vec<FileDiff>> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    let from = git2::Oid::from_str(from_oid)
        .map_err(|e| AppError::Protocol(format!("invalid from OID: {e}")))?;
    let to = git2::Oid::from_str(to_oid)
        .map_err(|e| AppError::Protocol(format!("invalid to OID: {e}")))?;
    infra_diff_paths(&repo, from, to)
}

/// Get blame lines for a file in the active repo.
pub fn get_blame(ctx: &AppContext, workspace_id: &str, path: &str) -> Result<Vec<BlameLine>> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_blame_file(&repo, path)
}

// ─── Branch use cases (Sprint 3) ────────────────────────────────────────────

/// List all branches in the active repo.
pub fn get_branches(ctx: &AppContext, workspace_id: &str) -> Result<Vec<BranchInfo>> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_list_branches(&repo)
}

/// Create a new branch in the active repo.
pub fn create_branch(
    ctx: &AppContext,
    workspace_id: &str,
    name: &str,
    from_sha: &str,
) -> Result<BranchInfo> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_create_branch(&repo, name, from_sha, false)?;
    // Re-list to return the full BranchInfo
    let branches = infra_list_branches(&repo)?;
    branches
        .into_iter()
        .find(|b| b.name == name)
        .ok_or_else(|| AppError::Unknown(format!("branch {} not found after creation", name)))
}

/// Delete a local branch.
pub fn delete_branch(ctx: &AppContext, workspace_id: &str, name: &str) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_delete_branch(&repo, name)
}

/// Delete `branch` on `remote` (push a bare refspec to the remote).
pub fn delete_remote_branch(
    ctx: &AppContext,
    workspace_id: &str,
    remote: &str,
    branch: &str,
) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_delete_remote_branch(&repo, remote, branch)
}

/// Check out a branch (updates HEAD and working tree).
pub fn checkout_branch(
    ctx: &AppContext,
    workspace_id: &str,
    name: &str,
    force: bool,
) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_checkout_branch(&repo, name, force)
}

/// Get ahead/behind counts for a branch against its upstream.
#[derive(Debug, Clone, serde::Serialize)]
pub struct AheadBehind {
    pub ahead: u32,
    pub behind: u32,
}

/// Get ahead/behind for a local branch relative to its upstream.
pub fn get_ahead_behind(
    ctx: &AppContext,
    workspace_id: &str,
    branch_name: &str,
) -> Result<AheadBehind> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    let (ahead, behind) = infra_ahead_behind(&repo, branch_name)?;
    Ok(AheadBehind { ahead, behind })
}

/// Merge a branch into the current HEAD.
pub fn merge_branch(
    ctx: &AppContext,
    workspace_id: &str,
    branch_name: &str,
) -> Result<MergeResult> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_merge_branch(&repo, branch_name)
}

pub fn list_conflicts(ctx: &AppContext, workspace_id: &str) -> Result<Vec<ConflictFile>> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_list_conflicts(&repo)
}

pub fn get_conflict_sides(
    ctx: &AppContext,
    workspace_id: &str,
    path: String,
) -> Result<ConflictSides> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_get_conflict_sides(&repo, &path)
}

pub fn resolve_conflict(
    ctx: &AppContext,
    workspace_id: &str,
    path: String,
    content: String,
) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_resolve_conflict(&repo, &path, &content)
}

pub fn abort_merge(ctx: &AppContext, workspace_id: &str) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_abort_merge(&repo)
}

pub fn merge_in_progress(ctx: &AppContext, workspace_id: &str) -> Result<bool> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    Ok(infra_merge_in_progress(&repo))
}

/// AI explains a conflict — never writes the resolution (P1).
pub async fn explain_conflict(
    ctx: &AppContext,
    workspace_id: String,
    path: String,
) -> Result<String> {
    let ws = get_workspace(ctx, workspace_id.clone())?;
    let settings = ws.settings;
    let provider = settings
        .ai_provider
        .clone()
        .ok_or_else(|| AppError::Protocol("AI provider not configured".into()))?;
    let model = settings
        .ai_model
        .clone()
        .unwrap_or_else(|| match provider.as_str() {
            "anthropic" => "claude-3-5-haiku-latest".into(),
            "ollama" => "llama3.2".into(),
            _ => "gpt-4o-mini".into(),
        });
    let api_key = if provider == "ollama" {
        None
    } else {
        crate::infrastructure::ai::get_api_key(&workspace_id, &provider)?
    };
    let sides = get_conflict_sides(ctx, &workspace_id, path.clone())?;
    let system = settings.prompt_templates.conflict.unwrap_or_else(|| {
        "You explain git merge conflicts for a human developer. \
         Describe what each side intends and suggest a resolution approach. \
         Do NOT output a full rewritten file unless asked. \
         Clearly state this is advice only — the user must apply changes."
            .into()
    });
    let user = format!(
        "Conflict in `{path}`\n\n=== BASE ===\n{}\n\n=== OURS ===\n{}\n\n=== THEIRS ===\n{}\n",
        sides.base.as_deref().unwrap_or("(missing)"),
        sides.ours.as_deref().unwrap_or("(missing)"),
        sides.theirs.as_deref().unwrap_or("(missing)"),
    );
    crate::infrastructure::ai::generate_text(crate::infrastructure::ai::AiGenerateRequest {
        provider,
        model,
        base_url: settings.ai_base_url,
        api_key,
        system,
        user,
    })
    .await
}

/// Rebase the current HEAD onto an upstream.
pub fn rebase_branch(ctx: &AppContext, workspace_id: &str, upstream: &str) -> Result<RebaseResult> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_rebase_branch(&repo, upstream)
}

pub fn plan_interactive_rebase(
    ctx: &AppContext,
    workspace_id: &str,
    upstream: &str,
) -> Result<Vec<InteractiveRebaseTodo>> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_plan_irebase(&repo, upstream)
}

pub fn execute_interactive_rebase(
    ctx: &AppContext,
    workspace_id: &str,
    upstream: &str,
    todos: Vec<InteractiveRebaseTodo>,
) -> Result<InteractiveRebaseResult> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_execute_irebase(&repo, upstream, &todos)
}

pub fn continue_interactive_rebase(
    ctx: &AppContext,
    workspace_id: &str,
) -> Result<InteractiveRebaseResult> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_continue_irebase(&repo)
}

pub fn abort_interactive_rebase_pause(ctx: &AppContext, workspace_id: &str) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_abort_irebase_pause(&repo)
}

pub fn interactive_rebase_paused(ctx: &AppContext, workspace_id: &str) -> Result<bool> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    Ok(infra_irebase_paused(&repo))
}

// ─── Working copy (Sprint 4) ────────────────────────────────────────────────

fn active_repo_id(ctx: &AppContext, workspace_id: &str) -> Result<String> {
    let workspaces = ctx
        .workspaces
        .lock()
        .expect("workspace repo mutex poisoned");
    let ws = workspaces
        .get(workspace_id)?
        .ok_or_else(|| AppError::Protocol(format!("workspace not found: {workspace_id}")))?;
    ws.last_active_repo_id
        .clone()
        .ok_or_else(|| AppError::Protocol("no active repo in workspace".into()))
}

pub fn get_working_copy(ctx: &AppContext, workspace_id: &str) -> Result<WorkingCopy> {
    let repo_id = active_repo_id(ctx, workspace_id)?;
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_wc_status(&repo, &repo_id)
}

pub fn stage_files(ctx: &AppContext, workspace_id: &str, paths: Vec<String>) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_stage_paths(&repo, &paths)
}

pub fn unstage_files(ctx: &AppContext, workspace_id: &str, paths: Vec<String>) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_unstage_paths(&repo, &paths)
}

pub fn stage_all(ctx: &AppContext, workspace_id: &str) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_stage_all(&repo)
}

pub fn commit(ctx: &AppContext, workspace_id: &str, message: String) -> Result<String> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_commit(&repo, &message)
}

/// Discard unstaged worktree changes for the given paths (destructive).
pub fn discard_changes(ctx: &AppContext, workspace_id: &str, paths: Vec<String>) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_discard_worktree_changes(&repo, &paths)
}

/// Append a pattern to the repo-root `.gitignore` (idempotent).
pub fn ignore_path(ctx: &AppContext, workspace_id: &str, pattern: String) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_ignore_path(&repo, &pattern)
}

pub fn fetch(
    ctx: &AppContext,
    workspace_id: &str,
    remote: Option<String>,
    on_progress: Option<Box<dyn Fn(SyncProgress) + Send>>,
) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_fetch(
        &repo,
        remote.as_deref().unwrap_or("origin"),
        crate::infrastructure::git::remote::SyncOperation::Fetch,
        on_progress,
    )
}

pub fn pull(
    ctx: &AppContext,
    workspace_id: &str,
    remote: Option<String>,
    branch: Option<String>,
    rebase: bool,
    stash: bool,
    on_progress: Option<Box<dyn Fn(SyncProgress) + Send>>,
) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let mut repo = ctx.open_repo(&repo_path)?;
    infra_pull_with_options(
        &mut repo,
        remote.as_deref().unwrap_or("origin"),
        PullOptions {
            branch,
            rebase,
            stash,
        },
        on_progress,
    )
}

pub fn list_remotes(ctx: &AppContext, workspace_id: &str) -> Result<Vec<String>> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_list_remotes(&repo)
}

pub fn push(
    ctx: &AppContext,
    workspace_id: &str,
    remote: Option<String>,
    tags: bool,
    force: bool,
    on_progress: Option<Box<dyn Fn(SyncProgress) + Send>>,
) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_push_with_options(
        &repo,
        remote.as_deref().unwrap_or("origin"),
        PushRequest { tags, force },
        on_progress,
    )
}

// ─── Stash (Sprint 5) ───────────────────────────────────────────────────────

pub fn list_stashes(ctx: &AppContext, workspace_id: &str) -> Result<Vec<StashEntry>> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let mut repo = ctx.open_repo(&repo_path)?;
    infra_list_stashes(&mut repo)
}

pub fn save_stash(ctx: &AppContext, workspace_id: &str, message: Option<String>) -> Result<String> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let mut repo = ctx.open_repo(&repo_path)?;
    infra_save_stash(&mut repo, message.as_deref())
}

pub fn apply_stash(ctx: &AppContext, workspace_id: &str, index: u32) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let mut repo = ctx.open_repo(&repo_path)?;
    infra_apply_stash(&mut repo, index as usize)
}

pub fn pop_stash(ctx: &AppContext, workspace_id: &str, index: u32) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let mut repo = ctx.open_repo(&repo_path)?;
    infra_pop_stash(&mut repo, index as usize)
}

pub fn drop_stash(ctx: &AppContext, workspace_id: &str, index: u32) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let mut repo = ctx.open_repo(&repo_path)?;
    infra_drop_stash(&mut repo, index as usize)
}

pub fn get_stash_diff(ctx: &AppContext, workspace_id: &str, oid: &str) -> Result<DiffSummary> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_stash_diff(&repo, oid)
}

// ─── Worktree (Sprint 5) ────────────────────────────────────────────────────

pub fn list_worktrees(ctx: &AppContext, workspace_id: &str) -> Result<Vec<WorktreeInfo>> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_list_worktrees(&repo)
}

pub fn add_worktree(
    ctx: &AppContext,
    workspace_id: &str,
    name: String,
    path: String,
    branch: String,
    create_branch: bool,
) -> Result<WorktreeInfo> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_add_worktree(
        &repo,
        &name,
        PathBuf::from(path).as_path(),
        &branch,
        create_branch,
    )
}

pub fn remove_worktree(ctx: &AppContext, workspace_id: &str, name: String) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_remove_worktree(&repo, &name)
}

// ─── Helper ─────────────────────────────────────────────────────────────────

/// Look up the active repo path for a workspace.
fn active_repo_path(ctx: &AppContext, workspace_id: &str) -> Result<String> {
    let workspaces = ctx
        .workspaces
        .lock()
        .expect("workspace repo mutex poisoned");
    let ws = workspaces.get(workspace_id).and_then(|opt| {
        opt.ok_or_else(|| AppError::Protocol(format!("workspace not found: {workspace_id}")))
    })?;
    let repo_id = ws
        .last_active_repo_id
        .as_ref()
        .ok_or_else(|| AppError::Protocol("no active repo in workspace".into()))?;
    let repos = workspaces.list_repos(workspace_id)?;
    let repo = repos
        .iter()
        .find(|r| r.id.as_str() == repo_id)
        .ok_or_else(|| AppError::Protocol(format!("repo not found: {repo_id}")))?;
    Ok(repo.path.clone())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::infrastructure::persistence::migrations;
    use rusqlite::Connection;
    use std::fs;

    fn fresh_ctx() -> AppContext {
        let conn = Connection::open_in_memory().expect("in-memory sqlite");
        migrations::apply(&conn).expect("migrations");
        AppContext::new(Arc::new(Mutex::new(SqliteWorkspaceRepo::new(conn))))
    }

    fn cleanup(path: &std::path::Path) {
        let _ = fs::remove_dir_all(path);
    }

    #[test]
    fn create_then_list() {
        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).expect("create");
        assert_eq!(ws.name, "Default");
        assert!(ws.id.starts_with("ws-"));
        let list = list_workspaces(&ctx).expect("list");
        assert_eq!(list.len(), 1);
        assert_eq!(list[0].id, ws.id);
    }

    #[test]
    fn empty_name_rejected() {
        let ctx = fresh_ctx();
        let err = create_workspace(&ctx, "   ".into()).expect_err("empty");
        assert_eq!(err.category(), "Protocol");
    }

    #[test]
    fn rename_then_delete() {
        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).unwrap();
        rename_workspace(&ctx, ws.id.clone(), "Renamed".into()).expect("rename");
        let ws_after = ctx.workspaces.lock().unwrap().get(&ws.id).unwrap().unwrap();
        assert_eq!(ws_after.name, "Renamed");
        delete_workspace(&ctx, ws.id.clone()).expect("delete");
        let after = ctx.workspaces.lock().unwrap().get(&ws.id).unwrap();
        assert!(after.is_none());
    }

    #[test]
    fn set_active_repo_roundtrip() {
        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).unwrap();
        set_active_repo(&ctx, ws.id.clone(), Some("r-1".into())).unwrap();
        let after = ctx.workspaces.lock().unwrap().get(&ws.id).unwrap().unwrap();
        assert_eq!(after.last_active_repo_id.as_deref(), Some("r-1"));
        set_active_repo(&ctx, ws.id.clone(), None).unwrap();
        let after = ctx.workspaces.lock().unwrap().get(&ws.id).unwrap().unwrap();
        assert_eq!(after.last_active_repo_id, None);
    }

    // ─── Repo use case tests ─────────────────────────────────────────────

    #[test]
    fn init_repo_creates_and_persists() {
        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).unwrap();
        let tmp = std::env::temp_dir().join(format!("gitwave-uc-init-{}", std::process::id()));
        let _ = fs::remove_dir_all(&tmp);

        let repo =
            init_repo(&ctx, ws.id.clone(), tmp.to_string_lossy().to_string()).expect("init_repo");
        assert_eq!(repo.workspace_id, ws.id);
        assert!(tmp.join(".git").exists());

        let list = list_repos(&ctx, ws.id).expect("list_repos");
        assert_eq!(list.len(), 1);
        assert_eq!(list[0].id, repo.id);

        fs::remove_dir_all(&tmp).ok();
    }

    #[test]
    fn add_local_repo_requires_valid_git_path() {
        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).unwrap();
        let tmp = std::env::temp_dir().join(format!("gitwave-uc-local-{}", std::process::id()));
        let _ = fs::remove_dir_all(&tmp);
        fs::create_dir_all(&tmp).unwrap();

        let err = add_local_repo(&ctx, ws.id.clone(), tmp.to_string_lossy().to_string())
            .expect_err("not a repo");
        assert_eq!(err.category(), "Protocol");

        fs::remove_dir_all(&tmp).ok();
    }

    #[test]
    fn add_local_repo_then_remove() {
        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).unwrap();
        let tmp = std::env::temp_dir().join(format!("gitwave-uc-addremove-{}", std::process::id()));
        let _ = fs::remove_dir_all(&tmp);
        fs::create_dir_all(&tmp).unwrap();
        crate::infrastructure::git::repo_adapter::init(&tmp).expect("init");

        let repo = add_local_repo(&ctx, ws.id.clone(), tmp.to_string_lossy().to_string())
            .expect("add_local_repo");
        assert_eq!(list_repos(&ctx, ws.id.clone()).unwrap().len(), 1);

        remove_repo(&ctx, ws.id.clone(), repo.id).expect("remove_repo");
        assert_eq!(list_repos(&ctx, ws.id).unwrap().len(), 0);

        fs::remove_dir_all(&tmp).ok();
    }

    #[test]
    fn list_repos_marks_missing_when_path_deleted() {
        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).unwrap();
        let tmp = std::env::temp_dir().join(format!("gitwave-uc-missing-{}", std::process::id()));
        let _ = fs::remove_dir_all(&tmp);
        fs::create_dir_all(&tmp).unwrap();
        crate::infrastructure::git::repo_adapter::init(&tmp).expect("init");

        add_local_repo(&ctx, ws.id.clone(), tmp.to_string_lossy().to_string()).expect("add");
        fs::remove_dir_all(&tmp).expect("delete path");

        let list = list_repos(&ctx, ws.id.clone()).expect("list");
        assert_eq!(list.len(), 1);
        assert_eq!(list[0].status, RepoStatus::Missing);

        // Restore path → list should flip back to Active
        fs::create_dir_all(&tmp).unwrap();
        crate::infrastructure::git::repo_adapter::init(&tmp).expect("re-init");
        let list = list_repos(&ctx, ws.id).expect("list after restore");
        assert_eq!(list[0].status, RepoStatus::Active);

        fs::remove_dir_all(&tmp).ok();
    }

    #[test]
    fn relink_repo_updates_path() {
        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).unwrap();
        let old = std::env::temp_dir().join(format!("gitwave-uc-old-{}", std::process::id()));
        let new_p = std::env::temp_dir().join(format!("gitwave-uc-new-{}", std::process::id()));
        let _ = fs::remove_dir_all(&old);
        let _ = fs::remove_dir_all(&new_p);
        fs::create_dir_all(&old).unwrap();
        crate::infrastructure::git::repo_adapter::init(&old).expect("init old");
        fs::create_dir_all(&new_p).unwrap();
        crate::infrastructure::git::repo_adapter::init(&new_p).expect("init new");

        let repo = add_local_repo(&ctx, ws.id.clone(), old.to_string_lossy().to_string())
            .expect("add_local_repo");
        relink_repo(
            &ctx,
            ws.id.clone(),
            repo.id.clone(),
            new_p.to_string_lossy().to_string(),
        )
        .expect("relink");

        let list = list_repos(&ctx, ws.id).unwrap();
        assert_eq!(list[0].path, new_p.to_string_lossy().to_string());

        fs::remove_dir_all(&old).ok();
        fs::remove_dir_all(&new_p).ok();
    }

    // ─── History / diff / blame use case tests ──────────────────────────

    #[test]
    fn get_commit_log_linear_repo() {
        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).unwrap();
        let tmp = std::env::temp_dir().join(format!("gitwave-history-{}", std::process::id()));
        let _ = fs::remove_dir_all(&tmp);

        // Set up a workspace with an active repo
        init_repo(&ctx, ws.id.clone(), tmp.to_string_lossy().to_string()).unwrap();
        let repos = list_repos(&ctx, ws.id.clone()).unwrap();
        let repo_ref = &repos[0];
        set_active_repo(&ctx, ws.id.clone(), Some(repo_ref.id.clone())).unwrap();

        // Build commits using git2 directly
        let git_repo = ctx.open_repo(&tmp.to_string_lossy()).unwrap();
        let sig = git2::Signature::now("Test", "test@local").unwrap();
        for i in 0..3u32 {
            let path = tmp.join(format!("file{i}.txt"));
            fs::write(&path, format!("v{i}\n")).unwrap();
            let mut index = git_repo.index().unwrap();
            index
                .add_path(std::path::Path::new(&format!("file{i}.txt")))
                .unwrap();
            let tree_oid = index.write_tree().unwrap();
            let tree = git_repo.find_tree(tree_oid).unwrap();
            let parent = if i == 0 {
                None
            } else {
                Some(git_repo.head().unwrap().peel_to_commit().unwrap())
            };
            let parents: Vec<&git2::Commit> = parent.iter().collect();
            git_repo
                .commit(
                    Some("HEAD"),
                    &sig,
                    &sig,
                    &format!("commit {i}"),
                    &tree,
                    &parents,
                )
                .unwrap();
        }

        let log = get_commit_log(&ctx, &ws.id, 10).expect("get_commit_log");
        cleanup(&tmp);

        assert_eq!(log.len(), 3, "expected 3 commits");
        for c in &log {
            assert!(!c.sha.is_empty());
        }
    }
}
