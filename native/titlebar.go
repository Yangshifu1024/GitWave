package main

import (
	"path/filepath"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

const titleBarHeight = 40

func (w *workbench) fullTitleBarWidths() (spacing, toolLabels core.Points) {
	if w.cx.Env().OS() == core.OSMacOS {
		return 1030, 1152
	}
	// The application menu shares this row on Windows and Linux.
	return 1110, 1248
}

func (w *workbench) configureTitleBar() {
	w.cx.SetTitle("GitWave")
	w.cx.SetTitleBar(core.TitleBarHidden, titleBarHeight)
	w.cx.SetWindowControls(core.WindowControls{TrafficLights: core.Point{X: 14, Y: 14}})
	if w.cx.Env().OS() == core.OSMacOS {
		w.cx.SetApplicationMenu([]core.MenuItem{
			{Label: "GitWave", Items: []core.MenuItem{
				{Role: core.MenuQuit},
			}},
			{Label: w.tr("Edit", "编辑"), Items: []core.MenuItem{
				{Role: core.MenuUndo}, {Role: core.MenuRedo}, {Separator: true},
				{Role: core.MenuCut}, {Role: core.MenuCopy}, {Role: core.MenuPaste}, {Role: core.MenuSelectAll},
			}},
			{Label: w.tr("Repository", "仓库"), Items: []core.MenuItem{
				{Label: w.tr("Add local repository", "添加本地仓库"), OnSelect: w.pick},
			}},
		})
	}
}

func (w *workbench) titleBar(cx *core.Scope, chrome palette) core.Node {
	width := core.NewSignal(cx, core.Points(0))
	fullSpacing, _ := w.fullTitleBarWidths()
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
	menu := func(label string, items ...core.MenuItem) core.Node {
		return core.View{
			Role: core.RoleButton, Label: label,
			Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOff},
			OnPress: func(e core.PressEvent) { w.cx.PopupMenu(items, e.PagePosition, nil) },
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
			Children: []core.Node{textLabel(label, chrome.muted, 12)},
		}
	}
	dragArea := core.View{TestID: "titlebar-drag",
		Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOn},
		Style:   core.Style{FlexGrow: 1, MinWidth: core.Pt(8), Height: core.Pt(titleBarHeight)}}
	row := []core.Node{
		core.Show{When: func() bool { return cx.Env().OS() != core.OSMacOS }, Then: func(*core.Scope) core.Node {
			return core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{
				menu(w.tr("File", "文件"), core.MenuItem{Label: w.tr("Exit", "退出"), OnSelect: w.cx.CloseWindow}),
				menu(w.tr("Workspace", "工作区"), core.MenuItem{Label: w.tr("New workspace", "新建工作区"), Disabled: true}),
				menu(w.tr("Repository", "仓库"), core.MenuItem{Label: w.tr("Add local repository", "添加本地仓库"), OnSelect: w.pick}),
				menu(w.tr("Branch", "分支"), core.MenuItem{Label: w.tr("New branch", "新建分支"), Disabled: true}),
			}}
		}},
		w.workspaceSelector(chrome),
		core.View{Style: core.Style{Direction: core.Row, Gap: 4}, Children: []core.Node{
			w.titleAction(chrome, clientWidth, w.tr("Files", "文件"), lucide.FolderOpen(), true, false, w.revealRepository),
			w.titleAction(chrome, clientWidth, w.tr("Terminal", "终端"), lucide.SquareTerminal(), true, false, nil),
			w.titleAction(chrome, clientWidth, w.tr("Editor", "编辑器"), lucide.SquareCode(), true, false, nil),
		}},
		dragArea,
		core.View{Style: core.Style{Direction: core.Row, Gap: 4}, Children: []core.Node{
			w.titleAction(chrome, clientWidth, w.tr("Changes", "变更"), lucide.FileDiff(), false, true, nil),
			w.titleAction(chrome, clientWidth, w.tr("Stash", "贮藏"), lucide.Archive(), false, false, nil),
			w.titleAction(chrome, clientWidth, w.tr("Fetch", "抓取"), lucide.ArrowDownUp(), false, false, nil),
		}},
		core.View{Style: core.Style{Width: core.Pt(1), Height: core.Pt(32), Background: chrome.border}},
		core.View{Style: core.Style{Direction: core.Row, Gap: 4}, Children: []core.Node{
			w.titleAction(chrome, clientWidth, w.tr("Pull", "拉取"), lucide.ArrowDown(), false, true, nil),
			w.titleAction(chrome, clientWidth, w.tr("Push", "推送"), lucide.ArrowUp(), false, true, nil),
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
			w.cx.PopupMenu([]core.MenuItem{
				{Label: w.tr("Add local repository", "添加本地仓库"), OnSelect: w.pick},
			}, e.PagePosition, nil)
		},
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Height: core.Pt(28),
			Padding: core.Edges{Left: 10, Right: 10}, Gap: 6, CornerRadius: 6, BorderWidth: 1,
			BorderColor: chrome.border, Hover: &core.Style{Background: chrome.elevated},
			FocusVisible: &core.Style{OutlineWidth: 1, OutlineColor: chrome.accent}},
		Children: []core.Node{
			core.Icon{SVG: lucide.FolderGit2(), Style: core.Style{Color: chrome.muted, FontSize: 14}},
			textLabel(label, chrome.muted, 12),
			core.Icon{SVG: lucide.ChevronDown(), Style: core.Style{Color: chrome.muted, FontSize: 12}},
		},
	}
}

func (w *workbench) titleAction(chrome palette, windowWidth func() core.Points, label string, icon *core.SVG, compact, count bool, invoke func()) core.Node {
	items := []core.Node{core.Icon{SVG: icon, Style: core.Style{Color: chrome.muted, FontSize: 14}}}
	fullSpacing, toolLabels := w.fullTitleBarWidths()
	minimum := core.Points(940)
	if compact {
		minimum = toolLabels
	}
	items = append(items, core.Show{When: func() bool { return windowWidth() >= minimum }, Then: func(*core.Scope) core.Node {
		return textLabel(label, chrome.muted, 12)
	}})
	if count {
		items = append(items, core.Show{When: func() bool { return windowWidth() >= fullSpacing }, Then: func(*core.Scope) core.Node {
			return core.View{Style: core.Style{Width: core.Pt(25)}}
		}})
	}
	button := toolbarAction(chrome, label, func() {
		if invoke != nil {
			invoke()
		}
	}, items...)
	button.Desktop = core.DesktopViewProps{WindowDrag: core.WindowDragOff}
	button.Disabled = func() bool { return invoke == nil || w.path.Get() == "" }
	button.Style.Disabled = &core.Style{Opacity: core.Alpha(.4)}
	return button
}

func (w *workbench) revealRepository() {
	if repo := w.path.Get(); repo != "" {
		w.cx.OpenFile(core.SharedFile{Path: repo}, func(_ bool, err error) {
			if err != nil {
				w.historyError.Set(err.Error())
			}
		})
	}
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

func (w *workbench) repositoryTabs(chrome palette) core.Node {
	return core.Show{When: func() bool { return w.path.Get() != "" }, Then: func(*core.Scope) core.Node {
		return core.View{
			TestID: "repository-tabs",
			Style: core.Style{Direction: core.Row, Height: core.Pt(32), Background: chrome.bar,
				Borders: core.Edges{Bottom: 1}, BorderColor: chrome.border},
			Children: []core.Node{core.View{
				Role:     core.RoleTab,
				Selected: func() bool { return true },
				Style: core.Style{Height: core.Pt(32), Padding: core.Edges{Left: 16, Right: 16},
					JustifyContent: core.JustifyCenter, Borders: core.Edges{Bottom: 2}, BorderColor: chrome.accent},
				Children: []core.Node{core.Text{
					ContentFunc: func() string { return filepath.Base(w.path.Get()) },
					MaxLines:    1,
					Style:       core.Style{FontSize: 12, FontWeight: 500, Color: chrome.text, MaxWidth: core.Pt(192)},
				}},
			}},
		}
	}}
}
