package main

import (
	"slices"

	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/core"
)

type refreshReason uint8

const (
	refreshIdle refreshReason = iota
	refreshForeground
	refreshManual
	refreshAutomatic
	refreshRemoteManual
)

type refreshJob struct {
	reason   refreshReason
	repo     changesWriteKey
	ws       string
	schedule uint64
}

func (w *workbench) queueRefresh(reason refreshReason) {
	job := refreshJob{reason: reason}
	if reason == refreshRemoteManual {
		job.repo = w.changesWriteKey()
	} else if reason == refreshAutomatic {
		job.ws = w.workspaceState.Get().Workspace.ID
		job.schedule = w.refreshSchedule
	}
	w.enqueueRefresh(job)
}

func (w *workbench) enqueueRefresh(job refreshJob) {
	if job.reason >= w.refreshRequest.Get().reason && w.sameRefreshTarget(job) {
		w.refreshRequest.Set(job)
	}
}

func (w *workbench) sameRefreshTarget(job refreshJob) bool {
	switch job.reason {
	case refreshRemoteManual:
		return job.repo == w.changesWriteKey()
	case refreshAutomatic:
		return job.ws == w.workspaceState.Get().Workspace.ID && job.schedule == w.refreshSchedule
	default:
		return true
	}
}

func (w *workbench) watchRefresh() {
	last := w.cx.Env().Lifecycle()
	w.cx.Effect(func() {
		phase := w.cx.Env().Lifecycle()
		if phase == core.LifecycleActive && phase != last {
			w.queueRefresh(refreshForeground)
		}
		last = phase
		job := w.refreshRequest.Get()
		reason := job.reason
		if reason == refreshIdle || (reason == refreshForeground && phase != core.LifecycleActive) || w.refreshRunning.Get() || w.refreshBlocked() {
			return
		}
		w.cx.Untrack(func() {
			w.refreshRequest.Set(refreshJob{})
			if w.sameRefreshTarget(job) {
				w.refreshWorkbench(job)
			}
		})
	})
}

func (w *workbench) refreshBlocked() bool {
	return w.workspaceBusy.Get() || w.workspaceModal.Get().kind != "" || w.refForm.Get() != nil ||
		w.checkoutPrompt.Get() != (checkoutPrompt{}) || w.fetch.running.Get() != nil || w.fetch.prompt.Get() != nil ||
		w.pull.form.Get() != nil || w.push.form.Get() != nil || w.push.review.Get() != nil || w.paletteForm.Get() != nil ||
		w.refreshSettingsOpen.Get() || w.aboutOpen.Get() || w.aiSettings.Get() != nil || w.commitAIResult.Get().setup ||
		w.healthAI.result.Get().text != "" || w.longReflogAdvice() || w.aiDraft.Get() != nil ||
		w.conflictPanel.Get() != nil || w.mergeAbort.Get() != nil || w.interactiveForm.Get() != nil ||
		w.worktreeForm.Get() != nil || w.worktreeRemove.Get() != nil || w.remoteForm.Get() != nil
}

func (w *workbench) refreshWorkbench(job refreshJob) {
	reason := job.reason
	store := w.workspaceStore
	if store == nil {
		w.refreshRepository(reason >= refreshManual)
		w.refreshFromRemotes(job, workspace.State{})
		return
	}
	state := w.workspaceState.Get()
	target := &workspaceSelection{state.Workspace.ID, state.Repo.ID}
	if w.workspaceReload.Get() {
		target = w.workspaceRetry
	}
	action := w.workspaceAction
	ctx := w.appCtx
	w.refreshRunning.Set(true)
	go func() {
		state, err := loadWindowWorkspace(ctx, store, target)
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			w.refreshRunning.Set(false)
			if action != w.workspaceAction ||
				(reason == refreshForeground && w.cx.Env().Lifecycle() != core.LifecycleActive) ||
				w.refreshBlocked() {
				w.enqueueRefresh(job)
				return
			}
			if err != nil {
				w.workspaceError.Set(err.Error())
				return
			}
			w.workspaceReload.Set(false)
			w.workspaceRetry = nil
			w.workspaceError.Set("")
			previous := w.repoCtx
			w.applyWorkspace(state, false)
			if previous == w.repoCtx {
				w.refreshRepository(reason >= refreshManual)
			}
			w.refreshFromRemotes(job, state)
		})
	}()
}

func (w *workbench) refreshRepository(force bool) {
	child := w.core
	if child == nil {
		if path := w.path.Get(); path != "" && !w.historyLoading.Get() {
			w.open(path)
		}
		return
	}
	force = force || w.historyLoading.Get() || w.historyError.Get() != ""
	loaded := len(w.commits.Get())
	path, saved := w.path.Get(), w.workspaceStore != nil
	ctx, stop, id := w.startHistoryRequest()
	go func() {
		openBackend := startBackend
		if saved {
			openBackend = startWorkspaceBackend
		}
		current, err := openBackend(ctx, path)
		var info repository
		var page *historyPage
		if err == nil {
			if child.repo.SameCheckout(current.repo) {
				current = child
			}
			info, page, err = current.refreshHistory(ctx, "", loaded, force)
		}
		w.cx.Post(func() {
			defer stop()
			if ctx.Err() != nil || id != w.historyID {
				return
			}
			w.historyLoading.Set(false)
			if err != nil {
				if current == nil || current != child {
					w.clearHealth()
				}
				w.loadHealth(current)
				w.historyError.Set(err.Error())
				return
			}
			previous := w.repo.Get()
			w.core = current
			if current != child || previous.Head != info.Head || previous.Branch != info.Branch || previous.Shallow != info.Shallow ||
				!slices.Equal(previous.Branches, info.Branches) ||
				!slices.Equal(previous.Tags, info.Tags) || !slices.Equal(previous.Remotes, info.Remotes) {
				w.repo.Set(info)
			}
			w.historyError.Set("")
			if current != child {
				w.closeCommandPalette()
				w.clearStashes()
				w.clearWorktrees()
				w.clearRemotes()
				w.clearSubmodules()
				w.closeLFS()
				w.closeRepoEditors()
				w.closeReflog()
				w.clearHealth()
				if w.stopChanges != nil {
					w.stopChanges()
					w.stopChanges = nil
				}
				w.cancelChangeDiff()
				w.changes.Set(workingChanges{})
				w.changeFile.Set(workingChange{})
				w.changeDiff.Set(workingDiffState{})
				w.changeCollapsed.Set(false)
				w.clearChangesNavigation()
				w.clearSelection()
				w.locate = ""
				w.historyOffset = core.Point{}
				w.publishHistoryPage(*page, true)
				w.historyView.ScrollToIndex(0, false)
				w.historyGraphView.ScrollTo(core.Point{}, false)
			} else if page != nil {
				w.replaceRefreshedHistory(*page)
			} else if w.locate != "" {
				w.finishLocate()
			}
			w.watchChanges()
			w.loadStashes()
			w.loadWorktrees()
			w.loadRemotes()
			w.loadSubmodules()
			w.loadReflog()
			w.loadHealth(current)
			if w.selected.Get() != "" && w.detailError.Get() != "" {
				w.retryCommitDetails()
			} else if path := w.file.Get(); path != "" && w.diffError.Get() != "" {
				w.selectFile(path, w.expanded.Get())
			}
		})
	}()
}

func (w *workbench) replaceRefreshedHistory(page historyPage) {
	var anchor string
	rows := w.commits.Get()
	top := int(w.historyOffset.Y / historyRowHeight)
	withinRow := w.historyOffset.Y - core.Points(top*historyRowHeight)
	if w.locate == "" && w.historyOffset.Y > 0 && top < len(rows) {
		anchor = rows[top].SHA
	}
	w.publishHistoryPage(page, true)
	if anchor == "" {
		return
	}
	if index := slices.IndexFunc(page.Commits, func(c commit) bool { return c.SHA == anchor }); index >= 0 {
		// History has no mounted list while Changes is visible.
		w.historyOffset.Y = core.Points(index*historyRowHeight) + withinRow
		w.historyView.ScrollToIndexAt(index, 0, -withinRow, false)
	}
}
