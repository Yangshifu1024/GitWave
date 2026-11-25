package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"gitwave.work/internal/ai"
)

func (s *Store) AIConfiguration(ctx context.Context, workspaceID string) (ai.Configuration, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT settings_json FROM workspaces WHERE id = ?", workspaceID).Scan(&raw)
	if err != nil {
		return ai.Configuration{}, err
	}
	var config ai.Configuration
	if err := json.Unmarshal([]byte(raw), &config); err != nil {
		return ai.Configuration{}, fmt.Errorf("invalid Workspace AI configuration")
	}
	return config, nil
}

// SaveAIConfiguration edits the legacy AI fields in place so older GitWave
// versions can read them. The API key belongs in the platform secret store.
func (s *Store) SaveAIConfiguration(ctx context.Context, workspaceID string, config ai.Configuration) error {
	normalized, err := config.Normalize()
	if err != nil {
		return err
	}
	optional := func(value string) *string {
		if value == "" {
			return nil
		}
		return &value
	}
	type fallback struct {
		Provider ai.Provider `json:"provider"`
		Model    *string     `json:"model"`
		BaseURL  *string     `json:"base_url"`
	}
	list := make([]fallback, len(normalized.Failover))
	for i, item := range normalized.Failover {
		list[i] = fallback{Provider: item.Provider, Model: optional(item.Model), BaseURL: optional(item.BaseURL)}
	}
	encoded, err := json.Marshal(map[string]any{
		"ai_provider": optional(string(normalized.Provider)),
		"ai_model":    optional(normalized.Model),
		"ai_base_url": optional(normalized.BaseURL),
		"ai_offline":  normalized.Offline,
		"ai_failover": list,
		"prompt_templates": map[string]*string{
			"commit": optional(normalized.Templates.Commit), "conflict": optional(normalized.Templates.Conflict),
			"pr": optional(normalized.Templates.PR), "reflog": optional(normalized.Templates.Reflog),
			"health": optional(normalized.Templates.Health),
		},
	})
	if err != nil {
		return err
	}
	var saved string
	err = s.db.QueryRowContext(ctx, `UPDATE workspaces
		SET settings_json = json_patch(settings_json, ?)
		WHERE id = ? AND json_valid(settings_json) AND json_type(settings_json) = 'object'
		RETURNING id`, string(encoded), workspaceID).Scan(&saved)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("Workspace was removed or its settings could not be read")
	}
	return err
}
