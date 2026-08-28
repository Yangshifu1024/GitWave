//! Workspace domain model — see `docs/pm/core/01-features.md` §1.4.
//!
//! Workspace is an abstract, persistent collection of Git repositories plus
//! shared configuration. It has no filesystem entity.

use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct Workspace {
    pub id: String,
    pub name: String,
    pub repos: Vec<RepoRef>,
    pub settings: WorkspaceSettings,
    pub last_active_repo_id: Option<String>,
    pub created_at: i64,
    pub updated_at: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq, Default)]
pub struct WorkspaceSettings {
    /// `openai` | `anthropic` | `ollama`
    pub ai_provider: Option<String>,
    /// Model id, e.g. `gpt-4o-mini`, `claude-3-5-haiku-latest`, `llama3.2`
    #[serde(default)]
    pub ai_model: Option<String>,
    /// API base URL override (provider-specific default when unset).
    #[serde(default)]
    pub ai_base_url: Option<String>,
    /// PM 1.6 offline mode: when true, cloud AI calls are refused; local
    /// Ollama keeps working.
    #[serde(default)]
    pub ai_offline: bool,
    /// Ordered fallback providers tried after `ai_provider` when a request
    /// fails with a network-level error. The primary stays the chain head
    /// so pre-v0.2 settings keep working unchanged.
    #[serde(default)]
    pub ai_failover: Vec<AiProviderConfig>,
    pub prompt_templates: PromptTemplates,
    pub commit_convention: Option<String>,
    pub theme_override: Option<String>,
    pub key_binding_profile: Option<String>,
}

/// One fallback entry in the AI provider chain.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq, Default)]
pub struct AiProviderConfig {
    /// `openai` | `anthropic` | `ollama`
    pub provider: String,
    /// Model id; provider-specific default when unset.
    #[serde(default)]
    pub model: Option<String>,
    /// API base URL override (provider-specific default when unset).
    #[serde(default)]
    pub base_url: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq, Default)]
pub struct PromptTemplates {
    pub commit: Option<String>,
    pub conflict: Option<String>,
    pub pr: Option<String>,
}

/// Lifecycle status of a repo reference. `Missing` indicates the repo's
/// `path` no longer points at a valid git working tree; the user can
/// relink it via `WorkspaceRepository::relink_repo`.
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq, Default)]
#[serde(rename_all = "snake_case")]
pub enum RepoStatus {
    #[default]
    Active,
    Missing,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct RepoRef {
    pub id: String,
    pub workspace_id: String,
    pub path: String,
    pub nickname: Option<String>,
    pub settings_override: Option<WorkspaceSettings>,
    #[serde(default)]
    pub status: RepoStatus,
    #[serde(default)]
    pub missing_since: Option<i64>,
    pub added_at: i64,
}

/// Lightweight projection of `RepoRef` for sidebar/listing UIs.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct RepoSummary {
    pub id: String,
    pub workspace_id: String,
    pub path: String,
    pub nickname: Option<String>,
    pub status: RepoStatus,
    pub is_active: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct WorkspaceSummary {
    pub id: String,
    pub name: String,
    pub last_active_repo_id: Option<String>,
    pub updated_at: i64,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn workspace_serialization_roundtrip() {
        let ws = Workspace {
            id: "ws-1".into(),
            name: "Default".into(),
            repos: vec![],
            settings: WorkspaceSettings::default(),
            last_active_repo_id: None,
            created_at: 0,
            updated_at: 0,
        };
        let json = serde_json::to_string(&ws).expect("serialize");
        let back: Workspace = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(ws, back);
    }

    #[test]
    fn workspace_settings_default_is_empty() {
        let s = WorkspaceSettings::default();
        assert!(s.ai_provider.is_none());
        assert!(s.prompt_templates.commit.is_none());
        assert!(s.prompt_templates.conflict.is_none());
        assert!(s.prompt_templates.pr.is_none());
        assert!(s.commit_convention.is_none());
        assert!(s.theme_override.is_none());
        assert!(s.key_binding_profile.is_none());
        assert!(s.ai_failover.is_empty());
    }

    #[test]
    fn settings_without_failover_field_still_deserializes() {
        // Pre-v0.2 settings_json has no ai_failover — smooth upgrade.
        let s: WorkspaceSettings =
            serde_json::from_str(r#"{"ai_provider":"openai","prompt_templates":{}}"#)
                .expect("deserialize legacy settings");
        assert_eq!(s.ai_provider.as_deref(), Some("openai"));
        assert!(s.ai_failover.is_empty());
    }

    #[test]
    fn failover_chain_roundtrips() {
        let s = WorkspaceSettings {
            ai_provider: Some("openai".into()),
            ai_failover: vec![
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
            ..WorkspaceSettings::default()
        };
        let json = serde_json::to_string(&s).expect("serialize");
        let back: WorkspaceSettings = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(s, back);
    }

    #[test]
    fn workspace_summary_roundtrip() {
        let summary = WorkspaceSummary {
            id: "ws-1".into(),
            name: "Default".into(),
            last_active_repo_id: Some("r-1".into()),
            updated_at: 1000,
        };
        let json = serde_json::to_string(&summary).expect("serialize");
        let back: WorkspaceSummary = serde_json::from_str(&json).expect("deserialize");
        assert_eq!(summary, back);
    }

    #[test]
    fn repo_ref_defaults_status_to_active() {
        let r: RepoRef = serde_json::from_str(
            r#"{"id":"r-1","workspace_id":"ws-1","path":"/tmp","added_at":1000}"#,
        )
        .expect("deserialize");
        assert_eq!(r.status, RepoStatus::Active);
        assert_eq!(r.missing_since, None);
    }

    #[test]
    fn repo_status_serializes_snake_case() {
        assert_eq!(
            serde_json::to_string(&RepoStatus::Missing).unwrap(),
            "\"missing\""
        );
        assert_eq!(
            serde_json::to_string(&RepoStatus::Active).unwrap(),
            "\"active\""
        );
    }
}
