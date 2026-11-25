package main

import (
	"fmt"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

func (w *workbench) updateSettings(chrome settingsTheme) core.Node {
	checkLabel := w.tr("Check for updates", "检查更新")
	check := chrome.action(checkLabel, func() { w.checkForAppUpdate(false) })
	check.TestID = "settings-check-updates"
	check.Disabled = w.updateCheckDisabled
	detailsLabel := w.tr("View update", "查看更新")
	details := chrome.action(detailsLabel, func() { w.updateDialogOpen.Set(true) })
	details.TestID = "settings-view-update"
	return core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
		chrome.label(w.tr("Updates", "更新"), chrome.muted, 12),
		ui.Checkbox{TestID: "settings-auto-update", Text: w.tr("Automatically check for updates on startup", "启动时自动检查更新"),
			Checked: w.autoUpdate.Get, Disabled: w.autoUpdateSaving.Get, OnChange: w.saveAutoUpdate},
		chrome.hint(w.tr("Checks GitHub Releases for a newer version. Nothing is downloaded without your confirmation.",
			"检查 GitHub Releases 是否有新版本。未经你确认不会下载安装包。")),
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Wrap: core.Wrap, Gap: 6}, Children: []core.Node{
			check, core.Show{When: w.updateCanBeShown,
				Then: func(*core.Scope) core.Node { return details }},
			core.Text{TestID: "settings-update-status", Role: core.RoleStatus, ContentFunc: w.updateStatusText,
				Style: core.Style{FontSize: chrome.uiSize(11)}, StyleFunc: settingsTextColor(chrome.muted)},
		}},
		core.Show{When: func() bool { return w.autoUpdateError.Get() != "" }, Then: func(*core.Scope) core.Node {
			return core.Text{TestID: "settings-auto-update-error", Role: core.RoleAlert, ContentFunc: w.autoUpdateError.Get,
				Style: core.Style{FontSize: chrome.uiSize(11)}, StyleFunc: settingsTextColor(chrome.danger)}
		}},
	}}
}

func (w *workbench) updateStatusText() string {
	current := "v" + appVersion()
	state := w.updateState.Get()
	var status string
	switch state.phase {
	case updateChecking:
		status = w.tr("Checking…", "检查中…")
	case updateCurrent:
		status = w.tr("You're up to date", "已是最新版本")
	case updateAvailable, updateDownloading, updateReady:
		status = fmt.Sprintf(w.tr("v%s available", "v%s 可用"), state.offer.version)
	case updateFailed:
		status = state.err
	}
	if status == "" {
		return current
	}
	return current + " · " + status
}

func (w *workbench) updateCanBeShown() bool {
	state := w.updateState.Get()
	return state.phase == updateAvailable || state.phase == updateDownloading || state.phase == updateReady
}

func (w *workbench) appUpdateDialog(chrome palette) core.Node {
	return core.Show{When: w.updateDialogOpen.Get, Then: func(cx *core.Scope) core.Node {
		offer := core.NewMemo(cx, func() updateOffer { return w.updateState.Get().offer })
		phase := core.NewMemo(cx, func() updatePhase { return w.updateState.Get().phase })
		actions := core.NewMemo(cx, func() [2]bool {
			state := w.updateState.Get()
			return [2]bool{state.retryInstall, state.restarting}
		})
		return ui.Dialog{TestID: "app-update-dialog", TitleFunc: func() string {
			if phase.Get() == updateReady {
				if w.installUpdateOnExit {
					return w.tr("Update ready to install", "更新已就绪")
				}
				return w.tr("Update installed", "更新已安装")
			}
			return w.tr("Update available", "有可用更新")
		}, Open: w.updateDialogOpen.Get,
			OnOpenChange: func(open bool, _ base.Reason) { w.updateDialogOpen.Set(open) },
			Style:        core.Style{Width: core.Pt(440), FontFamily: chrome.fonts.ui},
			Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
				offer := offer.Get()
				notesLabel := w.tr("View release notes", "查看发布说明")
				notesLink := toolbarAction(chrome, notesLabel, func() { w.openUpdatePage(offer.page) },
					core.Icon{SVG: lucide.ExternalLink(), Style: core.Style{FontSize: 14, Color: chrome.text}}, chrome.label(notesLabel, chrome.text, 12))
				notesLink.Style.AlignSelf = core.AlignStart
				children := []core.Node{
					chrome.label(fmt.Sprintf(w.tr("GitWave v%s is available.", "GitWave v%s 现已可用。"), offer.version), chrome.text, 13),
					settingsHint(chrome, fmt.Sprintf(w.tr("You are running v%s.", "当前运行的是 v%s。"), appVersion())), notesLink,
				}
				if offer.notes != "" {
					children = append(children, chrome.label(w.tr("What's changed", "更新内容"), chrome.text, 12),
						core.Scroll{TestID: "app-update-notes", Style: core.Style{MaxHeight: core.Pt(160),
							Padding: core.All(8), Background: chrome.elevated, CornerRadius: 4}, Children: []core.Node{
							core.Text{Content: offer.notes, Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.text}},
						}})
				}
				return core.View{Style: core.Style{Gap: 8}, Children: children}
			}), core.Dynamic(func(*core.Scope) core.Node {
				switch phase.Get() {
				case updateDownloading:
					return w.updateProgressView(chrome)
				case updateReady:
					return settingsHint(chrome, w.tr("Restart GitWave to finish applying the update.", "重启 GitWave 以完成更新。"))
				case updateAvailable:
					if w.installUpdate == nil && cx.Env().OS() == core.OSLinux {
						return core.Text{Content: w.tr("Please get the new package from the releases page. The AppImage build updates in-app.",
							"请从发布页获取新安装包。AppImage 版本支持应用内更新。"),
							Style: core.Style{FontSize: chrome.uiSize(11), Color: chrome.muted}}
					}
				}
				return nil
			}), core.Show{When: func() bool { return w.updateState.Get().err != "" }, Then: func(*core.Scope) core.Node {
				return core.Text{Role: core.RoleAlert, ContentFunc: func() string { return w.updateState.Get().err },
					Style: core.Style{FontSize: chrome.uiSize(11), Color: chrome.danger}}
			}}},
			Footer: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
				flags := actions.Get()
				return w.updateActionsView(chrome, phase.Get(), flags[0], flags[1])
			})}}
	}}
}

func (w *workbench) updateProgressView(chrome palette) core.Node {
	return core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
		ui.Progress{TestID: "app-update-download-progress", Label: w.tr("Downloading update", "正在下载更新"),
			Value: func() float32 {
				state := w.updateState.Get()
				return 100 * float32(state.received) / float32(max(1, state.total))
			}, Indeterminate: func() bool { return w.updateState.Get().total == 0 }},
		core.Text{TestID: "app-update-download-size", Role: core.RoleStatus, ContentFunc: func() string {
			state := w.updateState.Get()
			if state.total > 0 {
				return formatUpdateBytes(state.received) + " / " + formatUpdateBytes(state.total)
			}
			return formatUpdateBytes(state.received)
		}, Style: core.Style{FontSize: chrome.uiSize(11), Color: chrome.muted}},
	}}
}

func formatUpdateBytes(size int64) string {
	switch {
	case size >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(size)/(1<<20))
	case size >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(size)/(1<<10))
	default:
		return fmt.Sprintf("%d B", size)
	}
}

func (w *workbench) updateActionsView(chrome palette, phase updatePhase, retry, restarting bool) core.Node {
	later := w.tr("Later", "稍后")
	if phase == updateDownloading {
		later = w.tr("Hide", "隐藏")
	}
	buttons := []core.Node{action(chrome, later, func() { w.updateDialogOpen.Set(false) })}
	var label string
	var invoke func()
	switch phase {
	case updateAvailable:
		if w.installUpdate != nil {
			label, invoke = w.tr("Download & Install", "下载并安装"), w.downloadAppUpdate
			if retry {
				label = w.tr("Try Again", "重试")
			}
		} else {
			label, invoke = w.tr("Open Releases Page", "打开 Releases 页面"), func() { w.openUpdatePage(w.updateState.Get().offer.page) }
		}
	case updateReady:
		label, invoke = w.tr("Restart Now", "立即重启"), w.restartAppUpdate
		if restarting {
			label = w.tr("Restarting…", "正在重启…")
		}
	}
	if invoke != nil {
		button := toolbarAction(chrome, label, invoke, chrome.label(label, chrome.accent, 12))
		button.TestID = "app-update-primary"
		button.Disabled = func() bool { return restarting }
		buttons = append(buttons, button)
	}
	return core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifyEnd, Gap: 8}, Children: buttons}
}

func (w *workbench) openUpdatePage(address string) {
	if address == "" {
		return
	}
	id := w.updateCheckID
	w.cx.OpenURL(address, func(opened bool) {
		if !opened && id == w.updateCheckID && w.updateDialogOpen.Get() {
			state := w.updateState.Get()
			state.err = w.tr("Could not open the release page.", "无法打开发行页面。")
			w.updateState.Set(state)
		}
	})
}
