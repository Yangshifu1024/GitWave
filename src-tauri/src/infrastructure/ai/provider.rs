//! HTTP clients for OpenAI / Anthropic / Ollama chat completions.

use std::sync::{Arc, RwLock};

use serde_json::{json, Value};

use crate::domain::error::{AppError, Result};
use crate::domain::error_codes as codes;
use crate::infrastructure::ai::scrubber::scrub_secrets;

/// Hand-rolled `Debug`: `api_key` must never hit logs even if a future
/// `{:?}` sneaks in (cf. credentials.rs `InlineAuth` masking).
#[derive(Clone)]
pub struct AiGenerateRequest {
    pub provider: String,
    pub model: String,
    pub base_url: Option<String>,
    pub api_key: Option<String>,
    pub system: String,
    pub user: String,
    /// Ordered fallback providers tried after `provider` fails. Empty = no
    /// failover. Base URLs/keys are per attempt (resolved by the caller).
    pub fallbacks: Vec<ProviderAttempt>,
}

impl std::fmt::Debug for AiGenerateRequest {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("AiGenerateRequest")
            .field("provider", &self.provider)
            .field("model", &self.model)
            .field("base_url", &self.base_url)
            .field("api_key", &self.api_key.as_ref().map(|_| "***"))
            .field("fallbacks", &self.fallbacks)
            .finish_non_exhaustive()
    }
}

/// One failover attempt: provider identity + its resolved credentials.
#[derive(Clone)]
pub struct ProviderAttempt {
    pub provider: String,
    pub base_url: Option<String>,
    pub api_key: Option<String>,
    /// Model id valid for THIS provider — model namespaces do not mix
    /// across vendors, so each attempt resolves its own.
    pub model: String,
}

impl std::fmt::Debug for ProviderAttempt {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("ProviderAttempt")
            .field("provider", &self.provider)
            .field("base_url", &self.base_url)
            .field("api_key", &self.api_key.as_ref().map(|_| "***"))
            .field("model", &self.model)
            .finish()
    }
}

fn trim_base(base: &str) -> String {
    base.trim().trim_end_matches('/').to_string()
}

/// Map an HTTP error response to an AppError. Auth failures (401/403)
/// become `Credential` so the failover chain STOPS and surfaces the root
/// cause instead of masking it behind later network errors; client errors
/// (400/422) become `Protocol` (retrying is pointless); every other HTTP
/// failure stays `Network` (the chain may retry elsewhere, incl. 429/5xx).
///
/// `message` carries only a truncated detail: it flows into the local JSON
/// log via `tracing::warn!(error = %e)`, and provider error bodies can echo
/// the offending input. The full detail stays in `params` for the UI.
fn http_error(provider: &str, status: reqwest::StatusCode, detail: &str) -> AppError {
    let short: String = detail.chars().take(200).collect();
    let message = format!("{provider} HTTP {status}: {short}");
    let params = [
        ("provider", provider.to_string()),
        ("status", status.to_string()),
        ("detail", detail.to_string()),
    ];
    use reqwest::StatusCode as S;
    if status == S::UNAUTHORIZED || status == S::FORBIDDEN {
        AppError::credential_with(codes::infra::PROVIDER_HTTP, message, &params)
    } else if status == S::BAD_REQUEST || status == S::UNPROCESSABLE_ENTITY {
        AppError::protocol_with(codes::infra::PROVIDER_HTTP, message, &params)
    } else {
        AppError::network_with(codes::infra::PROVIDER_HTTP, message, &params)
    }
}

/// Cloud-vendor base URLs must be https: the API key travels in
/// `Authorization` / `x-api-key` headers and must never go out in cleartext
/// because of a `http://` gateway typo. Loopback stays allowed (local LiteLLM
/// / proxy gateways); Ollama is loopback-only by design and unchecked.
fn require_https_url(trimmed: &str) -> Result<String> {
    let url = reqwest::Url::parse(trimmed).map_err(|e| {
        AppError::protocol_with(
            codes::infra::UNSUPPORTED_PROVIDER,
            format!("invalid AI base_url: {e}"),
            &[("base_url", trimmed.to_string())],
        )
    })?;
    let host = url.host_str().unwrap_or("");
    let loopback = host == "localhost" || host == "127.0.0.1" || host == "::1";
    if url.scheme() != "https" && !loopback {
        return Err(AppError::protocol_with(
            codes::infra::UNSUPPORTED_PROVIDER,
            format!(
                "AI base_url must use https (got scheme {:?}); localhost gateways stay allowed",
                url.scheme()
            ),
            &[("base_url", trimmed.to_string())],
        ));
    }
    Ok(trimmed.to_string())
}

fn openai_endpoint(base: Option<String>) -> Result<String> {
    let base = require_https_url(&trim_base(
        base.as_deref().unwrap_or("https://api.openai.com"),
    ))?;
    if base.ends_with("/chat/completions") {
        return Ok(base);
    }
    if base.ends_with("/v1") {
        return Ok(format!("{}/chat/completions", base));
    }
    Ok(format!("{}/v1/chat/completions", base))
}

fn anthropic_endpoint(base: Option<String>) -> Result<String> {
    let base = require_https_url(&trim_base(
        base.as_deref().unwrap_or("https://api.anthropic.com"),
    ))?;
    if base.ends_with("/messages") {
        return Ok(base);
    }
    if base.ends_with("/v1") {
        return Ok(format!("{}/messages", base));
    }
    Ok(format!("{}/v1/messages", base))
}

fn ollama_base(base: Option<String>) -> String {
    trim_base(base.as_deref().unwrap_or("http://127.0.0.1:11434"))
}

/// Shared AI HTTP client slot. `None` = rebuild on next use (see
/// [`rebuild_http_client`]).
static AI_HTTP_CLIENT: RwLock<Option<Arc<reqwest::Client>>> = RwLock::new(None);

/// Shared HTTP client: fixed request timeout (a hung provider must not
/// freeze the UI's generate flow) instead of a fresh client per call.
///
/// Rebuildable (F013): saving proxy settings rewrites the process env the
/// client's proxy config was built from, so [`rebuild_http_client`] drops
/// this one and the next request constructs a fresh client.
/// Build a fresh shared client config. Extracted from [`client`] so the
/// build-failure path (e.g. illegal proxy env) is unit-testable without
/// touching the process-wide slot.
fn build_client() -> Result<reqwest::Client> {
    reqwest::Client::builder()
        .timeout(std::time::Duration::from_secs(60))
        .build()
        .map_err(|e| {
            AppError::network_with(
                codes::infra::AI_CLIENT_BUILD,
                format!("ai http client build failed: {e}"),
                &[("error", e.to_string())],
            )
        })
}

fn client() -> Result<Arc<reqwest::Client>> {
    // A poisoned lock means a previous build panicked — recover with the
    // inner value instead of cascading the panic into every AI call.
    // A failed build (e.g. illegal proxy env) is a plain error for the same
    // reason: it must never poison this process-wide slot.
    if let Some(client) = AI_HTTP_CLIENT
        .read()
        .unwrap_or_else(std::sync::PoisonError::into_inner)
        .as_ref()
    {
        return Ok(Arc::clone(client));
    }
    let mut guard = AI_HTTP_CLIENT
        .write()
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    if let Some(client) = guard.as_ref() {
        return Ok(Arc::clone(client));
    }
    let built = build_client()?;
    let shared = Arc::new(built);
    *guard = Some(Arc::clone(&shared));
    Ok(shared)
}

/// Drop the shared AI HTTP client so the next request rebuilds it (picking
/// up the current process env, i.e. proxy changes).
pub fn rebuild_http_client() {
    *AI_HTTP_CLIENT
        .write()
        .unwrap_or_else(std::sync::PoisonError::into_inner) = None;
}

/// Generate assistant text. Never auto-applies git mutations (P1).
///
/// Tries `req.provider` first, then each `req.fallbacks` entry in order.
/// Network-level failures get one same-provider retry; other errors move
/// straight to the next provider. When every attempt fails, the LAST error
/// is returned.
pub async fn generate_text(req: AiGenerateRequest) -> Result<String> {
    let user = scrub_secrets(&req.user);
    let system = scrub_secrets(&req.system);

    let mut attempts: Vec<ProviderAttempt> = vec![ProviderAttempt {
        provider: req.provider.clone(),
        base_url: req.base_url.clone(),
        api_key: req.api_key.clone(),
        model: req.model.clone(),
    }];
    attempts.extend(req.fallbacks);
    let total = attempts.len();

    let mut last_err: Option<AppError> = None;
    let client = client()?;
    for attempt in &attempts {
        for pass in 0..2 {
            let result = attempt_chat(&client, attempt, &system, &user).await;
            match result {
                Ok(text) => return Ok(text),
                Err(e) => {
                    let transient = matches!(e, AppError::Network { .. });
                    last_err = Some(e);
                    if pass == 0 && !transient {
                        break; // non-transient: fail over to the next provider now
                    }
                }
            }
        }
    }

    Err(last_err.unwrap_or_else(|| {
        AppError::unknown_with(
            codes::infra::ALL_ATTEMPTS_FAILED,
            format!("all {total} AI provider attempt(s) failed with no error captured"),
            &[("total", total.to_string())],
        )
    }))
}

/// Dispatch one request to a single provider attempt (each attempt carries
/// its own model — provider ids and model namespaces do not mix).
async fn attempt_chat(
    client: &reqwest::Client,
    attempt: &ProviderAttempt,
    system: &str,
    user: &str,
) -> Result<String> {
    let model = attempt.model.as_str();
    match attempt.provider.to_ascii_lowercase().as_str() {
        "openai" => match attempt.api_key.as_deref().filter(|k| !k.is_empty()) {
            Some(key) => {
                openai_chat(client, key, model, system, user, attempt.base_url.clone()).await
            }
            None => Err(AppError::credential(
                codes::infra::OPENAI_KEY_MISSING,
                "OpenAI API key not configured",
            )),
        },
        "anthropic" => match attempt.api_key.as_deref().filter(|k| !k.is_empty()) {
            Some(key) => {
                anthropic_chat(client, key, model, system, user, attempt.base_url.clone()).await
            }
            None => Err(AppError::credential(
                codes::infra::ANTHROPIC_KEY_MISSING,
                "Anthropic API key not configured",
            )),
        },
        "ollama" => {
            ollama_chat(
                client,
                &ollama_base(attempt.base_url.clone()),
                model,
                system,
                user,
            )
            .await
        }
        other => Err(AppError::protocol_with(
            codes::infra::UNSUPPORTED_PROVIDER,
            format!("unsupported AI provider: {other} (use openai, anthropic, or ollama)"),
            &[("provider", other.to_string())],
        )),
    }
}

/// Probe local Ollama (`GET /api/tags`) using the shared timeout client.
pub async fn probe_ollama(base_url: Option<String>) -> Result<Vec<String>> {
    let base = ollama_base(base_url);
    let url = format!("{}/api/tags", base);
    let resp = client()?.get(&url).send().await.map_err(|e| {
        AppError::network_with(
            codes::infra::OLLAMA_UNREACHABLE,
            format!("ollama unreachable: {e}"),
            &[("error", e.to_string())],
        )
    })?;
    if !resp.status().is_success() {
        return Err(AppError::network_with(
            codes::infra::OLLAMA_PROBE_FAILED,
            format!("ollama probe failed: HTTP {}", resp.status()),
            &[("status", resp.status().to_string())],
        ));
    }
    let body: Value = resp.json().await.map_err(|e| {
        AppError::unknown_with(
            codes::infra::OLLAMA_JSON,
            format!("ollama json: {e}"),
            &[("error", e.to_string())],
        )
    })?;
    let models = body["models"]
        .as_array()
        .unwrap_or(&vec![])
        .iter()
        .filter_map(|m| m["name"].as_str().map(str::to_string))
        .collect();
    Ok(models)
}

async fn openai_chat(
    client: &reqwest::Client,
    api_key: &str,
    model: &str,
    system: &str,
    user: &str,
    base_url: Option<String>,
) -> Result<String> {
    let url = openai_endpoint(base_url)?;
    let resp = client
        .post(&url)
        .bearer_auth(api_key)
        .json(&json!({
            "model": model,
            "messages": [
                {"role": "system", "content": system},
                {"role": "user", "content": user}
            ],
            "temperature": 0.2,
        }))
        .send()
        .await
        .map_err(|e| {
            AppError::network_with(
                codes::infra::OPENAI_REQUEST,
                format!("openai: {e}"),
                &[("error", e.to_string())],
            )
        })?;
    let status = resp.status();
    let body: Value = resp.json().await.map_err(|e| {
        AppError::unknown_with(
            codes::infra::OPENAI_JSON,
            format!("openai json: {e}"),
            &[("error", e.to_string())],
        )
    })?;
    if !status.is_success() {
        return Err(http_error(
            "openai",
            status,
            body["error"]["message"]
                .as_str()
                .unwrap_or("request failed"),
        ));
    }
    body["choices"][0]["message"]["content"]
        .as_str()
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty())
        .ok_or_else(|| {
            AppError::unknown(
                codes::infra::OPENAI_EMPTY_CONTENT,
                "openai returned empty content",
            )
        })
}

async fn anthropic_chat(
    client: &reqwest::Client,
    api_key: &str,
    model: &str,
    system: &str,
    user: &str,
    base_url: Option<String>,
) -> Result<String> {
    let url = anthropic_endpoint(base_url)?;
    // Hybrid-reasoning models (GLM 5.x, Claude extended thinking) always
    // emit a thinking block first; max_tokens must leave ample room for it
    // or the response is truncated mid-thought with no text block at all
    // (stop_reason: max_tokens — the Aug 2026 AI-generate error dialog).
    let resp = client
        .post(&url)
        .header("x-api-key", api_key)
        .header("anthropic-version", "2023-06-01")
        .json(&json!({
            "model": model,
            "max_tokens": 32768,
            "system": system,
            "messages": [
                {"role": "user", "content": user}
            ],
        }))
        .send()
        .await
        .map_err(|e| {
            AppError::network_with(
                codes::infra::ANTHROPIC_REQUEST,
                format!("anthropic: {e}"),
                &[("error", e.to_string())],
            )
        })?;
    let status = resp.status();
    let body: Value = resp.json().await.map_err(|e| {
        AppError::unknown_with(
            codes::infra::ANTHROPIC_JSON,
            format!("anthropic json: {e}"),
            &[("error", e.to_string())],
        )
    })?;
    if !status.is_success() {
        return Err(http_error(
            "anthropic",
            status,
            body["error"]["message"]
                .as_str()
                .unwrap_or("request failed"),
        ));
    }
    if let Some(text) = anthropic_content_text(&body) {
        return Ok(text);
    }
    Err(anthropic_no_text_error(&body))
}

/// Error for a response that carried no text block, with a dedicated hint
/// for the `max_tokens` case (budget spent on reasoning before any text).
fn anthropic_no_text_error(body: &Value) -> AppError {
    let stop_reason = body["stop_reason"].as_str().unwrap_or("none");
    if stop_reason == "max_tokens" {
        return AppError::unknown(
            codes::infra::ANTHROPIC_MAX_TOKENS,
            "anthropic: the model hit max_tokens while still reasoning, so no commit \
             message was produced — try again or switch to a non-reasoning model",
        );
    }
    // The response shape stays in `params` for the UI, but `message` must not
    // carry response content: it flows into the local JSON log, and thinking
    // blocks routinely restate diff content.
    let content_json: String = body["content"].to_string().chars().take(200).collect();
    AppError::unknown_with(
        codes::infra::ANTHROPIC_NO_TEXT,
        format!("anthropic returned no text content (stop_reason: {stop_reason})"),
        &[
            ("stop_reason", stop_reason.to_string()),
            ("content", content_json),
        ],
    )
}

/// Join the `text` of every content block, skipping non-text blocks such
/// as the `thinking` blocks emitted by hybrid-reasoning models (GLM,
/// Claude extended thinking): those carry their payload in `thinking`,
/// not `text`, and must not shadow the answer that follows them.
fn anthropic_content_text(body: &Value) -> Option<String> {
    let text = body["content"]
        .as_array()?
        .iter()
        .filter_map(|block| block["text"].as_str())
        .collect::<Vec<_>>()
        .join("");
    let text = text.trim().to_string();
    if text.is_empty() {
        None
    } else {
        Some(text)
    }
}

async fn ollama_chat(
    client: &reqwest::Client,
    base: &str,
    model: &str,
    system: &str,
    user: &str,
) -> Result<String> {
    let url = format!("{}/api/chat", base);
    let resp = client
        .post(&url)
        .json(&json!({
            "model": model,
            "stream": false,
            "messages": [
                {"role": "system", "content": system},
                {"role": "user", "content": user}
            ],
        }))
        .send()
        .await
        .map_err(|e| {
            AppError::network_with(
                codes::infra::OLLAMA_REQUEST,
                format!("ollama: {e}"),
                &[("error", e.to_string())],
            )
        })?;
    let status = resp.status();
    let body: Value = resp.json().await.map_err(|e| {
        AppError::unknown_with(
            codes::infra::OLLAMA_JSON,
            format!("ollama json: {e}"),
            &[("error", e.to_string())],
        )
    })?;
    if !status.is_success() {
        return Err(http_error("ollama", status, "request failed"));
    }
    body["message"]["content"]
        .as_str()
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty())
        .ok_or_else(|| {
            AppError::unknown(
                codes::infra::OLLAMA_EMPTY_CONTENT,
                "ollama returned empty content",
            )
        })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn unsupported_chain_reports_last_error() {
        let req = AiGenerateRequest {
            provider: "acme".into(),
            model: "m".into(),
            base_url: None,
            api_key: None,
            system: "s".into(),
            user: "u".into(),
            fallbacks: vec![ProviderAttempt {
                provider: "acme2".into(),
                base_url: None,
                api_key: None,
                model: "m".into(),
            }],
        };
        let err = generate_text(req).await.unwrap_err();
        assert!(
            err.to_string().contains("acme2"),
            "last attempt's error should surface: {err}"
        );
    }

    #[test]
    fn extracts_plain_text_block() {
        let body = json!({"content": [{"type": "text", "text": "  hello  "}]});
        assert_eq!(anthropic_content_text(&body).as_deref(), Some("hello"));
    }

    #[test]
    fn skips_thinking_block_and_joins_text_blocks() {
        // Hybrid-reasoning models (GLM 5.x, Claude extended thinking)
        // prepend a thinking block whose payload lives in `thinking`.
        let body = json!({"content": [
            {"type": "thinking", "thinking": "let me think"},
            {"type": "text", "text": "feat(ai): "},
            {"type": "text", "text": "parse response"}
        ]});
        assert_eq!(
            anthropic_content_text(&body).as_deref(),
            Some("feat(ai): parse response")
        );
    }

    #[test]
    fn none_when_only_thinking_blocks() {
        let body = json!({"content": [{"type": "thinking", "thinking": "hmm"}]});
        assert_eq!(anthropic_content_text(&body), None);
        assert_eq!(anthropic_content_text(&json!({})), None);
    }

    #[test]
    fn no_text_error_hints_at_max_tokens_reasoning() {
        let body = json!({
            "stop_reason": "max_tokens",
            "content": [{"type": "thinking", "thinking": "truncated thought"}]
        });
        let err = anthropic_no_text_error(&body).to_string();
        assert!(
            err.contains("max_tokens"),
            "error should mention the budget: {err}"
        );
        assert!(
            !err.contains("truncated thought"),
            "raw content dump is noise for this case: {err}"
        );
    }

    #[test]
    fn no_text_error_keeps_diagnostic_out_of_message() {
        let body = json!({
            "stop_reason": "end_turn",
            "content": [{"type": "tool_use", "id": "t1"}]
        });
        let err = anthropic_no_text_error(&body);
        // Message (log-bound) carries only the stop reason; the response
        // shape stays in params for the UI.
        assert!(
            err.to_string().contains("stop_reason: end_turn"),
            "diagnostic reason expected: {err}"
        );
        assert!(
            !err.to_string().contains("tool_use"),
            "response content must not reach the log-bound message: {err}"
        );
        let in_params = match &err {
            AppError::Unknown { params, .. } => params.iter().any(|(_, v)| v.contains("tool_use")),
            _ => false,
        };
        assert!(
            in_params,
            "response shape must stay available to the UI: {err:?}"
        );
    }

    #[test]
    fn http_error_maps_status_to_retry_class() {
        use reqwest::StatusCode as S;
        assert_eq!(
            http_error("openai", S::UNAUTHORIZED, "bad key").category(),
            "Credential"
        );
        assert_eq!(
            http_error("openai", S::FORBIDDEN, "nope").category(),
            "Credential"
        );
        assert_eq!(
            http_error("openai", S::BAD_REQUEST, "invalid").category(),
            "Protocol",
            "400 must not be retried as a network flake"
        );
        assert_eq!(
            http_error("openai", S::UNPROCESSABLE_ENTITY, "bad").category(),
            "Protocol"
        );
        assert_eq!(
            http_error("openai", S::TOO_MANY_REQUESTS, "slow").category(),
            "Network"
        );
        assert_eq!(
            http_error("openai", S::INTERNAL_SERVER_ERROR, "boom").category(),
            "Network"
        );
    }

    #[test]
    fn http_error_truncates_detail_in_message() {
        use reqwest::StatusCode as S;
        let long = "x".repeat(500);
        let err = http_error("openai", S::INTERNAL_SERVER_ERROR, &long);
        assert!(
            err.to_string().len() < 300,
            "log-bound message must stay short"
        );
    }

    #[test]
    fn cloud_endpoints_require_https() {
        assert!(openai_endpoint(Some("https://api.openai.com".into())).is_ok());
        assert!(openai_endpoint(None).is_ok());
        assert!(openai_endpoint(Some("http://127.0.0.1:11434/v1".into())).is_ok());
        assert!(openai_endpoint(Some("http://localhost:8080".into())).is_ok());
        let err = openai_endpoint(Some("http://proxy:8080".into())).unwrap_err();
        assert_eq!(err.category(), "Protocol");
        assert_eq!(err.code(), codes::infra::UNSUPPORTED_PROVIDER);
        let err = anthropic_endpoint(Some("http://proxy:8080".into())).unwrap_err();
        assert_eq!(err.category(), "Protocol");
    }

    #[test]
    fn debug_impls_mask_api_keys() {
        let req = AiGenerateRequest {
            provider: "openai".into(),
            model: "m".into(),
            base_url: None,
            api_key: Some("sk-secret".into()),
            system: "s".into(),
            user: "u".into(),
            fallbacks: vec![],
        };
        let dbg = format!("{req:?}");
        assert!(!dbg.contains("sk-secret"), "key must not appear: {dbg}");
        assert!(dbg.contains("***"), "mask marker expected: {dbg}");
    }

    /// Plan-promised regression: the client build must never panic on
    /// hostile proxy env — the pre-hardening code panicked on
    /// `expect("poisoned")`/builder errors instead. reqwest 0.12 is lenient
    /// at build time (malformed http(s)/socks env only surfaces per-request),
    /// so the lock-in here is the no-panic invariant: `Ok` and `Err` are
    /// both acceptable outcomes, a panic is not.
    #[test]
    fn client_build_never_panics_on_hostile_proxy_env() {
        std::env::set_var("http_proxy", "not a url at all");
        std::env::set_var("https_proxy", "::::");
        std::env::set_var("ALL_PROXY", "socks5://127.0.0.1:1080");
        let outcome = std::panic::catch_unwind(build_client);
        std::env::remove_var("http_proxy");
        std::env::remove_var("https_proxy");
        std::env::remove_var("ALL_PROXY");
        assert!(
            outcome.is_ok(),
            "client build must not panic on garbage proxy env"
        );
    }
}
