package main

import (
	"fmt"
	"slices"
	"strings"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
)

type reflogUI struct {
	recoveryOpen   *core.Signal[bool]
	recoveryBranch *core.Signal[bool]
	reflog         *core.Signal[reflogState]
	reflogSelected *core.Signal[gitcli.ReflogEntry]
	reflogReadID   uint64
	reflogAI       aiAdvice
}

type reflogState struct {
	reference string
	entries   []gitcli.ReflogEntry
	loading   bool
	err       string
}

func newReflogUI(cx *core.Scope) reflogUI {
	return reflogUI{
		recoveryOpen: core.NewSignal(cx, false), recoveryBranch: core.NewSignal(cx, false),
		reflog:         core.NewSignalFunc(cx, reflogState{}, nil),
		reflogSelected: core.NewSignal(cx, gitcli.ReflogEntry{}),
		reflogAI:       newAIAdvice(cx),
	}
}

func (w *workbench) closeReflog() {
	w.reflogAI.clear()
	w.reflogReadID++
	w.recoveryOpen.Set(false)
	w.recoveryBranch.Set(false)
	w.reflog.Set(reflogState{})
	w.reflogSelected.Set(gitcli.ReflogEntry{})
}

func (w *workbench) reflogLocalBranch() branch {
	for _, b := range w.repo.Get().Branches {
		if b.Current && b.Kind == "local" {
			return b
		}
	}
	return branch{}
}

func (w *workbench) loadReflog() {
	child := w.core
	if child == nil || !w.recoveryOpen.Get() {
		return
	}
	ref := "HEAD"
	if branch := w.reflogLocalBranch(); w.recoveryBranch.Get() && branch.Name != "" {
		ref = "refs/heads/" + branch.Name
	}
	if w.reflog.Get().reference != ref {
		w.reflogAI.clear()
		w.reflogSelected.Set(gitcli.ReflogEntry{})
		w.reflog.Set(reflogState{reference: ref, loading: true})
	}
	w.reflogReadID++
	id, ctx := w.reflogReadID, w.repoCtx
	go func() {
		entries, err := child.repo.Reflog(ctx, ref)
		w.cx.Post(func() {
			if ctx.Err() != nil || w.core != child || w.reflogReadID != id {
				return
			}
			if err != nil {
				w.reflogAI.clear()
				w.reflog.Set(reflogState{reference: ref, err: err.Error()})
				w.reflogSelected.Set(gitcli.ReflogEntry{})
				return
			}
			previous := w.reflog.Get()
			if previous.loading || previous.err != "" || !slices.Equal(previous.entries, entries) {
				w.reflog.Set(reflogState{reference: ref, entries: entries})
			}
			if !slices.Contains(entries, w.reflogSelected.Get()) {
				w.reflogAI.clear()
				w.reflogSelected.Set(gitcli.ReflogEntry{})
			}
		})
	}()
}

func (w *workbench) requestHardReset(oid string) {
	local := w.reflogLocalBranch()
	if local.Tip == "" || oid == "" || !w.canSync() {
		return
	}
	target := gitcli.CheckoutTarget{Ref: "refs/heads/" + local.Name, OID: local.Tip}
	w.openRefForm(refDialogState{kind: resetBranchForm, branch: local, base: oid, target: target, checking: true})
	form := w.refForm.Get()
	if form == nil || form.kind != resetBranchForm {
		return
	}
	ctx := w.repoCtx
	go func() {
		plan, err := form.child.repo.PrepareHardReset(ctx, target, oid)
		w.cx.Post(func() {
			if ctx.Err() != nil || w.refForm.Get() != form || w.core != form.child || w.changesWriteKey() != form.key {
				return
			}
			loaded := *form
			loaded.checking, loaded.reset = false, plan
			w.refForm.Set(&loaded)
			if err != nil {
				w.refFailure.Set(changesWriteError(err))
			}
		})
	}()
}

func (w *workbench) submitHardReset(form *refDialogState) {
	w.beginChangesWrite(form.key)
	ctx := w.appCtx
	go func() {
		result, err := form.reset.ApplyHardReset(ctx)
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			w.setChangesWrite(form.key, changesWrite{})
			if w.core != form.child || w.changesWriteKey() != form.key {
				return
			}
			if w.refForm.Get() == form {
				if err != nil {
					w.refFailure.Set(err.Error())
				} else {
					w.refForm.Set(nil)
					w.reflogSelected.Set(gitcli.ReflogEntry{})
					message := fmt.Sprintf(w.tr("Branch reset to %s", "已将分支重置到 %s"), shortSHA(result.OID))
					if strings.TrimSpace(result.Warning) != "" {
						message += " · " + result.Warning
					}
					w.refNotice.Set(referenceNotice{text: message, failed: result.Warning != ""})
				}
			}
			w.loadChanges()
			w.refreshRepository(false)
		})
	}()
}
