package main

import (
	"math"
	"strconv"
	"strings"
	"time"

	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

type refreshSettingsUI struct {
	refreshSchedule     uint64
	refreshPrefs        *core.Signal[workspace.RefreshPreferences]
	refreshSettingsOpen *core.Signal[bool]
	refreshSettingsBusy *core.Signal[bool]
	refreshSettingsErr  *core.Signal[string]
	settingsSection     *core.Signal[settingsSection]
	appearanceOffset    core.Point
	refreshInterval     *core.Signal[string]
}

func newRefreshSettingsUI(cx *core.Scope) refreshSettingsUI {
	return refreshSettingsUI{
		refreshPrefs:        core.NewSignal(cx, workspace.RefreshPreferences{Minutes: workspace.DefaultRefreshMinutes}),
		refreshSettingsOpen: core.NewSignal(cx, false),
		refreshSettingsBusy: core.NewSignal(cx, false),
		refreshSettingsErr:  core.NewSignal(cx, ""),
		settingsSection:     core.NewSignal(cx, settingsGeneral),
		refreshInterval:     core.NewSignal(cx, strconv.Itoa(workspace.DefaultRefreshMinutes)),
	}
}

func (w *workbench) applyRefreshSettings(prefs workspace.RefreshPreferences) {
	if prefs != w.refreshPrefs.Get() {
		w.refreshSchedule++
		w.refreshPrefs.Set(prefs)
	}
}

func (w *workbench) loadRefreshSettings() {
	store := w.workspaceStore
	if store == nil {
		return
	}
	w.refreshSettingsBusy.Set(true)
	go func() {
		prefs, err := store.RefreshPreferences(w.appCtx)
		w.cx.Post(func() {
			if w.appCtx.Err() != nil || w.workspaceStore != store {
				return
			}
			w.applyRefreshSettings(prefs)
			if err != nil {
				w.refreshSettingsErr.Set(err.Error())
			}
			w.refreshSettingsBusy.Set(false)
		})
	}()
}

func (w *workbench) saveRefreshSettings(prefs workspace.RefreshPreferences) {
	if w.refreshSettingsBusy.Get() || prefs == w.refreshPrefs.Get() {
		return
	}
	store := w.workspaceStore
	if store == nil {
		w.applyRefreshSettings(prefs)
		return
	}
	w.refreshSettingsBusy.Set(true)
	w.refreshSettingsErr.Set("")
	go func() {
		err := store.SaveRefreshPreferences(w.appCtx, prefs)
		w.cx.Post(func() {
			if w.appCtx.Err() != nil || w.workspaceStore != store {
				return
			}
			if err != nil {
				w.refreshSettingsErr.Set(err.Error())
			} else {
				w.applyRefreshSettings(prefs)
			}
			w.refreshSettingsBusy.Set(false)
		})
	}()
}

func (w *workbench) watchRefreshTimer() {
	var stop func() bool
	w.cx.Effect(func() {
		prefs := w.refreshPrefs.Get()
		if stop != nil {
			stop()
			stop = nil
		}
		if !prefs.Enabled {
			return
		}
		interval := time.Duration(prefs.Minutes) * time.Minute
		schedule := w.refreshSchedule
		var tick func()
		tick = func() {
			if w.appCtx.Err() != nil || w.refreshSchedule != schedule {
				return
			}
			if w.fetch.running.Get() == nil {
				w.queueRefresh(refreshAutomatic)
			}
			stop = w.cx.AfterFunc(interval, tick)
		}
		stop = w.cx.AfterFunc(interval, tick)
	})
	w.cx.Cleanup(func() {
		if stop != nil {
			stop()
		}
	})
}

func (w *workbench) openRefreshSettings() {
	// A theme change rebuilds workbench dialogs. Keep Settings from opening
	// over an existing dialog whose next render would cover it.
	if w.refreshBlocked() || w.refreshSettingsBusy.Get() || w.updateDialogOpen.Get() || len(w.quitGuard.dirty.Get()) != 0 ||
		w.amendPrompt.Get() != nil || w.discardPrompt.Get() != nil || w.repoEditor.Get() != nil ||
		w.ignoreForm.Get() != nil || w.submoduleForm.Get() != nil || w.lfsPanel.Get() != nil ||
		w.stashSave.Get() != nil || w.stashApply.Get() != nil || w.stashDrop.Get() != nil || w.stashPreview.Get().entry.OID != "" {
		return
	}
	w.closeSSHSettings()
	w.settingsSection.Set(settingsGeneral)
	w.refreshInterval.Set(strconv.Itoa(w.refreshPrefs.Get().Minutes))
	w.appearanceOffset = core.Point{}
	if !w.desktopPrefsBusy.Get() && w.fontDraft.Get() == w.fontBaseline {
		w.fontBaseline = w.desktopPrefs.Get().Fonts
		w.fontDraft.Set(w.fontBaseline)
	}
	w.refreshSettingsOpen.Set(true)
}

func refreshMinutes(value string) int {
	minutes, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(minutes) || math.IsInf(minutes, 0) {
		return workspace.DefaultRefreshMinutes
	}
	return int(max(1, min(1440, math.Round(minutes))))
}

func (w *workbench) commitRefreshInterval() {
	prefs := w.refreshPrefs.Get()
	if !prefs.Enabled || w.refreshSettingsBusy.Get() {
		return
	}
	prefs.Minutes = refreshMinutes(w.refreshInterval.Get())
	w.refreshInterval.Set(strconv.Itoa(prefs.Minutes))
	w.saveRefreshSettings(prefs)
}

func (w *workbench) refreshSettingsControls(chrome settingsTheme) core.Node {
	label := w.tr("Interval (minutes)", "刷新间隔（分钟）")
	return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
		chrome.label(w.tr("Auto refresh", "自动刷新"), chrome.muted, 12),
		ui.Checkbox{TestID: "auto-refresh", Text: w.tr("Refresh repository data automatically", "自动刷新仓库数据"),
			Checked: func() bool { return w.refreshPrefs.Get().Enabled }, Disabled: w.refreshSettingsBusy.Get,
			OnChange: func(enabled bool) {
				prefs := w.refreshPrefs.Get()
				prefs.Enabled = enabled
				prefs.Minutes = refreshMinutes(w.refreshInterval.Get())
				w.refreshInterval.Set(strconv.Itoa(prefs.Minutes))
				w.saveRefreshSettings(prefs)
			}},
		chrome.hint(w.tr("Refreshes every repository in the workspace and fetches from each remote. It never pulls or pushes.",
			"刷新 Workspace 内所有仓库的数据，并向各远端获取更新。不会自动 pull 或 push。")),
		chrome.label(label, chrome.text, 12),
		ui.Input{TestID: "auto-refresh-interval", Label: label, Value: w.refreshInterval.Get,
			OnChange: w.refreshInterval.Set, OnBlur: w.commitRefreshInterval, OnSubmit: func(string) { w.commitRefreshInterval() },
			Disabled: func() bool { return !w.refreshPrefs.Get().Enabled || w.refreshSettingsBusy.Get() },
			Style:    core.Style{Width: core.Pt(160)}},
		chrome.hint(w.tr("1–1440 minutes", "1–1440 分钟")),
		core.Show{When: func() bool { return w.refreshSettingsErr.Get() != "" }, Then: func(*core.Scope) core.Node {
			return core.Text{Role: core.RoleStatus, ContentFunc: w.refreshSettingsErr.Get, StyleFunc: settingsTextColor(chrome.danger)}
		}},
	}}
}
