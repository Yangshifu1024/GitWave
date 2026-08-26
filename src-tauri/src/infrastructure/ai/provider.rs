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
    pub offline: bool,
}

fn is_cloud(provider: &str) -> bool {
    matches!(provider, "openai" | "anthropic")
}

/// Generate assistant text. Never auto-applies git mutations (P1).
pub async fn generate_text(req: AiGenerateRequest) -> Result<String> {
    let provider = req.provider.to_ascii_lowercase();
    if req.offline && is_cloud(&provider) {
        return Err(AppError::Protocol(
            "offline mode is on — cloud AI providers are disabled".into(),
        ));
    }

    let user = scrub_secrets(&req.user);
    let system = scrub_secrets(&req.system);

    match provider.as_str() {
        "openai" => {
            let key = req
                .api_key
                .filter(|k| !k.is_empty())
                .ok_or_else(|| AppError::Credential("OpenAI API key not configured".into()))?;
            openai_chat(&key, &req.model, &system, &user).await
        }
        "anthropic" => {
            let key = req
                .api_key
                .filter(|k| !k.is_empty())
                .ok_or_else(|| AppError::Credential("Anthropic API key not configured".into()))?;
            anthropic_chat(&key, &req.model, &system, &user).await
        }
        "ollama" => {
            let base = req
                .base_url
                .unwrap_or_else(|| "http://127.0.0.1:11434".into());
            ollama_chat(&base, &req.model, &system, &user).await
        }
        other => Err(AppError::Protocol(format!(
            "unsupported AI provider: {other} (use openai, anthropic, or ollama)"
        ))),
    }
}

/// Probe local Ollama (`GET /api/tags`).
pub async fn probe_ollama(base_url: Option<String>) -> Result<Vec<String>> {
    let base = base_url.unwrap_or_else(|| "http://127.0.0.1:11434".into());
    let url = format!("{}/api/tags", base.trim_end_matches('/'));
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

async fn openai_chat(api_key: &str, model: &str, system: &str, user: &str) -> Result<String> {
    let client = reqwest::Client::new();
    let resp = client
        .post("https://api.openai.com/v1/chat/completions")
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

async fn anthropic_chat(api_key: &str, model: &str, system: &str, user: &str) -> Result<String> {
    let client = reqwest::Client::new();
    let resp = client
        .post("https://api.anthropic.com/v1/messages")
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
    let url = format!("{}/api/chat", base.trim_end_matches('/'));
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
