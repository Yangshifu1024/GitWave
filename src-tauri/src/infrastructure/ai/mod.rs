//! AI provider adapters — BYOK cloud + local Ollama.
//!
//! API keys never leave the OS keychain / credential store. Diff text is
//! scrubbed before any network call. Offline mode blocks cloud providers.

pub mod provider;
pub mod secrets;
pub mod scrubber;

pub use provider::{generate_text, probe_ollama, AiGenerateRequest};
pub use secrets::{clear_api_key, get_api_key, has_api_key, set_api_key};
pub use scrubber::scrub_secrets;
