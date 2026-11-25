package gitcli

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

func pushFixture(t *testing.T) (string, string, *Repository) {
	t.Helper()
	client := newFixture(t)
	remote := t.TempDir()
	fixtureGit(t, remote, "init", "--quiet", "--bare", "-b", "main")
	fixtureGit(t, client, "remote", "add", "origin", remote)
	return client, remote, openFixture(t, client)
}

func pushOptions(t *testing.T, client string, repo *Repository, tags ...string) PushOptions {
	t.Helper()
	remotes, err := repo.PushRemotes(t.Context())
	if err != nil || len(remotes) != 1 {
		t.Fatalf("could not read push remote: %+v, %v", remotes, err)
	}
	opts := PushOptions{Remote: remotes[0], Branch: CheckoutTarget{
		Ref: "refs/heads/main", OID: gitValue(t, client, "rev-parse", "refs/heads/main"),
	}}
	for _, name := range tags {
		ref := "refs/tags/" + name
		opts.Tags = append(opts.Tags, CheckoutTarget{Ref: ref, OID: gitValue(t, client, "rev-parse", ref)})
	}
	return opts
}

func TestPushUsesApprovedTipAndReportsDivergedTags(t *testing.T) {
	client, remote, repo := pushFixture(t)
	fixtureGit(t, client, "tag", "collision")
	fixtureGit(t, client, "tag", "old-only")
	fixtureGit(t, client, "push", "origin", "main", "refs/tags/collision")
	before := gitValue(t, remote, "rev-parse", "refs/tags/collision")
	fixtureGit(t, client, "commit", "--allow-empty", "-m", "New commit to push")
	fixtureGit(t, client, "tag", "-f", "collision")
	fixtureGit(t, client, "tag", "-a", "annotated", "-m", "annotated tag")
	fixtureGit(t, client, "tag", "lightweight")
	opts := pushOptions(t, client, repo, "annotated", "collision", "lightweight")
	plan, err := repo.PreparePush(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.Apply(t.Context(), FetchOptions{}, nil)
	if err != nil || result.Branches != 1 || !slices.Equal(result.SkippedTags, []string{"collision"}) ||
		len(result.Warnings) != 0 {
		t.Fatalf("partial tag push was not recorded correctly: %+v, %v", result, err)
	}
	for name, want := range map[string]string{
		opts.Branch.Ref: opts.Branch.OID, "refs/tags/collision": before,
		"refs/tags/annotated": opts.Tags[0].OID, "refs/tags/lightweight": opts.Tags[2].OID,
	} {
		if got := gitValue(t, remote, "rev-parse", name); got != want {
			t.Fatalf("remote %s = %s, want %s", name, got, want)
		}
	}
	if got := gitValue(t, remote, "for-each-ref", "refs/tags/old-only"); got != "" {
		t.Fatalf("push sent an unselected tag: %s", got)
	}
	if got := gitValue(t, client, "rev-parse", "refs/remotes/origin/main"); got != opts.Branch.OID {
		t.Fatalf("push did not update the same remote's tracking ref: %s", got)
	}
}

func TestPushAndForceReviewWithHundredsOfTags(t *testing.T) {
	client, remote, repo := pushFixture(t)
	opts := pushOptions(t, client, repo)
	for i := range 700 {
		opts.Tags = append(opts.Tags, CheckoutTarget{Ref: fmt.Sprintf("refs/tags/release-series-%04d", i), OID: opts.Branch.OID})
	}
	writeTags := func() {
		t.Helper()
		var updates strings.Builder
		for _, tag := range opts.Tags {
			fmt.Fprintf(&updates, "update %s %s\n", tag.Ref, tag.OID)
		}
		cmd := fixtureCommand(t, client, "update-ref", "--stdin")
		cmd.Stdin = strings.NewReader(updates.String())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("create tag fixture: %v %s", err, out)
		}
	}
	checkRemote := func(want string) {
		t.Helper()
		out := gitValue(t, remote, "for-each-ref", "--format=%(objectname)", "refs/tags/release-series-*")
		oids := strings.Split(out, "\n")
		if len(oids) != len(opts.Tags) || slices.ContainsFunc(oids, func(oid string) bool { return oid != want }) {
			t.Fatalf("remote has %d of %d expected tags, or their targets differ", len(oids), len(opts.Tags))
		}
	}
	writeTags()
	plan, err := repo.PreparePush(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := plan.Apply(t.Context(), FetchOptions{}, nil); err != nil || result.Branches != 1 || len(result.SkippedTags) != 0 {
		t.Fatalf("batch push failed: %+v, %v", result, err)
	}
	before := opts.Branch.OID
	checkRemote(before)
	suffixMatch := "refs/tags/nested/" + opts.Tags[0].Ref
	fixtureGit(t, remote, "update-ref", suffixMatch, before)
	fixtureGit(t, client, "commit", "--allow-empty", "-m", "Retag a new release commit")
	opts.Branch.OID = gitValue(t, client, "rev-parse", "HEAD")
	for i := range opts.Tags {
		opts.Tags[i].OID = opts.Branch.OID
	}
	writeTags()
	opts.Force = true
	plan, err = repo.PreparePush(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	review, err := plan.Review(t.Context(), FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(review.Changes) != len(opts.Tags)+1 || slices.ContainsFunc(review.Changes, func(lease PushLease) bool {
		return lease.Before != before || lease.After != opts.Branch.OID
	}) {
		t.Fatalf("force review got %d changes with incomplete leases", len(review.Changes))
	}
	if result, err := plan.Apply(t.Context(), FetchOptions{}, review); err != nil || result.Branches != 1 {
		t.Fatalf("batch force push failed: %+v, %v", result, err)
	}
	checkRemote(opts.Branch.OID)
	if got := gitValue(t, remote, "rev-parse", suffixMatch); got != before {
		t.Fatalf("push changed a ref matched by suffix only: %s", got)
	}
}

func TestPushRequiresReviewAndLeasesTheRemoteBranch(t *testing.T) {
	client, remote, repo := pushFixture(t)
	fixtureGit(t, client, "push", "origin", "main")
	previous := gitValue(t, remote, "rev-parse", "main")
	other := t.TempDir()
	fixtureGit(t, other, "clone", "--quiet", "--", remote, ".")
	fixtureGit(t, other, "config", "user.name", "Another author")
	fixtureGit(t, other, "config", "user.email", "other@example.invalid")
	fixtureGit(t, other, "commit", "--allow-empty", "-m", "Origin moved after review")
	fixtureGit(t, client, "commit", "--allow-empty", "-m", "Local competing history")
	opts := pushOptions(t, client, repo)
	opts.Force = true
	plan, err := repo.PreparePush(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(t.Context(), FetchOptions{}, nil); err == nil {
		t.Fatal("force push accepted without a reference review")
	}
	review, err := plan.Review(t.Context(), FetchOptions{})
	if err != nil || len(review.Changes) != 1 || review.Changes[0].Before != previous || review.Changes[0].After != opts.Branch.OID {
		t.Fatalf("force push review did not capture the exact references: %+v, %v", review, err)
	}
	fixtureGit(t, other, "push", "origin", "main")
	intervening := gitValue(t, other, "rev-parse", "HEAD")
	if result, err := plan.Apply(t.Context(), FetchOptions{}, review); err == nil || !strings.Contains(err.Error(), "stale info") ||
		result.Branches != 0 {
		t.Fatalf("force push overwrote a newer reference: %+v, %v", result, err)
	}
	if got := gitValue(t, remote, "rev-parse", "main"); got != intervening {
		t.Fatalf("force push lost the intervening commit: %s", got)
	}
	plan, err = repo.PreparePush(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	review, err = plan.Review(t.Context(), FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := plan.Apply(t.Context(), FetchOptions{}, review); err != nil || result.Branches != 1 {
		t.Fatalf("freshly approved force push failed: %+v, %v", result, err)
	}
	if got := gitValue(t, remote, "rev-parse", "main"); got != opts.Branch.OID {
		t.Fatalf("force push landed the wrong commit: %s", got)
	}
}

func TestForcePushPreservesATagChangedAfterReview(t *testing.T) {
	client, remote, repo := pushFixture(t)
	base := gitValue(t, client, "rev-parse", "HEAD")
	fixtureGit(t, client, "tag", "version")
	fixtureGit(t, client, "push", "origin", "main", "refs/tags/version")
	tree := gitValue(t, client, "rev-parse", "HEAD^{tree}")
	other := gitValue(t, client, "commit-tree", tree, "-p", base, "-m", "Another tag target")
	fixtureGit(t, client, "push", "origin", other+":refs/heads/other")
	fixtureGit(t, client, "commit", "--allow-empty", "-m", "New local branch tip")
	fixtureGit(t, client, "tag", "-f", "version")
	opts := pushOptions(t, client, repo, "version")
	opts.Force = true
	plan, err := repo.PreparePush(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	review, err := plan.Review(t.Context(), FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, remote, "update-ref", "refs/tags/version", other)
	result, err := plan.Apply(t.Context(), FetchOptions{}, review)
	if err == nil || !strings.Contains(err.Error(), "stale info") || result.Branches != 1 {
		t.Fatalf("expected a successful branch and stale tag rejection: %+v, %v", result, err)
	}
	if branch := gitValue(t, remote, "rev-parse", "refs/heads/main"); branch != opts.Branch.OID {
		t.Fatalf("branch result was incorrect: %s", branch)
	}
	if tag := gitValue(t, remote, "rev-parse", "refs/tags/version"); tag != other {
		t.Fatalf("force push replaced the tag changed after review: %s", tag)
	}
}

func TestPushDoesNotInheritUnselectedPushOptions(t *testing.T) {
	client, remote, repo := pushFixture(t)
	config := filepath.Join(t.TempDir(), "gitconfig")
	fixtureGit(t, client, "config", "--file", config, "push.pushOption", "unselected-option")
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	opts := pushOptions(t, client, repo)
	plan, err := repo.PreparePush(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := plan.Apply(t.Context(), FetchOptions{}, nil); err != nil || result.Branches != 1 {
		t.Fatalf("inherited push options broke the push: %+v, %v", result, err)
	}
	if got := gitValue(t, remote, "rev-parse", "main"); got != opts.Branch.OID {
		t.Fatalf("push did not publish the selected commit: %s", got)
	}
}

func TestPushUsesSeparateURLsAndPinsSelectedEndpoints(t *testing.T) {
	client, fetchRemote, repo := pushFixture(t)
	first, second := t.TempDir(), t.TempDir()
	for _, remote := range []string{first, second} {
		fixtureGit(t, remote, "init", "--quiet", "--bare", "-b", "main")
		fixtureGit(t, client, "remote", "set-url", "--add", "--push", "origin", remote)
	}
	opts := pushOptions(t, client, repo)
	if opts.Remote.DestinationCount() != 2 {
		t.Fatalf("expected two push URLs: %+v", opts.Remote)
	}
	plan, err := repo.PreparePush(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, client, "remote", "set-url", "--delete", "--push", "origin", second)
	if _, err := plan.Apply(t.Context(), FetchOptions{}, nil); err == nil || !strings.Contains(err.Error(), "destination changed") {
		t.Fatalf("push accepted a different URL list: %v", err)
	}
	fixtureGit(t, client, "remote", "set-url", "--add", "--push", "origin", second)
	if result, err := plan.Apply(t.Context(), FetchOptions{}, nil); err != nil || result.Branches != 2 {
		t.Fatalf("configured Push URLs did not both receive the branch: %+v, %v", result, err)
	}
	for _, remote := range []string{first, second} {
		if got := gitValue(t, remote, "rev-parse", "main"); got != opts.Branch.OID {
			t.Fatalf("push did not update %s: %s", remote, got)
		}
	}
	if got := gitValue(t, fetchRemote, "for-each-ref", "refs/heads/main"); got != "" {
		t.Fatalf("push wrote to the fetch-only destination: %s", got)
	}
	if got := gitValue(t, client, "for-each-ref", "refs/remotes/origin/main"); got != "" {
		t.Fatalf("push misrepresented the fetch-only destination locally: %s", got)
	}
}

func TestPushDoesNotOverwriteASharedTrackingAlias(t *testing.T) {
	client, remote, repo := pushFixture(t)
	base := gitValue(t, client, "rev-parse", "HEAD")
	fixtureGit(t, client, "push", "origin", "HEAD:refs/heads/other/main")
	fixtureGit(t, client, "config", "--replace-all", "remote.origin.fetch", "+refs/heads/other/*:refs/remotes/origin/*")
	fixtureGit(t, client, "config", "--add", "remote.origin.fetch", "+refs/heads/group/*:refs/remotes/origin/*")
	if _, err := repo.Fetch(t.Context(), FetchOptions{}); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, client, "commit", "--allow-empty", "-m", "Only the intended remote source")
	branch := gitValue(t, client, "rev-parse", "HEAD")
	fixtureGit(t, client, "branch", "group/main")
	opts := pushOptions(t, client, repo)
	opts.Branch = CheckoutTarget{Ref: "refs/heads/group/main", OID: branch}
	plan, err := repo.PreparePush(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := plan.Apply(t.Context(), FetchOptions{}, nil); err != nil || result.Branches != 1 {
		t.Fatalf("push of a source with an aliased tracking destination failed: %+v, %v", result, err)
	}
	for ref, want := range map[string]string{"refs/heads/group/main": branch, "refs/heads/other/main": base} {
		if got := gitValue(t, remote, "rev-parse", ref); got != want {
			t.Fatalf("remote %s = %s, want %s", ref, got, want)
		}
	}
	if got := gitValue(t, client, "rev-parse", "refs/remotes/origin/main"); got != base {
		t.Fatalf("push overwrote the tracking alias from a different remote source: %s", got)
	}
}

func TestPushHTTPSRetriesOnlyTheChallengingDestination(t *testing.T) {
	client, first, repo := pushFixture(t)
	root := t.TempDir()
	remote := filepath.Join(root, "project.git")
	fixtureGit(t, root, "init", "--quiet", "--bare", "-b", "main", remote)
	fixtureGit(t, remote, "config", "http.receivepack", "true")
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
			w.Header().Set("WWW-Authenticate", "Basic realm=\"GitWave push test\"")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	useTestTLSConfig(t, client, server)
	fixtureGit(t, client, "remote", "set-url", "--add", "--push", "origin", first)
	fixtureGit(t, client, "remote", "set-url", "--add", "--push", "origin", server.URL+"/project.git")
	opts := pushOptions(t, client, repo)
	plan, err := repo.PreparePush(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	var challenge *RemoteAuthError
	if result, err := plan.Apply(t.Context(), FetchOptions{}, nil); !errors.As(err, &challenge) ||
		challenge.Remote != "origin" || challenge.operation != "push" || result.Branches != 1 {
		t.Fatalf("expected a completed destination and an HTTPS challenge: %+v, %+v, %v", result, challenge, err)
	}
	tree := gitValue(t, client, "rev-parse", "HEAD^{tree}")
	intervening := gitValue(t, client, "commit-tree", tree, "-p", opts.Branch.OID, "-m", "First destination advanced")
	fixtureGit(t, client, "push", first, intervening+":refs/heads/main")
	retry := FetchOptions{Challenge: challenge, Credentials: &HTTPSCredential{Username: username, Password: token}}
	if result, err := plan.Apply(t.Context(), retry, nil); err != nil || result.Branches != 2 {
		t.Fatalf("authenticated push failed: %+v, %v", result, err)
	}
	if got := gitValue(t, first, "rev-parse", "main"); got != intervening {
		t.Fatalf("authentication resumed a destination that had already succeeded: %s", got)
	}
	if got := gitValue(t, remote, "rev-parse", "main"); got != opts.Branch.OID {
		t.Fatalf("authenticated push landed the wrong commit: %q", got)
	}
}

func TestPushDoesNotContactAnHTTPRedirectDestination(t *testing.T) {
	client, _, repo := pushFixture(t)
	var contacted atomic.Bool
	unapproved := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacted.Store(true)
		http.NotFound(w, r)
	}))
	t.Cleanup(unapproved.Close)
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, unapproved.URL+r.URL.RequestURI(), http.StatusTemporaryRedirect)
	}))
	t.Cleanup(redirect.Close)
	endpoint := redirect.URL + "/project.git"
	fixtureGit(t, client, "remote", "set-url", "--push", "origin", endpoint)
	config := filepath.Join(t.TempDir(), "gitconfig")
	fixtureGit(t, client, "config", "--file", config, "http."+endpoint+".followRedirects", "true")
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	opts := pushOptions(t, client, repo)
	plan, err := repo.PreparePush(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := plan.Apply(t.Context(), FetchOptions{}, nil); err == nil || result.Branches != 0 || contacted.Load() {
		t.Fatalf("push contacted an unapproved HTTP destination: %+v, %v, contacted=%t", result, err, contacted.Load())
	}
	opts.Force = true
	plan, err = repo.PreparePush(t.Context(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if review, err := plan.Review(t.Context(), FetchOptions{}); err == nil || review != nil || contacted.Load() {
		t.Fatalf("force push review contacted an unapproved HTTP destination: %+v, %v, contacted=%t", review, err, contacted.Load())
	}
}

func TestPushDoesNotTreatANewlineAsAnAdditionalConfiguredURL(t *testing.T) {
	client, first, repo := pushFixture(t)
	second := t.TempDir()
	fixtureGit(t, second, "init", "--quiet", "--bare", "-b", "main")
	fixtureGit(t, client, "config", "remote.origin.pushurl", first+"\n"+second)
	remotes, err := repo.PushRemotes(t.Context())
	if err != nil || len(remotes) != 1 {
		t.Fatalf("could not read remote choices: %+v, %v", remotes, err)
	}
	options := PushOptions{Remote: remotes[0], Branch: CheckoutTarget{
		Ref: "refs/heads/main", OID: gitValue(t, client, "rev-parse", "HEAD"),
	}}
	if _, err := repo.PreparePush(t.Context(), options); err == nil || !strings.Contains(err.Error(), "line break") {
		t.Fatalf("push accepted multiple URLs from a single malformed value: %v", err)
	}
	for _, remote := range []string{first, second} {
		if got := gitValue(t, remote, "for-each-ref", "refs/heads/main"); got != "" {
			t.Fatalf("a malformed URL changed %s: %s", remote, got)
		}
	}
}
