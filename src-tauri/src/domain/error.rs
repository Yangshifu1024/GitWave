//! Top-level error type for GitWave domain and application layers.
//!
//! `Serialize` so it crosses the Tauri IPC boundary as structured JSON
//! (category / message / trace_id, plus optional code / params) and is
//! rendered with friendly text on the frontend: when `code` is present the
//! UI looks up `errors.<code>` in the active locale (interpolating
//! `params`), otherwise it falls back to the English `message`. Categories
//! follow `docs/tech/engineering/00-overview.md` § 错误处理与日志.

use serde::Serialize;
use std::collections::BTreeMap;

/// Stable i18n error code (`"area.name_style"`), from `domain/error_codes`.
pub type ErrorCode = &'static str;

/// Interpolation params for the translated message (`{{name}}` on the UI
/// side). The English `message` keeps the same values inline as fallback.
pub type ErrorParams = Vec<(&'static str, String)>;

#[derive(Debug, thiserror::Error)]
pub enum AppError {
    #[error("network error: {message}")]
    Network {
        code: ErrorCode,
        message: String,
        params: ErrorParams,
    },

    #[error("credential error: {message}")]
    Credential {
        code: ErrorCode,
        message: String,
        params: ErrorParams,
    },

    #[error("permission error: {message}")]
    Permission {
        code: ErrorCode,
        message: String,
        params: ErrorParams,
    },

    #[error("version conflict: {message}")]
    VersionConflict {
        code: ErrorCode,
        message: String,
        params: ErrorParams,
    },

    #[error("protocol error: {message}")]
    Protocol {
        code: ErrorCode,
        message: String,
        params: ErrorParams,
    },

    #[error("unknown error: {message}")]
    Unknown {
        code: ErrorCode,
        message: String,
        params: ErrorParams,
    },
}

impl From<git2::Error> for AppError {
    fn from(e: git2::Error) -> Self {
        // Raw libgit2 text is English and untranslatable — it rides as the
        // `detail` param so the UI can localize the framing around it.
        AppError::unknown_with(
            crate::domain::error_codes::git::RAW,
            format!("git: {e}"),
            &[("detail", e.to_string())],
        )
    }
}

impl AppError {
    /// Shorthand constructor without interpolation params.
    #[must_use]
    pub fn network(code: ErrorCode, message: impl Into<String>) -> Self {
        Self::network_with(code, message, &[])
    }

    /// Constructor with interpolation params for the UI translation.
    #[must_use]
    pub fn network_with(
        code: ErrorCode,
        message: impl Into<String>,
        params: &[(&'static str, String)],
    ) -> Self {
        Self::Network {
            code,
            message: message.into(),
            params: params.to_vec(),
        }
    }

    #[must_use]
    pub fn credential(code: ErrorCode, message: impl Into<String>) -> Self {
        Self::credential_with(code, message, &[])
    }

    #[must_use]
    pub fn credential_with(
        code: ErrorCode,
        message: impl Into<String>,
        params: &[(&'static str, String)],
    ) -> Self {
        Self::Credential {
            code,
            message: message.into(),
            params: params.to_vec(),
        }
    }

    #[must_use]
    pub fn permission(code: ErrorCode, message: impl Into<String>) -> Self {
        Self::permission_with(code, message, &[])
    }

    #[must_use]
    pub fn permission_with(
        code: ErrorCode,
        message: impl Into<String>,
        params: &[(&'static str, String)],
    ) -> Self {
        Self::Permission {
            code,
            message: message.into(),
            params: params.to_vec(),
        }
    }

    #[must_use]
    pub fn version_conflict(code: ErrorCode, message: impl Into<String>) -> Self {
        Self::version_conflict_with(code, message, &[])
    }

    #[must_use]
    pub fn version_conflict_with(
        code: ErrorCode,
        message: impl Into<String>,
        params: &[(&'static str, String)],
    ) -> Self {
        Self::VersionConflict {
            code,
            message: message.into(),
            params: params.to_vec(),
        }
    }

    #[must_use]
    pub fn protocol(code: ErrorCode, message: impl Into<String>) -> Self {
        Self::protocol_with(code, message, &[])
    }

    #[must_use]
    pub fn protocol_with(
        code: ErrorCode,
        message: impl Into<String>,
        params: &[(&'static str, String)],
    ) -> Self {
        Self::Protocol {
            code,
            message: message.into(),
            params: params.to_vec(),
        }
    }

    #[must_use]
    pub fn unknown(code: ErrorCode, message: impl Into<String>) -> Self {
        Self::unknown_with(code, message, &[])
    }

    #[must_use]
    pub fn unknown_with(
        code: ErrorCode,
        message: impl Into<String>,
        params: &[(&'static str, String)],
    ) -> Self {
        Self::Unknown {
            code,
            message: message.into(),
            params: params.to_vec(),
        }
    }

    /// Short string tag for UI categorization.
    #[must_use]
    pub fn category(&self) -> &'static str {
        match self {
            Self::Network { .. } => "Network",
            Self::Credential { .. } => "Credential",
            Self::Permission { .. } => "Permission",
            Self::VersionConflict { .. } => "VersionConflict",
            Self::Protocol { .. } => "Protocol",
            Self::Unknown { .. } => "Unknown",
        }
    }

    /// Stable i18n code for this error.
    #[must_use]
    pub fn code(&self) -> &'static str {
        match self {
            Self::Network { code, .. }
            | Self::Credential { code, .. }
            | Self::Permission { code, .. }
            | Self::VersionConflict { code, .. }
            | Self::Protocol { code, .. }
            | Self::Unknown { code, .. } => code,
        }
    }

    /// English fallback message (category-prefixed), shown when the UI has
    /// no translation for `code`.
    #[must_use]
    pub fn message(&self) -> String {
        self.to_string()
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

/// Wire shape across IPC. `code` is always present (every constructor takes
/// one); empty `params` is omitted to keep payloads lean. The UI falls back
/// to the English `message` whenever it lacks a translation for `code`.
#[derive(Serialize)]
struct AppErrorDto<'a> {
    category: &'a str,
    message: String,
    trace_id: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    code: Option<&'a str>,
    #[serde(skip_serializing_if = "Option::is_none")]
    params: Option<BTreeMap<&'a str, &'a str>>,
}

impl Serialize for AppError {
    fn serialize<'a, S>(&'a self, serializer: S) -> std::result::Result<S::Ok, S::Error>
    where
        S: serde::Serializer,
    {
        let (code, params) = match self {
            Self::Network { code, params, .. }
            | Self::Credential { code, params, .. }
            | Self::Permission { code, params, .. }
            | Self::VersionConflict { code, params, .. }
            | Self::Protocol { code, params, .. }
            | Self::Unknown { code, params, .. } => (Some(*code), Some(params)),
        };
        let params = params.and_then(|ps| {
            if ps.is_empty() {
                None
            } else {
                Some(
                    ps.iter()
                        .map(|(key, value)| (*key, value.as_str()))
                        .collect::<BTreeMap<&'a str, &'a str>>(),
                )
            }
        });
        let dto = AppErrorDto {
            category: self.category(),
            message: self.to_string(),
            trace_id: self.trace_id(),
            code,
            params,
        };
        dto.serialize(serializer)
    }
}

/// Result alias for fallible operations across GitWave.
pub type Result<T> = std::result::Result<T, AppError>;

#[cfg(test)]
mod tests {
    use super::*;

    const TEST_CODE: ErrorCode = "test.boom";

    #[test]
    fn category_returns_expected_tag() {
        assert_eq!(AppError::network(TEST_CODE, "x").category(), "Network");
        assert_eq!(
            AppError::credential(TEST_CODE, "x").category(),
            "Credential"
        );
        assert_eq!(
            AppError::permission(TEST_CODE, "x").category(),
            "Permission"
        );
        assert_eq!(
            AppError::version_conflict(TEST_CODE, "x").category(),
            "VersionConflict"
        );
        assert_eq!(AppError::protocol(TEST_CODE, "x").category(), "Protocol");
        assert_eq!(AppError::unknown(TEST_CODE, "x").category(), "Unknown");
    }

    #[test]
    fn code_round_trips() {
        assert_eq!(AppError::protocol(TEST_CODE, "x").code(), TEST_CODE);
        assert_eq!(
            AppError::protocol_with(TEST_CODE, "x", &[("name", "n".into())]).code(),
            TEST_CODE
        );
    }

    #[test]
    fn trace_id_is_stable_for_same_payload() {
        let a = AppError::network(TEST_CODE, "boom").trace_id();
        let b = AppError::network(TEST_CODE, "boom").trace_id();
        assert_eq!(a, b);
    }

    #[test]
    fn serializes_to_structured_json() {
        let err = AppError::credential(TEST_CODE, "bad token");
        let json = serde_json::to_string(&err).expect("serialize");
        assert!(json.contains("\"category\":\"Credential\""));
        assert!(json.contains("\"message\":\"credential error: bad token\""));
        assert!(json.contains("\"trace_id\":\""));
        assert!(json.contains("\"code\":\"test.boom\""));
        assert!(!json.contains("params"), "no params key when empty: {json}");
    }

    #[test]
    fn serializes_params_as_object() {
        let err = AppError::unknown_with(TEST_CODE, "boom", &[("name", "origin".into())]);
        let json = serde_json::to_string(&err).expect("serialize");
        assert!(json.contains("\"params\":{\"name\":\"origin\"}"), "{json}");
    }
}
