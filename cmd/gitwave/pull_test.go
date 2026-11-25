package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNativePullUsesNewCheckoutAfterChangesRefresh(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Local main")
	git("", "switch", "-c", "topic")
	git("", "commit", "--allow-empty", "-m", "Local topic")
	git("", "switch", "main")
	git("", "remote", "add", "origin", ".")
	git("", "fetch", "origin")
	git("", "branch", "--set-upstream-to=origin/main", "main")
	git("", "branch", "--set-upstream-to=origin/topic", "topic")

	app := refTestApp(t, repo)
	app.WaitFor("id:workbench-branch >> text:→ origin/main", refTestTimeout)
	git("", "switch", "topic")
	app.Press("id:navigate-changes")
	app.WaitFor("id:workbench-branch >> text:→ origin/topic", refTestTimeout)
	app.WaitUntil(refTestTimeout, func() bool { return app.Find("id:status-current-branch").Text == "topic" })
	app.Press("id:pull")
	app.WaitFor("id:pull-dialog >> text:/^topic$/", refTestTimeout)
	app.WaitFor("id:pull-branch >> text:origin/topic", refTestTimeout)
	app.Press("label:Cancel")
	app.WaitForGone("id:pull-dialog", refTestTimeout)
}

func TestNativePullSelectsRemoteBranchAndReappliesLocalChanges(t *testing.T) {
	remote, gitRemote := historyRepository(t)
	tracked := filepath.Join(remote, "tracked.txt")
	if err := os.WriteFile(tracked, []byte("base contents\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gitRemote("", "add", "tracked.txt")
	gitRemote("", "commit", "-m", "Base for Pull")
	gitRemote("", "branch", "topic")
	client, git := historyRepository(t)
	git("", "remote", "add", "origin", remote)
	git("", "config", "--replace-all", "remote.origin.fetch", "+refs/heads/main:refs/remotes/origin/main")
	git("", "config", "--add", "remote.origin.fetch", "+refs/heads/topic:refs/remotes/origin/mirror")
	git("", "fetch", "origin")
	git("", "reset", "--hard", "origin/main")
	if err := os.WriteFile(filepath.Join(client, "tracked.txt"), []byte("local working edit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := refTestApp(t, client)
	const timeout = 10 * time.Second
	app.WaitFor("label:/Base for Pull/", timeout)
	gitRemote("", "switch", "topic")
	gitRemote("", "commit", "--allow-empty", "-m", "Latest from the selected remote branch")
	target := gitRemote("", "rev-parse", "HEAD")

	app.Press("id:pull")
	app.WaitFor("id:pull-dialog", timeout)
	app.WaitUntil(timeout, func() bool { return app.Enabled("id:pull-confirm") })
	app.Press("label:Cancel")
	app.WaitForGone("id:pull-dialog", timeout)
	if got := git("", "rev-parse", "origin/mirror"); got == target {
		t.Fatal("cancelled Pull still fetched the remote")
	}
	app.Press("id:pull")
	app.WaitFor("id:pull-dialog", timeout)
	app.WaitUntil(timeout, func() bool { return app.Enabled("id:pull-confirm") })
	app.Press("id:pull-branch")
	app.Key("End")
	app.Key("Enter")
	app.Press("id:pull-stash")
	app.Press("id:pull-confirm")
	app.WaitFor("text:Pulled from origin", timeout)
	app.WaitFor("label:/Latest from the selected remote branch/", timeout)
	if got := git("", "rev-parse", "HEAD"); got != target {
		t.Fatalf("Pull advanced to %s, want %s", got, target)
	}
	if got := git("", "symbolic-ref", "HEAD"); got != "refs/heads/main" {
		t.Fatalf("Pull switched the head attachment to %s", got)
	}
	if contents, err := os.ReadFile(filepath.Join(client, "tracked.txt")); err != nil || string(contents) != "local working edit\n" {
		t.Fatalf("Pull lost the working edit: %q, %v", contents, err)
	}
}
