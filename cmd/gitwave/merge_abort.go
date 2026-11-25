package main

import (
	"context"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type mergeAbortPrompt struct {
	child    *backend
	key      changesWriteKey
	plan     *gitcli.MergeAbortPlan
	failure  string
	checking bool
	cancel   context.CancelFunc
}

func (w *workbench) canAbortMerge() bool {
	return w.changes.Get().status.Commit.MergeHeads != "" && w.mergeAbort.Get() == nil &&
		!w.currentChangesWrite().pending && w.fetch.running.Get() == nil
}

func (w *workbench) requestAbortMerge() {
	if w.core == nil || !w.canAbortMerge() {
		return
	}
	ctx, cancel := context.WithCancel(w.repoCtx)
	prompt := &mergeAbortPrompt{child: w.core, key: w.changesWriteKey(), checking: true, cancel: cancel}
	w.mergeAbort.Set(prompt)
	go func() {
		plan, err := prompt.child.repo.PrepareMergeAbort(ctx)
		w.cx.Post(func() {
			defer cancel()
			if ctx.Err() != nil || !w.currentMergeAbort(prompt) {
				return
			}
			loaded := *prompt
			loaded.checking, loaded.plan = false, plan
			if err != nil {
				loaded.failure = changesWriteError(err)
			}
			w.mergeAbort.Set(&loaded)
		})
	}()
}

func (w *workbench) closeMergeAbort() {
	if prompt := w.mergeAbort.Get(); prompt != nil {
		prompt.cancel()
		w.mergeAbort.Set(nil)
	}
}

func (w *workbench) currentMergeAbort(prompt *mergeAbortPrompt) bool {
	return w.mergeAbort.Get() == prompt && w.core == prompt.child && w.changesWriteKey() == prompt.key
}

func (w *workbench) submitMergeAbort() {
	prompt := w.mergeAbort.Get()
	if prompt == nil || prompt.plan == nil || !w.currentMergeAbort(prompt) || w.currentChangesWrite().pending {
		return
	}
	w.beginChangesWrite(prompt.key)
	ctx := w.appCtx
	go func() {
		warning, err := prompt.plan.Apply(ctx)
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			w.setChangesWrite(prompt.key, changesWrite{})
			if !w.currentMergeAbort(prompt) {
				return
			}
			if err != nil {
				failed := *prompt
				failed.plan, failed.failure = nil, changesWriteError(err)
				w.mergeAbort.Set(&failed)
				if prompt.plan.SavesAutostash() {
					w.loadStashes()
				}
			} else {
				w.closeMergeAbort()
				w.closeConflicts()
				notice := w.tr("Merge aborted", "合并已中止")
				if prompt.plan.SavesAutostash() {
					notice = w.tr("Merge aborted · Autostash saved to Stashes", "合并已中止 · 自动贮藏已保存至贮藏列表")
				}
				if warning != nil {
					notice += " · " + warning.Error()
				}
				w.refNotice.Set(referenceNotice{text: notice, failed: warning != nil})
				w.refreshRepository(false)
			}
			w.loadChanges()
		})
	}()
}

func (w *workbench) mergeAbortButton(chrome palette, id string) core.View {
	label := w.tr("Abort merge", "中止合并")
	button := toolbarAction(chrome, label, w.requestAbortMerge,
		core.Icon{SVG: lucide.CircleX(), Style: core.Style{FontSize: 14, Color: chrome.danger}},
		chrome.label(label, chrome.danger, 12))
	button.TestID = id
	button.Disabled = func() bool { return !w.canAbortMerge() }
	return button
}

func (w *workbench) mergeAbortDialog(chrome palette) core.Node {
	pending := func() bool { return w.currentChangesWrite().pending }
	close := func() {
		if !pending() {
			w.closeMergeAbort()
		}
	}
	cancelLabel := w.tr("Cancel", "取消")
	cancel := toolbarAction(chrome, cancelLabel, close, chrome.label(cancelLabel, chrome.text, 12))
	cancel.TestID, cancel.Disabled = "cancel-merge-abort", pending
	confirmLabel := w.tr("Abort merge", "中止合并")
	confirm := toolbarAction(chrome, confirmLabel, w.submitMergeAbort, chrome.label(confirmLabel, chrome.danger, 12))
	confirm.TestID = "confirm-merge-abort"
	confirm.Disabled = func() bool {
		prompt := w.mergeAbort.Get()
		return prompt == nil || prompt.plan == nil || pending()
	}
	return ui.Dialog{TestID: "merge-abort-dialog", Role: core.RoleAlertDialog,
		Open: func() bool { return w.mergeAbort.Get() != nil }, NoCloseOnBackdropPress: true,
		Title: w.tr("Abort merge?", "中止合并？"),
		Description: w.tr("This discards pending merge changes and unsaved conflict drafts. Independent changes are kept.",
			"此操作将丢弃本次合并的改动与未保存的冲突草稿，保留独立的改动。"),
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				close()
			}
		},
		Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			if prompt := w.mergeAbort.Get(); prompt != nil {
				if prompt.failure != "" {
					return core.Text{TestID: "merge-abort-error", Role: core.RoleAlert, Content: prompt.failure,
						Style: core.Style{Color: chrome.danger, FontSize: chrome.uiSize(12)}}
				}
				if prompt.checking {
					return chrome.label(w.tr("Checking merge…", "正在检查合并…"), chrome.muted, 12)
				}
				if prompt.plan != nil && prompt.plan.SavesAutostash() {
					return core.Text{Content: w.tr(
						"Changes saved by Git before the merge will be kept in Stashes. You can restore them after aborting.",
						"Git 在合并前保存的改动会保留在贮藏列表中。中止后可手动恢复。"),
						Style: core.Style{Color: chrome.text, FontSize: chrome.uiSize(12)}}
				}
			}
			return core.View{Style: core.Style{Display: core.DisplayNone}}
		})},
		Footer: []core.Node{cancel, confirm},
	}
}
