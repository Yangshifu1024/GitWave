package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"gitwave.work/internal/proxy"
)

const proxyKey = "proxy"

func (s *Store) ProxySettings(ctx context.Context) (proxy.Settings, error) {
	defaults := proxy.Settings{Mode: proxy.System}
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT value_json FROM app_settings WHERE key = ?", proxyKey).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return defaults, nil
	}
	if err != nil {
		return defaults, err
	}
	settings := defaults
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return defaults, errors.New("invalid saved proxy settings")
	}
	if normalized, err := settings.Normalize(); err == nil {
		return normalized, nil
	}
	return defaults, errors.New("invalid saved proxy settings")
}

func (s *Store) SaveProxySettings(ctx context.Context, settings proxy.Settings) error {
	settings, err := settings.Normalize()
	if err != nil {
		return err
	}
	var manualURL *string
	if settings.ManualURL != "" {
		manualURL = &settings.ManualURL
	}
	value, err := json.Marshal(struct {
		Mode      proxy.Mode `json:"mode"`
		ManualURL *string    `json:"manual_url"`
	}{settings.Mode, manualURL})
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO app_settings (key, value_json, updated_at)
		VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET
		value_json = json_patch(iif(json_valid(app_settings.value_json), app_settings.value_json, '{}'), excluded.value_json),
		updated_at = excluded.updated_at`,
		proxyKey, string(value), time.Now().Unix())
	return err
}
