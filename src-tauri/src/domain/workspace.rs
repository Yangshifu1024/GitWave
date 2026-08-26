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
    pub ai_provider: Option<String>,
    pub prompt_templates: PromptTemplates,
    pub commit_convention: Option<String>,
    pub theme_override: Option<String>,
    pub key_binding_profile: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq, Default)]
pub struct PromptTemplates {
    pub commit: Option<String>,
    pub conflict: Option<String>,
    pub pr: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct RepoRef {
    pub id: String,
    pub path: String,
    pub nickname: Option<String>,
    pub settings_override: Option<WorkspaceSettings>,
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
    }
}
