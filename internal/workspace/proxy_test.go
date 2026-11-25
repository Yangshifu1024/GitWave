package workspace

import (
	"path/filepath"
	"testing"

	"gitwave.work/internal/proxy"
)

func TestReadAndUpdateLegacyProxySettings(t *testing.T) {
	ctx := t.Context()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	must(t, err)
	defer store.Close()
	_, err = store.db.ExecContext(ctx, "INSERT INTO app_settings VALUES ('proxy', ?, 0)",
		`{"mode":"manual","manual_url":" 127.0.0.1:7890/ ","future":{"enabled":true}}`)
	must(t, err)
	got, err := store.ProxySettings(ctx)
	must(t, err)
	if got.Mode != proxy.Manual || got.ManualURL != "http://127.0.0.1:7890" {
		t.Fatalf("legacy settings = %+v", got)
	}
	for _, invalid := range []string{"file:///tmp/proxy", "http://127.0.0.1:65536"} {
		if err := store.SaveProxySettings(ctx, proxy.Settings{Mode: proxy.Manual, ManualURL: invalid}); err == nil {
			t.Fatalf("invalid proxy address was saved: %s", invalid)
		}
	}
	got.Mode = proxy.Off
	must(t, store.SaveProxySettings(ctx, got))
	loaded, err := store.ProxySettings(ctx)
	must(t, err)
	if loaded != got {
		t.Fatalf("persisted settings = %+v, want %+v", loaded, got)
	}
	must(t, store.SaveProxySettings(ctx, proxy.Settings{Mode: proxy.Manual}))
	var preserved, cleared bool
	must(t, store.db.QueryRowContext(ctx, `SELECT json_extract(value_json, '$.future.enabled'),
		json_extract(value_json, '$.manual_url') IS NULL FROM app_settings WHERE key = 'proxy'`).Scan(&preserved, &cleared))
	if !preserved || !cleared {
		t.Fatalf("unknown field preserved = %t, manual URL cleared = %t", preserved, cleared)
	}
}
