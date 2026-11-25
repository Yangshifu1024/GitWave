package main

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
)

type worktreeUI struct {
	worktreeList      *core.Signal[[]gitcli.Worktree]
	worktreeOpen      *core.Signal[bool]
	worktreeError     *core.Signal[string]
	worktreeListError *core.Signal[string]
	worktreeForm      *core.Signal[*worktreeCreateForm]
	worktreeRemove    *core.Signal[*worktreeRemoval]
	worktreeName      *core.Signal[string]
	worktreePath      *core.Signal[string]
	worktreeStart     *core.Signal[string]
	worktreeReadID    uint64
}

type worktreeCreateForm struct {
	child   *backend
	key     changesWriteKey
	targets map[string]gitcli.CheckoutTarget
	options []string
}

type worktreeRemoval struct {
	child   *backend
	key     changesWriteKey
	entry   gitcli.Worktree
	plan    gitcli.WorktreeRemovePlan
	loading bool
	ready   bool
}

func newWorktreeUI(cx *core.Scope) worktreeUI {
	return worktreeUI{
		worktreeList: core.NewSignalFunc(cx, []gitcli.Worktree(nil), nil), worktreeOpen: core.NewSignal(cx, false),
		worktreeError: core.NewSignal(cx, ""), worktreeForm: core.NewSignal[*worktreeCreateForm](cx, nil),
		worktreeListError: core.NewSignal(cx, ""),
		worktreeRemove:    core.NewSignal[*worktreeRemoval](cx, nil), worktreeName: core.NewSignal(cx, ""),
		worktreePath: core.NewSignal(cx, ""), worktreeStart: core.NewSignal(cx, ""),
	}
}

func (w *workbench) clearWorktrees() {
	w.worktreeReadID++
	w.worktreeList.Set(nil)
	w.worktreeOpen.Set(false)
	w.worktreeError.Set("")
	w.worktreeListError.Set("")
	w.worktreeForm.Set(nil)
	w.worktreeRemove.Set(nil)
}

func (w *workbench) loadWorktrees() {
	child, ctx := w.core, w.repoCtx
	if child == nil {
		return
	}
	w.worktreeReadID++
	id := w.worktreeReadID
	go func() {
		list, err := child.repo.Worktrees(ctx)
		list = slices.DeleteFunc(list, func(item gitcli.Worktree) bool { return item.Provisional })
		w.cx.Post(func() {
			if ctx.Err() != nil || w.core != child || w.worktreeReadID != id {
				return
			}
			if err != nil {
				w.worktreeListError.Set(changesWriteError(err))
				return
			}
			w.worktreeListError.Set("")
			previous := w.worktreeList.Get()
			if len(list) == 0 || (len(previous) > 1) != (len(list) > 1) {
				w.worktreeOpen.Set(len(list) > 1)
			}
			if !slices.Equal(previous, list) {
				w.worktreeList.Set(list)
			}
		})
	}()
}

func (w *workbench) canManageWorktrees() bool {
	return w.core != nil && w.canSync() && w.worktreeForm.Get() == nil && w.worktreeRemove.Get() == nil && w.remoteForm.Get() == nil
}

func (w *workbench) newWorktreeMenuItem() core.MenuItem {
	head := w.repo.Get().Head
	return core.MenuItem{Label: w.tr("New worktree", "新建工作树"),
		Disabled: head == "" || !w.canManageWorktrees(),
		OnSelect: w.scopedRefAction(w.openWorktreeForm)}
}

func (w *workbench) openWorktreeForm() {
	info := w.repo.Get()
	if !w.canManageWorktrees() || info.Head == "" {
		return
	}
	form := &worktreeCreateForm{child: w.core, key: w.changesWriteKey(), targets: make(map[string]gitcli.CheckoutTarget)}
	for _, branch := range info.Branches {
		if branch.Kind == "local" && branch.Tip != "" {
			form.targets[branch.Name] = gitcli.CheckoutTarget{Ref: "refs/heads/" + branch.Name, OID: branch.Tip}
			form.options = append(form.options, branch.Name)
		}
	}
	start := info.Branch
	if _, found := form.targets[start]; !found {
		start = "HEAD (" + shortSHA(info.Head) + ")"
		form.targets[start] = gitcli.CheckoutTarget{OID: info.Head}
		form.options = append([]string{start}, form.options...)
	}
	w.worktreeStart.Set(start)
	w.worktreeName.Set("")
	w.worktreePath.Set("")
	w.worktreeError.Set("")
	w.worktreeForm.Set(form)
}

func (w *workbench) canCreateWorktree() bool {
	form := w.worktreeForm.Get()
	return form != nil && form.child == w.core && form.key == w.changesWriteKey() && w.canSync() &&
		form.targets[w.worktreeStart.Get()].OID != "" && strings.TrimSpace(w.worktreePath.Get()) != "" &&
		strings.TrimSpace(w.worktreeName.Get()) != ""
}

func (w *workbench) createWorktree() {
	if !w.canCreateWorktree() {
		return
	}
	form := w.worktreeForm.Get()
	request := gitcli.WorktreeAddRequest{Name: strings.TrimSpace(w.worktreeName.Get()),
		Destination: strings.TrimSpace(w.worktreePath.Get()), Start: form.targets[w.worktreeStart.Get()]}
	w.runWorktreeWrite(form.child, form.key, func(ctx context.Context) (string, error) {
		result, err := form.child.repo.AddWorktree(ctx, request)
		return result.Warning, err
	}, func() { w.worktreeForm.Set(nil) }, fmt.Sprintf(w.tr("Created worktree %s", "已创建工作树 %s"), request.Name))
}

func (w *workbench) openSelectedWorktree(entry gitcli.Worktree) {
	if entry.Current || entry.Prunable || !w.canManageWorktrees() {
		return
	}
	child, ctx := w.core, w.repoCtx
	w.worktreeError.Set("")
	if store := w.workspaceStore; store != nil {
		ws := w.workspaceState.Get().Workspace.ID
		w.runWorkspace(func(ctx context.Context) (workspaceResult, error) {
			target, err := child.repo.OpenWorktree(ctx, entry)
			if err != nil {
				return workspaceResult{}, err
			}
			id, err := store.SelectOrAddRepo(ctx, ws, target.Path())
			return workspaceResult{workspaceID: ws, repoID: id}, err
		})
		return
	}
	go func() {
		target, err := child.repo.OpenWorktree(ctx, entry)
		w.cx.Post(func() {
			if ctx.Err() != nil || child != w.core || !w.canManageWorktrees() {
				return
			}
			if err != nil {
				w.worktreeError.Set(changesWriteError(err))
				return
			}
			w.open(target.Path())
		})
	}()
}

func (w *workbench) requestWorktreeRemoval(entry gitcli.Worktree) {
	if entry.Main || entry.Current || entry.Locked || !w.canManageWorktrees() {
		return
	}
	prompt := &worktreeRemoval{child: w.core, key: w.changesWriteKey(), entry: entry, loading: true}
	w.worktreeError.Set("")
	w.worktreeRemove.Set(prompt)
	ctx := w.repoCtx
	go func() {
		plan, err := prompt.child.repo.PrepareWorktreeRemoval(ctx, entry)
		w.cx.Post(func() {
			if ctx.Err() != nil || w.worktreeRemove.Get() != prompt || w.core != prompt.child {
				return
			}
			ready := *prompt
			ready.plan, ready.loading, ready.ready = plan, false, err == nil
			if err != nil {
				w.worktreeError.Set(changesWriteError(err))
			}
			w.worktreeRemove.Set(&ready)
		})
	}()
}

func (w *workbench) canRemoveWorktree() bool {
	prompt := w.worktreeRemove.Get()
	return prompt != nil && prompt.ready && w.core == prompt.child && w.changesWriteKey() == prompt.key && w.canSync()
}

func (w *workbench) removeSelectedWorktree() {
	if !w.canRemoveWorktree() {
		return
	}
	prompt := w.worktreeRemove.Get()
	w.runWorktreeWrite(prompt.child, prompt.key, func(ctx context.Context) (string, error) {
		result, err := prompt.plan.Remove(ctx)
		return result.Warning, err
	}, func() { w.worktreeRemove.Set(nil) }, fmt.Sprintf(w.tr("Removed worktree %s", "已移除工作树 %s"), prompt.entry.Name))
}

func (w *workbench) runWorktreeWrite(child *backend, key changesWriteKey, action func(context.Context) (string, error),
	closeDialog func(), success string) {
	w.worktreeError.Set("")
	w.setChangesWrite(key, changesWrite{pending: true})
	ctx := w.appCtx
	go func() {
		warning, err := action(ctx)
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			w.setChangesWrite(key, changesWrite{})
			if w.core != child || w.changesWriteKey() != key {
				return
			}
			if err != nil {
				w.worktreeError.Set(changesWriteError(err))
			} else {
				closeDialog()
				message := success
				if warning != "" {
					message += ". " + warning
				}
				w.refNotice.Set(referenceNotice{text: message})
				w.queueRefresh(refreshManual)
			}
			w.loadWorktrees()
		})
	}()
}
