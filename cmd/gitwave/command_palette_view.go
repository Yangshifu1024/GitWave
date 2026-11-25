package main

import (
	"fmt"
	"strings"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

type paletteRow struct {
	id, label, author, hint string
	icon                    *core.SVG
	run                     func()
	disabled                func() bool
}

func (w *workbench) paletteRows(form *commandPaletteForm, page paletteSearchPage) []paletteRow {
	settingsHint := "Ctrl+,"
	if w.cx.Env().OS() == core.OSMacOS {
		settingsHint = "⌘,"
	}
	commands := []paletteRow{
		{id: "settings", label: w.tr("Open settings", "打开设置"), icon: lucide.Settings(), hint: settingsHint, run: w.openRefreshSettings},
		{id: "fetch", label: w.tr("Fetch from remote", "从远程获取"), icon: lucide.Download(), run: func() { w.startFetch("") },
			disabled: func() bool { return !w.canSync() || w.fetch.prompt.Get() != nil }},
	}
	query := strings.ToLower(strings.TrimSpace(form.query.Get()))
	rows := make([]paletteRow, 0, len(commands)+len(page.commits))
	for _, command := range commands {
		if strings.Contains(strings.ToLower(command.label), query) {
			rows = append(rows, command)
		}
	}
	for _, c := range page.commits {
		rows = append(rows, paletteRow{id: c.SHA, label: c.Message, author: c.Author, hint: shortSHA(c.SHA),
			icon: lucide.GitCommitHorizontal(), run: func() { w.locatePaletteCommit(c.SHA) }})
	}
	return rows
}

func (w *workbench) runPaletteRow(form *commandPaletteForm, row paletteRow) {
	if w.currentCommandPalette(form) && !form.ai.Get().saving && (row.disabled == nil || !row.disabled()) {
		w.closeCommandPalette()
		row.run()
	}
}

func (w *workbench) commandPaletteDialog(chrome palette) core.Node {
	return core.Show{When: func() bool { return w.paletteForm.Get() != nil }, Then: func(*core.Scope) core.Node {
		form := w.paletteForm.Get()
		overlay := core.RGBA(0, 0.38)
		if w.cx.Env().Dark() {
			overlay = core.RGBA(0, 0.55)
		}
		return base.Dialog{TestID: "command-palette", Label: w.tr("Command palette", "命令面板"),
			Open: func() bool { return w.paletteForm.Get() == form },
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open && w.paletteForm.Get() == form && !form.ai.Get().saving {
					w.closeCommandPalette()
				}
			},
			BackdropStyle: core.Style{Background: overlay, BackdropFilters: []core.Filter{core.Blur(4)}},
			LayerStyle:    core.Style{JustifyContent: core.JustifyStart},
			Before: []core.Node{core.View{PointerEvents: core.PointerEventsNone,
				Style: core.Style{Height: core.Pct(12)}}},
			PopupStyle: core.Style{Width: core.Pt(560), MaxWidth: core.Pct(92), CornerRadius: 12, BorderWidth: 1,
				BorderColor: chrome.border, Background: chrome.elevated, Overflow: core.Hidden},
			Children: []core.Node{core.Dynamic(func(cx *core.Scope) core.Node {
				return w.commandPaletteContent(cx, chrome, form)
			})},
		}
	}}
}

func (w *workbench) commandPaletteContent(cx *core.Scope, chrome palette, form *commandPaletteForm) core.Node {
	var list, inputRef core.Ref
	rows := core.NewMemoFunc(cx, func() []paletteRow { return w.paletteRows(form, form.page.Get()) }, nil)
	cx.OnKey("mod+k", w.toggleCommandPalette)
	move := func(delta int) {
		count := len(rows.Get())
		if !w.currentCommandPalette(form) || count == 0 || form.ai.Get().busy() {
			return
		}
		i := min(max(form.selection.Get()+delta, 0), count-1)
		form.selection.Set(i)
		list.ScrollToIndexAt(i, 0.5, 0, false)
	}
	cx.OnKeyWith("up", core.KeyOptions{When: core.KeyAlways, Repeat: true}, func() { move(-1) })
	cx.OnKeyWith("down", core.KeyOptions{When: core.KeyAlways, Repeat: true}, func() { move(1) })
	placeholder := w.tr("Type a command, or describe what you want to do…", "输入命令，或描述你想做的事…")
	input := core.Input{TestID: "palette-input", Ref: &inputRef, Label: placeholder, Placeholder: placeholder,
		Value: form.query.Get, OnChange: func(value string) { w.updatePaletteQuery(form, value) }, AutoFocus: true,
		NoAutoCorrect: true, MaxLength: 2048,
		Style: core.Style{Height: core.Pt(30), FlexGrow: 1, MinWidth: core.Pt(0), FontSize: chrome.uiSize(13), Color: chrome.text},
		OnSubmit: func(string) {
			if form.ai.Get().busy() {
				return
			}
			visible, index := rows.Get(), form.selection.Get()
			if index >= 0 && index < len(visible) {
				w.runPaletteRow(form, visible[index])
			} else {
				w.askPaletteAI(form)
			}
		}}
	search := core.Show{When: func() bool { return !form.ai.Get().saving }, Then: func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
			Padding: core.Edges{Top: 8, Bottom: 8, Left: 12, Right: 12},
			Borders: core.Edges{Bottom: 1}, BorderColor: chrome.border}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
				FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Height: core.Pt(32), CornerRadius: 6,
				Padding:     core.Edges{Left: 8, Right: 4},
				FocusWithin: &core.Style{OutlineWidth: 2, OutlineColor: chrome.accent, OutlineOffset: -1}},
				Children: []core.Node{
					core.Icon{SVG: lucide.Search(), Style: core.Style{FontSize: 16, Color: chrome.muted}}, input,
					core.Show{When: func() bool { return form.query.Get() != "" }, Then: func(*core.Scope) core.Node {
						button := iconAction(chrome, w.tr("Clear search", "清除搜索"), lucide.Delete(), func() {
							w.updatePaletteQuery(form, "")
							inputRef.Focus()
						})
						button.TestID = "palette-clear"
						return button
					}},
				}},
			core.Show{When: func() bool { return strings.TrimSpace(form.query.Get()) != "" }, Then: func(*core.Scope) core.Node {
				return core.Dynamic(func(*core.Scope) core.Node {
					label := w.tr("Ask AI", "询问 AI")
					if form.ai.Get().loading {
						label = w.tr("Thinking…", "思考中…")
					}
					foreground := core.RGB(0xffffff)
					button := toolbarAction(chrome, label, func() { w.askPaletteAI(form) },
						core.Icon{SVG: lucide.Sparkles(), Style: core.Style{FontSize: 13, Color: foreground}}, chrome.label(label, foreground, 12))
					button.TestID, button.Disabled = "palette-ask-ai", func() bool { return form.ai.Get().busy() }
					button.Style.Background = chrome.accent
					button.Style.Hover = &core.Style{Background: chrome.accent, Opacity: core.Alpha(0.88)}
					button.Style.Disabled = &core.Style{Opacity: core.Alpha(0.4)}
					return button
				})
			}},
		}}
	}}
	return core.View{Children: []core.Node{
		search,
		w.paletteAIResult(chrome, form),
		core.Show{When: func() bool { return !form.ai.Get().busy() }, Then: func(*core.Scope) core.Node {
			return w.paletteResults(chrome, form, &list, rows.Get)
		}},
	}}
}

func (w *workbench) paletteResults(chrome palette, form *commandPaletteForm, list *core.Ref, rows func() []paletteRow) core.Node {
	lineHeight := chrome.uiSize(30)
	return core.View{Style: core.Style{Padding: core.All(8), Gap: 4}, Children: []core.Node{
		core.Dynamic(func(*core.Scope) core.Node {
			return w.paletteSearchStatus(chrome, form, len(rows()))
		}),
		core.List{TestID: "palette-results", Ref: list, EstimatedSize: lineHeight,
			StyleFunc: func() core.Style { return core.Style{Height: core.Pt(lineHeight * core.Points(min(len(rows()), 9)))} },
			Count:     func() int { return len(rows()) }, Key: func(i int) any { return rows()[i].id },
			Row: func(_ *core.Scope, item core.ListRow) core.Node {
				return core.Dynamic(func(*core.Scope) core.Node {
					visible, i := rows(), item.Index()
					if i < 0 || i >= len(visible) {
						return core.View{}
					}
					return w.paletteResultRow(chrome, form, visible[i], i, lineHeight)
				})
			}},
		core.Show{When: func() bool { return strings.TrimSpace(form.query.Get()) != "" }, Then: func(*core.Scope) core.Node {
			return chrome.label(w.tr("Enter asks AI to interpret this request", "按 Enter 由 AI 解析此请求"), chrome.muted, 11)
		}},
	}}
}

func (w *workbench) paletteSearchStatus(chrome palette, form *commandPaletteForm, results int) core.Node {
	page := form.page.Get()
	children := make([]core.Node, 0, 5)
	if page.loading {
		children = append(children, core.Text{Content: w.tr("Searching commits…", "正在搜索提交…"), Role: core.RoleStatus,
			Style: core.Style{Color: chrome.muted, FontSize: chrome.uiSize(12)}})
	}
	if page.failure != "" {
		children = append(children, core.Text{Content: page.failure, Role: core.RoleAlert,
			Style: core.Style{Color: chrome.danger, FontSize: chrome.uiSize(12)}},
			action(chrome, w.tr("Retry", "重试"), func() { w.searchPalette(form, false) }))
	}
	if page.scanned > 0 {
		children = append(children, chrome.label(fmt.Sprintf(w.tr("Searched %d of %d commits", "已搜索 %d / %d 个提交"), page.scanned, page.size), chrome.muted, 11))
	}
	if page.truncated {
		children = append(children, chrome.label(fmt.Sprintf(w.tr("Search limited to %d commits", "搜索范围限于 %d 个提交"), page.size), chrome.warning, 11))
	}
	if page.cursor != "" && page.failure == "" {
		button := toolbarAction(chrome, w.tr("Continue searching older commits", "继续搜索更早的提交"), func() { w.searchPalette(form, false) },
			chrome.label(w.tr("Continue searching older commits", "继续搜索更早的提交"), chrome.text, 12))
		button.TestID, button.Disabled = "palette-more", func() bool { return form.page.Get().loading }
		children = append(children, button)
	}
	if results > 0 && len(page.commits) == results {
		children = append(children, chrome.label(w.tr("Commits", "提交"), chrome.muted, 11))
	} else if results == 0 && !page.loading && page.failure == "" {
		children = append(children, chrome.label(w.tr("Press Enter to ask AI: e.g. “create branch fix/auth”, “tag this as v1.2.0”, “explain the last commit”",
			"按 Enter 询问 AI，例如创建分支 fix/auth、标记为 v1.2.0 或解释最后一次提交"), chrome.muted, 12))
	}
	return core.View{Style: core.Style{Gap: 4}, Children: children}
}

func (w *workbench) paletteResultRow(chrome palette, form *commandPaletteForm, row paletteRow, index int, height core.Points) core.View {
	children := []core.Node{core.Icon{SVG: row.icon, Style: core.Style{FontSize: 14, Color: chrome.muted}}}
	hint := core.Text{Content: row.hint, MaxLines: 1, Style: core.Style{Color: chrome.muted,
		FontFamily: chrome.fonts.code, FontSize: chrome.codeSize(11)}}
	if row.author != "" {
		children = append(children, hint)
	}
	children = append(children, core.Text{Content: row.label, MaxLines: 1, Style: core.Style{
		Color: chrome.text, FontSize: chrome.uiSize(12), FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}})
	if row.author != "" {
		children = append(children, chrome.label(row.author, chrome.muted, 11))
	} else if row.hint != "" {
		children = append(children, hint)
	}
	button := toolbarAction(chrome, row.label, func() { w.runPaletteRow(form, row) }, children...)
	button.TestID, button.Disabled = "palette-row-"+row.id, row.disabled
	button.Style.Height, button.Style.JustifyContent = core.Pt(height), core.JustifyStart
	button.OnMouseEnter = func(core.MouseEvent) { form.selection.Set(index) }
	button.Selected = func() bool { return form.selection.Get() == index }
	button.StyleFunc = func() core.Style {
		if button.Selected() {
			return core.Style{Background: chrome.selected}
		}
		return core.Style{}
	}
	return button
}

func (w *workbench) paletteAIResult(chrome palette, form *commandPaletteForm) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		state := form.ai.Get()
		if state.busy() {
			message := w.tr("Interpreting your request…", "正在解析你的请求…")
			if state.saving {
				message = w.tr("Saving stash…", "正在保存贮藏…")
			}
			return core.Text{Content: message, Role: core.RoleStatus,
				Style: core.Style{Padding: core.All(12), Color: chrome.muted, FontSize: chrome.uiSize(12)}}
		}
		children := make([]core.Node, 0, 2)
		if state.intent != nil {
			children = append(children, w.paletteIntentCard(chrome, form, state.intent))
		}
		if state.failure != "" {
			children = append(children, core.Text{TestID: "palette-ai-error", Role: core.RoleAlert, Content: state.failure,
				Style: core.Style{Padding: core.All(8), Color: chrome.danger, FontSize: chrome.uiSize(12)}})
		}
		return core.View{Children: children}
	})
}

func (w *workbench) paletteIntentCard(chrome palette, form *commandPaletteForm, intent *paletteIntent) core.Node {
	label := w.paletteActionLabel(intent.action)
	heading := []core.Node{core.Icon{SVG: lucide.ArrowRight(), Style: core.Style{FontSize: 13, Color: chrome.accent}}, chrome.label(label, chrome.text, 13)}
	confirmLabel := w.tr("Run", "执行")
	if intent.confirm {
		heading = append(heading, chrome.label(w.tr("Needs confirmation", "需确认"), chrome.muted, 11))
		confirmLabel = w.tr("Confirm", "确认")
	}
	explanation := intent.explanation
	if explanation == "" {
		explanation = w.tr("Run this action?", "执行此操作？")
	}
	confirm := toolbarAction(chrome, confirmLabel, func() { w.runPaletteIntent(form, intent) }, chrome.label(confirmLabel, chrome.accent, 12))
	confirm.TestID = "palette-confirm"
	cancelLabel := w.tr("Cancel", "取消")
	cancel := toolbarAction(chrome, cancelLabel, form.cancelAI, chrome.label(cancelLabel, chrome.text, 12))
	cancel.TestID = "palette-cancel-intent"
	children := []core.Node{
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6}, Children: heading},
		core.Text{Content: explanation, MaxLines: 3, Style: core.Style{Color: chrome.muted, FontSize: chrome.uiSize(12)}},
	}
	details := []string{}
	if intent.name != "" {
		details = append(details, intent.name)
	}
	if intent.oid != "" {
		details = append(details, shortSHA(intent.oid))
	}
	if intent.action == "checkout_branch" || intent.action == "stash_changes" {
		from := strings.TrimPrefix(intent.source.Ref, "refs/heads/")
		if from == "" {
			from = shortSHA(intent.source.OID)
		}
		details = append(details, fmt.Sprintf(w.tr("From %s", "源工作区分支 %s"), from))
	}
	if intent.action == "stash_changes" {
		details = append(details, w.tr("Includes untracked files", "包含未跟踪文件"))
		if intent.message != "" {
			details = append(details, intent.message)
		}
	}
	if len(details) != 0 {
		children = append(children, core.Text{Content: strings.Join(details, " · "), MaxLines: 3,
			Style: core.Style{Color: chrome.muted, FontFamily: chrome.fonts.code, FontSize: chrome.codeSize(11)}})
	}
	children = append(children, core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifyEnd, Gap: 5}, Children: []core.Node{cancel, confirm}})
	return core.View{TestID: "palette-intent", Style: core.Style{Margin: core.All(8), Padding: core.All(8), Gap: 4,
		Background: chrome.panel, CornerRadius: 6, BorderWidth: 1, BorderColor: chrome.border}, Children: children}
}

func (w *workbench) paletteActionLabel(action string) string {
	switch action {
	case "explain_commit":
		return w.tr("Explain commit", "解释 commit")
	case "locate_commit":
		return w.tr("Locate commit in history", "在历史中定位 commit")
	case "create_branch":
		return w.tr("Create branch", "创建分支")
	case "checkout_branch":
		return w.tr("Checkout branch", "检出分支")
	case "create_tag":
		return w.tr("Create tag", "创建 tag")
	case "stash_changes":
		return w.tr("Stash changes", "贮藏改动")
	case "fetch_remotes":
		return w.tr("Fetch remotes", "获取远程")
	default:
		return w.tr("No matching action", "没有匹配的操作")
	}
}
