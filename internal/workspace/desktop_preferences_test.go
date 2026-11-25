package workspace

import (
	"path/filepath"
	"testing"
)

func TestDesktopPreferencesKeepIndependentWritesAndUnknownFields(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "state.db")
	first, err := Open(ctx, path)
	must(t, err)
	t.Cleanup(func() { first.Close() })
	second, err := Open(ctx, path)
	must(t, err)
	t.Cleanup(func() { second.Close() })
	initial, err := first.DesktopPreferences(ctx)
	must(t, err)
	if initial != DefaultDesktopPreferences() {
		t.Fatalf("default preferences = %+v", initial)
	}
	_, err = first.db.ExecContext(ctx, "INSERT INTO app_settings VALUES (?, ?, 0)", desktopPreferencesKey,
		`{"future":{"enabled":true},"ui_language":64,"theme":"obsolete"}`)
	must(t, err)
	done := make(chan error, 2)
	go func() {
		_, err := first.SaveDesktopPreference(ctx, UILanguagePreference, "zh-CN")
		done <- err
	}()
	go func() {
		_, err := second.SaveDesktopPreference(ctx, PalettePreference, TideStudio)
		done <- err
	}()
	for range 2 {
		must(t, <-done)
	}
	_, err = second.SaveDesktopPreference(ctx, AILanguagePreference, "ko")
	must(t, err)
	if _, err := first.SaveDesktopPreference(ctx, ThemePreference, "obsolete"); err == nil {
		t.Fatal("an invalid theme was saved")
	}
	got, err := first.DesktopPreferences(ctx)
	must(t, err)
	want := DesktopPreferences{UILanguage: "zh-CN", AILanguage: "ko", Theme: SystemTheme, Palette: TideStudio}
	if got != want {
		t.Fatalf("saved preferences = %+v, want %+v", got, want)
	}
	var stillEnabled bool
	err = first.db.QueryRowContext(ctx, "SELECT json_extract(value_json, '$.future.enabled') FROM app_settings WHERE key = ?",
		desktopPreferencesKey).Scan(&stillEnabled)
	must(t, err)
	if !stillEnabled {
		t.Fatal("the unknown setting was overwritten")
	}
}

func TestDesktopFontPreferencesNormalizeAndPersistChangedFields(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(ctx, path)
	must(t, err)
	defer func() { must(t, store.Close()) }()

	if got := NormalizeFontFamilies("  \"Menlo\", , PingFang\u2028  SC,  思源黑体\u200d "); got != "Menlo, PingFang SC, 思源黑体" {
		t.Fatalf("normalized fonts = %q", got)
	}
	for input, want := range map[string]string{
		"": "", "abc": "", "16.7": "16", "  +016pt": "16", "-1": "12", "999999999999999999999": "20", "++5": "",
	} {
		if got := NormalizeFontSize(input, 12); got != want {
			t.Errorf("normalized font size %q = %q, want %q", input, got, want)
		}
	}
	_, err = store.SaveDesktopPreference(ctx, ThemePreference, DarkTheme)
	must(t, err)
	_, err = store.SaveDesktopPreferences(ctx, map[DesktopPreference]string{
		UIFontPreference: "Arial", UIFontSizePreference: "19", CodeFontPreference: "Menlo, Consolas", CodeSizePreference: "18",
	})
	must(t, err)
	if _, err := store.SaveDesktopPreferences(ctx, map[DesktopPreference]string{UIFontPreference: "", CodeSizePreference: "50"}); err == nil {
		t.Fatal("invalid batch was saved")
	}
	saved, err := store.SaveDesktopPreferences(ctx, nil)
	must(t, err)
	if saved.Theme != DarkTheme || saved.Fonts != (DesktopFonts{UI: "Arial", UISize: "19", Code: "Menlo, Consolas", CodeSize: "18"}) {
		t.Fatalf("saved preference = %+v", saved)
	}
}
