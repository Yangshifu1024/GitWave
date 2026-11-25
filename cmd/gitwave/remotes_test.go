package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

func TestNativeRemoteAddFetchEditAndRemove(t *testing.T) {
	testNativeRemoteFetchEditAndRemove(t, false)
}

func TestNativeLegacyRemoteFetchEditAndRemove(t *testing.T) {
	testNativeRemoteFetchEditAndRemove(t, true)
}

func TestNativeRemoteSidebarReappliesDefaultAtCountThreshold(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Remote section fixture")
	remote := t.TempDir()
	git("", "remote", "add", "primary", remote)
	git("", "remote", "add", "backup", remote)
	app := refTestApp(t, repo)
	const timeout = 8 * time.Second
	const header = "id:remotes-toggle"
	const list = "id:remotes-list"
	app.WaitFor(list+" >> label:primary", timeout)
	app.RightClick(list + " >> label:backup")
	app.ChoosePopup("Remove")
	app.Press("id:remote-confirm")
	app.WaitForGone(list, timeout)
	app.WaitForGone("id:remote-dialog", timeout)
	app.RightClick(header)
	app.ChoosePopup("Add remote")
	app.Type("id:remote-name", "backup")
	app.Type("id:remote-fetch-url", remote)
	app.Press("id:remote-confirm")
	app.WaitFor(list+" >> label:backup", timeout)
}

func testNativeRemoteFetchEditAndRemove(t *testing.T, legacy bool) {
	t.Helper()
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Local base")
	base := git("", "rev-parse", "HEAD")
	hub, hubGit := historyRepository(t)
	hubGit("", "commit", "--allow-empty", "-m", "Commit on selected remote")
	hubTip := hubGit("", "rev-parse", "HEAD")
	var legacyFile string
	if legacy {
		dir := filepath.Join(repo, ".git", "remotes")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		legacyFile = filepath.Join(dir, "origin")
		if err := os.WriteFile(legacyFile, []byte("URL: "+hub+"\nPull: +refs/heads/*:refs/remotes/origin/*\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app := refTestApp(t, repo)
	const timeout = 8 * time.Second
	app.WaitFor("label:/^Local base,/", timeout)
	if !legacy {
		app.Focus("id:remotes-toggle")
		app.Key("shift+f10")
		app.ChoosePopup("Add remote")
		app.WaitFor("id:remote-dialog", timeout)
		app.Type("id:remote-name", "origin")
		app.Type("id:remote-fetch-url", hub)
		app.Press("label:Cancel")
		app.WaitForGone("id:remote-dialog", timeout)
		if got := git("", "remote"); got != "" {
			t.Fatalf("cancel created the remote: %q", got)
		}
		app.RightClick("id:remotes-toggle")
		app.ChoosePopup("Add remote")
		app.Type("id:remote-name", "origin")
		app.Type("id:remote-fetch-url", hub)
		app.Press("id:remote-confirm")
		app.WaitForGone("id:remote-dialog", timeout)
		app.WaitFor("text:Remote \"origin\" added", timeout)
	}
	app.WaitUntil(timeout, func() bool { return app.Find("id:remotes-toggle").Props.Role == core.RoleButton })
	if app.Exists("id:remote-row") {
		t.Fatal("a single remote should start collapsed")
	}
	app.Press("id:remotes-toggle")
	app.WaitFor("id:remote-row", timeout)
	app.RightClick("id:remote-row")
	app.ChoosePopup("Fetch")
	app.WaitFor("text:Fetched from origin", timeout)
	app.WaitFor("label:/^Commit on selected remote,/", timeout)
	if got := git("", "rev-parse", "refs/remotes/origin/main"); got != hubTip {
		t.Fatalf("fetched tracking tip: %s", got)
	}

	app.Focus("id:remote-row")
	app.Key("shift+f10")
	app.ChoosePopup("Edit…")
	app.WaitFor("id:remote-push-url", timeout)
	app.Clear("id:remote-name")
	app.Type("id:remote-name", "upstream")
	app.Type("id:remote-push-url", "/alternate-push.git")
	app.Press("id:remote-confirm")
	app.WaitForGone("id:remote-dialog", timeout)
	app.WaitFor("label:upstream", timeout)
	if got := git("", "remote", "get-url", "--push", "upstream"); got != "/alternate-push.git" {
		t.Fatalf("push URL was not updated: %q", got)
	}
	if legacy {
		if _, err := os.Lstat(legacyFile); !os.IsNotExist(err) {
			t.Fatalf("legacy remote remained after migration: %v", err)
		}
	}
	app.RightClick("id:remote-row")
	app.ChoosePopup("Remove")
	app.WaitFor("id:remote-dialog", timeout)
	app.Press("label:Cancel")
	app.WaitForGone("id:remote-dialog", timeout)
	if got := git("", "remote"); got != "upstream" {
		t.Fatalf("cancel removed the remote: %q", got)
	}
	app.RightClick("id:remote-row")
	app.ChoosePopup("Remove")
	app.Press("id:remote-confirm")
	app.WaitForGone("id:remote-dialog", timeout)
	app.WaitForGone("id:remote-row", timeout)
	app.WaitUntil(timeout, func() bool { return app.Find("id:remotes-toggle").Props.Role == core.RoleGroup })
	if names := git("", "remote"); names != "" {
		t.Fatalf("remote remains: %q", names)
	}
	if head := git("", "rev-parse", "HEAD"); head != base {
		t.Fatalf("local checkout moved: %s", head)
	}
}
