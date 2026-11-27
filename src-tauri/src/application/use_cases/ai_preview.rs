//! Local prompt preparation and single-use preview handles.
use super::*;
use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;
use std::hash::{DefaultHasher, Hash, Hasher};

#[derive(Clone, Deserialize, Hash)]
#[serde(tag = "kind", rename_all = "snake_case")]
pub enum AiTask {
    CommitMessage {
        repo_id: Option<String>,
    },
    Pr {
        base: Option<String>,
    },
    Commit {
        sha: String,
    },
    Conflict {
        path: String,
    },
    Palette {
        query: String,
    },
    Health,
    Reflog {
        old_oid: String,
        new_oid: String,
        action: String,
        message: String,
    },
}

#[derive(Serialize)]
#[serde(untagged)]
pub enum AiReply {
    Generation(AiGenerateOutcome),
    Pr(PrDescriptionOutcome),
    Intent(PaletteIntent),
    Text(String),
}

#[derive(Serialize)]
pub struct Preview {
    pub id: u64,
    pub prompt: PreparedPrompt,
}

#[derive(Default)]
pub(super) struct PreviewStore {
    next_id: u64,
    // Only sanitized prompt text is retained. The scope fingerprint avoids
    // keeping raw palette queries and reflog messages in this cache.
    entries: BTreeMap<u64, (u64, PreparedPrompt)>,
}

fn expired() -> AppError {
    AppError::protocol(
        codes::usecases::AI_PREVIEW_EXPIRED,
        "AI preview expired; open it again",
    )
}

fn repository<'a>(task: &AiTask, ws: &'a Workspace) -> Result<&'a RepoRef> {
    let id = match task {
        AiTask::CommitMessage { repo_id: Some(id) } => id,
        _ => ws.last_active_repo_id.as_ref().ok_or_else(|| {
            AppError::protocol(codes::usecases::NO_ACTIVE_REPO, "no active repository")
        })?,
    };
    ws.repos.iter().find(|repo| &repo.id == id).ok_or_else(|| {
        AppError::protocol_with(
            codes::usecases::REPO_NOT_FOUND,
            "repository not in workspace",
            &[("id", id.clone())],
        )
    })
}

fn scope(
    ctx: &AppContext,
    ws: &Workspace,
    task: &AiTask,
    language: &Option<String>,
) -> Result<u64> {
    let target = repository(task, ws)?;
    let mut hasher = DefaultHasher::new();
    (&ws.id, &target.id, &ws.last_active_repo_id, task, language).hash(&mut hasher);
    // Branch-scoped previews survive edits on that branch. Switching to a
    // different branch (or detached commit) changes the target itself.
    if matches!(task, AiTask::Pr { .. } | AiTask::CommitMessage { .. }) {
        let repo = ctx.open_repo(&target.path)?;
        let head = repo.find_reference("HEAD")?;
        head.symbolic_target_bytes().hash(&mut hasher);
        head.target().hash(&mut hasher);
    }
    Ok(hasher.finish())
}

fn prepare(
    ctx: &AppContext,
    ws: Workspace,
    task: &AiTask,
    language: Option<String>,
) -> Result<PreparedPrompt> {
    let repo = ctx.open_repo(&repository(task, &ws)?.path)?;
    match task.clone() {
        AiTask::CommitMessage { .. } => prepare_commit_message(&repo, ws.settings, language),
        AiTask::Pr { base } => prepare_pr(&repo, ws.settings, base, language),
        AiTask::Commit { sha } => prepare_commit(&repo, sha, language),
        AiTask::Conflict { path } => prepare_conflict(&repo, ws.settings, path, language),
        AiTask::Palette { query } => prepare_palette(&repo, ws.repos, query),
        AiTask::Health => prepare_health(&repo, ws.settings, language),
        AiTask::Reflog {
            old_oid,
            new_oid,
            action,
            message,
        } => prepare_reflog(
            &repo,
            ws.settings,
            old_oid,
            new_oid,
            action,
            message,
            language,
        ),
    }
}

pub fn preview(
    ctx: &AppContext,
    workspace_id: &str,
    task: &AiTask,
    language: Option<String>,
    expected_repo_id: Option<&str>,
) -> Result<Preview> {
    let ws = get_workspace(ctx, workspace_id.to_owned())?;
    if let Some(expected) = expected_repo_id {
        if repository(task, &ws)?.id != expected {
            return Err(expired());
        }
    }
    let binding = scope(ctx, &ws, task, &language)?;
    let prompt = prepare(ctx, ws, task, language)?;
    let mut store = ctx
        .previews
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    // Bound memory across abandoned webviews. A released or evicted handle
    // always fails closed and cannot trigger a freshly assembled request.
    if store.entries.len() >= 16 {
        store.entries.pop_first();
    }
    store.next_id += 1;
    let id = store.next_id;
    store.entries.insert(id, (binding, prompt.clone()));
    Ok(Preview { id, prompt })
}

pub fn discard(ctx: &AppContext, id: u64) {
    ctx.previews
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner)
        .entries
        .remove(&id);
}

pub(super) async fn direct(
    ctx: &AppContext,
    workspace_id: &str,
    task: &AiTask,
    language: Option<String>,
) -> Result<AiGenerateOutcome> {
    let ws = get_workspace(ctx, workspace_id.to_owned())?;
    let chain = chain(&ws, task)?;
    let prompt = prepare(ctx, ws, task, language)?;
    generate_with_failover(chain, prompt).await
}

fn chain(ws: &Workspace, task: &AiTask) -> Result<Vec<ResolvedAiProvider>> {
    let mut settings = ws.settings.clone();
    settings.ai_offline |= repository(task, ws)?
        .settings_override
        .as_ref()
        .is_some_and(|r| r.ai_offline);
    resolve_ai_chain(&settings, &|provider| {
        crate::infrastructure::ai::get_api_key(&ws.id, provider)
    })
}

pub async fn send_preview(
    ctx: &AppContext,
    workspace_id: &str,
    task: &AiTask,
    language: Option<String>,
    id: u64,
) -> Result<AiReply> {
    let (binding, prompt) = ctx
        .previews
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner)
        .entries
        .remove(&id)
        .ok_or_else(expired)?;
    let ws = get_workspace(ctx, workspace_id.to_owned())?;
    if scope(ctx, &ws, task, &language)? != binding {
        return Err(expired());
    }
    let outcome = generate_with_failover(chain(&ws, task)?, prompt).await?;
    Ok(match task {
        AiTask::Pr { .. } => AiReply::Pr(pr_description(outcome)),
        AiTask::Palette { .. } => AiReply::Intent(parse_palette_intent(&outcome.text)?),
        AiTask::Reflog { .. } | AiTask::Health => AiReply::Text(outcome.text),
        _ => AiReply::Generation(outcome),
    })
}

#[cfg(test)]
mod offline_tests {
    use super::*;
    #[test]
    fn effective_offline_uses_the_actual_task_repository_and_workspace_can_always_force_it() {
        let repo = |id: &str, offline| RepoRef {
            id: id.into(),
            workspace_id: "ws".into(),
            path: id.into(),
            nickname: None,
            settings_override: Some(WorkspaceSettings {
                ai_offline: offline,
                ..Default::default()
            }),
            status: RepoStatus::Active,
            missing_since: None,
            added_at: 0,
        };
        let mut ws = Workspace {
            id: "ws".into(),
            name: "Test".into(),
            last_active_repo_id: Some("active".into()),
            repos: vec![repo("active", false), repo("restricted", true)],
            settings: WorkspaceSettings {
                ai_provider: Some("ollama".into()),
                ..Default::default()
            },
            created_at: 0,
            updated_at: 0,
        };
        let explicit = AiTask::CommitMessage {
            repo_id: Some("restricted".into()),
        };
        assert!(!chain(&ws, &AiTask::Health).unwrap()[0].offline);
        assert!(chain(&ws, &explicit).unwrap()[0].offline);
        ws.settings.ai_offline = true;
        assert!(chain(&ws, &AiTask::Health).unwrap()[0].offline);
        ws.settings.ai_base_url = Some("https://remote.example".into());
        assert_eq!(
            chain(&ws, &AiTask::Health).unwrap_err().code(),
            codes::usecases::AI_OFFLINE_MODE
        );
    }
}
