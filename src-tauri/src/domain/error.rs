//! Top-level error type for GitWave domain and application layers.
//!
//! `Serialize` so it crosses the Tauri IPC boundary as structured JSON
//! (category / message / trace_id) and is rendered with friendly text on the
//! frontend. Categories follow `docs/tech/engineering/00-overview.md` §
//! 错误处理与日志.

use serde::Serialize;

#[derive(Debug, thiserror::Error)]
pub enum AppError {
    #[error("network error: {0}")]
    Network(String),

    #[error("credential error: {0}")]
    Credential(String),

    #[error("permission error: {0}")]
    Permission(String),

    #[error("version conflict: {0}")]
    VersionConflict(String),

    #[error("protocol error: {0}")]
    Protocol(String),

    #[error("unknown error: {0}")]
    Unknown(String),
}

impl From<git2::Error> for AppError {
    fn from(e: git2::Error) -> Self {
        AppError::Unknown(format!("git: {e}"))
    }
}

impl AppError {
    /// Short string tag for UI categorization.
    #[must_use]
    pub fn category(&self) -> &'static str {
        match self {
            Self::Network(_) => "Network",
            Self::Credential(_) => "Credential",
            Self::Permission(_) => "Permission",
            Self::VersionConflict(_) => "VersionConflict",
            Self::Protocol(_) => "Protocol",
            Self::Unknown(_) => "Unknown",
        }
    }

    /// Short trace id for log ↔ UI correlation.
    #[must_use]
    pub fn trace_id(&self) -> String {
        use std::hash::{Hash, Hasher};
        let mut h = std::collections::hash_map::DefaultHasher::new();
        self.to_string().hash(&mut h);
        format!("{:x}", h.finish())
    }
}

impl Serialize for AppError {
    fn serialize<S>(&self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        use serde::ser::SerializeStruct;
        let mut s = serializer.serialize_struct("AppError", 3)?;
        s.serialize_field("category", self.category())?;
        s.serialize_field("message", &self.to_string())?;
        s.serialize_field("trace_id", &self.trace_id())?;
        s.end()
    }
}

/// Result alias for fallible operations across GitWave.
pub type Result<T> = std::result::Result<T, AppError>;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn category_returns_expected_tag() {
        assert_eq!(AppError::Network("x".into()).category(), "Network");
        assert_eq!(AppError::Credential("x".into()).category(), "Credential");
        assert_eq!(AppError::Permission("x".into()).category(), "Permission");
        assert_eq!(
            AppError::VersionConflict("x".into()).category(),
            "VersionConflict"
        );
        assert_eq!(AppError::Protocol("x".into()).category(), "Protocol");
        assert_eq!(AppError::Unknown("x".into()).category(), "Unknown");
    }

    #[test]
    fn trace_id_is_stable_for_same_payload() {
        let a = AppError::Network("boom".into()).trace_id();
        let b = AppError::Network("boom".into()).trace_id();
        assert_eq!(a, b);
    }

    #[test]
    fn serializes_to_structured_json() {
        let err = AppError::Credential("bad token".into());
        let json = serde_json::to_string(&err).expect("serialize");
        assert!(json.contains("\"category\":\"Credential\""));
        assert!(json.contains("\"message\":\"credential error: bad token\""));
        assert!(json.contains("\"trace_id\":\""));
    }
}
