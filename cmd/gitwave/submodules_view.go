package main

import (
	"fmt"
	"net/url"
	"strings"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

func (w *workbench) submodulesSidebar(chrome palette) core.Node {
	return core.Show{When: func() bool { return w.path.Get() != "" }, Then: func(cx *core.Scope) core.Node {
		label := w.tr("Submodules", "子模块")
		hasItems := core.NewMemo(cx, func() bool { return len(w.submodules.Get()) > 0 }).Get
		return core.View{Children: []core.Node{
			core.Dynamic(func(*core.Scope) core.Node {
				if hasItems() {
					return sidebarHeading(chrome, "submodules-toggle", label, w.submodulesOpen.Get, func() {
						w.submodulesOpen.Set(!w.submodulesOpen.Get())
					})
				}
				heading := sidebarHeading(chrome, "submodules-toggle", label, nil, nil)
				if w.hasSubmoduleWorktree() {
					heading.Children = append(heading.Children, w.submoduleAddButton(chrome, true))
				}
				return heading
			}),
			core.Show{When: func() bool { return hasItems() && w.submodulesOpen.Get() },
				Then: func(*core.Scope) core.Node { return w.submoduleRows(chrome) }},
			core.Show{When: func() bool {
				if hasItems() {
					return w.submodulesOpen.Get()
				}
				form := w.submoduleForm.Get()
				return form != nil && form.adding
			}, Then: func(*core.Scope) core.Node {
				return w.submoduleAdd(chrome)
			}},
			core.Show{When: func() bool { return w.submoduleCancel.Get() != nil }, Then: func(*core.Scope) core.Node {
				label := w.tr("Cancel", "取消")
				button := toolbarAction(chrome, label, func() {
					if cancel := w.submoduleCancel.Get(); cancel != nil {
						cancel()
					}
				}, chrome.label(label, chrome.text, 11))
				button.TestID = "submodule-cancel-operation"
				return bar(chrome, 32, chrome.panel, chrome.label(w.tr("Working on submodules…", "正在处理子模块…"), chrome.muted, 11), button)
			}},
			sidebarReadError(chrome, "submodule-list-error", w.submoduleListError.Get),
			core.Show{When: func() bool { return w.submoduleError.Get() != "" && w.submoduleForm.Get() == nil },
				Then: func(*core.Scope) core.Node { return w.submoduleErrorText(chrome) }},
		}}
	}}
}

func (w *workbench) submoduleAddButton(chrome palette, compact bool) core.View {
	label := w.tr("Add submodule", "添加子模块")
	open := func() {
		if !w.canEditSubmodules() || w.submoduleForm.Get() != nil {
			return
		}
		w.submoduleError.Set("")
		w.submoduleForm.Set(&submoduleForm{adding: true})
	}
	var button core.View
	if compact {
		button = iconAction(chrome, label, lucide.Plus(), open)
		button.Style.Height, button.Style.Padding = core.Pt(24), core.Edges{Left: 4, Right: 4}
	} else {
		button = toolbarAction(chrome, label, open,
			core.Icon{SVG: lucide.Plus(), Style: core.Style{FontSize: 12, Color: chrome.muted}},
			chrome.label(label, chrome.muted, 11))
	}
	button.TestID = "submodule-open-add"
	button.Disabled = func() bool { return !w.canEditSubmodules() || w.submoduleForm.Get() != nil }
	return button
}

func (w *workbench) submoduleAdd(chrome palette) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		form := w.submoduleForm.Get()
		if form == nil || !form.adding {
			return w.submoduleAddButton(chrome, false)
		}
		busy := func() bool {
			current := w.submoduleForm.Get()
			return w.currentChangesWrite().pending || current != nil && current.challenge != nil
		}
		style := core.Style{Height: core.Pt(27), BorderWidth: 1, BorderColor: chrome.border, Padding: core.All(5)}
		input := func(id, label string, value *core.Signal[string]) core.Input {
			return core.Input{TestID: id, Label: label, Placeholder: label, Disabled: busy,
				Value: value.Get, OnChange: value.Set, Style: style,
				OnSubmit: func(string) { w.runSubmodule(gitcli.SubmoduleInfo{}, "add") }}
		}
		cancelLabel, addLabel := w.tr("Cancel", "取消"), w.tr("Add", "添加")
		cancel := toolbarAction(chrome, cancelLabel, w.closeSubmoduleForm, chrome.label(cancelLabel, chrome.text, 11))
		cancel.Disabled = busy
		add := toolbarAction(chrome, addLabel, func() { w.runSubmodule(gitcli.SubmoduleInfo{}, "add") },
			chrome.label(addLabel, chrome.accent, 11))
		add.TestID = "submodule-add"
		add.Disabled = func() bool {
			return busy() || strings.TrimSpace(w.submoduleURL.Get()) == "" || strings.TrimSpace(w.submodulePath.Get()) == ""
		}
		return core.View{TestID: "submodule-add-form", Style: core.Style{Gap: 4, Padding: core.All(6)}, Children: []core.Node{
			input("submodule-url", w.tr("URL", "URL"), w.submoduleURL),
			input("submodule-path", w.tr("Path", "路径"), w.submodulePath),
			core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifyEnd}, Children: []core.Node{cancel, add}},
			w.submoduleErrorText(chrome),
		}}
	})
}

func (w *workbench) submoduleRows(chrome palette) core.Node {
	const rowHeight = 65
	return core.View{StyleFunc: func() core.Style {
		return core.Style{Height: core.Pt(core.Points(min(rowHeight*3, rowHeight*len(w.submodules.Get()))))}
	}, Children: []core.Node{core.List{TestID: "submodule-list", EstimatedSize: rowHeight,
		Style: core.Style{FlexGrow: 1, FlexShrink: 1},
		Count: func() int { return len(w.submodules.Get()) },
		Key:   func(i int) any { return w.submodules.Get()[i].Name },
		Row: func(_ *core.Scope, row core.ListRow) core.Node {
			return core.Dynamic(func(*core.Scope) core.Node {
				items, index := w.submodules.Get(), row.Index()
				if index < 0 || index >= len(items) {
					return core.View{}
				}
				return w.submoduleRow(chrome, items[index])
			})
		},
	}}}
}

func (w *workbench) submoduleRow(chrome palette, item gitcli.SubmoduleInfo) core.Node {
	address := item.URL
	if address == "" {
		address = item.Path
	} else if parsed, err := url.Parse(address); err == nil {
		address = parsed.Redacted()
	}
	if item.Head != "" {
		address += " · " + shortSHA(item.Head)
	}
	action := func(id, label string, click func()) core.View {
		button := toolbarAction(chrome, label, click, chrome.label(label, chrome.text, 11))
		button.TestID = id
		button.Disabled = func() bool { return !w.canEditSubmodules() || w.submoduleForm.Get() != nil }
		button.Style.Height = core.Pt(23)
		return button
	}
	label := chrome.label(item.Name, chrome.text, 12)
	addressText := chrome.label(address, chrome.muted, 10)
	label.MaxLines, addressText.MaxLines = 1, 1
	status := lucide.Blocks()
	color := chrome.muted
	if item.Initialized {
		status, color = lucide.CircleAlert(), chrome.danger
		if item.InSync {
			status, color = lucide.CircleCheck(), chrome.accent
		}
	}
	var actions []core.Node
	if item.Initialized {
		updateLabel := w.tr("Sync", "同步")
		if item.InSync {
			updateLabel = w.tr("Update", "更新")
		}
		actions = append(actions, action("submodule-update", updateLabel, func() { w.runSubmodule(item, "update") }),
			action("submodule-deinit", w.tr("Deactivate", "取消注册"), func() {
				if current, found := w.currentSubmodule(item); found {
					w.submoduleError.Set("")
					w.submoduleForm.Set(&submoduleForm{entry: current})
				}
			}))
	} else {
		actions = append(actions, action("submodule-init", w.tr("Initialize", "初始化"), func() { w.runSubmodule(item, "init") }))
	}
	return core.View{TestID: "submodule-row", Label: item.Name, Tooltip: chrome.label(item.Path, chrome.text, 11),
		Style: core.Style{Padding: core.Edges{Left: 10, Right: 5, Top: 4, Bottom: 3}, MinHeight: core.Pt(65), Gap: 2,
			Hover: &core.Style{Background: chrome.hover}},
		Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, Gap: 6}, Children: []core.Node{
				core.Icon{SVG: status, Style: core.Style{FontSize: 13, Color: color}},
				core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{label, addressText}},
			}},
			core.View{Style: core.Style{Direction: core.Row, Gap: 4}, Children: actions},
		}}
}

func (w *workbench) submoduleErrorText(chrome palette) core.Node {
	return core.Text{TestID: "submodule-error", ContentFunc: w.submoduleError.Get,
		Role: core.RoleAlert, Style: core.Style{FontSize: chrome.uiSize(11), Color: chrome.danger}}
}

func (w *workbench) submoduleDialog(chrome palette) core.Node {
	return core.Show{When: func() bool {
		form := w.submoduleForm.Get()
		return form != nil && (!form.adding || form.challenge != nil)
	}, Then: func(*core.Scope) core.Node {
		form := w.submoduleForm.Get()
		busy := func() bool { return w.currentChangesWrite().pending }
		cancel := func() {
			if active := w.submoduleCancel.Get(); active != nil {
				active()
			} else if !busy() {
				w.closeSubmoduleForm()
			}
		}
		title := fmt.Sprintf(w.tr("Deactivate submodule %q?", "取消注册子模块「%s」？"), form.entry.Name)
		description := w.tr("Remove the local registration. The submodule directory and its files are kept.", "移除本地注册，保留子模块目录及其中的文件。")
		label, action := w.tr("Deactivate", "取消注册"), "deinit"
		role, color := core.RoleAlertDialog, chrome.danger
		var inputs []core.Node
		if form.challenge != nil {
			title = w.tr("Submodule authentication required", "子模块需要身份验证")
			description = form.challenge.Remote + " · " + form.challenge.Location()
			label, action = w.tr("Retry", "重试"), "retry"
			role, color = core.RoleDialog, chrome.accent
			inputs = append(inputs, w.inlineAuthInputs("submodule", w.submoduleAuth,
				core.Style{Height: core.Pt(32), BorderWidth: 1, BorderColor: chrome.border, Padding: core.All(6)},
				func() { w.runSubmodule(form.entry, action) }))
		}
		inputs = append(inputs, w.submoduleErrorText(chrome))
		cancelLabel := w.tr("Cancel", "取消")
		cancelButton := toolbarAction(chrome, cancelLabel, cancel, chrome.label(cancelLabel, chrome.text, 12))
		cancelButton.Disabled = func() bool { return busy() && w.submoduleCancel.Get() == nil }
		confirm := toolbarAction(chrome, label, func() { w.runSubmodule(form.entry, action) }, chrome.label(label, color, 12))
		confirm.TestID = "submodule-confirm"
		confirm.Disabled = func() bool {
			return !w.canEditSubmodules() || form.challenge != nil && w.submoduleAuth.incomplete()
		}
		return ui.Dialog{TestID: "submodule-dialog", Title: title, Description: description, Role: role,
			Open: func() bool { return true }, OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					cancel()
				}
			}, Children: []core.Node{core.View{Style: core.Style{Gap: 7}, Children: inputs}},
			Footer: []core.Node{cancelButton, confirm}}
	}}
}
