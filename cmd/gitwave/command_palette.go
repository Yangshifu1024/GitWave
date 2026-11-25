package main

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

func (w *workbench) locatePaletteCommit(sha string) {
	if slices.ContainsFunc(w.commits.Get(), func(c commit) bool { return c.SHA == sha }) {
		w.locateCommit(sha)
		return
	}
	w.showChanges(false)
	w.focus.Set(false)
	w.selectCommit(sha)
	w.locate = sha
	w.refreshRepository(false)
}

type commandPaletteForm struct {
	child      *backend
	key        changesWriteKey
	query      *core.Signal[string]
	selection  *core.Signal[int]
	page       *core.Signal[paletteSearchPage]
	searchStop context.CancelFunc
	searchID   uint64
	snapshot   *historySnapshot
	ai         *core.Signal[paletteAIState]
	aiStop     context.CancelFunc
	aiID       uint64
}

type paletteSearchPage struct {
	commits   []commit
	loading   bool
	failure   string
	cursor    string
	scanned   int
	size      int
	truncated bool
}

func (w *workbench) toggleCommandPalette() {
	if form := w.paletteForm.Get(); form != nil {
		if !form.ai.Get().saving {
			w.closeCommandPalette()
		}
		return
	}
	if w.refreshBlocked() {
		return
	}
	w.paletteForm.Set(&commandPaletteForm{child: w.core, key: w.changesWriteKey(),
		query: core.NewSignal(w.cx, ""), selection: core.NewSignal(w.cx, -1),
		page: core.NewSignalFunc(w.cx, paletteSearchPage{}, nil), ai: core.NewSignal(w.cx, paletteAIState{})})
}

func (w *workbench) currentCommandPalette(form *commandPaletteForm) bool {
	return w.appCtx.Err() == nil && w.paletteForm.Get() == form && form.child == w.core && form.key == w.changesWriteKey()
}

func (w *workbench) closeCommandPalette() {
	if form := w.paletteForm.Get(); form != nil {
		if form.searchStop != nil {
			form.searchStop()
		}
		form.cancelAI()
		w.paletteForm.Set(nil)
	}
}

func (w *workbench) updatePaletteQuery(form *commandPaletteForm, value string) {
	if !w.currentCommandPalette(form) || form.ai.Get().saving || form.query.Get() == value {
		return
	}
	if form.searchStop != nil {
		form.searchStop()
		form.searchStop = nil
	}
	form.cancelAI()
	form.query.Set(value)
	form.selection.Set(-1)
	form.page.Set(paletteSearchPage{})
	form.snapshot = nil
	if strings.TrimSpace(value) != "" && form.child != nil {
		w.searchPalette(form, true)
	}
}

func (w *workbench) searchPalette(form *commandPaletteForm, debounce bool) {
	page := form.page.Get()
	query := strings.ToLower(strings.TrimSpace(form.query.Get()))
	if form.child == nil || page.loading || query == "" || !w.currentCommandPalette(form) {
		return
	}
	page.loading, page.failure = true, ""
	form.page.Set(page)
	snapshot, child := form.snapshot, form.child
	repo := child.repo
	ctx, cancel := context.WithCancel(w.repoCtx)
	form.searchStop = cancel
	form.searchID++
	id := form.searchID
	go func() {
		defer cancel()
		if debounce {
			timer := time.NewTimer(300 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
		}
		var result historyPage
		var err error
		if snapshot == nil {
			if refs, readErr := repo.References(ctx); readErr == nil {
				snapshot, err = child.paletteHistorySnapshot(ctx, refs, query)
			} else {
				err = readErr
			}
		}
		if err == nil {
			result, err = snapshot.pageWithLimit(ctx, repo, page.scanned, 10)
		}
		if ctx.Err() != nil {
			return
		}
		w.cx.Post(func() {
			if !w.currentCommandPalette(form) || form.searchID != id || strings.ToLower(strings.TrimSpace(form.query.Get())) != query {
				return
			}
			form.searchStop = nil
			form.snapshot = snapshot
			page.loading = false
			if err != nil {
				page.failure = err.Error()
			} else {
				page.commits = append(page.commits, result.Commits...)
				page.cursor, page.size, page.truncated = result.Cursor, result.Size, result.Truncated
				page.scanned = page.size
				if result.More {
					page.scanned, _ = snapshot.offset(result.Cursor)
				}
			}
			form.page.Set(page)
		})
	}()
}
