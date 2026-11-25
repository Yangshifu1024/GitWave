package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gitwave.work/internal/gitcli"
	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestHealthToggleKeepsPressWhenMeasurementFinishes(t *testing.T) {
	if os.Getenv("NSGO_TEST_TARGET") != "" {
		t.Skip("uses a model press held across a controlled result")
	}
	for _, fixture := range []struct {
		name, status string
		report       gitcli.HealthReport
	}{
		{name: "ordinary measurement", status: "1 changed", report: gitcli.HealthReport{DirtyFiles: 1}},
		{name: "operation residue", status: "Operation residue", report: gitcli.HealthReport{ConflictResidue: []string{"MERGE_HEAD"}}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var complete func()
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				w := newWorkbench(cx)
				w.health.Set(healthState{loading: true})
				complete = func() {
					cx.Post(func() { w.health.Set(healthState{report: &fixture.report}) })
				}
				return w.healthSidebar(colors(false))
			})
			press := app.Touch("id:health-toggle", core.Point{X: 20, Y: 12})
			complete()
			app.WaitFor("text:"+fixture.status, 5*time.Second)
			press.Lift()
			app.WaitFor("id:health-ai-summary", 5*time.Second)
		})
	}
}

func TestRepositoryHealthReportsDistinctDirtyFilesAndOperationResidue(t *testing.T) {
	dir, git := historyRepository(t)
	t.Setenv("GIT_AUTHOR_DATE", "@1700000000 +0000")
	t.Setenv("GIT_COMMITTER_DATE", "@1700000000 +0000")
	git("", "commit", "--allow-empty", "-m", "old base")
	git("", "branch", "stale/feature")
	git("", "branch", "stale/topic")
	git("", "remote", "add", "origin", "https://example.invalid/gitwave.git")
	base := git("", "rev-parse", "HEAD")
	git("", "update-ref", "refs/remotes/origin/main", base)
	git("", "branch", "--set-upstream-to=origin/main")
	now := time.Now().UTC().Format(time.RFC3339)
	t.Setenv("GIT_AUTHOR_DATE", now)
	t.Setenv("GIT_COMMITTER_DATE", now)
	writeRepositoryFixtureFile(t, dir, "tracked.txt", "first")
	writeRepositoryFixtureFile(t, dir, ".gitignore", "ignored.txt\n")
	git("", "add", ".gitignore", "tracked.txt")
	git("", "commit", "-m", "local update")
	git("", "tag", "demo")
	git("", "tag", "-a", "annotated", "-m", "tag message")
	git("", "mv", "tracked.txt", "renamed.txt")
	writeRepositoryFixtureFile(t, dir, "renamed.txt", "also changed in the worktree")
	writeRepositoryFixtureFile(t, dir, "ignored.txt", "quiet")
	writeRepositoryFixtureFile(t, dir, "big.bin", strings.Repeat("a", 2<<20))
	writeRepositoryFixtureFile(t, dir, "BUILD/unreported.bin", strings.Repeat("a", 3<<20))
	writeRepositoryFixtureFile(t, dir, ".git/MERGE_HEAD", base+"\n")
	writeRepositoryFixtureFile(t, dir, ".git/MERGE_MSG", "merge\n")

	repo, err := gitcli.OpenRoot(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	report, err := repo.Health(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	unpushed := 1
	want := gitcli.HealthReport{
		Unpushed: &unpushed, DirtyFiles: 4, BranchCount: 3, TagCount: 2,
		StaleBranches:   []string{"stale/feature", "stale/topic"},
		ConflictResidue: []string{"MERGE_HEAD"},
		LargeFiles:      []gitcli.LargeFile{{Path: "big.bin", SizeBytes: 2 << 20}},
	}
	if !reflect.DeepEqual(report, want) {
		t.Fatalf("health report = %+v, want %+v", report, want)
	}
}

func TestNativeRepositoryHealthExpandsAndRefreshes(t *testing.T) {
	dir, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Base for health")
	writeRepositoryFixtureFile(t, dir, "large.bin", strings.Repeat("a", 1<<20))
	app := refTestApp(t, dir)
	const timeout = 14 * time.Second
	app.WaitFor("text:1 changed", timeout)
	app.Press("id:health-toggle")
	app.WaitFor("text:no upstream", timeout)
	app.WaitFor("text:large.bin (1.0 MB)", timeout)
	app.WaitFor("text:1 / 0", timeout)
	if err := os.Remove(filepath.Join(dir, "large.bin")); err != nil {
		t.Fatal(err)
	}
	app.Key("mod+r")
	app.WaitFor("text:Clean", timeout)
	app.WaitFor("text:none (>1 MB)", timeout)
	app.Press("id:health-toggle")
	app.WaitForGone("id:health-refs", timeout)
}

func TestNativeRepositoryHealthSwitchesToARepoWithBrokenHistory(t *testing.T) {
	first, gitFirst := historyRepository(t)
	second, gitSecond := historyRepository(t)
	gitFirst("", "commit", "--allow-empty", "-m", "First repository")
	gitSecond("", "commit", "--allow-empty", "-m", "Second repository")
	writeRepositoryFixtureFile(t, second, ".git/refs/heads/lost", strings.Repeat("f", 40)+"\n")
	writeRepositoryFixtureFile(t, second, "work.txt", "untracked content")
	store, err := workspace.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.CreateWithRepos(t.Context(), "Health workspace", []string{first, second}); err != nil {
		t.Fatal(err)
	}
	app := workspaceApp(t, store)
	const timeout = 12 * time.Second
	app.WaitFor("text:Clean", timeout)
	app.Press("id:health-toggle")
	app.WaitFor("text:1 / 0", timeout)
	app.Press("label:" + filepath.Base(second))
	app.WaitFor("text:1 changed", timeout)
	app.WaitForGone("id:health-refs", timeout)
	app.Press("id:health-toggle")
	app.WaitFor("text:2 / 0", timeout)
}

func writeRepositoryFixtureFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
