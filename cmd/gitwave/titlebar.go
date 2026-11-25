package main

import (
	"gitwave.work/internal/externaltools"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

const titleBarHeight = 40

func (w *workbench) fullTitleBarWidths(chrome palette) (spacing, toolLabels core.Points) {
	if w.cx.Env().OS() == core.OSMacOS {
		return max(740, chrome.uiSize(740)), max(1152, chrome.uiSize(1152))
	}
	// The application menu shares this row on Windows and Linux.
	return max(1110, chrome.uiSize(1110)), max(1248, chrome.uiSize(1248))
}

func (w *workbench) configureTitleBar() {
	w.cx.SetTitle("GitWave")
	w.cx.SetTitleBar(core.TitleBarHidden, titleBarHeight)
	if w.cx.Env().OS() == core.OSMacOS {
		// NSGo positions the top of the button. The old Y inset centered it 14 points down.
		w.cx.SetWindowControls(core.WindowControls{TrafficLights: core.Point{X: 14, Y: 7}})
		w.cx.Effect(func() {
			w.syncLocale()
			w.cx.SetApplicationMenu([]core.MenuItem{
				{Label: "GitWave", Items: []core.MenuItem{
					{Label: w.tr("About GitWave", "关于 GitWave"), OnSelect: w.openAbout},
					{Label: w.tr("Check for Updates…", "检查更新…"), OnSelect: w.checkUpdatesFromMenu},
					{Separator: true},
					{Label: w.tr("Settings…", "设置…"), Shortcut: "mod+,", OnSelect: w.openRefreshSettings},
					{Separator: true},
					{Role: core.MenuHide}, {Role: core.MenuHideOthers}, {Role: core.MenuShowAll},
					{Separator: true},
					{Label: w.tr("Quit GitWave", "退出 GitWave"), Role: core.MenuQuit},
				}},
				{Label: w.tr("Edit", "编辑"), Items: []core.MenuItem{
					{Role: core.MenuUndo}, {Role: core.MenuRedo}, {Separator: true},
					{Role: core.MenuCut}, {Role: core.MenuCopy}, {Role: core.MenuPaste}, {Role: core.MenuSelectAll},
				}},
				{Label: w.tr("Workspace", "工作区"), Items: w.workspaceActions(true)},
				{Label: w.tr("Repository", "仓库"), Items: w.repositoryMenu()},
				{Label: w.tr("Branch", "分支"), Items: w.branchMenu()},
				{Label: w.tr("Window", "窗口"), Role: core.MenuWindowMenu, Items: []core.MenuItem{
					{Role: core.MenuMinimize}, {Role: core.MenuZoom}, {Role: core.MenuToggleFullScreen},
					{Separator: true}, {Role: core.MenuBringAllToFront},
				}},
			})
		})
	}
}

func (w *workbench) titleBar(cx *core.Scope, chrome palette) core.Node {
	width := core.NewSignal(cx, core.Points(0))
	fullSpacing, _ := w.fullTitleBarWidths(chrome)
	insets := func() core.Edges {
		reserved := cx.Env().TitleBar()
		left := max(core.Points(8), reserved.Left)
		if cx.Env().OS() == core.OSMacOS {
			left = max(78, left)
		}
		return core.Edges{Left: left, Right: max(8, reserved.Right)}
	}
	clientWidth := func() core.Points {
		padding := insets()
		width := width.Get() - padding.Left - padding.Right
		if cx.Env().OS() == core.OSLinux && !cx.Env().FullScreen() {
			width -= 96
		}
		return width
	}
	menu := func(label string, items func() []core.MenuItem) core.Node {
		return core.View{
			Role: core.RoleButton, Label: label,
			Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOff},
			OnPress: func(e core.PressEvent) { w.cx.PopupMenu(items(), e.PagePosition, nil) },
			Style: core.Style{Height: core.Pt(28), AlignItems: core.AlignCenter,
				JustifyContent: core.JustifyCenter, CornerRadius: 6, Padding: core.Edges{Left: 8, Right: 8},
				Hover:        &core.Style{Background: chrome.elevated},
				FocusVisible: &core.Style{OutlineWidth: 1, OutlineColor: chrome.accent}},
			StyleFunc: func() core.Style {
				if clientWidth() < fullSpacing {
					return core.Style{Padding: core.Edges{Left: 6, Right: 6}}
				}
				return core.Style{}
			},
			Children: []core.Node{chrome.label(label, chrome.muted, 12)},
		}
	}
	dragArea := core.View{TestID: "titlebar-drag",
		Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOn},
		Style:   core.Style{FlexGrow: 1, MinWidth: core.Pt(8), Height: core.Pt(titleBarHeight)}}
	fetch := w.titleAction(chrome, clientWidth, w.tr("Fetch", "获取"), lucide.ArrowDownUp(), false, nil,
		func() { w.startFetch("") })
	fetch.TestID = "fetch"
	fetch.Disabled = func() bool { return !w.canSync() || w.fetch.prompt.Get() != nil }
	pull := w.titleAction(chrome, clientWidth, w.tr("Pull", "拉取"), lucide.ArrowDown(), false, w.pullBehind, w.openPull)
	pull.TestID = "pull"
	pull.Disabled = func() bool { return !w.canOpenPull() }
	push := w.titleAction(chrome, clientWidth, w.tr("Push", "推送"), lucide.ArrowUp(), false, w.pushAhead, w.openPush)
	push.TestID = "push"
	push.Disabled = func() bool { return !w.canPushFrom(w.pullSourceBranch()) }
	stash := w.titleAction(chrome, clientWidth, w.tr("Stash", "贮藏"), lucide.Archive(), false, nil, w.openStashSave)
	stash.TestID = "stash-save"
	stash.Disabled = func() bool { return !w.canSaveStash() }
	row := []core.Node{
		core.Show{When: func() bool { return cx.Env().OS() != core.OSMacOS }, Then: func(*core.Scope) core.Node {
			return core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{
				menu(w.tr("File", "文件"), func() []core.MenuItem {
					return []core.MenuItem{
						{Label: w.tr("Settings", "设置"), OnSelect: w.openRefreshSettings},
						{Label: w.tr("About", "关于"), OnSelect: w.openAbout},
						{Label: w.tr("Check for Updates", "检查更新"), OnSelect: w.checkUpdatesFromMenu},
						{Separator: true}, {Label: w.tr("Exit", "退出"), OnSelect: w.requestExit},
					}
				}),
				menu(w.tr("Workspace", "工作区"), func() []core.MenuItem { return w.workspaceActions(true) }),
				menu(w.tr("Repository", "仓库"), w.repositoryMenu),
				menu(w.tr("Branch", "分支"), w.branchMenu),
			}}
		}},
		w.workspaceSelector(chrome),
		core.View{Style: core.Style{Direction: core.Row, Gap: 4}, Children: []core.Node{
			w.externalTitleAction(chrome, clientWidth, "reveal-repository", w.tr("Files", "文件"),
				w.tr("Reveal in file manager", "在文件管理器中打开"), lucide.FolderOpen(), w.revealRepository),
			w.externalTitleAction(chrome, clientWidth, "open-terminal", w.tr("Terminal", "终端"),
				w.tr("Open in terminal", "在终端中打开"), lucide.SquareTerminal(), func() {
					w.launchExternalTool(w.path.Get(), externaltools.OpenTerminal)
				}),
			w.editorTitleAction(chrome, clientWidth),
		}},
		dragArea,
		core.View{Style: core.Style{Direction: core.Row, Gap: 4}, Children: []core.Node{
			w.changesTitleAction(chrome, clientWidth),
			stash,
			fetch,
		}},
		core.View{Style: core.Style{Padding: core.Edges{Left: 4, Right: 4}}, Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(1), Height: core.Pt(32), Background: chrome.border}},
		}},
		core.View{Style: core.Style{Direction: core.Row, Gap: 4}, Children: []core.Node{
			pull,
			push,
		}},
		core.Show{When: func() bool { return cx.Env().OS() == core.OSLinux && !cx.Env().FullScreen() },
			Then: func(*core.Scope) core.Node { return w.linuxCaption(chrome) }},
	}
	return core.View{
		TestID: "custom-titlebar", Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOn},
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Height: core.Pt(titleBarHeight),
			Gap: 12, Background: chrome.bar, Borders: core.Edges{Bottom: 1}, BorderColor: chrome.border},
		StyleFunc: func() core.Style {
			style := core.Style{Padding: insets()}
			if clientWidth() < fullSpacing {
				style.Gap = 4
			}
			return style
		},
		OnLayout: func(bounds core.Bounds) { width.Set(bounds.Size.Width) },
		Children: row,
	}
}

func (w *workbench) workspaceSelector(chrome palette) core.Node {
	label := w.tr("Workspaces", "工作区")
	return core.View{
		Role: core.RoleButton, Label: label, TestID: "workspace-selector",
		Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOff},
		OnPress: func(e core.PressEvent) {
			w.cx.PopupMenu(w.workspaceMenu(), e.PagePosition, nil)
		},
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Height: core.Pt(28), FlexShrink: 1, MinWidth: core.Pt(0),
			Padding: core.Edges{Left: 10, Right: 10}, Gap: 6, CornerRadius: 6, BorderWidth: 1,
			BorderColor: chrome.border, Hover: &core.Style{Background: chrome.elevated},
			FocusVisible: &core.Style{OutlineWidth: 1, OutlineColor: chrome.accent}},
		Children: []core.Node{
			core.Icon{SVG: lucide.FolderGit2(), Style: core.Style{Color: chrome.muted, FontSize: 14}},
			core.Text{MaxLines: 1, Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.muted, MaxWidth: core.Pt(180),
				FlexShrink: 1, MinWidth: core.Pt(0)},
				ContentFunc: func() string {
					if name := w.workspaceState.Get().Workspace.Name; name != "" {
						return name
					}
					return label
				}},
			core.Icon{SVG: lucide.ChevronDown(), Style: core.Style{Color: chrome.muted, FontSize: 12}},
		},
	}
}

func (w *workbench) titleAction(chrome palette, windowWidth func() core.Points, label string, icon *core.SVG, compact bool, count func() string, invoke func()) core.View {
	items := []core.Node{core.Icon{SVG: icon, Style: core.Style{FontSize: 14}}}
	fullSpacing, toolLabels := w.fullTitleBarWidths(chrome)
	minimum := max(940, chrome.uiSize(940))
	if w.cx.Env().OS() == core.OSMacOS {
		minimum = fullSpacing
	}
	if compact {
		minimum = toolLabels
	}
	items = append(items, core.Show{When: func() bool { return windowWidth() >= minimum }, Then: func(*core.Scope) core.Node {
		return chrome.label(label, 0, 12)
	}})
	countText := func() string {
		if count != nil {
			if value := count(); value != "" && value != "0" {
				return "(" + value + ")"
			}
		}
		return ""
	}
	if count != nil {
		items = append(items, core.Show{When: func() bool { return windowWidth() >= fullSpacing }, Then: func(*core.Scope) core.Node {
			return core.Text{ContentFunc: countText, MaxLines: 1, Style: core.Style{MinWidth: core.Pt(chrome.uiSize(36)),
				FontSize: chrome.uiSize(12), FontVariant: core.FontTabularNums, TextAlign: core.TextAlignCenter}}
		}})
	}
	button := toolbarAction(chrome, label, func() {
		if invoke != nil {
			invoke()
		}
	}, items...)
	button.Desktop = core.DesktopViewProps{WindowDrag: core.WindowDragOff}
	button.Disabled = func() bool { return invoke == nil || w.path.Get() == "" }
	tip := chrome.label("", chrome.text, 12)
	tip.ContentFunc = func() string {
		if value := countText(); value != "" {
			return label + " " + value
		}
		return label
	}
	button.Tooltip = tip
	button.Style.Color = chrome.muted
	button.Style.Padding, button.Style.CornerRadius = core.Edges{Left: 10, Right: 10}, 6
	button.Style.Hover = &core.Style{Background: chrome.elevated, Color: chrome.text}
	button.Style.Disabled = &core.Style{Opacity: core.Alpha(.4)}
	return button
}

func (w *workbench) changesTitleAction(chrome palette, width func() core.Points) core.View {
	button := w.titleAction(chrome, width, w.tr("Changes", "变更"), lucide.FileDiff(), false, w.changesCount, func() { w.showChanges(true) })
	button.TestID = "titlebar-changes"
	button.StyleFunc = func() core.Style {
		status := w.changes.Get()
		var color core.Color
		if len(status.conflicts) != 0 {
			color = chrome.danger
		} else if len(status.status.Changes) != 0 {
			color = chrome.accent
		}
		if color == 0 {
			return core.Style{}
		}
		return core.Style{Color: color, Hover: &core.Style{Background: color.Faded(0.1)}}
	}
	return button
}

func (w *workbench) linuxCaption(chrome palette) core.Node {
	return core.View{
		Style:   core.Style{Direction: core.Row},
		Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOff},
		Children: []core.Node{
			iconAction(chrome, w.tr("Minimize", "最小化"), lucide.Minus(), w.cx.MinimizeWindow),
			iconAction(chrome, w.tr("Maximize or restore", "最大化或还原"), lucide.Square(), w.cx.ToggleMaximize),
			iconAction(chrome, w.tr("Close", "关闭"), lucide.X(), w.cx.CloseWindow),
		},
	}
}
