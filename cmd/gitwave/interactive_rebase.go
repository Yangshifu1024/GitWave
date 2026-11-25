package main

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
)

type interactiveRebaseForm struct {
	child    *backend
	key      changesWriteKey
	upstream string
	plan     *gitcli.InteractiveRebasePlan
}

type interactiveRebaseUI struct {
	interactiveForm     *core.Signal[*interactiveRebaseForm]
	interactiveTodos    *core.Signal[[]gitcli.RebaseTodo]
	interactiveMessages *core.Signal[map[string]string]
	interactiveError    *core.Signal[string]
}

func newInteractiveRebaseUI(cx *core.Scope) interactiveRebaseUI {
	return interactiveRebaseUI{
		interactiveForm:     core.NewSignal[*interactiveRebaseForm](cx, nil),
		interactiveTodos:    core.NewSignalFunc(cx, []gitcli.RebaseTodo(nil), slices.Equal),
		interactiveMessages: core.NewSignalFunc(cx, map[string]string(nil), maps.Equal),
		interactiveError:    core.NewSignal(cx, ""),
	}
}

func (w *workbench) openInteractiveRebase(branch branch, checkout repository) {
	child := w.core
	if child == nil || !w.canRebaseBranch(branch, checkout) {
		return
	}
	form := &interactiveRebaseForm{child: child, key: w.changesWriteKey(), upstream: branch.Name}
	w.interactiveError.Set("")
	w.interactiveTodos.Set(nil)
	w.interactiveMessages.Set(nil)
	w.interactiveForm.Set(form)
	ctx := w.repoCtx
	local := gitcli.CheckoutTarget{Ref: "refs/heads/" + checkout.Branch, OID: checkout.Head}
	upstream := gitcli.CheckoutTarget{Ref: "refs/heads/" + branch.Name, OID: branch.Tip}
	go func() {
		plan, err := child.repo.PrepareInteractiveRebase(ctx, upstream, local)
		w.cx.Post(func() {
			if ctx.Err() != nil || w.interactiveForm.Get() != form || w.core != child || w.changesWriteKey() != form.key {
				return
			}
			if err != nil {
				w.interactiveError.Set(changesWriteError(err))
				return
			}
			w.interactiveTodos.Set(plan.Todos())
			w.interactiveForm.Set(&interactiveRebaseForm{child: child, key: form.key, upstream: form.upstream, plan: plan})
		})
	}()
}

func (w *workbench) canStartInteractiveRebase() bool {
	form := w.interactiveForm.Get()
	if form == nil || form.plan == nil || w.core != form.child || w.changesWriteKey() != form.key ||
		w.currentChangesWrite().pending || len(w.changes.Get().status.Changes) != 0 {
		return false
	}
	todos := w.interactiveTodos.Get()
	first := slices.IndexFunc(todos, func(todo gitcli.RebaseTodo) bool { return todo.Action != gitcli.RebaseDrop })
	return len(todos) != 0 && (first == -1 || todos[first].Action != gitcli.RebaseSquash && todos[first].Action != gitcli.RebaseFixup)
}

func (w *workbench) moveInteractiveTodo(oid string, target int) {
	todos := w.interactiveTodos.Get()
	from := slices.IndexFunc(todos, func(todo gitcli.RebaseTodo) bool { return todo.OID == oid })
	if w.currentChangesWrite().pending || from < 0 || target < 0 || target >= len(todos) || target == from {
		return
	}
	next := slices.Clone(todos)
	item := next[from]
	next = slices.Delete(next, from, from+1)
	w.interactiveTodos.Set(slices.Insert(next, target, item))
}

func (w *workbench) updateInteractiveTodo(oid string, update func(*gitcli.RebaseTodo)) {
	todos := w.interactiveTodos.Get()
	index := slices.IndexFunc(todos, func(todo gitcli.RebaseTodo) bool { return todo.OID == oid })
	if w.currentChangesWrite().pending || index < 0 {
		return
	}
	next := slices.Clone(todos)
	update(&next[index])
	w.interactiveTodos.Set(next)
}

func (w *workbench) submitInteractiveRebase() {
	if !w.canStartInteractiveRebase() {
		return
	}
	form, todos := w.interactiveForm.Get(), slices.Clone(w.interactiveTodos.Get())
	messages := w.interactiveMessages.Get()
	for i := range todos {
		todos[i].Message = messages[todos[i].OID]
	}
	w.runInteractiveRebase(form.child, form.key, form, false, func(ctx context.Context) (gitcli.InteractiveRebaseResult, error) {
		return form.plan.Apply(ctx, todos)
	})
}

func (w *workbench) finishInteractivePause(abort bool) {
	child, key := w.core, w.changesWriteKey()
	pause := w.changes.Get().status.Commit.InteractiveRebase
	if child == nil || pause == "" || !w.canSync() || w.fetch.prompt.Get() != nil {
		return
	}
	w.runInteractiveRebase(child, key, nil, abort, func(ctx context.Context) (gitcli.InteractiveRebaseResult, error) {
		if abort {
			return gitcli.InteractiveRebaseResult{}, child.repo.AbortInteractiveRebase(ctx, pause)
		}
		return child.repo.ContinueInteractiveRebase(ctx, pause)
	})
}

func (w *workbench) runInteractiveRebase(child *backend, key changesWriteKey, form *interactiveRebaseForm, abort bool,
	run func(context.Context) (gitcli.InteractiveRebaseResult, error)) {
	w.beginChangesWrite(key)
	w.workspaceNotice.Set("")
	w.refNotice.Set(referenceNotice{})
	w.interactiveError.Set("")
	ctx := w.appCtx
	go func() {
		result, err := run(ctx)
		var info repository
		if err == nil {
			info, _ = child.loadRepository(ctx)
		}
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			active := w.core == child && w.changesWriteKey() == key
			if active && info.Head != "" {
				w.repo.Set(info)
			}
			w.setChangesWrite(key, changesWrite{})
			if !active {
				return
			}
			if form != nil && w.interactiveForm.Get() == form {
				if err != nil {
					w.interactiveError.Set(changesWriteError(err))
				} else {
					w.interactiveForm.Set(nil)
				}
			} else if err != nil {
				w.refNotice.Set(referenceNotice{text: changesWriteError(err), failed: true})
			}
			if err == nil {
				w.refNotice.Set(w.interactiveRebaseNotice(result, abort))
			}
			w.loadChanges()
			w.refreshRepository(false)
		})
	}()
}

func (w *workbench) interactiveRebaseNotice(result gitcli.InteractiveRebaseResult, abort bool) referenceNotice {
	if len(result.Conflicts) != 0 {
		message := w.tr("Interactive rebase hit conflicts: %s", "交互式变基遇到冲突：%s")
		if result.Paused {
			message = w.tr("Rebase remains paused. Conflicting files: %s", "变基仍处于暂停状态。冲突文件：%s")
		}
		return referenceNotice{text: fmt.Sprintf(message, strings.Join(result.Conflicts, ", ")), failed: true}
	}
	if result.Paused {
		return referenceNotice{text: w.tr("Paused for edit. Stage changes in Working Copy, then continue from Branches.",
			"已暂停以供编辑。在工作副本中暂存改动后，从分支面板继续。")}
	}
	if abort {
		return referenceNotice{text: w.tr("Aborted interactive rebase", "已中止交互式变基")}
	}
	return referenceNotice{text: w.tr("Interactive rebase completed", "交互式变基已完成")}
}
