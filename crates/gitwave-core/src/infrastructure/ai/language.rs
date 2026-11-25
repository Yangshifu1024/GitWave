//! AI reply language (F010).
//!
//! The app's global AI-language preference rides each prose-producing
//! command as an optional IPC argument. The prompt body stays English —
//! instruction-following is most reliable in English — and only a trailing
//! directive governs the reply language.

/// Languages the Settings UI offers; anything else sanitizes to `None`.
pub const AI_LANGUAGES: [&str; 4] = ["en", "zh", "ja", "ko"];

/// Sanitize a client-supplied language tag. Unknown values behave like the
/// default (no directive) instead of erroring — the preference is advisory.
#[must_use]
pub fn sanitize(value: Option<&str>) -> Option<&'static str> {
    let value = value?;
    AI_LANGUAGES.iter().find(|tag| **tag == value).copied()
}

/// Reply-language directive for a sanitized language tag.
#[must_use]
pub fn reply_directive(language: Option<&str>) -> Option<String> {
    match language {
        Some("zh") => Some("Always respond in Chinese (简体中文).".into()),
        Some("ja") => Some("Always respond in Japanese (日本語).".into()),
        Some("ko") => Some("Always respond in Korean (한국어).".into()),
        // Explicit even for English so the model never drifts mid-reply.
        Some("en") => Some("Always respond in English.".into()),
        _ => None,
    }
}

/// Append the reply-language directive to a system prompt. No-op when the
/// language is absent or unknown — byte-identical to the legacy prompt.
#[must_use]
pub fn with_reply_language(system: String, language: Option<&str>) -> String {
    match reply_directive(language) {
        Some(directive) => format!("{system}\n\n{directive}"),
        None => system,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn sanitize_accepts_supported_languages() {
        assert_eq!(sanitize(Some("en")), Some("en"));
        assert_eq!(sanitize(Some("zh")), Some("zh"));
        assert_eq!(sanitize(Some("ja")), Some("ja"));
        assert_eq!(sanitize(Some("ko")), Some("ko"));
    }

    #[test]
    fn sanitize_rejects_unknown_and_absent() {
        assert_eq!(sanitize(None), None);
        assert_eq!(sanitize(Some("fr")), None);
        assert_eq!(sanitize(Some("zh-CN")), None, "UI locales are not AI tags");
        assert_eq!(sanitize(Some("")), None);
    }

    #[test]
    fn directive_per_language() {
        assert_eq!(
            reply_directive(Some("zh")).as_deref(),
            Some("Always respond in Chinese (简体中文).")
        );
        assert_eq!(
            reply_directive(Some("ja")).as_deref(),
            Some("Always respond in Japanese (日本語).")
        );
        assert_eq!(
            reply_directive(Some("ko")).as_deref(),
            Some("Always respond in Korean (한국어).")
        );
        assert_eq!(
            reply_directive(Some("en")).as_deref(),
            Some("Always respond in English.")
        );
        assert_eq!(reply_directive(None), None);
    }

    #[test]
    fn with_reply_language_appends_directive() {
        let out = with_reply_language("Be terse.".into(), Some("zh"));
        assert_eq!(out, "Be terse.\n\nAlways respond in Chinese (简体中文).");
    }

    #[test]
    fn with_reply_language_noop_without_language() {
        let base = "Be terse.";
        assert_eq!(with_reply_language(base.into(), None), base);
        assert_eq!(with_reply_language(base.into(), Some("fr")), base);
    }
}
