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
        return Err(AppError::Network(format!(
            "openai HTTP {status}: {}",
            body["error"]["message"]
                .as_str()
                .unwrap_or("request failed")
        )));
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
    let resp = client
        .post(&url)
        .header("x-api-key", api_key)
        .header("anthropic-version", "2023-06-01")
        .json(&json!({
            "model": model,
            "max_tokens": 1024,
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
        return Err(AppError::Network(format!(
            "anthropic HTTP {status}: {}",
            body["error"]["message"]
                .as_str()
                .unwrap_or("request failed")
        )));
    }
    body["content"][0]["text"]
        .as_str()
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty())
        .ok_or_else(|| AppError::Unknown("anthropic returned empty content".into()))
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
        return Err(AppError::Network(format!("ollama HTTP {status}")));
    }
    body["message"]["content"]
        .as_str()
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty())
        .ok_or_else(|| AppError::Unknown("ollama returned empty content".into()))
}
