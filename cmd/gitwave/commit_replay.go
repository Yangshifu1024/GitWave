package main

import (
	"errors"
	"fmt"
	"strings"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

func (w *workbench) commitReplayItems(sha, subject string, contextMenu bool) []core.MenuItem {
	checkout := w.repo.Get()
	target := gitcli.CheckoutTarget{OID: checkout.Head}
	for _, branch := range checkout.Branches {
		if branch.Kind == "local" && branch.Current {
			target.Ref = "refs/heads/" + branch.Name
			break
		}
	}
	canOpen := func() bool {
		return sha != "" && target.OID != "" && w.canSync() && w.fetch.prompt.Get() == nil &&
			w.changes.Get().status.Commit == (gitcli.CommitState{})
	}
	menu := func(kind refFormKind, label string) core.MenuItem {
		return core.MenuItem{Label: label, Disabled: !canOpen(), OnSelect: w.scopedRefAction(func() {
			if canOpen() {
				w.openRefForm(refDialogState{kind: kind, base: sha, baseLabel: subject, target: target})
			}
		})}
	}
	pick, revert := w.tr("Cherry-pick", "拣选"), w.tr("Revert", "撤销")
	if contextMenu {
		pick, revert = w.tr("Cherry-pick Commit…", "拣选提交…"), w.tr("Revert Commit…", "撤销提交…")
	}
	reset := core.MenuItem{Label: w.tr("Reset branch here…", "重置分支到此处…"), Disabled: target.Ref == "" || !canOpen(),
		OnSelect: w.scopedRefAction(func() { w.requestHardReset(sha) })}
	return []core.MenuItem{menu(cherryPickForm, pick), menu(revertCommitForm, revert), reset}
}

func (w *workbench) commitMoreButton(chrome palette, sha, subject string) core.View {
	ref := new(core.Ref)
	label := w.tr("More actions", "更多操作")
	open := func() {
		if bounds, ok := ref.Bounds(); ok {
			at := bounds.Origin
			at.Y += bounds.Size.Height
			items := []core.MenuItem{{Label: w.tr("Tag", "标签"), OnSelect: w.scopedRefAction(func() { w.showTagManager(sha) })}}
			w.cx.PopupMenu(append(items, w.commitReplayItems(sha, subject, false)...), at, nil)
		}
	}
	button := toolbarAction(chrome, label, open, chrome.label(label, chrome.text, 12),
		core.Icon{SVG: lucide.ChevronDown(), Style: core.Style{FontSize: 12, Color: chrome.muted}})
	button.Ref, button.TestID = ref, "commit-more-actions"
	button.Disabled = func() bool { return w.currentChangesWrite().pending || sha == "" }
	return button
}

func (w *workbench) describeCommitReplay(form refDialogState) refDialogCopy {
	sha := shortSHA(form.base)
	target := strings.TrimPrefix(form.target.Ref, "refs/heads/")
	if target == "" {
		target = w.tr("detached HEAD", "分离的 HEAD")
	}
	if form.kind == revertCommitForm {
		return refDialogCopy{
			title: fmt.Sprintf(w.tr("Revert %s?", "撤销 %s？"), sha),
			description: fmt.Sprintf(w.tr("Create a commit on %s that undoes %q. The working copy must be clean.",
				"在 %s 上创建一个撤销“%s”的提交。工作副本必须干净。"), target, form.baseLabel),
			confirm: w.tr("Revert", "撤销"), destructive: true,
		}
	}
	return refDialogCopy{
		title: fmt.Sprintf(w.tr("Cherry-pick %s?", "拣选 %s？"), sha),
		description: fmt.Sprintf(w.tr("Apply %q as a new commit on %s, preserving its author. The working copy must be clean.",
			"将“%s”作为新提交应用到 %s，并保留原作者。工作副本必须干净。"), form.baseLabel, target),
		confirm: w.tr("Cherry-pick", "拣选"),
	}
}

func (w *workbench) refActionFailure(err error) string {
	var conflict *gitcli.CommitReplayConflicts
	if errors.As(err, &conflict) {
		message := w.tr("Cherry-pick hit conflicts in %s", "拣选时遇到冲突：%s")
		if conflict.Action == gitcli.RevertCommit {
			message = w.tr("Revert hit conflicts in %s", "撤销时遇到冲突：%s")
		}
		return fmt.Sprintf(message, strings.Join(conflict.Paths, ", "))
	}
	return changesWriteError(err)
}
