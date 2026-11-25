package main

import (
	"fmt"
	"strings"
	"time"

	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

type palette struct {
	bar, panel, elevated, border, text, muted, accent, hover, selected core.Color
	textMuted                                                          core.Color
	add, remove, addWord, removeWord, hunk                             core.Color
	danger, warning                                                    core.Color
	fonts                                                              fontMetrics
}

func colors(dark bool) palette {
	rgb := core.RGB
	if dark {
		return palette{
			bar: rgb(0x262628), panel: rgb(0x2a2a2d), elevated: rgb(0x313134),
			border: rgb(0x333336), text: rgb(0xefeff1), muted: rgb(0xa8a8ad),
			textMuted: rgb(0x78787e),
			accent:    rgb(0x0485f7), hover: rgb(0x363639), selected: rgb(0x263e54),
			add: rgb(0x283a32), remove: rgb(0x452e35), addWord: rgb(0x30523e),
			removeWord: rgb(0x633b42), hunk: rgb(0x313134),
			danger: rgb(0xff7376), warning: rgb(0xf7b750),
		}
	}
	return palette{
		bar: rgb(0xececee), panel: rgb(0xf8f8f8), elevated: rgb(0xf4f4f5),
		border: rgb(0xdbdbdd), text: rgb(0x1b1b1d), muted: rgb(0x55555b),
		textMuted: rgb(0x85858b),
		accent:    rgb(0x0485f7), hover: rgb(0xdfdfdf), selected: rgb(0xe3eef8),
		add: rgb(0xe7f6eb), remove: rgb(0xfce7e9), addWord: rgb(0xc5e9ce),
		removeWord: rgb(0xf8c4c8), hunk: rgb(0xf0f0f3),
		danger: rgb(0xb4232f), warning: rgb(0xf5a524),
	}
}

func (w *workbench) view() core.Node {
	w.configureTitleBar()
	w.configureQuitGuard()
	w.watchRefresh()
	w.watchRefreshTimer()
	w.watchBranchTracking()
	w.watchStartupUpdate()
	w.cx.OnKey("mod+k", w.toggleCommandPalette)
	w.cx.OnKey("mod+r", func() { w.queueRefresh(refreshRemoteManual) })
	w.cx.OnKey("mod+,", w.openRefreshSettings)
	paletteName := core.NewMemo(w.cx, func() string { return w.desktopPrefs.Get().Palette })
	fonts := core.NewMemo(w.cx, func() workspace.DesktopFonts { return w.desktopPrefs.Get().Fonts })
	colors := core.NewMemo(w.cx, func() palette { return desktopColors(w.cx.Env().Dark(), paletteName.Get()) }).Get
	return core.View{Style: core.Style{Height: core.Pct(100), MinHeight: core.Pt(0)},
		StyleFunc: func() core.Style { return core.Style{Background: colors().panel} },
		Children: []core.Node{
			core.Dynamic(func(cx *core.Scope) core.Node {
				w.syncLocale()
				surface := colors()
				surface.fonts = desktopFonts(fonts.Get())
				return w.workbenchSurface(cx, surface)
			}),
			core.Dynamic(func(*core.Scope) core.Node {
				w.syncLocale()
				return w.settingsDialog(colors, fonts.Get)
			}),
			core.Dynamic(func(*core.Scope) core.Node {
				w.syncLocale()
				surface := colors()
				surface.fonts = desktopFonts(fonts.Get())
				return w.appUpdateDialog(surface)
			}),
		}}
}

func (w *workbench) workbenchSurface(cx *core.Scope, chrome palette) core.Node {
	return core.View{Style: core.Style{Height: core.Pct(100), MinHeight: core.Pt(0), Background: chrome.panel, Color: chrome.text,
		FontFamily: chrome.fonts.ui, FontSize: chrome.uiSize(14)},
		Children: []core.Node{
			w.titleBar(cx, chrome),
			w.mergeBanner(chrome),
			w.repositoryTabs(chrome),
			split(cx, chrome, splitSpec{"navigator", base.Horizontal, 19, 15, 32}, w.tr("Resize sidebar", "调整侧栏宽度"),
				func() bool { return !w.sideHidden.Get() }, w.sidebar(chrome), w.mainPanel(cx, chrome)),
			w.statusBar(chrome),
			w.commandPaletteDialog(chrome),
			w.workspaceDialogView(chrome),
			w.quitDialog(chrome),
			w.refDialog(cx, chrome),
			w.checkoutDialog(chrome),
			w.pullDialog(chrome),
			w.pushDialog(chrome),
			w.pushReviewDialog(chrome),
			w.fetchAuthDialog(chrome),
			w.aboutDialog(chrome),
			w.aiSettingsDialog(chrome),
			w.commitAIConfigureDialog(chrome),
			w.aiDraftDialog(chrome),
			w.aiAdviceDialog(chrome, "health-ai", w.tr("AI health summary", "AI 健康总结"), w.healthAI,
				func() bool { return w.healthAI.result.Get().text != "" }),
			w.aiAdviceDialog(chrome, "reflog-ai", w.tr("AI explanation of the reflog entry", "Reflog 记录的 AI 解释"),
				w.reflogAI, w.longReflogAdvice),
			w.conflictsDialog(chrome),
			w.discardConflictDialog(chrome),
			w.conflictMarkersDialog(chrome),
			w.mergeAbortDialog(chrome),
			w.interactiveRebaseDialog(chrome),
			w.stashSaveDialog(chrome),
			w.stashPreviewDialog(chrome),
			w.stashApplyDialog(chrome),
			w.stashDropDialog(chrome),
			core.Show{When: func() bool { return w.worktreeForm.Get() != nil },
				Then: func(*core.Scope) core.Node { return w.worktreeCreateDialog(chrome) }},
			core.Show{When: func() bool { return w.worktreeRemove.Get() != nil },
				Then: func(*core.Scope) core.Node { return w.worktreeRemoveDialog(chrome) }},
			w.remoteDialog(chrome),
			w.submoduleDialog(chrome),
			w.lfsDialog(chrome),
			w.repositoryEditorDialog(chrome),
			w.ignoreDialog(chrome),
		}}
}

func bar(chrome palette, height core.Points, background core.Color, children ...core.Node) core.View {
	return core.View{Style: core.Style{
		Direction: core.Row, AlignItems: core.AlignCenter, Height: core.Pt(height),
		Padding: core.Edges{Left: 8, Right: 8}, Gap: 8, Background: background,
		Borders: core.Edges{Bottom: 1}, BorderColor: chrome.border,
	}, Children: children}
}

func (chrome palette) label(value string, color core.Color, size core.Points) core.Text {
	return core.Text{Content: value, MaxLines: 1, Style: core.Style{FontSize: chrome.uiSize(size), Color: color}}
}

func action(chrome palette, label string, invoke func()) core.Node {
	return toolbarAction(chrome, label, invoke, chrome.label(label, chrome.text, 12))
}

func iconAction(chrome palette, label string, icon *core.SVG, invoke func()) core.View {
	button := toolbarAction(chrome, label, invoke, core.Icon{SVG: icon, Style: core.Style{FontSize: 14, Color: chrome.muted}})
	button.Tooltip = chrome.label(label, chrome.text, 12)
	return button
}

func toolbarAction(chrome palette, label string, invoke func(), children ...core.Node) core.View {
	return core.View{
		Role: core.RoleButton, Label: label,
		OnPress: func(core.PressEvent) { invoke() },
		Style: core.Style{
			Height: core.Pt(28), Padding: core.Edges{Left: 8, Right: 8}, AlignItems: core.AlignCenter,
			Direction: core.Row, Gap: 6, JustifyContent: core.JustifyCenter, CornerRadius: 4,
			Hover:        &core.Style{Background: chrome.hover},
			FocusVisible: &core.Style{OutlineWidth: 1, OutlineColor: chrome.accent},
		},
		Children: children,
	}
}

func notice(chrome palette, message, button string, invoke func()) core.Node {
	children := []core.Node{core.Text{
		Content: message, Role: core.RoleStatus,
		Style: core.Style{FontSize: chrome.uiSize(13), Color: chrome.muted, TextAlign: core.TextAlignCenter},
	}}
	if invoke != nil {
		children = append(children, action(chrome, button, invoke))
	}
	return core.View{
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, Padding: core.All(12),
			AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 6},
		Children: children,
	}
}

func workbenchEmptyState(chrome palette, icon *core.SVG, title, description string) core.Node {
	var children []core.Node
	if icon != nil {
		children = append(children, core.Icon{SVG: icon, Style: core.Style{
			FontSize: 22, Color: chrome.muted, Opacity: core.Alpha(0.6),
		}})
	}
	children = append(children, core.View{Style: core.Style{Gap: 4, AlignItems: core.AlignCenter}, Children: []core.Node{
		core.Text{Content: title, Style: core.Style{
			FontSize: chrome.uiSize(14), FontWeight: 500, TextAlign: core.TextAlignCenter}},
		core.Text{Content: description, Style: core.Style{
			FontSize: chrome.uiSize(12), Color: chrome.muted, TextAlign: core.TextAlignCenter, MaxWidth: core.Pt(320)}},
	}})
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Padding: core.All(32),
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 12}, Children: children}
}

func (w *workbench) sidebar(chrome palette) core.Node {
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), Background: chrome.bar},
		Children: []core.Node{
			core.View{Style: core.Style{Padding: core.Edges{Top: 8, Bottom: 8}, Gap: 4,
				Borders: core.Edges{Bottom: 1}, BorderColor: chrome.border},
				Children: []core.Node{
					w.navRow(chrome, w.tr("History", "历史"), lucide.RotateCcwClock(), false),
					w.navRow(chrome, w.tr("Changes", "变更"), lucide.FileDiff(), true),
				}},
			core.Show{When: func() bool { return w.path.Get() == "" && w.workspaceState.Get().Workspace.ID == "" },
				Then: func(*core.Scope) core.Node {
					return workbenchEmptyState(chrome, lucide.FolderOpen(), w.tr("No workspace selected", "未选择工作区"),
						w.tr("Choose or create a workspace from the Workspaces menu.", "请从「工作区」菜单选择或创建工作区。"))
				}, Else: func(*core.Scope) core.Node { return w.repositorySidebar(chrome) }},
		}}
}

func (w *workbench) repositorySidebar(chrome palette) core.Node {
	return core.Scroll{TestID: "repository-sidebar", Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
		Children: []core.Node{core.View{Style: core.Style{Gap: 8, Padding: core.Edges{Top: 8, Bottom: 8}},
			Children: []core.Node{w.healthSidebar(chrome), w.branchSidebar(chrome), w.stashSidebar(chrome),
				w.tagSidebar(chrome), w.remotesSidebar(chrome), w.worktreeSidebar(chrome),
				w.submodulesSidebar(chrome), w.recoverySidebar(chrome)}}}}
}

func sidebarHeading(chrome palette, id, label string, expanded func() bool, toggle func()) core.View {
	text := chrome.label(label, chrome.muted, 11)
	text.Style.FontWeight = 500
	text.Style.FlexGrow, text.Style.FlexShrink, text.Style.MinWidth = 1, 1, core.Pt(0)
	heading := core.View{TestID: id, Label: label, Role: core.RoleGroup,
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Height: core.Pt(28),
			Padding: core.Edges{Left: 12, Right: 12}}, Children: []core.Node{text}}
	if toggle != nil {
		heading.Role = core.RoleButton
		heading.ShowsExpanded, heading.Expanded = true, expanded
		heading.OnPress = func(core.PressEvent) { toggle() }
		heading.Style.Hover = &core.Style{Background: chrome.elevated}
	}
	heading.Style.FocusVisible = &core.Style{OutlineWidth: 1, OutlineColor: chrome.accent, OutlineOffset: -1}
	return heading
}

func sidebarReadError(chrome palette, id string, message func() string) core.Node {
	return core.Show{When: func() bool { return message() != "" }, Then: func(*core.Scope) core.Node {
		return core.Text{TestID: id, ContentFunc: message, Role: core.RoleAlert,
			Style: core.Style{FontSize: chrome.uiSize(11), Color: chrome.danger}}
	}}
}

func (w *workbench) navRow(chrome palette, label string, icon *core.SVG, changes bool) core.Node {
	style := core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
		Padding: core.Edges{Left: 8, Right: 8}, Height: core.Pt(32),
		Color:   chrome.muted,
		Borders: core.Edges{Left: 3}, BorderColor: core.Transparent, Hover: &core.Style{Background: chrome.hover},
		Disabled:     &core.Style{Opacity: core.Alpha(0.45)},
		FocusVisible: &core.Style{OutlineWidth: 2, OutlineColor: chrome.accent, OutlineOffset: -2}}
	current := func() bool { return w.changesMode.Get() == changes }
	title := chrome.label(label, 0, 14)
	title.Style.FlexGrow = 1
	children := []core.Node{core.Icon{SVG: icon, Style: core.Style{FontSize: 16}}, title}
	if changes {
		children = append(children, core.Text{ContentFunc: w.changesCount, Style: core.Style{FontSize: chrome.uiSize(12), FontVariant: core.FontTabularNums}})
	}
	id := "navigate-history"
	if changes {
		id = "navigate-changes"
	}
	return core.View{TestID: id, Role: core.RoleButton, Label: label, Selected: current,
		Disabled: func() bool { return w.path.Get() == "" }, OnPress: func(core.PressEvent) { w.showChanges(changes) },
		Style: style, StyleFunc: func() core.Style {
			if current() {
				return core.Style{Background: chrome.accent.Faded(.1), BorderColor: chrome.accent,
					Color: chrome.text, FontWeight: 500, Hover: &core.Style{Background: chrome.accent.Faded(.15)}}
			}
			return core.Style{}
		}, Children: children}
}

func (w *workbench) workbenchBar(chrome palette) core.Node {
	repoMissing := func() bool { return w.path.Get() == "" }
	disabledStyle := &core.Style{Opacity: core.Alpha(0.45)}
	return bar(chrome, 40, chrome.elevated,
		core.Dynamic(func(*core.Scope) core.Node {
			icon := lucide.PanelLeftClose()
			if w.sideHidden.Get() {
				icon = lucide.PanelLeftOpen()
			}
			return iconAction(chrome, w.tr("Toggle navigator", "显示或隐藏导航"), icon, func() { w.sideHidden.Set(!w.sideHidden.Get()) })
		}),
		core.Dynamic(func(*core.Scope) core.Node {
			history, changes := w.tr("History", "历史"), w.tr("Changes", "变更")
			if w.sideHidden.Get() {
				mode := func(label string, changes bool) core.Node {
					selected := func() bool { return w.changesMode.Get() == changes }
					button := toolbarAction(chrome, label, func() { w.showChanges(changes) }, chrome.label(label, chrome.text, 12))
					button.TestID = "workbench-history-mode"
					if changes {
						button.TestID = "workbench-changes-mode"
					}
					button.Disabled, button.Selected = repoMissing, selected
					button.Style.Disabled = disabledStyle
					button.Style.AriaSelected = &core.Style{Background: chrome.selected}
					return button
				}
				return core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{
					mode(history, false), mode(changes, true),
				}}
			}
			label := chrome.label(history, chrome.text, 14)
			if w.changesMode.Get() && !repoMissing() {
				label.Content = changes
			}
			label.Style.FontWeight = 600
			return label
		}),
		core.Dynamic(func(*core.Scope) core.Node { return w.workbenchBranch(chrome) }),
		core.Show{When: func() bool { return repoMissing() || !w.changesMode.Get() }, Then: func(*core.Scope) core.Node {
			button := toolbarAction(chrome, w.tr("Search commits", "搜索 commit"), w.toggleCommandPalette,
				core.Icon{SVG: lucide.Search(), Style: core.Style{FontSize: 14, Color: chrome.muted}},
				chrome.label(w.tr("Search commits", "搜索 commit"), chrome.text, 12))
			button.Disabled, button.Style.Disabled = repoMissing, disabledStyle
			return button
		}},
		core.Dynamic(func(*core.Scope) core.Node {
			label := w.tr("Focus diff", "专注 diff")
			icon := lucide.Maximize2()
			if w.focus.Get() {
				label = w.tr("Restore layout", "恢复布局")
				icon = lucide.Minimize2()
			}
			button := toolbarAction(chrome, label, func() { w.focus.Set(!w.focus.Get()) },
				core.Icon{SVG: icon, Style: core.Style{FontSize: 14, Color: chrome.muted}}, chrome.label(label, chrome.text, 12))
			button.Disabled, button.Style.Disabled = repoMissing, disabledStyle
			return button
		}))
}

func (w *workbench) workbenchBranch(chrome palette) core.Node {
	state := w.changes.Get()
	if w.path.Get() == "" || !state.ready || state.bare || state.status.Branch == "" {
		return core.View{Style: core.Style{FlexGrow: 1}}
	}
	status := state.status
	name, tooltip, color, icon := status.Branch, status.Branch, chrome.accent, lucide.GitBranch()
	if status.HeadRef == "" && status.HeadOID != "" {
		name = w.tr("detached @ ", "分离于 ") + shortSHA(status.HeadOID)
		tooltip = w.tr("detached @ ", "分离于 ") + status.HeadOID
		color, icon = chrome.muted, lucide.GitCommitHorizontal()
	}
	children := []core.Node{
		core.Icon{SVG: icon, Style: core.Style{FontSize: 13, Color: color}},
		core.Text{Content: name, MaxLines: 1, Style: core.Style{
			FlexShrink: 1, MinWidth: core.Pt(0), FontSize: chrome.uiSize(12), Color: chrome.text,
		}},
	}
	if status.Upstream != "" {
		upstream := "→ " + status.Upstream
		tooltip += " " + upstream
		children = append(children, core.Text{Content: upstream, MaxLines: 1, Style: core.Style{
			FlexShrink: 3, MinWidth: core.Pt(0), FontSize: chrome.uiSize(11), Color: chrome.muted,
		}})
	}
	if status.Ahead > 0 {
		count := fmt.Sprintf("↑%d", status.Ahead)
		tooltip += " " + count
		children = append(children, chrome.label(count, chrome.accent, 11))
	}
	if status.Behind > 0 {
		count := fmt.Sprintf("↓%d", status.Behind)
		tooltip += " " + count
		children = append(children, chrome.label(count, chrome.warning, 11))
	}
	return core.View{TestID: "workbench-branch", Tooltip: chrome.label(tooltip, chrome.text, 11),
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6,
			FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Padding: core.Edges{Left: 12},
			Borders: core.Edges{Left: 1}, BorderColor: chrome.border}, Children: children}
}

func (w *workbench) statusBar(chrome palette) core.Node {
	status := bar(chrome, 32, chrome.bar, core.Text{
		TestID: "status-current-branch",
		ContentFunc: func() string {
			if w.path.Get() == "" {
				return w.tr("No repository selected", "未选择仓库")
			}
			if current := w.changes.Get(); current.ready && !current.bare && current.status.Branch != "" {
				return current.status.Branch
			}
			return w.repo.Get().Branch
		},
		Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.muted},
	}, core.Show{
		When: func() bool {
			return !w.workspaceFailed && (w.workspaceError.Get() != "" || w.workspaceNotice.Get() != "") && w.workspaceModal.Get().kind == ""
		},
		Then: func(*core.Scope) core.Node {
			return core.Text{MaxLines: 1,
				ContentFunc: func() string {
					if failure := w.workspaceError.Get(); failure != "" {
						return failure
					}
					return w.workspaceNotice.Get()
				},
				Style: core.Style{FontSize: chrome.uiSize(11), Color: chrome.muted, FlexShrink: 1},
				StyleFunc: func() core.Style {
					if w.workspaceError.Get() != "" {
						return core.Style{Color: chrome.danger}
					}
					return core.Style{}
				}}
		},
	})
	status.Children = append(status.Children, core.Show{When: func() bool {
		return w.refNotice.Get().text != "" && w.workspaceError.Get() == "" && w.workspaceNotice.Get() == ""
	}, Then: func(*core.Scope) core.Node {
		return core.Dynamic(func(*core.Scope) core.Node {
			feedback := w.refNotice.Get()
			role, color := core.RoleStatus, chrome.muted
			if feedback.failed {
				role, color = core.RoleAlert, chrome.danger
			}
			return core.Text{Content: feedback.text, TestID: "ref-action-status", Role: role,
				MaxLines: 1, Style: core.Style{Color: color, FontSize: chrome.uiSize(11), FlexShrink: 1}}
		})
	}})
	status.Children = append(status.Children, w.fetchStatusView(chrome))
	status.Style.Borders = core.Edges{Top: 1}
	status.Style.Padding.Left = 20
	return status
}

func (w *workbench) inspector(cx *core.Scope, chrome palette) core.Node {
	height := core.NewSignal(cx, core.Points(0))
	fileArea := func(cx *core.Scope) core.Node {
		switch {
		case w.detailLoading.Get():
			return notice(chrome, w.tr("Loading diff…", "正在加载 diff…"), "", nil)
		case w.detailError.Get() != "":
			return notice(chrome, w.detailError.Get(), w.tr("Retry", "重试"), w.retryCommitDetails)
		default:
			return split(cx, chrome, splitSpec{"history-files", base.Horizontal, 24, 16, 48}, w.tr("Resize files", "调整文件列表宽度"),
				func() bool { return !w.focus.Get() }, w.fileList(chrome), w.diffPanel(chrome))
		}
	}
	fit := core.Style{FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), MinHeight: core.Pt(0)}
	return core.View{Style: fit, OnLayout: func(bounds core.Bounds) { height.Set(bounds.Size.Height) }, Children: []core.Node{
		core.Dynamic(func(cx *core.Scope) core.Node {
			if w.selected.Get() == "" {
				return workbenchEmptyState(chrome, nil, w.tr("No commit selected", "未选择提交"),
					w.tr("Click a commit in the history graph to see its diff.", "点击历史图中的提交即可查看其 diff。"))
			}
			details := w.details.Get()
			if details.SHA == "" {
				return fileArea(cx)
			}
			return core.View{Style: fit, Children: []core.Node{
				w.commitHeader(chrome, details, func() core.Points { return height.Get() * .45 }), core.Dynamic(fileArea),
			}}
		}),
	}}
}

func (w *workbench) commitHeader(chrome palette, details commitDetails, maxHeight func() core.Points) core.Node {
	title, body, _ := strings.Cut(details.Message, "\n")
	disclosure := core.View{
		TestID: "commit-message-toggle", Role: core.RoleButton, Hint: title, Expanded: w.messageExpanded.Get,
		OnPress: func(core.PressEvent) { w.messageExpanded.Set(!w.messageExpanded.Get()) },
		LabelFunc: func() string {
			if w.messageExpanded.Get() {
				return w.tr("Collapse commit message", "折叠提交消息")
			}
			return w.tr("Expand commit message", "展开提交消息")
		},
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignStart, Gap: 6,
			FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0),
			Hover:        &core.Style{Background: chrome.hover},
			FocusVisible: &core.Style{OutlineColor: chrome.accent, OutlineWidth: 1}},
		Children: []core.Node{
			core.Dynamic(func(*core.Scope) core.Node {
				icon := lucide.ChevronRight()
				if w.messageExpanded.Get() {
					icon = lucide.ChevronDown()
				}
				return core.Icon{SVG: icon, Style: core.Style{FontSize: 14, Color: chrome.muted, FlexShrink: 0}}
			}),
			core.Dynamic(func(*core.Scope) core.Node {
				lines := 1
				if w.messageExpanded.Get() {
					lines = 0
				}
				return core.Text{Content: title, MaxLines: lines, Style: core.Style{
					FontSize: chrome.uiSize(14), FontWeight: 500, Color: chrome.text, FlexShrink: 1, MinWidth: core.Pt(0)}}
			}),
		},
	}
	metadata := func(text, tooltip string) core.Node {
		label := core.Text{Content: text, Selectable: core.SelectableOn, MaxLines: 1,
			Style: core.Style{FontSize: chrome.uiSize(11), Color: chrome.muted, FlexShrink: 1, MinWidth: core.Pt(0)}}
		if tooltip == "" {
			return label
		}
		return core.View{Style: core.Style{FlexShrink: 1, MinWidth: core.Pt(0)},
			Tooltip: chrome.label(tooltip, chrome.text, 11), Children: []core.Node{label}}
	}
	body = strings.TrimSpace(body)
	return core.Scroll{TestID: "commit-header", NoShrink: true, Style: core.Style{MinHeight: core.Pt(0),
		Background: chrome.elevated, Borders: core.Edges{Bottom: 1}, BorderColor: chrome.border},
		StyleFunc: func() core.Style { return core.Style{MaxHeight: core.Pt(maxHeight())} },
		Children: []core.Node{core.View{Style: core.Style{Padding: core.Edges{Top: 6, Bottom: 6, Left: 12, Right: 12}, Gap: 4},
			Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignStart, Gap: 7}, Children: []core.Node{
					disclosure, w.commitExplainButton(chrome, details.SHA, title), w.commitMoreButton(chrome, details.SHA, title),
				}},
				core.Show{When: func() bool { return body != "" && w.messageExpanded.Get() }, Then: func(*core.Scope) core.Node {
					return core.Text{TestID: "commit-message-body", Content: body, Selectable: core.SelectableOn,
						Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.text}}
				}},
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Wrap: core.Wrap, Gap: 5}, Children: []core.Node{
					metadata(shortSHA(details.SHA), details.SHA), metadata(details.Author, details.Email), metadata("·", ""),
					metadata(time.Unix(details.Time, 0).Format(w.tr("Jan 2, 2006, 03:04 PM", "2006年1月2日 15:04")), ""),
				}},
				core.Dynamic(func(*core.Scope) core.Node { return w.commitHeaderReferences(chrome, details.SHA) }),
			},
		}},
	}
}

func (w *workbench) commitHeaderReferences(chrome palette, sha string) core.Node {
	info := w.repo.Get()
	var badges []core.Node
	for _, branch := range info.Branches {
		if branch.Tip == sha {
			ref := historyRef{commitRef: commitRef{Name: branch.Name, Kind: branch.Kind + "_branch"}}
			badges = append(badges, w.referenceBadge(chrome, ref, false, branch.Current))
		}
	}
	for _, tag := range info.Tags {
		if tag.Tip == sha {
			ref := historyRef{commitRef: commitRef{Name: tag.Name, Kind: "tag"}}
			badges = append(badges, w.referenceBadge(chrome, ref, false, false))
		}
	}
	if len(badges) == 0 {
		return core.View{}
	}
	return core.Scroll{TestID: "commit-header-refs", Style: core.Style{MaxHeight: core.Pt(48)}, Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Wrap: core.Wrap, Gap: 4}, Children: badges},
	}}
}

func (w *workbench) fileList(chrome palette) core.Node {
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Background: chrome.elevated},
		Children: []core.Node{
			bar(chrome, 32, chrome.elevated, chrome.label(w.tr("Changed files", "变更文件"), chrome.muted, 12),
				core.View{Style: core.Style{FlexGrow: 1}},
				core.Text{ContentFunc: func() string { return fmt.Sprint(len(w.files.Get())) },
					Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.muted}}),
			core.List{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, EstimatedSize: 28,
				Count: func() int { return len(w.files.Get()) },
				Row: func(_ *core.Scope, row core.ListRow) core.Node {
					return core.Dynamic(func(*core.Scope) core.Node {
						files := w.files.Get()
						i := row.Index()
						if i < 0 || i >= len(files) {
							return core.View{}
						}
						path := files[i].Path
						return core.View{
							Role: core.RoleButton, Label: path, TestID: "changed-file",
							Selected: func() bool { return w.file.Get() == path },
							OnPress:  func(core.PressEvent) { w.selectFile(path, false) },
							Style: core.Style{
								Height: core.Pt(28), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6,
								Padding: core.Edges{Left: 8, Right: 6}, Borders: core.Edges{Left: 3},
								BorderColor: core.Transparent, Hover: &core.Style{Background: chrome.hover},
								FocusVisible: &core.Style{OutlineWidth: 1, OutlineColor: chrome.accent, OutlineOffset: -1},
							},
							StyleFunc: func() core.Style {
								if w.file.Get() == path {
									return core.Style{Background: chrome.selected, BorderColor: chrome.accent}
								}
								return core.Style{}
							},
							Children: []core.Node{
								core.Icon{SVG: lucide.File(), Style: core.Style{FontSize: 14, Color: chrome.muted}},
								core.Text{Content: path, MaxLines: 1, Style: core.Style{Color: chrome.text, FontSize: chrome.codeSize(12),
									FontFamily: chrome.fonts.code, FlexShrink: 1, MinWidth: core.Pt(0)}},
							},
						}
					})
				}},
		}}
}
