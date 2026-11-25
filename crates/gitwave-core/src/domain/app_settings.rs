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
