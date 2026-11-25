package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNativeBranchRebaseReportsFastForwardConflictAndDirtyWorktree(t *testing.T) {
	path, git := historyRepository(t)
	file := filepath.Join(path, "example.txt")
	write := func(text string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(text+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("base")
	git("", "add", "example.txt")
	git("", "commit", "-m", "Base before rebase")
	git("", "switch", "-c", "upstream")
	write("upstream")
	git("", "commit", "-am", "Upstream change")
	upstream := git("", "rev-parse", "HEAD")
	git("", "switch", "main")
	git("", "switch", "-c", "collision")
	write("collision")
	git("", "commit", "-am", "Conflicting change")
	git("", "switch", "main")
	app := refTestApp(t, path)
	const timeout = 12 * time.Second
	rebase := func(branch string) {
		t.Helper()
		app.WaitFor("label:"+branch, timeout)
		app.RightClick("label:" + branch)
		app.ChoosePopup("Rebase current onto this")
	}

	rebase("upstream")
	app.WaitFor("text:Rebased onto upstream (fast forward)", timeout)
	if head := git("", "rev-parse", "HEAD"); head != upstream {
		t.Fatalf("fast-forward HEAD = %s", head)
	}
	app.WaitFor("label:/^Upstream change,/", timeout)
	rebase("upstream")
	app.WaitFor("text:Rebased onto upstream (already up to date)", timeout)

	rebase("collision")
	app.WaitFor("text:Rebase onto collision hit conflicts: example.txt", timeout)
	if status := git("", "status", "--porcelain"); status != "" {
		t.Fatalf("conflict changed working copy: %s", status)
	}
	if head := git("", "rev-parse", "HEAD"); head != upstream {
		t.Fatalf("conflict changed HEAD: %s", head)
	}

	write("local draft")
	rebase("collision")
	app.WaitFor("text:/Working copy has 1 uncommitted file/", timeout)
	if got, err := os.ReadFile(file); string(got) != "local draft\n" || err != nil {
		t.Fatalf("dirty rebase changed the worktree file: %q, %v", got, err)
	}
}
