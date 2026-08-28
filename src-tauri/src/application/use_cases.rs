//! Application use cases — workspace, repo, SSH, history, diff, blame, and branch.
//!
//! See `docs/tasks/feat-history-graph/plan.md` steps 6-10.

use std::path::PathBuf;
use std::sync::{Arc, Mutex};

use crate::domain::blame::BlameLine;
use crate::domain::branch::BranchInfo;
use crate::domain::diff::{DiffLineKind, FileDiff};
use crate::domain::error::{AppError, Result};
use crate::domain::history::{CommitDetails, CommitSummary};
use crate::domain::stash::StashEntry;
use crate::domain::working_copy::WorkingCopy;
use crate::domain::workspace::{
    RepoRef, RepoStatus, Workspace, WorkspaceSettings, WorkspaceSummary,
};
use crate::domain::worktree::WorktreeInfo;
use crate::infrastructure::ai::ProviderAttempt;
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
    diff_index_to_head as infra_diff_index_to_head,
    diff_index_to_head_files as infra_diff_index_to_head_files, diff_paths as infra_diff_paths,
    diff_workdir_to_index as infra_diff_workdir_to_index, DiffSummary,
};
use crate::infrastructure::git::health::{collect_health as infra_collect_health, HealthReport};
use crate::infrastructure::git::history::{
    ahead_behind as infra_ahead_behind, commit_details as infra_commit_details,
    commit_log as infra_commit_log, commit_recent_messages as infra_commit_recent_messages,
    list_branches as infra_list_branches,
};
use crate::infrastructure::git::interactive_rebase::{
    abort_interactive_rebase_pause as infra_abort_irebase_pause,
    continue_interactive_rebase as infra_continue_irebase,
    execute_interactive_rebase as infra_execute_irebase,
    interactive_rebase_paused as infra_irebase_paused,
    plan_interactive_rebase as infra_plan_irebase, InteractiveRebaseResult, InteractiveRebaseTodo,
};
use crate::infrastructure::git::merge::{
    merge_branch as infra_merge_branch, merge_preview as infra_merge_preview, MergePreview,
    MergeResult,
};
use crate::infrastructure::git::rebase::{rebase_branch as infra_rebase_branch, RebaseResult};
use crate::infrastructure::git::reflog::{list_reflog as infra_list_reflog, ReflogEntry};
use crate::infrastructure::git::remote::{
    add_remote as infra_add_remote, list_remote_details as infra_list_remote_details,
    remove_remote as infra_remove_remote, rename_remote as infra_rename_remote,
    set_remote_push_url as infra_set_remote_push_url, set_remote_url as infra_set_remote_url,
    RemoteInfo,
};
use crate::infrastructure::git::remote::{
    delete_remote_branch as infra_delete_remote_branch, fetch as infra_fetch,
    list_remotes as infra_list_remotes, pull_with_options as infra_pull_with_options,
    push_with_options as infra_push_with_options, PullOptions, PushRequest, SyncProgress,
};
use crate::infrastructure::git::revert::{
    cherry_pick_commit as infra_cherry_pick_commit, revert_commit as infra_revert_commit,
};
use crate::infrastructure::git::stash::{
    apply_stash as infra_apply_stash, drop_stash as infra_drop_stash,
    list_stashes as infra_list_stashes, pop_stash as infra_pop_stash,
    save_stash as infra_save_stash, stash_diff as infra_stash_diff,
};
use crate::infrastructure::git::submodule::{
    init_submodule as infra_submodule_init, list_submodules as infra_list_submodules,
    update_submodule as infra_submodule_update,
};
use crate::infrastructure::git::tag::{
    create_tag as infra_create_tag, delete_tag as infra_delete_tag, list_tags as infra_list_tags,
};
use crate::infrastructure::git::working_copy::{
    commit as infra_commit, discard_worktree_changes as infra_discard_worktree_changes,
    ignore_path as infra_ignore_path, reset_head_hard as infra_reset_head_hard,
    stage_all as infra_stage_all, stage_paths as infra_stage_paths, status as infra_wc_status,
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
    let chain = ai_chain(&settings, &workspace_id)?;
    let primary = &chain[0];

    let repo_path = active_repo_path(ctx, &workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    let staged = infra_diff_index_to_head(&repo)?;
    if staged.files.is_empty() {
        return Err(AppError::Protocol(
            "no staged changes — stage files before generating a commit message".into(),
        ));
    }
    let recent = infra_commit_log(&repo, 8, None)?;

    let mut user = String::new();
    user.push_str("Recent commits (newest first):\n");
    for c in &recent {
        user.push_str(&format!("- {}\n", c.message_summary));
    }
    // Full messages of the branch's last 3 commits so the model can mirror
    // the repo's actual style (language, body formatting).
    user.push_str("\nLast 3 commit messages on this branch (style reference):\n");
    for msg in infra_commit_recent_messages(&repo, 3)? {
        user.push_str("---\n");
        user.push_str(&msg);
        user.push_str("\n---\n");
    }
    user.push_str("\nStaged changes:\n");
    append_diff_summary(&mut user, &staged);

    // The model must see WHAT changed, not just which files — file names
    // alone make it hallucinate plausible-sounding but wrong subjects (the
    // Aug 2026 "word wrap toggle" incident). Capped so huge diffs cannot
    // blow the context.
    let staged_files = infra_diff_index_to_head_files(&repo)?;
    user.push_str("\nStaged diff (unified format, may be truncated):\n");
    append_diff_patch(&mut user, &staged_files, 12_000);

    let system = settings.prompt_templates.commit.unwrap_or_else(|| {
        "You write concise git commit messages. Output ONLY the message text. \
             Prefer conventional commits (type: summary). First line <= 72 chars. \
             Base the message strictly on the provided staged diff — do not invent \
             changes that are not visible in it. \
             Do not wrap in markdown fences."
            .into()
    });

    crate::infrastructure::ai::generate_text(crate::infrastructure::ai::AiGenerateRequest {
        provider: primary.provider.clone(),
        model: primary.model.clone(),
        base_url: primary.base_url.clone(),
        api_key: primary.api_key.clone(),
        system,
        user,
        fallbacks: chain[1..].to_vec(),
    })
    .await
}

/// Provider-appropriate default model when the user has not configured one.
fn default_model(provider: &str) -> String {
    match provider {
        "anthropic" => "claude-3-5-haiku-latest".into(),
        "ollama" => "llama3.2".into(),
        _ => "gpt-4o-mini".into(),
    }
}

/// Ordered provider ids for AI calls: primary `ai_provider` then the
/// `ai_providers` fallback chain (deduped). Offline mode keeps only Ollama.
fn provider_chain_ids(settings: &WorkspaceSettings) -> Vec<String> {
    let mut ids: Vec<String> = Vec::new();
    if let Some(p) = settings.ai_provider.as_deref() {
        let p = p.trim();
        if !p.is_empty() {
            ids.push(p.to_string());
        }
    }
    for f in &settings.ai_providers {
        let f = f.trim();
        if !f.is_empty() && !ids.iter().any(|i| i == f) {
            ids.push(f.to_string());
        }
    }
    if settings.ai_offline {
        ids.retain(|p| p == "ollama");
    }
    ids
}

/// Resolve the chain into attempts with per-provider keychain credentials.
/// Cloud providers without a stored key are skipped (they cannot succeed).
/// The primary keeps the workspace base-url override; fallbacks use each
/// provider's default endpoint.
fn ai_chain(settings: &WorkspaceSettings, workspace_id: &str) -> Result<Vec<ProviderAttempt>> {
    let ids = provider_chain_ids(settings);
    if ids.is_empty() && settings.ai_offline {
        return Err(AppError::Protocol(
            "offline mode is enabled — configure local Ollama to use AI features".into(),
        ));
    }
    if ids.is_empty() {
        return Err(AppError::Protocol(
            "no usable AI provider configured (check provider, key and offline settings)".into(),
        ));
    }

    let mut out = Vec::new();
    for provider in ids {
        let is_primary = settings
            .ai_provider
            .as_deref()
            .map(|p| p.eq_ignore_ascii_case(&provider))
            .unwrap_or(false);
        let api_key = if provider == "ollama" {
            None
        } else {
            match crate::infrastructure::ai::get_api_key(workspace_id, &provider) {
                Ok(Some(k)) if !k.is_empty() => Some(k),
                _ => continue,
            }
        };
        // The user's base-url override and configured model belong to the
        // provider they were set for; fallback attempts use their own
        // defaults (model namespaces do not mix across vendors).
        let base_url = if is_primary {
            settings.ai_base_url.clone()
        } else {
            None
        };
        let model = if is_primary {
            settings
                .ai_model
                .clone()
                .unwrap_or_else(|| default_model(&provider))
        } else {
            default_model(&provider)
        };
        out.push(ProviderAttempt {
            provider,
            base_url,
            api_key,
            model,
        });
    }
    if out.is_empty() {
        return Err(AppError::Protocol(
            "no usable AI provider configured (check provider, key and offline settings)".into(),
        ));
    }
    Ok(out)
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

/// Render the staged files as unified-diff text, stopping at `budget`
/// characters. Files without hunks (binary / mode-only) are skipped — the
/// summary section already lists them.
fn append_diff_patch(buf: &mut String, files: &[FileDiff], budget: usize) {
    let mut written = 0usize;
    let mut truncated = false;
    'outer: for file in files {
        if file.hunks.is_empty() {
            continue;
        }
        if written + file.path.len() * 2 + 16 > budget {
            truncated = true;
            break;
        }
        buf.push_str(&format!("\n--- a/{}\n+++ b/{}\n", file.path, file.path));
        written += file.path.len() * 2 + 16;
        for hunk in &file.hunks {
            let header = format!(
                "@@ -{},{} +{},{} @@\n",
                hunk.old_start, hunk.old_lines, hunk.new_start, hunk.new_lines
            );
            if written + header.len() > budget {
                truncated = true;
                break 'outer;
            }
            buf.push_str(&header);
            written += header.len();
            for line in &hunk.lines {
                let sign = match line.kind {
                    DiffLineKind::Added => '+',
                    DiffLineKind::Removed => '-',
                    DiffLineKind::Context => ' ',
                };
                if written + line.content.len() + 1 > budget {
                    truncated = true;
                    break 'outer;
                }
                buf.push(sign);
                buf.push_str(&line.content);
                buf.push('\n');
                written += line.content.len() + 1;
            }
        }
    }
    if truncated {
        buf.push_str("\n[diff truncated due to size]\n");
    }
}

// ─── History use cases (Sprint 3) ───────────────────────────────────────────

/// Get the commit log for the active repo in a workspace.
pub fn get_commit_log(
    ctx: &AppContext,
    workspace_id: &str,
    max: u32,
    filter: Option<String>,
) -> Result<Vec<CommitSummary>> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_commit_log(&repo, max, filter.as_deref())
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

/// Full details for one commit (inspector header): message, author, files.
pub fn get_commit_details(
    ctx: &AppContext,
    workspace_id: &str,
    commit_oid: &str,
) -> Result<CommitDetails> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_commit_details(&repo, commit_oid)
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
    no_ff: bool,
) -> Result<MergeResult> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_merge_branch(&repo, branch_name, no_ff)
}

/// Dry-run a merge for the confirmation dialog (no side effects).
pub fn merge_preview(
    ctx: &AppContext,
    workspace_id: &str,
    branch_name: &str,
) -> Result<MergePreview> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_merge_preview(&repo, branch_name)
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
    let chain = ai_chain(&settings, &workspace_id)?;
    let primary = &chain[0];
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
        provider: primary.provider.clone(),
        model: primary.model.clone(),
        base_url: primary.base_url.clone(),
        api_key: primary.api_key.clone(),
        system,
        user,
        fallbacks: chain[1..].to_vec(),
    })
    .await
}

/// Commit subject for a reflog position, or a placeholder when the oid
/// is zero/unresolvable (branch creation, pruned commits).
fn subject_of(repo: &git2::Repository, oid: &str) -> String {
    git2::Oid::from_str(oid)
        .ok()
        .and_then(|o| repo.find_commit(o).ok())
        .and_then(|c| c.summary().map(str::to_string))
        .unwrap_or_else(|| "(not resolvable — maybe zero oid or pruned)".into())
}

/// Rebase the current HEAD onto an upstream.
pub fn rebase_branch(ctx: &AppContext, workspace_id: &str, upstream: &str) -> Result<RebaseResult> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_rebase_branch(&repo, upstream)
}

/// Revert a single commit on the current branch (user-initiated; creates a
/// `Revert "…"` commit — this is the operation's git semantics, not auto-commit).
pub fn revert_commit(ctx: &AppContext, workspace_id: &str, oid: &str) -> Result<String> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_revert_commit(&repo, oid)
}

/// Cherry-pick a single commit onto the current branch.
pub fn cherry_pick_commit(ctx: &AppContext, workspace_id: &str, oid: &str) -> Result<String> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_cherry_pick_commit(&repo, oid)
}

/// Remotes with URLs (M1).
pub fn list_remote_details(ctx: &AppContext, workspace_id: &str) -> Result<Vec<RemoteInfo>> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_list_remote_details(&repo)
}

/// `git remote add` (errors when the name already exists).
pub fn add_remote(ctx: &AppContext, workspace_id: &str, name: &str, url: &str) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_add_remote(&repo, name, url)
}

/// `git remote set-url`.
pub fn set_remote_url(ctx: &AppContext, workspace_id: &str, name: &str, url: &str) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_set_remote_url(&repo, name, url)
}

/// `git remote set-url --push`.
pub fn set_remote_push_url(
    ctx: &AppContext,
    workspace_id: &str,
    name: &str,
    url: Option<String>,
) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_set_remote_push_url(&repo, name, url.as_deref())
}

/// `git remote rename`.
pub fn rename_remote(
    ctx: &AppContext,
    workspace_id: &str,
    name: &str,
    new_name: &str,
) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_rename_remote(&repo, name, new_name)
}

/// `git remote remove`.
pub fn remove_remote(ctx: &AppContext, workspace_id: &str, name: &str) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_remove_remote(&repo, name)
}

/// Full reflog of `reference` (HEAD or branch shorthand), newest first.
/// Read-only foundation for the M2 recovery UI.
pub fn list_reflog(
    ctx: &AppContext,
    workspace_id: &str,
    reference: Option<String>,
) -> Result<Vec<ReflogEntry>> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_list_reflog(&repo, reference.as_deref().unwrap_or("HEAD"))
}

/// Deterministic repo health metrics (M3).
pub fn get_health(ctx: &AppContext, workspace_id: &str) -> Result<HealthReport> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_collect_health(&repo, 30)
}

/// AI summary of the health report (advice only, P1).
pub async fn explain_health(ctx: &AppContext, workspace_id: String) -> Result<String> {
    let ws = get_workspace(ctx, workspace_id.clone())?;
    let settings = ws.settings;
    let chain = ai_chain(&settings, &workspace_id)?;
    let primary = &chain[0];

    let report = get_health(ctx, &workspace_id)?;
    let user = format!(
        "Repo health metrics (JSON):
{}

Write a short health assessment:          what looks fine, what needs attention, and the single most          valuable next action. Plain text, no markdown fences.",
        serde_json::to_string_pretty(&report)
            .map_err(|e| AppError::Unknown(format!("serialize report: {e}")))?,
    );
    let system = settings.prompt_templates.health.clone().unwrap_or_else(|| {
        "You are a repository health assistant. You receive deterministic          metrics about a git repository and summarize them for a developer.          Advice only — you never execute anything."
            .into()
    });

    crate::infrastructure::ai::generate_text(crate::infrastructure::ai::AiGenerateRequest {
        provider: primary.provider.clone(),
        model: primary.model.clone(),
        base_url: primary.base_url.clone(),
        api_key: primary.api_key.clone(),
        system,
        user,
        fallbacks: chain[1..].to_vec(),
    })
    .await
}

/// `git reset --hard <oid>` on the current branch — M2 recovery action,
/// always behind an explicit user confirmation upstream (P1).
pub fn reset_hard(ctx: &AppContext, workspace_id: &str, oid: &str) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_reset_head_hard(&repo, oid)
}

/// AI explanation of a reflog event + recovery advice (advice only, P1).
pub async fn explain_reflog(
    ctx: &AppContext,
    workspace_id: String,
    old_oid: String,
    new_oid: String,
    action: String,
    message: String,
) -> Result<String> {
    let ws = get_workspace(ctx, workspace_id.clone())?;
    let settings = ws.settings;
    let chain = ai_chain(&settings, &workspace_id)?;
    let primary = &chain[0];

    let repo_path = active_repo_path(ctx, &workspace_id)?;
    // git2::Repository is !Send — resolve subjects inside a scope so it is
    // dropped before the network await.
    let (old_subject, new_subject) = {
        let repo = ctx.open_repo(&repo_path)?;
        (subject_of(&repo, &old_oid), subject_of(&repo, &new_oid))
    };

    let system = settings.prompt_templates.reflog.clone().unwrap_or_else(|| {
        "You are a git recovery assistant. A single reflog entry is provided.          In 2-4 sentences: explain what happened to the branch, then give one          concrete recovery recommendation (create a recovery branch at a sha,          git reset --hard, or checkout). Advice only — you never execute          anything. Do not wrap in markdown fences."
            .into()
    });
    let user = format!(
        "Reflog entry\nAction: {action}\nMessage: {message}\n\nPrevious position: {old}\n  ({old_subject})\n\nNew position: {new}\n  ({new_subject})\n",
        old = old_oid,
        new = new_oid,
    );

    crate::infrastructure::ai::generate_text(crate::infrastructure::ai::AiGenerateRequest {
        provider: primary.provider.clone(),
        model: primary.model.clone(),
        base_url: primary.base_url.clone(),
        api_key: primary.api_key.clone(),
        system,
        user,
        fallbacks: chain[1..].to_vec(),
    })
    .await
}

/// All tags in the active repo (S3).
pub fn list_tags(
    ctx: &AppContext,
    workspace_id: &str,
) -> Result<Vec<crate::infrastructure::git::tag::TagInfo>> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_list_tags(&repo)
}

/// Create a tag on `target_oid` (None = HEAD); `message` makes it annotated.
pub fn create_tag(
    ctx: &AppContext,
    workspace_id: &str,
    name: &str,
    target_oid: Option<String>,
    message: Option<String>,
) -> Result<String> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_create_tag(&repo, name, target_oid.as_deref(), message.as_deref())
}

/// Delete a tag by short name.
pub fn delete_tag(ctx: &AppContext, workspace_id: &str, name: &str) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_delete_tag(&repo, name)
}

/// List submodules of the active repo (S1).
pub fn list_submodules(
    ctx: &AppContext,
    workspace_id: &str,
) -> Result<Vec<crate::infrastructure::git::submodule::SubmoduleInfo>> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_list_submodules(&repo)
}

/// `git submodule init <name>`.
pub fn init_submodule(ctx: &AppContext, workspace_id: &str, name: &str) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_submodule_init(&repo, name)
}

/// `git submodule update --init <name>` (clone + checkout the worktree).
pub fn update_submodule(ctx: &AppContext, workspace_id: &str, name: &str) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_submodule_update(&repo, name)
}

/// Read the repo-root `.gitignore` (empty string when absent) — S2 editor.
pub fn get_gitignore(ctx: &AppContext, workspace_id: &str) -> Result<String> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let path = std::path::Path::new(&repo_path).join(".gitignore");
    if !path.exists() {
        return Ok(String::new());
    }
    std::fs::read_to_string(&path).map_err(|e| AppError::Unknown(format!("read .gitignore: {e}")))
}

/// Overwrite the repo-root `.gitignore` (S2 editor). Trailing newline is
/// normalized on so appended ignore patterns from the UI keep working.
pub fn write_gitignore(ctx: &AppContext, workspace_id: &str, content: &str) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let path = std::path::Path::new(&repo_path).join(".gitignore");
    let normalized = if content.ends_with('\n') || content.is_empty() {
        content.to_string()
    } else {
        format!("{content}\n")
    };
    std::fs::write(&path, normalized)
        .map_err(|e| AppError::Unknown(format!("write .gitignore: {e}")))
}

/// Payload shape of a `.gitwave-workspace.json` transfer file (S6).
#[derive(serde::Serialize, serde::Deserialize)]
pub struct WorkspaceTransfer {
    pub version: u32,
    pub name: String,
    /// Repo filesystem paths. Keys/API keys are intentionally excluded —
    /// secrets never leave the machine through import/export.
    pub repos: Vec<String>,
}

/// Export a workspace (name + repo paths) as a transfer JSON file (S6).
pub fn export_workspace(ctx: &AppContext, workspace_id: &str, dest_path: &str) -> Result<String> {
    let ws = get_workspace(ctx, workspace_id.to_string())?;
    let transfer = WorkspaceTransfer {
        version: 1,
        name: ws.name.clone(),
        repos: ws.repos.iter().map(|r| r.path.clone()).collect(),
    };
    let json = serde_json::to_string_pretty(&transfer)
        .map_err(|e| AppError::Unknown(format!("serialize workspace: {e}")))?;
    std::fs::write(dest_path, json)
        .map_err(|e| AppError::Unknown(format!("write transfer file: {e}")))?;
    Ok(dest_path.to_string())
}

/// Import a transfer JSON file: creates a new workspace and re-adds every
/// repo path that currently exists on disk. Missing paths are skipped and
/// reported so the user can relink them later (W3 relink covers that).
pub fn import_workspace(
    ctx: &AppContext,
    src_path: &str,
    new_name: Option<String>,
) -> Result<WorkspaceSummary> {
    let raw = std::fs::read_to_string(src_path)
        .map_err(|e| AppError::Unknown(format!("read transfer file: {e}")))?;
    let transfer: WorkspaceTransfer = serde_json::from_str(&raw)
        .map_err(|e| AppError::Protocol(format!("invalid transfer file: {e}")))?;
    if transfer.version != 1 {
        return Err(AppError::Protocol(format!(
            "unsupported transfer file version: {}",
            transfer.version
        )));
    }
    let name = new_name
        .map(|n| n.trim().to_string())
        .filter(|n| !n.is_empty())
        .unwrap_or(transfer.name.clone());
    let ws = create_workspace(ctx, name)?;

    let mut missing = 0usize;
    for path in &transfer.repos {
        if std::path::Path::new(path).join(".git").exists()
            && add_local_repo(ctx, ws.id.clone(), path.clone()).is_ok()
        {
            continue;
        }
        missing += 1;
    }
    if missing > 0 {
        eprintln!("workspace import skipped {missing} missing repo path(s)");
    }
    list_workspaces(ctx)?
        .into_iter()
        .find(|sum| sum.id == ws.id)
        .ok_or_else(|| AppError::Unknown("imported workspace vanished".into()))
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
    use crate::domain::diff::{DiffHunk, DiffLine};
    use crate::infrastructure::persistence::migrations;
    use rusqlite::Connection;
    use std::fs;

    fn patch_fixture(long_content: Option<String>) -> Vec<FileDiff> {
        let content = long_content.unwrap_or_else(|| "new".into());
        vec![FileDiff {
            path: "a.txt".into(),
            old_sha: None,
            new_sha: None,
            additions: 1,
            deletions: 1,
            hunks: vec![DiffHunk {
                old_start: 1,
                old_lines: 2,
                new_start: 1,
                new_lines: 2,
                lines: vec![
                    DiffLine {
                        kind: DiffLineKind::Context,
                        content: "keep".into(),
                        old_line_no: Some(1),
                        new_line_no: Some(1),
                    },
                    DiffLine {
                        kind: DiffLineKind::Removed,
                        content: "old".into(),
                        old_line_no: Some(2),
                        new_line_no: None,
                    },
                    DiffLine {
                        kind: DiffLineKind::Added,
                        content,
                        old_line_no: None,
                        new_line_no: Some(2),
                    },
                ],
            }],
            staged: None,
        }]
    }

    #[test]
    fn diff_patch_renders_unified_lines() {
        let mut buf = String::new();
        append_diff_patch(&mut buf, &patch_fixture(None), 10_000);
        assert!(buf.contains("--- a/a.txt\n+++ b/a.txt\n"));
        assert!(buf.contains("@@ -1,2 +1,2 @@\n"));
        assert!(buf.contains("\n keep\n") && buf.contains("\n-old\n") && buf.contains("\n+new\n"));
        assert!(!buf.contains("truncated"));
    }

    #[test]
    fn diff_patch_truncates_at_budget() {
        let mut buf = String::new();
        append_diff_patch(&mut buf, &patch_fixture(Some("x".repeat(500))), 40);
        assert!(buf.contains("[diff truncated due to size]"));
        assert!(buf.len() < 500, "budget must bound output: {}", buf.len());
    }

    #[test]
    fn provider_chain_dedups_and_respects_offline() {
        let settings = WorkspaceSettings {
            ai_provider: Some("openai".into()),
            ai_providers: vec![
                "anthropic".into(),
                "openai".into(),
                "ollama".into(),
                "".into(),
            ],
            ..WorkspaceSettings::default()
        };
        let ids = provider_chain_ids(&settings);
        assert_eq!(
            ids,
            vec![
                "openai".to_string(),
                "anthropic".to_string(),
                "ollama".to_string()
            ]
        );

        let offline = WorkspaceSettings {
            ai_offline: true,
            ..settings
        };
        assert_eq!(provider_chain_ids(&offline), vec!["ollama".to_string()]);
    }

    #[test]
    fn ai_chain_offline_without_ollama_errors_deterministically() {
        let settings = WorkspaceSettings {
            ai_provider: Some("openai".into()),
            ai_offline: true,
            ..WorkspaceSettings::default()
        };
        let err = ai_chain(&settings, "ws-offline").unwrap_err();
        assert!(err.to_string().contains("offline"), "got: {err}");
    }

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

        let log = get_commit_log(&ctx, &ws.id, 10, None).expect("get_commit_log");
        cleanup(&tmp);

        assert_eq!(log.len(), 3, "expected 3 commits");
        for c in &log {
            assert!(!c.sha.is_empty());
        }
    }
}
