package workspace

import (
	"path/filepath"
	"testing"

	"gitwave.work/internal/ai"
)

func TestWorkspaceAIConfigurationKeepsLegacyAndIndependentFields(t *testing.T) {
	ctx := t.Context()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	must(t, err)
	defer store.Close()
	first, err := store.CreateWithRepos(ctx, "First", nil)
	must(t, err)
	second, err := store.CreateWithRepos(ctx, "Second", nil)
	must(t, err)
	_, err = store.db.Exec(`UPDATE workspaces SET settings_json = ? WHERE id = ?`,
		`{"ai_provider":"ollama","ai_model":null,"ai_base_url":"http://localhost:11434","ai_offline":true,"ai_failover":[{"provider":"ollama","model":null,"base_url":null}],"prompt_templates":{"commit":"old","future":"keep"},"theme_override":"dark","future":{"enabled":true}}`, first.ID)
	must(t, err)
	legacy, err := store.AIConfiguration(ctx, first.ID)
	must(t, err)
	if legacy.Provider != ai.Ollama || legacy.Model != "" || !legacy.Offline || len(legacy.Failover) != 1 || legacy.Templates.Commit != "old" {
		t.Fatalf("legacy configuration = %+v", legacy)
	}
	legacy.BaseURL = ""
	legacy.Templates.Commit = "new"
	must(t, store.SaveAIConfiguration(ctx, first.ID, legacy))
	other, err := store.AIConfiguration(ctx, second.ID)
	must(t, err)
	if other.Provider != "" || other.Templates.Commit != "" {
		t.Fatalf("unrelated Workspace = %+v", other)
	}
	var theme, template, cleared string
	err = store.db.QueryRowContext(ctx, `SELECT json_extract(settings_json,'$.theme_override'),
		json_extract(settings_json,'$.prompt_templates.future'),
		COALESCE(json_extract(settings_json,'$.ai_base_url'), 'unset')
		FROM workspaces WHERE id = ?`, first.ID).Scan(&theme, &template, &cleared)
	must(t, err)
	if theme != "dark" || template != "keep" || cleared != "unset" {
		t.Fatalf("retained fields = %q, %q, %q", theme, template, cleared)
	}
	invalid := legacy
	invalid.Provider, invalid.BaseURL = ai.OpenAI, "http://192.0.2.1"
	if err := store.SaveAIConfiguration(ctx, first.ID, invalid); err == nil {
		t.Fatal("saved an unencrypted cloud endpoint")
	}
	next, err := store.AIConfiguration(ctx, first.ID)
	must(t, err)
	if next.Provider != ai.Ollama || next.Templates.Commit != "new" || next.BaseURL != "" {
		t.Fatalf("saved configuration = %+v", next)
	}
}
