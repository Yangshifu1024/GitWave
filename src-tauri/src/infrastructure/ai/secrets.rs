//! OS keychain storage for BYOK API keys (Workspace-scoped).

use keyring::Entry;

use crate::domain::error::{AppError, Result};

/// AI keys keep the historical service name — existing installs must keep
/// resolving their stored keys.
pub const SERVICE_AI: &str = "gitwave.ai";

/// Namespace reserved for remote credentials (M1+). Not used yet: remote
/// auth still goes through the system `git credential fill` helper.
pub const SERVICE_REMOTE: &str = "gitwave.remote";

fn account(workspace_id: &str, provider: &str) -> String {
    format!("{workspace_id}:{provider}")
}

fn entry(workspace_id: &str, provider: &str) -> Result<Entry> {
    entry_in_service(SERVICE_AI, &account(workspace_id, provider))
}

/// Keychain entry under an explicit service namespace, so other features
/// can share the keyring without colliding with AI keys.
pub(crate) fn entry_in_service(service: &str, account: &str) -> Result<Entry> {
    Entry::new(service, account).map_err(|e| AppError::Unknown(format!("keychain: {e}")))
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn ai_service_namespace_is_stable() {
        assert_eq!(SERVICE_AI, "gitwave.ai");
        assert_ne!(SERVICE_REMOTE, SERVICE_AI);
    }
}
