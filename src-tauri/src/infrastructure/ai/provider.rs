//! HTTP clients for OpenAI / Anthropic / Ollama chat completions.

use serde_json::{json, Value};

use crate::domain::error::{AppError, Result};
use crate::infrastructure::ai::scrubber::scrub_secrets;

#[derive(Debug, Clone)]
pub struct AiGenerateRequest {
    pub provider: String,
    pub model: String,
    pub base_url: Option<String>,
    pub api_key: Option<String>,
    pub system: String,
    pub user: String,
}

fn trim_base(base: &str) -> String {
    base.trim().trim_end_matches('/').to_string()
}

/// Map an HTTP error response to an AppError. Auth failures (401/403)
/// become `Credential` so the failover chain STOPS and surfaces the root
/// cause instead of masking it behind later network errors; every other
/// HTTP failure stays `Network` (the chain may retry elsewhere).
fn http_error(provider: &str, status: reqwest::StatusCode, detail: &str) -> AppError {
    let message = format!("{provider} HTTP {status}: {detail}");
    if status == reqwest::StatusCode::UNAUTHORIZED || status == reqwest::StatusCode::FORBIDDEN {
        AppError::Credential(message)
    } else {
        AppError::Network(message)
    }
}

fn openai_endpoint(base: Option<String>) -> String {
    let base = trim_base(base.as_deref().unwrap_or("https://api.openai.com"));
    if base.ends_with("/chat/completions") {
        return base;
    }
    if base.ends_with("/v1") {
        return format!("{}/chat/completions", base);
    }
    format!("{}/v1/chat/completions", base)
}

fn anthropic_endpoint(base: Option<String>) -> String {
    let base = trim_base(base.as_deref().unwrap_or("https://api.anthropic.com"));
    if base.ends_with("/messages") {
        return base;
    }
    if base.ends_with("/v1") {
        return format!("{}/messages", base);
    }
    format!("{}/v1/messages", base)
}

fn ollama_base(base: Option<String>) -> String {
    trim_base(base.as_deref().unwrap_or("http://127.0.0.1:11434"))
}

/// Generate assistant text. Never auto-applies git mutations (P1).
pub async fn generate_text(req: AiGenerateRequest) -> Result<String> {
    let provider = req.provider.to_ascii_lowercase();
    let user = scrub_secrets(&req.user);
    let system = scrub_secrets(&req.system);

    match provider.as_str() {
        "openai" => {
            let key = req
                .api_key
                .filter(|k| !k.is_empty())
                .ok_or_else(|| AppError::Credential("OpenAI API key not configured".into()))?;
            openai_chat(&key, &req.model, &system, &user, req.base_url).await
        }
        "anthropic" => {
            let key = req
                .api_key
                .filter(|k| !k.is_empty())
                .ok_or_else(|| AppError::Credential("Anthropic API key not configured".into()))?;
            anthropic_chat(&key, &req.model, &system, &user, req.base_url).await
        }
        "ollama" => ollama_chat(&ollama_base(req.base_url), &req.model, &system, &user).await,
        other => Err(AppError::Protocol(format!(
            "unsupported AI provider: {other} (use openai, anthropic, or ollama)"
        ))),
    }
}

/// Probe local Ollama (`GET /api/tags`).
pub async fn probe_ollama(base_url: Option<String>) -> Result<Vec<String>> {
    let base = ollama_base(base_url);
    let url = format!("{}/api/tags", base);
    let client = reqwest::Client::new();
    let resp = client
        .get(&url)
        .send()
        .await
        .map_err(|e| AppError::Network(format!("ollama unreachable: {e}")))?;
    if !resp.status().is_success() {
        return Err(AppError::Network(format!(
            "ollama probe failed: HTTP {}",
            resp.status()
        )));
    }
    let body: Value = resp
        .json()
        .await
        .map_err(|e| AppError::Unknown(format!("ollama json: {e}")))?;
    let models = body["models"]
        .as_array()
        .unwrap_or(&vec![])
        .iter()
        .filter_map(|m| m["name"].as_str().map(str::to_string))
        .collect();
    Ok(models)
}

async fn openai_chat(
    api_key: &str,
    model: &str,
    system: &str,
    user: &str,
    base_url: Option<String>,
) -> Result<String> {
    let url = openai_endpoint(base_url);
    let client = reqwest::Client::new();
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
        .map_err(|e| AppError::Network(format!("openai: {e}")))?;
    let status = resp.status();
    let body: Value = resp
        .json()
        .await
        .map_err(|e| AppError::Unknown(format!("openai json: {e}")))?;
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
        .ok_or_else(|| AppError::Unknown("openai returned empty content".into()))
}

async fn anthropic_chat(
    api_key: &str,
    model: &str,
    system: &str,
    user: &str,
    base_url: Option<String>,
) -> Result<String> {
    let url = anthropic_endpoint(base_url);
    let client = reqwest::Client::new();
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
        .map_err(|e| AppError::Network(format!("anthropic: {e}")))?;
    let status = resp.status();
    let body: Value = resp
        .json()
        .await
        .map_err(|e| AppError::Unknown(format!("anthropic json: {e}")))?;
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
        return AppError::Unknown(
            "anthropic: the model hit max_tokens while still reasoning, so no commit \
             message was produced — try again or switch to a non-reasoning model"
                .into(),
        );
    }
    // Include the response shape in the error so provider-side changes stay
    // debuggable without a proxy.
    let content_json: String = body["content"].to_string().chars().take(200).collect();
    AppError::Unknown(format!(
        "anthropic returned no text content (stop_reason: {stop_reason}, content: {content_json})"
    ))
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

async fn ollama_chat(base: &str, model: &str, system: &str, user: &str) -> Result<String> {
    let url = format!("{}/api/chat", base);
    let client = reqwest::Client::new();
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
        .map_err(|e| AppError::Network(format!("ollama: {e}")))?;
    let status = resp.status();
    let body: Value = resp
        .json()
        .await
        .map_err(|e| AppError::Unknown(format!("ollama json: {e}")))?;
    if !status.is_success() {
        return Err(http_error("ollama", status, "request failed"));
    }
    body["message"]["content"]
        .as_str()
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty())
        .ok_or_else(|| AppError::Unknown("ollama returned empty content".into()))
}

#[cfg(test)]
mod tests {
    use super::*;

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
    fn no_text_error_keeps_diagnostic_for_other_reasons() {
        let body = json!({
            "stop_reason": "end_turn",
            "content": [{"type": "tool_use", "id": "t1"}]
        });
        let err = anthropic_no_text_error(&body).to_string();
        assert!(
            err.contains("stop_reason: end_turn") && err.contains("tool_use"),
            "diagnostic shape expected: {err}"
        );
    }
}
