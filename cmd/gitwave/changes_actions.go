package main

import (
	"errors"
	"maps"
	"os/exec"
	"strings"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
)

type changesWriteKey struct {
	scope changesScope
	path  string
}

type changesWrite struct {
	pending bool
	err     string
	commit  string
	warning string
}

func (w *workbench) changesWriteKey() changesWriteKey {
	return changesWriteKey{scope: w.changesScope, path: w.path.Get()}
}

func (w *workbench) currentChangesWrite() changesWrite {
	return w.changesWrites.Get()[w.changesWriteKey()]
}

func (w *workbench) setChangesWrite(key changesWriteKey, value changesWrite) {
	writes := maps.Clone(w.changesWrites.Get())
	if value == (changesWrite{}) {
		delete(writes, key)
	} else {
		writes[key] = value
	}
	w.changesWrites.Set(writes)
}

func (w *workbench) beginChangesWrite(key changesWriteKey) {
	w.cancelCommitAI()
	w.setChangesWrite(key, changesWrite{pending: true})
	if w.stopChanges != nil {
		w.stopChanges()
		w.stopChanges = nil
	}
	w.cancelChangeDiff()
	w.closeBlame()
}

func changesWriteError(err error) string {
	var exit *exec.ExitError
	if errors.As(err, &exit) && len(exit.Stderr) != 0 {
		return strings.TrimSpace(string(exit.Stderr))
	}
	return err.Error()
}

func (c workingChange) canWriteIndex() bool {
	return c.key.staged || !c.change.SubmoduleFilesOnly()
}

func (w *workbench) writeStageSelection(files []workingChange) bool {
	child := w.core
	shown := w.changes.Get()
	key := w.changesWriteKey()
	if len(files) == 0 || child == nil || !shown.ready || w.changesWrites.Get()[key].pending {
		return false
	}
	stage := !files[0].key.staged
	targets := make([]gitcli.Change, 0, len(files))
	var moves []gitcli.WorkingMove
	for _, file := range files {
		if !file.canWriteIndex() {
			continue
		}
		if file.move.Source.Path != "" {
			moves = append(moves, file.move)
		} else {
			targets = append(targets, file.change)
		}
	}
	if len(targets) == 0 && len(moves) == 0 {
		return false
	}
	w.beginChangesWrite(key)
	ctx := w.appCtx
	go func() {
		err := child.repo.SetStaged(ctx, shown.status, targets, stage, moves...)
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			var result changesWrite
			if err != nil {
				result.err = changesWriteError(err)
			}
			w.setChangesWrite(key, result)
			if w.changesWriteKey() == key {
				w.loadChanges()
			}
		})
	}()
	return true
}

type changeSelection struct {
	paths  map[string]bool
	anchor string
}

func selectWorkingFiles(items []workingChange, current changeSelection, path string, mods core.Modifiers) changeSelection {
	if mods.Shift {
		from, to := -1, -1
		for i, item := range items {
			if item.key.path == current.anchor {
				from = i
			}
			if item.key.path == path {
				to = i
			}
		}
		if from != -1 && to != -1 {
			paths := make(map[string]bool)
			for _, item := range items[min(from, to) : max(from, to)+1] {
				paths[item.key.path] = true
			}
			return changeSelection{paths: paths, anchor: current.anchor}
		}
	}
	if mods.Control || mods.Platform {
		paths := make(map[string]bool, len(current.paths)+1)
		maps.Copy(paths, current.paths)
		if paths[path] {
			delete(paths, path)
		} else {
			paths[path] = true
		}
		return changeSelection{paths: paths, anchor: current.anchor}
	}
	return changeSelection{paths: map[string]bool{path: true}, anchor: path}
}
