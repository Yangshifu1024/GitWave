package main

import (
	"fmt"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

func (w *workbench) worktreeSidebar(chrome palette) core.Node {
	return core.Show{When: func() bool { return w.path.Get() != "" }, Then: func(cx *core.Scope) core.Node {
		label := w.tr("Worktrees", "工作树")
		hasWorktrees := core.NewMemo(cx, func() bool { return len(w.worktreeList.Get()) > 0 })
		return core.View{Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			if !hasWorktrees.Get() {
				return sidebarHeading(chrome, "worktree-toggle", label, nil, nil)
			}
			return sidebarHeading(chrome, "worktree-toggle", label, w.worktreeOpen.Get, func() {
				w.worktreeOpen.Set(!w.worktreeOpen.Get())
			})
		}),
			sidebarReadError(chrome, "worktree-list-error", w.worktreeListError.Get),
			core.Show{When: func() bool {
				return w.worktreeError.Get() != "" && w.worktreeForm.Get() == nil && w.worktreeRemove.Get() == nil
			}, Then: func(*core.Scope) core.Node { return w.worktreeErrorText(chrome) }},
			core.Show{When: func() bool { return w.worktreeOpen.Get() && hasWorktrees.Get() }, Then: func(*core.Scope) core.Node {
				const rowHeight = 37
				return core.View{Style: core.Style{FlexShrink: 1},
					StyleFunc: func() core.Style {
						return core.Style{Height: core.Pt(core.Points(min(208, rowHeight*len(w.worktreeList.Get()))))}
					},
					Children: []core.Node{core.List{TestID: "worktree-list", EstimatedSize: rowHeight,
						Style: core.Style{FlexGrow: 1, FlexShrink: 1},
						Count: func() int { return len(w.worktreeList.Get()) },
						Key:   func(i int) any { return w.worktreeList.Get()[i].Path },
						Row: func(_ *core.Scope, row core.ListRow) core.Node {
							return core.Dynamic(func(*core.Scope) core.Node {
								entries := w.worktreeList.Get()
								i := row.Index()
								if i < 0 || i >= len(entries) {
									return core.View{}
								}
								return w.worktreeRow(chrome, entries[i])
							})
						}}},
				}
			}},
		}}
	}}
}

func (w *workbench) worktreeRow(chrome palette, entry gitcli.Worktree) core.Node {
	name := entry.Name
	if entry.Branch != "" {
		name += " · " + entry.Branch
	}
	title, path := chrome.label(name, chrome.text, 11), chrome.label(entry.Path, chrome.muted, 10)
	title.MaxLines, path.MaxLines = 1, 1
	row := core.View{TestID: "worktree-row", Label: name, Style: core.Style{
		Direction: core.Row, AlignItems: core.AlignCenter, Gap: 5, Padding: core.Edges{Left: 11, Right: 5},
		MinHeight: core.Pt(37),
	}, Children: []core.Node{
		core.Icon{SVG: lucide.FolderTree(), Style: core.Style{FontSize: 14, Color: chrome.accent}},
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)},
			Tooltip: chrome.label(entry.Path, chrome.text, 12), Children: []core.Node{title, path}},
	}}
	if entry.Main {
		row.Children = append(row.Children, chrome.label(w.tr("Main", "主工作树"), chrome.accent, 10))
	} else if entry.Locked {
		row.Children = append(row.Children, core.Icon{SVG: lucide.Lock(), Label: w.tr("Locked", "已锁定"),
			Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.muted}})
	}
	if !entry.Current && !entry.Prunable {
		switchButton := iconAction(chrome, w.tr("Switch to worktree", "切换到工作树"), lucide.ArrowRightLeft(),
			func() { w.openSelectedWorktree(entry) })
		switchButton.TestID = "worktree-switch"
		switchButton.Disabled = func() bool { return !w.canManageWorktrees() }
		switchButton.Style.Padding = core.Edges{Left: 4, Right: 4}
		row.Children = append(row.Children, switchButton)
	}
	if !entry.Main && !entry.Current && !entry.Locked {
		remove := iconAction(chrome, w.tr("Remove worktree", "移除工作树"), lucide.Trash(),
			func() { w.requestWorktreeRemoval(entry) })
		remove.TestID = "worktree-remove"
		remove.Disabled = func() bool { return !w.canManageWorktrees() }
		remove.Style.Padding = core.Edges{Left: 4, Right: 4}
		row.Children = append(row.Children, remove)
	}
	return row
}

func (w *workbench) worktreeErrorText(chrome palette) core.Node {
	return core.Text{TestID: "worktree-error", ContentFunc: w.worktreeError.Get,
		Role: core.RoleAlert, Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.danger}}
}

func (w *workbench) worktreeCreateDialog(chrome palette) core.Node {
	busy := func() bool { return w.currentChangesWrite().pending }
	closeForm := func() {
		if !busy() {
			w.worktreeError.Set("")
			w.worktreeForm.Set(nil)
		}
	}
	cancelLabel, createLabel := w.tr("Cancel", "取消"), w.tr("Create", "创建")
	cancel := toolbarAction(chrome, cancelLabel, closeForm, chrome.label(cancelLabel, chrome.text, 12))
	cancel.Disabled = busy
	create := toolbarAction(chrome, createLabel, w.createWorktree, chrome.label(createLabel, chrome.accent, 12))
	create.TestID = "worktree-create-confirm"
	create.Disabled = func() bool { return !w.canCreateWorktree() }
	return ui.Dialog{TestID: "worktree-create-dialog", Title: w.tr("Create Worktree", "创建工作树"),
		Description: w.tr("Create a branch and check it out in a separate worktree.", "创建分支并在单独的工作树中检出。"),
		Open:        func() bool { return w.worktreeForm.Get() != nil },
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				closeForm()
			}
		},
		Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			form := w.worktreeForm.Get()
			if form == nil {
				return core.View{}
			}
			input := func(id, label, placeholder string, value *core.Signal[string], focus bool) core.Input {
				return core.Input{TestID: id, Label: label, Placeholder: placeholder, Value: value.Get, OnChange: value.Set,
					Disabled: busy, AutoFocus: focus, OnSubmit: func(string) { w.createWorktree() },
					Style: core.Style{Height: core.Pt(32), Padding: core.All(6), BorderWidth: 1, BorderColor: chrome.border, FlexGrow: 1}}
			}
			start, name, location := w.tr("Start from", "起始分支"), w.tr("Branch name", "分支名"), w.tr("Location", "位置")
			pick := iconAction(chrome, w.tr("Choose folder", "选择文件夹"), lucide.FolderOpen(), func() {
				w.cx.PickDirectories(core.PickDirectoriesOptions{Title: w.tr("Choose the worktree folder", "选择工作树文件夹")},
					func(dirs []core.Directory, err error) {
						if w.worktreeForm.Get() != form || busy() {
							return
						}
						if err != nil {
							w.worktreeError.Set(err.Error())
						} else if len(dirs) > 0 && dirs[0].Path != "" {
							w.worktreePath.Set(dirs[0].Path)
						}
					})
			})
			pick.Disabled = busy
			return core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
				syncSelect(chrome, "worktree-create-start", start, w.worktreeStart.Get, func() []string { return form.options }, func(value string) {
					if !busy() {
						w.worktreeStart.Set(value)
					}
				}),
				chrome.label(name, chrome.muted, 12),
				input("worktree-create-name", name, "feature/my-worktree", w.worktreeName, true),
				chrome.label(location, chrome.muted, 12),
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4},
					Children: []core.Node{input("worktree-create-path", location, w.tr("Path for the new worktree", "新工作树的路径"),
						w.worktreePath, false), pick}},
				w.worktreeErrorText(chrome),
			}}
		})},
		Footer: []core.Node{cancel, create},
	}
}

func (w *workbench) worktreeRemoveDialog(chrome palette) core.Node {
	busy := func() bool { return w.currentChangesWrite().pending }
	closePrompt := func() {
		if !busy() {
			w.worktreeRemove.Set(nil)
			w.worktreeError.Set("")
		}
	}
	cancelLabel, removeLabel := w.tr("Cancel", "取消"), w.tr("Remove", "移除")
	cancel := toolbarAction(chrome, cancelLabel, closePrompt, chrome.label(cancelLabel, chrome.text, 12))
	cancel.Disabled = busy
	remove := toolbarAction(chrome, removeLabel, w.removeSelectedWorktree, chrome.label(removeLabel, chrome.danger, 12))
	remove.TestID = "worktree-remove-confirm"
	remove.Disabled = func() bool { return !w.canRemoveWorktree() }
	return ui.Dialog{TestID: "worktree-remove-dialog", Role: core.RoleAlertDialog,
		Open: func() bool { return w.worktreeRemove.Get() != nil },
		TitleFunc: func() string {
			if prompt := w.worktreeRemove.Get(); prompt != nil {
				return fmt.Sprintf(w.tr("Remove worktree %q?", "移除工作树「%s」？"), prompt.entry.Name)
			}
			return ""
		},
		DescriptionFunc: func() string {
			if prompt := w.worktreeRemove.Get(); prompt != nil {
				return prompt.entry.Path
			}
			return ""
		},
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				closePrompt()
			}
		},
		Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			prompt := w.worktreeRemove.Get()
			if prompt == nil {
				return core.View{}
			}
			if prompt.loading {
				return chrome.label(w.tr("Checking this worktree…", "正在检查工作树…"), chrome.muted, 12)
			}
			if !prompt.ready || w.worktreeError.Get() != "" {
				return w.worktreeErrorText(chrome)
			}
			if prompt.plan.MetadataOnly() {
				return chrome.label(w.tr("Remove the missing worktree registration. Any files at this path will be kept.",
					"移除已失效的工作树记录。此路径中的文件会保留。"), chrome.text, 12)
			}
			if prompt.entry.Branch == "" {
				return chrome.label(fmt.Sprintf(w.tr("Delete the worktree folder with detached HEAD at %s.",
					"删除 HEAD 分离在 %s 的工作树目录。"), shortSHA(prompt.entry.Head)), chrome.danger, 12)
			}
			return chrome.label(w.tr("Delete this worktree folder and keep its branch.", "删除此工作树目录并保留其分支。"), chrome.danger, 12)
		})},
		Footer: []core.Node{cancel, remove},
	}
}
