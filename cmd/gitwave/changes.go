package main

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"time"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
)

type changeKey struct {
	path   string
	staged bool
}

type workingChange struct {
	key    changeKey
	change gitcli.Change
	move   gitcli.WorkingMove
}

func (c workingChange) kind() byte {
	switch {
	case c.change.Kind == '?', c.change.Kind == 'u':
		return c.change.Kind
	case c.key.staged:
		return c.change.Index
	default:
		return c.change.WorkTree
	}
}

func (c workingChange) pathLabel() string {
	if c.change.PreviousPath != "" && (c.kind() == 'R' || c.kind() == 'C') {
		return c.change.PreviousPath + " → " + c.key.path
	}
	return c.key.path
}

type workingChanges struct {
	ready                       bool
	bare                        bool
	err                         string
	status                      gitcli.Status
	unstaged, staged, conflicts []workingChange
}

func groupChanges(status gitcli.Status, moves ...gitcli.WorkingMove) workingChanges {
	result := workingChanges{ready: true, status: status}
	paired := make(map[changeKey]bool, 2*len(moves))
	for _, move := range moves {
		change := move.Change()
		key := changeKey{path: change.Path, staged: move.Staged()}
		paired[changeKey{path: move.Source.Path, staged: key.staged}], paired[key] = true, true
		item := workingChange{key: key, change: change, move: move}
		if key.staged {
			result.staged = append(result.staged, item)
		} else {
			result.unstaged = append(result.unstaged, item)
		}
	}
	for _, change := range status.Changes {
		entry := workingChange{key: changeKey{path: change.Path}, change: change}
		switch change.Kind {
		case 'u':
			result.conflicts = append(result.conflicts, entry)
		case '?':
			if !paired[entry.key] {
				result.unstaged = append(result.unstaged, entry)
			}
		default:
			if change.WorkTree != '.' && !paired[entry.key] {
				result.unstaged = append(result.unstaged, entry)
			}
			entry.key.staged = true
			if change.Index != '.' && !paired[entry.key] {
				result.staged = append(result.staged, entry)
			}
		}
	}
	for _, group := range [][]workingChange{result.unstaged, result.staged, result.conflicts} {
		slices.SortFunc(group, func(a, b workingChange) int {
			return strings.Compare(a.key.path, b.key.path)
		})
	}
	return result
}

func (s workingChanges) find(key changeKey) workingChange {
	if key.path == "" {
		return workingChange{}
	}
	groups := [][]workingChange{s.unstaged, s.staged}
	if key.staged {
		groups[0], groups[1] = groups[1], groups[0]
	}
	for _, group := range append(groups, s.conflicts) {
		if i := slices.IndexFunc(group, func(c workingChange) bool {
			return c.key.path == key.path || c.move.Source.Path == key.path
		}); i != -1 {
			return group[i]
		}
	}
	return workingChange{}
}

type workingDiffState struct {
	result   preview
	err      string
	loading  bool
	expanded bool
}

type changesSession struct {
	active  bool
	file    changeKey
	history historySelection
}

type changesScope struct {
	workspace, repo, path string
}

type changesUI struct {
	commitAIUI
	changesMode         *core.Signal[bool]
	changes             *core.Signal[workingChanges]
	changeFile          *core.Signal[workingChange]
	changeDiff          *core.Signal[workingDiffState]
	changeCollapsed     *core.Signal[bool]
	changeLists         map[string]*changeListState
	workingDiffViewport diffViewport
	changesScope        changesScope
	changesSessions     map[changesScope]changesSession
	changesWrites       *core.Signal[map[changesWriteKey]changesWrite]
	commitDrafts        *core.Signal[map[changesWriteKey]commitDraft]
	amendPrompt         *core.Signal[*amendConfirmation]
	discardPrompt       *core.Signal[*discardConfirmation]
	stopChanges         context.CancelFunc
	stopChangeDiff      context.CancelFunc
	stopChangesTimer    func() bool
	readingChange       bool
}

type changeListState struct {
	selection *core.Signal[changeSelection]
	focused   *core.Signal[string]
	offset    core.Point
}

func newChangesUI(cx *core.Scope) changesUI {
	lists := make(map[string]*changeListState, 3)
	for _, name := range []string{"staged", "unstaged", "conflicts"} {
		lists[name] = &changeListState{selection: core.NewSignalFunc(cx, changeSelection{}, nil), focused: core.NewSignal(cx, "")}
	}
	return changesUI{
		commitAIUI:  newCommitAIUI(cx),
		changesMode: core.NewSignal(cx, false), changes: core.NewSignalFunc(cx, workingChanges{}, nil),
		changeFile: core.NewSignal(cx, workingChange{}), changeDiff: core.NewSignalFunc(cx, workingDiffState{}, nil),
		changeCollapsed: core.NewSignal(cx, false), changesSessions: make(map[changesScope]changesSession),
		changesWrites: core.NewSignalFunc(cx, make(map[changesWriteKey]changesWrite), nil),
		commitDrafts:  core.NewSignalFunc(cx, make(map[changesWriteKey]commitDraft), nil),
		amendPrompt:   core.NewSignal[*amendConfirmation](cx, nil),
		discardPrompt: core.NewSignal[*discardConfirmation](cx, nil),
		changeLists:   lists,
	}
}

func (w *workbench) clearChangesNavigation() {
	for _, list := range w.changeLists {
		list.offset = core.Point{}
		list.selection.Set(changeSelection{})
		list.focused.Set("")
	}
	w.workingDiffViewport = diffViewport{}
}

func (w *workbench) saveChangesSession() {
	if w.changesScope != (changesScope{}) {
		history := historySelection{repoPath: w.path.Get(), sha: w.selected.Get(), file: w.file.Get()}
		previous := w.changesSessions[w.changesScope].history
		if previous.repoPath == history.repoPath {
			// A user can switch away again before the remembered details load.
			if w.core == nil {
				history = previous
			} else if history.sha == previous.sha && (w.detailLoading.Get() || w.detailError.Get() != "") {
				history.file = previous.file
			}
		}
		w.changesSessions[w.changesScope] = changesSession{active: w.changesMode.Get(), file: w.changeFile.Get().key, history: history}
	}
}

func (w *workbench) bindChanges(path string) {
	state := w.workspaceState.Get()
	scope := changesScope{workspace: state.Workspace.ID, repo: state.Repo.ID}
	if scope.repo == "" {
		scope.path = path
	}
	w.changesScope = scope
	session := w.changesSessions[scope]
	w.changesMode.Set(session.active)
	w.focus.Set(false)
	w.changeFile.Set(workingChange{key: session.file})
}

func (w *workbench) closeChanges() {
	w.saveChangesSession()
	w.cancelCommitAI()
	w.amendPrompt.Set(nil)
	w.cancelDiscard()
	w.changesScope = changesScope{}
	if w.stopChanges != nil {
		w.stopChanges()
		w.stopChanges = nil
	}
	if w.stopChangesTimer != nil {
		w.stopChangesTimer()
		w.stopChangesTimer = nil
	}
	w.cancelChangeDiff()
	w.changesMode.Set(false)
	w.changes.Set(workingChanges{})
	w.changeFile.Set(workingChange{})
	w.changeDiff.Set(workingDiffState{})
	w.changeCollapsed.Set(false)
	w.clearChangesNavigation()
}

func (w *workbench) cancelChangeDiff() {
	if w.stopChangeDiff != nil {
		w.stopChangeDiff()
		w.stopChangeDiff = nil
	}
	w.readingChange = false
}

func (w *workbench) showChanges(active bool) {
	if w.path.Get() == "" {
		return
	}
	w.focus.Set(false)
	if active == w.changesMode.Get() {
		return
	}
	w.closeBlame()
	w.changesMode.Set(active)
	if !active {
		w.cancelCommitAI()
		w.amendPrompt.Set(nil)
		w.cancelDiscard()
		w.cancelChangeDiff()
		return
	}
	if item := w.changeFile.Get(); w.changes.Get().ready && item.key.path != "" {
		w.loadChangeDiff(item, w.changeDiff.Get().expanded, false)
	}
	w.loadChanges()
}

func (w *workbench) loadChanges() {
	child := w.core
	if child == nil || w.stopChanges != nil || w.currentChangesWrite().pending {
		return
	}
	if child.repo.WorkTree == "" {
		w.changes.Set(workingChanges{ready: true, bare: true})
		return
	}
	ctx, stop := context.WithCancel(w.repoCtx)
	w.stopChanges = stop
	go func() {
		status, err := child.repo.Status(ctx)
		var result workingChanges
		if err == nil {
			moves := child.repo.WorkingMoves(ctx, status)
			moves = append(moves, child.repo.StagedMoves(ctx, status)...)
			result = groupChanges(status, moves...)
		}
		w.cx.Post(func() {
			defer stop()
			if ctx.Err() != nil {
				return
			}
			w.stopChanges = nil
			if w.core != child {
				w.loadChanges()
				return
			}
			if err != nil {
				result = workingChanges{err: err.Error()}
			}
			previous := w.changes.Get()
			if !reflect.DeepEqual(previous, result) {
				w.changes.Set(result)
				if err == nil && previous.ready && w.historyError.Get() == "" {
					w.loadHealth(child)
				}
			}
			if err == nil {
				w.reconcileWorkingChanges(result)
				w.reconcileConflictPanel(result)
			}
		})
	}()
}

func (w *workbench) watchChanges() {
	w.loadChanges()
	w.scheduleChangesRefresh()
}

func (w *workbench) scheduleChangesRefresh() {
	if w.core == nil || w.core.repo.WorkTree == "" || w.stopChangesTimer != nil {
		return
	}
	w.stopChangesTimer = w.cx.AfterFunc(2*time.Second, func() {
		w.stopChangesTimer = nil
		if w.cx.Env().Lifecycle() == core.LifecycleActive || w.conflictPanel.Get() != nil {
			w.loadChanges()
		}
		w.scheduleChangesRefresh()
	})
}

func (w *workbench) reconcileWorkingChanges(status workingChanges) {
	current := w.changeFile.Get()
	next := status.find(current.key)
	if current.key != next.key {
		w.selectChange(next)
		return
	}
	w.changeFile.Set(next)
	if w.changesMode.Get() && next.key.path != "" {
		w.loadChangeDiff(next, w.changeDiff.Get().expanded, true)
	}
}

func (w *workbench) selectChange(item workingChange) {
	w.cancelChangeDiff()
	w.closeBlame()
	if w.changeFile.Get().key != item.key {
		w.workingDiffViewport = diffViewport{}
	}
	w.changeFile.Set(item)
	w.changeCollapsed.Set(false)
	w.changeDiff.Set(workingDiffState{})
	if w.changesMode.Get() && item.key.path != "" && item.kind() != 'u' {
		w.loadChangeDiff(item, false, false)
	}
}

func (w *workbench) loadChangeDiff(item workingChange, expanded, refresh bool) {
	child := w.core
	if child == nil || item.key.path == "" || item.kind() == 'u' || (refresh && w.readingChange) {
		return
	}
	w.cancelChangeDiff()
	ctx, stop := context.WithCancel(w.repoCtx)
	w.stopChangeDiff = stop
	w.readingChange = true
	if !refresh {
		w.changeDiff.Set(workingDiffState{result: w.changeDiff.Get().result, loading: true, expanded: expanded})
	}
	go func() {
		var result preview
		var err error
		if item.move.Source.Path != "" {
			result, err = child.repo.WorkingMoveDiff(ctx, item.move, expanded)
		} else {
			result, err = child.repo.WorkingDiff(ctx, item.change, item.key.staged, expanded)
		}
		w.cx.Post(func() {
			defer stop()
			if ctx.Err() != nil || w.core != child {
				return
			}
			w.stopChangeDiff = nil
			w.readingChange = false
			current := w.changeDiff.Get()
			if current.loading && err == nil && reflect.DeepEqual(current.result, result) {
				// Reuse the same hunks so the remounted preview can restore measured widths.
				result = current.result
			}
			updated := workingDiffState{result: result, expanded: expanded}
			if err != nil {
				updated.err = err.Error()
			}
			if !reflect.DeepEqual(current, updated) {
				w.changeDiff.Set(updated)
			}
		})
	}()
}
