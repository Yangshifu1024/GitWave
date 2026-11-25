package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	legacyPrefsKey  = "legacy.webview-preferences.v1"
	legacyImportKey = "native.imported-webview-preferences.v1"
	legacySplitKey  = "native.legacy-workbench-splits"
	maxLegacyPrefs  = 64 << 10
)

type legacyPreferenceSnapshot struct {
	Version int               `json:"version"`
	Values  map[string]string `json:"values"`
}

var legacySplitIDs = []string{"navigator", "history-details", "history-files", "changes-files"}

var ErrLegacyPreferencesMissing = errors.New("previous WebView preferences snapshot is not available")

// ImportLegacyPreferences consumes the transitional Tauri snapshot. An empty
// source reads the snapshot from a state.db already copied into the Go profile.
func (s *Store) ImportLegacyPreferences(ctx context.Context, source string) error {
	imported, err := legacyPreferencesImported(ctx, s.db)
	if err != nil || imported {
		return err
	}
	snapshot, err := readLegacySnapshot(ctx, s.db)
	if errors.Is(err, ErrLegacyPreferencesMissing) {
		var fresh bool
		if queryErr := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app_settings
			WHERE key = ? AND value_json = json_quote(''))`, stableProfileKey).Scan(&fresh); queryErr != nil || fresh {
			return queryErr
		}
	}
	if errors.Is(err, ErrLegacyPreferencesMissing) && source != "" && filepath.Clean(source) != filepath.Clean(s.path) {
		var copied bool
		if err := s.db.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM app_settings WHERE key = ?)", legacyDatabaseCopyKey).Scan(&copied); err != nil {
			return err
		}
		if copied {
			return ErrLegacyPreferencesMissing
		}
		previous, openErr := openLegacyReadOnly(source)
		if errors.Is(openErr, os.ErrNotExist) {
			return nil
		}
		if openErr != nil {
			return openErr
		}
		defer previous.Close()
		snapshot, err = readLegacySnapshot(ctx, previous)
		if err != nil {
			return err
		}
	}
	if err != nil {
		return err
	}
	return s.transaction(ctx, nil, func(tx *sql.Tx) error {
		imported, err := legacyPreferencesImported(ctx, tx)
		if err != nil || imported {
			return err
		}
		if err := importLegacyDesktopSettings(ctx, tx, snapshot.Values); err != nil {
			return err
		}
		if err := importLegacySelection(ctx, tx, snapshot.Values); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "INSERT INTO app_settings VALUES (?, '1', ?)", legacyImportKey, time.Now().Unix())
		return err
	})
}

func readLegacySnapshot(ctx context.Context, db *sql.DB) (legacyPreferenceSnapshot, error) {
	var snapshot legacyPreferenceSnapshot
	var table bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_schema
		WHERE type = 'table' AND name = 'app_settings')`).Scan(&table); err != nil {
		return snapshot, err
	}
	if !table {
		return snapshot, ErrLegacyPreferencesMissing
	}
	var raw sql.NullString
	err := db.QueryRowContext(ctx, `SELECT CASE WHEN length(CAST(value_json AS BLOB)) <= ?
		THEN value_json END FROM app_settings WHERE key = ?`, maxLegacyPrefs, legacyPrefsKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return snapshot, ErrLegacyPreferencesMissing
	}
	if err != nil {
		return snapshot, err
	}
	if !raw.Valid || json.Unmarshal([]byte(raw.String), &snapshot) != nil || snapshot.Version != 1 || snapshot.Values == nil {
		return legacyPreferenceSnapshot{}, errors.New("invalid saved WebView preferences snapshot")
	}
	return snapshot, nil
}

func legacyPreferencesImported(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) (bool, error) {
	var imported bool
	err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM app_settings WHERE key = ?)", legacyImportKey).Scan(&imported)
	return imported, err
}

func importLegacyDesktopSettings(ctx context.Context, tx *sql.Tx, values map[string]string) error {
	for key, preference := range map[string]DesktopPreference{
		"gitwave.language": UILanguagePreference, "gitwave.aiLanguage": AILanguagePreference,
		"gitwave-theme": ThemePreference, "gitwave-palette": PalettePreference,
		"gitwave-font-sans": UIFontPreference, "gitwave-font-mono": CodeFontPreference,
		"gitwave-font-sans-size": UIFontSizePreference, "gitwave-font-mono-size": CodeSizePreference,
	} {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch preference {
		case UIFontPreference, CodeFontPreference:
			value = NormalizeFontFamilies(value)
		case UIFontSizePreference:
			value = NormalizeFontSize(value, 12)
		case CodeSizePreference:
			value = NormalizeFontSize(value, 10)
		}
		if preference.accepts(value) {
			// Insert only missing top-level fields. Existing JSON, including
			// explicit nulls and fields from later versions, wins unchanged.
			_, err := tx.ExecContext(ctx, `INSERT INTO app_settings VALUES (?, json_object(?, ?), ?)
				ON CONFLICT(key) DO UPDATE SET value_json = json_insert(app_settings.value_json, ?, ?), updated_at = excluded.updated_at
				WHERE CASE WHEN json_valid(app_settings.value_json) THEN json_type(app_settings.value_json) = 'object' ELSE 0 END`,
				desktopPreferencesKey, string(preference), value, time.Now().Unix(), "$."+string(preference), value)
			if err != nil {
				return err
			}
		}
	}
	layout := map[string]string{}
	for _, id := range legacySplitIDs {
		key := "gitwave.split." + id
		if value, ok := legacyNumber(values[key]); ok {
			layout[key] = strconv.FormatFloat(value, 'f', -1, 32)
		}
	}
	if len(layout) > 0 {
		if err := writeLegacySetting(ctx, tx, legacySplitKey, layout); err != nil {
			return err
		}
	}
	enabled, hasRefresh := legacyBool(values["gitwave-auto-refresh"])
	minutes, hasMinutes := legacyNumber(values["gitwave-auto-refresh-interval"])
	if hasRefresh || hasMinutes {
		prefs := RefreshPreferences{Enabled: enabled, Minutes: DefaultRefreshMinutes}
		if hasMinutes {
			prefs.Minutes = int(max(1, min(math.Round(minutes), 1440)))
		}
		if err := writeLegacySetting(ctx, tx, refreshKey, prefs); err != nil {
			return err
		}
	}
	if autoUpdate, ok := legacyBool(values["gitwave-auto-update"]); ok {
		return writeLegacySetting(ctx, tx, autoUpdateKey, autoUpdate)
	}
	return nil
}

func legacyNumber(raw string) (float64, bool) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 32)
	return value, err == nil && !math.IsInf(value, 0) && !math.IsNaN(value)
}

func legacyBool(raw string) (bool, bool) {
	return raw == "true", raw == "true" || raw == "false"
}

func writeLegacySetting(ctx context.Context, tx *sql.Tx, key string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO app_settings VALUES (?, ?, ?) ON CONFLICT(key) DO NOTHING", key, string(encoded), time.Now().Unix())
	return err
}

func importLegacySelection(ctx context.Context, tx *sql.Tx, values map[string]string) error {
	ws, repo := values["gitwave.lastActiveWorkspaceId"], values["gitwave.lastActiveRepoId"]
	if ws == "" {
		return nil
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO app_settings (key, value_json, updated_at)
		SELECT ?, json_quote(id), ? FROM workspaces WHERE id = ? ON CONFLICT(key) DO NOTHING`,
		selectionKey, time.Now().Unix(), ws)
	if err != nil {
		return err
	}
	restored, err := result.RowsAffected()
	if err != nil || restored == 0 || repo == "" {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE workspaces SET last_active_repo_id = ?
		WHERE id = ? AND EXISTS (SELECT 1 FROM repos WHERE id = ? AND workspace_id = ? AND status = 'active')`,
		repo, ws, repo, ws)
	return err
}

func (s *Store) LegacyWorkbenchSplits(ctx context.Context) (map[string]string, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT value_json FROM app_settings WHERE key = ?", legacySplitKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var stored map[string]string
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return nil, fmt.Errorf("read imported workbench layout: %w", err)
	}
	layout := make(map[string]string, len(legacySplitIDs))
	for _, id := range legacySplitIDs {
		key := "gitwave.split." + id
		if _, ok := legacyNumber(stored[key]); ok {
			layout[key] = stored[key]
		}
	}
	return layout, nil
}
