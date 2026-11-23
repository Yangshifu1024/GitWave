package main

import (
	"context"
	"flag"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
)

func main() {
	repo := flag.String("repo", "", "open an existing repository")
	flag.Parse()
	nsgo.Run(func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		if *repo != "" {
			w.open(*repo)
		}
		return w.view()
	}, nsgo.WindowSize(1200, 800),
		nsgo.WindowConstraints(core.WindowConstraints{MinWidth: 1024, MinHeight: 768}))
}

type workbench struct {
	cx         *core.Scope
	appCtx     context.Context
	repoCtx    context.Context
	cancelRepo context.CancelFunc
	core       *backend
	repo       *core.Signal[repository]
	path       *core.Signal[string]
	language   string

	commits        *core.Signal[[]commit]
	art            *core.Signal[[]graphRow]
	cursor         string
	more           *core.Signal[bool]
	snapshotLimit  *core.Signal[int]
	historyLoading *core.Signal[bool]
	historyError   *core.Signal[string]
	historyID      uint64
	filter         string
	locate         string
	historyView    core.Ref

	selected      *core.Signal[string]
	details       *core.Signal[commitDetails]
	files         *core.Signal[[]fileDiff]
	file          *core.Signal[string]
	diff          *core.Signal[preview]
	diffLoading   *core.Signal[bool]
	detailLoading *core.Signal[bool]
	detailError   *core.Signal[string]
	diffError     *core.Signal[string]
	expanded      *core.Signal[bool]
	splitDiff     *core.Signal[bool]
	diffCollapsed *core.Signal[bool]
	stopSelection context.CancelFunc
	stopDiff      context.CancelFunc
	selectionID   uint64
	diffID        uint64

	sideHidden *core.Signal[bool]
	focus      *core.Signal[bool]
	search     *core.Signal[bool]
}

func newWorkbench(cx *core.Scope) *workbench {
	w := &workbench{
		cx: cx, appCtx: cx.Context(), language: cx.Env().Locale(), path: core.NewSignal(cx, ""),
		repo: core.NewSignalFunc(cx, repository{}, nil), commits: core.NewSignalFunc(cx, []commit(nil), nil),
		art: core.NewSignalFunc(cx, []graphRow(nil), nil), more: core.NewSignal(cx, false),
		snapshotLimit:  core.NewSignal(cx, 0),
		historyLoading: core.NewSignal(cx, false), historyError: core.NewSignal(cx, ""),
		selected: core.NewSignal(cx, ""), details: core.NewSignalFunc(cx, commitDetails{}, nil),
		files: core.NewSignalFunc(cx, []fileDiff(nil), nil), file: core.NewSignal(cx, ""),
		diff: core.NewSignalFunc(cx, preview{}, nil), diffLoading: core.NewSignal(cx, false),
		detailLoading: core.NewSignal(cx, false), detailError: core.NewSignal(cx, ""),
		diffError: core.NewSignal(cx, ""), expanded: core.NewSignal(cx, false),
		diffCollapsed: core.NewSignal(cx, false),
		splitDiff:     core.NewSignal(cx, false), sideHidden: core.NewSignal(cx, false),
		focus: core.NewSignal(cx, false), search: core.NewSignal(cx, false),
	}
	cx.Cleanup(w.closeRepository)
	return w
}

func (w *workbench) closeRepository() {
	if w.cancelRepo != nil {
		w.cancelRepo()
	}
	if w.core != nil {
		w.core.close()
		w.core = nil
	}
}

func (w *workbench) tr(en, zh string) string {
	if strings.HasPrefix(w.language, "zh") {
		return zh
	}
	return en
}

func (w *workbench) pick() {
	w.cx.PickDirectories(core.PickDirectoriesOptions{
		Title: w.tr("Add local repository", "添加本地仓库"),
	}, func(dirs []core.Directory, err error) {
		if err != nil {
			w.historyError.Set(err.Error())
			return
		}
		if len(dirs) != 0 && dirs[0].Path != "" {
			w.open(dirs[0].Path)
		}
	})
}

func (w *workbench) open(path string) {
	w.closeRepository()
	ctx, cancel := context.WithCancel(w.appCtx)
	w.repoCtx, w.cancelRepo = ctx, cancel
	w.path.Set(filepath.Clean(path))
	w.repo.Set(repository{})
	w.resetHistory("")
	w.historyLoading.Set(true)
	go func() {
		child, err := startBackend(ctx, path)
		var info repository
		if err == nil {
			err = child.call(ctx, request{Method: "repository"}, &info)
			if err != nil || ctx.Err() != nil {
				child.close()
				child = nil
			}
		}
		// The child belongs to the request until the engine installs it. On
		// window close or a new pick, cancel guarantees that it exits anyway.
		w.cx.Post(func() {
			if ctx.Err() != nil {
				if child != nil {
					child.close()
				}
				return
			}
			w.historyLoading.Set(false)
			if err != nil {
				w.historyError.Set(err.Error())
				return
			}
			w.core = child
			w.repo.Set(info)
			w.loadHistory()
		})
	}()
}

func (w *workbench) resetHistory(filter string) {
	w.historyID++
	w.filter, w.cursor, w.locate = filter, "", ""
	w.historyLoading.Set(false)
	w.more.Set(true)
	w.snapshotLimit.Set(0)
	w.historyError.Set("")
	w.commits.Set(nil)
	w.art.Set(nil)
	w.clearSelection()
}

func (w *workbench) loadHistory() {
	if w.core == nil || w.historyLoading.Get() || !w.more.Get() || w.historyError.Get() != "" {
		return
	}
	w.historyLoading.Set(true)
	id, ctx, child := w.historyID, w.repoCtx, w.core
	req := request{Method: "history", Cursor: w.cursor, Filter: w.filter}
	go func() {
		page, replace, err := child.fetchHistory(ctx, req)
		w.cx.Post(func() {
			if ctx.Err() != nil || id != w.historyID {
				return
			}
			w.historyLoading.Set(false)
			if err != nil {
				w.historyError.Set(err.Error())
				if len(page.Commits) == 0 {
					return
				}
			}
			list := page.Commits
			if !replace {
				list = slices.Concat(w.commits.Get(), list)
			}
			w.cursor = page.Cursor
			w.more.Set(page.More)
			limit := 0
			if page.Truncated {
				limit = page.Size
			}
			w.snapshotLimit.Set(limit)
			w.art.Set(computeGraph(list))
			w.commits.Set(list)
			if w.locate != "" {
				w.cx.RequestAnimationFrame(func(time.Time) { w.finishLocate() })
			}
			if len(page.Commits) == 0 && page.More {
				w.loadHistory()
			}
		})
	}()
}

func (w *workbench) locateCommit(sha string) {
	if sha == "" {
		return
	}
	w.focus.Set(false)
	w.locate = sha
	if w.filter != "" {
		w.resetHistory("")
		w.locate = sha
	}
	w.finishLocate()
}

func (w *workbench) finishLocate() {
	if w.locate == "" {
		return
	}
	index := slices.IndexFunc(w.commits.Get(), func(c commit) bool { return c.SHA == w.locate })
	if index >= 0 {
		if _, ready := w.historyView.Frame(); !ready {
			return
		}
		w.selectCommit(w.locate)
		w.historyView.ScrollToIndexAt(index, 0.5, 0, false)
		w.locate = ""
	} else if w.more.Get() {
		w.loadHistory()
	}
}

func (w *workbench) clearSelection() {
	if w.stopSelection != nil {
		w.stopSelection()
	}
	if w.stopDiff != nil {
		w.stopDiff()
	}
	w.selectionID++
	w.diffID++
	w.selected.Set("")
	w.details.Set(commitDetails{})
	w.files.Set(nil)
	w.file.Set("")
	w.diff.Set(preview{})
	w.detailError.Set("")
	w.diffError.Set("")
	w.detailLoading.Set(false)
	w.diffLoading.Set(false)
	w.diffCollapsed.Set(false)
}

func (w *workbench) selectCommit(sha string) {
	if w.core == nil || sha == w.selected.Get() {
		return
	}
	w.clearSelection()
	w.selected.Set(sha)
	w.detailLoading.Set(true)
	ctx, stop := context.WithCancel(w.repoCtx)
	w.stopSelection = stop
	child, id := w.core, w.selectionID
	go func() {
		var details commitDetails
		var summary preview
		err := child.call(ctx, request{Method: "commit", SHA: sha}, &details)
		if err == nil {
			err = child.call(ctx, request{Method: "diff", SHA: sha}, &summary)
		}
		w.cx.Post(func() {
			if ctx.Err() != nil || id != w.selectionID {
				return
			}
			w.detailLoading.Set(false)
			if err != nil {
				w.detailError.Set(err.Error())
				return
			}
			w.details.Set(details)
			w.files.Set(summary.Diff.Files)
			if len(summary.Diff.Files) > 0 {
				w.selectFile(summary.Diff.Files[0].Path, false)
			}
		})
	}()
}

func (w *workbench) selectFile(path string, expanded bool) {
	if w.stopDiff != nil {
		w.stopDiff()
	}
	if w.file.Get() != path {
		w.diffCollapsed.Set(false)
	}
	w.diffID++
	w.file.Set(path)
	w.diff.Set(preview{})
	w.diffError.Set("")
	w.diffLoading.Set(true)
	w.expanded.Set(expanded)
	req := request{Method: "diff", SHA: w.selected.Get(), Path: path, Expanded: expanded}
	ctx, stop := context.WithCancel(w.repoCtx)
	w.stopDiff = stop
	child, id := w.core, w.diffID
	go func() {
		var result preview
		err := child.call(ctx, req, &result)
		w.cx.Post(func() {
			if ctx.Err() != nil || id != w.diffID {
				return
			}
			w.diffLoading.Set(false)
			if err != nil {
				w.diffError.Set(err.Error())
			} else {
				w.diff.Set(result)
			}
		})
	}()
}
