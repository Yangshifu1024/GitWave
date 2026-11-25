package workspace

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"
)

const autoUpdateKey = "native.auto-update"

func (s *Store) AutoUpdate(ctx context.Context) (bool, error) {
	var raw sql.NullString
	var previousProfile, importedPreferences bool
	err := s.db.QueryRowContext(ctx, `SELECT
		(SELECT value_json FROM app_settings WHERE key = ?),
		EXISTS(SELECT 1 FROM app_settings WHERE key = ? OR (key = ? AND value_json <> json_quote(''))),
		EXISTS(SELECT 1 FROM app_settings WHERE key = ?)`,
		autoUpdateKey, legacyDatabaseCopyKey, stableProfileKey, legacyImportKey).Scan(&raw, &previousProfile, &importedPreferences)
	if err != nil {
		return false, err
	}
	if !raw.Valid {
		return !previousProfile || importedPreferences, nil
	}
	enabled, valid := legacyBool(raw.String)
	if !valid {
		return false, errors.New("invalid saved auto-update preference")
	}
	return enabled, nil
}

func (s *Store) SaveAutoUpdate(ctx context.Context, enabled bool) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO app_settings (key, value_json, updated_at)
		VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`,
		autoUpdateKey, strconv.FormatBool(enabled), time.Now().Unix())
	return err
}
