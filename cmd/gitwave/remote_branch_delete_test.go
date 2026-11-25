package main

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNativeDeleteRemoteBranchFromHistory(t *testing.T) {
	client, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Initial local commit")
	base := git("", "rev-parse", "HEAD")
	hub := filepath.Join(t.TempDir(), "remote.git")
	git("", "init", "--bare", "-b", "main", "--", hub)
	git("", "remote", "add", "origin", hub)
	git("", "switch", "-c", "topic")
	git("", "commit", "--allow-empty", "-m", "Remote-only topic tip")
	tip := git("", "rev-parse", "HEAD")
	git("", "push", "origin", "topic")
	git("", "switch", "main")
	git("", "branch", "-D", "topic")

	app := refTestApp(t, client)
	const timeout = 8 * time.Second
	ref := "label:/^Remote-only topic tip,/ >> id:history-refs"
	app.WaitFor(ref, timeout)
	open := func() {
		app.RightClick(ref)
		app.ChoosePopup("Delete Remote Branch…")
		app.WaitFor("id:remote-delete-destinations", timeout)
	}
	open()
	app.WaitFor("text:/origin\\/topic · "+shortSHA(tip)+"/", timeout)
	app.Press("label:Cancel")
	app.WaitForGone("id:ref-dialog", timeout)
	if got := git("", "--git-dir="+hub, "rev-parse", "refs/heads/topic"); got != tip {
		t.Fatalf("cancel changed the remote branch: %s", got)
	}

	open()
	app.Press("id:confirm-ref-action")
	app.WaitForGone("id:ref-dialog", timeout)
	app.WaitFor("text:Deleted remote branch origin/topic", timeout)
	app.WaitForGone("label:origin/topic", timeout)
	if got := git("", "--git-dir="+hub, "for-each-ref", "--format=%(refname)", "refs/heads/topic"); got != "" {
		t.Fatalf("the branch still exists remotely: %s", got)
	}
	if got := git("", "rev-parse", "HEAD"); got != base {
		t.Fatalf("the current checkout changed: %s", got)
	}
}

func TestNativeDeleteLocalBranchWithSelectedRemoteCounterparts(t *testing.T) {
	client, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Base for counterpart")
	base := git("", "rev-parse", "HEAD")
	hub := filepath.Join(t.TempDir(), "remote.git")
	git("", "init", "--quiet", "--bare", "-b", "main", "--", hub)
	git("", "remote", "add", "origin", hub)
	git("", "branch", "topic")
	git("", "push", "origin", "topic")

	app := refTestApp(t, client)
	const timeout = 8 * time.Second
	app.WaitFor("label:topic", timeout)
	app.RightClick("label:topic")
	app.ChoosePopup("Delete Branch…")
	app.WaitFor("id:also-delete-remote-branch", timeout)
	app.WaitUntil(timeout, func() bool { return app.Enabled("id:also-delete-remote-branch") })
	app.Press("id:also-delete-remote-branch")
	app.WaitFor("id:remote-delete-destinations", timeout)
	app.Press("id:confirm-ref-action")
	app.WaitFor("text:/Deleted local branch topic.*Deleted remote branch/", timeout)
	app.WaitForGone("label:topic", timeout)
	if got := git("", "--git-dir="+hub, "for-each-ref", "--format=%(refname)", "refs/heads/topic"); got != "" {
		t.Fatalf("matching branch still exists remotely: %s", got)
	}
	if got := git("", "rev-parse", "HEAD"); got != base {
		t.Fatalf("the current checkout changed: %s", got)
	}
}
