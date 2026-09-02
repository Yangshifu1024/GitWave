//! Application use cases — workspace, repo, SSH, history, diff, blame, and branch.
//!
//! See `docs/tasks/feat-history-graph/plan.md` steps 6-10.

use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::{Arc, Mutex, OnceLock};

use crate::domain::app_settings::{ProxyMode, ProxySettings};
use crate::domain::blame::BlameLine;
use crate::domain::branch::{BranchInfo, CheckoutRemoteOutcome};
use crate::domain::diff::{DiffLineKind, FileDiff};
use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;
use crate::domain::history::{CommitDetails, CommitSummary, PrCommit};
use crate::domain::hooks::HookInfo;
use crate::domain::lfs::LfsStatus;
use crate::domain::stash::StashEntry;
use crate::domain::working_copy::WorkingCopy;
use crate::domain::workspace::{
    RepoRef, RepoStatus, Workspace, WorkspaceSettings, WorkspaceSummary,
};
use crate::domain::worktree::WorktreeInfo;
use crate::infrastructure::ai::read_ai_rules as infra_read_ai_rules;
use crate::infrastructure::ai::rebuild_http_client as infra_rebuild_http_client;
use crate::infrastructure::ai::with_reply_language;
use crate::infrastructure::git::blame::blame_file as infra_blame_file;
use crate::infrastructure::git::branch::{
    checkout_branch as infra_checkout_branch, checkout_commit as infra_checkout_commit,
    checkout_remote_branch as infra_checkout_remote_branch, create_branch as infra_create_branch,
    delete_branch as infra_delete_branch, rename_branch as infra_rename_branch,
    set_branch_upstream as infra_set_branch_upstream,
};
use crate::infrastructure::git::conflict::{
    abort_merge as infra_abort_merge, get_conflict_sides as infra_get_conflict_sides,
    is_merge_in_progress as infra_merge_in_progress, list_conflicts as infra_list_conflicts,
    resolve_conflict as infra_resolve_conflict, ConflictFile, ConflictSides,
};
use crate::infrastructure::git::credentials::InlineAuth;
use crate::infrastructure::git::diff::{
    diff_commit_vs_parent as infra_diff_commit_vs_parent,
    diff_commit_vs_parent_files as infra_diff_commit_vs_parent_files,
    diff_index_to_head as infra_diff_index_to_head,
    diff_index_to_head_files as infra_diff_index_to_head_files, diff_paths as infra_diff_paths,
    diff_workdir_to_index as infra_diff_workdir_to_index, DiffSummary,
};
use crate::infrastructure::git::health::{collect_health as infra_collect_health, HealthReport};
use crate::infrastructure::git::history::{
    ahead_behind as infra_ahead_behind, commit_details as infra_commit_details,
    commit_log as infra_commit_log, commit_recent_messages as infra_commit_recent_messages,
    commits_ahead_of as infra_commits_ahead_of, list_branches as infra_list_branches,
    resolve_ref_oid as infra_resolve_ref_oid,
};
use crate::infrastructure::git::hooks::{
    list_hooks as infra_list_hooks, read_hook as infra_read_hook, write_hook as infra_write_hook,
};
use crate::infrastructure::git::interactive_rebase::{
    abort_interactive_rebase_pause as infra_abort_irebase_pause,
    continue_interactive_rebase as infra_continue_irebase,
    execute_interactive_rebase as infra_execute_irebase,
    interactive_rebase_paused as infra_irebase_paused,
    plan_interactive_rebase as infra_plan_irebase, InteractiveRebaseResult, InteractiveRebaseTodo,
};
use crate::infrastructure::git::lfs::{
    lfs_available as infra_lfs_available, lfs_install as infra_lfs_install,
    lfs_installed as infra_lfs_installed, list_tracked_patterns as infra_lfs_list_patterns,
    track_pattern as infra_lfs_track, untrack_pattern as infra_lfs_untrack,
};
use crate::infrastructure::git::merge::{
    merge_branch as infra_merge_branch, merge_preview as infra_merge_preview, MergePreview,
    MergeResult,
};
use crate::infrastructure::git::rebase::{
    finalize_rebase as infra_finalize_rebase, rebase_branch as infra_rebase_branch, RebaseKind,
    RebaseResult,
};
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
    push_with_options as infra_push_with_options, worktree_is_dirty, CancelFlag, PullOptions,
    PushOutcome, PushRequest, SyncProgress,
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
    add_submodule as infra_submodule_add, deinit_submodule as infra_submodule_deinit,
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
use crate::infrastructure::persistence::app_settings_repo::AppSettingsRepository;
use crate::infrastructure::persistence::workspace_repo::WorkspaceRepository;
use crate::infrastructure::persistence::SqliteAppSettingsRepo;
use crate::infrastructure::persistence::SqliteWorkspaceRepo;
use crate::infrastructure::proxy::{
    apply_to_env as infra_apply_proxy_to_env, normalize_manual_url as infra_normalize_proxy_url,
};
use crate::infrastructure::ssh::keys::{expand_tilde, SshKey, SshKeyList, SshTestResult};

// ─── AppContext ──────────────────────────────────────────────────────────────

/// Application context — bundles infrastructure adapters and exposes use
/// cases. Held by Tauri as managed state.
pub struct AppContext {
    pub workspaces: Arc<Mutex<SqliteWorkspaceRepo>>,
    /// App-level global settings (`app_settings` table, F013). Own SQLite
    /// connection — see `SqliteAppSettingsRepo`.
    pub app_settings: Arc<Mutex<SqliteAppSettingsRepo>>,
}

impl AppContext {
    #[must_use]
    pub fn new(
        workspaces: Arc<Mutex<SqliteWorkspaceRepo>>,
        app_settings: Arc<Mutex<SqliteAppSettingsRepo>>,
    ) -> Self {
        Self {
            workspaces,
            app_settings,
        }
    }

    /// Open a Repository for the given repo path (no caching — git2::Repository
    /// is !Sync so can't be stored in shared state).
    #[allow(dead_code)]
    pub fn open_repo(&self, repo_path: &str) -> Result<git2::Repository> {
        git2::Repository::open(PathBuf::from(repo_path)).map_err(|e| {
            AppError::unknown_with(
                codes::usecases::REPO_OPEN_FAILED,
                format!("git open: {e}"),
                &[("error", e.to_string())],
            )
        })
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
        return Err(AppError::protocol(
            codes::usecases::WORKSPACE_NAME_EMPTY,
            "workspace name cannot be empty",
        ));
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
        return Err(AppError::protocol(
            codes::usecases::WORKSPACE_NAME_EMPTY,
            "workspace name cannot be empty",
        ));
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
        .ok_or_else(|| {
            AppError::protocol_with(
                codes::usecases::WORKSPACE_NOT_FOUND,
                format!("workspace not found: {id}"),
                &[("id", id.clone())],
            )
        })
}

pub fn update_workspace_settings(
    ctx: &AppContext,
    id: String,
    mut settings: WorkspaceSettings,
) -> Result<()> {
    let provider = settings.ai_provider.as_deref().unwrap_or("");
    if !provider.is_empty() && !matches!(provider, "openai" | "anthropic" | "ollama") {
        return Err(AppError::protocol_with(
            codes::usecases::AI_PROVIDER_UNSUPPORTED,
            format!("unsupported ai_provider: {provider}"),
            &[("provider", provider.to_string())],
        ));
    }
    for fb in &settings.ai_failover {
        if !matches!(fb.provider.as_str(), "openai" | "anthropic" | "ollama") {
            return Err(AppError::protocol_with(
                codes::usecases::AI_FAILOVER_PROVIDER_UNSUPPORTED,
                format!("unsupported ai_failover provider: {}", fb.provider),
                &[("provider", fb.provider.clone())],
            ));
        }
    }
    // A blank template means "use the built-in default", not "empty system
    // prompt" — normalize on save so consumers can unwrap directly.
    let templates = &mut settings.prompt_templates;
    for template in [
        &mut templates.commit,
        &mut templates.conflict,
        &mut templates.pr,
    ] {
        if template.as_deref().map(str::trim) == Some("") {
            *template = None;
        }
    }
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .update_settings(&id, &settings)
}

// ─── Proxy settings (F013) ───────────────────────────────────────────────────

/// The `app_settings` key holding the proxy configuration (F013).
const PROXY_SETTINGS_KEY: &str = "proxy";

/// Read the proxy settings; unset / corrupt store entries fall back to the
/// default (follow the system proxy, no manual URL).
pub fn get_proxy_settings(ctx: &AppContext) -> Result<ProxySettings> {
    let raw = ctx
        .app_settings
        .lock()
        .expect("app settings repo mutex poisoned")
        .get(PROXY_SETTINGS_KEY)?;
    let Some(raw) = raw else {
        return Ok(ProxySettings::default());
    };
    match serde_json::from_str(&raw) {
        Ok(settings) => Ok(settings),
        Err(e) => {
            tracing::warn!("stored proxy settings unreadable, using defaults: {e}");
            Ok(ProxySettings::default())
        }
    }
}

/// Validate + persist proxy settings and make them take effect immediately:
/// the env bridge feeds libgit2 (re-read per operation), git subprocesses
/// (inheritance) and any newly constructed reqwest client; the AI singleton
/// is rebuilt for good measure. Returns the normalized settings.
pub fn set_proxy_settings(ctx: &AppContext, settings: ProxySettings) -> Result<ProxySettings> {
    let settings = validate_and_store_proxy_settings(ctx, settings)?;
    infra_apply_proxy_to_env(&settings);
    infra_rebuild_http_client();
    Ok(settings)
}

/// Validation + persistence only (no env/client side effects) so tests can
/// exercise the rules without touching the process environment.
fn validate_and_store_proxy_settings(
    ctx: &AppContext,
    mut settings: ProxySettings,
) -> Result<ProxySettings> {
    if matches!(settings.mode, ProxyMode::Manual) {
        let url = settings.manual_url.as_deref().unwrap_or("").trim();
        if url.is_empty() {
            // Manual mode before an address is typed simply means no proxy.
            settings.manual_url = None;
        } else {
            let Some(normalized) = infra_normalize_proxy_url(url) else {
                return Err(AppError::protocol_with(
                    codes::usecases::PROXY_URL_INVALID,
                    format!("invalid manual proxy url: {url:?}"),
                    &[("url", url.to_string())],
                ));
            };
            settings.manual_url = Some(normalized);
        }
    } else if settings.manual_url.as_deref().map(str::trim) == Some("") {
        // A blank URL in system/off mode means "none", not "empty string".
        settings.manual_url = None;
    }

    let json = serde_json::to_string(&settings).map_err(|e| {
        AppError::unknown_with(
            codes::usecases::PROXY_SERIALIZE_FAILED,
            format!("serialize proxy settings: {e}"),
            &[("error", e.to_string())],
        )
    })?;
    ctx.app_settings
        .lock()
        .expect("app settings repo mutex poisoned")
        .set(PROXY_SETTINGS_KEY, &json)?;
    Ok(settings)
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
        std::fs::remove_dir_all(&dest).map_err(|e| {
            AppError::unknown_with(
                codes::usecases::CLONE_DEST_CLEAR_FAILED,
                format!("failed to clear dest for retry: {e}"),
                &[("error", e.to_string())],
            )
        })?;
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

/// Persist the Repository Tab order (F005). `repo_ids` must be a permutation
/// of the workspace's repos; the persistence layer validates that.
pub fn reorder_repos(ctx: &AppContext, workspace_id: String, repo_ids: Vec<String>) -> Result<()> {
    ctx.workspaces
        .lock()
        .expect("workspace repo mutex poisoned")
        .reorder_repos(&workspace_id, &repo_ids)
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

pub fn list_ssh_keys() -> Result<SshKeyList> {
    crate::infrastructure::ssh::keys::list_loaded()
}

pub fn add_ssh_key(path: String) -> Result<()> {
    let p = expand_tilde(&path);
    crate::infrastructure::ssh::keys::add(&p)
}

pub fn delete_ssh_key(path: String) -> Result<()> {
    let p = expand_tilde(&path);
    crate::infrastructure::ssh::keys::delete(&p)
}

pub fn test_ssh_connection(host: String, user: String) -> Result<SshTestResult> {
    crate::infrastructure::ssh::keys::test_connection(&host, &user)
}

/// Ask the OS to start the ssh-agent service (Windows only — via one UAC
/// prompt). On other platforms the UI guides the user to a terminal.
pub fn start_ssh_agent_service() -> Result<()> {
    #[cfg(windows)]
    {
        crate::infrastructure::ssh::keys::start_windows_agent_service()
    }
    #[cfg(not(windows))]
    {
        Err(AppError::protocol(
            codes::infra::AGENT_START_FAILED,
            "starting the ssh-agent service is only supported on Windows; \
             on macOS/Linux run eval $(ssh-agent)",
        ))
    }
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
/// 4xx/5xx other than auth, rate limits) move to the next provider. Auth
/// failures (401/403, mapped to `Credential` in `provider::http_error`)
/// stop the chain so the root cause surfaces; content and configuration
/// errors stop too — the next provider would make the same mistake on the
/// same prompt.
fn should_failover(err: &AppError) -> bool {
    matches!(err, AppError::Network { .. })
}

/// Resolve the primary + failover entries into concrete attempts. Cloud
/// entries whose API key cannot be resolved are skipped (they can never
/// succeed); a missing key on the *primary* is a hard error, matching the
/// pre-failover behavior. Offline mode (PM 1.6) is checked FIRST so a
/// disabled cloud primary cannot shadow a reachable Ollama fallback.
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
            AppError::protocol(
                codes::usecases::AI_PROVIDER_NOT_CONFIGURED,
                "AI provider not configured for this Workspace",
            )
        })?;

    // Offline mode: keep only Ollama entries, before any cloud key checks —
    // demanding a key for a provider the user just disabled is misleading.
    if settings.ai_offline {
        let mut chain = Vec::new();
        if primary == "ollama" {
            chain.push(resolve(
                &primary,
                &settings.ai_model,
                &settings.ai_base_url,
            )?);
        }
        for fb in &settings.ai_failover {
            if fb.provider.trim() == "ollama" {
                chain.push(resolve("ollama", &fb.model, &fb.base_url)?);
            }
        }
        if chain.is_empty() {
            return Err(AppError::protocol(
                codes::usecases::AI_OFFLINE_MODE,
                "offline mode is enabled — cloud AI calls are disabled (use Ollama or turn it off in AI settings)",
            ));
        }
        return Ok(chain);
    }

    let head = resolve(&primary, &settings.ai_model, &settings.ai_base_url)?;
    if head.provider != "ollama" && head.api_key.is_none() {
        return Err(AppError::credential_with(
            codes::usecases::AI_API_KEY_MISSING,
            format!("{primary} API key not configured"),
            &[("provider", primary.clone())],
        ));
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
    Ok(chain)
}

/// Run the chain in order; the first non-network outcome wins. Network
/// failures walk to the next entry; when the chain is exhausted the last
/// error is raised, with a per-provider summary when several failed.
async fn generate_with_failover(
    chain: Vec<ResolvedAiProvider>,
    system: String,
    user: String,
) -> Result<AiGenerateOutcome> {
    let total = chain.len();
    let mut failures: Vec<String> = Vec::new();
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
            // The chain loop above IS the failover — per-request fallbacks
            // stay empty (v0.3's in-request fallback field is unused here).
            fallbacks: Vec::new(),
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
                failures.push(format!("{}: {e}", entry.provider));
                last_err = Some(e);
            }
            // Non-network errors (auth, config, content) stop the chain so
            // the root cause surfaces instead of being masked by later
            // providers' failures.
            Err(e) => return Err(e),
        }
    }
    let last = last_err.expect("resolve_ai_chain never returns an empty chain");
    if failures.len() > 1 {
        Err(AppError::unknown_with(
            codes::usecases::AI_ALL_PROVIDERS_FAILED,
            format!("all providers failed — {}", failures.join("; ")),
            &[("errors", failures.join("; "))],
        ))
    } else {
        Err(last)
    }
}

/// Generate a commit message suggestion from staged/workdir diff + recent commits.
/// Result is always returned for the user to edit — never auto-commits (P1).
pub async fn generate_commit_message(
    ctx: &AppContext,
    workspace_id: String,
    language: Option<String>,
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
        return Err(AppError::protocol(
            codes::usecases::COMMIT_NO_STAGED,
            "no staged changes — stage files before generating a commit message",
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

    let system = with_reply_language(
        with_repo_rules(
            settings.prompt_templates.commit.unwrap_or_else(|| {
                "You write concise git commit messages. Output ONLY the message text. \
                 Prefer conventional commits (type: summary). First line <= 72 chars. \
                 Base the message strictly on the provided staged diff — do not invent \
                 changes that are not visible in it. \
                 Do not wrap in markdown fences."
                    .into()
            }),
            repo.workdir().and_then(infra_read_ai_rules),
        ),
        language.as_deref(),
    );

    generate_with_failover(chain, system, user).await
}

/// Append the repo's `.gitwave/AI.md` (when present) to a system prompt so
/// per-repo conventions ride along with every AI request.
fn with_repo_rules(system: String, rules: Option<String>) -> String {
    match rules {
        Some(rules) if !rules.trim().is_empty() => format!(
            "{system}\n\nRepository AI rules (from .gitwave/AI.md — they refine the \
             guidance above for this repository):\n{rules}"
        ),
        _ => system,
    }
}

/// Report the active repo's per-repo AI rules content so the settings UI
/// can show whether rules are in effect. `None` when absent.
pub fn get_repo_ai_rules(ctx: &AppContext, workspace_id: &str) -> Result<Option<String>> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    Ok(repo.workdir().and_then(infra_read_ai_rules))
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
    let oid = git2::Oid::from_str(commit_oid).map_err(|e| {
        AppError::protocol_with(
            codes::usecases::COMMIT_OID_INVALID,
            format!("invalid commit OID: {e}"),
            &[("error", e.to_string())],
        )
    })?;
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
    let from = git2::Oid::from_str(from_oid).map_err(|e| {
        AppError::protocol_with(
            codes::usecases::FILE_DIFF_FROM_OID_INVALID,
            format!("invalid from OID: {e}"),
            &[("error", e.to_string())],
        )
    })?;
    let to = git2::Oid::from_str(to_oid).map_err(|e| {
        AppError::protocol_with(
            codes::usecases::FILE_DIFF_TO_OID_INVALID,
            format!("invalid to OID: {e}"),
            &[("error", e.to_string())],
        )
    })?;
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
        .ok_or_else(|| {
            AppError::unknown_with(
                codes::usecases::BRANCH_NOT_FOUND_AFTER_CREATE,
                format!("branch {} not found after creation", name),
                &[("name", name.to_string())],
            )
        })
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
    cancel: Option<CancelFlag>,
    auth: Option<InlineAuth>,
) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_delete_remote_branch(&repo, remote, branch, cancel, auth.as_ref())
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

/// Check out a remote-tracking branch DWIM-style (F012): reuse the
/// same-named local branch or create it with upstream tracking, then switch.
pub fn checkout_remote_branch(
    ctx: &AppContext,
    workspace_id: &str,
    name: &str,
    force: bool,
) -> Result<CheckoutRemoteOutcome> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_checkout_remote_branch(&repo, name, force)
}

/// Check out a commit directly (detached HEAD, updates HEAD and working tree).
pub fn checkout_commit(ctx: &AppContext, workspace_id: &str, oid: &str, force: bool) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_checkout_commit(&repo, oid, force)
}

/// Rename a local branch (the ref, plus its upstream tracking relationship).
pub fn rename_branch(
    ctx: &AppContext,
    workspace_id: &str,
    old_name: &str,
    new_name: &str,
    force: bool,
) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_rename_branch(&repo, old_name, new_name, force)
}

/// Set (or clear) the upstream a local branch tracks, e.g. `origin/main`.
pub fn set_branch_upstream(
    ctx: &AppContext,
    workspace_id: &str,
    branch_name: &str,
    upstream: Option<String>,
) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_set_branch_upstream(&repo, branch_name, upstream.as_deref())
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
    language: Option<String>,
) -> Result<AiGenerateOutcome> {
    let ws = get_workspace(ctx, workspace_id.clone())?;
    let settings = ws.settings;
    let key_lookup =
        |provider: &str| crate::infrastructure::ai::get_api_key(&workspace_id, provider);
    let chain = resolve_ai_chain(&settings, &key_lookup)?;
    let sides = get_conflict_sides(ctx, &workspace_id, path.clone())?;
    let repo_path = active_repo_path(ctx, &workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    let rules = repo.workdir().and_then(infra_read_ai_rules);
    let system = with_reply_language(
        with_repo_rules(
            settings.prompt_templates.conflict.unwrap_or_else(|| {
                "You explain git merge conflicts for a human developer. \
             Describe what each side intends and suggest a resolution approach. \
             Do NOT output a full rewritten file unless asked. \
             Clearly state this is advice only — the user must apply changes."
                    .into()
            }),
            rules,
        ),
        language.as_deref(),
    );
    let user = format!(
        "Conflict in `{path}`\n\n=== BASE ===\n{}\n\n=== OURS ===\n{}\n\n=== THEIRS ===\n{}\n",
        sides.base.as_deref().unwrap_or("(missing)"),
        sides.ours.as_deref().unwrap_or("(missing)"),
        sides.theirs.as_deref().unwrap_or("(missing)"),
    );
    generate_with_failover(chain, system, user).await
}

// ─── AI PR description ──────────────────────────────────────────────────────

/// Generated PR title + markdown body, plus which provider served it.
#[derive(Debug, Clone, serde::Serialize)]
pub struct PrDescriptionOutcome {
    pub title: String,
    pub body: String,
    pub provider_used: String,
    pub used_fallback: bool,
}

pub const DEFAULT_PR_SYSTEM: &str =
    "You write pull request titles and descriptions for a human reviewer. \
The FIRST line of your output is the PR title: plain text, at most 72 characters, no prefix. \
After the title leave one blank line, then write the description in markdown: a short summary \
paragraph, bullet points for the notable changes, and a short 'Testing' section. \
Base everything strictly on the provided commits and diff — do not invent changes. \
Do not wrap the output in markdown fences.";

/// Default base candidates for PR description generation, in order.
const PR_BASE_CANDIDATES: [&str; 4] = ["origin/main", "origin/master", "main", "master"];

fn default_pr_base(repo: &git2::Repository) -> Result<String> {
    for candidate in PR_BASE_CANDIDATES {
        if infra_resolve_ref_oid(repo, candidate).is_ok() {
            return Ok(candidate.to_string());
        }
    }
    Err(AppError::protocol(
        codes::usecases::PR_NO_BASE_BRANCH,
        "no base branch found (tried origin/main, origin/master, main, master) — \
         pick a base branch explicitly",
    ))
}

/// Split raw model output into title (first line) + markdown body.
fn split_pr_text(text: &str) -> (String, String) {
    let text = text.trim();
    match text.split_once('\n') {
        Some((title, rest)) => (title.trim().to_string(), rest.trim().to_string()),
        None => (text.to_string(), String::new()),
    }
}

/// Assemble the user prompt: branch segment commits + combined diff.
fn build_pr_user_prompt(
    branch: &str,
    base: &str,
    commits: &[PrCommit],
    files: &[FileDiff],
) -> String {
    let mut user = format!(
        "Branch: {branch}\nBase: {base}\n\nCommits ({}):\n",
        commits.len()
    );
    for c in commits {
        let short = &c.sha[..c.sha.len().min(7)];
        user.push_str(&format!("- {short} {}\n", c.subject));
    }
    user.push_str("\nFull commit messages of the branch (style reference):\n");
    for c in commits.iter().take(5) {
        user.push_str("---\n");
        user.push_str(&c.message_full);
        user.push_str("\n---\n");
    }
    user.push_str("\nCombined diff of the branch (unified format, may be truncated):\n");
    append_diff_patch(&mut user, files, 12_000);
    user
}

/// AI-generated PR description for the active branch vs `base` (default:
/// first existing of origin/main, origin/master, main, master). Never
/// creates a PR (P1) — output is copy-ready text for the user.
pub async fn generate_pr_description(
    ctx: &AppContext,
    workspace_id: String,
    base: Option<String>,
    language: Option<String>,
) -> Result<PrDescriptionOutcome> {
    let ws = get_workspace(ctx, workspace_id.clone())?;
    let settings = ws.settings;
    let key_lookup =
        |provider: &str| crate::infrastructure::ai::get_api_key(&workspace_id, provider);
    let chain = resolve_ai_chain(&settings, &key_lookup)?;

    // Scoped so the !Send git2 handles (Repository / Commit) drop before
    // the await — the command future must stay Send.
    let (user, system) = {
        let repo_path = active_repo_path(ctx, &workspace_id)?;
        let repo = ctx.open_repo(&repo_path)?;
        let head = match repo.head() {
            Ok(head) => head.peel_to_commit().map_err(AppError::from)?,
            Err(e) if e.code() == git2::ErrorCode::UnbornBranch => {
                return Err(AppError::protocol(
                    codes::usecases::PR_NO_COMMITS_YET,
                    "repository has no commits yet — nothing to describe",
                ));
            }
            Err(e) => return Err(AppError::from(e)),
        };
        let branch = repo
            .head()
            .ok()
            .and_then(|h| h.shorthand().map(str::to_string))
            .unwrap_or_else(|| "HEAD (detached)".to_string());

        let base_name = match base.filter(|b| !b.trim().is_empty()) {
            Some(b) => b,
            None => default_pr_base(&repo)?,
        };
        let base_oid = infra_resolve_ref_oid(&repo, &base_name)?;
        let merge_base = repo.merge_base(base_oid, head.id()).map_err(|_| {
            AppError::protocol_with(
                codes::usecases::PR_NO_COMMON_ANCESTOR,
                format!("no common ancestor between HEAD and {base_name}"),
                &[("base_name", base_name.clone())],
            )
        })?;
        let commits = infra_commits_ahead_of(&repo, merge_base, head.id(), 30)?;
        if commits.is_empty() {
            return Err(AppError::protocol_with(
                codes::usecases::PR_NO_COMMITS_AHEAD,
                format!("no commits ahead of {base_name} — nothing to describe"),
                &[("base_name", base_name.clone())],
            ));
        }
        let files = infra_diff_paths(&repo, merge_base, head.id())?;

        let user = build_pr_user_prompt(&branch, &base_name, &commits, &files);
        let system = with_reply_language(
            with_repo_rules(
                settings
                    .prompt_templates
                    .pr
                    .clone()
                    .filter(|t| !t.trim().is_empty())
                    .unwrap_or_else(|| DEFAULT_PR_SYSTEM.to_string()),
                repo.workdir().and_then(infra_read_ai_rules),
            ),
            language.as_deref(),
        );
        (user, system)
    };

    let outcome = generate_with_failover(chain, system, user).await?;
    let (title, body) = split_pr_text(&outcome.text);
    Ok(PrDescriptionOutcome {
        title,
        body,
        provider_used: outcome.provider_used,
        used_fallback: outcome.used_fallback,
    })
}

// ─── AI history explain ─────────────────────────────────────────────────────

pub const DEFAULT_EXPLAIN_SYSTEM: &str = "You explain git commits for a human developer. \
Describe what changed and why it likely matters: start from the commit message, then the diff. \
Keep it under 200 words, plain prose or short bullets. If the diff contradicts the message, \
say so. Base everything strictly on the provided data.";

/// Assemble the explain prompt: full message + changed-file summary + patch.
fn build_explain_user_prompt(
    message_full: &str,
    summary: &DiffSummary,
    files: &[FileDiff],
) -> String {
    let mut user = format!("Commit message:\n{message_full}\n\nChanged files:\n");
    append_diff_summary(&mut user, summary);
    user.push_str("\nDiff (unified format, may be truncated):\n");
    append_diff_patch(&mut user, files, 12_000);
    user
}

/// AI explanation of a single commit — read-only advice, nothing is
/// applied to the repository (P1).
pub async fn explain_commit(
    ctx: &AppContext,
    workspace_id: String,
    sha: String,
    language: Option<String>,
) -> Result<AiGenerateOutcome> {
    let ws = get_workspace(ctx, workspace_id.clone())?;
    let settings = ws.settings;
    let key_lookup =
        |provider: &str| crate::infrastructure::ai::get_api_key(&workspace_id, provider);
    let chain = resolve_ai_chain(&settings, &key_lookup)?;

    // Scoped so the !Send git2 handle drops before the await.
    let (user, system) = {
        let repo_path = active_repo_path(ctx, &workspace_id)?;
        let repo = ctx.open_repo(&repo_path)?;
        let oid = infra_resolve_ref_oid(&repo, &sha)?;
        let details = infra_commit_details(&repo, &sha)?;
        let summary = infra_diff_commit_vs_parent(&repo, oid)?;
        let files = infra_diff_commit_vs_parent_files(&repo, oid)?;
        let user = build_explain_user_prompt(&details.message_full, &summary, &files);
        let system = with_reply_language(
            with_repo_rules(
                DEFAULT_EXPLAIN_SYSTEM.to_string(),
                repo.workdir().and_then(infra_read_ai_rules),
            ),
            language.as_deref(),
        );
        (user, system)
    };
    generate_with_failover(chain, system, user).await
}

// ─── AI command palette (Cmd+K) ─────────────────────────────────────────────

/// One AI-proposed palette action. Mutating actions carry
/// `requires_confirm = true` and the UI must not execute them without an
/// explicit user confirmation (P1). commit / push / merge / rebase are not
/// in the whitelist at all and are rejected server-side.
#[derive(Debug, Clone, serde::Serialize)]
pub struct PaletteIntent {
    pub action: String,
    pub params: serde_json::Value,
    pub explanation: String,
    pub requires_confirm: bool,
}

/// Actions the palette may execute without further confirmation (read-only
/// navigation or dialogs).
pub const PALETTE_ACTIONS_IMMEDIATE: [&str; 3] = ["explain_commit", "locate_commit", "none"];
/// Actions that only run after the user confirms the intent card.
pub const PALETTE_ACTIONS_CONFIRM: [&str; 5] = [
    "create_branch",
    "checkout_branch",
    "create_tag",
    "stash_changes",
    "fetch_remotes",
];

pub const PALETTE_SYSTEM: &str = "You convert a user request into exactly ONE GitWave UI action. \
Respond with ONLY a JSON object, no prose, no markdown fences: \
{\"action\": \"...\", \"params\": {...}, \"explanation\": \"one short sentence\"}. \
Allowed actions and their params:
- explain_commit {\"sha\": string} — AI explains what a commit changed
- locate_commit {\"sha\": string} — scroll the history graph to a commit
- create_branch {\"name\": string, \"from\": string (optional sha or branch)}
- checkout_branch {\"name\": string}
- create_tag {\"name\": string, \"sha\": string (optional, defaults to current tip)}
- stash_changes {\"message\": string (optional)}
- fetch_remotes {}
- none {} — when nothing matches
Pick shas from the provided recent commits when possible. Mutating actions are shown to the \
user for confirmation first, so prefer the closest matching action over refusing. \
Never invent actions outside this list.";

fn strip_json_fences(raw: &str) -> &str {
    let trimmed = raw.trim();
    let Some(without_open) = trimmed.strip_prefix("```") else {
        return trimmed;
    };
    // Drop an optional language tag on the opening fence line.
    let body = without_open
        .split_once('\n')
        .map(|(_, rest)| rest)
        .unwrap_or(without_open);
    body.trim()
        .strip_suffix("```")
        .map(str::trim)
        .unwrap_or(body.trim())
}

/// Tolerant parse + whitelist validation of the model's JSON output.
/// Invalid shapes and non-whitelisted actions are hard errors — the palette
/// shows them as AI errors instead of executing anything.
fn parse_palette_intent(raw: &str) -> Result<PaletteIntent> {
    let text = strip_json_fences(raw);
    let (start, end) = match (text.find('{'), text.rfind('}')) {
        (Some(s), Some(e)) if e > s => (s, e),
        _ => {
            return Err(AppError::protocol(
                codes::usecases::PALETTE_NO_JSON,
                "AI did not return a JSON action — try rephrasing the request",
            ));
        }
    };
    let value: serde_json::Value = serde_json::from_str(&text[start..=end]).map_err(|_| {
        AppError::protocol(
            codes::usecases::PALETTE_MALFORMED_JSON,
            "AI returned malformed JSON — try again",
        )
    })?;

    let action = value["action"]
        .as_str()
        .unwrap_or_default()
        .trim()
        .to_ascii_lowercase();
    let explanation = value["explanation"]
        .as_str()
        .unwrap_or_default()
        .trim()
        .to_string();
    let params = if value["params"].is_object() {
        value["params"].clone()
    } else {
        serde_json::json!({})
    };

    let whitelisted = PALETTE_ACTIONS_IMMEDIATE.contains(&action.as_str())
        || PALETTE_ACTIONS_CONFIRM.contains(&action.as_str());
    if !whitelisted {
        let hinted = if action.is_empty() {
            "empty action".to_string()
        } else {
            format!("unsupported action: {action}")
        };
        return Err(AppError::protocol_with(
            codes::usecases::PALETTE_ACTION_FORBIDDEN,
            format!("{hinted} — commit, push, merge and rebase are never palette-driven (P1)"),
            &[("hinted", hinted)],
        ));
    }

    let require_str = |key: &str| -> Result<String> {
        params[key]
            .as_str()
            .map(str::trim)
            .filter(|s| !s.is_empty())
            .map(str::to_string)
            .ok_or_else(|| {
                AppError::protocol_with(
                    codes::usecases::PALETTE_PARAM_MISSING,
                    format!("AI action \"{action}\" is missing \"{key}\""),
                    &[("action", action.clone()), ("key", key.to_string())],
                )
            })
    };
    match action.as_str() {
        "explain_commit" | "locate_commit" => {
            require_str("sha")?;
        }
        "create_branch" | "checkout_branch" | "create_tag" => {
            require_str("name")?;
        }
        _ => {}
    }

    Ok(PaletteIntent {
        requires_confirm: PALETTE_ACTIONS_CONFIRM.contains(&action.as_str()),
        action,
        params,
        explanation,
    })
}

/// Repository context snapshot fed to the palette prompt: enough for the
/// model to resolve branch / tag / sha references, small enough to stay
/// cheap. Read-only assembly — no working-tree access.
#[derive(serde::Serialize)]
struct PaletteContext {
    current_branch: String,
    local_branches: Vec<String>,
    remote_branches: Vec<String>,
    tags: Vec<String>,
    repos: Vec<PaletteRepo>,
    recent_commits: Vec<PaletteCommit>,
}

#[derive(serde::Serialize)]
struct PaletteRepo {
    id: String,
    name: String,
}

#[derive(serde::Serialize)]
struct PaletteCommit {
    sha: String,
    subject: String,
}

/// Interpret a natural-language request as a whitelisted palette action.
/// The AI only *proposes* — execution (with confirmation for mutating
/// actions) stays in the frontend.
pub async fn ai_palette_intent(
    ctx: &AppContext,
    workspace_id: String,
    query: String,
) -> Result<PaletteIntent> {
    if query.trim().is_empty() {
        return Err(AppError::protocol(
            codes::usecases::PALETTE_EMPTY_REQUEST,
            "empty palette request",
        ));
    }
    let ws = get_workspace(ctx, workspace_id.clone())?;
    let settings = ws.settings;
    let key_lookup =
        |provider: &str| crate::infrastructure::ai::get_api_key(&workspace_id, provider);
    let chain = resolve_ai_chain(&settings, &key_lookup)?;

    let repos = list_repos(ctx, workspace_id.clone())?
        .into_iter()
        .map(|r| PaletteRepo {
            id: r.id,
            name: r.nickname.unwrap_or(r.path),
        })
        .collect();
    let tags = list_tags(ctx, &workspace_id)?
        .into_iter()
        .take(20)
        .map(|t| t.name)
        .collect();

    // Scoped so the !Send git2 handle drops before the await.
    let (current_branch, local_branches, remote_branches, recent_commits, rules) = {
        let repo_path = active_repo_path(ctx, &workspace_id)?;
        let repo = ctx.open_repo(&repo_path)?;
        let current_branch = repo
            .head()
            .ok()
            .and_then(|h| h.shorthand().map(str::to_string))
            .unwrap_or_else(|| "HEAD (detached)".to_string());
        let mut local = Vec::new();
        let mut remote = Vec::new();
        if let Ok(branches) = repo.branches(None) {
            for item in branches.flatten() {
                let (branch, kind) = item;
                let Some(name) = branch.name().ok().flatten() else {
                    continue;
                };
                match kind {
                    git2::BranchType::Local if local.len() < 20 => local.push(name.to_string()),
                    git2::BranchType::Remote if remote.len() < 15 => remote.push(name.to_string()),
                    _ => {}
                }
            }
        }
        let recent_commits = infra_commit_log(&repo, 15, None)?
            .into_iter()
            .map(|c| PaletteCommit {
                sha: c.sha,
                subject: c.message_summary,
            })
            .collect();
        let rules = repo.workdir().and_then(infra_read_ai_rules);
        (current_branch, local, remote, recent_commits, rules)
    };

    let snapshot = PaletteContext {
        current_branch,
        local_branches,
        remote_branches,
        tags,
        repos,
        recent_commits,
    };
    let user = format!(
        "Repository context:\n{}\n\nUser request: {}",
        serde_json::to_string_pretty(&snapshot).map_err(|e| {
            AppError::unknown_with(
                codes::usecases::PALETTE_CONTEXT_SERIALIZE_FAILED,
                format!("palette context: {e}"),
                &[("error", e.to_string())],
            )
        })?,
        query.trim()
    );
    let system = with_repo_rules(PALETTE_SYSTEM.to_string(), rules);

    let outcome = generate_with_failover(chain, system, user).await?;
    parse_palette_intent(&outcome.text)
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
    // The in-memory rewrite finalizes with a force checkout — refuse a
    // dirty worktree instead of clobbering local changes (git refuses the
    // same way).
    if worktree_is_dirty(&repo)? {
        return Err(AppError::protocol(
            codes::usecases::REBASE_DIRTY_WORKTREE,
            "rebase needs a clean worktree; commit or stash your changes first",
        ));
    }
    let result = infra_rebase_branch(&repo, upstream)?;
    if result.kind == RebaseKind::Clean {
        // In-memory rebase leaves refs and the workdir untouched; land the
        // rewritten head on the current branch here. Clone keeps `new_head`
        // in the returned result — its contract promises it on Clean.
        let new_head = result.new_head.clone().ok_or_else(|| {
            AppError::protocol(
                codes::usecases::REBASE_NO_NEW_HEAD,
                "rebase finished without a new HEAD",
            )
        })?;
        infra_finalize_rebase(&repo, &new_head)?;
    }
    Ok(result)
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
pub async fn explain_health(
    ctx: &AppContext,
    workspace_id: String,
    language: Option<String>,
) -> Result<String> {
    let ws = get_workspace(ctx, workspace_id.clone())?;
    let settings = ws.settings;
    let key_lookup =
        |provider: &str| crate::infrastructure::ai::get_api_key(&workspace_id, provider);
    let chain = resolve_ai_chain(&settings, &key_lookup)?;

    let report = get_health(ctx, &workspace_id)?;
    let user = format!(
        "Repo health metrics (JSON):
{}

Write a short health assessment:          what looks fine, what needs attention, and the single most          valuable next action. Plain text, no markdown fences.",
        serde_json::to_string_pretty(&report).map_err(|e| {
            AppError::unknown_with(
                codes::usecases::HEALTH_SERIALIZE_FAILED,
                format!("serialize report: {e}"),
                &[("error", e.to_string())],
            )
        })?,
    );
    let system = with_reply_language(
        settings.prompt_templates.health.clone().unwrap_or_else(|| {
            "You are a repository health assistant. You receive deterministic          metrics about a git repository and summarize them for a developer.          Advice only — you never execute anything."
                .into()
        }),
        language.as_deref(),
    );

    let outcome = generate_with_failover(chain, system, user).await?;
    Ok(outcome.text)
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
    language: Option<String>,
) -> Result<String> {
    let ws = get_workspace(ctx, workspace_id.clone())?;
    let settings = ws.settings;
    let key_lookup =
        |provider: &str| crate::infrastructure::ai::get_api_key(&workspace_id, provider);
    let chain = resolve_ai_chain(&settings, &key_lookup)?;

    let repo_path = active_repo_path(ctx, &workspace_id)?;
    // git2::Repository is !Send — resolve subjects inside a scope so it is
    // dropped before the network await.
    let (old_subject, new_subject) = {
        let repo = ctx.open_repo(&repo_path)?;
        (subject_of(&repo, &old_oid), subject_of(&repo, &new_oid))
    };

    let system = with_reply_language(
        settings.prompt_templates.reflog.clone().unwrap_or_else(|| {
            "You are a git recovery assistant. A single reflog entry is provided.          In 2-4 sentences: explain what happened to the branch, then give one          concrete recovery recommendation (create a recovery branch at a sha,          git reset --hard, or checkout). Advice only — you never execute          anything. Do not wrap in markdown fences."
                .into()
        }),
        language.as_deref(),
    );
    let user = format!(
        "Reflog entry\nAction: {action}\nMessage: {message}\n\nPrevious position: {old}\n  ({old_subject})\n\nNew position: {new}\n  ({new_subject})\n",
        old = old_oid,
        new = new_oid,
    );

    let outcome = generate_with_failover(chain, system, user).await?;
    Ok(outcome.text)
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

/// `git submodule update --init <name>` (clone + checkout the worktree);
/// with `recursive`, also update nested submodules of its worktree.
pub fn update_submodule(
    ctx: &AppContext,
    workspace_id: &str,
    name: &str,
    recursive: bool,
) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_submodule_update(&repo, name, recursive)
}

/// `git submodule add <url> <path>` — clones and stages the gitlink +
/// `.gitmodules` entry. Staged only; the user commits via the normal flow.
pub fn add_submodule(
    ctx: &AppContext,
    workspace_id: &str,
    url: String,
    path: String,
) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_submodule_add(&repo, &url, &path)
}

/// `git submodule deinit <name>` — unregisters from `.git/config`. Milder
/// than git's version: the submodule worktree is left untouched (nothing
/// is deleted); the entry stays in `.gitmodules` and the index.
pub fn deinit_submodule(ctx: &AppContext, workspace_id: &str, name: &str) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_submodule_deinit(&repo, name)
}

// ─── Git LFS use cases ──────────────────────────────────────────────────────

/// Snapshot of the active repo's LFS state (binary available, local filters
/// wired, tracked patterns).
pub fn lfs_status(ctx: &AppContext, workspace_id: &str) -> Result<LfsStatus> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    Ok(LfsStatus {
        available: infra_lfs_available(),
        installed: infra_lfs_installed(&repo)?,
        patterns: infra_lfs_list_patterns(&repo)?,
    })
}

/// Wire LFS filters into the active repository (`git lfs install --local`).
/// Requires a `git lfs` binary on PATH.
pub fn lfs_install(ctx: &AppContext, workspace_id: &str) -> Result<String> {
    if !infra_lfs_available() {
        return Err(AppError::protocol(
            codes::usecases::LFS_NOT_INSTALLED,
            "git lfs is not installed — install Git LFS (https://git-lfs.com) and restart GitWave",
        ));
    }
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_lfs_install(&repo)
}

/// Track a path pattern with LFS (appends to `.gitattributes`).
pub fn lfs_track(ctx: &AppContext, workspace_id: &str, pattern: String) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_lfs_track(&repo, &pattern)
}

/// Stop tracking a pattern with LFS (removes the `.gitattributes` line).
pub fn lfs_untrack(ctx: &AppContext, workspace_id: &str, pattern: &str) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_lfs_untrack(&repo, pattern)
}

/// Read the repo-root `.gitignore` (empty string when absent) — S2 editor.
pub fn get_gitignore(ctx: &AppContext, workspace_id: &str) -> Result<String> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let path = std::path::Path::new(&repo_path).join(".gitignore");
    if !path.exists() {
        return Ok(String::new());
    }
    std::fs::read_to_string(&path).map_err(|e| {
        AppError::unknown_with(
            codes::usecases::GITIGNORE_READ_FAILED,
            format!("read .gitignore: {e}"),
            &[("error", e.to_string())],
        )
    })
}

// ─── Git hooks editor ───────────────────────────────────────────────────────

/// The known client-side hooks with presence markers. GitWave edits hooks;
/// it never executes them (P1).
pub fn list_hooks(ctx: &AppContext, workspace_id: &str) -> Result<Vec<HookInfo>> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_list_hooks(&repo)
}

/// Read a hook's script (empty when the hook does not exist yet).
pub fn get_hook(ctx: &AppContext, workspace_id: &str, name: &str) -> Result<String> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_read_hook(&repo, name)
}

/// Write a hook script (on unix it is made executable).
pub fn save_hook(ctx: &AppContext, workspace_id: &str, name: &str, content: String) -> Result<()> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_write_hook(&repo, name, &content)
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
    std::fs::write(&path, normalized).map_err(|e| {
        AppError::unknown_with(
            codes::usecases::GITIGNORE_WRITE_FAILED,
            format!("write .gitignore: {e}"),
            &[("error", e.to_string())],
        )
    })
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
    let json = serde_json::to_string_pretty(&transfer).map_err(|e| {
        AppError::unknown_with(
            codes::usecases::TRANSFER_SERIALIZE_FAILED,
            format!("serialize workspace: {e}"),
            &[("error", e.to_string())],
        )
    })?;
    std::fs::write(dest_path, json).map_err(|e| {
        AppError::unknown_with(
            codes::usecases::TRANSFER_WRITE_FAILED,
            format!("write transfer file: {e}"),
            &[("error", e.to_string())],
        )
    })?;
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
    let raw = std::fs::read_to_string(src_path).map_err(|e| {
        AppError::unknown_with(
            codes::usecases::TRANSFER_READ_FAILED,
            format!("read transfer file: {e}"),
            &[("error", e.to_string())],
        )
    })?;
    let transfer: WorkspaceTransfer = serde_json::from_str(&raw).map_err(|e| {
        AppError::protocol_with(
            codes::usecases::TRANSFER_INVALID,
            format!("invalid transfer file: {e}"),
            &[("error", e.to_string())],
        )
    })?;
    if transfer.version != 1 {
        return Err(AppError::protocol_with(
            codes::usecases::TRANSFER_VERSION_UNSUPPORTED,
            format!("unsupported transfer file version: {}", transfer.version),
            &[("version", transfer.version.to_string())],
        ));
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
        .ok_or_else(|| {
            AppError::unknown(
                codes::usecases::TRANSFER_WORKSPACE_VANISHED,
                "imported workspace vanished",
            )
        })
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
    let ws = workspaces.get(workspace_id)?.ok_or_else(|| {
        AppError::protocol_with(
            codes::usecases::WORKSPACE_NOT_FOUND,
            format!("workspace not found: {workspace_id}"),
            &[("id", workspace_id.to_string())],
        )
    })?;
    ws.last_active_repo_id.clone().ok_or_else(|| {
        AppError::protocol(
            codes::usecases::NO_ACTIVE_REPO,
            "no active repo in workspace",
        )
    })
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

/// Per-workspace fetch mutex: auto-refresh and a manual fetch can overlap,
/// and two concurrent credential prompts for the same remote would each
/// pop the helper — the credential fill gate only dedups within a single
/// operation.
fn workspace_fetch_lock(workspace_id: &str) -> Arc<Mutex<()>> {
    static LOCKS: OnceLock<Mutex<HashMap<String, Arc<Mutex<()>>>>> = OnceLock::new();
    let mut locks = LOCKS
        .get_or_init(|| Mutex::new(HashMap::new()))
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    locks
        .entry(workspace_id.to_string())
        .or_insert_with(|| Arc::new(Mutex::new(())))
        .clone()
}

pub fn fetch(
    ctx: &AppContext,
    workspace_id: &str,
    remote: Option<String>,
    on_progress: Option<Box<dyn Fn(SyncProgress) + Send>>,
    cancel: Option<CancelFlag>,
    auth: Option<InlineAuth>,
) -> Result<()> {
    let fetch_lock = workspace_fetch_lock(workspace_id);
    let _serialized = fetch_lock
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    let Some(remote) = remote else {
        // No name given means "fetch every configured remote" (toolbar
        // Fetch, command palette and auto-refresh all pass None). Each
        // remote is attempted even if one fails; the first error wins.
        let names = infra_list_remotes(&repo)?;
        // The progress closure is not Clone; share it behind a Mutex and
        // re-box per remote so transfer events keep flowing for each fetch.
        // (Arc<Mutex<_>> is Send+Sync, so the per-remote wrapper stays Send.)
        let shared = on_progress.map(|f| Arc::new(Mutex::new(f)));
        let mut first_err: Option<AppError> = None;
        for name in names {
            let cb = shared.clone().map(|f| {
                Box::new(move |p: SyncProgress| {
                    (f.lock().unwrap())(p);
                }) as Box<dyn Fn(SyncProgress) + Send>
            });
            if let Err(e) = infra_fetch(
                &repo,
                &name,
                crate::infrastructure::git::remote::SyncOperation::Fetch,
                cb,
                cancel.clone(),
                auth.as_ref(),
            ) {
                // A cancelled operation must not grind through the remaining
                // remotes — the user asked for the whole fetch to stop.
                if cancel
                    .as_deref()
                    .is_some_and(|flag| flag.load(std::sync::atomic::Ordering::Relaxed))
                {
                    return Err(e);
                }
                // Same for auth failures: credentials are host-scoped, so
                // stop the batch and name the remote that challenged (the
                // error carries it) — the F012 retry targets it alone.
                if e.code() == codes::git::FETCH_AUTH_FAILED {
                    return Err(e);
                }
                if first_err.is_none() {
                    first_err = Some(e);
                }
            }
        }
        return match first_err {
            Some(e) => Err(e),
            None => Ok(()),
        };
    };
    infra_fetch(
        &repo,
        &remote,
        crate::infrastructure::git::remote::SyncOperation::Fetch,
        on_progress,
        cancel,
        auth.as_ref(),
    )
}

#[allow(clippy::too_many_arguments)]
pub fn pull(
    ctx: &AppContext,
    workspace_id: &str,
    remote: Option<String>,
    branch: Option<String>,
    rebase: bool,
    stash: bool,
    on_progress: Option<Box<dyn Fn(SyncProgress) + Send>>,
    cancel: Option<CancelFlag>,
    auth: Option<InlineAuth>,
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
        cancel,
        auth.as_ref(),
    )
}

pub fn list_remotes(ctx: &AppContext, workspace_id: &str) -> Result<Vec<String>> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_list_remotes(&repo)
}

#[allow(clippy::too_many_arguments)]
pub fn push(
    ctx: &AppContext,
    workspace_id: &str,
    remote: Option<String>,
    tags: bool,
    force: bool,
    branch: Option<String>,
    on_progress: Option<Box<dyn Fn(SyncProgress) + Send>>,
    cancel: Option<CancelFlag>,
    auth: Option<InlineAuth>,
) -> Result<PushOutcome> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_push_with_options(
        &repo,
        remote.as_deref().unwrap_or("origin"),
        PushRequest {
            tags,
            force,
            branch,
        },
        &on_progress,
        cancel,
        auth.as_ref(),
    )
}

// ─── Stash (Sprint 5) ───────────────────────────────────────────────────────

pub fn list_stashes(ctx: &AppContext, workspace_id: &str) -> Result<Vec<StashEntry>> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let mut repo = ctx.open_repo(&repo_path)?;
    infra_list_stashes(&mut repo)
}

pub fn save_stash(
    ctx: &AppContext,
    workspace_id: &str,
    message: Option<String>,
    include_untracked: bool,
) -> Result<String> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let mut repo = ctx.open_repo(&repo_path)?;
    infra_save_stash(&mut repo, message.as_deref(), include_untracked)
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
    start_point: Option<String>,
) -> Result<WorktreeInfo> {
    let repo_path = active_repo_path(ctx, workspace_id)?;
    let repo = ctx.open_repo(&repo_path)?;
    infra_add_worktree(
        &repo,
        &name,
        PathBuf::from(path).as_path(),
        &branch,
        create_branch,
        start_point.as_deref(),
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
        opt.ok_or_else(|| {
            AppError::protocol_with(
                codes::usecases::WORKSPACE_NOT_FOUND,
                format!("workspace not found: {workspace_id}"),
                &[("id", workspace_id.to_string())],
            )
        })
    })?;
    let repo_id = ws.last_active_repo_id.as_ref().ok_or_else(|| {
        AppError::protocol(
            codes::usecases::NO_ACTIVE_REPO,
            "no active repo in workspace",
        )
    })?;
    let repos = workspaces.list_repos(workspace_id)?;
    let repo = repos
        .iter()
        .find(|r| r.id.as_str() == repo_id)
        .ok_or_else(|| {
            AppError::protocol_with(
                codes::usecases::REPO_NOT_FOUND,
                format!("repo not found: {repo_id}"),
                &[("id", repo_id.to_string())],
            )
        })?;
    Ok(repo.path.clone())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::domain::diff::{DiffHunk, DiffLine};
    use crate::infrastructure::persistence::migrations;
    use rusqlite::Connection;
    use std::fs;

    #[test]
    fn proxy_settings_unset_falls_back_to_default() {
        let ctx = fresh_ctx();
        assert_eq!(
            get_proxy_settings(&ctx).expect("get"),
            ProxySettings::default()
        );
    }

    #[test]
    fn proxy_settings_manual_url_is_normalized_and_persisted() {
        let ctx = fresh_ctx();
        let stored = validate_and_store_proxy_settings(
            &ctx,
            ProxySettings {
                mode: ProxyMode::Manual,
                manual_url: Some(" 127.0.0.1:7890 ".to_string()),
            },
        )
        .expect("manual with valid url");
        assert_eq!(stored.manual_url.as_deref(), Some("http://127.0.0.1:7890"));
        assert_eq!(get_proxy_settings(&ctx).expect("get"), stored);
    }

    #[test]
    fn proxy_settings_manual_rejects_unusable_url() {
        let ctx = fresh_ctx();
        let err = validate_and_store_proxy_settings(
            &ctx,
            ProxySettings {
                mode: ProxyMode::Manual,
                manual_url: Some("ftp://proxy:21".to_string()),
            },
        )
        .expect_err("manual with ftp url must be rejected");
        assert_eq!(err.code(), codes::usecases::PROXY_URL_INVALID);
    }

    #[test]
    fn proxy_settings_manual_with_blank_url_means_no_proxy() {
        let ctx = fresh_ctx();
        let stored = validate_and_store_proxy_settings(
            &ctx,
            ProxySettings {
                mode: ProxyMode::Manual,
                manual_url: Some("   ".to_string()),
            },
        )
        .expect("manual with blank url is allowed");
        assert_eq!(stored.manual_url, None);
    }

    #[test]
    fn proxy_settings_off_keeps_stored_url_but_system_blanks_it() {
        let ctx = fresh_ctx();
        let stored = validate_and_store_proxy_settings(
            &ctx,
            ProxySettings {
                mode: ProxyMode::Off,
                manual_url: Some("http://127.0.0.1:7890".to_string()),
            },
        )
        .expect("off keeps the url");
        assert_eq!(stored.manual_url.as_deref(), Some("http://127.0.0.1:7890"));

        let stored = validate_and_store_proxy_settings(
            &ctx,
            ProxySettings {
                mode: ProxyMode::System,
                manual_url: Some("  ".to_string()),
            },
        )
        .expect("system with blank url");
        assert_eq!(stored.manual_url, None);
    }

    #[test]
    fn workspace_fetch_lock_is_per_workspace() {
        let a1 = workspace_fetch_lock("ws-lock-a");
        let a2 = workspace_fetch_lock("ws-lock-a");
        let b = workspace_fetch_lock("ws-lock-b");
        assert!(Arc::ptr_eq(&a1, &a2), "same workspace must share one lock");
        assert!(!Arc::ptr_eq(&a1, &b), "workspaces must not share locks");
    }

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
            keys.get(provider).cloned().ok_or_else(|| {
                AppError::unknown(
                    codes::usecases::TEST_KEYCHAIN_UNAVAILABLE,
                    "keychain unavailable",
                )
            })
        }
    }

    #[test]
    fn failover_policy_only_walks_network_errors() {
        assert!(should_failover(&AppError::network(
            codes::usecases::TEST_NETWORK,
            "timeout",
        )));
        assert!(!should_failover(&AppError::protocol(
            codes::usecases::TEST_PROTOCOL,
            "bad prompt",
        )));
        assert!(!should_failover(&AppError::credential(
            codes::usecases::TEST_CREDENTIAL,
            "no key",
        )));
        assert!(!should_failover(&AppError::unknown(
            codes::usecases::TEST_UNKNOWN,
            "empty content",
        )));
    }

    #[test]
    fn repo_rules_are_appended_only_when_present() {
        let base = "Base prompt.".to_string();
        assert_eq!(with_repo_rules(base.clone(), None), base);
        assert_eq!(with_repo_rules(base.clone(), Some("   ".into())), base);
        let with = with_repo_rules(base, Some("Keep subjects short.".into()));
        assert!(with.starts_with("Base prompt."));
        assert!(with.contains(".gitwave/AI.md"));
        assert!(with.contains("Keep subjects short."));
    }

    #[test]
    fn split_pr_text_separates_title_and_body() {
        let (title, body) = split_pr_text("feat: add LFS support\n\nSummary.\n- item");
        assert_eq!(title, "feat: add LFS support");
        assert_eq!(body, "Summary.\n- item");
        let (title, body) = split_pr_text("  title only  ");
        assert_eq!(title, "title only");
        assert_eq!(body, "");
        let (title, body) = split_pr_text("\n\ntitle\n");
        assert_eq!(title, "title");
        assert_eq!(body, "");
    }

    #[test]
    fn pr_user_prompt_lists_commits_and_diff() {
        let commits = vec![PrCommit {
            sha: "1234567890abcdef".into(),
            subject: "feat: a".into(),
            message_full: "feat: a\n\nbody".into(),
        }];
        let prompt =
            build_pr_user_prompt("feature/x", "origin/main", &commits, &patch_fixture(None));
        assert!(prompt.contains("Branch: feature/x"));
        assert!(prompt.contains("Base: origin/main"));
        assert!(prompt.contains("Commits (1):"));
        assert!(prompt.contains("- 1234567 feat: a"));
        assert!(
            prompt.contains("feat: a\n\nbody"),
            "style reference included"
        );
        assert!(prompt.contains("--- a/a.txt"), "diff patch included");
    }

    #[test]
    fn explain_prompt_carries_message_files_and_patch() {
        let summary = DiffSummary {
            files: vec![],
            total_additions: 0,
            total_deletions: 0,
        };
        let prompt = build_explain_user_prompt("feat: a\n\nbody", &summary, &patch_fixture(None));
        assert!(prompt.starts_with("Commit message:\nfeat: a\n\nbody\n"));
        assert!(prompt.contains("Changed files:"));
        assert!(prompt.contains("(none)"));
        assert!(prompt.contains("--- a/a.txt"));
    }

    #[test]
    fn palette_intent_parses_tolerantly() {
        let intent = parse_palette_intent(
            "```json\n{\"action\": \"create_branch\", \"params\": {\"name\": \"fix/auth\"}, \
             \"explanation\": \"create a branch\"}\n```",
        )
        .expect("fenced json");
        assert_eq!(intent.action, "create_branch");
        assert!(intent.requires_confirm);
        assert_eq!(intent.params["name"], "fix/auth");

        let intent =
            parse_palette_intent("Sure! {\"action\":\"locate_commit\",\"params\":{\"sha\":\"abc1234\"},\"explanation\":\"jump\"}")
                .expect("prose-wrapped json");
        assert!(!intent.requires_confirm);
        assert_eq!(intent.params["sha"], "abc1234");

        // Missing params object defaults to {} for param-less actions.
        let intent =
            parse_palette_intent("{\"action\":\"fetch_remotes\",\"explanation\":\"fetch\"}")
                .expect("no params");
        assert_eq!(intent.action, "fetch_remotes");
    }

    #[test]
    fn palette_intent_rejects_non_whitelisted_and_malformed() {
        // P1: commit / push / merge / rebase are never palette-driven.
        for action in ["commit", "push", "merge", "rebase", "force_push"] {
            let raw = format!("{{\"action\":\"{action}\",\"params\":{{}},\"explanation\":\"x\"}}");
            let err = parse_palette_intent(&raw).expect_err(action);
            assert!(err.to_string().contains("unsupported action"), "{err}");
        }
        assert!(parse_palette_intent("no json here").is_err());
        assert!(parse_palette_intent("{\"action\":\"locate_commit\"}").is_err());
        let err = parse_palette_intent(
            "{\"action\":\"explain_commit\",\"params\":{},\"explanation\":\"x\"}",
        )
        .expect_err("missing sha");
        assert!(err.to_string().contains("missing \"sha\""), "{err}");
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
        assert!(matches!(err, AppError::Credential { .. }), "got: {err:?}");
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

    #[test]
    fn ai_chain_offline_does_not_demand_cloud_key() {
        // Offline + cloud primary without a stored key must still reach the
        // Ollama fallback — the user disabled cloud calls on purpose.
        let settings = chain_settings(
            Some("openai"),
            vec![AiProviderConfig {
                provider: "ollama".into(),
                model: None,
                base_url: None,
            }],
            true,
        );
        let lookup = key_lookup_for(&[("openai", None)]);
        let chain = resolve_ai_chain(&settings, &lookup).expect("chain reaches ollama");
        assert_eq!(chain.len(), 1);
        assert_eq!(chain[0].provider, "ollama");
    }

    fn fresh_ctx() -> AppContext {
        let conn = Connection::open_in_memory().expect("in-memory sqlite");
        migrations::apply(&conn).expect("migrations");
        let app_settings = SqliteAppSettingsRepo::open_in_memory().expect("in-memory app settings");
        AppContext::new(
            Arc::new(Mutex::new(SqliteWorkspaceRepo::new(conn))),
            Arc::new(Mutex::new(app_settings)),
        )
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
    fn update_settings_normalizes_blank_templates_to_default() {
        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Ws".into()).expect("create");
        let mut settings = WorkspaceSettings::default();
        settings.prompt_templates.commit = Some("   ".into());
        settings.prompt_templates.pr = Some("Custom PR prompt.".into());
        update_workspace_settings(&ctx, ws.id.clone(), settings).expect("update");
        let stored = get_workspace(&ctx, ws.id).expect("get").settings;
        assert_eq!(stored.prompt_templates.commit, None, "blank means default");
        assert_eq!(
            stored.prompt_templates.pr.as_deref(),
            Some("Custom PR prompt.")
        );
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

    // ─── fetch: remote=None fans out to every configured remote ─────────

    use crate::infrastructure::git::test_helpers::{
        build_linear_repo, make_commit, write_and_stage,
    };

    fn uc_sig() -> git2::Signature<'static> {
        git2::Signature::now("Test", "test@local").unwrap()
    }

    #[test]
    fn fetch_without_remote_updates_every_remote() {
        let (origin_path, origin) = build_linear_repo(1);
        let (gitlab_path, gitlab) = build_linear_repo(1);
        let local_path = origin_path.with_extension("clone");
        let local = git2::Repository::clone(origin_path.to_str().unwrap(), &local_path).unwrap();
        crate::infrastructure::git::remote::add_remote(
            &local,
            "gitlab",
            gitlab_path.to_str().unwrap(),
        )
        .unwrap();

        // A new commit on origin (child of what local cloned) plus gitlab's
        // own unrelated tip; neither is known to local before the fetch.
        let base = local.head().unwrap().peel_to_commit().unwrap().id();
        let tree = write_and_stage(&origin, "from-origin.txt", "o\n");
        let origin_tip = make_commit(&origin, &uc_sig(), "origin tip", tree, &[base]);
        let gitlab_tip = gitlab.head().unwrap().peel_to_commit().unwrap().id();

        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).unwrap();
        let repo_ref = add_local_repo(
            &ctx,
            ws.id.clone(),
            local_path.to_string_lossy().to_string(),
        )
        .expect("add_local_repo");
        set_active_repo(&ctx, ws.id.clone(), Some(repo_ref.id)).unwrap();

        fetch(&ctx, &ws.id, None, None, None, None).expect("fetch without a remote name");

        let local = git2::Repository::open(&local_path).unwrap();
        assert_eq!(
            local
                .find_reference("refs/remotes/origin/main")
                .unwrap()
                .target()
                .unwrap(),
            origin_tip,
            "origin's remote-tracking ref must advance"
        );
        assert_eq!(
            local
                .find_reference("refs/remotes/gitlab/main")
                .unwrap()
                .target()
                .unwrap(),
            gitlab_tip,
            "the second remote (gitlab) must be fetched too"
        );

        let _ = fs::remove_dir_all(&origin_path);
        let _ = fs::remove_dir_all(&gitlab_path);
        let _ = fs::remove_dir_all(&local_path);
    }

    #[test]
    fn fetch_without_remote_succeeds_on_repo_without_remotes() {
        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).unwrap();
        let tmp = std::env::temp_dir().join(format!("gitwave-uc-noremote-{}", std::process::id()));
        let _ = fs::remove_dir_all(&tmp);
        crate::infrastructure::git::repo_adapter::init(&tmp).expect("init");
        let repo_ref = add_local_repo(&ctx, ws.id.clone(), tmp.to_string_lossy().to_string())
            .expect("add_local_repo");
        set_active_repo(&ctx, ws.id.clone(), Some(repo_ref.id)).unwrap();

        fetch(&ctx, &ws.id, None, None, None, None).expect("no remotes means a silent no-op");
        cleanup(&tmp);
    }

    #[test]
    fn fetch_without_remote_tries_all_and_reports_first_failure() {
        let (origin_path, origin) = build_linear_repo(1);
        let local_path = origin_path.with_extension("clone");
        let local = git2::Repository::clone(origin_path.to_str().unwrap(), &local_path).unwrap();
        // A remote whose URL is not a repository, listed before origin.
        crate::infrastructure::git::remote::add_remote(
            &local,
            "bad",
            local_path.with_extension("missing").to_str().unwrap(),
        )
        .unwrap();

        let base = local.head().unwrap().peel_to_commit().unwrap().id();
        let tree = write_and_stage(&origin, "still.txt", "o\n");
        let origin_tip = make_commit(&origin, &uc_sig(), "origin tip", tree, &[base]);

        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).unwrap();
        let repo_ref = add_local_repo(
            &ctx,
            ws.id.clone(),
            local_path.to_string_lossy().to_string(),
        )
        .expect("add_local_repo");
        set_active_repo(&ctx, ws.id.clone(), Some(repo_ref.id)).unwrap();

        let err = fetch(&ctx, &ws.id, None, None, None, None)
            .expect_err("bad remote must fail the batch");
        assert_eq!(err.category(), "Network");

        // Best-effort semantics: origin was still fetched despite "bad".
        let local = git2::Repository::open(&local_path).unwrap();
        assert_eq!(
            local
                .find_reference("refs/remotes/origin/main")
                .unwrap()
                .target()
                .unwrap(),
            origin_tip,
            "remaining remotes must still be fetched after a failure"
        );

        let _ = fs::remove_dir_all(&origin_path);
        let _ = fs::remove_dir_all(&local_path);
    }

    #[test]
    fn fetch_all_remotes_stops_immediately_when_cancelled() {
        // A cancelled "fetch all" must abort at the first remote instead of
        // grinding through the batch (the cancel button / timeout path).
        let (origin_path, origin) = build_linear_repo(1);
        let local_path = origin_path.with_extension("clone");
        let local = git2::Repository::clone(origin_path.to_str().unwrap(), &local_path).unwrap();
        // A remote whose URL is not a repository, listed before origin.
        crate::infrastructure::git::remote::add_remote(
            &local,
            "bad",
            local_path.with_extension("missing").to_str().unwrap(),
        )
        .unwrap();

        let base = local.head().unwrap().peel_to_commit().unwrap().id();
        let tree = write_and_stage(&origin, "still.txt", "o\n");
        let origin_tip = make_commit(&origin, &uc_sig(), "origin tip", tree, &[base]);

        let ctx = fresh_ctx();
        let ws = create_workspace(&ctx, "Default".into()).unwrap();
        let repo_ref = add_local_repo(
            &ctx,
            ws.id.clone(),
            local_path.to_string_lossy().to_string(),
        )
        .expect("add_local_repo");
        set_active_repo(&ctx, ws.id.clone(), Some(repo_ref.id)).unwrap();

        let cancel = Arc::new(std::sync::atomic::AtomicBool::new(true)); // pre-set
        let err = fetch(&ctx, &ws.id, None, None, Some(cancel), None)
            .expect_err("cancelled batch must fail");
        assert_eq!(err.code(), crate::domain::error_codes::git::SYNC_CANCELLED);

        // The batch stopped at the first remote; origin was never reached.
        let local = git2::Repository::open(&local_path).unwrap();
        assert_ne!(
            local
                .find_reference("refs/remotes/origin/main")
                .unwrap()
                .target()
                .unwrap(),
            origin_tip,
            "cancel must stop the batch before origin is fetched"
        );

        let _ = fs::remove_dir_all(&origin_path);
        let _ = fs::remove_dir_all(&local_path);
    }
}
