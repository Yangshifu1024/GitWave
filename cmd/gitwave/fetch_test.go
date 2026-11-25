package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNativeFetchRefreshesRemoteHistoryWithoutSwitchingHEAD(t *testing.T) {
	remote, gitRemote := historyRepository(t)
	gitRemote("", "commit", "--allow-empty", "-m", "Original remote history")
	client, git := historyRepository(t)
	git("", "remote", "add", "origin", remote)
	git("", "fetch", "origin")
	git("", "reset", "--hard", "origin/main")
	base := git("", "rev-parse", "HEAD")
	path := filepath.Join(client, "untracked.txt")
	if err := os.WriteFile(path, []byte("local-only bytes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := refTestApp(t, client)
	const timeout = 8 * time.Second
	app.WaitFor("label:/Original remote history/", timeout)
	gitRemote("", "commit", "--allow-empty", "-m", "Tip from the remote")
	app.Press("id:fetch")
	app.WaitFor("text:Fetched", timeout)
	app.WaitFor("label:/Tip from the remote/", timeout)
	if got := git("", "rev-parse", "HEAD"); got != base {
		t.Fatalf("fetch moved the working branch: %q want %q", got, base)
	}
	if contents, err := os.ReadFile(path); err != nil || string(contents) != "local-only bytes\n" {
		t.Fatalf("fetch changed local bytes: %q, %v", contents, err)
	}
}

func TestNativeFetchCancelsFromStatusBar(t *testing.T) {
	client, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "History before cancellation")
	received := make(chan struct{}, 1)
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case received <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(remote.Close)
	git("", "remote", "add", "origin", remote.URL)
	app := refTestApp(t, client)
	const timeout = 7 * time.Second
	app.WaitFor("label:/History before cancellation/", timeout)
	app.Press("id:fetch")
	app.WaitFor("id:fetch-cancel", timeout)
	select {
	case <-received:
	case <-time.After(timeout):
		t.Fatal("fetch did not reach the remote")
	}
	app.Press("id:fetch-cancel")
	app.WaitFor("text:Cancelled", timeout)
	app.WaitForGone("id:fetch-cancel", timeout)
	if got := git("", "for-each-ref", "refs/remotes"); got != "" {
		t.Fatalf("cancelled fetch published tracking refs: %q", got)
	}
}
