package gitcli

import (
	"errors"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRemoteBranchDeleteUsesTheConfirmedRemoteTip(t *testing.T) {
	client, hub, repo := pushFixture(t)
	base := gitValue(t, client, "rev-parse", "HEAD")
	fixtureGit(t, client, "branch", "topic")
	fixtureGit(t, client, "push", "origin", "topic")
	target := CheckoutTarget{Ref: "refs/remotes/origin/topic", OID: base}
	plan, err := repo.PrepareRemoteBranchDelete(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	if destinations := plan.Destinations(); len(destinations) != 1 || destinations[0].Tip != base {
		t.Fatalf("incorrect reviewed destinations: %+v", destinations)
	}
	fixtureGit(t, client, "commit", "--allow-empty", "-m", "New commit on the remote branch")
	advanced := gitValue(t, client, "rev-parse", "HEAD")
	// Push to the path so the displayed client tracking ref remains stale.
	fixtureGit(t, client, "push", "--", hub, advanced+":refs/heads/topic")
	if result, err := plan.Apply(t.Context(), FetchOptions{}); err == nil || len(result.Deleted) != 0 {
		t.Fatalf("deleted a branch that advanced: %+v, %v", result, err)
	}
	if got := gitValue(t, hub, "rev-parse", "refs/heads/topic"); got != advanced {
		t.Fatalf("the newer remote tip was lost: %s", got)
	}
	fixtureGit(t, client, "fetch", "origin")
	target.OID = advanced
	plan, err = repo.PrepareRemoteBranchDelete(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := plan.Apply(t.Context(), FetchOptions{}); err != nil ||
		len(result.Deleted) != 1 || result.Local != "" || len(result.Warnings) != 0 {
		t.Fatalf("confirmed branch was not deleted: %+v, %v", result, err)
	}
	if got := gitValue(t, hub, "for-each-ref", "--format=%(refname)", "refs/heads/topic"); got != "" {
		t.Fatalf("the remote branch remains: %s", got)
	}
	if got := gitValue(t, client, "for-each-ref", "--format=%(refname)", target.Ref); got != "" {
		t.Fatalf("the obsolete tracking reference remains: %s", got)
	}
	if got := gitValue(t, client, "rev-parse", "refs/heads/topic"); got != base {
		t.Fatalf("local branch changed: %s", got)
	}
}

func TestRemoteBranchDeleteMapsConfiguredTrackingRefspec(t *testing.T) {
	client, hub, repo := pushFixture(t)
	base := gitValue(t, client, "rev-parse", "HEAD")
	fixtureGit(t, client, "remote", "rename", "origin", "team/upstream")
	fixtureGit(t, client, "config", "remote.team/upstream.fetch", "+refs/heads/review/*:refs/remotes/reviews/*")
	fixtureGit(t, client, "push", "--", hub, base+":refs/heads/review/topic")
	fixtureGit(t, client, "fetch", "team/upstream")
	target := CheckoutTarget{Ref: "refs/remotes/reviews/topic", OID: base}
	plan, err := repo.PrepareRemoteBranchDelete(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	if d := plan.Destinations(); len(d) != 1 || d[0].Branch != "team/upstream/review/topic" {
		t.Fatalf("incorrect destination: %+v", d)
	}
	if result, err := plan.Apply(t.Context(), FetchOptions{}); err != nil || len(result.Deleted) != 1 {
		t.Fatalf("mapped branch remove: %+v, %v", result, err)
	}
	if got := gitValue(t, hub, "for-each-ref", "--format=%(refname)", "refs/heads"); got != "" {
		t.Fatalf("a remote branch remains: %s", got)
	}
}

func TestRemoteBranchDeleteKeepsPartialOutcomeWithLocalDeletion(t *testing.T) {
	client, first, repo := pushFixture(t)
	second := t.TempDir()
	fixtureGit(t, second, "init", "--quiet", "--bare", "-b", "main")
	fixtureGit(t, client, "remote", "add", "second", second)
	fixtureGit(t, client, "branch", "topic")
	base := gitValue(t, client, "rev-parse", "HEAD")
	fixtureGit(t, client, "push", "origin", "topic")
	fixtureGit(t, client, "push", "second", "topic")
	plan, err := repo.PrepareRemoteBranchDelete(t.Context(), CheckoutTarget{Ref: "refs/heads/topic", OID: base})
	if err != nil {
		t.Fatal(err)
	}
	tree := gitValue(t, client, "rev-parse", "HEAD^{tree}")
	newer := gitValue(t, client, "commit-tree", tree, "-p", base, "-m", "New tip on the second remote")
	fixtureGit(t, client, "push", "--", second, newer+":refs/heads/topic")
	result, err := plan.Apply(t.Context(), FetchOptions{})
	if err == nil || result.Local != "topic" || len(result.Deleted) != 1 || result.Deleted[0].Branch != "origin/topic" {
		t.Fatalf("partial outcome was not reported: %+v, %v", result, err)
	}
	if got := gitValue(t, client, "for-each-ref", "--format=%(refname)", "refs/heads/topic"); got != "" {
		t.Fatalf("the chosen local deletion did not finish: %q", got)
	}
	if got := gitValue(t, first, "for-each-ref", "--format=%(refname)", "refs/heads/topic"); got != "" {
		t.Fatalf("an acknowledged branch remains on first remote: %q", got)
	}
	if got := gitValue(t, second, "rev-parse", "refs/heads/topic"); got != newer {
		t.Fatalf("lost newer remote commit: %q", got)
	}
}

func TestRemoteBranchDeleteResumesOnlyTheAuthenticatingDestination(t *testing.T) {
	client, first, repo := pushFixture(t)
	root := t.TempDir()
	second := filepath.Join(root, "project.git")
	fixtureGit(t, root, "init", "--quiet", "--bare", "-b", "main", second)
	fixtureGit(t, second, "config", "http.receivepack", "true")
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	const username, token = "fixture-user", "fixture-token"
	handler := &cgi.Handler{Path: git, Args: []string{"http-backend"},
		Env: []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1", "REMOTE_USER=" + username}}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != username || password != token {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"GitWave delete test\"")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	useTestTLSConfig(t, client, server)
	base := gitValue(t, client, "rev-parse", "HEAD")
	fixtureGit(t, client, "push", "origin", base+":refs/heads/topic")
	fixtureGit(t, client, "push", "--", second, base+":refs/heads/topic")
	fixtureGit(t, client, "remote", "set-url", "--add", "--push", "origin", first)
	fixtureGit(t, client, "remote", "set-url", "--add", "--push", "origin", server.URL+"/project.git")
	plan, err := repo.PrepareRemoteBranchDelete(t.Context(), CheckoutTarget{Ref: "refs/remotes/origin/topic", OID: base})
	if err != nil {
		t.Fatal(err)
	}
	var challenge *RemoteAuthError
	result, err := plan.Apply(t.Context(), FetchOptions{})
	if !errors.As(err, &challenge) || challenge.operation != deleteRemoteOperation || len(result.Deleted) != 1 {
		t.Fatalf("expected first deletion and second challenge: %+v, %v", result, err)
	}
	// Recreating the first branch at the same OID makes an unintended
	// retry on the already-confirmed destination detectably destructive.
	fixtureGit(t, client, "push", "--", first, base+":refs/heads/topic")
	fixtureGit(t, client, "fetch", "origin")
	result, err = plan.Apply(t.Context(), FetchOptions{Challenge: challenge,
		Credentials: &HTTPSCredential{Username: username, Password: token}})
	if err != nil || len(result.Deleted) != 2 || len(result.Warnings) != 0 {
		t.Fatalf("authenticated deletion failed: %+v, %v", result, err)
	}
	if got := gitValue(t, first, "rev-parse", "refs/heads/topic"); got != base {
		t.Fatalf("the operation retried the completed destination: %s", got)
	}
	if got := gitValue(t, client, "rev-parse", "refs/remotes/origin/topic"); got != base {
		t.Fatalf("the freshly fetched tracking branch was cleared by the retry: %s", got)
	}
	if got := gitValue(t, second, "for-each-ref", "--format=%(refname)", "refs/heads/topic"); got != "" {
		t.Fatalf("branch remains on the second destination: %s", got)
	}
}

func TestRemoteBranchDeleteRejectsAChangedDestination(t *testing.T) {
	client, hub, repo := pushFixture(t)
	base := gitValue(t, client, "rev-parse", "HEAD")
	fixtureGit(t, client, "push", "origin", base+":refs/heads/topic")
	plan, err := repo.PrepareRemoteBranchDelete(t.Context(), CheckoutTarget{Ref: "refs/remotes/origin/topic", OID: base})
	if err != nil {
		t.Fatal(err)
	}
	replacement := t.TempDir()
	fixtureGit(t, replacement, "init", "--quiet", "--bare", "-b", "main")
	fixtureGit(t, client, "push", "--", replacement, base+":refs/heads/topic")
	fixtureGit(t, client, "remote", "set-url", "--push", "origin", replacement)
	if result, err := plan.Apply(t.Context(), FetchOptions{}); err == nil || !strings.Contains(err.Error(), "changed") || result.Attempted {
		t.Fatalf("accepted a different destination: %+v, %v", result, err)
	}
	for _, remote := range []string{hub, replacement} {
		if tip := gitValue(t, remote, "rev-parse", "refs/heads/topic"); tip != base {
			t.Fatalf("the branch in %q changed: %s", remote, tip)
		}
	}
}

func TestRemoteBranchDeleteBindsTheRepositoryBehindALocalPath(t *testing.T) {
	client, hub, repo := pushFixture(t)
	base := gitValue(t, client, "rev-parse", "HEAD")
	fixtureGit(t, client, "push", "origin", base+":refs/heads/topic")
	plan, err := repo.PrepareRemoteBranchDelete(t.Context(), CheckoutTarget{Ref: "refs/remotes/origin/topic", OID: base})
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(t.TempDir(), "original.git")
	if err := os.Rename(hub, moved); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, client, "init", "--quiet", "--bare", "-b", "main", hub)
	fixtureGit(t, client, "push", "--", hub, base+":refs/heads/topic")
	result, err := plan.Apply(t.Context(), FetchOptions{})
	if err == nil || !strings.Contains(err.Error(), "local push repository changed") || result.Attempted {
		t.Fatalf("the replacement destination was accepted: %+v, %v", result, err)
	}
	for _, path := range []string{hub, moved} {
		if got := gitValue(t, path, "rev-parse", "refs/heads/topic"); got != base {
			t.Fatalf("the branch in %q changed: %s", path, got)
		}
	}
}

func TestRemoteBranchDeleteDeduplicatesMatchingDestinations(t *testing.T) {
	client, hub, repo := pushFixture(t)
	base := gitValue(t, client, "rev-parse", "HEAD")
	fixtureGit(t, client, "remote", "add", "alias", hub)
	fixtureGit(t, client, "branch", "topic")
	fixtureGit(t, client, "push", "origin", "topic")
	fixtureGit(t, client, "fetch", "alias")
	plan, err := repo.PrepareRemoteBranchDelete(t.Context(), CheckoutTarget{Ref: "refs/heads/topic", OID: base})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(plan.Destinations()); got != 1 {
		t.Fatalf("same target appears %d times", got)
	}
	result, err := plan.Apply(t.Context(), FetchOptions{})
	if err != nil || result.Local != "topic" || len(result.Deleted) != 1 || len(result.Warnings) != 0 {
		t.Fatalf("expected a single remote deletion: %+v, %v", result, err)
	}
	if refs := gitValue(t, client, "for-each-ref", "--format=%(refname)", "refs/remotes"); refs != "" {
		t.Fatalf("the same endpoint's tracking references were not removed: %s", refs)
	}
}

func TestRemoteBranchDeleteRejectsAReplacedCheckoutBeforeSending(t *testing.T) {
	client, hub, repo := pushFixture(t)
	base := gitValue(t, client, "rev-parse", "HEAD")
	fixtureGit(t, client, "push", "origin", base+":refs/heads/topic")
	plan, err := repo.PrepareRemoteBranchDelete(t.Context(), CheckoutTarget{Ref: "refs/remotes/origin/topic", OID: base})
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(t.TempDir(), "replacement")
	original := filepath.Join(t.TempDir(), "original")
	fixtureGit(t, client, "clone", "--quiet", hub, replacement)
	if err := os.Rename(client, original); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, client); err != nil {
		t.Fatal(err)
	}
	if result, err := plan.Apply(t.Context(), FetchOptions{}); err == nil || result.Attempted {
		t.Fatalf("the replaced checkout authorized a deletion: %+v, %v", result, err)
	}
	if tip := gitValue(t, hub, "rev-parse", "refs/heads/topic"); tip != base {
		t.Fatalf("remote branch changed: %s", tip)
	}
}

func TestRemoteBranchDeletePreservesTrackingAfterFetchAliasChanges(t *testing.T) {
	client, first, repo := pushFixture(t)
	base := gitValue(t, client, "rev-parse", "HEAD")
	second := t.TempDir()
	fixtureGit(t, second, "init", "--quiet", "--bare", "-b", "main")
	fixtureGit(t, client, "push", "--", first, base+":refs/heads/topic")
	fixtureGit(t, client, "push", "--", second, base+":refs/heads/topic")
	link := filepath.Join(t.TempDir(), "fetch")
	if err := os.Symlink(first, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	fixtureGit(t, client, "remote", "set-url", "origin", link)
	fixtureGit(t, client, "remote", "set-url", "--push", "origin", first)
	fixtureGit(t, client, "fetch", "origin")
	plan, err := repo.PrepareRemoteBranchDelete(t.Context(), CheckoutTarget{Ref: "refs/remotes/origin/topic", OID: base})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second, link); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, client, "fetch", "origin")
	result, err := plan.Apply(t.Context(), FetchOptions{})
	if err != nil || len(result.Deleted) != 1 || len(result.Warnings) != 1 ||
		!strings.Contains(result.Warnings[0], "local fetch repository changed") {
		t.Fatalf("did not report the changed fetch source: %+v, %v", result, err)
	}
	if tip := gitValue(t, client, "rev-parse", "refs/remotes/origin/topic"); tip != base {
		t.Fatalf("deleted the other repository's freshly fetched tracking ref: %s", tip)
	}
	if tip := gitValue(t, second, "rev-parse", "refs/heads/topic"); tip != base {
		t.Fatalf("deleted a branch from an unselected push destination: %s", tip)
	}
}

func TestRemoteBranchDeleteDoesNotChangeAReplacementCheckoutAfterSending(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a shell hook in a temporary remote")
	}
	client, hub, repo := pushFixture(t)
	base := gitValue(t, client, "rev-parse", "HEAD")
	fixtureGit(t, client, "push", "origin", base+":refs/heads/topic")
	plan, err := repo.PrepareRemoteBranchDelete(t.Context(), CheckoutTarget{Ref: "refs/remotes/origin/topic", OID: base})
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(t.TempDir(), "replacement")
	original := filepath.Join(t.TempDir(), "original")
	fixtureGit(t, client, "clone", "--quiet", hub, replacement)
	t.Setenv("GITWAVE_DELETE_TEST_CLIENT", client)
	t.Setenv("GITWAVE_DELETE_TEST_ORIGINAL", original)
	t.Setenv("GITWAVE_DELETE_TEST_REPLACEMENT", replacement)
	hook := filepath.Join(hub, "hooks", "post-receive")
	if err := os.WriteFile(hook, []byte(`#!/bin/sh
set -e
mv "$GITWAVE_DELETE_TEST_CLIENT" "$GITWAVE_DELETE_TEST_ORIGINAL"
mv "$GITWAVE_DELETE_TEST_REPLACEMENT" "$GITWAVE_DELETE_TEST_CLIENT"
`), 0o700); err != nil {
		t.Fatal(err)
	}
	result, err := plan.Apply(t.Context(), FetchOptions{})
	if err != nil || len(result.Deleted) != 1 || len(result.Warnings) != 1 ||
		!strings.Contains(result.Warnings[0], "local checkout changed") {
		t.Fatalf("did not report the replaced checkout after remote success: %+v, %v", result, err)
	}
	if tip := gitValue(t, client, "rev-parse", "refs/remotes/origin/topic"); tip != base {
		t.Fatalf("replacement checkout tracking ref changed: %s", tip)
	}
}

func TestRemoteBranchDeleteDeduplicatesWorkTreeAndExplicitGitDirectory(t *testing.T) {
	client := newFixture(t)
	hub := newFixture(t)
	repo := openFixture(t, client)
	base := gitValue(t, client, "rev-parse", "HEAD")
	fixtureGit(t, client, "remote", "add", "origin", hub)
	fixtureGit(t, client, "remote", "add", "metadata", filepath.Join(hub, ".git"))
	fixtureGit(t, client, "branch", "topic")
	fixtureGit(t, client, "push", "origin", "topic")
	fixtureGit(t, client, "fetch", "metadata")
	plan, err := repo.PrepareRemoteBranchDelete(t.Context(), CheckoutTarget{Ref: "refs/heads/topic", OID: base})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(plan.Destinations()); got != 1 {
		t.Fatalf("Git directory and worktree were treated as %d destinations", got)
	}
	result, err := plan.Apply(t.Context(), FetchOptions{})
	if err != nil || len(result.Deleted) != 1 || len(result.Warnings) != 0 {
		t.Fatalf("delete using mixed local aliases: %+v, %v", result, err)
	}
	if got := gitValue(t, hub, "for-each-ref", "--format=%(refname)", "refs/heads/topic"); got != "" {
		t.Fatalf("target branch remains: %s", got)
	}
}

func TestRemoteBranchDeleteRejectsDisagreeingTrackingTipsForOneDestination(t *testing.T) {
	client, hub, repo := pushFixture(t)
	base := gitValue(t, client, "rev-parse", "HEAD")
	fixtureGit(t, client, "remote", "add", "alias", hub)
	fixtureGit(t, client, "branch", "topic")
	fixtureGit(t, client, "push", "origin", "topic")
	fixtureGit(t, client, "fetch", "alias")
	tree := gitValue(t, client, "rev-parse", "HEAD^{tree}")
	stale := gitValue(t, client, "commit-tree", tree, "-m", "Different tracking snapshot")
	fixtureGit(t, client, "update-ref", "refs/remotes/alias/topic", stale)
	if _, err := repo.PrepareRemoteBranchDelete(t.Context(), CheckoutTarget{Ref: "refs/heads/topic", OID: base}); err == nil ||
		!strings.Contains(err.Error(), "disagree") {
		t.Fatalf("conflicting snapshots were presented as one approved deletion: %v", err)
	}
	if tip := gitValue(t, client, "rev-parse", "refs/heads/topic"); tip != base {
		t.Fatalf("local branch changed: %s", tip)
	}
	if tip := gitValue(t, hub, "rev-parse", "refs/heads/topic"); tip != base {
		t.Fatalf("remote branch changed: %s", tip)
	}
}
