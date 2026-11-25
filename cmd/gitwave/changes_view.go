package main

import (
	"fmt"
	"slices"
	"strconv"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

func (w *workbench) changesCount() string {
	status := w.changes.Get()
	if !status.ready || status.bare {
		return ""
	}
	paths := make(map[string]struct{}, len(status.status.Changes))
	for _, group := range [][]workingChange{status.unstaged, status.staged, status.conflicts} {
		for _, item := range group {
			paths[item.key.path] = struct{}{}
		}
	}
	return strconv.Itoa(len(paths))
}

func (w *workbench) mainPanel(cx *core.Scope, chrome palette) core.Node {
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), MinHeight: core.Pt(0)}, Children: []core.Node{
		w.workbenchBar(chrome),
		core.Dynamic(func(cx *core.Scope) core.Node {
			if w.path.Get() == "" {
				return w.historyStatus(chrome)
			}
			if w.changesMode.Get() {
				return w.changesPanel(cx, chrome)
			}
			return split(cx, chrome, splitSpec{"history-details", base.Vertical, 42, 22, 72}, w.tr("Resize history", "调整提交列表高度"),
				func() bool { return !w.focus.Get() }, w.history(cx, chrome), w.inspector(cx, chrome))
		}),
	}}
}

func (w *workbench) changesPanel(cx *core.Scope, chrome palette) core.Node {
	submoduleWarning := func() string { return w.changes.Get().status.SubmoduleWarning }
	return core.View{TestID: "working-changes", Style: core.Style{FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), MinHeight: core.Pt(0)},
		Children: []core.Node{core.Show{When: func() bool { return submoduleWarning() != "" }, Then: func(*core.Scope) core.Node {
			message := core.Text{TestID: "submodule-status-warning", Role: core.RoleAlert, MaxLines: 2,
				ContentFunc: func() string {
					return w.tr("Could not read all submodule changes. ", "无法读取全部子模块改动。") + submoduleWarning()
				},
				Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.warning, FlexGrow: 1, FlexShrink: 1}}
			return bar(chrome, 40, chrome.elevated, message,
				iconAction(chrome, w.tr("Copy error", "复制错误"), lucide.Copy(), func() { w.cx.SetClipboard(submoduleWarning()) }))
		}}, core.Show{When: func() bool { return w.currentChangesWrite().err != "" }, Then: func(*core.Scope) core.Node {
			message := core.Text{TestID: "changes-write-error", ContentFunc: func() string { return w.currentChangesWrite().err },
				Role: core.RoleAlert, MaxLines: 2, Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.danger, FlexGrow: 1, FlexShrink: 1}}
			return bar(chrome, 40, chrome.elevated, message,
				iconAction(chrome, w.tr("Copy error", "复制错误"), lucide.Copy(), func() {
					w.cx.SetClipboard(w.currentChangesWrite().err)
				}),
				iconAction(chrome, w.tr("Dismiss", "关闭"), lucide.X(), func() {
					w.setChangesWrite(w.changesWriteKey(), changesWrite{})
				}))
		}}, core.Show{When: func() bool { return w.currentChangesWrite().commit != "" }, Then: func(*core.Scope) core.Node {
			message := core.Text{TestID: "commit-success", Role: core.RoleStatus, MaxLines: 2,
				ContentFunc: func() string {
					result := w.currentChangesWrite()
					summary := fmt.Sprintf(w.tr("Committed %s", "已提交 %s"), shortSHA(result.commit))
					if result.warning != "" {
						return summary + "\n" + result.warning
					}
					return summary
				}, Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.text, FlexGrow: 1, FlexShrink: 1}}
			return bar(chrome, 40, chrome.elevated, message,
				action(chrome, w.tr("View in History", "在历史中查看"), func() { w.locateCommit(w.currentChangesWrite().commit) }),
				iconAction(chrome, w.tr("Dismiss", "关闭"), lucide.X(), func() {
					w.setChangesWrite(w.changesWriteKey(), changesWrite{})
				}))
		}}, split(cx, chrome, splitSpec{"changes-files", base.Horizontal, 36, 28, 58},
			w.tr("Resize files", "调整文件列表宽度"), func() bool { return !w.focus.Get() },
			w.changesFileList(chrome), w.workingDiffPanel(chrome)), w.amendDialog(chrome), w.discardDialog(cx, chrome)}}
}

func (w *workbench) changesFileList(chrome palette) core.Node {
	return core.Show{When: func() bool {
		status := w.changes.Get()
		return status.ready && !status.bare
	}, Else: func(*core.Scope) core.Node {
		return w.changesListNotice(chrome)
	}, Then: func(cx *core.Scope) core.Node {
		label := w.tr("Edit .gitignore", "编辑 .gitignore")
		editor := toolbarAction(chrome, label, func() { w.openRepoEditor(false) }, chrome.label(label, chrome.textMuted, 11))
		editor.TestID = "edit-gitignore"
		editor.Disabled = func() bool { return !w.canOpenRepoEditor() }
		editor.Style.Height, editor.Style.Padding = core.Pt(20), core.Edges{Left: 4, Right: 4}
		return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), Background: chrome.panel}, Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
				core.Show{When: func() bool { return len(w.changes.Get().conflicts) > 0 }, Then: func(cx *core.Scope) core.Node {
					return w.changeSection(cx, chrome, "conflicts", w.tr("Conflicts", "冲突"), "", func() []workingChange {
						return w.changes.Get().conflicts
					})
				}},
				w.changeSection(cx, chrome, "unstaged", w.tr("Unstaged", "未暂存"), w.tr("No unstaged changes", "没有未暂存的变更"), func() []workingChange {
					return w.changes.Get().unstaged
				}),
				w.changeSection(cx, chrome, "staged", w.tr("Staged", "已暂存"), w.tr("No staged changes", "没有已暂存的变更"), func() []workingChange {
					return w.changes.Get().staged
				}),
			}},
			core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifyEnd, Padding: core.Edges{Right: 8}},
				Children: []core.Node{editor}},
			w.commitComposer(cx, chrome),
		}}
	}}
}

func (w *workbench) changesListNotice(chrome palette) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		status := w.changes.Get()
		switch {
		case status.bare:
			return notice(chrome, w.tr("This repository has no working copy.", "此仓库没有工作副本。"), "", nil)
		case status.err != "":
			return notice(chrome, w.tr("Failed to load working copy", "无法读取工作副本")+"\n"+status.err,
				w.tr("Retry", "重试"), w.loadChanges)
		default:
			if failure := w.historyError.Get(); failure != "" && w.core == nil {
				return notice(chrome, failure, w.tr("Retry", "重试"), func() { w.queueRefresh(refreshManual) })
			}
			return notice(chrome, w.tr("Loading…", "加载中…"), "", nil)
		}
	})
}

func (w *workbench) changeSection(cx *core.Scope, chrome palette, id, title, empty string, items func() []workingChange) core.Node {
	state := w.changeLists[id]
	selection, focused := state.selection, state.focused
	listRef := new(core.Ref)
	refs := make(map[string]*core.Ref)
	var pendingFocus string
	commands := id != "conflicts"
	selectRow := func(item workingChange, mods core.Modifiers) {
		selection.Set(selectWorkingFiles(items(), selection.Get(), item.key.path, mods))
		w.selectChange(item)
	}
	focusTarget := func() {
		if pendingFocus != "" {
			if ref := refs[pendingFocus]; ref != nil {
				pendingFocus = ""
				ref.Focus()
				ref.Reveal(core.RevealOptions{})
			}
		}
	}
	tabPath := core.NewMemo(cx, func() string {
		rows := items()
		if len(rows) == 0 {
			return ""
		}
		if path := focused.Get(); slices.ContainsFunc(rows, func(c workingChange) bool { return c.key.path == path }) {
			return path
		}
		return rows[0].key.path
	})
	list := core.List{TestID: "changes-" + id + "-list", Ref: listRef,
		Count: func() int { return len(items()) }, Key: func(i int) any { return items()[i].key }, EstimatedSize: 32,
		ContentOffset: state.offset, OnScroll: func(event core.ScrollEvent) {
			if !event.Stale {
				state.offset = event.Offset
			}
		},
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
		Empty: core.Text{Content: empty, Style: core.Style{FontSize: chrome.uiSize(12), FontStyle: core.FontStyleItalic, Color: chrome.textMuted, Padding: core.All(8)}},
		Row: func(_ *core.Scope, row core.ListRow) core.Node {
			return core.Dynamic(func(rowScope *core.Scope) core.Node {
				index := row.Index()
				rows := items()
				if index < 0 || index >= len(rows) {
					return core.View{}
				}
				item := rows[index]
				path := item.key.path
				ref := new(core.Ref)
				refs[path] = ref
				rowScope.Cleanup(func() {
					if refs[path] == ref {
						delete(refs, path)
					}
				})
				if pendingFocus == path {
					rowScope.Post(focusTarget)
				}
				node := w.changeRow(chrome, title, item,
					func() bool { return selection.Get().paths[path] },
					func(e core.PressEvent) {
						focused.Set(path)
						rowScope.Post(ref.Focus)
						selectRow(item, e.Modifiers)
					},
					commands)
				node.Ref = ref
				node.Focusable = func() bool { return tabPath.Get() == path }
				node.OnFocus = func() { focused.Set(path) }
				rowMenu := func(position core.Point) {
					target := discardableFiles([]workingChange{item})
					w.cx.PopupMenu([]core.MenuItem{
						{Label: w.tr("Discard changes…", "丢弃变更…"), Disabled: len(target) == 0 || w.currentChangesWrite().pending,
							OnSelect: w.discardMenuAction(target)},
						{Label: w.tr("Add to .gitignore…", "添加到 .gitignore…"),
							Disabled: item.kind() != '?' || len(gitcli.GitignoreOptions(path)) == 0 || !w.canOpenRepoEditor(),
							OnSelect: w.scopedRefAction(func() {
								if item.kind() == '?' {
									w.openIgnoreForm(path)
								}
							})},
					}, position, nil)
				}
				if id == "unstaged" {
					node.OnContextMenu = func(e core.ContextMenuEvent) { rowMenu(e.PagePosition) }
				}
				node.OnKeyDown = func(e core.KeyEvent) bool {
					if id == "unstaged" && ((e.Key == "f10" && e.Modifiers.Shift) || e.Key == "contextmenu") {
						if bounds, ok := ref.Bounds(); ok {
							pos := bounds.Origin
							pos.Y += bounds.Size.Height
							rowMenu(pos)
						}
						return true
					}
					rows := items()
					to := row.Index()
					switch e.Key {
					case "a":
						if e.Modifiers.Shift || (!e.Modifiers.Control && !e.Modifiers.Platform) {
							return false
						}
						paths := make(map[string]bool, len(rows))
						for _, row := range rows {
							paths[row.key.path] = true
						}
						selection.Set(changeSelection{paths: paths, anchor: selection.Get().anchor})
						return true
					case "space":
						mods := e.Modifiers
						mods.Control = true
						selectRow(item, mods)
						return true
					case "up":
						to = max(0, to-1)
					case "down":
						to = min(len(rows)-1, to+1)
					case "home":
						to = 0
					case "end":
						to = len(rows) - 1
					default:
						return false
					}
					if to < 0 || to >= len(rows) {
						return true
					}
					target := rows[to]
					focused.Set(target.key.path)
					pendingFocus = target.key.path
					if refs[pendingFocus] == nil {
						listRef.ScrollToIndexAt(to, 0.5, 0, false)
					}
					cx.Post(focusTarget)
					if !e.Modifiers.Control && !e.Modifiers.Platform {
						selectRow(target, e.Modifiers)
					}
					return true
				}
				return node
			})
		},
	}
	name := core.Text{ContentFunc: func() string { return fmt.Sprintf("%s (%d)", title, len(items())) },
		MaxLines: 1, Style: core.Style{FontSize: chrome.uiSize(12), FontWeight: 600, Color: chrome.muted,
			FlexGrow: 1, Padding: core.Edges{Left: 4}}}
	header := []core.Node{name}
	var trailing core.Node
	if commands {
		name.Style.FlexShrink = 0
		actionLabel, allLabel := w.tr("Stage", "暂存"), w.tr("Stage All", "全部暂存")
		if id == "staged" {
			actionLabel, allLabel = w.tr("Unstage", "取消暂存"), w.tr("Unstage All", "全部取消暂存")
		}
		chosen := core.NewMemoFunc(cx, func() []workingChange {
			selected := selection.Get().paths
			var files []workingChange
			for _, file := range items() {
				if selected[file.key.path] {
					files = append(files, file)
				}
			}
			return files
		}, slices.Equal[[]workingChange])
		stageButton := func(label string, files func() []workingChange, primary bool) core.Node {
			color := chrome.text
			if primary {
				color = core.RGB(0xffffff)
			}
			caption := chrome.label(label, color, 12)
			caption.Style.FontWeight = 500
			button := toolbarAction(chrome, label, func() {
				if w.writeStageSelection(files()) {
					selection.Set(changeSelection{})
				}
			}, caption)
			button.Disabled = func() bool {
				return w.currentChangesWrite().pending || !slices.ContainsFunc(files(), workingChange.canWriteIndex)
			}
			button.Style.FlexShrink = 0
			button.Style.CornerRadius = 6
			button.Style.Disabled = &core.Style{Opacity: core.Alpha(.4)}
			if primary {
				button.Style.Background = chrome.accent
				button.Style.Hover = &core.Style{Background: chrome.accent, Opacity: core.Alpha(.88)}
			} else {
				button.Style.BorderWidth, button.Style.BorderColor = 1, chrome.border
			}
			return button
		}
		header = append(header, stageButton(allLabel, items, true),
			core.Show{When: func() bool { return len(chosen.Get()) != 0 },
				Then: func(*core.Scope) core.Node { return stageButton(actionLabel, chosen.Get, false) }})
		if id == "unstaged" {
			label := w.tr("More file actions", "更多文件操作")
			more := toolbarAction(chrome, label, func() {},
				chrome.label(w.tr("More", "更多"), chrome.text, 12),
				core.Icon{SVG: lucide.ChevronDown(), Style: core.Style{FontSize: 12, Color: chrome.text}})
			more.TestID = "unstaged-more-actions"
			more.Disabled = func() bool { return len(items()) == 0 || w.currentChangesWrite().pending }
			more.OnPress = func(event core.PressEvent) {
				all := discardableFiles(items())
				selected := discardableFiles(chosen.Get())
				w.cx.PopupMenu([]core.MenuItem{
					{Label: w.tr("Discard All", "丢弃全部"), Disabled: len(all) == 0, OnSelect: w.discardMenuAction(all)},
					{Label: w.tr("Discard", "丢弃"), Disabled: len(selected) == 0, OnSelect: w.discardMenuAction(selected)},
				}, event.PagePosition, nil)
			}
			trailing = core.Show{When: func() bool { return len(items()) != 0 }, Then: func(*core.Scope) core.Node {
				return core.View{Style: core.Style{Padding: core.Edges{Right: 8}}, Children: []core.Node{more}}
			}}
		}
	}
	viewportWidth := core.NewSignal(cx, core.Points(0))
	heading := bar(chrome, 36, chrome.panel, header...)
	heading.Style.Gap, heading.Style.Borders = 4, core.Edges{}
	heading.StyleFunc = func() core.Style {
		style := core.Style{MinWidth: core.Pt(viewportWidth.Get())}
		if id == "unstaged" && len(items()) != 0 {
			style.Padding = core.Edges{Left: 8, Right: 4}
		}
		return style
	}
	headerRow := []core.Node{core.Scroll{Horizontal: true, Style: core.Style{
		Height: core.Pt(36), FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0)},
		OnLayout: func(bounds core.Bounds) { viewportWidth.Set(bounds.Size.Width) }, Children: []core.Node{heading}}}
	if trailing != nil {
		headerRow = append(headerRow, trailing)
	}
	return core.View{TestID: "changes-" + id, Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinHeight: core.Pt(0),
		Borders: core.Edges{Bottom: 1}, BorderColor: chrome.border},
		Role: core.RoleListbox, Label: title,
		Children: []core.Node{core.View{TestID: "changes-" + id + "-heading", Style: core.Style{
			Direction: core.Row, AlignItems: core.AlignCenter, Height: core.Pt(36), MinHeight: core.Pt(36), Background: chrome.panel},
			Children: headerRow}, list}}
}

func (w *workbench) changeRow(chrome palette, section string, item workingChange, selected func() bool, selectRow func(core.PressEvent), commands bool) core.View {
	name := item.key.path
	if item.move.Source.Path != "" {
		name = item.pathLabel()
	}
	statusLetter, color := "M", chrome.warning
	switch item.kind() {
	case 'A':
		statusLetter, color = "A", core.RGB(0x17c964)
	case 'D':
		statusLetter, color = "D", chrome.danger
	case 'R', 'C':
		statusLetter, color = string(item.kind()), chrome.accent
	case '?':
		statusLetter, color = "?", chrome.textMuted
	case 'u':
		statusLetter, color = "!", chrome.warning
	}
	letter := chrome.label(statusLetter, color, 12)
	letter.Style.FontFamily, letter.Style.FontWeight = chrome.fonts.code, 700
	if !item.key.staged {
		letter.Style.Opacity = core.Alpha(.8)
	}
	var status core.Node = letter
	if commands {
		label := w.tr("Stage ", "暂存 ") + item.key.path
		if item.key.staged {
			label = w.tr("Unstage ", "取消暂存 ") + item.key.path
		}
		button := toolbarAction(chrome, label, func() { w.writeStageSelection([]workingChange{item}) }, status)
		button.Style.Height, button.Style.Width, button.Style.Padding = core.Pt(20), core.Pt(20), core.Edges{}
		button.Disabled = func() bool { return !item.canWriteIndex() || w.currentChangesWrite().pending }
		button.Focusable = func() bool { return false }
		tip := label
		if !item.canWriteIndex() {
			tip = w.tr("Stage these files inside the submodule", "请在子模块内暂存这些文件")
		}
		button.Tooltip = chrome.label(tip, chrome.text, 12)
		status = button
	}
	return core.View{Role: core.RoleOption, Label: section + " · " + name, Selected: selected,
		OnPress: selectRow,
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Height: core.Pt(32), Gap: 8,
			Padding: core.Edges{Left: 12, Right: 12}, Borders: core.Edges{Left: 3}, BorderColor: core.Transparent,
			Hover:        &core.Style{Background: chrome.hover},
			FocusVisible: &core.Style{OutlineWidth: 1, OutlineColor: chrome.accent, OutlineOffset: -1}},
		StyleFunc: func() core.Style {
			if selected() {
				return core.Style{Background: chrome.accent.Faded(.1), BorderColor: chrome.accent, FontWeight: 500,
					Hover: &core.Style{Background: chrome.accent.Faded(.15)}}
			}
			if w.changeFile.Get().key == item.key {
				return core.Style{BorderColor: chrome.accent}
			}
			return core.Style{}
		},
		Tooltip: chrome.label(item.pathLabel(), chrome.text, 12),
		Children: []core.Node{
			status,
			core.Text{Content: name, MaxLines: 1, Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0),
				FontSize: chrome.uiSize(12), Color: chrome.text}},
		},
	}
}

func (w *workbench) workingDiffPanel(chrome palette) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		item := w.changeFile.Get()
		state := w.changeDiff.Get()
		if blame := w.blame.Get(); item.key.path != "" && blame.path == item.key.path {
			return w.blamePanel(cx, chrome, blame)
		}
		switch {
		case item.key.path == "":
			return workbenchEmptyState(chrome, nil, w.tr("No file selected", "未选择文件"),
				w.tr("Click a file on the left to see its diff.", "点击左侧文件查看 diff。"))
		case !w.changes.Get().ready || state.loading:
			return notice(chrome, w.tr("Loading diff…", "正在加载 diff…"), "", nil)
		case item.kind() == 'u':
			label := w.tr("Mark as resolved", "标记为已解决")
			hint := w.tr("After editing the working file, stage it to record the resolution.", "编辑工作文件后，将其暂存以记录冲突解决结果。")
			if item.change.WorkTreeMode == "000000" {
				label = w.tr("Resolve by deleting file", "以删除文件解决冲突")
				hint = w.tr("The working file is missing. Stage the deletion to resolve this conflict.", "工作文件不存在。可暂存删除操作来解决此冲突。")
			}
			button := toolbarAction(chrome, label, func() {
				w.writeStageSelection([]workingChange{item})
			}, chrome.label(label, chrome.text, 12))
			button.TestID = "mark-conflict-resolved"
			button.Disabled = func() bool { return w.currentChangesWrite().pending }
			open := toolbarAction(chrome, w.tr("Open conflict editor", "打开冲突编辑器"), func() { w.openConflicts(item.key.path) },
				chrome.label(w.tr("Open conflict editor", "打开冲突编辑器"), chrome.accent, 12))
			open.TestID = "open-conflict-editor"
			return core.View{Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 6},
				Children: []core.Node{chrome.label(hint, chrome.muted, 13), open, button}}
		case state.err != "":
			return notice(chrome, state.err, w.tr("Retry", "重试"), func() {
				w.loadChangeDiff(item, state.expanded, false)
			})
		}
		chip := w.tr("Unstaged", "未暂存")
		if item.key.staged {
			chip = w.tr("Staged", "已暂存")
		}
		return w.fileDiffView(chrome, state.result, fileDiffOptions{chip: chip, title: item.pathLabel(),
			collapsed: w.changeCollapsed, expanded: state.expanded,
			viewport:   &w.workingDiffViewport,
			editorHint: !item.key.staged && item.kind() != 'D',
			expand:     func() { w.loadChangeDiff(item, true, false) },
			blame:      func(file fileDiff) { w.loadBlame(file.Path, false) },
		})
	})
}
