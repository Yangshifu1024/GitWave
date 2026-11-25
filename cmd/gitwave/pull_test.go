package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
