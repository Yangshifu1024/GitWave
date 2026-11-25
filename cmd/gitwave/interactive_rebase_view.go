package main

import (
	"fmt"
	"maps"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

var interactiveRebaseActions = []ui.Option[gitcli.RebaseAction]{
	{Value: gitcli.RebasePick, Label: "pick"},
	{Value: gitcli.RebaseReword, Label: "reword"},
	{Value: gitcli.RebaseEdit, Label: "edit"},
	{Value: gitcli.RebaseSquash, Label: "squash"},
	{Value: gitcli.RebaseFixup, Label: "fixup"},
	{Value: gitcli.RebaseDrop, Label: "drop"},
}

func (w *workbench) interactiveRebaseControls(chrome palette) core.Node {
	return core.Show{When: func() bool { return w.changes.Get().status.Commit.InteractiveRebase != "" },
		Then: func(*core.Scope) core.Node {
			control := func(label, id string, abort bool) core.View {
				button := toolbarAction(chrome, label, func() { w.finishInteractivePause(abort) }, chrome.label(label, chrome.text, 11))
				button.TestID, button.Disabled = id, func() bool { return !w.canSync() }
				return button
			}
			label := w.tr("Continue rebase", "继续变基")
			discard := w.tr("Discard pause", "放弃暂停")
			return bar(chrome, 34, chrome.elevated, control(label, "interactive-rebase-continue", false),
				control(discard, "interactive-rebase-abort", true))
		}}
}

func (w *workbench) interactiveRebaseDialog(chrome palette) core.Node {
	busy := func() bool { return w.currentChangesWrite().pending }
	close := func() {
		if !busy() {
			w.interactiveForm.Set(nil)
		}
	}
	cancel := toolbarAction(chrome, w.tr("Cancel", "取消"), close, chrome.label(w.tr("Cancel", "取消"), chrome.text, 12))
	cancel.TestID, cancel.Disabled = "interactive-rebase-cancel", busy
	label := w.tr("Start rebase", "开始变基")
	confirm := toolbarAction(chrome, label, w.submitInteractiveRebase, chrome.label(label, core.RGB(0xffffff), 12))
	confirm.TestID = "interactive-rebase-start"
	confirm.Disabled = func() bool { return !w.canStartInteractiveRebase() }
	confirm.Style.Background = chrome.accent
	confirm.Style.Disabled = &core.Style{Opacity: core.Alpha(.45)}
	return ui.Dialog{TestID: "interactive-rebase-dialog",
		Open: func() bool { return w.interactiveForm.Get() != nil },
		TitleFunc: func() string {
			if form := w.interactiveForm.Get(); form != nil {
				return fmt.Sprintf(w.tr("Interactive rebase onto %s", "交互式变基到 %s"), form.upstream)
			}
			return ""
		},
		Style: core.Style{Width: core.Pct(94), MaxWidth: core.Pt(720)},
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				close()
			}
		},
		Children: []core.Node{
			chrome.label(w.tr("Drag to reorder. Actions: pick / reword / edit / squash / fixup / drop.",
				"拖动可调整顺序。操作：pick / reword / edit / squash / fixup / drop。"), chrome.muted, 12),
			core.Show{When: func() bool { return len(w.changes.Get().status.Changes) != 0 },
				Then: func(*core.Scope) core.Node {
					return core.Text{Role: core.RoleAlert, ContentFunc: func() string {
						return fmt.Sprintf(w.tr("Working copy has %d uncommitted file(s). Commit or stash before rebasing.",
							"工作副本中有 %d 个未提交文件。请先提交或贮藏，再变基。"), len(w.changes.Get().status.Changes))
					}, Style: core.Style{Color: chrome.danger, FontSize: chrome.uiSize(12)}}
				}},
			core.Show{When: func() bool { return w.interactiveError.Get() != "" }, Then: func(*core.Scope) core.Node {
				return core.Text{TestID: "interactive-rebase-error", ContentFunc: w.interactiveError.Get, Role: core.RoleAlert,
					Style: core.Style{Color: chrome.danger, FontSize: chrome.uiSize(12)}}
			}},
			core.Dynamic(func(cx *core.Scope) core.Node {
				form := w.interactiveForm.Get()
				if form == nil {
					return core.View{}
				}
				if form.plan == nil {
					if w.interactiveError.Get() != "" {
						return core.View{}
					}
					return chrome.label(w.tr("Loading commits…", "正在加载提交…"), chrome.muted, 12)
				}
				if len(form.plan.Todos()) == 0 {
					return chrome.label(fmt.Sprintf(w.tr("Already up to date with %s.", "已与 %s 保持一致。"), form.upstream), chrome.muted, 12)
				}
				return w.interactiveRebaseList(cx, chrome, busy)
			}),
		},
		Footer: []core.Node{cancel, confirm},
	}
}

func (w *workbench) interactiveRebaseList(cx *core.Scope, chrome palette, busy func() bool) core.Node {
	todos := w.interactiveTodos.Get
	rows := make(map[int]*core.Ref)
	grips := make(map[int]*core.Ref)
	active := core.NewSignal(cx, "")
	return core.List{TestID: "interactive-rebase-todos",
		Style: core.Style{Height: core.Pt(360), MaxHeight: core.Pct(60), Borders: core.All(1), BorderColor: chrome.border},
		Count: func() int { return len(todos()) }, EstimatedSize: 42,
		Row: func(scope *core.Scope, row core.ListRow) core.Node {
			position := row.Index()
			ref := new(core.Ref)
			grip := new(core.Ref)
			rows[position] = ref
			grips[position] = grip
			scope.Cleanup(func() {
				if rows[position] == ref {
					delete(rows, position)
					delete(grips, position)
				}
			})
			return core.Dynamic(func(*core.Scope) core.Node {
				items, index := todos(), row.Index()
				if index < 0 || index >= len(items) {
					return core.View{}
				}
				item, oid := items[index], items[index].OID
				return w.interactiveTodoRow(chrome, item, ref, grip, busy, func() bool { return active.Get() == oid },
					func(e core.PanEvent) {
						if busy() {
							return
						}
						switch e.Phase {
						case core.GestureBegan:
							active.Set(oid)
						case core.GestureCancelled:
							active.Set("")
						case core.GestureEnded:
							if active.Get() != oid {
								return
							}
							active.Set("")
							if bounds, ok := ref.Bounds(); ok {
								y := bounds.Origin.Y + bounds.Size.Height/2 + e.Translation.Y
								w.moveInteractiveTodo(oid, nearestInteractiveRow(rows, y, index))
							}
						}
					}, func(offset int) {
						target := row.Index() + offset
						if target < 0 || target >= len(todos()) || busy() {
							return
						}
						w.moveInteractiveTodo(oid, target)
						cx.Post(func() {
							if next := grips[target]; next != nil {
								next.Focus()
							}
						})
					})
			})
		}}
}

func nearestInteractiveRow(rows map[int]*core.Ref, y core.Points, index int) int {
	best := core.Points(1 << 24)
	for i, ref := range rows {
		if bounds, ok := ref.Bounds(); ok {
			gap := y - bounds.Origin.Y - bounds.Size.Height/2
			gap = max(gap, -gap)
			if gap < best || gap == best && i < index {
				index, best = i, gap
			}
		}
	}
	return index
}

func (w *workbench) interactiveTodoRow(chrome palette, todo gitcli.RebaseTodo, ref, gripRef *core.Ref, busy, dragging func() bool,
	pan func(core.PanEvent), move func(int)) core.Node {
	label := w.tr("Reorder commit", "调整提交顺序")
	grip := core.View{TestID: "interactive-rebase-grip", Label: label, Ref: gripRef, Role: core.RoleButton, Focusable: func() bool { return !busy() },
		Style:    core.Style{Width: core.Pt(25), Height: core.Pt(28), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		Children: []core.Node{core.Icon{SVG: lucide.GripVertical(), Style: core.Style{FontSize: 15, Color: chrome.muted}}},
		OnPan:    pan, PanActivation: core.PanActivation{ActiveOffsetY: 5, FailOffsetX: 18},
		OnKeyDown: func(event core.KeyEvent) bool {
			if event.Key != "up" && event.Key != "down" {
				return false
			}
			step := 1
			if event.Key == "up" {
				step = -1
			}
			move(step)
			return true
		}}
	selectAction := ui.Select[gitcli.RebaseAction]{TestID: "interactive-rebase-action", Label: w.tr("Rebase action", "变基操作"),
		Options: interactiveRebaseActions, Disabled: busy, Value: func() gitcli.RebaseAction { return todo.Action },
		OnChange: func(value gitcli.RebaseAction) {
			w.updateInteractiveTodo(todo.OID, func(todo *gitcli.RebaseTodo) { todo.Action = value })
		}, Style: core.Style{Width: core.Pt(100), Height: core.Pt(28)}}
	sha := chrome.label(shortSHA(todo.OID), chrome.muted, 11)
	sha.Style.FontFamily = "monospace"
	summary := chrome.label(todo.Summary, chrome.text, 12)
	summary.Style.FlexGrow, summary.Style.FlexShrink, summary.Style.MinWidth = 1, 1, core.Pt(0)
	row := core.View{TestID: "interactive-rebase-todo", Label: todo.Summary, Ref: ref,
		Style: core.Style{Padding: core.All(4), Gap: 3, Borders: core.Edges{Bottom: 1}, BorderColor: chrome.border},
		StyleFunc: func() core.Style {
			if dragging() {
				return core.Style{Background: chrome.selected}
			}
			return core.Style{}
		},
		Children: []core.Node{core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6},
			Children: []core.Node{grip, core.View{Style: core.Style{Width: core.Pt(110)}, Children: []core.Node{selectAction}}, sha, summary}}}}
	if todo.Action == gitcli.RebaseReword || todo.Action == gitcli.RebaseSquash {
		label := w.tr("New commit message", "新的提交信息")
		if todo.Action == gitcli.RebaseSquash {
			label = w.tr("Combined message (optional)", "合并后的提交信息（可选）")
		}
		row.Children = append(row.Children, core.Input{TestID: "interactive-rebase-message", Label: label, Placeholder: label,
			Multiline: true, NoAutoCorrect: true, Disabled: busy, Value: func() string { return w.interactiveMessages.Get()[todo.OID] },
			OnChange: func(message string) {
				current := w.interactiveMessages.Get()
				messages := make(map[string]string, len(current)+1)
				maps.Copy(messages, current)
				messages[todo.OID] = message
				w.interactiveMessages.Set(messages)
			}, Style: core.Style{Height: core.Pt(54), Padding: core.All(5), BorderWidth: 1, BorderColor: chrome.border}})
	}
	return row
}
