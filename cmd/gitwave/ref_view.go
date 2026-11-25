package main

import (
	"fmt"
	"slices"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

func (w *workbench) branchMenu() []core.MenuItem {
	current := w.repo.Get()
	local := w.reflogLocalBranch()
	return []core.MenuItem{{
		Label: w.tr("New", "新建"), Disabled: current.Head == "" || local.Name == "" || w.currentChangesWrite().pending,
		OnSelect: w.scopedRefAction(func() {
			w.openRefForm(refDialogState{kind: createBranchForm, base: current.Head, baseLabel: current.Branch})
		}),
	}, {Label: w.tr("Pull", "拉取"), Disabled: !w.canOpenPull(), OnSelect: w.scopedRefAction(w.openPull)},
		{Label: w.tr("Push", "推送"), Disabled: !w.canPushFrom(w.pullSourceBranch()), OnSelect: w.scopedRefAction(w.openPush)},
		{Label: "PR",
			Disabled: w.workspaceStore == nil || w.workspaceState.Get().Workspace.ID == "" || local.Name == "" || current.Head == "",
			OnSelect: w.scopedRefAction(w.openPullRequestDraft)}}
}

func (w *workbench) branchRowMenu(b branch, event core.Point) {
	w.cx.PopupMenu(w.branchRowItems(b), event, nil)
}

func (w *workbench) branchRowItems(b branch) []core.MenuItem {
	pending := w.currentChangesWrite().pending
	checkout := w.repo.Get()
	paused := b.Current && w.changes.Get().status.Commit.InteractiveRebase != ""
	open := func(kind refFormKind) func() {
		return w.scopedRefAction(func() {
			w.openRefForm(refDialogState{kind: kind, branch: b, base: b.Tip, baseLabel: b.Name})
		})
	}
	items := []core.MenuItem{
		{Label: w.tr("Checkout", "检出"), Disabled: pending || b.Current || b.Tip == "",
			OnSelect: w.scopedRefAction(func() { w.checkoutBranch(b) })},
		{Label: w.tr("New Branch…", "新建分支…"), Disabled: pending || b.Tip == "", OnSelect: open(createBranchForm)},
	}
	if b.Kind == "local" {
		items = slices.Insert(items, 1, w.branchPushItem(b, checkout.Remotes))
		items = append(items,
			core.MenuItem{Label: w.tr("Merge into current", "合并到当前分支"), Disabled: !w.canOpenMerge(b),
				OnSelect: w.scopedRefAction(func() { w.openBranchMerge(b) })},
			core.MenuItem{Label: w.tr("Rebase current onto this", "将当前分支变基到此分支"), Disabled: !w.canRebaseBranch(b, checkout),
				OnSelect: w.scopedRefAction(func() { w.startBranchRebase(b, checkout) })},
			core.MenuItem{Label: w.tr("Interactive rebase", "交互式变基"), Disabled: !w.canRebaseBranch(b, checkout),
				OnSelect: w.scopedRefAction(func() { w.openInteractiveRebase(b, checkout) })},
			core.MenuItem{Label: w.tr("Rename…", "重命名…"), Disabled: pending || paused || b.Tip == "", OnSelect: open(renameBranchForm)},
			core.MenuItem{Label: w.tr("Tracking…", "设置跟踪…"), Disabled: pending || b.Tip == "", OnSelect: open(trackingForm)},
			core.MenuItem{Label: w.tr("Delete Branch…", "删除分支…"), Disabled: pending || b.Current || b.Tip == "", OnSelect: open(deleteBranchForm)})
	} else if b.Kind == "remote" {
		items = append(items, core.MenuItem{Label: w.tr("Delete Remote Branch…", "删除远端分支…"),
			Disabled: pending || b.Current || b.Tip == "" || w.fetch.prompt.Get() != nil, OnSelect: open(deleteRemoteBranchForm)})
	}
	items = append(items,
		core.MenuItem{Label: w.tr("Tag…", "标签…"), Disabled: pending || b.Tip == "", OnSelect: open(tagManagerForm)},
		core.MenuItem{Label: w.tr("Copy Branch Name", "复制分支名称"), OnSelect: func() { w.cx.SetClipboard(b.Name) }})
	return items
}

func (w *workbench) showTagManager(sha string) {
	if sha != "" {
		w.openRefForm(refDialogState{kind: tagManagerForm, base: sha})
	}
}

func (w *workbench) tagSidebar(chrome palette) core.Node {
	label := w.tr("Tags", "标签")
	return core.Show{When: func() bool { return w.path.Get() != "" }, Then: func(cx *core.Scope) core.Node {
		hasTags := core.NewMemo(cx, func() bool { return len(w.repo.Get().Tags) > 0 })
		cx.Effect(func() {
			if !hasTags.Get() {
				w.tagsOpen.Set(false)
			}
		})
		return core.View{Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			if !hasTags.Get() {
				return sidebarHeading(chrome, "tags-toggle", label, nil, nil)
			}
			return sidebarHeading(chrome, "tags-toggle", label, w.tagsOpen.Get, func() {
				w.tagsOpen.Set(!w.tagsOpen.Get())
			})
		}), core.Show{When: func() bool { return w.tagsOpen.Get() && hasTags.Get() }, Then: func(*core.Scope) core.Node {
			return core.List{TestID: "tags-list", EstimatedSize: 28,
				Style: core.Style{Height: core.Pt(160), FlexShrink: 1, MinHeight: core.Pt(0)},
				StyleFunc: func() core.Style {
					return core.Style{Height: core.Pt(core.Points(min(160, 28*len(w.repo.Get().Tags))))}
				},
				Count: func() int { return len(w.repo.Get().Tags) },
				Key:   func(i int) any { return w.repo.Get().Tags[i].Name },
				Row: func(_ *core.Scope, row core.ListRow) core.Node {
					return core.Dynamic(func(*core.Scope) core.Node {
						tags, i := w.repo.Get().Tags, row.Index()
						if i < 0 || i >= len(tags) {
							return core.View{}
						}
						return w.tagSidebarRow(chrome, tags[i])
					})
				}}
		}},
		}}
	}}
}

func (w *workbench) tagSidebarRow(chrome palette, tag tagRef) core.Node {
	label := tag.Name
	name := chrome.label(label, chrome.text, 12)
	name.Style.FlexGrow, name.Style.FlexShrink = 1, 1
	ref := new(core.Ref)
	row := toolbarAction(chrome, label, func() { w.selectTag(tag) },
		core.Icon{SVG: lucide.Tag(), Style: core.Style{FontSize: 12, Color: chrome.muted}},
		name, chrome.label(shortSHA(tag.Tip), chrome.muted, 11))
	row.Ref, row.TestID = ref, "tag-row"
	row.Style.Padding.Left = 16
	row.Selected = func() bool { return tag.Tip != "" && w.selected.Get() == tag.Tip }
	row.Style.Selected = &core.Style{Background: chrome.selected}
	menu := func(at core.Point) {
		w.cx.PopupMenu([]core.MenuItem{
			{Label: w.tr("Checkout", "检出"), Disabled: tag.Tip == "" || w.currentChangesWrite().pending,
				OnSelect: w.scopedRefAction(func() { w.checkoutTag(tag) })},
			{
				Label: w.tr("Delete tag", "删除标签"), Disabled: w.currentChangesWrite().pending,
				OnSelect: w.scopedRefAction(func() { w.openRefForm(refDialogState{kind: deleteTagForm, tag: tag}) }),
			},
		}, at, nil)
	}
	row.OnContextMenu = func(e core.ContextMenuEvent) { menu(e.PagePosition) }
	row.OnKeyDown = func(e core.KeyEvent) bool { return refMenuKey(ref, e, menu) }
	return row
}

func refMenuKey(ref *core.Ref, event core.KeyEvent, show func(core.Point)) bool {
	if event.Key != "contextmenu" && (event.Key != "f10" || !event.Modifiers.Shift) {
		return false
	}
	if bounds, ok := ref.Bounds(); ok {
		position := bounds.Origin
		position.Y += bounds.Size.Height
		show(position)
	}
	return true
}

type refDialogCopy struct {
	title, description, confirm string
	destructive                 bool
}

func (w *workbench) describeRefForm(form *refDialogState) refDialogCopy {
	if form == nil {
		return refDialogCopy{}
	}
	switch form.kind {
	case createBranchForm:
		return refDialogCopy{
			title:       fmt.Sprintf(w.tr("New branch from %q", "从“%s”新建分支"), form.baseLabel),
			description: w.tr("Creates a local branch at this commit. The current branch stays checked out.", "从这个提交创建本地分支。当前分支保持检出。"),
			confirm:     w.tr("Create", "创建"),
		}
	case recoveryBranchForm:
		return refDialogCopy{title: w.tr("Create recovery branch", "创建恢复分支"),
			description: fmt.Sprintf(w.tr("Create a recovery branch at %s.", "在 %s 创建恢复分支。"), shortSHA(form.base)),
			confirm:     w.tr("Create branch", "创建分支")}
	case resetBranchForm:
		return refDialogCopy{title: fmt.Sprintf(w.tr("Reset %s to %s?", "将 %s 重置到 %s？"), form.branch.Name, shortSHA(form.base)),
			description: w.tr("This permanently discards staged and unstaged changes to tracked files. The previous commit stays in the reflog.",
				"此操作会永久丢弃跟踪文件中已暂存和未暂存的改动。上一个提交仍可在 reflog 中找到。"),
			confirm: w.tr("Reset branch", "重置分支"), destructive: true}
	case renameBranchForm:
		return refDialogCopy{title: w.tr("Rename Branch", "重命名分支"), confirm: w.tr("Rename", "重命名")}
	case deleteBranchForm:
		return refDialogCopy{title: w.tr("Delete Branch", "删除分支"),
			description: fmt.Sprintf(w.tr("Delete local branch %q from this repository?", "从此仓库删除本地分支“%s”？"), form.branch.Name),
			confirm:     w.tr("Delete", "删除"), destructive: true}
	case deleteRemoteBranchForm:
		return refDialogCopy{title: w.tr("Delete Remote Branch", "删除远端分支"),
			description: fmt.Sprintf(w.tr("Delete remote branch %q?", "删除远端分支“%s”？"), form.branch.Name),
			confirm:     w.tr("Delete", "删除"), destructive: true}
	case trackingForm:
		description := w.tr("Pick the remote branch this branch should track.", "选择此分支要跟踪的远程分支。")
		if form.branch.Upstream != "" {
			description = fmt.Sprintf(w.tr("Current upstream is %s. Pick the remote branch this branch should track.",
				"当前 upstream 为 %s。请选择此分支要跟踪的远程分支。"), form.branch.Upstream)
		}
		return refDialogCopy{title: fmt.Sprintf(w.tr("Tracking for %s", "%s 的跟踪设置"), form.branch.Name),
			description: description, confirm: w.tr("Set tracking", "设置跟踪")}
	case mergeBranchForm:
		return refDialogCopy{title: w.tr("Merge branch", "合并分支"),
			description: fmt.Sprintf(w.tr("Merge %s into %s.", "将 %s 合并到 %s。"), form.branch.Name, form.baseLabel),
			confirm:     w.tr("Merge", "合并")}
	case cherryPickForm, revertCommitForm:
		return w.describeCommitReplay(*form)
	case tagManagerForm:
		return refDialogCopy{title: fmt.Sprintf(w.tr("Tags on %s", "%s 上的标签"), shortSHA(form.base)),
			confirm: w.tr("Create tag", "创建标签")}
	case replaceTagForm:
		return refDialogCopy{title: fmt.Sprintf(w.tr("Replace tag %q?", "替换标签“%s”？"), form.tag.Name),
			description: fmt.Sprintf(w.tr("The local tag will point to %s.", "本地标签将指向 %s。"), shortSHA(form.base)),
			confirm:     w.tr("Replace tag", "替换标签"), destructive: true}
	case deleteTagForm:
		return refDialogCopy{title: w.tr("Delete Tag", "删除标签"),
			description: fmt.Sprintf(w.tr("Delete tag %q from this repository? Its commit is kept.", "从此仓库删除标签“%s”？对应的提交会保留。"), form.tag.Name),
			confirm:     w.tr("Delete", "删除"), destructive: true}
	}
	return refDialogCopy{}
}

func (w *workbench) refDialog(cx *core.Scope, chrome palette) core.Node {
	copy := core.NewMemo(cx, func() refDialogCopy { return w.describeRefForm(w.refForm.Get()) })
	busy := func() bool { return w.currentChangesWrite().pending }
	label := func() string { return copy.Get().confirm }
	confirm := toolbarAction(chrome, "", w.submitRefForm, core.Text{ContentFunc: label,
		Style: core.Style{Color: core.RGB(0xffffff), FontSize: chrome.uiSize(12)}})
	confirm.TestID, confirm.LabelFunc = "confirm-ref-action", label
	confirm.Disabled = func() bool { return !w.canSubmitRefForm() }
	confirm.Style.Background = chrome.accent
	confirm.Style.Disabled = &core.Style{Opacity: core.Alpha(.4)}
	confirm.StyleFunc = func() core.Style {
		if copy.Get().destructive {
			return core.Style{Background: chrome.danger}
		}
		return core.Style{}
	}
	return ui.Dialog{
		TestID: "ref-dialog", Open: func() bool { return w.refForm.Get() != nil },
		TitleFunc:       func() string { return copy.Get().title },
		DescriptionFunc: func() string { return copy.Get().description },
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				w.cancelRefForm()
			}
		},
		Children: []core.Node{core.Dynamic(func(cx *core.Scope) core.Node {
			form := w.refForm.Get()
			if form == nil {
				return core.View{Style: core.Style{Display: core.DisplayNone}}
			}
			switch form.kind {
			case createBranchForm, recoveryBranchForm, renameBranchForm:
				return w.refNameField(chrome, w.tr("Branch name", "分支名称"), w.tr("feature/my-branch", "feature/my-branch"), busy)
			case trackingForm:
				return w.trackingField(busy)
			case deleteBranchForm, deleteRemoteBranchForm:
				return w.remoteBranchDeleteFields(chrome, form, busy)
			case tagManagerForm:
				return w.tagManagerFields(cx, chrome, *form, busy)
			case mergeBranchForm:
				return w.mergeFields(chrome, form)
			case resetBranchForm:
				if form.checking {
					return chrome.label(w.tr("Checking the working copy…", "正在检查工作区…"), chrome.muted, 12)
				}
				if form.reset != nil && form.reset.DirtyFiles != 0 {
					return chrome.label(fmt.Sprintf(w.tr("Tracked files with changes · %d", "有改动的跟踪文件 · %d"), form.reset.DirtyFiles), chrome.warning, 12)
				}
				return core.View{Style: core.Style{Display: core.DisplayNone}}
			default:
				return core.View{Style: core.Style{Display: core.DisplayNone}}
			}
		}), core.Show{When: func() bool { return w.refFailure.Get() != "" }, Then: func(*core.Scope) core.Node {
			return core.Text{TestID: "ref-action-error", ContentFunc: w.refFailure.Get, Role: core.RoleAlert,
				Style: core.Style{Color: chrome.danger, FontSize: chrome.uiSize(12)}}
		}}},
		Footer: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			label := w.tr("Cancel", "取消")
			if form := w.refForm.Get(); form != nil && form.kind == tagManagerForm {
				label = w.tr("Close", "关闭")
			}
			button := toolbarAction(chrome, label, w.cancelRefForm, chrome.label(label, chrome.text, 12))
			button.Disabled = busy
			return button
		}), confirm},
	}
}

func (w *workbench) refNameField(chrome palette, label, placeholder string, busy func() bool) core.Input {
	return core.Input{TestID: "ref-name", Label: label, Placeholder: placeholder,
		Value: w.refName.Get, OnChange: w.refName.Set, AutoFocus: true, NoAutoCorrect: true, Disabled: busy,
		OnSubmit: func(string) { w.submitRefForm() }, Style: core.Style{
			Height: core.Pt(32), Padding: core.All(6), BorderWidth: 1, BorderColor: chrome.border,
		}}
}

func (w *workbench) trackingField(busy func() bool) core.Node {
	options := []ui.Option[string]{{Value: "", Label: w.tr("No upstream", "无 upstream")}}
	for _, b := range w.repo.Get().Branches {
		if b.Kind == "remote" {
			options = append(options, ui.Option[string]{Value: "refs/remotes/" + b.Name, Label: b.Name})
		}
	}
	current := w.refForm.Get().branch.UpstreamRef
	if current != "" && !slices.ContainsFunc(options, func(option ui.Option[string]) bool { return option.Value == current }) {
		options = append(options, ui.Option[string]{Value: current, Label: current})
	}
	return ui.Select[string]{TestID: "branch-upstream", Label: w.tr("Upstream branch", "Upstream 分支"),
		Options: options, Value: w.refUpstream.Get, OnChange: w.refUpstream.Set, Disabled: busy, Searchable: true}
}

func (w *workbench) tagManagerFields(cx *core.Scope, chrome palette, form refDialogState, busy func() bool) core.Node {
	tags := core.NewMemoFunc(cx, func() []tagRef {
		var selected []tagRef
		for _, tag := range w.repo.Get().Tags {
			if tag.Tip == form.base {
				selected = append(selected, tag)
			}
		}
		return selected
	}, slices.Equal[[]tagRef])
	list := core.Dynamic(func(scope *core.Scope) core.Node {
		values := tags.Get()
		if len(values) == 0 {
			return chrome.label(w.tr("No tags on this commit.", "此提交没有标签。"), chrome.muted, 12)
		}
		notes := core.NewSignalFunc(scope, map[string]string{}, nil)
		ctx := scope.Context()
		go func() {
			messages := make(map[string]string)
			for _, tag := range values {
				if ctx.Err() != nil {
					return
				}
				if tag.Annotated {
					if message, err := form.child.repo.TagAnnotation(ctx, tag.Object); err == nil {
						messages[tag.Object] = message
					}
				}
			}
			scope.Post(func() {
				if ctx.Err() == nil {
					notes.Set(messages)
				}
			})
		}()
		rows := make([]core.Node, 0, len(values))
		for _, tag := range values {
			label := tag.Name
			if tag.Annotated {
				label += w.tr(" (annotated)", "（附注）")
			}
			title := chrome.label(label, chrome.text, 12)
			title.Style.FlexGrow, title.Style.FlexShrink = 1, 1
			remove := iconAction(chrome, fmt.Sprintf(w.tr("Delete tag %s", "删除标签 %s"), tag.Name), lucide.Trash(), func() {
				state := w.refForm.Get()
				if state != nil && state.kind == tagManagerForm && !busy() {
					next := *state
					next.kind, next.tag = deleteTagForm, tag
					w.refFailure.Set("")
					w.refForm.Set(&next)
				}
			})
			remove.Disabled = busy
			row := bar(chrome, 30, chrome.elevated, core.Icon{SVG: lucide.Tag(), Style: core.Style{FontSize: 12, Color: chrome.muted}}, title, remove)
			if tag.Annotated {
				row.Tooltip = core.Text{ContentFunc: func() string { return notes.Get()[tag.Object] },
					MaxLines: 6, Style: core.Style{MaxWidth: core.Pt(300), Color: chrome.text, FontSize: chrome.uiSize(12)}}
			}
			rows = append(rows, row)
		}
		return core.Scroll{Style: core.Style{MaxHeight: core.Pt(160)}, Children: rows}
	})
	return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
		list,
		w.refNameField(chrome, w.tr("Tag name", "标签名称"), w.tr("Tag name (e.g. v1.2.0)", "标签名称（例如 v1.2.0）"), busy),
		core.Input{TestID: "tag-annotation", Label: w.tr("Annotation", "附注"), Multiline: true, Disabled: busy,
			Placeholder: w.tr("Annotation (optional)", "附注（可选）"), Value: w.refAnnotation.Get, OnChange: w.refAnnotation.Set,
			Style: core.Style{Height: core.Pt(55), Padding: core.All(6), BorderWidth: 1, BorderColor: chrome.border}},
	}}
}
