package main

import (
	"slices"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
)

type conflictSession struct {
	child   *backend
	key     changesWriteKey
	drafts  map[string]conflictDraft
	request uint64
}

type conflictDraft struct {
	file   *gitcli.Conflict
	seed   string
	text   string
	choice gitcli.ConflictChoice
}

func (d conflictDraft) dirty() bool { return d.text != d.seed || d.choice != gitcli.ConflictText }

type conflictsUI struct {
	conflictPanel   *core.Signal[*conflictSession]
	conflictFile    *core.Signal[string]
	conflictDraft   *core.Signal[*conflictDraft]
	conflictError   *core.Signal[string]
	conflictLoading *core.Signal[bool]
	conflictDiscard *core.Signal[bool]
	conflictMarkers *core.Signal[bool]
	conflictHunk    *core.Signal[int]
	conflictAI      aiAdvice
}

func newConflictsUI(cx *core.Scope) conflictsUI {
	return conflictsUI{
		conflictPanel:   core.NewSignal[*conflictSession](cx, nil),
		conflictFile:    core.NewSignal(cx, ""),
		conflictDraft:   core.NewSignal[*conflictDraft](cx, nil),
		conflictError:   core.NewSignal(cx, ""),
		conflictLoading: core.NewSignal(cx, false),
		conflictDiscard: core.NewSignal(cx, false),
		conflictMarkers: core.NewSignal(cx, false),
		conflictHunk:    core.NewSignal(cx, 0),
		conflictAI:      newAIAdvice(cx),
	}
}

func (w *workbench) openConflicts(path string) {
	if w.core == nil || len(w.changes.Get().conflicts) == 0 || w.currentChangesWrite().pending || w.refForm.Get() != nil ||
		w.checkoutPrompt.Get() != (checkoutPrompt{}) || w.workspaceBusy.Get() || w.workspaceReload.Get() ||
		w.workspaceModal.Get().kind != "" || w.fetch.prompt.Get() != nil {
		return
	}
	if w.conflictPanel.Get() == nil {
		w.conflictPanel.Set(&conflictSession{child: w.core, key: w.changesWriteKey(), drafts: make(map[string]conflictDraft)})
	}
	if path != "" {
		w.selectConflict(path)
	}
}

func (w *workbench) closeConflicts() {
	w.conflictAI.clear()
	w.conflictPanel.Set(nil)
	w.conflictFile.Set("")
	w.conflictDraft.Set(nil)
	w.conflictError.Set("")
	w.conflictLoading.Set(false)
	w.conflictDiscard.Set(false)
	w.conflictMarkers.Set(false)
	w.conflictHunk.Set(0)
}

func (w *workbench) requestCloseConflicts() {
	session := w.conflictPanel.Get()
	if session == nil || w.currentChangesWrite().pending || w.mergeAbort.Get() != nil {
		return
	}
	for _, draft := range session.drafts {
		if draft.dirty() {
			w.conflictDiscard.Set(true)
			return
		}
	}
	w.closeConflicts()
}

func (w *workbench) selectConflict(path string) {
	session := w.conflictPanel.Get()
	if session == nil || w.currentChangesWrite().pending {
		return
	}
	shown := w.changes.Get()
	var selected gitcli.Change
	for _, file := range shown.conflicts {
		if file.key.path == path {
			selected = file.change
			break
		}
	}
	draft, cached := session.drafts[path]
	if selected.Path == "" && !(cached && draft.dirty()) {
		return
	}
	if cached && !draft.dirty() && !draft.file.MatchesStatus(shown.status) {
		delete(session.drafts, path)
		cached = false
	}
	w.conflictAI.clear()
	session.request++
	request := session.request
	w.conflictFile.Set(path)
	w.conflictError.Set("")
	w.conflictHunk.Set(0)
	if cached {
		w.conflictDraft.Set(&draft)
		w.conflictLoading.Set(false)
		if selected.Path == "" {
			w.conflictError.Set(w.tr("This conflict was resolved on disk. You can still copy the unsaved edits.", "此冲突已在磁盘上解决。仍可复制尚未保存的编辑。"))
		} else if !draft.file.MatchesStatus(shown.status) {
			w.reportChangedConflict()
		}
		return
	}
	w.conflictDraft.Set(nil)
	w.conflictLoading.Set(true)
	ctx := w.repoCtx
	go func() {
		file, err := session.child.repo.OpenConflict(ctx, shown.status, selected)
		w.cx.Post(func() {
			if ctx.Err() != nil || w.conflictPanel.Get() != session || session.request != request {
				return
			}
			w.conflictLoading.Set(false)
			if err != nil {
				w.conflictError.Set(changesWriteError(err))
				return
			}
			seed := file.Working.Text
			if file.Working.Kind == "missing" {
				seed = file.Ours.Text
				if file.Ours.Kind == "missing" {
					seed = file.Theirs.Text
				}
			}
			draft := conflictDraft{file: file, seed: seed, text: seed}
			session.drafts[path] = draft
			w.conflictDraft.Set(&draft)
		})
	}()
}

func (w *workbench) conflictPaths() []string {
	var paths []string
	present := make(map[string]bool)
	for _, file := range w.changes.Get().conflicts {
		path := file.key.path
		paths = append(paths, path)
		present[path] = true
	}
	if session := w.conflictPanel.Get(); session != nil {
		for path, draft := range session.drafts {
			if !present[path] && draft.dirty() {
				paths = append(paths, path)
			}
		}
	}
	slices.Sort(paths)
	return paths
}

func (w *workbench) updateConflict(file *gitcli.Conflict, choice gitcli.ConflictChoice, text string) {
	session, draft := w.conflictPanel.Get(), w.conflictDraft.Get()
	if session == nil || draft == nil || draft.file != file || w.currentChangesWrite().pending || !file.MatchesStatus(w.changes.Get().status) {
		return
	}
	next := *draft
	next.choice, next.text = choice, text
	session.drafts[next.file.Path] = next
	w.conflictDraft.Set(&next)
}

func (w *workbench) saveConflict() {
	if draft := w.conflictDraft.Get(); draft != nil && draft.choice == gitcli.ConflictText &&
		len(gitcli.ConflictRegions(draft.text, draft.file.MarkerSize)) != 0 {
		w.conflictMarkers.Set(true)
		return
	}
	w.applyConflict()
}

func (w *workbench) applyConflict() {
	w.conflictMarkers.Set(false)
	session, draft := w.conflictPanel.Get(), w.conflictDraft.Get()
	if session == nil || draft == nil || w.currentChangesWrite().pending ||
		!draft.file.MatchesStatus(w.changes.Get().status) || (draft.choice == gitcli.ConflictText && !draft.file.CanEdit()) {
		return
	}
	w.conflictAI.clear()
	w.beginChangesWrite(session.key)
	w.conflictError.Set("")
	ctx := w.appCtx
	go func() {
		err := draft.file.Resolve(ctx, draft.choice, draft.text)
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			w.setChangesWrite(session.key, changesWrite{})
			if w.core != session.child || w.changesWriteKey() != session.key {
				return
			}
			if w.conflictPanel.Get() == session {
				if err == nil {
					delete(session.drafts, draft.file.Path)
					w.conflictFile.Set("")
					w.conflictDraft.Set(nil)
				} else {
					w.conflictError.Set(changesWriteError(err))
				}
			}
			w.loadChanges()
		})
	}()
}

func (w *workbench) reconcileConflictPanel(changes workingChanges) {
	session := w.conflictPanel.Get()
	if session == nil {
		return
	}
	if len(changes.conflicts) == 0 {
		w.conflictAI.clear()
		if w.conflictDraft.Get() != nil && w.conflictError.Get() != "" {
			return
		}
		for _, draft := range session.drafts {
			if draft.dirty() {
				w.conflictError.Set(w.tr("The conflicts were resolved on disk. Copy any unsaved edits before closing.", "磁盘上的冲突已解决。请在关闭前复制尚未保存的编辑。"))
				return
			}
		}
		w.closeConflicts()
	} else if draft := w.conflictDraft.Get(); draft != nil && !draft.file.MatchesStatus(changes.status) {
		w.conflictAI.clear()
		if !w.isCurrentConflict(draft.file.Path) {
			w.conflictError.Set(w.tr("This conflict was resolved on disk. Copy any unsaved edits before selecting another file.", "此冲突已在磁盘上解决。请选择其他文件前先复制尚未保存的编辑。"))
		} else if draft.dirty() {
			w.reportChangedConflict()
		} else {
			w.selectConflict(draft.file.Path)
		}
	}
}

func (w *workbench) reportChangedConflict() {
	w.conflictError.Set(w.tr("The index stages changed. Copy any unsaved edits, then reopen the conflict to load the new stages.",
		"索引阶段已变化。请先复制未保存的编辑，再重新打开冲突以加载最新阶段。"))
}

func (w *workbench) isCurrentConflict(path string) bool {
	for _, conflict := range w.changes.Get().conflicts {
		if conflict.key.path == path {
			return true
		}
	}
	return false
}
