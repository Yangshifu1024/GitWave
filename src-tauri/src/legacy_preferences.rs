use std::collections::BTreeMap;
use std::sync::Arc;

use crate::application::AppContext;
use crate::infrastructure::persistence::AppSettingsRepository;

const SNAPSHOT_KEY: &str = "legacy.webview-preferences.v1";

fn snapshot(mut values: BTreeMap<String, String>) -> Result<String, String> {
    values.retain(|key, _| {
        matches!(
            key.as_str(),
            "gitwave.language"
                | "gitwave.aiLanguage"
                | "gitwave-theme"
                | "gitwave-palette"
                | "gitwave-font-sans"
                | "gitwave-font-mono"
                | "gitwave-font-sans-size"
                | "gitwave-font-mono-size"
                | "gitwave.split.navigator"
                | "gitwave.split.history-details"
                | "gitwave.split.history-files"
                | "gitwave.split.changes-files"
                | "gitwave.lastActiveWorkspaceId"
                | "gitwave.lastActiveRepoId"
                | "gitwave-auto-refresh"
                | "gitwave-auto-refresh-interval"
                | "gitwave-auto-update"
        )
    });
    let encoded = serde_json::json!({ "version": 1, "values": values }).to_string();
    if encoded.len() > 64 * 1024 {
        return Err("WebView preferences snapshot is too large".into());
    }
    Ok(encoded)
}

#[tauri::command]
pub async fn cmd_write_legacy_preferences(
    ctx: tauri::State<'_, AppContext>,
    values: BTreeMap<String, String>,
) -> Result<(), String> {
    let encoded = snapshot(values)?;
    let settings = Arc::clone(&ctx.app_settings);
    tauri::async_runtime::spawn_blocking(move || {
        let settings = settings
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        if !settings.is_persistent() {
            return Err("the previous version's settings database is unavailable".to_string());
        }
        settings
            .set(SNAPSHOT_KEY, &encoded)
            .map_err(|_| "could not save WebView preferences snapshot".to_string())
    })
    .await
    .map_err(|_| "could not write WebView preferences snapshot".to_string())?
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn exports_only_known_versioned_preferences() {
        let encoded = snapshot(BTreeMap::from([
            ("gitwave-theme".into(), "dark".into()),
            ("gitwave-font-sans".into(), "Noto Sans CJK SC".into()),
            ("ai-api-key".into(), "fixture secret".into()),
        ]))
        .expect("encode preferences");
        assert_eq!(
            serde_json::from_str::<serde_json::Value>(&encoded).unwrap(),
            serde_json::json!({ "version": 1, "values": {
                "gitwave-theme": "dark", "gitwave-font-sans": "Noto Sans CJK SC"
            } })
        );
        assert!(snapshot(BTreeMap::from([(
            "gitwave-font-sans".into(),
            "a".repeat(65536)
        )]))
        .is_err());
    }
}
