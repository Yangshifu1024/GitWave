package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNativeCheckoutFromBranchesHistoryAndTags(t *testing.T) {
	repo, git := historyRepository(t)
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("tracked.txt", "main version\n")
	git("", "add", "tracked.txt")
	git("", "commit", "-m", "Main checkout version")
	main := git("", "rev-parse", "HEAD")
	git("", "tag", "-a", "-m", "Release", "v-main", main)
	git("", "switch", "-c", "topic")
	write("tracked.txt", "topic version\n")
	git("", "add", "tracked.txt")
	git("", "commit", "-m", "Topic checkout version")
	topic := git("", "rev-parse", "HEAD")
	git("", "switch", "main")
	git("", "remote", "add", "origin", "https://example.invalid/gitwave.git")
	git("", "update-ref", "refs/remotes/origin/from-main", main)
	app := refTestApp(t, repo)
	waitStatus := func(want string) {
		t.Helper()
		app.WaitUntil(refTestTimeout, func() bool {
			return app.Exists("id:ref-action-status") && app.Find("id:ref-action-status").Text == want
		})
	}
	checkout := func(selector, label string) {
		t.Helper()
		app.RightClick(selector)
		app.ChoosePopup(label)
	}
	app.WaitFor("label:topic", refTestTimeout)
	app.Focus("label:topic")
	app.Key("shift+f10")
	app.ChoosePopup("Checkout")
	waitStatus("Checked out topic")
	if got := git("", "branch", "--show-current"); got != "topic" {
		t.Fatalf("branch checkout left HEAD on %q", got)
	}

	write("unsaved.txt", "untracked edit\n")
	checkout("label:main", "Checkout")
	app.WaitFor("id:checkout-dialog", refTestTimeout)
	app.WaitFor("text:Switch to main?", refTestTimeout)
	app.Press("label:Cancel")
	app.WaitForGone("id:checkout-dialog", refTestTimeout)
	if got := git("", "rev-parse", "HEAD"); got != topic {
		t.Fatalf("dirty checkout moved HEAD to %q", got)
	}
	if data, err := os.ReadFile(filepath.Join(repo, "unsaved.txt")); err != nil || string(data) != "untracked edit\n" {
		t.Fatalf("dirty checkout lost the edit: %q %v", data, err)
	}
	if err := os.Remove(filepath.Join(repo, "unsaved.txt")); err != nil {
		t.Fatal(err)
	}

	app.Press("id:branch-group >> text:ORIGIN")
	checkout("label:origin/from-main", "Checkout")
	waitStatus("Created and switched to from-main")
	if got := git("", "rev-parse", "--symbolic-full-name", "@{upstream}"); got != "refs/remotes/origin/from-main" {
		t.Fatalf("remote checkout set unexpected upstream: %q", got)
	}
	app.WaitFor("label:from-main", refTestTimeout)
	app.WaitForGone("id:branch-row >> label:origin/from-main", refTestTimeout)
	app.WaitFor("label:/^Topic checkout version,/", refTestTimeout)
	checkout("label:/^Topic checkout version,/", "Checkout Commit")
	waitStatus("Checked out " + shortSHA(topic))
	if got := git("", "rev-parse", "HEAD"); got != topic {
		t.Fatalf("commit checkout selected %q", got)
	}
	app.Press("id:tags-toggle")
	app.WaitFor("label:v-main", refTestTimeout)
	checkout("label:v-main", "Checkout")
	waitStatus("Checked out v-main")
	if head, branch := git("", "rev-parse", "HEAD"), git("", "branch", "--show-current"); head != main || branch != "" {
		t.Fatalf("tag checkout got %q on branch %q", head, branch)
	}
	if data, err := os.ReadFile(filepath.Join(repo, "tracked.txt")); err != nil || string(data) != "main version\n" {
		t.Fatalf("tag checkout did not update the worktree: %q %v", data, err)
	}
}

func TestNativeCheckoutConfirmsStashThenDiscard(t *testing.T) {
	repo, git := historyRepository(t)
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("tracked.txt", "main version\n")
	write("edited.txt", "base version\n")
	git("", "add", ".")
	git("", "commit", "-m", "Base version")
	main := git("", "rev-parse", "HEAD")
	git("", "switch", "-c", "topic")
	write("tracked.txt", "topic version\n")
	git("", "add", "tracked.txt")
	git("", "commit", "-m", "Topic version")
	topic := git("", "rev-parse", "HEAD")
	git("", "switch", "main")
	write("edited.txt", "staged edit\n")
	git("", "add", "edited.txt")
	write("edited.txt", "later edit\n")
	write("untracked.txt", "untracked content\n")

	app := refTestApp(t, repo)
	request := func(branch string) {
		t.Helper()
		app.WaitFor("label:"+branch, refTestTimeout)
		app.RightClick("label:" + branch)
		app.ChoosePopup("Checkout")
		app.WaitFor("id:checkout-dialog", refTestTimeout)
		app.WaitFor("text:Switch to "+branch+"?", refTestTimeout)
	}
	waitStatus := func(message string) {
		t.Helper()
		app.WaitUntil(refTestTimeout, func() bool {
			return app.Exists("id:ref-action-status") && app.Find("id:ref-action-status").Text == message
		})
	}
	request("topic")
	app.WaitFor("id:checkout-discard", refTestTimeout)
	app.Press("id:checkout-stash")
	waitStatus("Checked out topic and re-applied stash")
	if head := git("", "rev-parse", "HEAD"); head != topic {
		t.Fatalf("stash and switch left HEAD at %s", head)
	}
	for name, want := range map[string]string{"edited.txt": "later edit\n", "untracked.txt": "untracked content\n"} {
		data, err := os.ReadFile(filepath.Join(repo, name))
		if err != nil || string(data) != want {
			t.Fatalf("%s after replay: %q (%v)", name, data, err)
		}
	}
	if got := git("", "show", ":edited.txt"); got != "staged edit" {
		t.Fatalf("replay lost the staged version: %q", got)
	}
	request("main")
	app.Press("id:checkout-discard")
	waitStatus("Checked out main")
	if head := git("", "rev-parse", "HEAD"); head != main {
		t.Fatalf("discard and switch left HEAD at %s", head)
	}
	if result := git("", "status", "--porcelain"); result != "" {
		t.Fatalf("discard and switch left changes: %q", result)
	}
	if result := git("", "stash", "list"); result != "" {
		t.Fatalf("successful switches left an unexpected stash: %q", result)
	}
}
