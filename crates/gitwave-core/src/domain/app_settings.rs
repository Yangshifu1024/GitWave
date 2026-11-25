//! App-level global settings (F013).
//!
//! Application-wide configuration that is not Workspace-scoped. Persisted as
//! a JSON blob per key in the `app_settings` table — the same pattern the
//! per-Workspace `settings_json` column uses, so adding keys never needs a
//! migration.

use serde::{Deserialize, Serialize};

/// Which proxy source the app uses for its outbound network paths
/// (AI requests, git fetch/push/clone, LFS, update checks).
#[derive(Debug, Clone, Copy, Serialize, Deserialize, PartialEq, Eq, Default)]
#[serde(rename_all = "snake_case")]
pub enum ProxyMode {
    /// Follow the OS proxy: Windows Internet Settings, macOS system network
    /// proxies; Linux's system convention is environment variables, which
    /// apply unchanged.
    #[default]
    System,
    /// Use the user-supplied proxy URL (`manual_url`).
    Manual,
    /// App injects no proxy of its own (explicitly set env vars still work —
    /// they are user intent outside the app).
    Off,
}

/// The `proxy` entry of app settings.
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq, Default)]
pub struct ProxySettings {
    #[serde(default)]
    pub mode: ProxyMode,
    /// Proxy URL for `ProxyMode::Manual`, e.g. `http://127.0.0.1:7890`.
    /// Blank/None in other modes. Normalized on save (scheme added, trailing
    /// slash trimmed).
    #[serde(default)]
    pub manual_url: Option<String>,
}

/// The `language` entry of app settings: which language the interface shows.
///
/// Which languages there are is the presentation layer's business. The core only
/// keeps the user's choice, as a language tag such as `"en"` or `"zh-CN"`;
/// `None` means "follow the system".
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq, Default)]
pub struct LanguageSettings {
    #[serde(default)]
    pub locale: Option<String>,
}

/// Reads a language tag the way the settings accept it. Whitespace around the
/// tag is dropped and a blank tag means "follow the system" (`Ok(None)`). Any
/// other tag has to look like one: one to three subtags of one to eight ASCII
/// letters or digits, separated by hyphens, the first starting with a letter.
/// The message of the error names the tag that was refused.
pub fn normalize_language_tag(tag: &str) -> Result<Option<String>, String> {
    let tag = tag.trim();
    if tag.is_empty() {
        return Ok(None);
    }
    if is_language_tag(tag) {
        Ok(Some(tag.to_string()))
    } else {
        Err(format!("not a language tag: {tag:?}"))
    }
}

fn is_language_tag(tag: &str) -> bool {
    let subtags: Vec<&str> = tag.split('-').collect();
    (1..=3).contains(&subtags.len())
        && subtags.iter().enumerate().all(|(index, subtag)| {
            (1..=8).contains(&subtag.len())
                && subtag
                    .chars()
                    .all(|character| character.is_ascii_alphanumeric())
                && (index > 0
                    || subtag.starts_with(|character: char| character.is_ascii_alphabetic()))
        })
}
