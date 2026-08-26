//! OS keychain storage for BYOK API keys (Workspace-scoped).

use keyring::Entry;

use crate::domain::error::{AppError, Result};

const SERVICE: &str = "gitwave.ai";

fn account(workspace_id: &str, provider: &str) -> String {
    format!("{workspace_id}:{provider}")
}

fn entry(workspace_id: &str, provider: &str) -> Result<Entry> {
    Entry::new(SERVICE, &account(workspace_id, provider))
        .map_err(|e| AppError::Unknown(format!("keychain: {e}")))
}

pub fn set_api_key(workspace_id: &str, provider: &str, key: &str) -> Result<()> {
    let trimmed = key.trim();
    if trimmed.is_empty() {
        return Err(AppError::Protocol("API key cannot be empty".into()));
    }
    entry(workspace_id, provider)?
        .set_password(trimmed)
        .map_err(|e| AppError::Unknown(format!("keychain set: {e}")))
}

pub fn get_api_key(workspace_id: &str, provider: &str) -> Result<Option<String>> {
    match entry(workspace_id, provider)?.get_password() {
        Ok(p) => Ok(Some(p)),
        Err(keyring::Error::NoEntry) => Ok(None),
        Err(e) => Err(AppError::Unknown(format!("keychain get: {e}"))),
    }
}

pub fn has_api_key(workspace_id: &str, provider: &str) -> Result<bool> {
    Ok(get_api_key(workspace_id, provider)?.is_some())
}

pub fn clear_api_key(workspace_id: &str, provider: &str) -> Result<()> {
    match entry(workspace_id, provider)?.delete_credential() {
        Ok(()) => Ok(()),
        Err(keyring::Error::NoEntry) => Ok(()),
        Err(e) => Err(AppError::Unknown(format!("keychain delete: {e}"))),
    }
}
