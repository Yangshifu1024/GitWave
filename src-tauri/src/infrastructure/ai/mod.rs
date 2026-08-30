//! AI provider adapters — BYOK cloud + local Ollama.
//!
//! API keys never leave the OS keychain / credential store. Diff text is
//! scrubbed before any network call. Offline mode blocks cloud providers.

pub mod language;
pub mod provider;
pub mod rules;
pub mod scrubber;
pub mod secrets;

pub use language::{sanitize as sanitize_ai_language, with_reply_language};
pub use provider::{generate_text, probe_ollama, AiGenerateRequest};
pub use rules::read_ai_rules;
pub use scrubber::scrub_secrets;
pub use secrets::{clear_api_key, get_api_key, has_api_key, set_api_key};
