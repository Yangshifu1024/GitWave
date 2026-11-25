package gitcli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitwave.work/internal/proxy"
)

func TestFetchAndCloneUseTheOperationProxy(t *testing.T) {
	t.Setenv("no_proxy", "*")
	t.Setenv("NO_PROXY", "*")
	source := newFixture(t)
	git := openFixture(t, source).git
	gitHandler := &cgi.Handler{Path: git, Args: []string{"http-backend"},
		Env: []string{"GIT_PROJECT_ROOT=" + filepath.Dir(source), "GIT_HTTP_EXPORT_ALL=1"}}
	var seen atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Scheme != "http" || r.Host != "gitwave-proxy-fixture.invalid" {
			http.Error(w, "unexpected proxy target", http.StatusBadRequest)
			return
		}
		seen.Add(1)
		gitHandler.ServeHTTP(w, r)
	}))
	defer server.Close()
	remote := "http://gitwave-proxy-fixture.invalid/" + filepath.Base(source) + "/.git"
	ctx := proxy.WithSettings(t.Context(), proxy.Settings{Mode: proxy.Manual, ManualURL: server.URL})
	target, err := PrepareClone(ctx, filepath.Join(t.TempDir(), "clone"))
	if err != nil {
		t.Fatal(err)
	}
	cloned, err := target.Clone(ctx, remote, false, CloneOptions{})
	if err != nil || seen.Load() == 0 {
		t.Fatalf("clone through proxy: %v, requests = %d", err, seen.Load())
	}
	before := seen.Load()
	fixtureGit(t, source, "commit", "--allow-empty", "-m", "Fetched through proxy")
	if result, err := cloned.Repo.Fetch(ctx, FetchOptions{}); err != nil || seen.Load() <= before || len(result.Completed) != 1 {
		t.Fatalf("fetch through proxy: %+v, %v, requests = %d", result, err, seen.Load()-before)
	}
	if got, want := gitValue(t, cloned.Repo.Path(), "rev-parse", "refs/remotes/origin/main"), gitValue(t, source, "rev-parse", "HEAD"); got != want {
		t.Fatalf("fetched tip = %s, want %s", got, want)
	}
	var repositoryProxyHits atomic.Int32
	repositoryProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		repositoryProxyHits.Add(1)
		gitHandler.ServeHTTP(w, r)
	}))
	defer repositoryProxy.Close()
	fixtureGit(t, cloned.Repo.Path(), "config", "http."+remote+".proxy", repositoryProxy.URL)
	before = seen.Load()
	if _, err := cloned.Repo.Fetch(ctx, FetchOptions{}); err != nil || repositoryProxyHits.Load() == 0 || seen.Load() != before {
		t.Fatalf("repository-specific proxy was lost: %v, app requests = %d, repo requests = %d", err, seen.Load()-before, repositoryProxyHits.Load())
	}
	fixtureGit(t, cloned.Repo.Path(), "config", "remote.origin.proxy", "")
	config, _, err := cloned.Repo.proxyContextFile(ctx, t.TempDir(), "origin", remote)
	if err != nil || config == "" {
		t.Fatalf("isolated proxy config = %q, %v", config, err)
	}
	value := gitValue(t, cloned.Repo.Path(), "config", "--file", config, "--get-urlmatch", "http.proxy", remote)
	if value != "" {
		t.Fatalf("a remote-specific empty proxy should override the URL-specific setting, got %q", value)
	}
}

func TestFetchUpdatesRefsAndPrunesWithoutTouchingWorkingCopy(t *testing.T) {
	source := newFixture(t)
	fixtureGit(t, source, "branch", "obsolete")
	client := t.TempDir()
	fixtureGit(t, client, "clone", "--quiet", "--", source, ".")
	repo := openFixture(t, client)
	head := gitValue(t, client, "rev-parse", "HEAD")
	fixtureGit(t, client, "tag", "only-local")
	writeFixture(t, client, "mixed.txt", "staged change\n")
	fixtureGit(t, client, "add", "mixed.txt")
	writeFixture(t, client, "mixed.txt", "working change\n")
	index := filepath.Join(repo.GitDir, "index")
	before, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, source, "commit", "--allow-empty", "-m", "New remote commit")
	fixtureGit(t, source, "tag", "new-release")
	fixtureGit(t, source, "branch", "-D", "obsolete")
	tip := gitValue(t, source, "rev-parse", "HEAD")
	result, err := repo.Fetch(t.Context(), FetchOptions{})
	if err != nil || len(result.Completed) != 1 || result.Completed[0] != "origin" {
		t.Fatalf("fetch result: %+v, %v", result, err)
	}
	for ref, want := range map[string]string{
		"refs/heads/main": head, "refs/remotes/origin/main": tip, "refs/tags/new-release": tip, "refs/tags/only-local": head,
	} {
		if got := gitValue(t, client, "rev-parse", ref); got != want {
			t.Fatalf("%s = %s, want %s", ref, got, want)
		}
	}
	if got := gitValue(t, client, "for-each-ref", "refs/remotes/origin/obsolete"); got != "" {
		t.Fatalf("fetch did not prune obsolete branch: %s", got)
	}
	if after, err := os.ReadFile(index); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("fetch changed the index: %v", err)
	}
	if contents, err := os.ReadFile(filepath.Join(client, "mixed.txt")); err != nil || string(contents) != "working change\n" {
		t.Fatalf("fetch changed the worktree: %q, %v", contents, err)
	}
}

func TestFetchAllContinuesAfterOrdinaryRemoteFailure(t *testing.T) {
	source, client := newFixture(t), newFixture(t)
	fixtureGit(t, client, "remote", "add", "a-broken", filepath.Join(t.TempDir(), "not a repository"))
	fixtureGit(t, client, "remote", "add", "z-good", source)
	repo := openFixture(t, client)
	result, err := repo.Fetch(t.Context(), FetchOptions{})
	if err == nil || !strings.Contains(err.Error(), "a-broken") || len(result.Completed) != 1 || result.Completed[0] != "z-good" {
		t.Fatalf("fetch all did not attempt both remotes: %+v, %v", result, err)
	}
	if _, err := repo.Fetch(t.Context(), FetchOptions{Remote: "z-good"}); err != nil {
		t.Fatalf("a named fetch attempted another remote: %v", err)
	}
}

func TestFetchProtectsLocalRefsFromConfiguredMapping(t *testing.T) {
	source, client := newFixture(t), newFixture(t)
	fixtureGit(t, source, "commit", "--allow-empty", "-m", "Unrelated local branch must stay intact")
	fixtureGit(t, client, "remote", "add", "origin", source)
	fixtureGit(t, client, "branch", "protected")
	before := gitValue(t, client, "rev-parse", "protected")
	repo := openFixture(t, client)
	fixtureGit(t, client, "config", "--replace-all", "remote.origin.fetch", "+refs/heads/main:refs/heads/protected")
	if _, err := repo.Fetch(t.Context(), FetchOptions{}); err == nil || !strings.Contains(err.Error(), "refspec") {
		t.Fatalf("fetch accepted a direct local branch mapping: %v", err)
	}
	fixtureGit(t, client, "config", "--replace-all", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*")
	fixtureGit(t, client, "symbolic-ref", "refs/remotes/origin/main", "refs/heads/protected")
	if _, err := repo.Fetch(t.Context(), FetchOptions{}); err == nil || !strings.Contains(err.Error(), "symbolic reference") {
		t.Fatalf("fetch accepted a symbolic mapping to a local branch: %v", err)
	}
	if after := gitValue(t, client, "rev-parse", "protected"); after != before {
		t.Fatalf("fetch changed protected branch: %s want %s", after, before)
	}
}

func TestFetchCannotCreateUnbornBranchThroughDanglingTrackingAlias(t *testing.T) {
	source := newFixture(t)
	client := t.TempDir()
	fixtureGit(t, client, "init", "--quiet", "-b", "main")
	fixtureGit(t, client, "remote", "add", "origin", source)
	fixtureGit(t, client, "symbolic-ref", "refs/remotes/origin/main", "refs/heads/main")
	repo := openFixture(t, client)
	if _, err := repo.Fetch(t.Context(), FetchOptions{}); err == nil {
		t.Fatal("fetch succeeded through a dangling remote-tracking alias")
	}
	status := readFixtureStatus(t, repo)
	if status.HeadRef != "refs/heads/main" || status.HeadOID != "(initial)" {
		t.Fatalf("fetch created or moved the unborn branch: %+v", status)
	}
	if target := gitValue(t, client, "symbolic-ref", "--no-recurse", "refs/remotes/origin/main"); target != "refs/heads/main" {
		t.Fatalf("fetch replaced the existing alias: %q", target)
	}
}

func TestFetchProtectsTrackingReferenceAttachedInAnotherWorktree(t *testing.T) {
	source := newFixture(t)
	client := t.TempDir()
	fixtureGit(t, client, "clone", "--quiet", "--", source, ".")
	old := gitValue(t, client, "rev-parse", "refs/remotes/origin/main")
	worktree := filepath.Join(t.TempDir(), "linked")
	fixtureGit(t, client, "worktree", "add", "--detach", worktree, "HEAD")
	fixtureGit(t, worktree, "symbolic-ref", "HEAD", "refs/remotes/origin/main")
	fixtureGit(t, source, "commit", "--allow-empty", "-m", "A new remote tip")
	repo := openFixture(t, client)
	if _, err := repo.Fetch(t.Context(), FetchOptions{}); err == nil || !strings.Contains(err.Error(), "checked-out reference") {
		t.Fatalf("fetch did not protect the linked checkout's symbolic HEAD: %v", err)
	}
	if got := gitValue(t, worktree, "rev-parse", "HEAD"); got != old {
		t.Fatalf("fetch moved the linked checkout's HEAD to %s", got)
	}
}

func TestFetchDoesNotTraverseReferenceDirectorySymlinks(t *testing.T) {
	source, client := newFixture(t), newFixture(t)
	fixtureGit(t, source, "commit", "--allow-empty", "-m", "Remote commit for symlink protection")
	fixtureGit(t, client, "remote", "add", "origin", source)
	repo := openFixture(t, client)
	tracking := filepath.Join(repo.CommonDir, "refs", "remotes")
	if err := os.MkdirAll(tracking, 0o700); err != nil {
		t.Fatal(err)
	}
	heads := filepath.Join(repo.CommonDir, "refs", "heads")
	if err := os.Symlink(heads, filepath.Join(tracking, "origin")); err != nil {
		t.Skipf("cannot create a directory symlink: %v", err)
	}
	before := gitValue(t, client, "rev-parse", "HEAD")
	if _, err := repo.Fetch(t.Context(), FetchOptions{}); err == nil {
		t.Fatal("fetch accepted a remote namespace symlink into local branches")
	}
	if after := gitValue(t, client, "rev-parse", "HEAD"); after != before {
		t.Fatalf("fetch changed the local branch to %q", after)
	}
	if _, err := os.Lstat(filepath.Join(heads, "main.lock")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fetch left a lock in the protected directory: %v", err)
	}
}

func TestFetchNeverPrunesOrOverwritesLocalTags(t *testing.T) {
	source, client := newFixture(t), newFixture(t)
	fixtureGit(t, client, "tag", "local")
	fixtureGit(t, client, "tag", "shared")
	localTip := gitValue(t, client, "rev-parse", "HEAD")
	fixtureGit(t, source, "commit", "--allow-empty", "-m", "Upstream tag target")
	fixtureGit(t, source, "tag", "shared")
	fixtureGit(t, source, "tag", "new")
	upstreamTip := gitValue(t, source, "rev-parse", "HEAD")
	fixtureGit(t, client, "remote", "add", "origin", source)
	fixtureGit(t, client, "config", "--add", "remote.origin.fetch", "+refs/tags/*:refs/tags/*")
	repo := openFixture(t, client)
	if _, err := repo.Fetch(t.Context(), FetchOptions{}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"local": localTip, "shared": localTip, "new": upstreamTip} {
		if got := gitValue(t, client, "rev-parse", "refs/tags/"+name); got != want {
			t.Fatalf("fetch changed tag %s to %s, want %s", name, got, want)
		}
	}
}

func TestFetchUsesTheRemoteEndpointAfterOneRewrite(t *testing.T) {
	first, second, client := newFixture(t), newFixture(t), newFixture(t)
	fixtureGit(t, first, "commit", "--allow-empty", "-m", "Expected address")
	fixtureGit(t, second, "commit", "--allow-empty", "-m", "Address after a second rewrite")
	fixtureGit(t, client, "config", "--add", "url."+first+".insteadOf", "gitwave-fixture:source")
	fixtureGit(t, client, "config", "--add", "url."+second+".insteadOf", first)
	fixtureGit(t, client, "remote", "add", "origin", "gitwave-fixture:source")
	repo := openFixture(t, client)
	if _, err := repo.Fetch(t.Context(), FetchOptions{}); err != nil {
		t.Fatal(err)
	}
	if got, want := gitValue(t, client, "rev-parse", "refs/remotes/origin/main"), gitValue(t, first, "rev-parse", "HEAD"); got != want {
		t.Fatalf("fetch used the wrong rewritten address: %s want %s", got, want)
	}
}

func TestFetchPreservesAnExistingShallowBoundary(t *testing.T) {
	source := newFixture(t)
	fixtureGit(t, source, "commit", "--allow-empty", "-m", "Clone depth 1 from here")
	client := t.TempDir()
	fixtureGit(t, client, "clone", "--quiet", "--no-local", "--depth=1", "--", source, ".")
	repo := openFixture(t, client)
	before, err := repo.References(t.Context())
	if err != nil || before.Shallow == "" {
		t.Fatalf("fixture did not produce a shallow repository: %+v, %v", before, err)
	}
	fixtureGit(t, source, "commit", "--allow-empty", "-m", "Fetch a descendant of the shallow boundary")
	if _, err := repo.Fetch(t.Context(), FetchOptions{}); err != nil {
		t.Fatal(err)
	}
	after, err := repo.References(t.Context())
	if err != nil || after.Shallow != before.Shallow || after.Head != before.Head ||
		findRef(after, "refs/remotes/origin/main").OID != gitValue(t, source, "rev-parse", "HEAD") {
		t.Fatalf("unexpected shallow repository after fetch: %+v, %v", after, err)
	}
}

func TestFetchCancelsTransportBeforeFetchingTheNextRemote(t *testing.T) {
	client := newFixture(t)
	started := make(chan struct{}, 1)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-time.After(4 * time.Second):
		}
	}))
	t.Cleanup(slow.Close)
	var second atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		second.Store(true)
		http.NotFound(w, r)
	}))
	t.Cleanup(other.Close)
	fixtureGit(t, client, "remote", "add", "first", slow.URL)
	fixtureGit(t, client, "remote", "add", "second", other.URL)
	repo := openFixture(t, client)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := repo.Fetch(ctx, FetchOptions{})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(6 * time.Second):
		t.Fatal("fetch never reached the first remote")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || second.Load() {
			t.Fatalf("cancel did not stop fetch: %v, second=%t", err, second.Load())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("fetch did not stop after cancellation")
	}
}

func TestFetchHTTPSCredentialRetryIsBoundToRemoteURL(t *testing.T) {
	source, client := newFixture(t), newFixture(t)
	fixtureGit(t, source, "update-server-info")
	const username, password = "fixture-user", "fixture-token"
	var authenticated atomic.Int32
	files := http.StripPrefix("/project.git/", http.FileServer(http.Dir(filepath.Join(source, ".git"))))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, token, ok := r.BasicAuth()
		if !ok || user != username || token != password {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"GitWave fixture\"")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		authenticated.Add(1)
		files.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	useTestTLSConfig(t, source, server)
	remote := server.URL + "/project.git"
	fixtureGit(t, client, "remote", "add", "origin", remote)
	// The later remote must wait while the first one needs credentials.
	var later atomic.Bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		later.Store(true)
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "fixture remote")
	}))
	t.Cleanup(other.Close)
	fixtureGit(t, client, "remote", "add", "z-later", other.URL)
	repo := openFixture(t, client)
	var challenge *RemoteAuthError
	if _, err := repo.Fetch(t.Context(), FetchOptions{}); !errors.As(err, &challenge) || challenge.Remote != "origin" || later.Load() {
		t.Fatalf("fetch did not stop for the challenging remote: %+v, %v, later=%t", challenge, err, later.Load())
	}
	credentials := &HTTPSCredential{Username: username, Password: password}
	fixtureGit(t, client, "remote", "set-url", "origin", server.URL+"/different.git")
	if _, err := repo.Fetch(t.Context(), FetchOptions{Challenge: challenge, Credentials: credentials}); err == nil ||
		!strings.Contains(err.Error(), "URL changed") || authenticated.Load() != 0 {
		t.Fatalf("retry sent the credential to a changed URL: %v", err)
	}
	fixtureGit(t, client, "remote", "set-url", "origin", remote)
	result, err := repo.Fetch(t.Context(), FetchOptions{Challenge: challenge, Credentials: credentials})
	if err != nil || len(result.Completed) != 1 || result.Completed[0] != "origin" || authenticated.Load() == 0 || later.Load() {
		t.Fatalf("fetch retried beyond the challenging remote: %+v, %v", result, err)
	}
}

type fixtureCredentialVault struct {
	lookup func(string, string) (HTTPSCredential, bool)
	stored []string
}

func (v *fixtureCredentialVault) Lookup(_ context.Context, endpoint, username string) (HTTPSCredential, bool, error) {
	auth, ok := v.lookup(endpoint, username)
	return auth, ok, nil
}

func (v *fixtureCredentialVault) Store(_ context.Context, endpoint string, _ HTTPSCredential) error {
	v.stored = append(v.stored, endpoint)
	return nil
}

func TestFetchHTTPSVaultFollowsHelperAndRemainsScoped(t *testing.T) {
	source, client := newFixture(t), newFixture(t)
	fixtureGit(t, source, "update-server-info")
	const user, token = "configured-user", "configured-fixture-token"
	var redirect, reachedSibling atomic.Bool
	files := http.StripPrefix("/project.git/", http.FileServer(http.Dir(filepath.Join(source, ".git"))))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/sibling.git/") {
			reachedSibling.Store(true)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != user || password != token {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"GitWave fixture\"")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if redirect.Load() {
			http.Redirect(w, r, "/sibling.git/info/refs", http.StatusFound)
			return
		}
		files.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	useTestTLSConfig(t, source, server)
	remote, config := server.URL+"/project.git", os.Getenv("GIT_CONFIG_GLOBAL")
	fixtureGit(t, client, "remote", "add", "origin", remote)
	fixtureGit(t, client, "config", "--local", "credential."+remote+".username", user)
	fixtureGit(t, client, "config", "--local", "credential."+remote+".useHttpPath", "true")
	helper := `!f() { if [ "$1" = get ] && grep -q '^path=project.git$'; then printf '%s\n' 'password=configured-fixture-token'; fi; }; f`
	fixtureGit(t, client, "config", "--file", config, "credential.helper", helper)
	repo := openFixture(t, client)
	lookups := 0
	credential := HTTPSCredential{Username: user, Password: token, Remember: true}
	vault := &fixtureCredentialVault{lookup: func(endpoint, username string) (HTTPSCredential, bool) {
		lookups++
		if endpoint != remote || username != user {
			t.Fatalf("unexpected vault request for %s and %s", endpoint, username)
		}
		return credential, true
	}}
	options := FetchOptions{Remote: "origin", Vault: vault}
	if _, err := repo.Fetch(t.Context(), options); err != nil || lookups != 0 {
		t.Fatalf("Git helper did not take priority (vault lookups %d): %v", lookups, err)
	}
	fixtureGit(t, client, "config", "--file", config, "--unset", "credential.helper")
	background := options
	background.NonInteractive = true
	if _, err := repo.Fetch(t.Context(), background); !errors.Is(err, ErrHTTPSAuthentication) || lookups != 0 {
		t.Fatalf("background Fetch asked the interactive keyring for a key (lookups %d): %v", lookups, err)
	}
	if _, err := repo.Fetch(t.Context(), options); err != nil || lookups != 1 || !slices.Equal(vault.stored, []string{remote}) {
		t.Fatalf("fetch did not recover and save the approved credential (vault lookups %d): %v", lookups, err)
	}
	credential.Password = "rejected-fixture-token"
	if _, err := repo.Fetch(t.Context(), options); !errors.Is(err, ErrHTTPSAuthentication) || lookups != 2 || len(vault.stored) != 1 {
		t.Fatalf("rejected stored credential was retried or saved (vault lookups %d): %v", lookups, err)
	}
	credential.Password = token
	redirect.Store(true)
	fixtureGit(t, client, "config", "--file", config, "http."+remote+".followRedirects", "true")
	if _, err := repo.Fetch(t.Context(), options); err == nil || reachedSibling.Load() || len(vault.stored) != 1 {
		t.Fatalf("vault authentication followed a redirect to a sibling repository: %v", err)
	}
}
