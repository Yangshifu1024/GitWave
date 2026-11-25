package main

import (
	"fmt"
	"path/filepath"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/base/editor"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

func (w *workbench) conflictsDialog(chrome palette) core.Node {
	visible := func() bool { return w.conflictPanel.Get() != nil }
	return core.Show{When: visible, Then: func(cx *core.Scope) core.Node {
		title := func() string {
			changes := w.changes.Get()
			label := w.tr("Unresolved conflicts (%d)", "未解决的冲突（%d）")
			if changes.status.Commit.MergeHeads != "" {
				label = w.tr("Merge conflicts (%d)", "合并冲突（%d）")
			}
			return fmt.Sprintf(label, len(changes.conflicts))
		}
		close := iconAction(chrome, w.tr("Close", "关闭"), lucide.X(), w.requestCloseConflicts)
		close.TestID = "close-conflicts"
		close.Disabled = func() bool { return w.currentChangesWrite().pending }
		abort := core.Show{When: func() bool { return w.changes.Get().status.Commit.MergeHeads != "" },
			Then: func(*core.Scope) core.Node { return w.mergeAbortButton(chrome, "abort-merge-conflicts") }}
		header := bar(chrome, 46, chrome.elevated, core.Icon{SVG: lucide.TriangleAlert(), Style: core.Style{FontSize: 16, Color: chrome.warning}},
			core.Text{ContentFunc: title, Role: core.RoleHeader, Style: core.Style{FontSize: chrome.uiSize(14), FlexGrow: 1, Color: chrome.text}}, abort, close)
		selected := core.NewMemo(cx, func() *gitcli.Conflict {
			if draft := w.conflictDraft.Get(); draft != nil {
				return draft.file
			}
			return nil
		})
		return ui.Dialog{TestID: "conflicts-dialog", Bare: true, LabelFunc: title, Open: visible, NoCloseOnBackdropPress: true,
			Style: core.Style{Width: core.Pct(95), MaxWidth: core.Pt(1100), Height: core.Pct(90), MaxHeight: core.Pt(720), Background: chrome.panel},
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open && !w.conflictDiscard.Get() && !w.conflictMarkers.Get() {
					w.requestCloseConflicts()
				}
			}, Children: []core.Node{
				header,
				core.Show{When: func() bool { return w.conflictError.Get() != "" }, Then: func(*core.Scope) core.Node {
					return core.Text{TestID: "conflict-error", Role: core.RoleAlert, ContentFunc: w.conflictError.Get,
						Style: core.Style{Padding: core.All(8), FontSize: chrome.uiSize(12), Color: chrome.danger, Background: chrome.panel}}
				}},
				core.View{Style: core.Style{Direction: core.Row, FlexGrow: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
					w.conflictsList(chrome),
					core.Dynamic(func(cx *core.Scope) core.Node {
						if file := selected.Get(); file != nil {
							return w.conflictFileEditor(cx, chrome, file)
						}
						if w.conflictLoading.Get() {
							return notice(chrome, w.tr("Loading conflict…", "正在加载冲突…"), "", nil)
						}
						return notice(chrome, w.tr("Select a conflicted file", "请选择一个冲突文件"), "", nil)
					}),
				}},
			}}
	}}
}

func (w *workbench) conflictsList(chrome palette) core.Node {
	files := w.conflictPaths
	return core.View{Role: core.RoleListbox, Label: w.tr("Conflicted files", "冲突文件列表"),
		Style: core.Style{Width: core.Pt(250), FlexShrink: 0, Borders: core.Edges{Right: 1}, BorderColor: chrome.border, Padding: core.All(4)},
		Children: []core.Node{core.List{TestID: "conflicts-list", Count: func() int { return len(files()) },
			Key: func(i int) any { return files()[i] }, EstimatedSize: 29,
			Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(0)},
			Row: func(_ *core.Scope, row core.ListRow) core.Node {
				return core.Dynamic(func(*core.Scope) core.Node {
					items, i := files(), row.Index()
					if i < 0 || i >= len(items) {
						return core.View{}
					}
					path := items[i]
					item := toolbarAction(chrome, path, func() { w.selectConflict(path) }, chrome.label(filepath.Base(path), chrome.text, 12))
					item.TestID, item.Role = "conflict-row", core.RoleOption
					item.Style.JustifyContent = core.JustifyStart
					item.Style.Selected = &core.Style{Background: chrome.selected}
					item.Selected = func() bool { return w.conflictFile.Get() == path }
					item.Disabled = func() bool { return w.currentChangesWrite().pending }
					item.Tooltip = chrome.label(path, chrome.text, 12)
					return item
				})
			},
		}}}
}

func (w *workbench) conflictFileEditor(cx *core.Scope, chrome palette, file *gitcli.Conflict) core.Node {
	text := func() string {
		if draft := w.conflictDraft.Get(); draft != nil {
			return draft.text
		}
		return ""
	}
	regions := core.NewMemoFunc(cx, func() []gitcli.ConflictRegion { return gitcli.ConflictRegions(text(), file.MarkerSize) }, nil)
	current := func() int { return max(0, min(w.conflictHunk.Get(), len(regions.Get())-1)) }
	ref := new(editor.Ref)
	jump := func(direction int) {
		items := regions.Get()
		index := current() + direction
		if index < 0 || index >= len(items) {
			return
		}
		region := items[index]
		w.conflictHunk.Set(index)
		ref.Focus()
		ref.SetSelections([]editor.Selection{{Anchor: editor.Position{Line: region.Start}, Head: editor.Position{Line: region.End, Column: 7}}})
		ref.ScrollToLine(region.Start)
	}
	previous := iconAction(chrome, w.tr("Previous conflict", "上一个冲突"), lucide.ChevronUp(), func() { jump(-1) })
	previous.TestID = "conflict-previous"
	previous.Disabled = func() bool { return current() == 0 || w.currentChangesWrite().pending }
	next := iconAction(chrome, w.tr("Next conflict", "下一个冲突"), lucide.ChevronDown(), func() { jump(1) })
	next.TestID = "conflict-next"
	next.Disabled = func() bool { return current()+1 >= len(regions.Get()) || w.currentChangesWrite().pending }
	path := chrome.label(file.Path, chrome.muted, 12)
	path.Style.FlexGrow, path.Style.FlexShrink = 1, 1
	path.Style.FontFamily, path.Style.FontSize = chrome.fonts.code, chrome.codeSize(12)
	currentStages := func() bool { return file.MatchesStatus(w.changes.Get().status) }
	markerCount := core.Text{TestID: "conflict-count", Role: core.RoleStatus, Style: core.Style{Color: chrome.muted, FontSize: chrome.uiSize(12)},
		ContentFunc: func() string {
			if count := len(regions.Get()); count != 0 {
				if count == 1 {
					return w.tr("1 conflict", "1 个冲突")
				}
				return fmt.Sprintf(w.tr("%d conflicts", "%d 个冲突"), count)
			}
			return w.tr("No conflicts", "无冲突")
		}}
	header := bar(chrome, 36, chrome.elevated, path, previous, markerCount, next)
	choose := func(label, id string, part gitcli.ConflictPart, choice gitcli.ConflictChoice) core.Node {
		button := toolbarAction(chrome, label, func() {
			w.conflictHunk.Set(0)
			w.updateConflict(file, choice, part.Text)
		}, chrome.label(label, chrome.text, 12))
		button.TestID = id
		button.Disabled = func() bool {
			return w.currentChangesWrite().pending || !currentStages() || part.Kind == "submodule"
		}
		button.Selected = func() bool {
			draft := w.conflictDraft.Get()
			return draft != nil && draft.choice == choice
		}
		button.Style.Selected = &core.Style{Background: chrome.selected}
		return button
	}
	confirmLabel := func() string {
		if draft := w.conflictDraft.Get(); draft != nil &&
			((draft.choice == gitcli.ConflictOurs && file.Ours.Kind == "missing") || (draft.choice == gitcli.ConflictTheirs && file.Theirs.Kind == "missing")) {
			return w.tr("Resolve by deleting file", "以删除文件解决冲突")
		}
		return w.tr("Mark resolved", "标记为已解决")
	}
	confirm := toolbarAction(chrome, "", func() {
		ref.Blur()
		w.saveConflict()
	}, core.Text{ContentFunc: confirmLabel, Style: core.Style{FontSize: chrome.uiSize(12), Color: core.RGB(0xffffff)}})
	confirm.LabelFunc, confirm.TestID = confirmLabel, "save-conflict"
	confirm.Style.Background = chrome.accent
	confirm.Style.Disabled = &core.Style{Opacity: core.Alpha(.45)}
	confirm.Disabled = func() bool {
		draft := w.conflictDraft.Get()
		return w.currentChangesWrite().pending || draft == nil || !currentStages() ||
			(draft.choice == gitcli.ConflictText && !file.CanEdit())
	}
	explain := core.Show{When: currentStages, Then: func(*core.Scope) core.Node {
		return w.aiAdviceButton(chrome, w.conflictAI, "conflict-ai-explain",
			w.tr("Explain with AI", "用 AI 解释"), func() { w.explainConflictAI(file) })
	}}
	actions := bar(chrome, 36, chrome.panel,
		choose(w.tr("Use ours", "使用我方"), "conflict-ours", file.Ours, gitcli.ConflictOurs),
		choose(w.tr("Use theirs", "使用对方"), "conflict-theirs", file.Theirs, gitcli.ConflictTheirs), explain, confirm,
		core.Show{When: func() bool { return !currentStages() && file.CanEdit() }, Then: func(*core.Scope) core.Node {
			return action(chrome, w.tr("Copy unsaved edits", "复制尚未保存的编辑"), func() { w.cx.SetClipboard(text()) })
		}})
	sides := core.View{Style: core.Style{Direction: core.Row, Height: core.Pt(136), Borders: core.Edges{Bottom: 1}, BorderColor: chrome.border},
		Children: []core.Node{
			w.conflictSide(chrome, w.tr("Ours", "我方"), file.Ours),
			w.conflictSide(chrome, w.tr("Base", "基准"), file.Base),
			w.conflictSide(chrome, w.tr("Theirs", "对方"), file.Theirs),
		}}
	var edit core.Node = notice(chrome, w.tr("Choose a side to resolve this file.", "请选择一方来解决此文件的冲突。"), "", nil)
	if file.CanEdit() {
		readOnly := core.NewMemo(cx, func() bool { return w.currentChangesWrite().pending || !currentStages() })
		edit = core.Dynamic(func(*core.Scope) core.Node {
			return editor.Editor{TestID: "conflict-editor", Label: w.tr("Resolved file content", "解决后的文件内容"), Ref: ref,
				Value: text, ReadOnly: readOnly.Get(), OnChange: func(value string) { w.updateConflict(file, gitcli.ConflictText, value) },
				NoHighlighting: true, NoLineNumbers: true, NoFolding: true, NoAutoClose: true, NoAutoIndent: true, NoActiveLine: true,
				ScrollBeyondLastLine: -1, FontFamily: chrome.fonts.code, FontSize: chrome.codeSize(12), LineHeight: chrome.codeSize(20),
				Style:  core.Style{FlexGrow: 1, MinHeight: core.Pt(0), Background: chrome.panel},
				Styles: editor.Styles{Line: core.Style{Color: chrome.text}, Caret: core.Style{Background: chrome.text}, Selection: core.Style{Background: chrome.selected}},
				Decorations: func() []editor.Decoration {
					var result []editor.Decoration
					for _, region := range regions.Get() {
						result = append(result, editor.Decoration{Range: editor.Range{
							Start: editor.Position{Line: region.Start}, End: editor.Position{Line: region.End, Column: 7}},
							LineStyle: core.Style{Background: chrome.remove}})
					}
					return result
				}}
		})
	}
	advice := core.Show{When: func() bool { return w.conflictAI.result.Get() != (aiAdviceResult{}) }, Then: func(*core.Scope) core.Node {
		return core.Scroll{TestID: "conflict-ai-scroll", Style: core.Style{MaxHeight: core.Pt(110), FlexShrink: 1}, Children: []core.Node{
			aiAdviceStatus(chrome, w.conflictAI, "conflict-ai-result", true),
		}}
	}}
	return core.View{Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(0), MinWidth: core.Pt(0)},
		Children: []core.Node{header, actions, advice, sides, edit}}
}

func (w *workbench) conflictSide(chrome palette, title string, part gitcli.ConflictPart) core.Node {
	caption := chrome.label(title, chrome.muted, 11)
	caption.Style.Padding = core.All(7)
	var body core.Node
	if part.Kind == "text" {
		body = editor.Editor{Label: title, DefaultValue: part.Text, ReadOnly: true,
			NoLineNumbers: true, NoHighlighting: true, NoFolding: true, NoActiveLine: true, ScrollBeyondLastLine: -1,
			FontFamily: chrome.fonts.code, FontSize: chrome.codeSize(11), LineHeight: chrome.codeSize(15),
			Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(0)}, Styles: editor.Styles{Line: core.Style{Color: chrome.muted}}}
	} else {
		label := part.Text
		switch part.Kind {
		case "missing":
			label = w.tr("(missing)", "（缺失）")
		case "binary":
			label = w.tr("Binary file", "二进制文件")
		case "large":
			label = w.tr("File is too large to preview", "文件过大，无法预览")
		case "symlink":
			label = w.tr("Symbolic link", "符号链接") + " " + part.Text
		}
		body = core.Text{Content: label, Style: core.Style{Padding: core.All(7), FontSize: chrome.uiSize(11), Color: chrome.muted}}
	}
	return core.View{Style: core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), Borders: core.Edges{Right: 1}, BorderColor: chrome.border},
		Children: []core.Node{caption, body}}
}

func (w *workbench) discardConflictDialog(chrome palette) core.Node {
	keep := func() { w.conflictDiscard.Set(false) }
	return ui.Dialog{TestID: "discard-conflict-edits", Role: core.RoleAlertDialog, Open: w.conflictDiscard.Get,
		Title:       w.tr("Discard unresolved edits?", "丢弃未解决的编辑？"),
		Description: w.tr("Close the editor and lose unsaved conflict edits?", "关闭编辑器并丢弃尚未保存的冲突修改？"),
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				keep()
			}
		}, Footer: []core.Node{
			action(chrome, w.tr("Keep editing", "继续编辑"), keep),
			action(chrome, w.tr("Discard edits", "丢弃编辑"), w.closeConflicts),
		}}
}

func (w *workbench) conflictMarkersDialog(chrome palette) core.Node {
	cancel := func() { w.conflictMarkers.Set(false) }
	return ui.Dialog{TestID: "confirm-conflict-markers", Role: core.RoleAlertDialog, Open: w.conflictMarkers.Get,
		Title:       w.tr("Stage text with conflict markers?", "暂存仍含冲突标记的文本？"),
		Description: w.tr("The edited text still contains conflict markers. Confirm to stage it as written.", "编辑后的文本仍有冲突标记。确认后会按原样暂存。"),
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				cancel()
			}
		}, Footer: []core.Node{
			action(chrome, w.tr("Keep editing", "继续编辑"), cancel),
			action(chrome, w.tr("Stage as written", "按原样暂存"), w.applyConflict),
		}}
}
