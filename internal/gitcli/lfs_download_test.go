package gitcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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
)

func TestGitLFSUsesTheSystemSSHFallbackOnlyWithoutAConfiguredCommand(t *testing.T) {
	dir := newFixture(t)
	global := filepath.Join(t.TempDir(), "gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	ctx := t.Context()
	repo := openFixture(t, dir)
	stage, err := repo.prepareTransportRepo(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	sshPath := filepath.Join(t.TempDir(), "system ssh.exe")
	fallback := "GIT_SSH=" + sshPath
	for _, explicit := range []bool{false, true} {
		if explicit {
			fixtureGit(t, dir, "config", "--file", global, "core.sshCommand", "custom-ssh -o BatchMode=yes")
		}
		for _, inherit := range []bool{false, true} {
			cmd := &exec.Cmd{Env: []string{"GIT_ALLOW_PROTOCOL=ssh"}}
			if inherit {
				cmd.Env = append(cmd.Env, fallback)
			}
			config := stage.command(ctx, "", "config", "--null", "--get", "core.sshCommand")
			if err := setLFSDefaultSSH(ctx, cmd, config, sshPath); err != nil {
				t.Fatal(err)
			}
			want := []string{"GIT_ALLOW_PROTOCOL=ssh"}
			if !explicit {
				want = append(want, fallback)
			}
			if !slices.Equal(cmd.Env, want) {
				t.Fatalf("explicit=%t, inherited=%t: got %v, want %v", explicit, inherit, cmd.Env, want)
			}
		}
	}
}

func TestPullDownloadsLFSObjectsOnlyFromTheSelectedTipAndPaths(t *testing.T) {
	for _, format := range []string{"sha1", "sha256"} {
		t.Run(format, func(t *testing.T) { testPullLFSPathScope(t, format) })
	}
}

func testPullLFSPathScope(t *testing.T, format string) {
	t.Helper()
	source, client, repo, options := pullFixture(t, "--object-format="+format)
	requireLFSCLI(t, repo)
	configureLFSFixtureFilters(t, client)
	configureLFSFixture(t, source)
	fixtureGit(t, client, "config", "lfs.storage", "client-lfs")
	fixtureGit(t, source, "config", "lfs.storage", "source-lfs")
	fixtureGit(t, client, "config", "lfs.fetchinclude", "images/**")
	fixtureGit(t, client, "config", "lfs.fetchexclude", "**/excluded.asset")
	sourceRepo := openFixture(t, source)
	const payload = "the selected LFS asset's real contents\x00"
	if err := os.Mkdir(filepath.Join(source, "images"), 0o755); err != nil {
		t.Fatal(err)
	}
	included := commitLFSPushAsset(t, source, sourceRepo, "images/selected.asset", payload)
	excluded := commitLFSPushAsset(t, source, sourceRepo, "images/excluded.asset", "filtered by exclude")
	_ = commitLFSPushAsset(t, source, sourceRepo, "images/nested excluded.asset", payload)
	unmatched := commitLFSPushAsset(t, source, sourceRepo, "other.asset", "filtered by include")
	fixtureGit(t, client, "config", "lfs.fetchexclude", "**/*excluded.asset")
	want := gitValue(t, source, "rev-parse", "HEAD")
	fixtureGit(t, source, "switch", "-c", "unselected")
	other := commitLFSPushAsset(t, source, sourceRepo, "images/other.asset", "found only on the other branch")
	for _, id := range []string{excluded, unmatched, other} {
		if err := os.Remove(filepath.Join(sourceRepo.CommonDir, "source-lfs", lfsObjectPath(id))); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := repo.PreparePull(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.Apply(t.Context(), FetchOptions{})
	if err != nil || result.OID != want || !result.Fetched {
		t.Fatalf("pull failed: %+v, %v", result, err)
	}
	if got := lfsRead(t, client, "images/selected.asset"); got != payload {
		t.Fatalf("checkout did not receive the downloaded asset: %q", got)
	}
	cache := filepath.Join(repo.CommonDir, "client-lfs")
	if got := lfsRead(t, cache, lfsObjectPath(included)); got != payload {
		t.Fatalf("download went to the wrong cache: %q", got)
	}
	for _, id := range []string{excluded, unmatched, other} {
		if _, err := os.Stat(filepath.Join(cache, lfsObjectPath(id))); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("excluded object entered the destination cache: %s, %v", id, err)
		}
	}
	if got := lfsRead(t, client, "images/excluded.asset"); !strings.Contains(got, "oid sha256:"+excluded) {
		t.Fatalf("excluded file did not retain its pointer: %q", got)
	}
	if got := lfsRead(t, client, "images/nested excluded.asset"); !strings.Contains(got, "oid sha256:"+included) {
		t.Fatalf("excluded path shared an object and was materialized: %q", got)
	}
	if status := readFixtureStatus(t, repo); len(status.Changes) != 0 {
		t.Fatalf("pull produced dirty changes: %+v", status.Changes)
	}
}

func TestPullDownloadsLFSWithScopedHTTPSCredentials(t *testing.T) {
	t.Run("same origin independent path", func(t *testing.T) { testPullDownloadsLFSHTTPS(t, false) })
	t.Run("independently authenticated origins", func(t *testing.T) { testPullDownloadsLFSHTTPS(t, true) })
}

func testPullDownloadsLFSHTTPS(t *testing.T, separate bool) {
	t.Helper()
	source, client, repo, options := pullFixture(t)
	requireLFSCLI(t, repo)
	configureLFSFixtureFilters(t, client)
	configureLFSFixture(t, source)
	sourceRepo := openFixture(t, source)
	const payload = "bytes downloaded from the alternate LFS path\x00"
	oid := commitLFSPushAsset(t, source, sourceRepo, "asset.asset", payload)
	example := fmt.Sprintf("version %s\noid sha256:%s\nsize 123\n", lfsPointerVersion, strings.Repeat("1", 64))
	writeFixture(t, source, "pointer-example.txt", example)
	fixtureGit(t, source, "add", "--", "pointer-example.txt")
	fixtureGit(t, source, "commit", "-m", "Document pointer syntax as plain text")
	fixtureGit(t, source, "update-server-info")
	const username, password = "media-user", "media-fixture-token"
	const gitUser, gitPassword = "git-user", "git-fixture-token"
	files := http.StripPrefix("/project.git/", http.FileServer(http.Dir(sourceRepo.GitDir)))
	var downloadCount atomic.Int32
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/separate-lfs/objects/batch":
			user, secret, ok := r.BasicAuth()
			if !ok || user != username || secret != password {
				w.Header().Set("WWW-Authenticate", "Basic realm=\"LFS Fixture\"")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var input struct {
				Operation string
				Objects   []struct {
					OID  string
					Size int
				}
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&input); err != nil ||
				input.Operation != "download" || len(input.Objects) != 1 || input.Objects[0].OID != oid ||
				input.Objects[0].Size != len(payload) {
				http.Error(w, "wrong download batch", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.git-lfs+json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"transfer": "basic", "objects": []any{map[string]any{
					"oid": oid, "size": len(payload), "authenticated": true,
					"actions": map[string]any{"download": map[string]any{
						"href": server.URL + "/media/" + oid, "header": map[string]string{"X-Media-Ticket": "fixture"},
					}},
				}},
			})
		case "/media/" + oid:
			if r.Method != http.MethodGet || r.Header.Get("X-Media-Ticket") != "fixture" ||
				r.Header.Get("Authorization") != "" {
				http.Error(w, "wrong media authorization", http.StatusBadRequest)
				return
			}
			downloadCount.Add(1)
			_, _ = io.WriteString(w, payload)
		default:
			if r.Header.Get("Authorization") != "" {
				http.Error(w, "LFS-only credentials used for Git", http.StatusBadRequest)
				return
			}
			files.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(server.Close)
	gitServer := server
	if separate {
		gitServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, secret, ok := r.BasicAuth()
			if !ok || user != gitUser || secret != gitPassword {
				w.Header().Set("WWW-Authenticate", "Basic realm=\"Git Fixture\"")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			files.ServeHTTP(w, r)
		}))
		t.Cleanup(gitServer.Close)
	}
	gitConfig := useTestTLSConfig(t, source, server)
	credentialFile := filepath.ToSlash(filepath.Join(filepath.Dir(gitConfig), "credentials"))
	helper := "store --file='" + strings.ReplaceAll(credentialFile, "'", "'\\''") + "'"
	fixtureGit(t, source, "config", "--file", gitConfig, "credential.helper", helper)
	fixtureGit(t, client, "remote", "set-url", "origin", gitServer.URL+"/project.git")
	config := "[lfs]\n\turl = " + server.URL + "/separate-lfs\n"
	writeFixture(t, client, ".lfsconfig", config)
	if err := os.MkdirAll(filepath.Join(client, ".git", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(client, ".git"), "info/exclude", ".lfsconfig\n")
	plan, err := repo.PreparePull(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	var challenge *RemoteAuthError
	if result, err := plan.Apply(t.Context(), FetchOptions{}); result.Fetched || !errors.As(err, &challenge) {
		t.Fatalf("LFS download did not request its HTTPS credentials: %+v, %v", result, err)
	}
	if separate {
		if challenge.LFSLocation() != "" {
			t.Fatal("the original Git challenge identified the media endpoint")
		}
		gitRetry := FetchOptions{Challenge: challenge, Credentials: &HTTPSCredential{Username: gitUser, Password: gitPassword}}
		if result, err := plan.Apply(t.Context(), gitRetry); result.Fetched || !errors.As(err, &challenge) {
			t.Fatalf("private media did not ask for separate authentication: %+v, %v", result, err)
		}
	}
	if challenge.LFSLocation() != server.URL+"/separate-lfs" {
		t.Fatalf("wrong LFS credential destination: %q", challenge.LFSLocation())
	}
	request := FetchOptions{Challenge: challenge, Credentials: &HTTPSCredential{Username: username, Password: password, Remember: true}}
	if !separate {
		writeFixture(t, client, ".lfsconfig", "[lfs]\n\turl = "+server.URL+"/different-store\n")
		if result, err := plan.Apply(t.Context(), request); err == nil || result.Fetched {
			t.Fatalf("the authentication retry accepted a changed LFS URL: %+v, %v", result, err)
		}
		writeFixture(t, client, ".lfsconfig", config)
	}
	result, err := plan.Apply(t.Context(), request)
	if err != nil || !result.Fetched || result.Warning != "" || downloadCount.Load() != 1 || lfsRead(t, client, "asset.asset") != payload {
		t.Fatalf("authenticated LFS download failed: %+v, %v, downloaded=%d", result, err, downloadCount.Load())
	}
	if got := lfsRead(t, client, "pointer-example.txt"); got != example {
		t.Fatalf("ordinary pointer-shaped text changed: %q", got)
	}
	stored, err := os.ReadFile(credentialFile)
	if err != nil || !strings.HasSuffix(strings.TrimSpace(string(stored)), "/separate-lfs") ||
		strings.Count(string(stored), "\n") != 1 {
		var paths []string
		for line := range strings.SplitSeq(strings.TrimSpace(string(stored)), "\n") {
			if address, parseErr := url.Parse(line); parseErr == nil {
				paths = append(paths, address.Path)
			}
		}
		t.Fatalf("the credential helper did not save only the LFS path: %q, %v", paths, err)
	}
}

func TestPullRejectsMissingOrCorruptLFSBeforePublishingRefs(t *testing.T) {
	source, client, repo, options := pullFixture(t)
	requireLFSCLI(t, repo)
	configureLFSFixtureFilters(t, client)
	configureLFSFixture(t, source)
	sourceRepo := openFixture(t, source)
	const payload = "exactly original bytes!"
	oid := commitLFSPushAsset(t, source, sourceRepo, "asset.asset", payload)
	path := filepath.Join(sourceRepo.CommonDir, "lfs", lfsObjectPath(oid))
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	plan, err := repo.PreparePull(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	assertUnchanged := func(result PullResult, err error) {
		t.Helper()
		if err == nil || result.Fetched {
			t.Fatalf("pull published an invalid LFS checkout: %+v, %v", result, err)
		}
		for _, ref := range []string{"HEAD", "refs/remotes/origin/main"} {
			if got := gitValue(t, client, "rev-parse", ref); got != options.Local.OID {
				t.Fatalf("pull changed %s before LFS succeeded: %s", ref, got)
			}
		}
		if _, err := os.Stat(filepath.Join(client, "asset.asset")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("failed pull wrote a worktree asset: %v", err)
		}
		if _, err := os.Stat(filepath.Join(repo.CommonDir, "lfs", lfsObjectPath(oid))); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("failed pull wrote to the cache: %v", err)
		}
	}
	assertUnchanged(plan.Apply(t.Context(), FetchOptions{}))
	if err := os.WriteFile(path, []byte(strings.Repeat("!", len(payload))), 0o600); err != nil {
		t.Fatal(err)
	}
	assertUnchanged(plan.Apply(t.Context(), FetchOptions{}))
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := plan.Apply(t.Context(), FetchOptions{})
	if err != nil || !result.Fetched || lfsRead(t, client, "asset.asset") != payload {
		t.Fatalf("corrected LFS asset did not allow retry: %+v, %v", result, err)
	}
}

func TestPullRechecksLFSCacheDuringRefPublication(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX Git wrapper")
	}
	for _, change := range []string{"replace directory", "remove object"} {
		t.Run(change, func(t *testing.T) {
			source, client, repo, options := pullFixture(t)
			requireLFSCLI(t, repo)
			configureLFSFixtureFilters(t, client)
			configureLFSFixture(t, source)
			oid := commitLFSPushAsset(t, source, openFixture(t, source), "asset.asset", "media prepared before publishing refs")
			original := t.TempDir()
			storage := filepath.Join(t.TempDir(), "cache")
			if err := os.Symlink(original, storage); err != nil {
				t.Fatal(err)
			}
			fixtureGit(t, client, "config", "lfs.storage", storage)
			wrapper := filepath.Join(t.TempDir(), "git")
			marker := filepath.Join(t.TempDir(), "changed")
			t.Setenv("GITWAVE_TEST_GIT", repo.git)
			t.Setenv("GITWAVE_TEST_GIT_DIR", "--git-dir="+repo.GitDir)
			t.Setenv("GITWAVE_TEST_LFS_OBJECT", filepath.Join(original, lfsObjectPath(oid)))
			t.Setenv("GITWAVE_TEST_LFS_CACHE", storage)
			t.Setenv("GITWAVE_TEST_LFS_REPLACEMENT", t.TempDir())
			t.Setenv("GITWAVE_TEST_LFS_CHANGE", change)
			t.Setenv("GITWAVE_TEST_LFS_MARKER", marker)
			const script = `#!/bin/sh
live=
for arg do
  if [ "$arg" = "$GITWAVE_TEST_GIT_DIR" ]; then live=yes; fi
  if [ "$arg" = update-ref ] && [ "$live" = yes ] && [ -f "$GITWAVE_TEST_LFS_OBJECT" ] && [ ! -e "$GITWAVE_TEST_LFS_MARKER" ]; then
    if [ "$GITWAVE_TEST_LFS_CHANGE" = 'replace directory' ]; then
      rm "$GITWAVE_TEST_LFS_CACHE" &&
        ln -s "$GITWAVE_TEST_LFS_REPLACEMENT" "$GITWAVE_TEST_LFS_CACHE" || exit 90
    else
      rm "$GITWAVE_TEST_LFS_OBJECT" || exit 90
    fi
    touch "$GITWAVE_TEST_LFS_MARKER" || exit 90
    break
  fi
done
exec "$GITWAVE_TEST_GIT" "$@"
`
			if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", filepath.Dir(wrapper)+string(os.PathListSeparator)+os.Getenv("PATH"))
			repo.git = wrapper
			plan, err := repo.PreparePull(t.Context(), options)
			if err != nil {
				t.Fatal(err)
			}
			result, err := plan.Apply(t.Context(), FetchOptions{})
			if _, markerErr := os.Stat(marker); markerErr != nil || err == nil || result.Fetched {
				t.Fatalf("cache change at publication was not rejected: %+v, %v, marker: %v", result, err, markerErr)
			}
			for _, ref := range []string{"HEAD", "refs/remotes/origin/main"} {
				if got := gitValue(t, client, "rev-parse", ref); got != options.Local.OID {
					t.Fatalf("%s changed despite losing the LFS cache: %s", ref, got)
				}
			}
		})
	}
}

func TestPullWithSkipSmudgeKeepsLFSPointer(t *testing.T) {
	source, client, repo, options := pullFixture(t)
	requireLFSCLI(t, repo)
	configureLFSFixtureFilters(t, client)
	configureLFSFixture(t, source)
	sourceRepo := openFixture(t, source)
	oid := commitLFSPushAsset(t, source, sourceRepo, "asset.asset", "the file is intentionally not downloaded")
	t.Setenv("GIT_LFS_SKIP_SMUDGE", "1")
	plan, err := repo.PreparePull(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(t.Context(), FetchOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := lfsRead(t, client, "asset.asset"); !strings.Contains(got, "oid sha256:"+oid) {
		t.Fatalf("checkout ignored skip-smudge: %q", got)
	}
	if _, err := os.Stat(filepath.Join(repo.CommonDir, "lfs", lfsObjectPath(oid))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("checkout downloaded an object despite skip-smudge: %v", err)
	}
}

func TestPullRebasePreservesTheLFSPathSelection(t *testing.T) {
	source, client, repo, options := pullFixture(t)
	requireLFSCLI(t, repo)
	configureLFSFixtureFilters(t, client)
	configureLFSFixture(t, source)
	fixtureGit(t, client, "config", "lfs.fetchinclude", "incoming.asset")
	writeFixture(t, client, "local-only.txt", "preserve my commit\n")
	fixtureGit(t, client, "add", "--", "local-only.txt")
	fixtureGit(t, client, "commit", "-m", "Keep a local change")
	options.Local.OID = gitValue(t, client, "rev-parse", "HEAD")
	options.Rebase = true
	const payload = "selected upstream object contents"
	sourceRepo := openFixture(t, source)
	oid := commitLFSPushAsset(t, source, sourceRepo, "incoming.asset", payload)
	commitLFSPushAsset(t, source, sourceRepo, "still-a-pointer.asset", payload)
	plan, err := repo.PreparePull(t.Context(), options)
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.Apply(t.Context(), FetchOptions{})
	if err != nil || !result.Rebased {
		t.Fatalf("Pull with Rebase failed: %+v, %v", result, err)
	}
	if got := lfsRead(t, client, "incoming.asset"); got != payload {
		t.Fatalf("rebased worktree did not materialize the selected asset: %q", got)
	}
	if got := lfsRead(t, client, "still-a-pointer.asset"); !strings.Contains(got, "oid sha256:"+oid) {
		t.Fatalf("rebased worktree materialized an excluded path: %q", got)
	}
	if got := lfsRead(t, client, "local-only.txt"); got != "preserve my commit\n" {
		t.Fatalf("rebased worktree changed the independent local commit: %q", got)
	}
}
