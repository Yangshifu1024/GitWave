package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const refreshKey = "native.auto-refresh"
const DefaultRefreshMinutes = 5

type RefreshPreferences struct {
	Enabled bool `json:"enabled"`
	Minutes int  `json:"minutes"`
}

func (s *Store) RefreshPreferences(ctx context.Context) (RefreshPreferences, error) {
	prefs := RefreshPreferences{Minutes: DefaultRefreshMinutes}
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value_json FROM app_settings WHERE key = ?", refreshKey).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return prefs, nil
	}
	if err != nil {
		return prefs, err
	}
	if err := json.Unmarshal([]byte(value), &prefs); err != nil || prefs.Minutes < 1 || prefs.Minutes > 1440 {
		return RefreshPreferences{Minutes: DefaultRefreshMinutes}, fmt.Errorf("invalid saved auto-refresh preferences")
	}
	return prefs, nil
}

func (s *Store) SaveRefreshPreferences(ctx context.Context, prefs RefreshPreferences) error {
	if prefs.Minutes < 1 || prefs.Minutes > 1440 {
		return fmt.Errorf("auto-refresh interval must be between 1 and 1440 minutes")
	}
	value, err := json.Marshal(prefs)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO app_settings (key, value_json, updated_at)
		VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`,
		refreshKey, string(value), time.Now().Unix())
	return err
}
