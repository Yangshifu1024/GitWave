package main

import (
	"context"
	"fmt"
	"slices"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

type discardConfirmation struct {
	key     changesWriteKey
	files   []gitcli.Change
	stop    context.CancelFunc
	plan    *gitcli.DiscardPlan
	failure string
}

func discardableFiles(files []workingChange) []gitcli.Change {
	var result []gitcli.Change
	for _, file := range files {
		if !file.key.staged && file.change.CanDiscard() {
			result = append(result, file.change)
		}
	}
	return result
}

func (w *workbench) discardMenuAction(files []gitcli.Change) func() {
	child, key := w.core, w.changesWriteKey()
	return func() {
		if w.core == child && w.changesWriteKey() == key {
			w.openDiscard(files)
		}
	}
}

func (w *workbench) openDiscard(files []gitcli.Change) {
	child, shown, key := w.core, w.changes.Get(), w.changesWriteKey()
	if child == nil || !w.changesMode.Get() || !shown.ready || len(files) == 0 ||
		w.currentChangesWrite().pending || w.discardPrompt.Get() != nil {
		return
	}
	ctx, stop := context.WithCancel(w.repoCtx)
	prompt := &discardConfirmation{key: key, files: files, stop: stop}
	w.discardPrompt.Set(prompt)
	go func() {
		plan, err := child.repo.PrepareDiscard(ctx, shown.status, files)
		w.cx.Post(func() {
			defer stop()
			if ctx.Err() != nil || w.discardPrompt.Get() != prompt {
				return
			}
			loaded := *prompt
			loaded.plan = plan
			if err != nil {
				loaded.failure = changesWriteError(err)
			}
			w.discardPrompt.Set(&loaded)
		})
	}()
}

func (w *workbench) cancelDiscard() {
	if prompt := w.discardPrompt.Get(); prompt != nil {
		prompt.stop()
		w.discardPrompt.Set(nil)
	}
}

func (w *workbench) canConfirmDiscard(prompt *discardConfirmation) bool {
	return prompt != nil && prompt.plan != nil && prompt.key == w.changesWriteKey() &&
		!w.currentChangesWrite().pending && w.changesMode.Get()
}

func (w *workbench) confirmDiscard() {
	prompt := w.discardPrompt.Get()
	if !w.canConfirmDiscard(prompt) {
		return
	}
	w.cancelDiscard()
	w.beginChangesWrite(prompt.key)
	ctx := w.appCtx
	go func() {
		err := prompt.plan.Apply(ctx)
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			var result changesWrite
			if err != nil {
				result.err = changesWriteError(err)
			}
			w.setChangesWrite(prompt.key, result)
			if w.changesWriteKey() == prompt.key {
				w.loadChanges()
			}
		})
	}()
}

func (w *workbench) discardDialog(cx *core.Scope, chrome palette) core.Node {
	caption := core.NewMemo(cx, func() discardCaption {
		if prompt := w.discardPrompt.Get(); prompt != nil {
			return w.describeDiscard(prompt.files)
		}
		return discardCaption{}
	})
	label := func() string { return caption.Get().action }
	confirm := toolbarAction(chrome, "", w.confirmDiscard, core.Text{ContentFunc: label,
		Style: core.Style{Color: core.RGB(0xffffff), FontSize: chrome.uiSize(12)}})
	confirm.TestID, confirm.LabelFunc = "confirm-discard", label
	confirm.Disabled = func() bool { return !w.canConfirmDiscard(w.discardPrompt.Get()) }
	confirm.Style.Background = chrome.danger
	confirm.Style.Hover = &core.Style{Background: chrome.danger, Opacity: core.Alpha(.88)}
	confirm.Style.Disabled = &core.Style{Opacity: core.Alpha(.4)}
	return ui.Dialog{
		TestID: "discard-confirmation", Role: core.RoleAlertDialog,
		Open:            func() bool { return w.discardPrompt.Get() != nil },
		TitleFunc:       func() string { return caption.Get().title },
		DescriptionFunc: func() string { return caption.Get().description },
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				w.cancelDiscard()
			}
		},
		Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			prompt := w.discardPrompt.Get()
			if prompt == nil || prompt.plan != nil {
				return core.View{Style: core.Style{Display: core.DisplayNone}}
			}
			if prompt.failure != "" {
				return core.Text{Content: prompt.failure, Role: core.RoleAlert, Style: core.Style{Color: chrome.danger, FontSize: chrome.uiSize(12)}}
			}
			return chrome.label(w.tr("Checking selected files…", "正在检查选中的文件…"), chrome.muted, 12)
		})},
		Footer: []core.Node{action(chrome, w.tr("Cancel", "取消"), w.cancelDiscard), confirm},
	}
}

type discardCaption struct{ title, description, action string }

func (w *workbench) describeDiscard(files []gitcli.Change) discardCaption {
	if len(files) == 1 {
		path := files[0].Path
		title := fmt.Sprintf(w.tr("Discard changes in %q?", "丢弃“%s”中的变更？"), path)
		if files[0].Kind == '?' {
			description := fmt.Sprintf(w.tr("%q is untracked. Discarding permanently deletes this file from disk.",
				"“%s”未被 Git 跟踪。丢弃将把该文件从磁盘永久删除。"), path)
			return discardCaption{title, description, w.tr("Delete file", "删除文件")}
		}
		description := fmt.Sprintf(w.tr("Uncommitted changes in %q will be restored to the last staged version. This cannot be undone.",
			"“%s”中未提交的变更将恢复到上次暂存的版本。此操作无法撤销。"), path)
		return discardCaption{title, description, w.tr("Discard", "丢弃")}
	}
	description := w.tr("Uncommitted changes in %d files will be restored to their last staged version. This cannot be undone.",
		"将把 %d 个文件中未提交的变更恢复到上次暂存的版本。此操作无法撤销。")
	if slices.ContainsFunc(files, func(file gitcli.Change) bool { return file.Kind == '?' }) {
		description = w.tr("Discarding %d files will restore tracked files to their last staged version and permanently delete untracked files.",
			"将丢弃 %d 个文件。已跟踪的文件恢复到上次暂存的版本，未跟踪的文件将从磁盘永久删除。")
	}
	return discardCaption{
		fmt.Sprintf(w.tr("Discard changes in %d files?", "丢弃 %d 个文件中的变更？"), len(files)),
		fmt.Sprintf(description, len(files)),
		fmt.Sprintf(w.tr("Discard %d files", "丢弃 %d 个文件"), len(files)),
	}
}
