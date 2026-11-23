package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type palette struct {
	bar, panel, elevated, border, text, muted, accent, hover, selected core.Color
	add, remove, addWord, removeWord, hunk                             core.Color
}

func colors(dark bool) palette {
	rgb := core.RGB
	if dark {
		return palette{
			bar: rgb(0x262628), panel: rgb(0x2a2a2d), elevated: rgb(0x313134),
			border: rgb(0x333336), text: rgb(0xefeff1), muted: rgb(0xa8a8ad),
			accent: rgb(0x2687ee), hover: rgb(0x363639), selected: rgb(0x263e54),
			add: rgb(0x283a32), remove: rgb(0x452e35), addWord: rgb(0x30523e),
			removeWord: rgb(0x633b42), hunk: rgb(0x313134),
		}
	}
	return palette{
		bar: rgb(0xececee), panel: rgb(0xf8f8f8), elevated: rgb(0xf4f4f5),
		border: rgb(0xdbdbdd), text: rgb(0x1b1b1d), muted: rgb(0x55555b),
		accent: rgb(0x2687ee), hover: rgb(0xdfdfdf), selected: rgb(0xe3eef8),
		add: rgb(0xe7f6eb), remove: rgb(0xfce7e9), addWord: rgb(0xc5e9ce),
		removeWord: rgb(0xf8c4c8), hunk: rgb(0xf0f0f3),
	}
}

func (w *workbench) view() core.Node {
	w.configureTitleBar()
	w.cx.OnKey("mod+k", func() { w.search.Set(true) })
	w.cx.OnKey("mod+r", func() {
		if path := w.path.Get(); path != "" {
			w.open(path)
		}
	})
	return core.Dynamic(func(cx *core.Scope) core.Node {
		chrome := colors(cx.Env().Dark())
		return core.View{Style: core.Style{FlexGrow: 1, Background: chrome.panel, Color: chrome.text},
			Children: []core.Node{
				w.titleBar(cx, chrome),
				w.repositoryTabs(chrome),
				split(cx, chrome, splitSpec{"navigator", base.Horizontal, 19, 15, 32}, w.tr("Resize sidebar", "调整侧栏宽度"),
					func() bool { return !w.sideHidden.Get() }, w.sidebar(chrome),
					core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
						w.historyBar(chrome),
						split(cx, chrome, splitSpec{"history-details", base.Vertical, 42, 22, 72}, w.tr("Resize history", "调整提交列表高度"),
							func() bool { return !w.focus.Get() }, w.history(cx, chrome), w.inspector(cx, chrome)),
					}}),
				w.statusBar(chrome),
				w.searchDialog(),
			}}
	})
}

func bar(chrome palette, height core.Points, background core.Color, children ...core.Node) core.View {
	return core.View{Style: core.Style{
		Direction: core.Row, AlignItems: core.AlignCenter, Height: core.Pt(height),
		Padding: core.Edges{Left: 8, Right: 8}, Gap: 8, Background: background,
		Borders: core.Edges{Bottom: 1}, BorderColor: chrome.border,
	}, Children: children}
}

func textLabel(value string, color core.Color, size core.Points) core.Text {
	return core.Text{Content: value, MaxLines: 1, Style: core.Style{FontSize: size, Color: color}}
}

func action(chrome palette, label string, invoke func()) core.Node {
	return toolbarAction(chrome, label, invoke, textLabel(label, chrome.text, 12))
}

func iconAction(chrome palette, label string, icon *core.SVG, invoke func()) core.Node {
	button := toolbarAction(chrome, label, invoke, core.Icon{SVG: icon, Style: core.Style{FontSize: 14, Color: chrome.muted}})
	button.Tooltip = textLabel(label, chrome.text, 12)
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
		Style: core.Style{FontSize: 13, Color: chrome.muted, TextAlign: core.TextAlignCenter},
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

func (w *workbench) sidebar(chrome palette) core.Node {
	return core.View{Style: core.Style{FlexGrow: 1, Background: chrome.bar},
		Children: []core.Node{
			core.View{Style: core.Style{Padding: core.Edges{Top: 8, Bottom: 8},
				Borders: core.Edges{Bottom: 1}, BorderColor: chrome.border},
				Children: []core.Node{
					w.navRow(chrome, "History", lucide.Clock(), true),
					w.navRow(chrome, "Changes", lucide.FileDiff(), false),
				}},
			bar(chrome, 32, chrome.bar, textLabel(w.tr("Branches", "分支"), chrome.muted, 12)),
			core.List{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, EstimatedSize: 28,
				Count: func() int { return len(w.repo.Get().Branches) },
				Row: func(_ *core.Scope, row core.ListRow) core.Node {
					return core.Dynamic(func(*core.Scope) core.Node {
						branches := w.repo.Get().Branches
						if row.Index() >= len(branches) {
							return core.View{}
						}
						b := branches[row.Index()]
						label := core.Text{Content: b.Name, MaxLines: 1,
							Style: core.Style{FontSize: 12, Color: chrome.text}}
						var current core.Node
						if b.Current {
							current = textLabel("HEAD", chrome.accent, 10)
						}
						content := []core.Node{label}
						if b.Upstream != "" {
							content = append(content, textLabel("→ "+b.Upstream, chrome.muted, 10))
						}
						return core.View{
							Role: core.RoleButton, Label: b.Name,
							OnPress: func(core.PressEvent) { w.locateCommit(b.Tip) },
							Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 7,
								Padding: core.Edges{Left: 12, Right: 8}, MinHeight: core.Pt(28),
								Borders: core.Edges{Left: 3}, BorderColor: core.Transparent,
								Hover:        &core.Style{Background: chrome.hover},
								FocusVisible: &core.Style{OutlineWidth: 1, OutlineColor: chrome.accent, OutlineOffset: -1}},
							StyleFunc: func() core.Style {
								if b.Tip != "" && w.selected.Get() == b.Tip && b.Current {
									return core.Style{Background: chrome.selected, BorderColor: chrome.accent}
								}
								return core.Style{}
							},
							Children: []core.Node{
								core.Icon{SVG: lucide.GitBranch(), Style: core.Style{FontSize: 14, Color: chrome.muted}},
								core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: content}, current,
							},
						}
					})
				}},
		}}
}

func (w *workbench) navRow(chrome palette, label string, icon *core.SVG, selected bool) core.Node {
	style := core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
		Padding: core.Edges{Left: 8}, Height: core.Pt(32),
		Borders: core.Edges{Left: 3}, BorderColor: core.Transparent}
	if selected {
		style.Background, style.BorderColor = chrome.selected, chrome.accent
	}
	return core.View{Style: style, Children: []core.Node{
		core.Icon{SVG: icon, Style: core.Style{FontSize: 16, Color: chrome.muted}}, textLabel(label, chrome.text, 13),
	}}
}

func (w *workbench) historyBar(chrome palette) core.Node {
	return bar(chrome, 40, chrome.elevated,
		core.Dynamic(func(*core.Scope) core.Node {
			icon := lucide.PanelLeftClose()
			if w.sideHidden.Get() {
				icon = lucide.PanelLeftOpen()
			}
			return iconAction(chrome, w.tr("Toggle navigator", "显示或隐藏导航"), icon, func() { w.sideHidden.Set(!w.sideHidden.Get()) })
		}),
		textLabel("History", chrome.text, 14),
		core.Icon{SVG: lucide.GitBranch(), Style: core.Style{FontSize: 13, Color: chrome.accent}},
		core.Text{
			ContentFunc: func() string { return w.repo.Get().Branch },
			MaxLines:    1, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 12, Color: chrome.muted},
		},
		toolbarAction(chrome, w.tr("Search commits", "搜索 commit"), func() { w.search.Set(true) },
			core.Icon{SVG: lucide.Search(), Style: core.Style{FontSize: 14, Color: chrome.muted}},
			textLabel(w.tr("Search commits", "搜索 commit"), chrome.text, 12)),
		core.Dynamic(func(*core.Scope) core.Node {
			label := w.tr("Focus diff", "专注 diff")
			icon := lucide.Maximize2()
			if w.focus.Get() {
				label = w.tr("Restore layout", "恢复布局")
				icon = lucide.Minimize2()
			}
			return toolbarAction(chrome, label, func() { w.focus.Set(!w.focus.Get()) },
				core.Icon{SVG: icon, Style: core.Style{FontSize: 14, Color: chrome.muted}}, textLabel(label, chrome.text, 12))
		}))
}

func (w *workbench) statusBar(chrome palette) core.Node {
	status := bar(chrome, 32, chrome.bar, core.Text{
		ContentFunc: func() string { return w.repo.Get().Branch },
		Style:       core.Style{FontSize: 11, Color: chrome.muted},
	})
	status.Style.Borders = core.Edges{Top: 1}
	return status
}

func (w *workbench) searchDialog() core.Node {
	return ui.Dialog{
		Open: w.search.Get, OnOpenChange: func(open bool, _ base.Reason) { w.search.Set(open) },
		Title: w.tr("Search", "搜索"),
		Children: []core.Node{core.Show{When: w.search.Get, Then: func(*core.Scope) core.Node {
			return core.Input{
				Placeholder: w.tr("Search commits…", "搜索提交…"), AutoFocus: true,
				Style: core.Style{Height: core.Pt(32), BorderWidth: 1, Padding: core.All(6)},
				OnSubmit: func(query string) {
					w.search.Set(false)
					w.resetHistory(strings.TrimSpace(query))
					w.loadHistory()
				},
			}
		}}},
	}
}

func (w *workbench) inspector(cx *core.Scope, chrome palette) core.Node {
	return core.View{Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
		core.Dynamic(func(cx *core.Scope) core.Node {
			switch {
			case w.selected.Get() == "":
				return notice(chrome, w.tr("Select a commit", "选择提交"), "", nil)
			case w.detailLoading.Get():
				return notice(chrome, w.tr("Loading diff…", "正在加载 diff…"), "", nil)
			case w.detailError.Get() != "":
				return notice(chrome, w.detailError.Get(), w.tr("Retry", "重试"), func() {
					sha := w.selected.Get()
					w.selected.Set("")
					w.selectCommit(sha)
				})
			}
			return core.View{Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
				w.commitHeader(chrome, w.details.Get()),
				split(cx, chrome, splitSpec{"history-files", base.Horizontal, 24, 16, 48}, w.tr("Resize files", "调整文件列表宽度"),
					func() bool { return !w.focus.Get() }, w.fileList(chrome), w.diffPanel(chrome)),
			}}
		}),
	}}
}

func (w *workbench) commitHeader(chrome palette, details commitDetails) core.Node {
	title, body, _ := strings.Cut(details.Message, "\n")
	children := []core.Node{
		core.Text{Content: title, Selectable: core.SelectableOn, MaxLines: 2, Style: core.Style{
			FontSize: 14, FontWeight: 600, Color: chrome.text}},
		core.Text{Content: fmt.Sprintf("%s · %s · %s", shortSHA(details.SHA), details.Author,
			time.Unix(details.Time, 0).Format(w.tr("Jan 2, 2006 15:04", "2006年1月2日 15:04"))),
			Selectable: core.SelectableOn, MaxLines: 1, Style: core.Style{FontSize: 12, Color: chrome.muted}},
	}
	if strings.TrimSpace(body) != "" {
		expanded := core.NewSignal(w.cx, false)
		children = append(children, action(chrome, w.tr("Details", "详情"), func() { expanded.Set(!expanded.Get()) }),
			core.Show{When: expanded.Get, Then: func(*core.Scope) core.Node {
				return core.Text{Content: strings.TrimSpace(body), Selectable: core.SelectableOn,
					MaxLines: 5, Style: core.Style{FontFamily: "monospace", FontSize: 12, Color: chrome.text}}
			}})
	}
	return core.View{Style: core.Style{Padding: core.Edges{Top: 8, Bottom: 8, Left: 12, Right: 12},
		Gap: 4, Borders: core.Edges{Bottom: 1}, BorderColor: chrome.border}, Children: children}
}

func (w *workbench) fileList(chrome palette) core.Node {
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Background: chrome.elevated},
		Children: []core.Node{
			bar(chrome, 32, chrome.elevated, textLabel(w.tr("Changed files", "变更文件"), chrome.muted, 12),
				core.View{Style: core.Style{FlexGrow: 1}},
				core.Text{ContentFunc: func() string { return fmt.Sprint(len(w.files.Get())) },
					Style: core.Style{FontSize: 12, Color: chrome.muted}}),
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
								core.Text{Content: path, MaxLines: 1, Style: core.Style{Color: chrome.text, FontSize: 12,
									FontFamily: "monospace", FlexShrink: 1, MinWidth: core.Pt(0)}},
							},
						}
					})
				}},
		}}
}
