package main

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type historyRef struct {
	commitRef
	synced bool
}

func historyRefs(refs []commitRef, remotes []string) []historyRef {
	if len(refs) == 0 {
		return nil
	}
	locals, remoteNames := make(map[string]bool), make(map[string]bool)
	for _, ref := range refs {
		switch ref.Kind {
		case "local_branch":
			locals[ref.Name] = true
		case "remote_branch":
			_, short := splitRemoteBranch(ref.Name, remotes)
			remoteNames[short] = true
		}
	}
	visible := make([]historyRef, 0, len(refs))
	for _, ref := range refs {
		if ref.Kind == "remote_branch" {
			if _, short := splitRemoteBranch(ref.Name, remotes); locals[short] {
				continue
			}
		}
		visible = append(visible, historyRef{ref, ref.Kind == "local_branch" && remoteNames[ref.Name]})
	}
	priority := func(kind string) int {
		switch kind {
		case "tag":
			return 0
		case "local_branch":
			return 1
		default:
			return 2
		}
	}
	slices.SortStableFunc(visible, func(a, b historyRef) int { return cmp.Compare(priority(a.Kind), priority(b.Kind)) })
	return visible
}

func (w *workbench) historyReferenceCell(cx *core.Scope, chrome palette, at func() commit, head func() bool, width func() core.Points) core.Node {
	refs := core.NewMemoFunc(cx, func() []historyRef { return historyRefs(at().Refs, w.repo.Get().Remotes) }, slices.Equal)
	sha := core.NewMemo(cx, func() string { return at().SHA })
	isHead := core.NewMemo(cx, head)
	opened := core.NewSignal(cx, "")
	closeDialog := func() { opened.Set("") }
	cx.Effect(func() {
		_ = sha.Get()
		closeDialog()
	})
	return core.View{Style: core.Style{Direction: core.Row, MinWidth: core.Pt(0), AlignItems: core.AlignCenter,
		JustifyContent: core.JustifyEnd, Gap: 4}, StyleFunc: func() core.Style {
		return core.Style{Width: core.Pt(width())}
	}, Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
		visible := refs.Get()
		if len(visible) == 0 {
			return core.View{}
		}
		commitSHA := sha.Get()
		primary := w.historyRefBadge(chrome, commitSHA, visible[0], isHead.Get(), true, closeDialog)
		if len(visible) == 1 {
			return primary
		}
		title := fmt.Sprintf(w.tr("All refs (%d)", "全部 refs（%d）"), len(visible))
		more := toolbarAction(chrome, title, func() { opened.Set(commitSHA) }, chrome.label("+"+strconv.Itoa(len(visible)-1), chrome.muted, 11))
		more.TestID = "history-refs-more"
		more.Style.Height, more.Style.Padding = core.Pt(18), core.Edges{Left: 4, Right: 4}
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4,
			MinWidth: core.Pt(0), MaxWidth: core.Pct(100), FlexShrink: 1}, Children: []core.Node{primary, more,
			core.Show{When: func() bool { return opened.Get() == commitSHA }, Then: func(*core.Scope) core.Node {
				list := core.List{TestID: "history-refs-list", EstimatedSize: 21, Gap: 8,
					Style: core.Style{Height: core.Pt(core.Points(min(384, len(visible)*29-8))), MinHeight: core.Pt(0), FlexShrink: 1},
					Count: func() int { return len(visible) }, Key: func(i int) any { return visible[i].commitRef },
					Row: func(_ *core.Scope, row core.ListRow) core.Node {
						return core.Dynamic(func(*core.Scope) core.Node {
							i := row.Index()
							if i < 0 || i >= len(visible) {
								return core.View{}
							}
							return core.View{Style: core.Style{AlignItems: core.AlignStart}, Children: []core.Node{
								w.historyRefBadge(chrome, commitSHA, visible[i], isHead.Get(), false, closeDialog),
							}}
						})
					},
				}
				return ui.Dialog{Title: title, TestID: "history-refs-dialog", Open: func() bool { return opened.Get() == commitSHA },
					Style: core.Style{Width: core.Pt(384)}, OnOpenChange: func(open bool, _ base.Reason) {
						if !open {
							closeDialog()
						}
					}, Children: []core.Node{list},
					Footer: []core.Node{action(chrome, w.tr("Close", "关闭"), closeDialog)}}
			}},
		}}
	})}}
}

func (w *workbench) historyRefBadge(chrome palette, sha string, ref historyRef, head, compact bool, closeDialog func()) core.View {
	badge := w.referenceBadge(chrome, ref, compact, head && ref.Kind != "remote_branch" && (compact || ref.Kind == "local_branch"))
	selectRow := func(core.PressEvent) {
		w.locate = ""
		w.selectCommit(sha)
	}
	anchor := new(core.Ref)
	menu := func(at core.Point) {
		selectRow(core.PressEvent{})
		w.cx.PopupMenu(w.historyRefItems(sha, ref.commitRef, closeDialog), at, nil)
	}
	badge.Ref, badge.TestID, badge.Role = anchor, "history-refs", core.RoleButton
	badge.OnContextMenu = func(e core.ContextMenuEvent) { menu(e.PagePosition) }
	badge.OnKeyDown = func(e core.KeyEvent) bool { return refMenuKey(anchor, e, menu) }
	if compact {
		badge.OnPress = selectRow
	}
	return badge
}

func (w *workbench) referenceBadge(chrome palette, ref historyRef, compact, emphasize bool) core.View {
	name := ref.Name
	if runes := []rune(name); compact && len(runes) > 20 {
		name = string(runes[:20]) + "..."
	}
	icon := lucide.GitBranch()
	switch {
	case ref.Kind == "tag":
		icon = lucide.Tag()
	case ref.Kind == "remote_branch":
		icon = lucide.Circle()
	case ref.synced:
		icon = lucide.CircleCheck()
	}
	ink, background, weight := chrome.muted, chrome.textMuted.Faded(.05), core.FontWeight(400)
	if emphasize {
		ink, background, weight = chrome.accent, chrome.accent.Faded(.1), 500
	}
	maxLines := 0
	if compact {
		maxLines = 1
	}
	return core.View{
		Label: w.tr("Reference ", "引用 ") + ref.Name, Tooltip: chrome.label(ref.Name, chrome.text, 11),
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, FlexShrink: 1,
			MinWidth: core.Pt(0), MaxWidth: core.Pct(100), Padding: core.Edges{Top: 2, Bottom: 2, Left: 6, Right: 6},
			Gap: 4, Background: background, CornerRadius: 4, Overflow: core.Hidden,
			FocusVisible: &core.Style{OutlineColor: chrome.accent, OutlineWidth: 1}},
		Children: []core.Node{core.Icon{SVG: icon, Style: core.Style{FontSize: 11, Color: ink}},
			core.Text{Content: name, MaxLines: maxLines, Style: core.Style{FontSize: chrome.uiSize(11), LineHeight: chrome.uiSize(11), Color: ink,
				FontWeight: weight, MinWidth: core.Pt(0), FlexShrink: 1}}}}
}

func (w *workbench) historyRefItems(sha string, ref commitRef, closeDialog func()) []core.MenuItem {
	info := w.repo.Get()
	pending := w.currentChangesWrite().pending
	writes := func(action func()) func() {
		return w.scopedRefAction(func() {
			closeDialog()
			action()
		})
	}
	items := []core.MenuItem{{Label: ref.Name, Disabled: true}, {Separator: true}}
	copyName := w.tr("Copy Branch Name", "复制分支名称")
	if ref.Kind == "tag" {
		copyName = w.tr("Copy Tag Name", "复制标签名")
		if i := slices.IndexFunc(info.Tags, func(tag tagRef) bool { return tag.Name == ref.Name && tag.Tip == sha }); i >= 0 {
			tag := info.Tags[i]
			items = append(items, core.MenuItem{Label: w.tr("Delete tag", "删除标签"), Disabled: pending,
				OnSelect: writes(func() { w.openRefForm(refDialogState{kind: deleteTagForm, tag: tag}) })})
		}
	} else {
		kind := "local"
		if ref.Kind == "remote_branch" {
			kind = "remote"
		}
		if i := slices.IndexFunc(info.Branches, func(b branch) bool { return b.Kind == kind && b.Name == ref.Name && b.Tip == sha }); i >= 0 {
			b := info.Branches[i]
			items = append(items, core.MenuItem{Label: w.tr("Checkout", "检出"), Disabled: pending || b.Current,
				OnSelect: writes(func() { w.checkoutBranch(b) })})
			if kind == "local" {
				items = append(items,
					core.MenuItem{Label: w.tr("Merge into current", "合并到当前分支"), Disabled: !w.canOpenMerge(b),
						OnSelect: writes(func() { w.openBranchMerge(b) })},
					core.MenuItem{Label: w.tr("Delete Branch…", "删除分支…"), Disabled: pending || b.Current,
						OnSelect: writes(func() {
							w.openRefForm(refDialogState{kind: deleteBranchForm, branch: b, base: b.Tip, baseLabel: b.Name})
						})})
			} else {
				items = append(items, core.MenuItem{Label: w.tr("Delete Remote Branch…", "删除远端分支…"),
					Disabled: pending || w.fetch.prompt.Get() != nil,
					OnSelect: writes(func() {
						w.openRefForm(refDialogState{kind: deleteRemoteBranchForm, branch: b, base: b.Tip, baseLabel: b.Name})
					})})
			}
		}
	}
	return append(items, core.MenuItem{Label: copyName, OnSelect: func() { w.cx.SetClipboard(ref.Name) }})
}
