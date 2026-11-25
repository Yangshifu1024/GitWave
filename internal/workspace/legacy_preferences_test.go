package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

func TestImportLegacyPreferencesReadsTheSnapshotOnce(t *testing.T) {
	ctx := t.Context()
	legacyPath := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := Open(ctx, legacyPath)
	must(t, err)
	defer legacy.Close()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "native.db"))
	must(t, err)
	defer store.Close()
	first, err := store.CreateWithRepos(ctx, "Previous workspace", []string{t.TempDir(), t.TempDir()})
	must(t, err)
	state, err := store.LoadWorkspace(ctx, first.ID)
	must(t, err)
	selected := state.Repos[1].ID
	_, err = store.db.ExecContext(ctx, "DELETE FROM app_settings WHERE key = ?", selectionKey)
	must(t, err)
	_, err = store.db.ExecContext(ctx, "INSERT INTO app_settings VALUES (?, ?, 0)", desktopPreferencesKey,
		`{"theme":"light","ui_font":"","future":{"enabled":true,"option":null},"unknown":null}`)
	must(t, err)
	values := map[string]string{
		"gitwave.language": "zh-CN", "gitwave.aiLanguage": "ja", "gitwave-theme": "dark", "gitwave-palette": "tide",
		"gitwave-font-sans": `"Noto Sans",  System`, "gitwave-font-mono": `"Menlo",  Consolas`,
		"gitwave-font-sans-size": "19", "gitwave-font-mono-size": "80",
		"gitwave.split.navigator": "24.25", "gitwave.split.history-details": "54",
		"gitwave.split.history-files": "NaN", "gitwave.split.changes-files": "39",
		"gitwave-auto-refresh": "true", "gitwave-auto-refresh-interval": "7.6", "gitwave-auto-update": "false",
		"gitwave.lastActiveWorkspaceId": first.ID, "gitwave.lastActiveRepoId": selected,
		"gitwave.private-token": "fixture token", "gitwave.split.unknown": "55",
	}
	saveWebViewSnapshot(t, ctx, legacy, values)
	done := make(chan error, 2)
	for range 2 {
		go func() { done <- store.ImportLegacyPreferences(ctx, legacyPath) }()
	}
	for range 2 {
		must(t, <-done)
	}
	prefs, err := store.DesktopPreferences(ctx)
	must(t, err)
	want := DesktopPreferences{UILanguage: "zh-CN", AILanguage: "ja", Theme: LightTheme, Palette: TideStudio,
		Fonts: DesktopFonts{UISize: "19", Code: "Menlo, Consolas", CodeSize: "20"}}
	if prefs != want {
		t.Fatalf("imported preferences = %#v, want %#v", prefs, want)
	}
	var future, autoUpdate, keepsNulls bool
	must(t, store.db.QueryRowContext(ctx, "SELECT json_extract(value_json, '$.future.enabled') FROM app_settings WHERE key=?",
		desktopPreferencesKey).Scan(&future))
	must(t, store.db.QueryRowContext(ctx, `SELECT json_type(value_json, '$.future.option') = 'null'
		AND json_type(value_json, '$.unknown') = 'null' FROM app_settings WHERE key=?`, desktopPreferencesKey).Scan(&keepsNulls))
	must(t, store.db.QueryRowContext(ctx, "SELECT json_extract(value_json, '$') FROM app_settings WHERE key=?",
		autoUpdateKey).Scan(&autoUpdate))
	if !future || !keepsNulls || autoUpdate {
		t.Fatalf("future field preserved = %t; explicit nulls preserved = %t; legacy auto-update = %t", future, keepsNulls, autoUpdate)
	}
	refresh, err := store.RefreshPreferences(ctx)
	must(t, err)
	if refresh != (RefreshPreferences{Enabled: true, Minutes: 8}) {
		t.Fatalf("restored refresh = %+v", refresh)
	}
	layout, err := store.LegacyWorkbenchSplits(ctx)
	must(t, err)
	if len(layout) != 3 || layout["gitwave.split.navigator"] != "24.25" || layout["gitwave.split.changes-files"] != "39" {
		t.Fatalf("restored split ratios = %v", layout)
	}
	restored, err := store.Load(ctx)
	must(t, err)
	if restored.Workspace.ID != first.ID || restored.Repo.ID != selected {
		t.Fatalf("restored selection = %+v / %+v", restored.Workspace, restored.Repo)
	}
	var marker, copiedSecret int
	must(t, legacy.db.QueryRowContext(ctx, "SELECT count(*) FROM app_settings WHERE key=?", legacyImportKey).Scan(&marker))
	must(t, store.db.QueryRowContext(ctx, "SELECT count(*) FROM app_settings WHERE value_json LIKE '%fixture token%'").Scan(&copiedSecret))
	if marker != 0 || copiedSecret != 0 {
		t.Fatalf("source changed or private value copied: %d / %d", marker, copiedSecret)
	}
	values["gitwave.language"] = "en"
	saveWebViewSnapshot(t, ctx, legacy, values)
	must(t, store.ImportLegacyPreferences(ctx, legacyPath))
	prefs, err = store.DesktopPreferences(ctx)
	must(t, err)
	if prefs.UILanguage != "zh-CN" {
		t.Fatal("a later legacy snapshot overwrote already restored preferences")
	}
}

func TestImportPreferencesFromCopiedDatabasePreservesNativeSelectionAndRefresh(t *testing.T) {
	ctx := t.Context()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "copied.db"))
	must(t, err)
	defer store.Close()
	if err := store.ImportLegacyPreferences(ctx, ""); !errors.Is(err, ErrLegacyPreferencesMissing) {
		t.Fatalf("missing legacy snapshot = %v", err)
	}
	first, err := store.CreateWithRepos(ctx, "Legacy", []string{t.TempDir()})
	must(t, err)
	native, err := store.CreateWithRepos(ctx, "Native", []string{t.TempDir()})
	must(t, err)
	must(t, store.SaveRefreshPreferences(ctx, RefreshPreferences{Minutes: 123}))
	saveWebViewSnapshot(t, ctx, store, map[string]string{
		"gitwave.lastActiveWorkspaceId": first.ID, "gitwave.lastActiveRepoId": first.LastRepoID,
		"gitwave-auto-refresh": "true", "gitwave-auto-refresh-interval": "0",
	})
	must(t, store.ImportLegacyPreferences(ctx, ""))
	state, err := store.Load(ctx)
	must(t, err)
	refresh, err := store.RefreshPreferences(ctx)
	must(t, err)
	if state.Workspace.ID != native.ID || refresh != (RefreshPreferences{Minutes: 123}) {
		t.Fatalf("migration changed native state: %+v, %+v", state.Workspace, refresh)
	}
}

func TestImportPreferencesKeepsStoredRepositoryWhenLegacySelectionWasRemoved(t *testing.T) {
	ctx := t.Context()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	must(t, err)
	defer store.Close()
	ws, err := store.CreateWithRepos(ctx, "Legacy", []string{t.TempDir(), t.TempDir()})
	must(t, err)
	state, err := store.LoadWorkspace(ctx, ws.ID)
	must(t, err)
	stored := state.Repos[1].ID
	_, err = store.db.ExecContext(ctx, "UPDATE workspaces SET last_active_repo_id = ? WHERE id = ?", stored, ws.ID)
	must(t, err)
	_, err = store.db.ExecContext(ctx, "DELETE FROM app_settings WHERE key = ?", selectionKey)
	must(t, err)
	saveWebViewSnapshot(t, ctx, store, map[string]string{
		"gitwave.lastActiveWorkspaceId": ws.ID, "gitwave.lastActiveRepoId": "removed after export",
	})
	must(t, store.ImportLegacyPreferences(ctx, ""))
	state, err = store.Load(ctx)
	must(t, err)
	if state.Workspace.ID != ws.ID || state.Repo.ID != stored {
		t.Fatalf("stale snapshot replaced stored selection: %+v", state)
	}
}

func saveWebViewSnapshot(t *testing.T, ctx context.Context, source *Store, values map[string]string) {
	t.Helper()
	encoded, err := json.Marshal(legacyPreferenceSnapshot{Version: 1, Values: values})
	must(t, err)
	_, err = source.db.ExecContext(ctx, "INSERT OR REPLACE INTO app_settings VALUES (?, ?, 0)", legacyPrefsKey, string(encoded))
	must(t, err)
}
