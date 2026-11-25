package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gitwave.work/internal/appupdate"
	"gitwave.work/internal/desktopgtk"
	"gitwave.work/internal/gitcli"
	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
)

func main() {
	desktopgtk.Probe()
	if gitcli.LocalLFSFilter() || gitcli.LFSPrePush() {
		return
	}
	if gitcli.CredentialHelper(os.Args[1:], os.Stdin, os.Stdout) {
		return
	}
	repo := flag.String("repo", "", "open an existing repository")
	if runtime.GOOS == "linux" {
		flag.Parse()
	}
	if err := desktopgtk.Prepare(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := appupdate.WaitForUpdateParent(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if runtime.GOOS != "linux" {
		flag.Parse()
	}
	var store *workspace.Store
	var storeErr error
	var legacyPrefsErr error
	var openProfile sync.Once
	defer func() {
		if store != nil {
			_ = store.Close()
		}
	}()
	nsgo.Run(func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.configureUpdates()
		if *repo != "" {
			w.open(*repo)
		} else {
			// The desktop callback only runs for the workbench, after NSGo
			// handles installer hooks and hands off secondary launches.
			openProfile.Do(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				store, storeErr = openAppWorkspace(ctx, core.App())
				cancel()
				if storeErr != nil {
					fmt.Fprintln(os.Stderr, storeErr)
					return
				}
				legacyPath, err := workspace.LegacyPath()
				if err == nil {
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					err = store.ImportLegacyPreferences(ctx, legacyPath)
					cancel()
				}
				legacyPrefsErr = err
			})
			w.connectWorkspace(store, storeErr)
			if errors.Is(legacyPrefsErr, workspace.ErrLegacyPreferencesMissing) {
				w.desktopPrefsErr.Set(w.tr("The previous version did not export its preferences", "旧版本未导出偏好设置"))
			} else if legacyPrefsErr != nil {
				w.desktopPrefsErr.Set(w.tr("Could not import the previous version's preferences", "无法导入旧版本的偏好设置"))
			}
		}
		return w.view()
	}, nsgo.WindowSize(1200, 800),
		nsgo.WindowConstraints(core.WindowConstraints{MinWidth: 1024, MinHeight: 768}))
}

type workbench struct {
	cx           *core.Scope
	appCtx       context.Context
	repoCtx      context.Context
	cancelRepo   context.CancelFunc
	core         *backend
	repo         *core.Signal[repository]
	path         *core.Signal[string]
	chinese      atomic.Bool
	systemLocale string
	desktopPreferencesUI
	appUpdateUI
	quitGuard
	networkSettingsUI
	aboutUI
	aiSettings  *core.Signal[*aiSettingsForm]
	aiDraft     *core.Signal[*aiDraftForm]
	paletteForm *core.Signal[*commandPaletteForm]
	workspaceUI
	changesUI
	conflictsUI
	interactiveRebaseUI
	stashUI
	worktreeUI
	remoteUI
	submoduleUI
	lfsUI
	repoEditorUI
	refUI
	reflogUI
	healthUI
	fetch          fetchUI
	pull           pullUI
	push           pushUI
	checkoutPrompt *core.Signal[checkoutPrompt]
	mergeAbort     *core.Signal[*mergeAbortPrompt]

	commits          *core.Signal[[]commit]
	art              *core.Signal[[]graphRow]
	cursor           string
	more             *core.Signal[bool]
	snapshotLimit    *core.Signal[int]
	historyLoading   *core.Signal[bool]
	historyError     *core.Signal[string]
	historyID        uint64
	stopHistory      context.CancelFunc
	locate           string
	historyView      core.Ref
	historyGraphView core.Ref
	historyOffset    core.Point

	refreshRequest *core.Signal[refreshJob]
	refreshRunning *core.Signal[bool]
	refreshSettingsUI
	sshSettings *core.Signal[*sshSettingsPanel]

	selected            *core.Signal[string]
	details             *core.Signal[commitDetails]
	messageExpanded     *core.Signal[bool]
	files               *core.Signal[[]fileDiff]
	file                *core.Signal[string]
	diff                *core.Signal[preview]
	diffLoading         *core.Signal[bool]
	detailLoading       *core.Signal[bool]
	detailError         *core.Signal[string]
	diffError           *core.Signal[string]
	expanded            *core.Signal[bool]
	splitDiff           *core.Signal[bool]
	diffCollapsed       *core.Signal[bool]
	historyDiffViewport diffViewport
	stopSelection       context.CancelFunc
	stopDiff            context.CancelFunc
	stopBlame           context.CancelFunc
	selectionID         uint64
	diffID              uint64
	blame               *core.Signal[blameState]

	sideHidden        *core.Signal[bool]
	focus             *core.Signal[bool]
	branchSectionOpen *core.Signal[bool]
	branchCollapsed   *core.Signal[map[branchFolderKey]bool]
	branchSelection   *core.Signal[branchKey]
	branchTracking    *core.Signal[map[string]branchTrackingStatus]
	branchPress       branchKey
}

type historySelection struct{ repoPath, sha, file string }

func newWorkbench(cx *core.Scope) *workbench {
	w := &workbench{
		cx: cx, appCtx: cx.Context(), path: core.NewSignal(cx, ""),
		repo: core.NewSignalFunc(cx, repository{}, nil), commits: core.NewSignalFunc(cx, []commit(nil), nil),
		art: core.NewSignalFunc(cx, []graphRow(nil), nil), more: core.NewSignal(cx, false),
		snapshotLimit:  core.NewSignal(cx, 0),
		historyLoading: core.NewSignal(cx, false), historyError: core.NewSignal(cx, ""),
		selected: core.NewSignal(cx, ""), details: core.NewSignalFunc(cx, commitDetails{}, nil),
		systemLocale: cx.Env().Locale(), messageExpanded: core.NewSignal(cx, false),
		files: core.NewSignalFunc(cx, []fileDiff(nil), nil), file: core.NewSignal(cx, ""),
		diff: core.NewSignalFunc(cx, preview{}, nil), diffLoading: core.NewSignal(cx, false),
		detailLoading: core.NewSignal(cx, false), detailError: core.NewSignal(cx, ""),
		diffError: core.NewSignal(cx, ""), expanded: core.NewSignal(cx, false),
		diffCollapsed: core.NewSignal(cx, false), blame: core.NewSignalFunc(cx, blameState{}, nil),
		splitDiff: core.NewSignal(cx, false), sideHidden: core.NewSignal(cx, false),
		focus:             core.NewSignal(cx, false),
		branchSectionOpen: core.NewSignal(cx, true),
		branchCollapsed:   core.NewSignalFunc(cx, make(map[branchFolderKey]bool), nil),
		branchSelection:   core.NewSignal(cx, branchKey{}),
		branchTracking:    core.NewSignalFunc(cx, map[string]branchTrackingStatus(nil), nil),
		workspaceUI:       newWorkspaceUI(cx), refreshRequest: core.NewSignal(cx, refreshJob{}),
		changesUI:            newChangesUI(cx),
		conflictsUI:          newConflictsUI(cx),
		interactiveRebaseUI:  newInteractiveRebaseUI(cx),
		stashUI:              newStashUI(cx),
		worktreeUI:           newWorktreeUI(cx),
		remoteUI:             newRemoteUI(cx),
		submoduleUI:          newSubmoduleUI(cx),
		lfsUI:                newLFSUI(cx),
		repoEditorUI:         newRepoEditorUI(cx),
		refUI:                newRefUI(cx),
		reflogUI:             newReflogUI(cx),
		healthUI:             newHealthUI(cx),
		fetch:                newFetchUI(cx),
		pull:                 newPullUI(cx),
		push:                 newPushUI(cx),
		checkoutPrompt:       core.NewSignal(cx, checkoutPrompt{}),
		mergeAbort:           core.NewSignal[*mergeAbortPrompt](cx, nil),
		refreshRunning:       core.NewSignal(cx, false),
		refreshSettingsUI:    newRefreshSettingsUI(cx),
		sshSettings:          core.NewSignal[*sshSettingsPanel](cx, nil),
		desktopPreferencesUI: newDesktopPreferencesUI(cx),
		appUpdateUI:          newAppUpdateUI(cx),
		quitGuard:            quitGuard{dirty: core.NewSignalFunc(cx, []dirtyRepository(nil), nil)},
		networkSettingsUI:    newNetworkSettingsUI(cx),
		aboutUI:              newAboutUI(cx),
		aiSettings:           core.NewSignal[*aiSettingsForm](cx, nil),
		aiDraft:              core.NewSignal[*aiDraftForm](cx, nil),
		paletteForm:          core.NewSignal[*commandPaletteForm](cx, nil),
	}
	w.syncLocale()
	cx.Cleanup(w.closeRepository)
	return w
}

func (w *workbench) closeRepository() {
	w.closeCommandPalette()
	w.closeAIDraft()
	if w.cancelRepo != nil {
		w.cancelRepo()
	}
	w.closeChanges()
	w.closeConflicts()
	w.interactiveForm.Set(nil)
	w.clearStashes()
	w.clearWorktrees()
	w.clearRemotes()
	w.clearSubmodules()
	w.closeLFS()
	w.closeRepoEditors()
	w.closeReflog()
	w.clearHealth()
	w.branchSelection.Set(branchKey{})
	w.branchPress = branchKey{}
	w.closeMergeAbort()
	w.refForm.Set(nil)
	w.fetch.prompt.Set(nil)
	w.fetch.auth.reset()
	w.fetch.notice.Set(nil)
	w.pull.form.Set(nil)
	w.push.form.Set(nil)
	w.push.review.Set(nil)
	w.checkoutPrompt.Set(checkoutPrompt{})
	w.refNotice.Set(referenceNotice{})
	w.tagsOpen.Set(false)
	w.core = nil
}

func (w *workbench) tr(en, zh string) string {
	if w.chinese.Load() {
		return zh
	}
	return en
}

// Read NSGo locale on the UI thread. Repository workers can translate
// progress and errors without touching the reactive tree.
func (w *workbench) syncLocale() {
	w.chinese.Store(strings.HasPrefix(strings.ToLower(w.cx.Env().Locale()), "zh"))
}

func (w *workbench) pick() {
	if w.workspaceBlocked() || w.workspaceModal.Get().kind != "" {
		return
	}
	ws := w.workspaceState.Get().Workspace.ID
	if w.workspaceStore != nil && ws == "" {
		w.newWorkspace()
		return
	}
	w.cx.PickDirectories(core.PickDirectoriesOptions{
		Title: w.tr("Add local repository", "添加本地仓库"), Multiple: w.workspaceStore != nil,
	}, func(dirs []core.Directory, err error) {
		if err != nil {
			if w.workspaceStore != nil {
				w.workspaceError.Set(err.Error())
			} else {
				w.historyError.Set(err.Error())
			}
			return
		}
		if len(dirs) == 0 {
			return
		}
		if w.workspaceStore != nil {
			if w.workspaceState.Get().Workspace.ID == ws {
				w.addWorkspaceRepos(ws, dirs)
			}
		} else if dirs[0].Path != "" {
			w.open(dirs[0].Path)
		}
	})
}

func (w *workbench) open(path string) {
	w.closeRepository()
	ctx, cancel := context.WithCancel(w.appCtx)
	w.repoCtx, w.cancelRepo = ctx, cancel
	w.path.Set(filepath.Clean(path))
	w.bindChanges(w.path.Get())
	savedHistory := w.changesSessions[w.changesScope].history
	w.repo.Set(repository{})
	w.resetHistory()
	w.historyLoading.Set(true)
	saved := w.workspaceStore != nil
	go func() {
		openBackend := startBackend
		if saved {
			openBackend = startWorkspaceBackend
		}
		child, err := openBackend(ctx, path)
		var info repository
		if err == nil {
			info, err = child.loadRepository(ctx)
		}
		// The backend belongs to the open request until the UI installs it.
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			w.historyLoading.Set(false)
			w.loadHealth(child)
			if err != nil {
				w.historyError.Set(err.Error())
				return
			}
			w.core = child
			w.repo.Set(info)
			if savedHistory.repoPath == w.path.Get() && savedHistory.sha != "" {
				w.selectHistoryCommit(savedHistory)
			}
			w.loadHistory()
			w.watchChanges()
			w.loadStashes()
			w.loadWorktrees()
			w.loadRemotes()
			w.loadSubmodules()
			w.loadReflog()
		})
	}()
}

func (w *workbench) resetHistory() {
	if w.stopHistory != nil {
		w.stopHistory()
		w.stopHistory = nil
	}
	w.historyID++
	w.cursor, w.locate = "", ""
	w.historyOffset = core.Point{}
	w.historyLoading.Set(false)
	w.more.Set(true)
	w.snapshotLimit.Set(0)
	w.historyError.Set("")
	w.commits.Set(nil)
	w.art.Set(nil)
	w.messageExpanded.Set(false)
	w.clearSelection()
}

func (w *workbench) loadHistory() {
	if w.core == nil || w.historyLoading.Get() || !w.more.Get() || w.historyError.Get() != "" {
		return
	}
	ctx, stop, id := w.startHistoryRequest()
	child := w.core
	cursor := w.cursor
	go func() {
		page, replace, err := child.fetchHistory(ctx, cursor, "")
		w.cx.Post(func() {
			defer stop()
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
			w.publishHistoryPage(page, replace)
			if len(page.Commits) == 0 && page.More {
				w.loadHistory()
			}
		})
	}()
}

func (w *workbench) startHistoryRequest() (context.Context, context.CancelFunc, uint64) {
	if w.stopHistory != nil {
		w.stopHistory()
	}
	ctx, stop := context.WithCancel(w.repoCtx)
	w.stopHistory = stop
	w.historyID++
	w.historyLoading.Set(true)
	return ctx, stop, w.historyID
}

func (w *workbench) publishHistoryPage(page historyPage, replace bool) {
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
		w.cx.Post(w.finishLocate)
	}
}

func (w *workbench) locateCommit(sha string) {
	if sha == "" {
		return
	}
	w.showChanges(false)
	w.focus.Set(false)
	w.locate = sha
	w.cx.Post(w.finishLocate)
}

func (w *workbench) finishLocate() {
	if w.locate == "" {
		return
	}
	index := slices.IndexFunc(w.commits.Get(), func(c commit) bool { return c.SHA == w.locate })
	if index >= 0 {
		list, listMounted := w.historyView.Frame()
		graph, graphMounted := w.historyGraphView.Frame()
		if !listMounted || list.Size.Height <= 0 || !graphMounted || graph.Size.Width <= 0 {
			return
		}
		w.selectCommit(w.locate)
		w.historyView.ScrollToIndexAt(index, 0.5, 0, false)
		w.locate = ""
	} else if w.historyLoading.Get() {
		return
	} else if w.more.Get() {
		w.loadHistory()
	} else {
		w.locate = ""
	}
}

func (w *workbench) clearSelection() {
	w.closeBlame()
	if w.stopSelection != nil {
		w.stopSelection()
	}
	if w.stopDiff != nil {
		w.stopDiff()
	}
	w.selectionID++
	w.diffID++
	w.historyDiffViewport = diffViewport{}
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
	commits := w.commits.Get()
	if i := slices.IndexFunc(commits, func(c commit) bool { return c.SHA == sha }); i != -1 {
		w.revealHistoryLane(commits[i].Lane)
	}
	w.selectHistoryCommit(historySelection{sha: sha})
}

func (w *workbench) selectHistoryCommit(selection historySelection) {
	sha := selection.sha
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
		var summary preview
		var missing bool
		details, err := child.loadCommit(ctx, sha)
		if err == nil {
			w.cx.Post(func() {
				if ctx.Err() == nil && id == w.selectionID {
					w.details.Set(details)
				}
			})
			summary, err = child.repo.CommitDiff(ctx, details, "", false)
		} else if selection.repoPath != "" && ctx.Err() == nil {
			exists, checkErr := child.repo.HasCommit(ctx, sha)
			missing = checkErr == nil && !exists
		}
		w.cx.Post(func() {
			if ctx.Err() != nil || id != w.selectionID {
				return
			}
			w.detailLoading.Set(false)
			if missing {
				w.clearSelection()
				return
			}
			if err != nil {
				w.detailError.Set(err.Error())
				return
			}
			w.files.Set(summary.Files)
			if len(summary.Files) > 0 {
				path := summary.Files[0].Path
				if slices.ContainsFunc(summary.Files, func(file fileDiff) bool { return file.Path == selection.file }) {
					path = selection.file
				}
				w.selectFile(path, false)
			}
		})
	}()
}

func (w *workbench) retryCommitDetails() {
	sha := w.selected.Get()
	selection := historySelection{sha: sha}
	if previous := w.changesSessions[w.changesScope].history; previous.sha == sha && previous.repoPath == w.path.Get() {
		selection = previous
	}
	w.selected.Set("")
	w.selectHistoryCommit(selection)
}

func (w *workbench) selectFile(path string, expanded bool) {
	if w.stopDiff != nil {
		w.stopDiff()
	}
	if w.file.Get() != path {
		w.closeBlame()
		w.historyDiffViewport = diffViewport{}
		w.diffCollapsed.Set(false)
	}
	w.diffID++
	w.file.Set(path)
	w.diff.Set(preview{})
	w.diffError.Set("")
	w.diffLoading.Set(true)
	w.expanded.Set(expanded)
	details := w.details.Get()
	ctx, stop := context.WithCancel(w.repoCtx)
	w.stopDiff = stop
	child, id := w.core, w.diffID
	go func() {
		result, err := child.repo.CommitDiff(ctx, details, path, expanded)
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
