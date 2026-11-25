package main

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
)

type refFormKind uint8

const (
	createBranchForm refFormKind = iota + 1
	renameBranchForm
	deleteBranchForm
	deleteRemoteBranchForm
	trackingForm
	tagManagerForm
	replaceTagForm
	deleteTagForm
	mergeBranchForm
	cherryPickForm
	revertCommitForm
	recoveryBranchForm
	resetBranchForm
)

type refDialogState struct {
	kind         refFormKind
	key          changesWriteKey
	child        *backend
	branch       branch
	tag          tagRef
	base         string
	baseLabel    string
	merge        *gitcli.MergePlan
	reset        *gitcli.CheckoutPlan
	target       gitcli.CheckoutTarget
	checking     bool
	remoteDelete *gitcli.RemoteBranchDeletePlan
}

type referenceNotice struct {
	text   string
	failed bool
}

type refUI struct {
	refForm          *core.Signal[*refDialogState]
	refName          *core.Signal[string]
	refAnnotation    *core.Signal[string]
	refUpstream      *core.Signal[string]
	refFailure       *core.Signal[string]
	refNotice        *core.Signal[referenceNotice]
	tagsOpen         *core.Signal[bool]
	mergeMode        *core.Signal[string]
	alsoDeleteRemote *core.Signal[bool]
}

func newRefUI(cx *core.Scope) refUI {
	return refUI{
		refForm: core.NewSignal[*refDialogState](cx, nil),
		refName: core.NewSignal(cx, ""), refAnnotation: core.NewSignal(cx, ""),
		refUpstream: core.NewSignal(cx, ""), refFailure: core.NewSignal(cx, ""),
		refNotice: core.NewSignal(cx, referenceNotice{}), tagsOpen: core.NewSignal(cx, false), mergeMode: core.NewSignal(cx, ""),
		alsoDeleteRemote: core.NewSignal(cx, false),
	}
}

func (w *workbench) openRefForm(form refDialogState) {
	if w.core == nil || w.currentChangesWrite().pending || w.refForm.Get() != nil {
		return
	}
	form.key, form.child = w.changesWriteKey(), w.core
	name, upstream := "", ""
	if form.kind == renameBranchForm {
		name = form.branch.Name
	}
	if form.kind == recoveryBranchForm {
		name = "recovery-" + shortSHA(form.base)
	}
	if form.kind == trackingForm {
		upstream = form.branch.UpstreamRef
	}
	w.refName.Set(name)
	w.refAnnotation.Set("")
	w.refUpstream.Set(upstream)
	w.refFailure.Set("")
	w.refNotice.Set(referenceNotice{})
	w.workspaceNotice.Set("")
	w.alsoDeleteRemote.Set(false)
	loadDeletion := form.kind == deleteBranchForm || form.kind == deleteRemoteBranchForm
	form.checking = form.checking || loadDeletion
	w.refForm.Set(&form)
	if loadDeletion {
		w.loadRemoteBranchDeletion(&form)
	}
}

func (w *workbench) selectTag(tag tagRef) {
	child := w.core
	if child == nil || tag.Tip == "" {
		return
	}
	w.locate = ""
	w.showChanges(false)
	w.selectCommit(tag.Tip)
	id := w.selectionID
	ctx := w.repoCtx
	go func() {
		if child.historyContains(tag.Tip) {
			w.cx.Post(func() {
				if ctx.Err() == nil && w.core == child && w.selectionID == id {
					w.locateCommit(tag.Tip)
				}
			})
		}
	}()
}

func (w *workbench) scopedRefAction(action func()) func() {
	child, key := w.core, w.changesWriteKey()
	return func() {
		if child != nil && w.core == child && w.changesWriteKey() == key {
			action()
		}
	}
}

func (w *workbench) cancelRefForm() {
	form := w.refForm.Get()
	if form == nil || w.currentChangesWrite().pending {
		return
	}
	if form.base != "" && (form.kind == replaceTagForm || form.kind == deleteTagForm) {
		manager := *form
		manager.kind, manager.tag = tagManagerForm, tagRef{}
		w.refFailure.Set("")
		w.refForm.Set(&manager)
		return
	}
	w.refForm.Set(nil)
}

func (w *workbench) canSubmitRefForm() bool {
	form := w.refForm.Get()
	if form == nil || w.core != form.child || w.changesWriteKey() != form.key || w.currentChangesWrite().pending {
		return false
	}
	switch form.kind {
	case deleteRemoteBranchForm:
		return !form.checking && form.remoteDelete != nil && w.fetch.prompt.Get() == nil && w.canSyncRepository()
	case deleteBranchForm:
		return !w.alsoDeleteRemote.Get() || form.remoteDelete != nil && w.fetch.prompt.Get() == nil && w.canSyncRepository()
	case createBranchForm, recoveryBranchForm, renameBranchForm, tagManagerForm:
		return strings.TrimSpace(w.refName.Get()) != ""
	case mergeBranchForm:
		return form.merge != nil && !form.merge.UpToDate && form.merge.DirtyFiles == 0
	case resetBranchForm:
		return form.reset != nil
	default:
		return true
	}
}

func (w *workbench) submitRefForm() {
	if !w.canSubmitRefForm() {
		return
	}
	form := w.refForm.Get()
	if form.kind == deleteRemoteBranchForm || form.kind == deleteBranchForm && w.alsoDeleteRemote.Get() {
		w.refForm.Set(nil)
		w.runSync(syncRequest{remoteDelete: form.remoteDelete})
		return
	}
	if form.kind == mergeBranchForm {
		w.applyBranchMerge(form)
		return
	}
	if form.kind == resetBranchForm {
		w.submitHardReset(form)
		return
	}
	name, annotation, upstream := strings.TrimSpace(w.refName.Get()), w.refAnnotation.Get(), w.refUpstream.Get()
	if form.kind == tagManagerForm {
		tags := w.repo.Get().Tags
		if i := slices.IndexFunc(tags, func(tag tagRef) bool { return tag.Name == name }); i >= 0 {
			replace := *form
			replace.kind, replace.tag = replaceTagForm, tags[i]
			w.refFailure.Set("")
			w.refForm.Set(&replace)
			return
		}
	}
	w.applyRefForm(form, name, annotation, upstream)
}

func (w *workbench) applyRefForm(form *refDialogState, name, annotation, upstream string) {
	w.refFailure.Set("")
	if form.kind == cherryPickForm || form.kind == revertCommitForm {
		w.beginChangesWrite(form.key)
	} else {
		w.setChangesWrite(form.key, changesWrite{pending: true})
	}
	ctx := w.appCtx
	go func() {
		err := form.apply(ctx, name, annotation, upstream)
		var info repository
		var loadErr error
		if err == nil {
			info, loadErr = form.child.loadRepository(ctx)
		}
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			w.setChangesWrite(form.key, changesWrite{})
			if w.changesWriteKey() != form.key {
				return
			}
			if err == nil && loadErr == nil && w.core == form.child {
				w.repo.Set(info)
			}
			if w.refForm.Get() == form {
				if err != nil {
					w.refFailure.Set(w.refActionFailure(err))
				} else if form.kind == deleteTagForm && form.base != "" {
					w.cancelRefForm()
					w.refNotice.Set(referenceNotice{text: w.refSuccess(*form, name, upstream)})
				} else {
					w.refForm.Set(nil)
					w.refNotice.Set(referenceNotice{text: w.refSuccess(*form, name, upstream)})
				}
			}
			w.refreshRepository(false)
		})
	}()
}

func (form refDialogState) apply(ctx context.Context, name, annotation, upstream string) error {
	repo, source := form.child.repo, form.branch
	switch form.kind {
	case createBranchForm, recoveryBranchForm:
		return repo.CreateBranch(ctx, name, form.base)
	case renameBranchForm:
		return repo.RenameBranch(ctx, source.Name, name, source.Tip)
	case deleteBranchForm:
		return repo.DeleteBranch(ctx, source.Name, source.Tip)
	case trackingForm:
		return repo.SetBranchUpstream(ctx, source.Name, source.Tip, source.UpstreamRef, upstream)
	case tagManagerForm:
		return repo.CreateTag(ctx, name, form.base, annotation, "")
	case replaceTagForm:
		return repo.CreateTag(ctx, form.tag.Name, form.base, annotation, form.tag.Object)
	case deleteTagForm:
		return repo.DeleteTag(ctx, form.tag.Name, form.tag.Object)
	case cherryPickForm, revertCommitForm:
		action := gitcli.CherryPickCommit
		if form.kind == revertCommitForm {
			action = gitcli.RevertCommit
		}
		_, err := repo.ReplayCommit(ctx, action, form.base, form.target)
		return err
	default:
		return fmt.Errorf("unknown reference action")
	}
}

func (w *workbench) refSuccess(form refDialogState, name, upstream string) string {
	switch form.kind {
	case createBranchForm:
		return fmt.Sprintf(w.tr("Created branch %s", "已创建分支 %s"), name)
	case recoveryBranchForm:
		return fmt.Sprintf(w.tr("Created recovery branch %s", "已创建恢复分支 %s"), name)
	case renameBranchForm:
		return fmt.Sprintf(w.tr("Renamed %s to %s", "已将 %s 重命名为 %s"), form.branch.Name, name)
	case deleteBranchForm:
		return fmt.Sprintf(w.tr("Deleted branch %s", "已删除分支 %s"), form.branch.Name)
	case trackingForm:
		if upstream == "" {
			return fmt.Sprintf(w.tr("Upstream cleared for %s", "已清除 %s 的 upstream"), form.branch.Name)
		}
		return fmt.Sprintf(w.tr("%s now tracks %s", "%s 现在跟踪 %s"), form.branch.Name, strings.TrimPrefix(upstream, "refs/remotes/"))
	case tagManagerForm, replaceTagForm:
		return fmt.Sprintf(w.tr("Tagged %s as %s", "已为 %s 添加标签 %s"), shortSHA(form.base), name)
	case deleteTagForm:
		return fmt.Sprintf(w.tr("Deleted tag %s", "已删除标签 %s"), form.tag.Name)
	case cherryPickForm:
		return fmt.Sprintf(w.tr("Cherry-picked %s", "已拣选 %s"), shortSHA(form.base))
	case revertCommitForm:
		return fmt.Sprintf(w.tr("Reverted %s", "已撤销 %s"), shortSHA(form.base))
	}
	return ""
}
