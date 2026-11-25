package main

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativePushFromBranchMenuAndToolbar(t *testing.T) {
	client, git := historyRepository(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	git("", "init", "--quiet", "--bare", "-b", "main", remote)
	git("", "remote", "add", "origin", remote)
	git("", "commit", "--allow-empty", "-m", "Commit on the non-current branch")
	topic := git("", "rev-parse", "HEAD")
	git("", "branch", "topic")
	git("", "tag", "old-tag")
	git("", "commit", "--allow-empty", "-m", "Commit on the current branch")
	main := git("", "rev-parse", "HEAD")
	git("", "tag", "-a", "release", "-m", "Version on the pushed tip")
	app := refTestApp(t, client)
	const timeout = 10 * time.Second
	app.WaitFor("label:/Commit on the current branch/", timeout)
	app.WaitFor("label:topic", timeout)
	app.RightClick("label:topic")
	app.ChoosePopup("Push to origin")
	app.WaitFor("id:push-dialog", timeout)
	app.WaitUntil(timeout, func() bool { return app.Enabled("id:push-confirm") })
	app.Press("id:push-confirm")
	app.WaitFor("text:Pushed to origin", timeout)
	if got := git("", "--git-dir", remote, "rev-parse", "refs/heads/topic"); got != topic {
		t.Fatalf("branch menu pushed a different commit: %s want %s", got, topic)
	}
	if got := git("", "branch", "--show-current"); got != "main" {
		t.Fatalf("non-current branch push moved HEAD to %q", got)
	}
	app.Press("id:push")
	app.WaitFor("id:push-dialog", timeout)
	app.WaitUntil(timeout, func() bool { return app.Enabled("id:push-confirm") })
	app.Press("id:push-tags")
	app.Press("id:push-confirm")
	app.WaitFor("text:Pushed to origin", timeout)
	if got := git("", "--git-dir", remote, "rev-parse", "refs/heads/main"); got != main {
		t.Fatalf("toolbar pushed a different commit: %s want %s", got, main)
	}
	tags := git("", "--git-dir", remote, "for-each-ref", "--format=%(refname)", "refs/tags")
	if tags != "refs/tags/release" {
		t.Fatalf("toolbar pushed the wrong tags: %q", tags)
	}
}

func TestNativeForcePushReviewsAndRejectsAChangedRemoteTip(t *testing.T) {
	client, git := historyRepository(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	git("", "init", "--quiet", "--bare", "-b", "main", remote)
	git("", "remote", "add", "origin", remote)
	git("", "commit", "--allow-empty", "-m", "Shared base")
	base := git("", "rev-parse", "HEAD")
	git("", "push", "origin", "main")
	git("", "commit", "--allow-empty", "-m", "Local force-push candidate")
	expected := git("", "rev-parse", "HEAD")
	tree := git("", "rev-parse", "HEAD^{tree}")
	other := git("", "commit-tree", tree, "-p", base, "-m", "New external change")
	app := refTestApp(t, client)
	const timeout = 12 * time.Second
	app.WaitFor("label:/Local force-push candidate/", timeout)
	review := func() {
		app.Press("id:push")
		app.WaitFor("id:push-dialog", timeout)
		app.WaitUntil(timeout, func() bool { return app.Enabled("id:push-confirm") })
		app.Press("id:push-force")
		app.Press("id:push-confirm")
		app.WaitFor("id:push-review-dialog", timeout)
	}
	review()
	app.WaitFor("text:"+base+" → "+expected, timeout)
	if got := git("", "--git-dir", remote, "rev-parse", "HEAD"); got != base {
		t.Fatalf("review sent the push before final confirmation: %s", got)
	}
	git("", "push", remote, other+":refs/heads/main")
	app.Press("id:push-force-confirm")
	app.WaitFor("text:/stale info/", timeout)
	if got := git("", "--git-dir", remote, "rev-parse", "HEAD"); got != other {
		t.Fatalf("stale force push replaced the concurrent commit: %s", got)
	}
	review()
	app.WaitFor("text:"+other+" → "+expected, timeout)
	app.Press("id:push-force-confirm")
	app.WaitFor("text:Pushed to origin", timeout)
	if got := git("", "--git-dir", remote, "rev-parse", "HEAD"); got != expected {
		t.Fatalf("confirmed force push reached the wrong tip: %s", got)
	}
	if got := git("", "symbolic-ref", "HEAD"); !strings.HasSuffix(got, "/main") {
		t.Fatalf("force push changed the checked-out branch: %s", got)
	}
}

func TestNativePushReportsPartialSuccessWhenAuthenticationIsCancelled(t *testing.T) {
	client, git := historyRepository(t)
	root := t.TempDir()
	first := filepath.Join(root, "first.git")
	git("", "init", "--quiet", "--bare", "-b", "main", first)
	git("", "commit", "--allow-empty", "-m", "Commit pushed before the HTTPS challenge")
	tip := git("", "rev-parse", "HEAD")
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", "Basic realm=\"GitWave test\"")
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)
	cert := filepath.Join(root, "fixture.pem")
	if err := os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "gitconfig")
	git("", "config", "--file", config, "http.sslCAInfo", cert)
	git("", "config", "--file", config, "http.schannelUseSSLCAInfo", "true")
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	git("", "remote", "add", "origin", first)
	git("", "remote", "set-url", "--add", "--push", "origin", first)
	git("", "remote", "set-url", "--add", "--push", "origin", server.URL+"/project.git")
	app := refTestApp(t, client)
	const timeout = 12 * time.Second
	app.WaitFor("label:/Commit pushed before the HTTPS challenge/", timeout)
	app.Press("id:push")
	app.WaitFor("id:push-dialog", timeout)
	app.WaitFor("text:This remote has 2 push destinations.", timeout)
	app.Press("id:push-confirm")
	app.WaitFor("id:push-auth-dialog", timeout)
	app.WaitFor("text:/The branch was already pushed to 1 of the selected destinations/", timeout)
	app.Press("label:Cancel")
	app.WaitFor("text:Cancelled after pushing the branch to 1 of the selected destinations", timeout)
	if got := git("", "--git-dir", first, "rev-parse", "refs/heads/main"); got != tip {
		t.Fatalf("the first destination did not receive the confirmed commit: %s", got)
	}
}
