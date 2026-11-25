package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gitwave.work/internal/gitcli"
	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

type workspaceUI struct {
	workspaceStore     *workspace.Store
	workspaceState     *core.Signal[workspace.State]
	workspaceBusy      *core.Signal[bool]
	workspaceReload    *core.Signal[bool]
	workspaceError     *core.Signal[string]
	workspaceNotice    *core.Signal[string]
	workspaceModal     *core.Signal[workspaceDialog]
	workspaceInput     *core.Signal[string]
	workspaceOverwrite *core.Signal[*workspace.TransferTargetExists]
	workspaceFailed    bool
	workspaceAction    uint64
	workspaceRetry     *workspaceSelection
	clone              cloneUI
}

type workspaceDialog struct {
	kind string
	ws   workspace.Summary
	repo workspace.Repo
}

type workspaceResult struct {
	notice              string
	workspaceID, repoID string
}

type workspaceSelection struct{ workspaceID, repoID string }

func (d workspaceDialog) hasInput() bool {
	return d.kind != "delete" && d.kind != "remove"
}

func newWorkspaceUI(cx *core.Scope) workspaceUI {
	return workspaceUI{
		workspaceState:     core.NewSignalFunc(cx, workspace.State{}, nil),
		workspaceBusy:      core.NewSignal(cx, false),
		workspaceReload:    core.NewSignal(cx, false),
		workspaceError:     core.NewSignal(cx, ""),
		workspaceNotice:    core.NewSignal(cx, ""),
		workspaceModal:     core.NewSignal(cx, workspaceDialog{}),
		workspaceInput:     core.NewSignal(cx, ""),
		workspaceOverwrite: core.NewSignal(cx, (*workspace.TransferTargetExists)(nil)),
		clone:              newCloneUI(cx),
	}
}

func workspacePathForApp(app core.AppInfo) (string, error) {
	if app.ID == "desktop.gitwave.work" && !app.Development {
		return workspace.LegacyPath()
	}
	return workspace.PreviewPath()
}

func openAppWorkspace(ctx context.Context, app core.AppInfo) (*workspace.Store, error) {
	path, err := workspacePathForApp(app)
	if err != nil {
		return nil, err
	}
	legacy, err := workspace.LegacyPath()
	if err != nil {
		return nil, err
	}
	if path == legacy {
		store, err := workspace.OpenStable(ctx, path)
		if !errors.Is(err, workspace.ErrStableProfileInUse) {
			return store, err
		}
		// The transition installer can start the new desktop while the
		// previous process is finishing its last write and releasing the lock.
		deadline := time.NewTimer(time.Second)
		defer deadline.Stop()
		retry := time.NewTicker(50 * time.Millisecond)
		defer retry.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-deadline.C:
				return nil, workspace.ErrStableProfileInUse
			case <-retry.C:
				store, err := workspace.OpenStable(ctx, path)
				if !errors.Is(err, workspace.ErrStableProfileInUse) {
					return store, err
				}
			}
		}
	}
	return workspace.OpenWithLegacy(ctx, path, legacy)
}

func (w *workbench) connectWorkspace(store *workspace.Store, err error) {
	w.workspaceStore = store
	if err != nil {
		w.workspaceFailed = true
		message := err.Error()
		if errors.Is(err, workspace.ErrStableProfileInUse) {
			message = w.tr("Quit the other GitWave instance using this workspace profile.", "请退出另一个正在使用此工作区数据的 GitWave 实例。")
		}
		w.workspaceError.Set(message)
		return
	}
	w.loadDesktopPreferences()
	w.loadLegacyWorkbenchLayout()
	proxyReady := w.loadProxySettings()
	w.loadRefreshSettings()
	w.loadAutoUpdate(proxyReady)
	w.runWorkspace(nil)
}

func (w *workbench) workspaceBlocked() bool {
	return w.workspaceFailed || w.workspaceBusy.Get() || w.workspaceReload.Get() ||
		w.conflictPanel.Get() != nil || w.mergeAbort.Get() != nil || w.interactiveForm.Get() != nil || w.aiSettings.Get() != nil
}

// runWorkspace serializes mutations and opens only the repository chosen by
// the resulting persisted selection.
func (w *workbench) runWorkspace(change func(context.Context) (workspaceResult, error)) {
	w.runWorkspaceAction(change, true)
}

func (w *workbench) runWorkspaceAction(change func(context.Context) (workspaceResult, error), refresh bool) {
	store := w.workspaceStore
	if store == nil || w.workspaceBusy.Get() || w.conflictPanel.Get() != nil || w.mergeAbort.Get() != nil ||
		w.interactiveForm.Get() != nil || (change != nil && w.workspaceReload.Get()) ||
		(change == nil && w.workspaceModal.Get().kind != "") {
		return
	}
	w.workspaceAction++
	w.workspaceBusy.Set(true)
	w.workspaceError.Set("")
	w.workspaceNotice.Set("")
	ctx := w.appCtx
	dialog := w.workspaceModal.Get()
	input := strings.TrimSpace(w.workspaceInput.Get())
	current := w.workspaceState.Get()
	startupOrRetry := w.workspaceRetry
	go func() {
		var result workspaceResult
		var err error
		if change != nil {
			result, err = change(ctx)
		}
		var state workspace.State
		var refreshErr error
		target := startupOrRetry
		if err == nil && refresh {
			if change != nil {
				ws, repo := result.workspaceID, result.repoID
				if ws == "" {
					ws, repo = current.Workspace.ID, current.Repo.ID
				}
				target = &workspaceSelection{ws, repo}
			}
			state, refreshErr = loadWindowWorkspace(ctx, store, target)
		}
		w.cx.Post(func() {
			if ctx.Err() != nil {
				return
			}
			w.workspaceBusy.Set(false)
			if dialog.kind == "clone" && w.finishCloneAttempt(err) {
				return
			}
			if err != nil {
				if (dialog.kind == "import" || dialog.kind == "export") && strings.TrimSpace(w.workspaceInput.Get()) != input {
					return
				}
				var target *workspace.TransferTargetExists
				if errors.As(err, &target) {
					w.workspaceOverwrite.Set(target)
					w.workspaceError.Set(fmt.Sprintf(w.tr("Replace the existing file %s?", "覆盖已有文件 %s？"), target.Path))
				} else {
					w.workspaceOverwrite.Set(nil)
					w.workspaceError.Set(err.Error())
				}
				return
			}
			w.workspaceModal.Set(workspaceDialog{})
			w.workspaceOverwrite.Set(nil)
			if !refresh {
				w.workspaceNotice.Set(result.notice)
				return
			}
			w.workspaceReload.Set(refreshErr != nil)
			if refreshErr != nil {
				w.workspaceRetry = target
				w.workspaceError.Set(refreshErr.Error())
				w.applyWorkspace(workspace.State{}, false)
				return
			}
			w.workspaceRetry = nil
			w.workspaceNotice.Set(result.notice)
			w.applyWorkspace(state, change == nil)
		})
	}()
}

func keepWorkspaceRepo(state *workspace.State, id string) {
	if id == "" {
		return
	}
	index := slices.IndexFunc(state.Repos, func(repo workspace.Repo) bool { return repo.ID == id && !repo.Missing })
	if index == -1 {
		index = slices.IndexFunc(state.Repos, func(repo workspace.Repo) bool { return !repo.Missing })
	}
	if index != -1 {
		state.Repo = state.Repos[index]
	}
}

func (w *workbench) applyWorkspace(state workspace.State, reload bool) {
	previous := w.workspaceState.Get()
	if action := w.fetch.running.Get(); action != nil && action.workspace != "" && action.workspace != state.Workspace.ID {
		action.cancel()
	}
	w.workspaceState.Set(state)
	if state.Repo.Path != "" {
		if reload || previous.Repo.ID != state.Repo.ID || previous.Workspace.ID != state.Workspace.ID ||
			previous.Repo.Path != state.Repo.Path || w.path.Get() != filepath.Clean(state.Repo.Path) {
			w.open(state.Repo.Path)
		}
	} else if w.path.Get() != "" {
		w.closeRepository()
		w.path.Set("")
		w.repo.Set(repository{})
		w.resetHistory()
	}
}

func loadWorkspace(ctx context.Context, store *workspace.Store) (workspace.State, error) {
	return loadCheckedWorkspace(ctx, store, store.Load)
}

func loadWindowWorkspace(ctx context.Context, store *workspace.Store, target *workspaceSelection) (workspace.State, error) {
	if target == nil {
		return loadWorkspace(ctx, store)
	}
	state, err := loadCheckedWorkspace(ctx, store, func(ctx context.Context) (workspace.State, error) {
		return store.LoadWorkspace(ctx, target.workspaceID)
	})
	if err == nil {
		keepWorkspaceRepo(&state, target.repoID)
	}
	return state, err
}

func loadCheckedWorkspace(ctx context.Context, store *workspace.Store, load func(context.Context) (workspace.State, error)) (workspace.State, error) {
	state, err := load(ctx)
	if err != nil {
		return workspace.State{}, err
	}
	var changed []workspace.Repo
	for _, repo := range state.Repos {
		_, err := gitcli.OpenRoot(ctx, repo.Path)
		if ctx.Err() != nil {
			return workspace.State{}, ctx.Err()
		}
		missing := errors.Is(err, os.ErrNotExist) || errors.Is(err, gitcli.ErrNotRepositoryRoot)
		if err != nil && !missing {
			continue
		}
		if missing != repo.Missing {
			repo.Missing = missing
			changed = append(changed, repo)
		}
	}
	if len(changed) == 0 && state.Workspace.LastRepoID == state.Repo.ID {
		return state, nil
	}
	if err := store.UpdatePresence(ctx, state.Workspace.ID, changed); err != nil {
		return workspace.State{}, err
	}
	return load(ctx)
}

func (w *workbench) addWorkspaceRepos(ws string, dirs []core.Directory) {
	store := w.workspaceStore
	w.runWorkspace(func(ctx context.Context) (workspaceResult, error) {
		paths := make([]string, 0, len(dirs))
		var rejected []string
		for _, dir := range dirs {
			if dir.Path == "" {
				rejected = append(rejected, dir.Name)
				continue
			}
			repo, err := gitcli.OpenRoot(ctx, dir.Path)
			if err != nil {
				rejected = append(rejected, dir.Path)
				continue
			}
			paths = append(paths, repo.Path())
		}
		if ctx.Err() != nil {
			return workspaceResult{}, ctx.Err()
		}
		_, last, err := store.AddRepos(ctx, ws, paths)
		if err != nil {
			return workspaceResult{}, err
		}
		var result workspaceResult
		if last != "" {
			result.workspaceID, result.repoID = ws, last
		}
		if len(rejected) != 0 {
			result.notice = fmt.Sprintf(w.tr("Could not add %s", "无法添加 %s"), strings.Join(rejected, ", "))
		}
		return result, nil
	})
}

func (w *workbench) selectWorkspace(id string) {
	if id == w.workspaceState.Get().Workspace.ID {
		return
	}
	w.runWorkspace(func(ctx context.Context) (workspaceResult, error) {
		return workspaceResult{workspaceID: id}, w.workspaceStore.SelectWorkspace(ctx, id)
	})
}

func (w *workbench) selectWorkspaceRepo(ws, id string) {
	state := w.workspaceState.Get()
	if ws != state.Workspace.ID || id == state.Repo.ID {
		return
	}
	w.runWorkspace(func(ctx context.Context) (workspaceResult, error) {
		return workspaceResult{workspaceID: ws, repoID: id}, w.workspaceStore.SelectRepo(ctx, ws, id)
	})
}

func (w *workbench) newWorkspace() { w.showWorkspaceDialog("create", workspace.Summary{}) }

func (w *workbench) showWorkspaceDialog(kind string, ws workspace.Summary) {
	w.showWorkspaceForm(workspaceDialog{kind: kind, ws: ws}, ws.Name)
}

func (w *workbench) showWorkspaceForm(dialog workspaceDialog, input string) {
	if w.workspaceStore == nil || w.workspaceBlocked() || w.workspaceModal.Get().kind != "" {
		return
	}
	if dialog.ws.ID != "" && dialog.ws.ID != w.workspaceState.Get().Workspace.ID {
		return
	}
	w.workspaceError.Set("")
	w.workspaceNotice.Set("")
	w.workspaceOverwrite.Set(nil)
	w.workspaceInput.Set(input)
	w.workspaceModal.Set(dialog)
}

func (w *workbench) pickWorkspaceDirectory(dialog workspaceDialog, title string) {
	if w.workspaceBlocked() || w.workspaceModal.Get() != dialog {
		return
	}
	w.cx.PickDirectories(core.PickDirectoriesOptions{Title: title},
		func(dirs []core.Directory, err error) {
			if w.workspaceModal.Get() != dialog || w.workspaceBusy.Get() {
				return
			}
			if err != nil {
				w.workspaceError.Set(err.Error())
			} else if len(dirs) != 0 && dirs[0].Path != "" {
				if dialog.kind == "clone" {
					w.clone.replacement.Set(nil)
					w.workspaceError.Set("")
				}
				w.workspaceInput.Set(dirs[0].Path)
			}
		})
}

func (w *workbench) initRepoMenuItem() core.MenuItem {
	ws := w.workspaceState.Get().Workspace
	return core.MenuItem{
		Label: w.tr("Initialize new repo", "初始化新仓库"), Disabled: ws.ID == "" || w.workspaceBlocked(),
		OnSelect: func() { w.showWorkspaceForm(workspaceDialog{kind: "init", ws: ws}, "") },
	}
}

func (w *workbench) repositoryMenu() []core.MenuItem {
	init, clone, worktree := w.initRepoMenuItem(), w.cloneRepoMenuItem(), w.newWorktreeMenuItem()
	init.Label, clone.Label = w.tr("Init", "初始化"), w.tr("Clone", "克隆")
	worktree.Label = w.tr("New worktree", "新建 worktree")
	addDisabled := w.workspaceBlocked() || (w.workspaceStore != nil && w.workspaceState.Get().Workspace.ID == "")
	return []core.MenuItem{
		init, clone,
		{Label: w.tr("Add", "添加"), Disabled: addDisabled, OnSelect: w.pick},
		{Label: w.tr("Fetch", "获取"), Disabled: !w.canSync() || w.fetch.prompt.Get() != nil,
			OnSelect: w.scopedRefAction(func() { w.startFetch("") })},
		{Label: "LFS", Disabled: !w.canOpenLFS(), OnSelect: w.scopedRefAction(w.openLFS)},
		{Label: w.tr("Hooks", "Hooks"), Disabled: !w.canOpenRepoEditor(),
			OnSelect: w.scopedRefAction(func() { w.openRepoEditor(true) })},
		worktree,
	}
}

func (w *workbench) workspaceMenu() []core.MenuItem {
	add := core.MenuItem{Label: w.tr("Add local repository", "添加本地仓库"), OnSelect: w.pick,
		Disabled: w.workspaceBlocked()}
	if w.workspaceStore == nil {
		return []core.MenuItem{add}
	}
	state := w.workspaceState.Get()
	items := make([]core.MenuItem, 0, len(state.Workspaces)+5)
	for _, ws := range state.Workspaces {
		items = append(items, core.MenuItem{Label: ws.Name, Radio: true, Checked: ws.ID == state.Workspace.ID,
			Disabled: w.workspaceBlocked(), OnSelect: func() { w.selectWorkspace(ws.ID) }})
	}
	if len(items) != 0 {
		items = append(items, core.MenuItem{Separator: true})
	}
	items = append(items, w.workspaceActions(false)...)
	if state.Workspace.ID != "" {
		items = append(items, core.MenuItem{Separator: true}, w.initRepoMenuItem(), w.cloneRepoMenuItem(), add)
	}
	return items
}

func (w *workbench) workspaceActions(compact bool) []core.MenuItem {
	disabled := w.workspaceStore == nil || w.workspaceBlocked()
	ws := w.workspaceState.Get().Workspace
	create := core.MenuItem{Label: w.tr("New workspace", "新建工作区"), Disabled: disabled, OnSelect: w.newWorkspace}
	rename := core.MenuItem{Label: w.tr("Rename workspace…", "重命名工作区…"), Disabled: disabled || ws.ID == "",
		OnSelect: func() { w.showWorkspaceDialog("rename", ws) }}
	remove := core.MenuItem{Label: w.tr("Delete workspace…", "删除工作区…"), Disabled: disabled || ws.ID == "",
		OnSelect: func() { w.showWorkspaceDialog("delete", ws) }}
	ai := core.MenuItem{Label: w.tr("AI settings…", "AI 设置…"), Disabled: disabled || ws.ID == "", OnSelect: w.openAISettings}
	export := core.MenuItem{Label: w.tr("Export workspace…", "导出工作区…"), Disabled: disabled || ws.ID == "",
		OnSelect: func() { w.showWorkspaceForm(workspaceDialog{kind: "export", ws: ws}, transferFilename(ws.Name)) }}
	load := core.MenuItem{Label: w.tr("Import workspace…", "导入工作区…"), Disabled: disabled,
		OnSelect: func() { w.showWorkspaceForm(workspaceDialog{kind: "import"}, "") }}
	if compact {
		create.Label, rename.Label, ai.Label = w.tr("New", "新建"), w.tr("Rename", "重命名"), "AI"
		export.Label, load.Label, remove.Label = w.tr("Export", "导出"), w.tr("Import", "导入"), w.tr("Delete", "删除")
		return []core.MenuItem{create, rename, ai, export, load, remove}
	}
	return []core.MenuItem{create, rename, remove, ai, {Separator: true}, export, load}
}

func (w *workbench) workspaceDialogView(chrome palette) core.Node {
	return core.Show{When: func() bool { return w.workspaceModal.Get().kind != "" }, Then: func(*core.Scope) core.Node {
		dialog := w.workspaceModal.Get()
		if dialog.kind == "import" || dialog.kind == "export" {
			return w.workspaceTransferDialog(chrome, dialog)
		}
		if dialog.kind == "clone" {
			return w.cloneDialog(chrome, dialog)
		}
		title := w.tr("New workspace", "新建工作区")
		label := w.tr("Create", "创建")
		var description string
		switch dialog.kind {
		case "init":
			title = w.tr("Initialize new repo", "初始化新仓库")
			description = w.tr("Create a fresh Git repository in an empty folder.", "在空文件夹中创建全新的 Git 仓库。")
		case "rename":
			title = fmt.Sprintf(w.tr("Rename %q", "重命名「%s」"), dialog.ws.Name)
			label = w.tr("Save", "保存")
		case "delete":
			title = fmt.Sprintf(w.tr("Delete %q?", "删除「%s」？"), dialog.ws.Name)
			label = w.tr("Delete", "删除")
		case "relink":
			title = fmt.Sprintf(w.tr("Relink %q", "重新关联「%s」"), dialog.repo.Path)
			label = w.tr("Relink", "重新关联")
		case "remove":
			title = fmt.Sprintf(w.tr("Remove %q?", "移除「%s」？"), filepath.Base(dialog.repo.Path))
			label = w.tr("Remove", "移除")
		}
		disabled := func() bool {
			return w.workspaceBusy.Get() || (dialog.hasInput() && strings.TrimSpace(w.workspaceInput.Get()) == "")
		}
		confirm := func() {
			if w.workspaceModal.Get() != dialog || disabled() {
				return
			}
			input := w.workspaceInput.Get()
			if dialog.kind != "init" {
				input = strings.TrimSpace(input)
			}
			w.runWorkspace(func(ctx context.Context) (workspaceResult, error) {
				switch dialog.kind {
				case "init":
					return w.initWorkspaceRepo(ctx, dialog.ws.ID, input)
				case "rename":
					return workspaceResult{}, w.workspaceStore.Rename(ctx, dialog.ws.ID, input)
				case "delete":
					return workspaceResult{}, w.workspaceStore.Delete(ctx, dialog.ws.ID)
				case "remove":
					return workspaceResult{}, w.workspaceStore.RemoveRepo(ctx, dialog.ws.ID, dialog.repo)
				case "relink":
					repo, err := gitcli.OpenRoot(ctx, input)
					if err != nil {
						return workspaceResult{}, fmt.Errorf("%s: %w", w.tr("Could not open this repository", "无法打开此仓库"), err)
					}
					err = w.workspaceStore.RelinkRepo(ctx, dialog.ws.ID, dialog.repo, repo.Path())
					if errors.Is(err, workspace.ErrRepoPathInUse) {
						err = errors.New(w.tr("This repository is already in the workspace", "此仓库已在工作区中"))
					}
					return workspaceResult{}, err
				default:
					ws, err := w.workspaceStore.CreateWithRepos(ctx, input, nil)
					return workspaceResult{workspaceID: ws.ID}, err
				}
			})
		}
		cancel := func() {
			if !w.workspaceBusy.Get() {
				w.workspaceModal.Set(workspaceDialog{})
			}
		}
		var body core.Node
		if dialog.hasInput() {
			input := core.Input{
				TestID: "workspace-name", Value: w.workspaceInput.Get, OnChange: w.workspaceInput.Set,
				Placeholder: w.tr("Workspace name", "工作区名称"), AutoFocus: true,
				OnSubmit: func(string) { confirm() },
				Style:    core.Style{Height: core.Pt(32), Padding: core.All(6), BorderWidth: 1, BorderColor: chrome.border},
			}
			body = input
			if dialog.kind == "relink" || dialog.kind == "init" {
				if dialog.kind == "init" {
					input.TestID = "repo-init-path"
					input.Label = w.tr("Location", "位置")
					input.Placeholder = w.tr("Path to the new repository", "新仓库的路径")
					input.Disabled = w.workspaceBusy.Get
				} else {
					input.TestID = "repo-relink-path"
					input.Placeholder = w.tr("New path to a valid git working tree", "指向有效 git 工作树的新路径")
				}
				input.Style.FlexGrow = 1
				body = core.View{Style: core.Style{Direction: core.Row, Gap: 6}, Children: []core.Node{
					input,
					action(chrome, w.tr("Browse…", "浏览…"), func() { w.pickWorkspaceDirectory(dialog, title) }),
				}}
				if dialog.kind == "init" {
					body = core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
						chrome.label(input.Label, chrome.muted, 12), body,
					}}
				}
			}
		} else if dialog.kind == "remove" {
			description = fmt.Sprintf(w.tr("Path: %s", "路径：%s"), dialog.repo.Path)
			body = core.Text{Content: w.tr("Removes the workspace reference. The local directory and its .git/ folder are not touched.",
				"仅移除工作区中的引用。本地目录及其 .git/ 文件夹不会被改动。")}
		} else {
			body = core.Text{Content: w.tr("This removes the workspace and its repo references from GitWave. It does not delete the local repositories themselves.",
				"将移除该工作区及其在 GitWave 中的仓库引用。本地仓库仍保留。")}
		}
		buttonColor := chrome.text
		if !dialog.hasInput() {
			buttonColor = chrome.danger
		}
		button := toolbarAction(chrome, label, confirm, chrome.label(label, buttonColor, 12))
		button.Disabled = disabled
		return ui.Dialog{
			TestID: "workspace-dialog", Title: title, Description: description,
			Open: func() bool { return w.workspaceModal.Get().kind != "" },
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					cancel()
				}
			},
			Children: []core.Node{
				body,
				core.Text{ContentFunc: w.workspaceError.Get, Style: core.Style{Color: chrome.danger, FontSize: chrome.uiSize(12)}},
			},
			Footer: []core.Node{
				action(chrome, w.tr("Cancel", "取消"), cancel),
				button,
			},
		}
	}}
}

func (w *workbench) initWorkspaceRepo(ctx context.Context, ws, path string) (workspaceResult, error) {
	if _, err := w.workspaceStore.LoadWorkspace(ctx, ws); err != nil {
		return workspaceResult{}, err
	}
	repo, err := gitcli.Init(ctx, path)
	if err != nil {
		return workspaceResult{}, err
	}
	return w.registerWorkspaceRepo(ctx, ws, repo)
}

func (w *workbench) registerWorkspaceRepo(ctx context.Context, ws string, repo *gitcli.Repository) (workspaceResult, error) {
	_, id, err := w.workspaceStore.AddRepos(ctx, ws, []string{repo.Path()})
	if err != nil {
		message := w.tr("The repository at %s is ready, but could not be added to the workspace", "%s 的仓库已就绪，但无法添加到工作区")
		return workspaceResult{}, fmt.Errorf("%s: %w", fmt.Sprintf(message, repo.Path()), err)
	}
	if id == "" {
		return workspaceResult{}, errors.New(w.tr("This repository is already in the workspace", "此仓库已在工作区中"))
	}
	return workspaceResult{workspaceID: ws, repoID: id}, nil
}

func repoTabName(repo workspace.Repo) string {
	if repo.Nickname != "" {
		return repo.Nickname
	}
	return filepath.Base(repo.Path)
}
