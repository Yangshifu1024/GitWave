package main

import (
	"fmt"
	"strings"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type lfsUI struct {
	lfsPanel   *core.Signal[*lfsPanel]
	lfsStatus  *core.Signal[*gitcli.LFSStatus]
	lfsPattern *core.Signal[string]
	lfsError   *core.Signal[string]
}

type lfsPanel struct {
	child *backend
	key   changesWriteKey
}

func newLFSUI(cx *core.Scope) lfsUI {
	return lfsUI{
		lfsPanel: core.NewSignal[*lfsPanel](cx, nil), lfsStatus: core.NewSignal[*gitcli.LFSStatus](cx, nil),
		lfsPattern: core.NewSignal(cx, ""), lfsError: core.NewSignal(cx, ""),
	}
}

func (w *workbench) canOpenLFS() bool {
	return w.canSync() && !w.historyLoading.Get() && w.historyError.Get() == "" && w.core != nil &&
		w.core.repo.WorkTree != "" && w.lfsPanel.Get() == nil
}

func (w *workbench) openLFS() {
	if !w.canOpenLFS() {
		return
	}
	panel, ctx := &lfsPanel{child: w.core, key: w.changesWriteKey()}, w.repoCtx
	w.lfsPanel.Set(panel)
	go func() {
		status, err := panel.child.repo.LFSStatus(ctx)
		w.cx.Post(func() {
			if ctx.Err() == nil && w.currentLFS(panel) {
				w.publishLFSStatus(status, err)
			}
		})
	}()
}

func (w *workbench) currentLFS(panel *lfsPanel) bool {
	return panel != nil && w.lfsPanel.Get() == panel && w.core == panel.child && w.changesWriteKey() == panel.key
}

func (w *workbench) publishLFSStatus(status gitcli.LFSStatus, err error) {
	if err != nil {
		w.lfsError.Set(changesWriteError(err))
		return
	}
	w.lfsStatus.Set(&status)
}

func (w *workbench) closeLFS() {
	w.lfsPanel.Set(nil)
	w.lfsStatus.Set(nil)
	w.lfsPattern.Set("")
	w.lfsError.Set("")
}

func (w *workbench) canEditLFS() bool {
	return w.currentLFS(w.lfsPanel.Get()) && w.lfsStatus.Get() != nil && w.canSync()
}

type lfsAction int

const (
	lfsInstall lfsAction = iota
	lfsTrack
	lfsUntrack
)

func (w *workbench) editLFS(action lfsAction, pattern string) {
	if !w.canEditLFS() || action == lfsInstall && (!w.lfsStatus.Get().Available || w.lfsStatus.Get().Installed) ||
		action == lfsTrack && pattern == "" {
		return
	}
	if action == lfsTrack {
		if err := gitcli.ValidateLFSPattern(pattern); err != nil {
			w.lfsError.Set(changesWriteError(err))
			return
		}
	}
	panel := w.lfsPanel.Get()
	w.lfsError.Set("")
	w.setChangesWrite(panel.key, changesWrite{pending: true})
	ctx := w.appCtx
	go func() {
		var err error
		var installed gitcli.LFSInstallResult
		repo := panel.child.repo
		switch action {
		case lfsInstall:
			installed, err = repo.InstallLFS(ctx)
		case lfsTrack:
			err = repo.TrackLFS(ctx, pattern)
		case lfsUntrack:
			err = repo.UntrackLFS(ctx, pattern)
		}
		status, statusErr := repo.LFSStatus(ctx)
		w.cx.Post(func() {
			if w.appCtx.Err() != nil {
				return
			}
			w.setChangesWrite(panel.key, changesWrite{})
			if !w.currentLFS(panel) {
				return
			}
			w.publishLFSStatus(status, statusErr)
			if err != nil {
				w.lfsError.Set(changesWriteError(err))
				return
			}
			if action == lfsTrack {
				w.lfsPattern.Set("")
			}
			if action == lfsInstall {
				message := w.tr("LFS filters wired into this repository", "已在此仓库接入 LFS 过滤器")
				if installed.ExternalHooks {
					message = w.tr("LFS filters installed. The external hooks directory was preserved.", "已安装 LFS 过滤器，保留仓库外的 hooks 目录。")
				}
				w.refNotice.Set(referenceNotice{text: message})
			}
			w.refreshRepository(false)
		})
	}()
}

func (w *workbench) lfsDialog(chrome palette) core.Node {
	return core.Show{When: func() bool { return w.lfsPanel.Get() != nil }, Then: func(*core.Scope) core.Node {
		close := func() {
			if !w.currentChangesWrite().pending {
				w.closeLFS()
			}
		}
		title := w.tr("Git LFS", "Git LFS")
		track := func() { w.editLFS(lfsTrack, strings.TrimSpace(w.lfsPattern.Get())) }
		trackLabel := w.tr("Track", "跟踪")
		button := toolbarAction(chrome, trackLabel, track,
			core.Icon{SVG: lucide.Plus(), Style: core.Style{FontSize: 13, Color: chrome.accent}}, chrome.label(trackLabel, chrome.text, 12))
		button.TestID = "lfs-track"
		button.Disabled = func() bool { return !w.canEditLFS() || strings.TrimSpace(w.lfsPattern.Get()) == "" }
		placeholder := w.tr("Pattern, e.g. *.psd or assets/**", "模式，如 *.psd 或 assets/**")
		input := core.Input{TestID: "lfs-pattern", Label: placeholder, Placeholder: placeholder, Value: w.lfsPattern.Get,
			OnChange: w.lfsPattern.Set, OnSubmit: func(string) { track() }, Disabled: func() bool { return !w.canEditLFS() },
			SubmitBehavior: core.SubmitBehaviorSubmit,
			Style:          core.Style{Height: core.Pt(32), FlexGrow: 1, BorderWidth: 1, BorderColor: chrome.border, Padding: core.All(6)}}
		form := core.View{Style: core.Style{Gap: 8, Padding: core.All(12), Background: chrome.panel, CornerRadius: 12}, Children: []core.Node{
			w.lfsPatternsView(chrome),
			core.View{Style: core.Style{Direction: core.Row, Gap: 6, AlignItems: core.AlignCenter}, Children: []core.Node{input, button}},
		}}
		closeLabel := w.tr("Close", "关闭")
		closeButton := toolbarAction(chrome, closeLabel, close,
			core.Icon{SVG: lucide.X(), Style: core.Style{FontSize: 15, Color: chrome.muted}})
		closeButton.TestID = "lfs-close"
		closeButton.Disabled = func() bool { return w.currentChangesWrite().pending }
		description := w.tr("Track large files with Git LFS. Patterns are stored in .gitattributes.", "用 Git LFS 跟踪大文件，模式保存在 .gitattributes 中。")
		header := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignStart, Gap: 12}, Children: []core.Node{
			core.View{Style: core.Style{Gap: 4, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
				core.Text{Content: title, Role: core.RoleHeader, Style: core.Style{FontSize: chrome.uiSize(16), FontWeight: 600, Color: chrome.text}},
				chrome.label(description, chrome.muted, 14),
			}}, closeButton,
		}}
		return ui.Dialog{TestID: "lfs-dialog", Label: title, Open: func() bool { return true },
			Style: core.Style{Width: core.Pt(480), Padding: core.All(16), CornerRadius: 12},
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					close()
				}
			},
			Children: []core.Node{header, w.lfsInstallView(chrome), form, core.Show{
				When: func() bool { return w.lfsError.Get() != "" },
				Then: func(*core.Scope) core.Node {
					return core.Text{TestID: "lfs-error", ContentFunc: w.lfsError.Get, Role: core.RoleAlert,
						Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.danger}}
				},
			}},
		}
	}}
}

func (w *workbench) lfsInstallView(chrome palette) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		status := w.lfsStatus.Get()
		switch {
		case status == nil:
			if w.lfsError.Get() != "" {
				return core.View{}
			}
			return chrome.label(w.tr("Checking Git LFS…", "正在检查 Git LFS…"), chrome.muted, 12)
		case !status.Available:
			return core.Text{TestID: "lfs-cli-unavailable",
				Content: w.tr("git lfs was not found on PATH. Install Git LFS from https://git-lfs.com.",
					"未在 PATH 中找到 git lfs。请访问 https://git-lfs.com 安装 Git LFS。"),
				Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.muted}}
		case status.Installed:
			return core.View{TestID: "lfs-installed", Style: core.Style{Direction: core.Row, Gap: 6}, Children: []core.Node{
				core.Icon{SVG: lucide.Check(), Style: core.Style{FontSize: 14, Color: chrome.accent}},
				chrome.label(w.tr("LFS filters are wired into this repository", "LFS 过滤器已接入此仓库"), chrome.text, 12),
			}}
		default:
			label := w.tr("Install LFS in this repository", "在此仓库安装 LFS")
			button := toolbarAction(chrome, label, func() { w.editLFS(lfsInstall, "") }, chrome.label(label, chrome.text, 12))
			button.TestID = "lfs-install"
			button.Disabled = func() bool { return !w.canEditLFS() }
			return core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{button}}
		}
	})
}

func (w *workbench) lfsPatterns() []string {
	if status := w.lfsStatus.Get(); status != nil {
		return status.Patterns
	}
	return nil
}

func (w *workbench) lfsPatternsView(chrome palette) core.Node {
	const rowHeight = 30
	return core.Show{When: func() bool { return len(w.lfsPatterns()) > 0 }, Else: func(*core.Scope) core.Node {
		return chrome.label(w.tr("No LFS patterns tracked yet.", "尚未跟踪任何 LFS 模式。"), chrome.muted, 12)
	}, Then: func(*core.Scope) core.Node {
		return core.List{TestID: "lfs-patterns", EstimatedSize: rowHeight,
			StyleFunc: func() core.Style {
				return core.Style{Height: core.Pt(core.Points(min(6, len(w.lfsPatterns())) * rowHeight))}
			},
			Count: func() int { return len(w.lfsPatterns()) }, Key: func(i int) any { return w.lfsPatterns()[i] },
			Row: func(_ *core.Scope, row core.ListRow) core.Node {
				return core.Dynamic(func(*core.Scope) core.Node {
					patterns, i := w.lfsPatterns(), row.Index()
					if i < 0 || i >= len(patterns) {
						return core.View{}
					}
					return w.lfsPatternRow(chrome, patterns[i])
				})
			},
		}
	}}
}

func (w *workbench) lfsPatternRow(chrome palette, pattern string) core.Node {
	label := fmt.Sprintf(w.tr("Stop tracking %s", "停止跟踪 %s"), pattern)
	stop := toolbarAction(chrome, label, func() { w.editLFS(lfsUntrack, pattern) },
		core.Icon{SVG: lucide.X(), Style: core.Style{FontSize: 13, Color: chrome.muted}})
	stop.TestID = "lfs-untrack"
	stop.Disabled = func() bool { return !w.canEditLFS() }
	text := chrome.label(pattern, chrome.text, 12)
	text.MaxLines, text.Style.FontFamily = 1, "monospace"
	text.Style.FlexGrow, text.Style.FlexShrink, text.Style.MinWidth = 1, 1, core.Pt(0)
	return core.View{TestID: "lfs-rule", Style: core.Style{Height: core.Pt(30), Direction: core.Row,
		AlignItems: core.AlignCenter, Padding: core.Edges{Left: 6, Right: 4}, BorderColor: chrome.border, BorderWidth: 1},
		Children: []core.Node{text, stop}}
}
