package main

import (
	"context"
	"fmt"
	"slices"
	"time"

	"gitwave.work/internal/gitcli"
	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

type dirtyRepository struct {
	name       string
	files      int
	incomplete bool
}

type quitGuard struct {
	dirty  *core.Signal[[]dirtyRepository]
	answer func(bool)
}

func (w *workbench) configureQuitGuard() {
	if w.cx.Env().OS() == core.OSMacOS {
		w.cx.OnKey("mod+w", w.requestExit)
	}
	w.cx.OnQuitRequest(func(request *core.QuitRequest) {
		if w.quitGuard.answer != nil {
			request.Cancel()
			return
		}
		w.checkBeforeQuit(request.Defer())
	})
	if window := w.cx.CurrentWindow(); window != nil {
		window.OnClosing(w.cx, func(closing *core.WindowClosing) {
			closing.Cancel()
			w.requestExit()
		})
	}
	w.cx.Cleanup(func() { w.answerQuit(false) })
}

func (w *workbench) requestExit() {
	if w.quitGuard.answer == nil {
		w.checkBeforeQuit(func(quit bool) {
			if quit {
				w.cx.Quit()
			}
		})
	}
}

func (w *workbench) checkBeforeQuit(answer func(bool)) {
	w.quitGuard.answer = answer
	repos := slices.Clone(w.workspaceState.Get().Repos)
	if w.workspaceStore == nil && w.path.Get() != "" {
		repos = []workspace.Repo{{Path: w.path.Get()}}
	}
	go func() {
		ctx, cancel := context.WithTimeout(w.appCtx, 1500*time.Millisecond)
		defer cancel()
		dirty := dirtyRepositories(ctx, repos)
		w.cx.Post(func() {
			if w.appCtx.Err() != nil {
				return
			}
			if len(dirty) == 0 {
				w.answerQuit(true)
			} else {
				w.quitGuard.dirty.Set(dirty)
			}
		})
	}()
}

func dirtyRepositories(ctx context.Context, repos []workspace.Repo) []dirtyRepository {
	results := make(chan dirtyRepository, len(repos))
	go func() {
		defer close(results)
		readDirtyRepositories(ctx, repos, results)
	}()
	var dirty []dirtyRepository
	next := 0
	collect := func(repo dirtyRepository) {
		next++
		if repo.files > 0 || repo.incomplete {
			dirty = append(dirty, repo)
		}
	}
	finish := func() []dirtyRepository {
		for _, entry := range repos[next:] {
			if !entry.Missing {
				dirty = append(dirty, dirtyRepository{name: quitRepoName(entry), incomplete: true})
			}
		}
		return dirty
	}
	for {
		select {
		case repo, ok := <-results:
			if !ok {
				return finish()
			}
			collect(repo)
		case <-ctx.Done():
			// A stalled filesystem call may not observe the deadline. Keep any results already available.
			for len(results) > 0 {
				collect(<-results)
			}
			return finish()
		}
	}
}

func readDirtyRepositories(ctx context.Context, repos []workspace.Repo, results chan<- dirtyRepository) {
	for _, entry := range repos {
		if ctx.Err() != nil {
			return
		}
		if entry.Missing {
			results <- dirtyRepository{}
			continue
		}
		name := quitRepoName(entry)
		repo, err := gitcli.OpenRoot(ctx, entry.Path)
		if err != nil {
			results <- dirtyRepository{name: name, incomplete: true}
			continue
		}
		files, err := repo.WorkingChangeCount(ctx)
		results <- dirtyRepository{name: name, files: files, incomplete: err != nil}
	}
}

func quitRepoName(entry workspace.Repo) string {
	if entry.Nickname != "" {
		return entry.Nickname
	}
	return entry.Path
}

func (w *workbench) answerQuit(quit bool) {
	answer := w.quitGuard.answer
	w.quitGuard.answer = nil
	if answer != nil {
		w.quitGuard.dirty.Set(nil)
		answer(quit)
	}
}

func (w *workbench) quitDialog(chrome palette) core.Node {
	dirty := w.quitGuard.dirty.Get
	incomplete := func() bool {
		return slices.ContainsFunc(dirty(), func(repo dirtyRepository) bool { return repo.incomplete })
	}
	var cancelFocus core.Ref
	cancelLabel := w.tr("Cancel", "取消")
	cancel := toolbarAction(chrome, cancelLabel, func() { w.answerQuit(false) }, chrome.label(cancelLabel, chrome.text, 12))
	cancel.Ref = &cancelFocus
	buttonLabel := w.tr("Quit anyway", "仍要退出")
	confirm := toolbarAction(chrome, buttonLabel, func() { w.answerQuit(true) }, chrome.label(buttonLabel, core.RGB(0xffffff), 12))
	confirm.Style.Background = chrome.accent
	confirm.Style.Hover = &core.Style{Background: chrome.accent, Opacity: core.Alpha(0.88)}
	return ui.Dialog{TestID: "quit-guard", Role: core.RoleAlertDialog,
		Open: func() bool { return len(dirty()) != 0 },
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				w.answerQuit(false)
			}
		},
		TitleFunc: func() string {
			if incomplete() {
				return w.tr("Review changes before quitting", "退出前检查改动")
			}
			return w.tr("Uncommitted changes", "存在未提交的改动")
		},
		DescriptionFunc: func() string {
			if incomplete() {
				return w.tr("Some repositories could not be fully checked for changes.", "部分仓库的改动未能检查完整。")
			}
			return w.tr("The following repositories in this workspace have uncommitted changes:", "当前工作区中以下仓库存在未提交的改动：")
		},
		Style: core.Style{Width: core.Pt(420)},
		Children: []core.Node{core.Func(func(cx *core.Scope) core.Node {
			cx.Post(cancelFocus.Focus)
			return core.List{EstimatedSize: 34, Gap: 6, Count: func() int { return len(dirty()) },
				StyleFunc: func() core.Style {
					count := min(7, len(dirty()))
					return core.Style{Height: core.Pt(core.Points(max(0, 40*count-6)))}
				},
				Row: func(_ *core.Scope, item core.ListRow) core.Node {
					return core.Dynamic(func(*core.Scope) core.Node {
						items, i := dirty(), item.Index()
						if i < 0 || i >= len(items) {
							return core.View{}
						}
						repo := items[i]
						label := chrome.label(repo.name, chrome.text, 14)
						label.Style.FlexGrow, label.Style.FlexShrink, label.Style.MinWidth = 1, 1, core.Pt(0)
						count := fmt.Sprintf(w.tr("%d uncommitted file(s)", "%d 个未提交文件"), repo.files)
						if repo.incomplete && repo.files == 0 {
							count = w.tr("Changes unknown", "改动状态未知")
						} else if repo.incomplete {
							count = fmt.Sprintf(w.tr("At least %d changed file(s)", "至少 %d 个改动文件"), repo.files)
						}
						return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Height: core.Pt(34),
							Gap: 12, Padding: core.Edges{Left: 12, Right: 12}, Background: chrome.elevated, CornerRadius: 6},
							Children: []core.Node{label, chrome.label(count, chrome.muted, 12)}}
					})
				}}
		})},
		Footer: []core.Node{cancel, confirm},
	}
}
