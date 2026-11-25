package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func settingsStore(t *testing.T) *workspace.Store {
	t.Helper()
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Settings test repository")
	store, err := workspace.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.CreateWithRepos(t.Context(), "Desktop settings", []string{repo}); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestNativeDesktopSettingsFitsAndClosesAtBothWindowSizes(t *testing.T) {
	app, _ := refreshApp(t, settingsStore(t), "")
	const timeout = 10 * time.Second
	const dialog = "id:settings-dialog"
	const closeButton = "id:settings-close"
	assertLayout := func(width, height core.Points) {
		t.Helper()
		app.WaitUntil(timeout, func() bool {
			frame := app.Find(dialog).Bounds()
			return frame.Size.Width == 800 && frame.Origin.X == (width-frame.Size.Width)/2 &&
				frame.Origin.Y >= (height-frame.Size.Height)/2-1 && frame.Origin.Y <= (height-frame.Size.Height)/2+1
		})
		frame := app.Find(dialog).Bounds()
		body := app.Find("id:settings-body").Bounds()
		button := app.Find(closeButton).Bounds()
		if body.Size.Height < height*.6-1 || body.Size.Height > height*.6+1 ||
			button.Origin.Y >= body.Origin.Y || button.Origin.X+button.Size.Width < frame.Origin.X+frame.Size.Width-18 {
			t.Fatalf("settings layout at %.0f×%.0f: dialog %+v, body %+v, close %+v", width, height, frame, body, button)
		}
	}
	app.WaitFor("label:/^Settings test repository,/", timeout)
	app.Key("mod+,")
	app.WaitFor(dialog, timeout)
	assertLayout(1200, 800)
	app.Press("id:settings-network")
	app.WaitFor("id:settings-proxy-system", timeout)
	app.Resize(1024, 768)
	assertLayout(1024, 768)
	app.WaitFor("id:settings-proxy-system", timeout)
	app.Focus("id:settings-proxy-system")
	app.Key("right")
	app.WaitFor("id:settings-proxy-address", timeout)
	app.Key("left")
	app.WaitForGone("id:settings-proxy-address", timeout)
	app.Press(closeButton)
	app.WaitForGone(dialog, timeout)
	app.Key("mod+,")
	app.WaitFor("id:settings-check-updates", timeout)
	body := app.Find("id:settings-body").Bounds()
	check := app.Find("id:settings-check-updates").Bounds()
	if check.Origin.Y+check.Size.Height > body.Origin.Y+body.Size.Height+1 {
		t.Fatalf("Check for updates is below the general page: button %+v, body %+v", check, body)
	}
	app.Key("escape")
	app.WaitForGone(dialog, timeout)
	app.Key("mod+,")
	app.WaitFor(dialog, timeout)
	app.PressAt("id:settings-backdrop", core.Point{X: 8, Y: 64})
	app.WaitForGone(dialog, timeout)
}

func TestNativeDesktopSettingsApplyToWindowAndMenus(t *testing.T) {
	store := settingsStore(t)
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	app.WaitFor("label:/^Settings test repository,/", timeout)
	initialBar := app.Find("id:custom-titlebar").Style.Background
	app.Key("mod+,")
	app.WaitFor("id:settings-general", timeout)
	app.Press("id:settings-ai-language")
	app.WaitFor("id:settings-ai-language-2", timeout)
	app.Press("id:settings-ai-language-2")
	app.WaitFor("text:日本語", timeout)
	app.Press("id:settings-ui-language")
	app.WaitFor("id:settings-ui-language-1", timeout)
	app.Press("id:settings-ui-language-1")
	app.WaitFor("label:历史", timeout)
	app.WaitFor("label:界面语言", timeout)
	app.Press("id:settings-appearance")
	app.WaitFor("label:深色", timeout)
	app.Focus("id:settings-theme-system")
	app.Key("left")
	app.WaitUntil(timeout, func() bool {
		return app.Find("id:custom-titlebar").Style.Background == core.RGB(0x262628) && app.Find("id:settings-theme-dark").Focused
	})
	app.Key("right")
	app.WaitUntil(timeout, func() bool {
		return app.Find("id:custom-titlebar").Style.Background == initialBar && app.Find("id:settings-theme-system").Focused
	})
	app.Press("id:settings-theme-dark")
	app.WaitUntil(timeout, func() bool { return app.Find("id:settings-theme-dark").Props.Checked })
	app.Focus("id:settings-palette-native-blue")
	app.Key("right")
	app.WaitUntil(timeout, func() bool {
		return app.Find("id:custom-titlebar").Style.Background == core.RGB(0x161b20) && app.Find("id:settings-palette-tide").Focused
	})
	app.Key("left")
	app.WaitUntil(timeout, func() bool {
		return app.Find("id:custom-titlebar").Style.Background == core.RGB(0x262628) && app.Find("id:settings-palette-native-blue").Focused
	})
	app.Press("id:settings-theme-system")
	app.WaitUntil(timeout, func() bool {
		prefs, err := store.DesktopPreferences(t.Context())
		return err == nil && prefs.Theme == workspace.SystemTheme && app.Find("id:custom-titlebar").Style.Background == initialBar
	})
	app.Press("label:关闭")
	app.WaitForGone("id:settings-dialog", timeout)
	app.ChooseMenu("GitWave", "设置…")
	app.WaitFor("label:界面语言", timeout)
	app.WaitFor("text:日本語", timeout)
	got, err := store.DesktopPreferences(t.Context())
	if err != nil || got.UILanguage != "zh-CN" || got.AILanguage != "ja" || got.Palette != workspace.NativeBlue {
		t.Fatalf("saved window settings = %+v, %v", got, err)
	}
}

func TestNativeDesktopSettingsRetainsFontEditingOnSystemAppearanceChange(t *testing.T) {
	store := settingsStore(t)
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	const dialog, field = "id:settings-dialog", "id:settings-font-ui-family"
	app.WaitFor("label:/^Settings test repository,/", timeout)
	app.Key("mod+,")
	app.Press("id:settings-appearance")
	app.WaitFor(field, timeout)
	app.Type(field, "Arial, Serif")
	app.Select(field, len("Arial, "), len("Arial, Serif"))
	native := os.Getenv("NSGO_TEST_TARGET") != ""
	edited := app.Find(field)
	if !native && edited.Selection != (core.TextRange{Start: len("Arial, "), End: len("Arial, Serif")}) {
		t.Fatalf("the fixture did not select the font name: %+v", edited.Selection)
	}
	input, selection := edited.ID, edited.Selection
	before := app.Find(dialog).Style.Background
	env := refreshEnvironment(core.LifecycleActive)
	env.Dark = before != desktopColors(true, workspace.NativeBlue).elevated
	app.SetEnvironment(env)
	app.WaitUntil(timeout, func() bool {
		return app.Find(dialog).Style.Background == desktopColors(env.Dark, workspace.NativeBlue).elevated
	})
	if got := app.Find(field); !got.Focused || got.ID != input || got.Text != "Arial, Serif" || (!native && got.Selection != selection) {
		t.Fatalf("appearance change replaced the focused input or draft: id=%v, focused=%t, text=%q, selection=%+v",
			got.ID, got.Focused, got.Text, got.Selection)
	}
	if prefs, err := store.DesktopPreferences(t.Context()); err != nil || prefs.Fonts.UI != "" {
		t.Fatalf("appearance change committed the in-progress draft: %+v, %v", prefs.Fonts, err)
	}
	if native {
		// AppKit does not report snapshot selection without a subscriber.
		// A key delivered to its field editor must replace the selected word.
		app.Key("m")
		app.WaitUntil(timeout, func() bool { return app.Find(field).Text == "Arial, m" })
	} else {
		app.Clear(field)
		app.Type(field, "Arial, m")
	}
	app.Type(field, "enlo")
	app.Press("id:settings-close")
	app.WaitForGone(dialog, timeout)
	app.WaitUntil(timeout, func() bool {
		prefs, err := store.DesktopPreferences(t.Context())
		return err == nil && prefs.Fonts.UI == "Arial, menlo"
	})
}

func TestNativeDesktopSettingsWaitsForAmendConfirmation(t *testing.T) {
	app, _ := refreshApp(t, settingsStore(t), "")
	const timeout = 10 * time.Second
	const settings, amend = "id:settings-dialog", "id:amend-confirmation"
	app.WaitFor("label:/^Settings test repository,/", timeout)
	app.Press("id:navigate-changes")
	app.WaitFor("id:open-amend", timeout)
	app.Press("id:open-amend")
	app.WaitFor(amend, timeout)
	app.ChooseMenu("GitWave", "Settings…")
	if app.Exists(settings) || !app.Exists(amend) {
		t.Fatal("Settings interrupted the commit amend confirmation")
	}
	app.Key("escape")
	app.WaitForGone(amend, timeout)
	app.ChooseMenu("GitWave", "Settings…")
	app.WaitFor(settings, timeout)
}

func TestNativeDesktopSettingsLoadsSavedPreferencesOnLaunch(t *testing.T) {
	store := settingsStore(t)
	for key, value := range map[workspace.DesktopPreference]string{
		workspace.UILanguagePreference: "zh-CN", workspace.AILanguagePreference: "ko",
		workspace.ThemePreference: workspace.DarkTheme, workspace.PalettePreference: workspace.TideStudio,
		workspace.UIFontPreference: "Arial", workspace.UIFontSizePreference: "20",
	} {
		if _, err := store.SaveDesktopPreference(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	app.WaitFor("label:历史", timeout)
	app.WaitFor("label:/^Settings test repository,/", timeout)
	if got := app.Find("id:custom-titlebar").Style.Background; got != core.RGB(0x161b20) {
		t.Fatalf("launch title bar = %v", got)
	}
	if font := app.Find("id:navigate-history >> text:历史").TextStyle.Font; font.Size != 17.5 || !strings.HasPrefix(font.Family, "Arial") {
		t.Fatalf("launch interface font = %+v", font)
	}
	app.Key("mod+,")
	app.WaitFor("label:界面语言", timeout)
	app.WaitFor("text:한국어", timeout)
	app.Press("id:settings-appearance")
	if !app.Find("id:settings-theme-dark").Props.Checked || !app.Find("id:settings-palette-tide").Props.Checked {
		t.Fatal("saved appearance choices were not selected")
	}
}

func TestNativeWorkbenchRestoresImportedWebViewPreferences(t *testing.T) {
	store := settingsStore(t)
	fixture, err := sql.Open("sqlite", filepath.Join(store.DataDirectory(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	_, err = fixture.ExecContext(t.Context(), "INSERT INTO app_settings VALUES (?, ?, 0)",
		"legacy.webview-preferences.v1", `{"version":1,"values":{"gitwave.language":"zh-CN","gitwave-theme":"dark",
		"gitwave.split.navigator":"27","gitwave.split.history-details":"55","gitwave-auto-refresh-interval":"14"}}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ImportLegacyPreferences(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		_ = cx.Storage().SetItem("gitwave.split.history-details", "46")
		w := newWorkbench(cx)
		w.connectWorkspace(store, nil)
		return w.view()
	}, nsgotest.Size(1200, 800), nsgotest.Environment(refreshEnvironment(core.LifecycleActive)))
	const timeout = 10 * time.Second
	app.WaitFor("label:历史", timeout)
	app.WaitFor("label:/^Settings test repository,/", timeout)
	if got := app.Find("id:resize-navigator").Props.Value.Now; got != 27 {
		t.Fatalf("sidebar ratio = %v, want imported 27", got)
	}
	if got := app.Find("id:resize-history-details").Props.Value.Now; got != 46 {
		t.Fatalf("history ratio = %v, want native 46", got)
	}
	if got := app.Find("id:custom-titlebar").Style.Background; got != core.RGB(0x262628) {
		t.Fatalf("launch theme color = %v, want dark", got)
	}
}

func TestNativeDesktopSettingsPreserveChangesSelectionAndScroll(t *testing.T) {
	store := settingsStore(t)
	state, err := store.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for i := range 55 {
		text := "untracked content\n"
		if i == 12 {
			text = strings.Repeat("A long content line for the working-copy diff\n", 90)
		}
		writeRepositoryFixtureFile(t, state.Repo.Path, fmt.Sprintf("modified-%02d.txt", i), text)
	}
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	const list, diff = "id:changes-unstaged-list", "id:diff-line-list"
	const selected = "label:Unstaged · modified-12.txt"
	app.WaitFor("label:/^Settings test repository,/", timeout)
	app.Press("id:navigate-changes")
	app.WaitFor("label:Unstaged · modified-00.txt", timeout)
	app.Scroll(list, 229)
	app.WaitFor(selected, timeout)
	app.Press(selected)
	app.WaitFor(diff, timeout)
	app.Scroll(diff, 244)
	app.WaitUntil(timeout, func() bool { return app.Find(diff).Offset.Y >= 200 })
	listOffset, diffOffset := app.Find(list).Offset.Y, app.Find(diff).Offset.Y
	if listOffset < 200 {
		t.Fatalf("fixture did not scroll the Changes list: %v", listOffset)
	}
	app.Key("mod+,")
	app.WaitFor("id:settings-general", timeout)
	app.Press("id:settings-appearance")
	app.Press("id:settings-palette-tide")
	app.WaitUntil(timeout, func() bool { return app.Find("id:settings-palette-tide").Props.Checked })
	app.Press("label:Close")
	app.WaitForGone("id:settings-dialog", timeout)
	app.WaitUntil(timeout, func() bool {
		return app.Find(list).Offset.Y >= listOffset-2 && app.Find(diff).Offset.Y >= diffOffset-2
	})
	if !app.Find(selected).Props.Selected {
		t.Fatal("changing the palette lost the selected file")
	}
}

func TestNativeDesktopSettingsPreserveHistoryDiffOnLanguageSwitch(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Before long diff")
	line := strings.Repeat("Wide diff content ", 16) + "\n"
	writeRepositoryFixtureFile(t, repo, "wide.txt", strings.Repeat(line, 90))
	git("", "add", "wide.txt")
	git("", "commit", "-m", "Long history diff")
	store, err := workspace.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.CreateWithRepos(t.Context(), "Diff settings", []string{repo}); err != nil {
		t.Fatal(err)
	}
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	const outer, inner = "id:diff-horizontal-scroll", "id:diff-line-list"
	const commit = "label:/^Long history diff,/"
	app.WaitFor(commit, timeout)
	app.Press(commit)
	app.WaitFor(inner, timeout)
	const horizontalDistance = 700
	app.WaitUntil(timeout, func() bool {
		view := app.Find(outer)
		return view.Content.Width >= view.Frame.Size.Width+horizontalDistance
	})
	// The native scroll gesture for the inner list starts at its center.
	// Bring that point into the outer viewport before scrolling vertically.
	app.Scroll(outer, horizontalDistance)
	app.WaitUntil(timeout, func() bool { return app.Find(outer).Offset.X >= horizontalDistance-50 })
	app.Scroll(inner, 240)
	app.WaitUntil(timeout, func() bool { return app.Find(inner).Offset.Y >= 230 })
	x, y := app.Find(outer).Offset.X, app.Find(inner).Offset.Y
	app.Key("mod+,")
	app.WaitFor("id:settings-general", timeout)
	app.Press("id:settings-ui-language")
	app.WaitFor("id:settings-ui-language-1", timeout)
	app.Press("id:settings-ui-language-1")
	app.WaitFor("label:界面语言", timeout)
	app.Press("id:settings-dialog >> label:关闭")
	app.WaitForGone("id:settings-dialog", timeout)
	app.WaitUntil(timeout, func() bool {
		return app.Find(outer).Offset.X >= x-2 && app.Find(inner).Offset.Y >= y-2
	})
}

func TestNativeDesktopSettingsFontsApplyAndReset(t *testing.T) {
	store := settingsStore(t)
	state, err := store.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	writeRepositoryFixtureFile(t, state.Repo.Path, "font-example.txt", "Hello fonts\n")
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	const family, size = "id:settings-font-code-family", "id:settings-font-code-size"
	app.WaitFor("label:/^Settings test repository,/", timeout)
	app.Press("id:navigate-changes")
	app.WaitFor("label:Unstaged · font-example.txt", timeout)
	app.Press("label:Unstaged · font-example.txt")
	app.WaitFor("text:Hello fonts", timeout)
	app.Key("mod+,")
	app.WaitFor("id:settings-general", timeout)
	app.Press("id:settings-appearance")
	app.WaitFor(family, timeout)
	app.Type(family, "\"Menlo\",  Consolas")
	app.Key("enter")
	app.WaitUntil(timeout, func() bool {
		prefs, err := store.DesktopPreferences(t.Context())
		return err == nil && prefs.Fonts.Code == "Menlo, Consolas"
	})
	app.WaitFor(size, timeout)
	app.Type(size, "25")
	preview := "id:settings-font-code-preview"
	app.WaitUntil(timeout, func() bool { return app.Find(preview).TextStyle.Font.Size == 20 })
	app.Key("enter")
	app.WaitUntil(timeout, func() bool {
		prefs, err := store.DesktopPreferences(t.Context())
		return err == nil && prefs.Fonts.CodeSize == "20"
	})
	app.Press("id:settings-dialog >> label:Close")
	app.WaitForGone("id:settings-dialog", timeout)
	app.WaitUntil(timeout, func() bool { return app.Find("text:Hello fonts").TextStyle.Font.Size == 20 })
	app.Key("mod+,")
	app.WaitFor("id:settings-general", timeout)
	app.Press("id:settings-appearance")
	app.WaitFor(size+"-reset", timeout)
	app.Press(size + "-reset")
	app.WaitUntil(timeout, func() bool {
		prefs, err := store.DesktopPreferences(t.Context())
		return err == nil && prefs.Fonts.CodeSize == "" && prefs.Fonts.Code == "Menlo, Consolas"
	})
}

func TestDesktopFontDraftSurvivesClosingDuringSave(t *testing.T) {
	if os.Getenv("NSGO_TEST_TARGET") != "" {
		t.Skip("This test holds the SQLite writer lock while the model enters text")
	}
	store := settingsStore(t)
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	const family, size = "id:settings-font-code-family", "id:settings-font-code-size"
	const palette = "id:settings-palette-tide"
	app.WaitFor("label:/^Settings test repository,/", timeout)
	app.Key("mod+,")
	app.WaitFor("id:settings-general", timeout)
	app.Press("id:settings-appearance")
	app.WaitFor(family, timeout)

	db, err := sql.Open("sqlite", filepath.Join(store.DataDirectory(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(t.Context(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	app.Type(family, "Menlo")
	app.Submit(family)
	app.Press("id:settings-dialog >> label:Close")
	app.WaitForGone("id:settings-dialog", timeout)
	app.Key("mod+,")
	app.WaitFor("id:settings-general", timeout)
	app.Press("id:settings-appearance")
	app.WaitFor(family, timeout)
	if got := app.Find(family).Text; got != "Menlo" {
		t.Fatalf("reopened family draft = %q", got)
	}
	app.Type(size, "17")
	app.Press(palette)
	if _, err := db.ExecContext(t.Context(), "COMMIT"); err != nil {
		t.Fatal(err)
	}
	app.WaitUntil(timeout, func() bool {
		prefs, err := store.DesktopPreferences(t.Context())
		return err == nil && prefs.Fonts.Code == "Menlo" && prefs.Fonts.CodeSize == "17" && prefs.Palette == workspace.TideStudio
	})
	app.Press("id:settings-dialog >> label:Close")
	app.WaitForGone("id:settings-dialog", timeout)
	prefs, err := store.DesktopPreferences(t.Context())
	if err != nil || prefs.Fonts.Code != "Menlo" || prefs.Fonts.CodeSize != "17" {
		t.Fatalf("saved fonts after close = %+v, %v", prefs.Fonts, err)
	}
}

func TestLargeInterfaceFontKeepsWindowsActionsOutsideCaption(t *testing.T) {
	if os.Getenv("NSGO_TEST_TARGET") != "" {
		t.Skip("The model supplies the Windows title bar inset")
	}
	store := settingsStore(t)
	if _, err := store.SaveDesktopPreference(t.Context(), workspace.UIFontSizePreference, "20"); err != nil {
		t.Fatal(err)
	}
	const captionRight = 112
	environment := core.Environment{OS: core.OSWindows, Idiom: core.IdiomDesktop, Locale: "en",
		Lifecycle: core.LifecycleActive, ReduceMotion: true, TitleBar: core.Edges{Right: captionRight, Top: 40}}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.connectWorkspace(store, nil)
		return w.view()
	}, nsgotest.Size(1024, 768), nsgotest.Environment(environment))
	app.WaitFor("label:/^Settings test repository,/", 10*time.Second)
	for _, width := range []core.Points{1024, 1060, 1160, 1260, 1450, 1600} {
		app.Resize(width, 768)
		button := app.Find("id:push").Bounds()
		if right := button.Origin.X + button.Size.Width; right > width-captionRight+1 {
			t.Errorf("window %.0f pt: Push ends at %.0f pt and caption begins at %.0f pt", width, right, width-captionRight)
		}
	}
}
