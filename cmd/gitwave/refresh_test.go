package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func refreshEnvironment(phase core.Lifecycle) core.Environment {
	return core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en", Lifecycle: phase, ReduceMotion: true}
}

func refreshApp(t *testing.T, store *workspace.Store, path string) (*nsgotest.App, func(bool)) {
	t.Helper()
	native := os.Getenv("NSGO_TEST_TARGET") != ""
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		if store == nil {
			w.open(path)
		} else {
			w.connectWorkspace(store, nil)
		}
		return w.view()
	}, nsgotest.Size(1200, 800), nsgotest.Environment(refreshEnvironment(core.LifecycleActive)))
	setActive := func(active bool) {
		t.Helper()
		if native {
			// The native test host cannot inject Lifecycle, and AppKit may
			// refuse focus after a test window is hidden or minimized. Use
			// the same refresh path via its normal keyboard entry there.
			if active {
				app.Key("mod+r")
			}
			return
		}
		phase := core.LifecycleInactive
		if active {
			phase = core.LifecycleActive
		}
		app.SetEnvironment(refreshEnvironment(phase))
	}
	return app, setActive
}

func TestWorkbenchRefreshPreservesLocatedCommitAndDiff(t *testing.T) {
	repo, git := historyRepository(t)
	mustWrite := func(contents string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte(contents+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		git("", "add", "tracked.txt")
	}
	mustWrite("original line")
	git("", "commit", "-m", "Outside search")
	mustWrite("retained patch line")
	git("", "commit", "-m", "Needle selected commit")
	selected := git("", "rev-parse", "HEAD")
	app, setActive := refreshApp(t, nil, repo)
	const timeout = 5 * time.Second
	app.WaitFor("label:/Needle selected commit/", timeout)
	app.Press("label:Search commits")
	app.WaitFor("id:palette-input", timeout)
	app.Type("id:palette-input", "needle")
	app.WaitFor("id:palette-row-"+selected, timeout)
	app.Press("id:palette-row-" + selected)
	app.WaitForGone("id:command-palette", timeout)
	app.Find("label:/Outside search/")
	app.WaitFor("text:retained patch line", timeout)
	setActive(false)
	git("", "commit", "--allow-empty", "-m", "Needle landed outside GitWave")
	git("", "commit", "--allow-empty", "-m", "Unrelated external change")
	if app.Exists("label:/Needle landed outside GitWave/") {
		t.Fatal("History changed before the window became active")
	}
	setActive(true)
	app.WaitFor("label:/Needle landed outside GitWave/", timeout)
	app.WaitFor("text:retained patch line", timeout)
	app.Find("id:commit-header")
	app.WaitFor("label:/Unrelated external change/", timeout)
	if !app.Find("label:/Needle selected commit/").Props.Selected {
		t.Fatal("refresh lost the selected commit")
	}
}

func TestNativeRefreshFetchesActiveRepoAndSweepsWorkspaceOnTimer(t *testing.T) {
	remote, upstream := historyRepository(t)
	upstream("", "commit", "--allow-empty", "-m", "Initial tracking tip")
	base := upstream("", "rev-parse", "HEAD")
	a, gitA := historyRepository(t)
	b, gitB := historyRepository(t)
	bad, gitBad := historyRepository(t)
	for _, git := range []func(string, ...string) string{gitA, gitB} {
		git("", "remote", "add", "origin", remote)
		git("", "fetch", "origin")
		git("", "reset", "--hard", "origin/main")
	}
	gitBad("", "commit", "--allow-empty", "-m", "Disconnected repository")
	gitBad("", "remote", "add", "origin", filepath.Join(t.TempDir(), "absent"))
	store, err := workspace.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.CreateWithRepos(t.Context(), "Auto refresh workspace", []string{a, bad, b}); err != nil {
		t.Fatal(err)
	}
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	app.WaitFor("label:/Initial tracking tip/", timeout)
	upstream("", "commit", "--allow-empty", "-m", "Manual refresh origin update")
	app.Key("mod+r")
	app.WaitFor("text:Repository refreshed", timeout)
	app.WaitFor("label:/Manual refresh origin update/", timeout)
	if got := gitB("", "rev-parse", "origin/main"); got != base {
		t.Fatalf("manual refresh fetched from an inactive repo: %q", got)
	}

	app.Key("mod+,")
	app.WaitFor("id:settings-dialog", timeout)
	if input := app.Find("id:auto-refresh-interval"); !input.Props.Disabled {
		t.Fatal("automatic refresh should start disabled")
	}
	app.Press("id:auto-refresh")
	app.WaitUntil(timeout, func() bool { return !app.Find("id:auto-refresh-interval").Props.Disabled })
	app.Clear("id:auto-refresh-interval")
	app.Type("id:auto-refresh-interval", "1")
	app.Submit("id:auto-refresh-interval")
	app.WaitUntil(timeout, func() bool {
		prefs, err := store.RefreshPreferences(t.Context())
		return err == nil && prefs.Enabled && prefs.Minutes == 1 && !app.Find("id:auto-refresh-interval").Props.Disabled
	})
	app.Press("label:Close")
	app.WaitForGone("id:settings-dialog", timeout)
	upstream("", "commit", "--allow-empty", "-m", "Scheduled origin update")
	newTip := upstream("", "rev-parse", "HEAD")
	app.FastForward(time.Minute)
	app.WaitFor("text:/Refreshed 2/3 repositories/", timeout)
	app.WaitFor("label:/Scheduled origin update/", timeout)
	for _, git := range []func(string, ...string) string{gitA, gitB} {
		if tracking, head := git("", "rev-parse", "origin/main"), git("", "rev-parse", "HEAD"); tracking != newTip || head != base {
			t.Fatalf("automatic refresh changed the wrong refs: tracking %s, HEAD %s", tracking, head)
		}
	}

	app.Key("mod+,")
	app.WaitFor("id:settings-dialog", timeout)
	upstream("", "commit", "--allow-empty", "-m", "Update on the new schedule")
	app.FastForward(time.Minute)
	app.Clear("id:auto-refresh-interval")
	app.Type("id:auto-refresh-interval", "2")
	app.Submit("id:auto-refresh-interval")
	app.WaitUntil(timeout, func() bool {
		prefs, err := store.RefreshPreferences(t.Context())
		return err == nil && prefs.Minutes == 2 && !app.Find("id:auto-refresh-interval").Props.Disabled
	})
	app.Press("label:Close")
	app.WaitForGone("id:settings-dialog", timeout)
	app.FastForward(time.Minute)
	if got := gitA("", "rev-parse", "origin/main"); got != newTip || app.Exists("id:refresh-cancel") {
		t.Fatalf("old timer fetched after its interval changed: %q", got)
	}
	app.FastForward(time.Minute)
	app.WaitFor("label:/Update on the new schedule/", timeout)
	newTip = upstream("", "rev-parse", "HEAD")

	app.Key("mod+,")
	app.WaitFor("id:settings-dialog", timeout)
	app.Press("id:auto-refresh")
	app.WaitUntil(timeout, func() bool {
		view := app.Find("id:auto-refresh")
		return !view.Props.Disabled && !view.Props.Checked
	})
	app.Press("label:Close")
	app.WaitForGone("id:settings-dialog", timeout)
	upstream("", "commit", "--allow-empty", "-m", "Update after disabling refresh")
	app.FastForward(2 * time.Minute)
	if got := gitA("", "rev-parse", "origin/main"); got != newTip || app.Exists("id:refresh-cancel") {
		t.Fatalf("disabled automatic refresh still fetched: %q", got)
	}
}

func TestNativeManualRefreshDoesNotFetchFallbackRepo(t *testing.T) {
	a, gitA := historyRepository(t)
	b, gitB := historyRepository(t)
	upstream, gitUpstream := historyRepository(t)
	gitA("", "commit", "--allow-empty", "-m", "Active repository before removal")
	gitB("", "commit", "--allow-empty", "-m", "Fallback repository stays local")
	gitUpstream("", "commit", "--allow-empty", "-m", "Unrequested remote update")
	gitB("", "remote", "add", "origin", upstream)
	store, err := workspace.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.CreateWithRepos(t.Context(), "Refresh target", []string{a, b}); err != nil {
		t.Fatal(err)
	}
	app, _ := refreshApp(t, store, "")
	const timeout = 8 * time.Second
	app.WaitFor("label:/Active repository before removal/", timeout)
	if err := os.Rename(filepath.Join(a, ".git"), filepath.Join(a, "git-away")); err != nil {
		t.Fatal(err)
	}
	app.Key("mod+r")
	app.WaitFor("label:/Fallback repository stays local/", timeout)
	if got := gitB("", "for-each-ref", "refs/remotes"); got != "" || app.Exists("id:refresh-status") {
		t.Fatalf("a refresh requested for the missing repository fetched into its fallback: %q", got)
	}
}

func TestNativeWorkspaceSweepSurvivesRepositorySwitch(t *testing.T) {
	a, gitA := historyRepository(t)
	b, gitB := historyRepository(t)
	upstream, gitUpstream := historyRepository(t)
	gitA("", "commit", "--allow-empty", "-m", "Slow first repository")
	gitB("", "commit", "--allow-empty", "-m", "Second repository")
	gitUpstream("", "commit", "--allow-empty", "-m", "Second repository remote update")
	gitB("", "remote", "add", "origin", upstream)
	started := make(chan struct{}, 1)
	released, release := context.WithCancel(t.Context())
	defer release()
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-released.Done():
			w.WriteHeader(http.StatusNotFound)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(remote.Close)
	gitA("", "remote", "add", "origin", remote.URL)
	store, err := workspace.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.CreateWithRepos(t.Context(), "Running workspace", []string{a, b}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRefreshPreferences(t.Context(), workspace.RefreshPreferences{Enabled: true, Minutes: 1}); err != nil {
		t.Fatal(err)
	}
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	app.WaitFor("label:/Slow first repository/", timeout)
	app.FastForward(time.Minute)
	app.WaitFor("id:refresh-cancel", timeout)
	select {
	case <-started:
	case <-time.After(timeout):
		t.Fatal("refresh did not reach the first repository's remote")
	}
	app.Press("label:" + filepath.Base(b))
	app.WaitFor("label:/^Second repository,/", timeout)
	app.Find("id:refresh-cancel")
	release()
	app.WaitFor("text:/Refreshed 1/2 repositories/", timeout)
	app.WaitFor("label:/Second repository remote update/", timeout)
}

func TestWorkbenchRefreshRepairsMissingRepoAndKeepsWorkspace(t *testing.T) {
	a, gitA := historyRepository(t)
	b, gitB := historyRepository(t)
	gitA("", "commit", "--allow-empty", "-m", "Repo that disappears")
	gitB("", "commit", "--allow-empty", "-m", "Fallback history")
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := workspace.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	current, err := store.CreateWithRepos(t.Context(), "Focused workspace", []string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	app, setActive := refreshApp(t, store, "")
	const timeout = 5 * time.Second
	app.WaitFor("label:/Repo that disappears/", timeout)
	app.Press("id:workspace-selector")
	app.ChoosePopup("New workspace")
	app.WaitFor("id:workspace-dialog", timeout)

	setActive(false)
	other, err := workspace.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err := other.Create(t.Context(), "Workspace selected in another window"); err != nil {
		t.Fatal(err)
	}
	gitDir, backup := filepath.Join(a, ".git"), filepath.Join(a, "git-away")
	if err := os.Rename(gitDir, backup); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("NSGO_TEST_TARGET") == "" {
		setActive(true)
		app.WaitFor("id:workspace-dialog", timeout)
		app.Press("label:Cancel")
	} else {
		app.Press("label:Cancel")
		app.WaitForGone("id:workspace-dialog", timeout)
		setActive(true)
	}
	app.WaitFor("label:/Fallback history/", timeout)
	app.Find("text:/^Focused workspace$/")
	missingTab := "label:" + filepath.Base(a) + " (missing)"
	app.Find(missingTab)
	actual, err := store.LoadWorkspace(t.Context(), current.ID)
	if err != nil || actual.Repo.Path != b || actual.Workspace.LastRepoID != actual.Repo.ID {
		t.Fatalf("fallback not persisted: %+v, %v", actual, err)
	}
	startup, err := other.Load(t.Context())
	if err != nil || startup.Workspace.Name != "Workspace selected in another window" {
		t.Fatalf("refresh changed the startup selection: %+v, %v", startup, err)
	}
	setActive(false)
	if err := os.Rename(backup, gitDir); err != nil {
		t.Fatal(err)
	}
	setActive(true)
	app.WaitForGone(missingTab, timeout)
	app.WaitFor("label:/^"+filepath.Base(a)+"$/", timeout)
	app.Find("label:/Fallback history/")

	if err := other.SelectRepo(t.Context(), current.ID, current.LastRepoID); err != nil {
		t.Fatal(err)
	}
	if err := other.Create(t.Context(), "A different startup workspace"); err != nil {
		t.Fatal(err)
	}
	app.Press("id:workspace-selector")
	app.ChoosePopup("Rename workspace…")
	app.WaitFor("id:workspace-name", timeout)
	app.Clear("id:workspace-name")
	app.Type("id:workspace-name", "Renamed focused workspace")
	app.Press("label:Save")
	app.WaitForGone("id:workspace-dialog", timeout)
	app.WaitFor("text:/^Renamed focused workspace$/", timeout)
	app.Find("label:/Fallback history/")
	startup, err = other.Load(t.Context())
	if err != nil || startup.Workspace.Name != "A different startup workspace" {
		t.Fatalf("rename changed the startup selection: %+v, %v", startup, err)
	}
}

func TestWorkbenchRefreshKeepsScrolledCommitInView(t *testing.T) {
	repo, git := historyRepository(t)
	var stream strings.Builder
	for i := range 40 {
		message := fmt.Sprintf("Numbered commit %02d", i)
		fmt.Fprintf(&stream, "commit refs/heads/main\ncommitter GitWave <test@example.invalid> %d +0000\ndata %d\n%s\n\n",
			1700000000+i, len(message), message)
	}
	git(stream.String(), "fast-import", "--quiet")
	app, setActive := refreshApp(t, nil, repo)
	const timeout = 5 * time.Second
	const list = "id:history-commit-list"
	app.WaitFor(list, timeout)
	app.Scroll(list, 8*historyRowHeight+6)
	start := app.Find(list).Offset.Y
	if start < 8*historyRowHeight {
		t.Fatalf("test did not scroll: %v", start)
	}
	setActive(false)
	git("", "commit", "--allow-empty", "-m", "Inserted before visible rows")
	setActive(true)
	app.WaitUntil(timeout, func() bool { return app.Find(list).Offset.Y >= start+historyRowHeight-1 })
}

func TestNativeHistoryRetainsGraphViewportAcrossModes(t *testing.T) {
	repo, git := historyRepository(t)
	var stream strings.Builder
	commit := func(ref, message string, mark, timeOffset int) {
		fmt.Fprintf(&stream, "commit %s\nmark :%d\ncommitter GitWave <test@example.invalid> %d +0000\ndata %d\n%s\n",
			ref, mark, 1700000000+timeOffset, len(message), message)
	}
	commit("refs/heads/main", "Graph base", 1, 0)
	stream.WriteString("\n")
	const lanes = 64
	for i := range lanes {
		commit(fmt.Sprintf("refs/heads/lane-%02d", i), fmt.Sprintf("Lane parent %02d", i), i+2, i+1)
		stream.WriteString("from :1\n\n")
	}
	commit("refs/heads/main", "Join all graph lanes", lanes+2, lanes+1)
	stream.WriteString("from :1\n")
	for i := range lanes {
		fmt.Fprintf(&stream, "merge :%d\n", i+2)
	}
	stream.WriteString("\n")
	git(stream.String(), "fast-import", "--quiet")
	merge := git("", "rev-parse", "HEAD")
	right := git("", "rev-parse", "refs/heads/lane-63")
	app, _ := refreshApp(t, nil, repo)
	const timeout = 10 * time.Second
	const graph, list = "id:history-graph-scroll", "id:history-commit-list"
	const selected = "label:/^Join all graph lanes,/"
	app.WaitFor(selected, timeout)
	app.Press(selected)
	app.WaitUntil(timeout, func() bool {
		view := app.Find(graph)
		return view.Content.Width >= view.Frame.Size.Width+240
	})
	app.Scroll(list, 8*historyRowHeight+6)
	app.Scroll(graph, 214)
	app.WaitUntil(timeout, func() bool { return app.Find(graph).Offset.X >= 200 && app.Find(list).Offset.Y >= 8*historyRowHeight })
	x, y := app.Find(graph).Offset.X, app.Find(list).Offset.Y
	app.Press("id:navigate-changes")
	app.WaitFor("id:working-changes", timeout)
	app.Press("id:navigate-history")
	app.WaitUntil(timeout, func() bool {
		return app.Find(graph).Offset.X >= x-2 && app.Find(list).Offset.Y >= y-2
	})
	app.Press("id:navigate-changes")
	app.WaitFor("id:working-changes", timeout)
	git("", "checkout", "-b", "graph-refreshed")
	git("", "commit", "--allow-empty", "-m", "Commit above the scrolled graph")
	app.Key("mod+r")
	app.WaitUntil(timeout, func() bool { return app.Exists("text:graph-refreshed") })
	app.Press("id:navigate-history")
	app.WaitUntil(timeout, func() bool {
		return app.Find(graph).Offset.X >= x-2 && app.Find(list).Offset.Y >= y+historyRowHeight-2
	})
	locate := func(query, sha string) {
		t.Helper()
		app.Key("mod+k")
		app.WaitFor("id:palette-input", timeout)
		app.Type("id:palette-input", query)
		app.WaitFor("id:palette-row-"+sha, timeout)
		app.Press("id:palette-row-" + sha)
		app.WaitForGone("id:command-palette", timeout)
	}
	locate("Join all graph lanes", merge)
	app.WaitUntil(timeout, func() bool {
		return app.Find(graph).Offset.X <= 5 && app.Find(list).Offset.Y <= 2 && app.Find(selected).Props.Selected
	})
	app.Press("id:navigate-changes")
	app.WaitForGone(graph, timeout)
	locate("Lane parent 63", right)
	app.WaitUntil(timeout, func() bool {
		return app.Find(graph).Offset.X >= 195 && app.Find("label:/^Lane parent 63,/").Props.Selected
	})
}

func TestWorkbenchRefreshReopensReplacedWorktree(t *testing.T) {
	_, a := historyRepository(t)
	_, b := historyRepository(t)
	a("", "commit", "--allow-empty", "-m", "Previous checkout")
	b("", "commit", "--allow-empty", "-m", "Replacement checkout")
	tree := filepath.Join(t.TempDir(), "worktree")
	a("", "worktree", "add", "--detach", tree)
	app, setActive := refreshApp(t, nil, tree)
	const timeout = 5 * time.Second
	app.WaitFor("label:/Previous checkout/", timeout)
	app.Press("label:/Previous checkout/")
	app.WaitFor("id:commit-header", timeout)
	setActive(false)
	a("", "worktree", "remove", tree)
	b("", "worktree", "add", "--detach", tree)
	setActive(true)
	app.WaitFor("label:/Replacement checkout/", timeout)
	app.WaitForGone("label:/Previous checkout/", timeout)
	app.WaitForGone("id:commit-header", timeout)
}

func TestNativeManualRefreshFollowsRetargetedWorkspaceAlias(t *testing.T) {
	a, gitA := historyRepository(t)
	b, gitB := historyRepository(t)
	gitA("", "commit", "--allow-empty", "-m", "Original link target")
	gitB("", "commit", "--allow-empty", "-m", "New link target")
	dir := t.TempDir()
	alias := filepath.Join(dir, "legacy-link")
	if err := os.Symlink(a, alias); err != nil {
		t.Skipf("test requires directory symlinks: %v", err)
	}
	store, err := workspace.Open(t.Context(), filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	_, err = store.CreateWithRepos(t.Context(), "Legacy workspace", []string{alias})
	if err != nil {
		t.Fatal(err)
	}
	app := workspaceApp(t, store)
	const timeout = 5 * time.Second
	app.WaitFor("label:/Original link target/", timeout)
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(b, alias); err != nil {
		t.Fatal(err)
	}
	app.Key("mod+r")
	app.WaitFor("label:/New link target/", timeout)
	app.WaitForGone("label:/Original link target/", timeout)
	state, err := store.Load(t.Context())
	if err != nil || state.Repo.Path != alias {
		t.Fatalf("refresh rewrote the stored alias: %+v, %v", state, err)
	}
}

func TestNativeOpeningRetryKeepsWindowWorkspace(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Retried repository")
	store, err := workspace.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.CreateWithRepos(t.Context(), "Retry workspace", []string{repo}); err != nil {
		t.Fatal(err)
	}
	gitPath := os.Getenv("PATH")
	t.Setenv("PATH", t.TempDir())
	app := workspaceApp(t, store)
	const timeout = 5 * time.Second
	app.WaitFor("label:Retry", timeout)
	if err := store.Create(t.Context(), "Startup workspace from another window"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", gitPath)
	app.Press("label:Retry")
	app.WaitFor("label:/Retried repository/", timeout)
	app.Find("text:/^Retry workspace$/")
}

func TestWorkbenchRefreshRetriesShallowDiff(t *testing.T) {
	repo, git := historyRepository(t)
	file := filepath.Join(repo, "tracked.txt")
	if err := os.WriteFile(file, []byte("before deepening\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("", "add", "tracked.txt")
	git("", "commit", "-m", "Parent")
	if err := os.WriteFile(file, []byte("patch after deepening\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("", "commit", "-am", "Shallow tip")
	clone := filepath.Join(t.TempDir(), "shallow")
	git("", "clone", "--no-local", "--depth=1", repo, clone)
	app, setActive := refreshApp(t, nil, clone)
	const timeout = 5 * time.Second
	app.WaitFor("label:/Shallow tip/", timeout)
	app.Press("label:/Shallow tip/")
	app.WaitFor("label:Retry", timeout)
	setActive(false)
	git("", "-C", clone, "fetch", "--unshallow")
	setActive(true)
	app.WaitFor("text:patch after deepening", timeout)
	app.WaitForGone("label:Retry", timeout)
	if !app.Find("label:/Shallow tip/").Props.Selected {
		t.Fatal("retry lost the selected commit")
	}
}
