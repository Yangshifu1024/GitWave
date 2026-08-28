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
    for fb in &settings.ai_failover {
        if !matches!(fb.provider.as_str(), "openai" | "anthropic" | "ollama") {
            return Err(AppError::Protocol(format!(
                "unsupported ai_failover provider: {}",
                fb.provider
            )));
        }
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

// ─── AI provider chain (failover) ───────────────────────────────────────────

/// One resolved attempt in the AI provider chain: the workspace primary
/// first, then its configured failover entries in order.
#[derive(Debug, Clone, serde::Serialize)]
pub struct ResolvedAiProvider {
    pub provider: String,
    pub model: String,
    pub base_url: Option<String>,
    pub api_key: Option<String>,
}

/// Result of a chain run — the text plus which provider produced it, so
/// the UI can tell the user when a fallback served the request.
#[derive(Debug, Clone, serde::Serialize)]
pub struct AiGenerateOutcome {
    pub text: String,
    pub provider_used: String,
    pub used_fallback: bool,
}

/// Key lookup abstraction so chain resolution is testable without touching
/// the OS keychain; production passes `ai::get_api_key` partially applied.
pub type AiKeyLookup<'a> = dyn Fn(&str) -> Result<Option<String>> + 'a;

pub fn default_ai_model(provider: &str) -> &'static str {
    match provider {
        "anthropic" => "claude-3-5-haiku-latest",
        "ollama" => "llama3.2",
        _ => "gpt-4o-mini",
    }
}

/// Failover policy: only network-level failures (unreachable host, HTTP
/// status, rate limits) move to the next provider. Content and
/// configuration errors stop the chain — the next provider would make the
/// same mistake on the same prompt.
fn should_failover(err: &AppError) -> bool {
    matches!(err, AppError::Network(_))
}

/// Resolve the primary + failover entries into concrete attempts. Cloud
/// entries whose API key cannot be resolved are skipped (they can never
/// succeed); a missing key on the *primary* is a hard error, matching the
/// pre-failover behavior. In offline mode (PM 1.6) the chain keeps only
/// local Ollama entries.
fn resolve_ai_chain(
    settings: &WorkspaceSettings,
    key_lookup: &AiKeyLookup,
) -> Result<Vec<ResolvedAiProvider>> {
    let resolve = |provider: &str,
                   model: &Option<String>,
                   base_url: &Option<String>|
     -> Result<ResolvedAiProvider> {
        Ok(ResolvedAiProvider {
            provider: provider.to_string(),
            model: model
                .clone()
                .filter(|m| !m.trim().is_empty())
                .unwrap_or_else(|| default_ai_model(provider).into()),
            base_url: base_url.clone(),
            api_key: if provider == "ollama" {
                None
            } else {
                key_lookup(provider)?
            },
        })
    };

    let primary = settings
        .ai_provider
        .clone()
        .filter(|p| !p.trim().is_empty())
        .ok_or_else(|| {
            AppError::Protocol("AI provider not configured for this Workspace".into())
        })?;
    let head = resolve(&primary, &settings.ai_model, &settings.ai_base_url)?;
    if head.provider != "ollama" && head.api_key.is_none() {
        return Err(AppError::Credential(format!(
            "{primary} API key not configured"
        )));
    }

    let mut chain = vec![head];
    for fb in &settings.ai_failover {
        if fb.provider.trim().is_empty() {
            continue;
        }
        match resolve(&fb.provider, &fb.model, &fb.base_url) {
            Ok(entry) => {
                if entry.provider != "ollama" && entry.api_key.is_none() {
                    continue;
                }
                chain.push(entry);
            }
            // A keychain failure on an optional fallback just drops that
            // entry; the chain still serves the request if it can.
            Err(_) => continue,
        }
    }

    if settings.ai_offline {
        chain.retain(|p| p.provider == "ollama");
        if chain.is_empty() {
            return Err(AppError::Protocol(
                "offline mode is enabled — cloud AI calls are disabled (use Ollama or turn it off in AI settings)"
                    .into(),
            ));
        }
    }
    Ok(chain)
}

/// Run the chain in order; the first non-network outcome wins. Network
/// failures walk to the next entry; when the chain is exhausted the last
/// error is raised.
async fn generate_with_failover(
    chain: Vec<ResolvedAiProvider>,
    system: String,
    user: String,
) -> Result<AiGenerateOutcome> {
    let total = chain.len();
    let mut last_err: Option<AppError> = None;
    for (i, entry) in chain.into_iter().enumerate() {
        tracing::info!(
            provider = %entry.provider,
            model = %entry.model,
            attempt = i + 1,
            total,
            "ai generate"
        );
        let req = crate::infrastructure::ai::AiGenerateRequest {
            provider: entry.provider.clone(),
            model: entry.model,
            base_url: entry.base_url,
            api_key: entry.api_key,
            system: system.clone(),
            user: user.clone(),
        };
        match crate::infrastructure::ai::generate_text(req).await {
            Ok(text) => {
                return Ok(AiGenerateOutcome {
                    text,
                    provider_used: entry.provider,
                    used_fallback: i > 0,
                });
            }
            Err(e) if should_failover(&e) => {
                tracing::warn!(provider = %entry.provider, error = %e, "ai provider failed, trying next");
                last_err = Some(e);
            }
            Err(e) => return Err(e),
        }
    }
    Err(last_err.expect("resolve_ai_chain never returns an empty chain"))
}

/// Generate a commit message suggestion from staged/workdir diff + recent commits.
/// Result is always returned for the user to edit — never auto-commits (P1).
pub async fn generate_commit_message(
    ctx: &AppContext,
    workspace_id: String,
) -> Result<AiGenerateOutcome> {
    let ws = get_workspace(ctx, workspace_id.clone())?;
    let settings = ws.settings;
    let key_lookup =
        |provider: &str| crate::infrastructure::ai::get_api_key(&workspace_id, provider);
    let chain = resolve_ai_chain(&settings, &key_lookup)?;

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

    generate_with_failover(chain, system, user).await
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
) -> Result<AiGenerateOutcome> {
    let ws = get_workspace(ctx, workspace_id.clone())?;
    let settings = ws.settings;
    let key_lookup =
        |provider: &str| crate::infrastructure::ai::get_api_key(&workspace_id, provider);
    let chain = resolve_ai_chain(&settings, &key_lookup)?;
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
    generate_with_failover(chain, system, user).await
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

    // ── AI provider chain ──

    use crate::domain::workspace::AiProviderConfig;

    fn chain_settings(
        primary: Option<&str>,
        failover: Vec<AiProviderConfig>,
        offline: bool,
    ) -> WorkspaceSettings {
        WorkspaceSettings {
            ai_provider: primary.map(str::to_string),
            ai_failover: failover,
            ai_offline: offline,
            ..WorkspaceSettings::default()
        }
    }

    fn key_lookup_for(keys: &[(&str, Option<&str>)]) -> impl Fn(&str) -> Result<Option<String>> {
        let keys: std::collections::HashMap<String, Option<String>> = keys
            .iter()
            .map(|(p, k)| (p.to_string(), k.map(str::to_string)))
            .collect();
        move |provider: &str| {
            keys.get(provider)
                .cloned()
                .ok_or_else(|| AppError::Unknown("keychain unavailable".into()))
        }
    }

    #[test]
    fn failover_policy_only_walks_network_errors() {
        assert!(should_failover(&AppError::Network("timeout".into())));
        assert!(!should_failover(&AppError::Protocol("bad prompt".into())));
        assert!(!should_failover(&AppError::Credential("no key".into())));
        assert!(!should_failover(&AppError::Unknown("empty content".into())));
    }

    #[test]
    fn ai_chain_resolves_primary_then_reachable_failovers() {
        let settings = chain_settings(
            Some("openai"),
            vec![
                AiProviderConfig {
                    provider: "anthropic".into(),
                    model: Some("claude-3-5-haiku-latest".into()),
                    base_url: None,
                },
                AiProviderConfig {
                    provider: "ollama".into(),
                    model: None,
                    base_url: Some("http://127.0.0.1:11434".into()),
                },
            ],
            false,
        );
        let lookup = key_lookup_for(&[("openai", Some("sk-test"))]);
        let chain = resolve_ai_chain(&settings, &lookup).expect("chain");
        // anthropic is dropped (no key), openai + ollama remain in order.
        assert_eq!(chain.len(), 2);
        assert_eq!(chain[0].provider, "openai");
        assert_eq!(chain[0].api_key.as_deref(), Some("sk-test"));
        assert_eq!(chain[0].model, "gpt-4o-mini", "default model applied");
        assert_eq!(chain[1].provider, "ollama");
        assert_eq!(chain[1].model, "llama3.2", "default model applied");
        assert_eq!(chain[1].api_key, None, "ollama needs no key");
        assert_eq!(chain[1].base_url.as_deref(), Some("http://127.0.0.1:11434"));
    }

    #[test]
    fn ai_chain_preserves_explicit_models() {
        let settings = WorkspaceSettings {
            ai_provider: Some("anthropic".into()),
            ai_model: Some("claude-custom".into()),
            ..chain_settings(None, vec![], false)
        };
        let lookup = key_lookup_for(&[("anthropic", Some("k"))]);
        let chain = resolve_ai_chain(&settings, &lookup).expect("chain");
        assert_eq!(chain[0].model, "claude-custom");
    }

    #[test]
    fn ai_chain_primary_without_key_is_a_hard_error() {
        let settings = chain_settings(Some("openai"), vec![], false);
        // openai is known to the keychain but has no stored key.
        let lookup = key_lookup_for(&[("openai", None)]);
        let err = resolve_ai_chain(&settings, &lookup).expect_err("no key");
        assert!(matches!(err, AppError::Credential(_)), "got: {err:?}");
    }

    #[test]
    fn ai_chain_requires_primary_provider() {
        let settings = chain_settings(None, vec![], false);
        let lookup = key_lookup_for(&[]);
        let err = resolve_ai_chain(&settings, &lookup).expect_err("no provider");
        assert!(
            err.to_string().contains("AI provider not configured"),
            "got: {err}"
        );
    }

    #[test]
    fn ai_chain_offline_keeps_only_ollama() {
        let settings = chain_settings(
            Some("openai"),
            vec![AiProviderConfig {
                provider: "ollama".into(),
                model: None,
                base_url: None,
            }],
            true,
        );
        let lookup = key_lookup_for(&[("openai", Some("sk-test"))]);
        let chain = resolve_ai_chain(&settings, &lookup).expect("chain");
        assert_eq!(chain.len(), 1);
        assert_eq!(chain[0].provider, "ollama");

        let cloud_only = chain_settings(Some("openai"), vec![], true);
        let err = resolve_ai_chain(&cloud_only, &lookup).expect_err("offline");
        assert!(err.to_string().contains("offline mode"), "got: {err}");
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
