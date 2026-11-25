package main

import (
	"fmt"
	"net/url"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

func (w *workbench) remotesSidebar(chrome palette) core.Node {
	return core.Show{When: func() bool { return w.path.Get() != "" }, Then: func(cx *core.Scope) core.Node {
		label := w.tr("Remotes", "远程")
		hasRemotes := core.NewMemo(cx, func() bool { return len(w.remoteList.Get()) > 0 })
		showMenu := func(at core.Point) {
			w.cx.PopupMenu([]core.MenuItem{{Label: w.tr("Add remote", "添加远程"), Disabled: !w.canManageRemotes(),
				OnSelect: w.scopedRefAction(func() { w.openRemoteForm("add", gitcli.RemoteInfo{}) })}}, at, nil)
		}
		return core.View{Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			var toggle func()
			if hasRemotes.Get() {
				toggle = func() { w.remoteOpen.Set(!w.remoteOpen.Get()) }
			}
			header := sidebarHeading(chrome, "remotes-toggle", label, w.remoteOpen.Get, toggle)
			ref := new(core.Ref)
			header.Ref, header.Focusable = ref, func() bool { return true }
			header.OnContextMenu = func(e core.ContextMenuEvent) { showMenu(e.PagePosition) }
			header.OnKeyDown = func(e core.KeyEvent) bool { return refMenuKey(ref, e, showMenu) }
			return header
		}),
			sidebarReadError(chrome, "remote-list-error", w.remoteListError.Get),
			core.Show{When: func() bool { return w.remoteError.Get() != "" && w.remoteForm.Get() == nil },
				Then: func(*core.Scope) core.Node { return w.remoteErrorText(chrome) }},
			core.Show{When: func() bool { return w.remoteOpen.Get() && hasRemotes.Get() },
				Then: func(*core.Scope) core.Node { return w.remoteRows(chrome) }},
		}}
	}}
}

func (w *workbench) remoteRows(chrome palette) core.Node {
	const rowHeight = 37
	return core.View{Style: core.Style{FlexShrink: 1}, StyleFunc: func() core.Style {
		return core.Style{Height: core.Pt(core.Points(min(185, rowHeight*len(w.remoteList.Get()))))}
	}, Children: []core.Node{core.List{TestID: "remotes-list", EstimatedSize: rowHeight,
		Style: core.Style{FlexGrow: 1, FlexShrink: 1},
		Count: func() int { return len(w.remoteList.Get()) },
		Key:   func(i int) any { return w.remoteList.Get()[i].Name },
		Row: func(_ *core.Scope, row core.ListRow) core.Node {
			return core.Dynamic(func(*core.Scope) core.Node {
				items := w.remoteList.Get()
				i := row.Index()
				if i < 0 || i >= len(items) {
					return core.View{}
				}
				return w.remoteRow(chrome, items[i])
			})
		},
	}}}
}

func (w *workbench) remoteRow(chrome palette, remote gitcli.RemoteInfo) core.Node {
	address := remote.FetchURL
	if address == "" {
		address = w.tr("(no URL)", "（无 URL）")
	} else if parsed, err := url.Parse(address); err == nil {
		address = parsed.Redacted()
	}
	name, addressText := chrome.label(remote.Name, chrome.text, 11), chrome.label(address, chrome.muted, 10)
	name.MaxLines, addressText.MaxLines = 1, 1
	ref := new(core.Ref)
	showMenu := func(at core.Point) {
		disabled := !w.canManageRemotes()
		w.cx.PopupMenu([]core.MenuItem{
			{Label: w.tr("Fetch", "获取"), Disabled: disabled, OnSelect: w.scopedRefAction(func() { w.fetchSelectedRemote(remote) })},
			{Label: w.tr("Edit…", "编辑…"), Disabled: disabled, OnSelect: w.scopedRefAction(func() { w.openRemoteForm("edit", remote) })},
			{Separator: true},
			{Label: w.tr("Remove", "移除"), Disabled: disabled, OnSelect: w.scopedRefAction(func() { w.openRemoteForm("remove", remote) })},
		}, at, nil)
	}
	return core.View{Label: remote.Name, Role: core.RoleButton, TestID: "remote-row", Ref: ref,
		Tooltip:       chrome.label(address, chrome.text, 12),
		OnPress:       func(e core.PressEvent) { showMenu(e.PagePosition) },
		OnContextMenu: func(e core.ContextMenuEvent) { showMenu(e.PagePosition) },
		OnKeyDown:     func(e core.KeyEvent) bool { return refMenuKey(ref, e, showMenu) },
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 5, MinHeight: core.Pt(37),
			Padding: core.Edges{Left: 11, Right: 5}, Hover: &core.Style{Background: chrome.hover},
			FocusVisible: &core.Style{OutlineWidth: 1, OutlineColor: chrome.accent}},
		Children: []core.Node{core.Icon{SVG: lucide.Cloud(), Style: core.Style{FontSize: 14, Color: chrome.muted}},
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{name, addressText}}},
	}
}

func (w *workbench) remoteErrorText(chrome palette) core.Node {
	return core.Text{TestID: "remote-error", ContentFunc: w.remoteError.Get, Role: core.RoleAlert,
		Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.danger}}
}

func (w *workbench) remoteDialog(chrome palette) core.Node {
	return core.Show{When: func() bool { return w.remoteForm.Get() != nil }, Then: func(*core.Scope) core.Node {
		form := w.remoteForm.Get()
		busy := func() bool { return w.currentChangesWrite().pending }
		cancel := func() {
			if !busy() {
				w.closeRemoteForm()
			}
		}
		title, confirmLabel := w.tr("Add remote", "添加远程"), w.tr("Add", "添加")
		var description string
		role, color := core.RoleDialog, chrome.accent
		if form.kind == "edit" {
			title = fmt.Sprintf(w.tr("Edit remote %q", "编辑远程「%s」"), form.entry.Name)
			confirmLabel = w.tr("Save", "保存")
		}
		if form.kind == "remove" {
			title = fmt.Sprintf(w.tr("Remove remote %q?", "移除远程「%s」？"), form.entry.Name)
			description = w.tr("Remove the remote configuration and its tracking branches. Local branches and commits are kept.",
				"移除远程配置和对应的远程跟踪分支，保留本地分支和提交。")
			confirmLabel, role, color = w.tr("Remove", "移除"), core.RoleAlertDialog, chrome.danger
		}
		cancelLabel := w.tr("Cancel", "取消")
		cancelButton := toolbarAction(chrome, cancelLabel, cancel, chrome.label(cancelLabel, chrome.text, 12))
		cancelButton.Disabled = busy
		confirm := toolbarAction(chrome, confirmLabel, w.submitRemoteForm, chrome.label(confirmLabel, color, 12))
		confirm.TestID = "remote-confirm"
		confirm.Disabled = func() bool { return !w.canSubmitRemoteForm() }
		input := func(id, label, placeholder string, value *core.Signal[string], focus bool) core.Node {
			return core.Input{TestID: id, Label: label, Placeholder: placeholder, Value: value.Get, OnChange: value.Set,
				Disabled: busy, AutoFocus: focus, OnSubmit: func(string) { w.submitRemoteForm() },
				Style: core.Style{Height: core.Pt(32), Padding: core.All(6), BorderWidth: 1, BorderColor: chrome.border}}
		}
		var fields []core.Node
		if form.kind != "remove" {
			fields = append(fields,
				input("remote-name", w.tr("Name", "名称"), w.tr("Name (e.g. origin)", "名称（如 origin）"), w.remoteName, true),
				input("remote-fetch-url", w.tr("Fetch URL", "获取 URL"), w.tr("Fetch URL", "获取 URL"), w.remoteFetch, false))
		}
		if form.kind == "edit" {
			fields = append(fields, input("remote-push-url", w.tr("Push URL", "推送 URL"),
				w.tr("Push URL (empty = same as fetch)", "推送 URL（留空 = 与 fetch 相同）"), w.remotePush, false))
		}
		fields = append(fields, w.remoteErrorText(chrome))
		return ui.Dialog{TestID: "remote-dialog", Title: title, Description: description, Role: role,
			Open: func() bool { return true }, OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					cancel()
				}
			},
			Children: []core.Node{core.View{Style: core.Style{Gap: 7}, Children: fields}},
			Footer:   []core.Node{cancelButton, confirm}}
	}}
}
