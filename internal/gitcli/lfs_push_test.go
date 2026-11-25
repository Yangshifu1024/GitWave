package gitcli

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cgi"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"gitwave.work/internal/proxy"
)

func commitLFSPushAsset(t *testing.T, dir string, repo *Repository, name, data string) string {
	t.Helper()
	writeFixture(t, dir, name, data)
	status := readFixtureStatus(t, repo)
	if err := repo.SetStaged(t.Context(), status, []Change{lfsFixtureChange(t, status, name)}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Commit(t.Context(), readFixtureStatus(t, repo), "Add LFS asset", ""); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
}

func applyFixtureLFSPush(t *testing.T, client string, repo *Repository) (PushResult, error) {
	t.Helper()
	plan, err := repo.PreparePush(t.Context(), pushOptions(t, client, repo))
	if err != nil {
		t.Fatal(err)
	}
	return plan.Apply(t.Context(), FetchOptions{}, nil)
}

func TestLFSPushUploadsMediaBeforeRemoteUpdatesReferences(t *testing.T) {
	client, remote, repo := pushFixture(t)
	configureLFSFixture(t, client)
	const contents = "payload received via the LFS store\x00\n"
	oid := commitLFSPushAsset(t, client, repo, "asset with spaces.asset", contents)
	target := filepath.Join(remote, "lfs", lfsObjectPath(oid))
	if runtime.GOOS != "windows" {
		hooks := filepath.Join(remote, "hooks")
		if err := os.MkdirAll(hooks, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("GITWAVE_TEST_EXPECTED_LFS_FILE", target)
		hook := "#!/bin/sh\ntest -f \"$GITWAVE_TEST_EXPECTED_LFS_FILE\"\n"
		if err := os.WriteFile(filepath.Join(hooks, "pre-receive"), []byte(hook), 0o700); err != nil {
			t.Fatal(err)
		}
		userHooks := filepath.Join(repo.CommonDir, "hooks")
		if err := os.MkdirAll(userHooks, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(userHooks, "pre-push"), []byte("#!/bin/sh\nexit 99\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	result, err := applyFixtureLFSPush(t, client, repo)
	if err != nil || result.Branches != 1 {
		t.Fatalf("LFS push failed: %+v, %v", result, err)
	}
	if got := lfsRead(t, filepath.Dir(target), filepath.Base(target)); got != contents {
		t.Fatalf("destination did not receive the LFS bytes: %q", got)
	}
	if got := gitValue(t, remote, "rev-parse", "main"); got != gitValue(t, client, "rev-parse", "HEAD") {
		t.Fatalf("destination ref was not updated to the selected commit: %s", got)
	}
}

func TestLFSPushUsesTheOperationProxyWithALocalGitDestination(t *testing.T) {
	client, remote, repo := pushFixture(t)
	requireLFSCLI(t, repo)
	configureLFSFixture(t, client)
	const payload = "object uploaded through the selected proxy\x00"
	const host = "gitwave-lfs-proxy-fixture.invalid"
	const endpoint = "http://" + host + "/lfs"
	const user, token = "gitwave-proxy-fixture", "synthetic:private-proxy-token"
	encoded := base64.StdEncoding.EncodeToString([]byte(user + ":" + token))
	oid := commitLFSPushAsset(t, client, repo, "proxied.asset", payload)
	fixtureGit(t, client, "config", "lfs.pushurl", endpoint)
	for _, name := range []string{"no_proxy", "NO_PROXY"} {
		t.Setenv(name, "")
	}
	var uploaded, reject atomic.Bool
	reject.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Scheme != "http" || r.Host != host {
			http.Error(w, "unexpected proxy target", http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case "/lfs/objects/batch":
			w.Header().Set("Content-Type", "application/vnd.git-lfs+json")
			if r.Header.Get("Proxy-Authorization") != "Basic "+encoded {
				t.Error("the configured proxy authentication did not reach the fixture")
			}
			if reject.Load() {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "Fixture proxy rejected " + token + " Basic " + encoded})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"transfer": "basic", "objects": []any{map[string]any{
				"oid": oid, "size": len(payload), "actions": map[string]any{"upload": map[string]any{"href": endpoint + "/upload"}},
			}}})
		case "/lfs/upload":
			body, err := io.ReadAll(io.LimitReader(r.Body, 1024))
			if err != nil || r.Method != http.MethodPut || string(body) != payload {
				http.Error(w, "invalid media upload", http.StatusBadRequest)
				return
			}
			uploaded.Store(true)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	address, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	address.User = url.UserPassword(user, token)
	ctx := proxy.WithSettings(t.Context(), proxy.Settings{Mode: proxy.Manual, ManualURL: address.String()})
	plan, err := repo.PreparePush(ctx, pushOptions(t, client, repo))
	if err != nil {
		t.Fatal(err)
	}
	if result, err := plan.Apply(ctx, FetchOptions{}, nil); err == nil || result.Branches != 0 ||
		!strings.Contains(err.Error(), "Fixture proxy rejected") || strings.Contains(err.Error(), token) || strings.Contains(err.Error(), encoded) {
		t.Fatal("a failed proxy upload should redact the credentials and leave the ref unchanged")
	}
	reject.Store(false)
	result, err := plan.Apply(ctx, FetchOptions{}, nil)
	if err != nil || result.Branches != 1 || !uploaded.Load() {
		t.Fatalf("push through proxy: %+v, %v, uploaded = %t", result, err, uploaded.Load())
	}
	if got, want := gitValue(t, remote, "rev-parse", "main"), gitValue(t, client, "rev-parse", "HEAD"); got != want {
		t.Fatalf("remote tip = %s, want %s", got, want)
	}
}

func TestLFSPushLeavesRemoteRefUnchangedWhenMediaIsMissing(t *testing.T) {
	client, remote, repo := pushFixture(t)
	configureLFSFixture(t, client)
	if result, err := applyFixtureLFSPush(t, client, repo); err != nil || result.Branches != 1 {
		t.Fatalf("baseline push failed: %+v, %v", result, err)
	}
	before := gitValue(t, remote, "rev-parse", "main")
	oid := commitLFSPushAsset(t, client, repo, "missing.asset", "payload that disappears before push")
	if err := os.Remove(filepath.Join(repo.CommonDir, "lfs", lfsObjectPath(oid))); err != nil {
		t.Fatal(err)
	}
	result, err := applyFixtureLFSPush(t, client, repo)
	if err == nil || !strings.Contains(err.Error(), "missing from the local cache") || result.Branches != 0 {
		t.Fatalf("push accepted a missing object: %+v, %v", result, err)
	}
	if got := gitValue(t, remote, "rev-parse", "main"); got != before {
		t.Fatalf("failed LFS upload changed the remote reference: %q", got)
	}
	if got := gitValue(t, client, "rev-parse", "refs/remotes/origin/main"); got != before {
		t.Fatalf("failed LFS upload changed local tracking: %q", got)
	}
}

func TestLFSPushFindsHistoricalMediaWithDistinctLocalStorage(t *testing.T) {
	client := newFixture(t, "--object-format=sha256")
	remote := filepath.Join(t.TempDir(), "destination with spaces.git")
	fixtureGit(t, filepath.Dir(remote), "init", "--quiet", "--bare", "--object-format=sha256", "-b", "main", remote)
	path := filepath.ToSlash(remote)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	endpoint := (&url.URL{Scheme: "file", Path: path}).String()
	fixtureGit(t, client, "remote", "add", "origin", endpoint)
	fixtureGit(t, remote, "config", "--local", "lfs.storage", "destination-cache")
	fixtureGit(t, client, "config", "--local", "lfs.storage", "source-cache")
	configureLFSFixture(t, client)
	repo := openFixture(t, client)
	const contents = "asset mentioned in the history only"
	oid := commitLFSPushAsset(t, client, repo, "historical.asset", contents)
	fixtureGit(t, client, "update-index", "--force-remove", "--", "historical.asset")
	if err := os.Remove(filepath.Join(client, "historical.asset")); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Commit(t.Context(), readFixtureStatus(t, repo), "Remove the asset from the tip", ""); err != nil {
		t.Fatal(err)
	}
	if result, err := applyFixtureLFSPush(t, client, repo); err != nil || result.Branches != 1 {
		t.Fatalf("push did not transfer the historical LFS object: %+v, %v", result, err)
	}
	if got := lfsRead(t, filepath.Join(remote, "destination-cache"), lfsObjectPath(oid)); got != contents {
		t.Fatalf("LFS upload ignored the destination's storage: %q", got)
	}
}

func TestLFSPushHTTPSUsesScopedGitCredentials(t *testing.T) {
	t.Run("default endpoint", func(t *testing.T) { testLFSPushHTTPS(t, false, false) })
	t.Run("separate LFS path", func(t *testing.T) { testLFSPushHTTPS(t, true, false) })
	t.Run("separate saved LFS account", func(t *testing.T) { testLFSPushHTTPS(t, true, true) })
}

func testLFSPushHTTPS(t *testing.T, override, separateAccounts bool) {
	t.Helper()
	mediaPath := "/project.git/info/lfs"
	if override {
		mediaPath = "/additional-lfs-store"
	}
	client, _, repo := pushFixture(t)
	requireLFSCLI(t, repo)
	configureLFSFixture(t, client)
	root := t.TempDir()
	remote := filepath.Join(root, "project.git")
	fixtureGit(t, root, "init", "--quiet", "--bare", "-b", "main", remote)
	fixtureGit(t, remote, "config", "http.receivepack", "true")
	const contents = "LFS object received by the loopback batch server\x00"
	oid := commitLFSPushAsset(t, client, repo, "remote.asset", contents)
	const user, secret = "fixture-lfs-user", "fixture-lfs-token"
	const mediaUser, mediaToken = "fixture-media-user", "fixture-media-token"
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	gitHandler := &cgi.Handler{Path: git, Args: []string{"http-backend"},
		Env: []string{"GIT_PROJECT_ROOT=" + root, "GIT_HTTP_EXPORT_ALL=1", "REMOTE_USER=" + user}}
	var received, wrongEndpointCredential atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/upload/" + oid:
			payload, err := io.ReadAll(io.LimitReader(request.Body, 4096))
			if err != nil || request.Method != http.MethodPut || request.Header.Get("X-LFS-Fixture") != "upload" || string(payload) != contents {
				http.Error(w, "unexpected media upload", http.StatusBadRequest)
				return
			}
			received.Store(true)
			return
		}
		name, password, ok := request.BasicAuth()
		wantName, wantPassword := user, secret
		if separateAccounts && strings.HasPrefix(request.URL.Path, mediaPath+"/") {
			wantName, wantPassword = mediaUser, mediaToken
		}
		if !ok || name != wantName || password != wantPassword {
			if separateAccounts && ok {
				wrongEndpointCredential.Store(true)
			}
			w.Header().Set("WWW-Authenticate", "Basic realm=\"GitWave LFS test\"")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch request.URL.Path {
		case mediaPath + "/locks/verify":
			w.WriteHeader(http.StatusNotImplemented)
		case mediaPath + "/objects/batch":
			var batch struct {
				Operation string
				Objects   []struct {
					OID  string
					Size int
				}
			}
			if err := json.NewDecoder(io.LimitReader(request.Body, 16<<10)).Decode(&batch); err != nil ||
				batch.Operation != "upload" || len(batch.Objects) != 1 || batch.Objects[0].OID != oid || batch.Objects[0].Size != len(contents) {
				http.Error(w, "invalid upload batch", http.StatusBadRequest)
				return
			}
			if override {
				object := filepath.Join(repo.CommonDir, "lfs", lfsObjectPath(oid))
				if err := os.WriteFile(object, []byte(strings.Repeat("x", len(contents))), 0o600); err != nil {
					http.Error(w, "could not replace source media for the test", http.StatusInternalServerError)
					return
				}
			}
			w.Header().Set("Content-Type", "application/vnd.git-lfs+json")
			value := map[string]any{"oid": oid, "size": len(contents), "authenticated": true,
				"actions": map[string]any{"upload": map[string]any{
					"href": "https://" + request.Host + "/upload/" + oid, "header": map[string]string{"X-LFS-Fixture": "upload"},
				}}}
			_ = json.NewEncoder(w).Encode(map[string]any{"transfer": "basic", "objects": []any{value}})
		case "/project.git/git-receive-pack":
			if !received.Load() {
				http.Error(w, "received a Git update before its LFS object", http.StatusConflict)
				return
			}
			gitHandler.ServeHTTP(w, request)
		default:
			gitHandler.ServeHTTP(w, request)
		}
	}))
	t.Cleanup(server.Close)
	useTestTLSConfig(t, client, server)
	fixtureGit(t, client, "remote", "set-url", "origin", server.URL+"/project.git")
	if override {
		fixtureGit(t, client, "config", "--file", ".lfsconfig", "lfs.url", server.URL+mediaPath)
	}
	plan, err := repo.PreparePush(t.Context(), pushOptions(t, client, repo))
	if err != nil {
		t.Fatal(err)
	}
	if separateAccounts {
		gitEndpoint, mediaEndpoint := server.URL+"/project.git", server.URL+mediaPath
		var checked []string
		vault := &fixtureCredentialVault{lookup: func(endpoint, _ string) (HTTPSCredential, bool) {
			checked = append(checked, endpoint)
			switch endpoint {
			case gitEndpoint:
				return HTTPSCredential{Username: user, Password: secret, Remember: true}, true
			case mediaEndpoint:
				return HTTPSCredential{Username: mediaUser, Password: mediaToken, Remember: true}, true
			default:
				return HTTPSCredential{}, false
			}
		}}
		result, err := plan.Apply(t.Context(), FetchOptions{Vault: vault}, nil)
		if err != nil || result.Branches != 1 || !received.Load() || wrongEndpointCredential.Load() ||
			!slices.Equal(checked, []string{gitEndpoint, mediaEndpoint}) || !slices.Equal(vault.stored, checked) {
			t.Fatalf("independent saved LFS authentication did not complete (lookups %d, stored %d): %+v, %v", len(checked), len(vault.stored), result, err)
		}
		if got := gitValue(t, remote, "rev-parse", "HEAD"); got != gitValue(t, client, "rev-parse", "HEAD") {
			t.Fatalf("incorrect target commit after the LFS upload: %q", got)
		}
		return
	}
	result, err := plan.Apply(t.Context(), FetchOptions{}, nil)
	var challenge *RemoteAuthError
	if !errors.As(err, &challenge) || result.Branches != 0 {
		t.Fatalf("expected a scoped HTTPS challenge: %+v, %v", result, err)
	}
	options := FetchOptions{Challenge: challenge, Credentials: &HTTPSCredential{Username: user, Password: secret}}
	if override {
		fixtureGit(t, client, "config", "--file", ".lfsconfig", "lfs.url", server.URL+"/other-media")
		if result, err := plan.Apply(t.Context(), options, nil); err == nil || !strings.Contains(err.Error(), "LFS upload configuration changed") ||
			result.Branches != 0 || received.Load() {
			t.Fatalf("authentication retry accepted a different media endpoint: %+v, %v", result, err)
		}
		fixtureGit(t, client, "config", "--file", ".lfsconfig", "lfs.url", server.URL+mediaPath)
		result, err = plan.Apply(t.Context(), options, nil)
		var separateChallenge *RemoteAuthError
		if !errors.As(err, &separateChallenge) || separateChallenge.LFSLocation() != server.URL+mediaPath ||
			result.Branches != 0 || received.Load() {
			t.Fatalf("Git credentials skipped the independent media confirmation: %+v, %v", result, err)
		}
		options.Challenge = separateChallenge
	}
	result, err = plan.Apply(t.Context(), options, nil)
	if err != nil || result.Branches != 1 || !received.Load() {
		t.Fatalf("authenticated LFS upload failed: %+v, %v, uploaded=%v", result, err, received.Load())
	}
	if got := gitValue(t, remote, "rev-parse", "HEAD"); got != gitValue(t, client, "rev-parse", "HEAD") {
		t.Fatalf("incorrect target commit after the LFS upload: %q", got)
	}
}

func TestLFSPushPinsTheLocalRepositoryAtSelection(t *testing.T) {
	client, remote, repo := pushFixture(t)
	configureLFSFixture(t, client)
	oid := commitLFSPushAsset(t, client, repo, "original.asset", "object on the confirmed branch")
	plan, err := repo.PreparePush(t.Context(), pushOptions(t, client, repo))
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "original.git")
	if err := os.Rename(remote, backup); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, filepath.Dir(remote), "init", "--quiet", "--bare", "-b", "main", remote)
	result, err := plan.Apply(t.Context(), FetchOptions{}, nil)
	if err == nil || !strings.Contains(err.Error(), "selected local Git or LFS destination changed") || result.Attempted {
		t.Fatalf("a replaced repository passed the push plan: %+v, %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(remote, "lfs", lfsObjectPath(oid))); !os.IsNotExist(err) {
		t.Fatalf("media was written to the replacement repository: %v", err)
	}
	if got := gitValue(t, remote, "for-each-ref", "refs/heads/main"); got != "" {
		t.Fatalf("the replacement repository received a branch: %q", got)
	}
}

func TestLFSPushRejectsAReplacedLocalDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a POSIX shell wrapper")
	}
	client, remote, repo := pushFixture(t)
	configureLFSFixture(t, client)
	oid := commitLFSPushAsset(t, client, repo, "frozen.asset", "source object")
	sibling := t.TempDir()
	replacement := filepath.Join(sibling, "replacement.git")
	previous := filepath.Join(sibling, "original.git")
	fixtureGit(t, sibling, "init", "--quiet", "--bare", "-b", "main", replacement)
	wrapper := filepath.Join(t.TempDir(), "git")
	t.Setenv("GITWAVE_TEST_LFS_REAL_GIT", repo.git)
	t.Setenv("GITWAVE_TEST_LFS_DESTINATION", remote)
	t.Setenv("GITWAVE_TEST_LFS_PREVIOUS", previous)
	t.Setenv("GITWAVE_TEST_LFS_REPLACEMENT", replacement)
	if err := os.WriteFile(wrapper, []byte(`#!/bin/sh
for arg do
  if [ "$arg" = push ]; then
    mv "$GITWAVE_TEST_LFS_DESTINATION" "$GITWAVE_TEST_LFS_PREVIOUS" || exit 89
    mv "$GITWAVE_TEST_LFS_REPLACEMENT" "$GITWAVE_TEST_LFS_DESTINATION" || exit 89
    break
  fi
done
exec "$GITWAVE_TEST_LFS_REAL_GIT" "$@"
`), 0o700); err != nil {
		t.Fatal(err)
	}
	repo.git = wrapper
	result, err := applyFixtureLFSPush(t, client, repo)
	if err == nil || !strings.Contains(err.Error(), "selected local Git destination changed") || result.Branches != 0 {
		t.Fatalf("push accepted a replaced local repository: %+v, %v", result, err)
	}
	for _, path := range []string{remote, previous} {
		if got := gitValue(t, path, "for-each-ref", "refs/heads/main"); got != "" {
			t.Fatalf("failed LFS push published a ref in %s: %q", path, got)
		}
		if _, err := os.Stat(filepath.Join(path, "lfs", lfsObjectPath(oid))); !os.IsNotExist(err) {
			t.Fatalf("failed LFS push wrote to %s: %v", path, err)
		}
	}
}

func TestLFSPushPinsAnExistingLocalCacheDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture needs permission to create directory symlinks")
	}
	client, remote, repo := pushFixture(t)
	configureLFSFixture(t, client)
	oid := commitLFSPushAsset(t, client, repo, "cache.asset", "media associated with the selected directory")
	first, newCache := t.TempDir(), t.TempDir()
	link := filepath.Join(remote, "media-cache")
	if err := os.Symlink(first, link); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, remote, "config", "lfs.storage", link)
	plan, err := repo.PreparePush(t.Context(), pushOptions(t, client, repo))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(newCache, link); err != nil {
		t.Fatal(err)
	}
	result, err := plan.Apply(t.Context(), FetchOptions{}, nil)
	if err == nil || !strings.Contains(err.Error(), "selected local Git or LFS destination changed") || result.Attempted {
		t.Fatalf("the plan accepted a replaced LFS cache directory: %+v, %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(newCache, lfsObjectPath(oid))); !os.IsNotExist(err) {
		t.Fatalf("media was written to the replacement cache: %v", err)
	}
	if got := gitValue(t, remote, "for-each-ref", "refs/heads/main"); got != "" {
		t.Fatalf("the rejected push published a branch: %q", got)
	}
}
