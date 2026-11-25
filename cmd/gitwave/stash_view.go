package main

import (
	"fmt"
	"slices"
	"strings"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

func (w *workbench) stashSaveDialog(chrome palette) core.Node {
	pending := func() bool { return w.currentChangesWrite().pending }
	closeForm := func() {
		if !pending() {
			w.stashSave.Set(nil)
		}
	}
	cancelLabel := w.tr("Cancel", "取消")
	cancel := toolbarAction(chrome, cancelLabel, closeForm, chrome.label(cancelLabel, chrome.text, 12))
	cancel.TestID, cancel.Disabled = "cancel-stash-save", pending
	saveLabel := w.tr("Save stash", "保存贮藏")
	save := toolbarAction(chrome, saveLabel, w.submitStashSave, chrome.label(saveLabel, chrome.accent, 12))
	save.TestID, save.Disabled = "confirm-stash-save", pending
	return ui.Dialog{
		TestID: "stash-save-dialog", Open: func() bool { return w.stashSave.Get() != nil },
		Title: w.tr("Save stash", "保存贮藏"), Description: w.tr("Save your local changes to a new stash", "将本地变更保存为新的贮藏"),
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				closeForm()
			}
		},
		Children: []core.Node{
			core.Input{TestID: "stash-message", Label: w.tr("Stash message", "贮藏消息"),
				Placeholder: w.tr("Stash message (optional)", "贮藏消息（可选）"),
				Value:       w.stashMessage.Get, OnChange: w.stashMessage.Set, Disabled: pending, AutoFocus: true,
				OnSubmit: func(string) { w.submitStashSave() }, Style: core.Style{
					Height: core.Pt(32), Padding: core.All(6), BorderWidth: 1, BorderColor: chrome.border,
				}},
			ui.Checkbox{TestID: "stash-untracked", Text: w.tr("Stage new files", "暂存新文件"),
				Checked: w.stashUntracked.Get, OnChange: w.stashUntracked.Set, Disabled: pending},
			chrome.label(w.tr("Include new, untracked files in this stash", "将未跟踪的新文件一并贮藏"), chrome.muted, 11),
			core.Show{When: func() bool { return w.stashError.Get() != "" }, Then: func(*core.Scope) core.Node {
				return core.Text{TestID: "stash-save-error", Role: core.RoleAlert, ContentFunc: w.stashError.Get,
					Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.danger}}
			}},
		},
		Footer: []core.Node{cancel, save},
	}
}

func (w *workbench) stashSidebar(chrome palette) core.Node {
	title := w.tr("Stash", "贮藏")
	return core.Show{When: func() bool { return w.path.Get() != "" }, Then: func(cx *core.Scope) core.Node {
		collapsible := core.NewMemo(cx, func() bool {
			state := w.stashList.Get()
			return len(state.entries) > 0 || state.err != ""
		})
		return core.View{Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			if !collapsible.Get() {
				return sidebarHeading(chrome, "stash-toggle", title, nil, nil)
			}
			return sidebarHeading(chrome, "stash-toggle", title, w.stashOpen.Get, func() {
				w.stashOpen.Set(!w.stashOpen.Get())
			})
		}),
			core.Dynamic(func(*core.Scope) core.Node {
				state := w.stashList.Get()
				if !w.stashOpen.Get() {
					return core.View{}
				}
				switch {
				case state.err != "":
					return core.View{Children: []core.Node{sidebarSectionHint(chrome, state.err),
						action(chrome, w.tr("Retry", "重试"), w.loadStashes)}}
				case len(state.entries) == 0:
					return core.View{}
				}
				return core.List{TestID: "stash-list", EstimatedSize: 56,
					Style: core.Style{Height: core.Pt(core.Points(min(168, 56*len(state.entries)))), FlexShrink: 1},
					Count: func() int { return len(state.entries) },
					Key:   func(i int) any { return state.entries[i].Index },
					Row: func(_ *core.Scope, row core.ListRow) core.Node {
						return core.Dynamic(func(*core.Scope) core.Node {
							i := row.Index()
							if i < 0 || i >= len(state.entries) {
								return core.View{}
							}
							return w.stashSidebarRow(chrome, state.entries[i])
						})
					}}
			}),
		}}
	}}
}

func sidebarSectionHint(chrome palette, value string) core.Text {
	return core.Text{Content: value, Style: core.Style{
		FontSize: chrome.uiSize(11), Color: chrome.muted, Padding: core.Edges{Left: 16, Bottom: 8}}}
}

func (w *workbench) stashSidebarRow(chrome palette, entry gitcli.StashEntry) core.View {
	label := w.stashTitle(entry)
	view := func() { w.openStashPreview(entry) }
	row := toolbarAction(chrome, label, view,
		core.Icon{SVG: lucide.Archive(), Style: core.Style{FontSize: 14, Color: chrome.accent}},
		core.View{Style: core.Style{FlexGrow: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
			core.Text{Content: label, MaxLines: 1, Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.text}},
			chrome.label(shortSHA(entry.OID), chrome.muted, 10),
		}})
	row.TestID = "stash-row"
	row.Style.Height = core.Pt(32)
	row.Style.JustifyContent = core.JustifyStart
	viewLabel := w.tr("View", "查看")
	viewButton := toolbarAction(chrome, fmt.Sprintf("%s stash@{%d}", viewLabel, entry.Index), view,
		chrome.label(viewLabel, chrome.muted, 10))
	selected := func() gitcli.StashEntry { return entry }
	apply := w.stashApplyButton(chrome, selected, "stash-row-apply", false, 10)
	pop := w.stashApplyButton(chrome, selected, "stash-row-pop", true, 10)
	drop := w.stashDropButton(chrome, selected, "stash-row-drop", 10)
	for _, button := range []*core.View{&viewButton, &apply, &pop, &drop} {
		button.Style.Height, button.Style.Padding = core.Pt(24), core.Edges{Left: 4, Right: 4}
	}
	return core.View{Children: []core.Node{row, core.View{
		Style:    core.Style{Direction: core.Row, Padding: core.Edges{Left: 20}},
		Children: []core.Node{viewButton, apply, pop, drop},
	}}}
}

func (w *workbench) stashApplyButton(chrome palette, entry func() gitcli.StashEntry, id string, pop bool, size core.Points) core.View {
	label := w.tr("Apply", "应用")
	tip := w.tr("Apply this stash and keep it in the list", "应用这个 stash，保留它在列表中")
	if pop {
		label = w.tr("Apply & delete", "应用并删除")
		tip = w.tr("Apply this stash, then drop it from the list", "应用这个 stash，然后从列表中删除它")
	}
	button := toolbarAction(chrome, label, func() { w.requestStashApply(entry(), pop) }, chrome.label(label, chrome.accent, size))
	button.TestID = id
	button.Tooltip = chrome.label(tip, chrome.text, 12)
	button.Disabled = func() bool {
		return !w.canApplyStash() || !slices.ContainsFunc(w.stashList.Get().entries, entry().SameSelection)
	}
	return button
}

func (w *workbench) stashDropButton(chrome palette, entry func() gitcli.StashEntry, id string, size core.Points) core.View {
	label := w.tr("Delete", "删除")
	button := toolbarAction(chrome, label, func() { w.requestStashDrop(entry()) }, chrome.label(label, chrome.danger, size))
	button.TestID = id
	button.Tooltip = chrome.label(w.tr("Delete this stash (cannot be undone in the UI)", "删除这个 stash（无法通过界面恢复）"), chrome.text, 12)
	button.Disabled = func() bool {
		return !w.canManageStash() || !slices.ContainsFunc(w.stashList.Get().entries, entry().SameSelection)
	}
	return button
}

func (w *workbench) stashApplyDialog(chrome palette) core.Node {
	return ui.Dialog{
		TestID: "stash-apply-dialog", Role: core.RoleAlertDialog,
		Open:  func() bool { prompt := w.stashApply.Get(); return prompt != nil && !prompt.checking },
		Title: w.tr("Working copy has uncommitted changes", "工作区有未提交的改动"),
		DescriptionFunc: func() string {
			prompt := w.stashApply.Get()
			if prompt == nil {
				return ""
			}
			count := len(prompt.shown.Changes)
			if count == 1 {
				return w.tr("1 uncommitted change is present. Applying this stash may conflict.", "当前有 1 个未提交改动，应用这个 stash 可能产生冲突。")
			}
			return fmt.Sprintf(w.tr("%d uncommitted changes are present. Applying this stash may conflict.",
				"当前有 %d 个未提交改动，应用这个 stash 可能产生冲突。"), count)
		},
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				w.stashApply.Set(nil)
			}
		},
		Children: []core.Node{core.Text{ContentFunc: func() string {
			if prompt := w.stashApply.Get(); prompt != nil {
				return w.stashTitle(prompt.entry)
			}
			return ""
		}, Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.text}}},
		Footer: []core.Node{action(chrome, w.tr("Cancel", "取消"), func() { w.stashApply.Set(nil) }),
			action(chrome, w.tr("Continue", "继续"), w.submitStashApply)},
	}
}

func (w *workbench) stashPreviewDialog(chrome palette) core.Node {
	entry := func() gitcli.StashEntry { return w.stashPreview.Get().entry }
	return ui.Dialog{
		TestID: "stash-preview-dialog", Open: func() bool { return w.stashPreview.Get().entry.OID != "" },
		TitleFunc: func() string { return w.stashTitle(w.stashPreview.Get().entry) },
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				w.closeStashPreview()
			}
		},
		Style: core.Style{Width: core.Pct(91), Height: core.Pct(80)},
		Children: []core.Node{core.Show{When: func() bool { return w.stashPreviewNotice() != "" }, Then: func(*core.Scope) core.Node {
			return core.Text{TestID: "stash-action-error", Role: core.RoleAlert, ContentFunc: w.stashPreviewNotice,
				Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.danger}}
		}}, core.View{Style: core.Style{
			Direction: core.Row, FlexGrow: 1, FlexBasis: core.Pt(0), MinHeight: core.Pt(0),
		},
			Children: []core.Node{
				core.Dynamic(func(*core.Scope) core.Node {
					return w.stashFileList(chrome, w.stashFiles.Get())
				}),
				core.View{Style: core.Style{FlexGrow: 1, MinWidth: core.Pt(0)},
					Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
						state := w.stashPreview.Get()
						switch {
						case state.loading:
							return notice(chrome, w.tr("Loading diff…", "正在加载 diff…"), "", nil)
						case state.err != "":
							return notice(chrome, state.err, w.tr("Retry", "重试"), func() { w.loadStashPreview(state) })
						case state.path == "":
							return notice(chrome, w.tr("No file selected", "未选择文件"), "", nil)
						}
						chip := ""
						if len(state.diff.Files) != 0 && state.diff.Files[0].Untracked {
							chip = w.tr("Untracked", "未跟踪")
						}
						return w.fileDiffView(chrome, state.diff, fileDiffOptions{
							collapsed: w.stashCollapsed, chip: chip, expanded: state.expanded,
							expand: func() {
								state.expanded = true
								w.loadStashPreview(state)
							},
						})
					})}},
			},
		}},
		Footer: []core.Node{action(chrome, w.tr("Close", "关闭"), w.closeStashPreview),
			w.stashDropButton(chrome, entry, "stash-preview-drop", 12), w.stashApplyButton(chrome, entry, "stash-preview-pop", true, 12),
			w.stashApplyButton(chrome, entry, "stash-preview-apply", false, 12)},
	}
}

func (w *workbench) stashDropDialog(chrome palette) core.Node {
	closePrompt := func() { w.stashDrop.Set(nil) }
	confirmLabel := w.tr("Delete this stash", "删除这个 stash")
	confirm := toolbarAction(chrome, confirmLabel, w.submitStashDrop, chrome.label(confirmLabel, chrome.danger, 12))
	confirm.TestID = "confirm-stash-drop"
	confirm.Disabled = func() bool { return !w.canSync() }
	return ui.Dialog{
		TestID: "stash-drop-dialog", Role: core.RoleAlertDialog,
		Open: func() bool { return w.stashDrop.Get() != nil },
		TitleFunc: func() string {
			if prompt := w.stashDrop.Get(); prompt != nil {
				return fmt.Sprintf(w.tr("Delete stash@{%d}?", "删除 stash@{%d}？"), prompt.entry.Index)
			}
			return ""
		},
		DescriptionFunc: func() string {
			if prompt := w.stashDrop.Get(); prompt != nil {
				return w.stashTitle(prompt.entry)
			}
			return ""
		},
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				closePrompt()
			}
		},
		Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			prompt := w.stashDrop.Get()
			if prompt == nil || (!prompt.loading && len(prompt.files) == 0) {
				return core.View{}
			}
			if prompt.loading {
				return chrome.label(w.tr("Reading the file list…", "正在读取文件列表…"), chrome.muted, 12)
			}
			count := len(prompt.files)
			names := make([]string, 0, min(5, count))
			for _, file := range prompt.files[:min(5, count)] {
				names = append(names, file.Path)
			}
			pattern := w.tr("%d files · %s", "%d 个文件 · %s")
			if count == 1 {
				pattern = w.tr("%d file · %s", "%d 个文件 · %s")
			}
			summary := fmt.Sprintf(pattern, count, strings.Join(names, ", "))
			if count > len(names) {
				summary += fmt.Sprintf(w.tr(" …and %d more", " …还有 %d 个"), count-len(names))
			}
			return core.Text{TestID: "stash-drop-files", Content: summary, Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.muted}}
		}), chrome.label(w.tr("This cannot be undone in the UI.", "删除后无法通过 UI 恢复。"), chrome.danger, 12)},
		Footer: []core.Node{action(chrome, w.tr("Cancel", "取消"), closePrompt), confirm},
	}
}

func (w *workbench) stashPreviewNotice() string {
	if message := w.stashError.Get(); message != "" {
		return message
	}
	list := w.stashList.Get()
	if entry := w.stashPreview.Get().entry; entry.OID != "" && list.ready && list.err == "" && !slices.ContainsFunc(list.entries, entry.SameSelection) {
		return w.staleStashMessage()
	}
	return ""
}

func (w *workbench) stashFileList(chrome palette, files []fileDiff) core.Node {
	child := core.Node(notice(chrome, w.tr("No changed files", "没有变更文件"), "", nil))
	if len(files) != 0 {
		child = core.List{TestID: "stash-files", Count: func() int { return len(files) }, EstimatedSize: 28,
			Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinHeight: core.Pt(0)},
			Key:   func(i int) any { return files[i].Path },
			Row: func(_ *core.Scope, row core.ListRow) core.Node {
				return core.Dynamic(func(*core.Scope) core.Node {
					i := row.Index()
					if i < 0 || i >= len(files) {
						return core.View{}
					}
					return w.stashFileRow(chrome, files[i])
				})
			}}
	}
	return core.View{Style: core.Style{Width: core.Pct(30), Borders: core.Edges{Right: 1}, BorderColor: chrome.border},
		Children: []core.Node{bar(chrome, 30, chrome.bar, chrome.label(w.tr("Files", "文件"), chrome.muted, 12)), child}}
}

func (w *workbench) stashFileRow(chrome palette, file fileDiff) core.View {
	label, color, icon := file.Path, chrome.muted, lucide.FileDiff()
	switch {
	case file.Untracked:
		label += " · " + w.tr("Untracked", "未跟踪")
		icon = lucide.FileQuestionMark()
	case file.OldSHA == "":
		icon = lucide.FilePlus()
	case file.NewSHA == "":
		icon, color = lucide.FileMinus(), chrome.danger
	}
	row := toolbarAction(chrome, label, func() {
		state := w.stashPreview.Get()
		state.path, state.expanded = file.Path, false
		w.stashCollapsed.Set(false)
		w.loadStashPreview(state)
	}, core.Icon{SVG: icon, Style: core.Style{FontSize: 12, Color: color}},
		core.Text{Content: label, MaxLines: 1, Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.text}})
	row.TestID = "stash-file-row"
	row.Selected = func() bool { return w.stashPreview.Get().path == file.Path }
	row.Style.Height, row.Style.JustifyContent = core.Pt(28), core.JustifyStart
	row.StyleFunc = func() core.Style {
		if row.Selected() {
			return core.Style{Background: chrome.selected}
		}
		return core.Style{}
	}
	return row
}
