package main

import (
	"context"
	"time"

	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/core"
)

type desktopPreferencesUI struct {
	desktopPrefs     *core.Signal[workspace.DesktopPreferences]
	desktopPrefsBusy *core.Signal[bool]
	desktopPrefsErr  *core.Signal[string]
	fontDraft        *core.Signal[workspace.DesktopFonts]
	fontBaseline     workspace.DesktopFonts
	fontCommitQueued bool
	queuedPrefs      map[workspace.DesktopPreference]string
}

func newDesktopPreferencesUI(cx *core.Scope) desktopPreferencesUI {
	return desktopPreferencesUI{
		desktopPrefs:     core.NewSignal(cx, workspace.DefaultDesktopPreferences()),
		desktopPrefsBusy: core.NewSignal(cx, false),
		desktopPrefsErr:  core.NewSignal(cx, ""),
		fontDraft:        core.NewSignal(cx, workspace.DesktopFonts{}),
	}
}

func (w *workbench) loadDesktopPreferences() {
	if w.workspaceStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(w.appCtx, time.Second)
	defer cancel()
	prefs, err := w.workspaceStore.DesktopPreferences(ctx)
	if err != nil {
		w.desktopPrefsErr.Set(err.Error())
		return
	}
	w.applyDesktopPreferences(prefs)
}

func (w *workbench) loadLegacyWorkbenchLayout() {
	if w.workspaceStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(w.appCtx, time.Second)
	defer cancel()
	layout, err := w.workspaceStore.LegacyWorkbenchSplits(ctx)
	if err == nil {
		storage := w.cx.Storage()
		for key, value := range layout {
			if _, exists := storage.GetItem(key); exists {
				continue
			}
			if err = storage.SetItem(key, value); err != nil {
				break
			}
		}
	}
	if err != nil {
		w.desktopPrefsErr.Set(w.tr("Could not restore the previous workbench layout", "无法恢复旧版本的工作台布局"))
	}
}

func (w *workbench) applyDesktopPreferences(prefs workspace.DesktopPreferences) {
	previous := w.desktopPrefs.Get()
	w.cx.Batch(func() {
		if prefs.UILanguage != previous.UILanguage {
			w.cx.SetLocale(prefs.UILanguage)
			w.syncLocale()
		}
		if prefs.Theme != previous.Theme {
			appearance := core.AppearanceSystem
			switch prefs.Theme {
			case workspace.LightTheme:
				appearance = core.AppearanceLight
			case workspace.DarkTheme:
				appearance = core.AppearanceDark
			}
			w.cx.SetAppearance(appearance)
		}
		if prefs.Palette != previous.Palette {
			w.cx.SetTheme(desktopTheme(prefs.Palette))
		}
		w.desktopPrefs.Set(prefs)
	})
}

func (w *workbench) saveDesktopPreference(key workspace.DesktopPreference, value string) {
	if w.desktopPrefsBusy.Get() {
		if w.queuedPrefs == nil {
			w.queuedPrefs = make(map[workspace.DesktopPreference]string)
		}
		w.queuedPrefs[key] = value
		return
	}
	w.applyDesktopPreferenceChanges(map[workspace.DesktopPreference]string{key: value})
}

func (w *workbench) applyDesktopPreferenceChanges(changes map[workspace.DesktopPreference]string) {
	current := w.desktopPrefs.Get()
	next := current
	for key, value := range changes {
		updated, err := next.With(key, value)
		if err != nil {
			w.desktopPrefsErr.Set(err.Error())
			return
		}
		next = updated
	}
	if next == current {
		return
	}
	w.saveDesktopPreferences(next, changes, nil)
}

func (w *workbench) saveDesktopPreferences(next workspace.DesktopPreferences, changes map[workspace.DesktopPreference]string,
	completed func(error)) {
	w.desktopPrefsErr.Set("")
	store := w.workspaceStore
	if store == nil {
		w.applyDesktopPreferences(next)
		if completed != nil {
			completed(nil)
		}
		return
	}
	w.desktopPrefsBusy.Set(true)
	go func() {
		prefs, err := store.SaveDesktopPreferences(w.appCtx, changes)
		w.cx.Post(func() {
			if w.appCtx.Err() != nil || w.workspaceStore != store {
				return
			}
			if err != nil {
				w.desktopPrefsErr.Set(err.Error())
			} else {
				w.applyDesktopPreferences(prefs)
			}
			w.desktopPrefsBusy.Set(false)
			if completed != nil {
				completed(err)
			}
			if queued := w.queuedPrefs; len(queued) > 0 {
				w.queuedPrefs = nil
				w.applyDesktopPreferenceChanges(queued)
			}
			if w.fontCommitQueued && !w.desktopPrefsBusy.Get() {
				w.fontCommitQueued = false
				w.commitFontSettings()
			}
		})
	}()
}

func desktopTheme(name string) *core.Theme {
	if name != workspace.TideStudio {
		return nil
	}
	theme := core.DefaultTheme()
	apply := func(p *core.Palette, c palette, accentHover core.Color) {
		p.Background, p.Surface, p.Border = c.panel, c.elevated, c.border
		p.Text, p.TextMuted = c.text, c.muted
		p.SurfaceHover, p.SurfacePressed = c.hover, c.selected
		p.Accent, p.AccentHover, p.AccentPressed = c.accent, accentHover, accentHover
		p.Focus, p.Selection = c.accent.Faded(0.65), c.accent.Faded(0.24)
	}
	apply(&theme.Light, desktopColors(false, name), core.RGB(0x157873))
	apply(&theme.Dark, desktopColors(true, name), core.RGB(0x5ec9c2))
	theme.Dark.AccentText = core.RGB(0x12161a)
	return theme
}

func desktopColors(dark bool, name string) palette {
	c := colors(dark)
	if name != workspace.TideStudio {
		return c
	}
	rgb := core.RGB
	if dark {
		c.bar, c.panel, c.elevated = rgb(0x161b20), rgb(0x191f25), rgb(0x1c2329)
		c.border, c.text, c.muted = rgb(0x2a323a), rgb(0xe8ecf0), rgb(0xb4bec8)
		c.textMuted = rgb(0x8b97a3)
		c.accent = rgb(0x3ebab3)
		c.hover, c.selected, c.hunk = c.border, rgb(0x20413f), c.elevated
	} else {
		c.bar, c.panel, c.elevated = rgb(0xe6ebef), rgb(0xf4f6f8), rgb(0xeef1f4)
		c.border, c.text, c.muted = rgb(0xd5dce2), rgb(0x1b2228), rgb(0x4a5560)
		c.textMuted = rgb(0x7a8692)
		c.accent = rgb(0x1a8f8a)
		c.hover, c.selected, c.hunk = rgb(0xe6ebef), rgb(0xd5ebe8), c.elevated
	}
	return c
}
