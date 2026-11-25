package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"gitwave.work/internal/gitcli"
	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestWorkspacePathUsesExistingProfileOnlyForPackagedStableApp(t *testing.T) {
	for _, tc := range []struct {
		app  core.AppInfo
		want string
	}{
		{core.AppInfo{ID: "desktop.gitwave.work"}, "GitWave"},
		{core.AppInfo{ID: "desktop.gitwave.work", Development: true}, "GitWave Preview"},
		{core.AppInfo{ID: "desktop.gitwave.work.native-preview"}, "GitWave Preview"},
		{core.AppInfo{}, "GitWave Preview"},
	} {
		path, err := workspacePathForApp(tc.app)
		if err != nil || filepath.Base(path) != "state.db" || filepath.Base(filepath.Dir(path)) != tc.want {
			t.Fatalf("app %+v selected %q, %v; want %s/state.db", tc.app, path, err, tc.want)
		}
	}
}

func TestNativeWorkspaceInitializesRepository(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	store, err := workspace.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.CreateWithRepos(t.Context(), "New project", nil); err != nil {
		t.Fatal(err)
	}
	app := workspaceApp(t, store)
	const timeout = 8 * time.Second
	open := func() {
		app.Press("id:workspace-selector")
		app.ChoosePopup("Initialize new repo")
		app.WaitFor("id:repo-init-path", timeout)
	}
	name := "New Repository"
	if runtime.GOOS != "windows" {
		name += " "
	}
	dest := filepath.Join(t.TempDir(), name)
	app.WaitFor("text:New project", timeout)
	open()
	app.Type("id:repo-init-path", dest)
	app.Press("label:Cancel")
	app.WaitForGone("id:workspace-dialog", timeout)
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		t.Fatalf("cancel created the destination: %v", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec("CREATE TRIGGER reject_repo BEFORE INSERT ON repos " +
		"BEGIN SELECT RAISE(ABORT, 'test write failure'); END"); err != nil {
		t.Fatal(err)
	}
	open()
	app.Type("id:repo-init-path", dest)
	app.Press("label:Create")
	app.WaitFor("text:/could not be added to the workspace/", timeout)
	repo, err := gitcli.OpenRoot(t.Context(), dest)
	if err != nil {
		t.Fatalf("workspace save failure lost the initialized repository: %v", err)
	}
	state, err := store.Load(t.Context())
	if err != nil || len(state.Repos) != 0 {
		t.Fatalf("workspace save failure left a partial registration: %+v, %v", state, err)
	}
	if _, err := db.Exec("DROP TRIGGER reject_repo"); err != nil {
		t.Fatal(err)
	}
	app.Press("label:Create")
	app.WaitForGone("id:workspace-dialog", timeout)
	app.WaitFor("id:repository-tabs", timeout)
	state, err = store.Load(t.Context())
	if err != nil || len(state.Repos) != 1 || state.Repo.Path != repo.Path() {
		t.Fatalf("initialization did not register and activate the repository: %+v, %v", state, err)
	}
	status, err := repo.Status(t.Context())
	if err != nil || status.HeadRef != "refs/heads/main" || status.HeadOID != "(initial)" || len(status.Changes) != 0 {
		t.Fatalf("expected an empty working repository on main: %+v, %v", status, err)
	}
	open()
	app.Type("id:repo-init-path", dest)
	app.Press("label:Create")
	app.WaitFor("text:This repository is already in the workspace", timeout)
	state, err = store.Load(t.Context())
	if err != nil || len(state.Repos) != 1 {
		t.Fatalf("retry added a duplicate repository: %+v, %v", state, err)
	}
}

func TestNativeWorkspaceCreatesAddsAndSwitches(t *testing.T) {
	if os.Getenv("NSGO_TEST_TARGET") != "" {
		t.Skip("requires a scripted folder picker")
	}
	a, gitA := historyRepository(t)
	b, gitB := historyRepository(t)
	for name, content := range map[string]string{"a.txt": "First history file\n", "b.txt": "Remember this history file\n"} {
		if err := os.WriteFile(filepath.Join(a, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gitA("", "add", "a.txt", "b.txt")
	gitA("", "commit", "-m", "History from repository A")
	gitB("", "commit", "--allow-empty", "-m", "History from repository B")
	invalid := t.TempDir()
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := workspace.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	app := workspaceApp(t, store)
	const timeout = 5 * time.Second
	app.WaitFor("text:Select a workspace", timeout)
	app.Find("text:No workspace selected")
	app.WaitForGone("id:split-history-details", timeout)
	for _, selector := range []string{"id:navigate-history", "id:navigate-changes", "label:Search commits", "label:Focus diff"} {
		if !app.Find(selector).Props.Disabled {
			t.Fatalf("%s should be disabled without an active repository", selector)
		}
	}
	app.Press("id:workspace-selector")
	app.ChoosePopup("New workspace")
	app.WaitFor("id:workspace-name", timeout)
	app.Type("id:workspace-name", "Project A")
	app.Press("label:Create")
	app.WaitForGone("id:workspace-dialog", timeout)
	app.WaitFor("text:Project A", timeout)
	app.PickDirectories = func(nsgotest.DirectoryPick) []core.Directory {
		return []core.Directory{{Path: a}, {Path: invalid}, {Path: a}, {Path: b}}
	}
	app.Find("text:/select a repository tab/")
	app.Press("id:workspace-selector")
	app.ChoosePopup("Add local repository")
	app.WaitFor("label:/History from repository B/", timeout)
	app.WaitFor("text:No commit selected", timeout)
	if app.Find("label:Search commits").Props.Disabled || app.Find("label:Focus diff").Props.Disabled {
		t.Fatal("repository workbench controls stayed disabled after opening a repository")
	}
	app.WaitFor("text:/Could not add/", timeout)
	app.Press("label:" + filepath.Base(a))
	app.WaitFor("label:/History from repository A/", timeout)
	app.WaitForGone("label:/History from repository B/", timeout)
	app.Press("label:/History from repository A/")
	app.WaitFor("label:b.txt", timeout)
	app.Press("label:b.txt")
	app.WaitFor("text:Remember this history file", timeout)
	app.Press("label:" + filepath.Base(b))
	app.WaitFor("text:No commit selected", timeout)
	app.WaitForGone("text:Remember this history file", timeout)
	tree := gitA("", "rev-parse", "HEAD^{tree}")
	treePath := filepath.Join(a, ".git", "objects", tree[:2], tree[2:])
	savedTree := filepath.Join(t.TempDir(), "tree")
	if err := os.Rename(treePath, savedTree); err != nil {
		t.Fatal(err)
	}
	app.Press("label:" + filepath.Base(a))
	app.WaitFor("id:commit-header >> text:History from repository A", timeout)
	app.WaitFor("id:split-history-details >> label:Retry", timeout)
	app.Press("label:" + filepath.Base(b))
	app.WaitFor("text:No commit selected", timeout)
	app.Press("label:" + filepath.Base(a))
	app.WaitFor("id:split-history-details >> label:Retry", timeout)
	if err := os.Rename(savedTree, treePath); err != nil {
		t.Fatal(err)
	}
	app.Press("id:split-history-details >> label:Retry")
	app.WaitFor("text:Remember this history file", timeout)

	app.Press("id:workspace-selector")
	app.ChoosePopup("New workspace")
	app.Type("id:workspace-name", "Project B")
	app.Press("label:Create")
	app.WaitForGone("id:workspace-dialog", timeout)
	app.WaitFor("text:Project B", timeout)
	app.WaitFor("text:/No repositories — add one/", timeout)
	app.Press("id:workspace-selector")
	app.ChoosePopup("Project A")
	app.WaitFor("label:/History from repository A/", timeout)
	app.WaitFor("text:Remember this history file", timeout)

	reopened, err := workspace.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	state, err := reopened.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if state.Workspace.Name != "Project A" || len(state.Repos) != 2 || filepath.Base(state.Repo.Path) != filepath.Base(a) {
		t.Fatalf("persisted workspace after switching: %+v", state)
	}
	app.Press("id:workspace-selector")
	app.ChoosePopup("Rename workspace…")
	app.Clear("id:workspace-name")
	app.Type("id:workspace-name", "Renamed project")
	app.Press("label:Save")
	app.WaitForGone("id:workspace-dialog", timeout)
	app.WaitFor("text:Renamed project", timeout)
	base := gitA("", "rev-parse", "HEAD")
	gitA("", "commit", "--allow-empty", "-m", "Temporary history selection")
	removed := gitA("", "rev-parse", "HEAD")
	app.Key("mod+r")
	app.WaitFor("label:/Temporary history selection/", timeout)
	app.Press("label:/Temporary history selection/")
	app.WaitFor("id:commit-header >> text:Temporary history selection", timeout)
	app.Press("label:" + filepath.Base(b))
	app.WaitFor("label:/History from repository B/", timeout)
	gitA("", "update-ref", "refs/heads/main", base, removed)
	if err := os.Remove(filepath.Join(a, ".git", "objects", removed[:2], removed[2:])); err != nil {
		t.Fatal(err)
	}
	app.Press("label:" + filepath.Base(a))
	app.WaitFor("label:/History from repository A/", timeout)
	app.WaitFor("text:No commit selected", timeout)
	app.Press("id:workspace-selector")
	app.ChoosePopup("Delete workspace…")
	app.Press("label:Cancel")
	app.WaitForGone("id:workspace-dialog", timeout)
	app.WaitFor("label:/History from repository A/", timeout)
	app.Press("id:workspace-selector")
	app.ChoosePopup("Delete workspace…")
	app.Press("label:Delete")
	app.WaitForGone("id:repository-tabs", timeout)
	app.WaitFor("text:Select a workspace", timeout)
	if _, err := os.Stat(filepath.Join(a, ".git")); err != nil {
		t.Fatalf("workspace deletion affected a local repository: %v", err)
	}
}

func TestNativeWorkspaceRestoresAfterMissingRepository(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Available repository history")
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := workspace.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Create(t.Context(), "Recovered workspace"); err != nil {
		t.Fatal(err)
	}
	state, err := store.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "vanished")
	if _, _, err := store.AddRepos(t.Context(), state.Workspace.ID, []string{repo, missing}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = workspace.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	app := workspaceApp(t, store)
	const timeout = 5 * time.Second
	app.WaitFor("label:/Available repository history/", timeout)
	app.Find("label:vanished (missing)")
}

func TestWorkspaceRefreshRecoversRestoredGitDirectory(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Recoverable history")
	store, err := workspace.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Create(t.Context(), "Git directory restored"); err != nil {
		t.Fatal(err)
	}
	state, err := store.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.AddRepos(t.Context(), state.Workspace.ID, []string{repo}); err != nil {
		t.Fatal(err)
	}
	gitDir, backup := filepath.Join(repo, ".git"), filepath.Join(repo, "backup.git")
	if err := os.Rename(gitDir, backup); err != nil {
		t.Fatal(err)
	}
	missing, err := loadWorkspace(t.Context(), store)
	if err != nil || len(missing.Repos) != 1 || !missing.Repos[0].Missing || missing.Repo.ID != "" {
		t.Fatalf("after removing Git directory: %+v, %v", missing, err)
	}
	if err := os.Rename(backup, gitDir); err != nil {
		t.Fatal(err)
	}
	restored, err := loadWorkspace(t.Context(), store)
	if err != nil || restored.Repo.ID == "" || restored.Repo.Missing || restored.Repo.Path != repo {
		t.Fatalf("after restoring Git directory: %+v, %v", restored, err)
	}
}

func TestNativeWorkspaceRepoTabs(t *testing.T) {
	a, gitA := historyRepository(t)
	b, gitB := historyRepository(t)
	c, gitC := historyRepository(t)
	gitA("", "commit", "--allow-empty", "-m", "Tab A history")
	gitB("", "commit", "--allow-empty", "-m", "Tab B history")
	gitC("", "commit", "--allow-empty", "-m", "Relinked history")
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := workspace.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Create(t.Context(), "Tab actions"); err != nil {
		t.Fatal(err)
	}
	initial, err := store.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "unavailable")
	if _, _, err := store.AddRepos(t.Context(), initial.Workspace.ID, []string{a, missing, b}); err != nil {
		t.Fatal(err)
	}
	app := workspaceApp(t, store)
	const timeout = 5 * time.Second
	app.WaitFor("label:/Tab B history/", timeout)
	tabA, tabB, tabC := "label:"+filepath.Base(a), "label:"+filepath.Base(b), "label:"+filepath.Base(c)
	const missingTab = "label:unavailable (missing)"
	origin := app.Find(tabA)
	last := app.Find(tabB)
	dx := last.Frame.Origin.X + last.Frame.Size.Width - (origin.Frame.Origin.X + origin.Frame.Size.Width/2)
	app.Drag(tabA, core.Point{X: dx}, core.Point{})
	app.WaitUntil(timeout, func() bool { return !app.Find("id:repository-tabs-scroll").Props.Busy })
	unchanged, err := store.Load(t.Context())
	if err != nil || unchanged.Repos[0].Path != a || unchanged.Repo.Path != b {
		t.Fatalf("returning the drag to its starting point changed the saved state: %+v, %v", unchanged, err)
	}
	app.Drag(tabA, core.Point{X: dx})
	app.WaitUntil(timeout, func() bool {
		state, err := store.Load(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		return state.Repos[0].Path == missing && state.Repos[2].Path == a &&
			!app.Find("id:repository-tabs-scroll").Props.Busy && app.Find(tabA).Frame.Origin.X > app.Find(tabB).Frame.Origin.X
	})
	app.Find("label:/Tab B history/")

	app.Focus(tabB)
	app.Key("left")
	app.WaitUntil(timeout, func() bool { return app.Find(missingTab).Focused })
	app.Key("shift+f10")
	app.ChoosePopup("Move Right")
	app.WaitUntil(timeout, func() bool {
		return app.Find(tabB).Frame.Origin.X < app.Find(missingTab).Frame.Origin.X
	})
	app.RightClick(missingTab)
	app.ChoosePopup("Relink…")
	app.WaitFor("id:repo-relink-path", timeout)
	setPath := func(value string) {
		app.Clear("id:repo-relink-path")
		app.Type("id:repo-relink-path", value)
		app.WaitUntil(timeout, func() bool { return app.Find("id:repo-relink-path").Text == value })
	}
	setPath(a)
	app.Press("label:Relink")
	app.WaitFor("text:This repository is already in the workspace", timeout)
	setPath(c)
	app.Press("label:Relink")
	app.WaitForGone("id:workspace-dialog", timeout)
	app.WaitForGone(missingTab, timeout)
	app.WaitFor(tabC, timeout)
	app.Find("label:/Tab B history/")

	app.RightClick(tabB)
	app.ChoosePopup("Remove")
	app.Press("label:Cancel")
	app.WaitForGone("id:workspace-dialog", timeout)
	app.Find(tabB)
	app.RightClick(tabB)
	app.ChoosePopup("Remove")
	app.Press("label:Remove")
	app.WaitFor("label:/Relinked history/", timeout)
	app.WaitForGone(tabB, timeout)
	app.Focus(tabC)
	app.Key("end")
	app.WaitFor("label:/Tab A history/", timeout)
	app.Key("home")
	app.WaitFor("label:/Relinked history/", timeout)
	reopened, err := workspace.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	state, err := reopened.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	canonicalC, err := filepath.EvalSymlinks(c)
	if err != nil {
		t.Fatal(err)
	}
	if state.Repo.Path != canonicalC || state.Workspace.LastRepoID != state.Repo.ID || len(state.Repos) != 2 || state.Repos[1].Path != a {
		t.Fatalf("unexpected repository order or selection after reopen: %+v", state)
	}
	if _, err := os.Stat(filepath.Join(b, ".git")); err != nil {
		t.Fatalf("removing a tab affected a local repository: %v", err)
	}
}

func TestRepositoryTabsRevealSelectionAndMapWheel(t *testing.T) {
	repos := []workspace.Repo{
		{ID: "1", Path: "/repos/first", Nickname: "A long first repository name"},
		{ID: "2", Path: "/repos/second", Nickname: "A long second repository name"},
		{ID: "3", Path: "/repos/third", Nickname: "A long third repository name"},
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.workspaceState.Set(workspace.State{
			Workspace: workspace.Summary{ID: "test", Name: "Tab overflow"}, Repos: repos, Repo: repos[2],
		})
		chrome := colors(false)
		return core.View{Children: []core.Node{
			w.repositoryTabs(chrome),
			action(chrome, "Select middle tab", func() {
				next := w.workspaceState.Get()
				next.Repo = repos[1]
				w.workspaceState.Set(next)
			}),
			action(chrome, "Remove first tab", func() {
				next := w.workspaceState.Get()
				next.Repos = repos[1:]
				w.workspaceState.Set(next)
			}),
		}}
	}, nsgotest.Size(270, 90), nsgotest.Environment(core.Environment{
		OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en",
	}))
	const selector = "id:repository-tabs-scroll"
	const timeout = 5 * time.Second
	app.WaitUntil(timeout, func() bool { return app.Find(selector).Offset.X > 0 })
	start := app.Find(selector).Offset.X
	tab := "label:" + repos[2].Nickname
	if os.Getenv("NSGO_TEST_TARGET") == "" {
		app.Wheel(tab, core.Point{Y: 90}, core.Modifiers{})
	} else {
		app.TouchpadScroll(tab, core.Point{}, core.Point{Y: 90})
	}
	app.WaitUntil(timeout, func() bool { return app.Find(selector).Offset.X < start })
	app.Press("label:Select middle tab")
	app.WaitUntil(timeout, func() bool {
		return app.Find("label:"+repos[1].Nickname).Props.Selected && app.Find(selector).Offset.X > 0
	})
	app.Press("label:Remove first tab")
	app.WaitUntil(timeout, func() bool { return app.Find(selector).Offset.X == 0 })
}

func workspaceApp(t *testing.T, store *workspace.Store) *nsgotest.App {
	t.Helper()
	return nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.connectWorkspace(store, nil)
		return w.view()
	}, nsgotest.Size(1200, 800), nsgotest.Environment(core.Environment{
		OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en",
	}))
}
