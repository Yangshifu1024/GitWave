package gitcli

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestMain(m *testing.M) {
	if LocalLFSFilter() || LFSPrePush() {
		return
	}
	if CredentialHelper(os.Args[1:], os.Stdin, os.Stdout) {
		return
	}
	os.Exit(m.Run())
}

func TestClonePublishesOnlyAfterSuccess(t *testing.T) {
	source := newFixture(t)
	root := t.TempDir()
	destination := filepath.Join(root, "new parent", "new repository")
	target, err := PrepareClone(t.Context(), destination)
	if err != nil {
		t.Fatal(err)
	}
	result, err := target.Clone(t.Context(), source, false, CloneOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Repo == nil || result.Backup != "" || readFixtureStatus(t, result.Repo).HeadOID != gitValue(t, source, "rev-parse", "HEAD") {
		t.Fatalf("unexpected clone result: %+v", result)
	}
	if got := gitValue(t, destination, "remote", "get-url", "origin"); got != source {
		t.Fatalf("clone changed origin: %q", got)
	}
	assertNoCloneStage(t, root)
}

func TestCloneVerifiesCheckoutWithConfiguredGitFilters(t *testing.T) {
	source := newFixture(t)
	root := t.TempDir()
	config := filepath.Join(root, "gitconfig")
	fixtureGit(t, source, "config", "--file", config, "filter.expansion.clean", "sed 's/^expanded /canonical /'")
	fixtureGit(t, source, "config", "--file", config, "filter.expansion.smudge", "sed 's/^canonical /expanded /'")
	fixtureGit(t, source, "config", "--file", config, "filter.expansion.required", "true")
	writeFixture(t, source, ".gitattributes", "filtered.txt filter=expansion\n")
	writeFixture(t, source, "filtered.txt", "canonical contents\n")
	fixtureGit(t, source, "add", ".gitattributes", "filtered.txt")
	fixtureGit(t, source, "commit", "-m", "Create a file selected by a Git filter")
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	dest := filepath.Join(root, "destination")
	target, err := PrepareClone(t.Context(), dest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := target.Clone(t.Context(), source, false, CloneOptions{}); err != nil {
		t.Fatal(err)
	}
	if contents, err := os.ReadFile(filepath.Join(dest, "filtered.txt")); err != nil || string(contents) != "expanded contents\n" {
		t.Fatalf("clone did not honor the configured filter: %q, %v", contents, err)
	}
}

func TestCloneMaterializesLFSWithTheConfiguredGitFilter(t *testing.T) {
	source := newFixture(t)
	repo := openFixture(t, source)
	requireLFSCLI(t, repo)
	configureLFSFixture(t, source)
	const name = "asset with spaces.asset"
	const payload = "complete media in a platform checkout\x00"
	oid := commitLFSPushAsset(t, source, repo, name, payload)
	root := t.TempDir()
	config := filepath.Join(root, "gitconfig")
	fixtureGit(t, source, "config", "--file", config, "filter.lfs.process", "git-lfs filter-process")
	fixtureGit(t, source, "config", "--file", config, "filter.lfs.required", "true")
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	dest := filepath.Join(root, "destination")
	target, err := PrepareClone(t.Context(), dest)
	if err != nil {
		t.Fatal(err)
	}
	result, err := target.Clone(t.Context(), source, false, CloneOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := lfsRead(t, dest, name); got != payload {
		t.Fatalf("clone checked out %q instead of the full asset", got)
	}
	if got := lfsRead(t, filepath.Join(result.Repo.CommonDir, "lfs"), lfsObjectPath(oid)); got != payload {
		t.Fatalf("clone did not retain downloaded media: %q", got)
	}
}

func TestCloneLFSStorageSurvivesThePublicationRename(t *testing.T) {
	for _, name := range []string{"parent-relative shared cache", "absolute path inside destination",
		"case variant of destination", "symlink to cache inside destination"} {
		t.Run(name, func(t *testing.T) {
			source := newFixture(t)
			repo := openFixture(t, source)
			requireLFSCLI(t, repo)
			configureLFSFixture(t, source)
			fixtureGit(t, source, "config", "lfs.storage", "source-lfs")
			const payload = "media retained after publication"
			oid := commitLFSPushAsset(t, source, repo, "asset.asset", payload)
			root := t.TempDir()
			dest := filepath.Join(root, "existing")
			if err := os.Mkdir(dest, 0o700); err != nil {
				t.Fatal(err)
			}
			writeFixture(t, dest, "original.txt", "previous checkout")
			storage := "../../shared-lfs"
			wantCache := filepath.Join(root, "shared-lfs")
			if name != "parent-relative shared cache" {
				wantCache = filepath.Join(dest, "private-lfs")
				storage = wantCache
				if err := os.Mkdir(wantCache, 0o700); err != nil {
					t.Fatal(err)
				}
				writeFixture(t, wantCache, "original.txt", "existing cache belongs to the backup")
				switch name {
				case "case variant of destination":
					storage = filepath.Join(root, "EXISTING", "private-lfs")
					if _, err := os.Stat(storage); errors.Is(err, os.ErrNotExist) {
						t.Skip("case-sensitive destination volume")
					} else if err != nil {
						t.Fatal(err)
					}
				case "symlink to cache inside destination":
					storage = filepath.Join(root, "linked-lfs")
					if err := os.Symlink(wantCache, storage); err != nil {
						t.Skipf("symbolic links are unavailable: %v", err)
					}
				}
			}
			config := filepath.Join(root, "gitconfig")
			fixtureGit(t, source, "config", "--file", config, "filter.lfs.process", "git-lfs filter-process")
			fixtureGit(t, source, "config", "--file", config, "filter.lfs.required", "true")
			fixtureGit(t, source, "config", "--file", config, "lfs.storage", storage)
			t.Setenv("GIT_CONFIG_GLOBAL", config)
			target, err := PrepareClone(t.Context(), dest)
			if err != nil {
				t.Fatal(err)
			}
			if name == "symlink to cache inside destination" {
				replacement := t.TempDir()
				options := CloneOptions{BeforePublish: func(_ context.Context, _ string) error {
					if err := os.Remove(storage); err != nil {
						return err
					}
					return os.Symlink(replacement, storage)
				}}
				result, err := target.Clone(t.Context(), source, true, options)
				if err == nil || result.Repo != nil || lfsRead(t, dest, "original.txt") != "previous checkout" {
					t.Fatalf("clone accepted a switched cache alias: %+v, %v", result, err)
				}
				if err := os.Remove(storage); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(wantCache, storage); err != nil {
					t.Fatal(err)
				}
			}
			result, err := target.Clone(t.Context(), source, true, CloneOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if got := lfsRead(t, wantCache, lfsObjectPath(oid)); got != payload {
				t.Fatalf("clone did not populate its final cache: %q", got)
			}
			if name != "parent-relative shared cache" {
				backupCache := filepath.Join(result.Backup, "private-lfs")
				if got := lfsRead(t, backupCache, "original.txt"); got != "existing cache belongs to the backup" {
					t.Fatalf("clone modified the previous cache: %q", got)
				}
				if _, err := os.Stat(filepath.Join(backupCache, lfsObjectPath(oid))); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("the new cache was populated inside the backup: %v", err)
				}
			}
			assertNoCloneStage(t, root)
		})
	}
}

func TestCloneRefusesASymlinkedLFSConfigBeforeDownloading(t *testing.T) {
	source := newFixture(t)
	repo := openFixture(t, source)
	requireLFSCLI(t, repo)
	configureLFSFixture(t, source)
	commitLFSPushAsset(t, source, repo, "asset.asset", "media with invalid project configuration")
	hash := fixtureCommand(t, source, "hash-object", "-w", "--stdin")
	hash.Stdin = strings.NewReader("[lfs]\n\turl = http://127.0.0.1:1/not-authorized\n")
	out, err := hash.Output()
	if err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, source, "update-index", "--add", "--cacheinfo", "120000,"+strings.TrimSpace(string(out))+",.lfsconfig")
	fixtureGit(t, source, "commit", "-m", "Create a non-regular LFS config")
	root := t.TempDir()
	config := filepath.Join(root, "gitconfig")
	fixtureGit(t, source, "config", "--file", config, "filter.lfs.process", "git-lfs filter-process")
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	target, err := PrepareClone(t.Context(), filepath.Join(root, "destination"))
	if err != nil {
		t.Fatal(err)
	}
	if result, err := target.Clone(t.Context(), source, false, CloneOptions{}); result.Repo != nil ||
		err == nil || !strings.Contains(err.Error(), ".lfsconfig is not a stable regular file") {
		t.Fatalf("clone accepted a symlink body as project LFS configuration: %+v, %v", result, err)
	}
	assertNoCloneStage(t, root)
}

func TestCloneRejectsMissingOrChangedLFSMediaBeforePublication(t *testing.T) {
	source := newFixture(t)
	repo := openFixture(t, source)
	requireLFSCLI(t, repo)
	configureLFSFixture(t, source)
	const payload = "verified large file data"
	oid := commitLFSPushAsset(t, source, repo, "asset.asset", payload)
	sourceMedia := filepath.Join(repo.CommonDir, "lfs", lfsObjectPath(oid))
	root := t.TempDir()
	dest := filepath.Join(root, "existing")
	if err := os.Mkdir(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dest, "original.txt", "previous checkout")
	config := filepath.Join(root, "gitconfig")
	fixtureGit(t, source, "config", "--file", config, "filter.lfs.process", "git-lfs filter-process")
	fixtureGit(t, source, "config", "--file", config, "filter.lfs.required", "true")
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	target, err := PrepareClone(t.Context(), dest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sourceMedia); err != nil {
		t.Fatal(err)
	}
	if result, err := target.Clone(t.Context(), source, true, CloneOptions{}); err == nil || result.Repo != nil {
		t.Fatalf("clone published despite absent remote media: %+v, %v", result, err)
	}
	assertNoCloneStage(t, root)
	if err := os.WriteFile(sourceMedia, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	injected := false
	if result, err := target.Clone(t.Context(), source, true, CloneOptions{BeforePublish: func(_ context.Context, _ string) error {
		dirs, err := filepath.Glob(filepath.Join(root, ".gitwave-clone-*", "repository"))
		if err != nil || len(dirs) != 1 {
			t.Fatalf("could not locate the staged checkout: %v, %v", dirs, err)
		}
		injected = true
		return os.Remove(filepath.Join(dirs[0], ".git", "lfs", lfsObjectPath(oid)))
	}}); err == nil || result.Repo != nil || !injected {
		t.Fatalf("clone published after losing a verified object: %+v, %v", result, err)
	}
	if got := lfsRead(t, dest, "original.txt"); got != "previous checkout" {
		t.Fatalf("failed clone changed the original checkout: %q", got)
	}
	assertNoCloneStage(t, root)
}

func TestCloneResolvesTheGitSourceOnceAndPreservesItsConfiguredName(t *testing.T) {
	source, other := newFixture(t), newFixture(t)
	fixtureGit(t, source, "commit", "--allow-empty", "-m", "selected source")
	fixtureGit(t, other, "commit", "--allow-empty", "-m", "wrong second rewrite")
	root := t.TempDir()
	config := filepath.Join(root, "gitconfig")
	const alias = "fixture:source"
	fixtureGit(t, source, "config", "--file", config, "url."+source+".insteadOf", alias)
	fixtureGit(t, source, "config", "--file", config, "url."+other+".insteadOf", source)
	fixtureGit(t, source, "config", "--file", config, "clone.defaultRemoteName", "upstream")
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	dest := filepath.Join(root, "checkout")
	target, err := PrepareClone(t.Context(), dest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := target.Clone(t.Context(), alias, false, CloneOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := gitValue(t, dest, "rev-parse", "HEAD"); got != gitValue(t, source, "rev-parse", "HEAD") {
		t.Fatalf("clone resolved the rewrite chain more than once: %s", got)
	}
	if got := gitValue(t, dest, "config", "--local", "--get", "remote.upstream.url"); got != alias {
		t.Fatalf("clone rewrote the stored repository address or remote name: %q", got)
	}
}

func TestCloneRetriesGitAndLFSAuthenticationWithoutReplacingTheDestination(t *testing.T) {
	source := newFixture(t)
	repo := openFixture(t, source)
	requireLFSCLI(t, repo)
	configureLFSFixture(t, source)
	const payload = "authenticated LFS object bytes\x00"
	oid := commitLFSPushAsset(t, source, repo, "selected.asset", payload)
	commitLFSPushAsset(t, source, repo, "excluded.asset", payload)
	const gitUser, gitToken = "fixture-git-user", "fixture-git-token"
	const mediaUser, mediaToken = "fixture-media-user", "fixture-media-token"
	files := http.StripPrefix("/project.git/", http.FileServer(http.Dir(repo.GitDir)))
	gitServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, secret, ok := r.BasicAuth()
		if !ok || user != gitUser || secret != gitToken {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Git Fixture\"")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		files.ServeHTTP(w, r)
	}))
	t.Cleanup(gitServer.Close)
	var mediaServer *httptest.Server
	var received atomic.Int32
	var checkingOut atomic.Bool
	var checkoutRequests atomic.Int32
	mediaServer = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if checkingOut.Load() {
			checkoutRequests.Add(1)
		}
		switch r.URL.Path {
		case "/lfs/objects/batch":
			user, secret, ok := r.BasicAuth()
			if !ok || user != mediaUser || secret != mediaToken {
				w.Header().Set("WWW-Authenticate", "Basic realm=\"Media Fixture\"")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var batch struct {
				Operation string
				Objects   []struct {
					OID  string
					Size int
				}
			}
			if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&batch); err != nil ||
				batch.Operation != "download" || len(batch.Objects) != 1 || batch.Objects[0].OID != oid ||
				batch.Objects[0].Size != len(payload) {
				http.Error(w, "invalid download batch", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.git-lfs+json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"transfer": "basic", "objects": []any{map[string]any{
					"oid": oid, "size": len(payload), "authenticated": true,
					"actions": map[string]any{"download": map[string]any{
						"href": mediaServer.URL + "/media/" + oid,
					}},
				}},
			})
		case "/media/" + oid:
			if r.Header.Get("Authorization") != "" {
				http.Error(w, "unexpected media authorization", http.StatusBadRequest)
				return
			}
			received.Add(1)
			_, _ = io.WriteString(w, payload)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mediaServer.Close)
	config := useTestTLSConfig(t, source, gitServer)
	executable, err := repo.lfsExecutable(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	filter := "'" + strings.ReplaceAll(filepath.ToSlash(executable), "'", "'\\''") + "' filter-process"
	fixtureGit(t, source, "config", "--file", config, "filter.lfs.process", filter)
	fixtureGit(t, source, "config", "--file", config, "filter.lfs.required", "true")
	setLFSURL := func(endpoint string) {
		t.Helper()
		writeFixture(t, source, ".lfsconfig", "[lfs]\n\turl = "+endpoint+"\n\tfetchexclude = excluded.asset\n")
		fixtureGit(t, source, "add", "--", ".lfsconfig")
		fixtureGit(t, source, "commit", "-m", "Select the LFS endpoint")
		fixtureGit(t, source, "update-server-info")
	}
	endpoint := mediaServer.URL + "/lfs"
	setLFSURL(endpoint)
	root := t.TempDir()
	dest := filepath.Join(root, "existing")
	if err := os.Mkdir(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dest, "original.txt", "keep prior checkout")
	target, err := PrepareClone(t.Context(), dest)
	if err != nil {
		t.Fatal(err)
	}
	remote := gitServer.URL + "/project.git"
	assertPending := func() {
		t.Helper()
		if got := lfsRead(t, dest, "original.txt"); got != "keep prior checkout" {
			t.Fatalf("unfinished clone changed existing content: %q", got)
		}
		assertNoCloneStage(t, root)
	}
	var gitChallenge *RemoteAuthError
	if _, err := target.Clone(t.Context(), remote, true, CloneOptions{}); !errors.As(err, &gitChallenge) ||
		gitChallenge.LFSLocation() != "" {
		t.Fatalf("clone did not request Git credentials: %v", err)
	}
	assertPending()
	options := CloneOptions{Challenge: gitChallenge, Credentials: &HTTPSCredential{Username: gitUser, Password: gitToken}}
	var mediaChallenge *RemoteAuthError
	if _, err := target.Clone(t.Context(), remote, true, options); !errors.As(err, &mediaChallenge) ||
		mediaChallenge.LFSLocation() != endpoint {
		t.Fatalf("clone did not request credentials for the separate LFS endpoint: %v", err)
	}
	assertPending()
	options.Challenge = mediaChallenge
	options.MediaCredentials = &HTTPSCredential{Username: mediaUser, Password: mediaToken}
	options.Credentials = nil
	options.Vault = &fixtureCredentialVault{lookup: func(endpoint, _ string) (HTTPSCredential, bool) {
		if endpoint != remote {
			return HTTPSCredential{}, false
		}
		return HTTPSCredential{Username: gitUser, Password: gitToken}, true
	}}
	setLFSURL(mediaServer.URL + "/different")
	if _, err := target.Clone(t.Context(), remote, true, options); err == nil ||
		!strings.Contains(err.Error(), "Git LFS endpoint changed") || received.Load() != 0 {
		t.Fatalf("clone used the old confirmation for a different LFS destination: %v", err)
	}
	assertPending()
	setLFSURL(endpoint)
	options.Progress = func(progress TransferProgress) {
		if progress.Phase != "Checking out files" || !checkingOut.CompareAndSwap(false, true) {
			return
		}
		dirs, err := filepath.Glob(filepath.Join(root, ".gitwave-clone-*", "repository"))
		if err != nil || len(dirs) != 1 {
			t.Errorf("could not locate the staged clone during checkout: %v, %v", dirs, err)
			return
		}
		if err := os.Remove(filepath.Join(dirs[0], ".git", "lfs", lfsObjectPath(oid))); err != nil {
			t.Errorf("could not remove the object before checkout: %v", err)
		}
	}
	if result, err := target.Clone(t.Context(), remote, true, options); err == nil || result.Repo != nil ||
		!checkingOut.Load() || received.Load() != 1 || checkoutRequests.Load() != 0 {
		t.Fatalf("checkout accepted missing media or contacted the network: %+v, %v, downloads=%d, checkout requests=%d",
			result, err, received.Load(), checkoutRequests.Load())
	}
	assertPending()
	checkingOut.Store(false)
	received.Store(0)
	options.Progress = nil
	result, err := target.Clone(t.Context(), remote, true, options)
	if err != nil || result.Repo == nil || result.Backup == "" || received.Load() != 1 {
		t.Fatalf("authenticated clone did not publish after downloading exactly once: %+v, %v", result, err)
	}
	if got := lfsRead(t, dest, "selected.asset"); got != payload {
		t.Fatalf("cloned LFS bytes differ: %q", got)
	}
	if got := lfsRead(t, dest, "excluded.asset"); !strings.Contains(got, "oid sha256:"+oid) {
		t.Fatalf("excluded clone path was materialized from the shared object: %q", got)
	}
	if got := lfsRead(t, result.Backup, "original.txt"); got != "keep prior checkout" {
		t.Fatalf("successful clone changed the backup: %q", got)
	}
	gitConfig := lfsRead(t, result.Repo.GitDir, "config")
	if strings.Contains(gitConfig, gitToken) || strings.Contains(gitConfig, mediaToken) || strings.Contains(gitConfig, "gitwave-clone:") {
		t.Fatal("clone persisted an inline credential or a private transport alias")
	}
	assertNoCloneStage(t, root)
}

func TestCloneBackupAndFailureLeaveDestinationFilesIntact(t *testing.T) {
	source := newFixture(t)
	dest := t.TempDir()
	writeFixture(t, dest, "keep.txt", "prior data\n")
	target, err := PrepareClone(t.Context(), dest)
	if err != nil || !target.NeedsReplacement() {
		t.Fatalf("expected confirmation: %+v, %v", target, err)
	}
	if _, err := target.Clone(t.Context(), source, false, CloneOptions{}); err == nil {
		t.Fatal("clone replaced an unconfirmed destination")
	}
	missing := filepath.Join(t.TempDir(), "missing source")
	if _, err := target.Clone(t.Context(), missing, true, CloneOptions{}); err == nil {
		t.Fatal("cloning a missing remote succeeded")
	}
	if got, err := os.ReadFile(filepath.Join(dest, "keep.txt")); err != nil || string(got) != "prior data\n" {
		t.Fatalf("failed clone changed the original destination: %q, %v", got, err)
	}
	result, err := target.Clone(t.Context(), source, true, CloneOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Backup == "" || result.Repo == nil {
		t.Fatalf("replacement did not return an original directory and new repo: %+v", result)
	}
	if got, err := os.ReadFile(filepath.Join(result.Backup, "keep.txt")); err != nil || string(got) != "prior data\n" {
		t.Fatalf("publish lost original contents: %q, %v", got, err)
	}
	defer os.RemoveAll(result.Backup)
	assertNoCloneStage(t, filepath.Dir(target.Path()))
}

func TestCloneRejectsFilesAddedAfterSelection(t *testing.T) {
	source := newFixture(t)
	root := t.TempDir()
	dest := filepath.Join(root, "existing folder")
	if err := os.Mkdir(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	target, err := PrepareClone(t.Context(), dest)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dest, "late.txt", "created after selection\n")
	if _, err := target.Clone(t.Context(), source, false, CloneOptions{}); !errors.Is(err, ErrCloneTargetChanged) {
		t.Fatalf("expected refusal after a concurrent change: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dest, "late.txt")); err != nil || string(got) != "created after selection\n" {
		t.Fatalf("late file changed: %q, %v", got, err)
	}
	assertNoCloneStage(t, root)
}

func TestCloneRechecksDestinationAfterDownload(t *testing.T) {
	source := newFixture(t)
	root := t.TempDir()
	dest := filepath.Join(root, "existing destination")
	if err := os.Mkdir(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	target, err := PrepareClone(t.Context(), dest)
	if err != nil {
		t.Fatal(err)
	}
	late := filepath.Join(dest, "new file.txt")
	var wrote bool
	var writeErr error
	_, err = target.Clone(t.Context(), source, false, CloneOptions{Progress: func(TransferProgress) {
		if !wrote {
			wrote = true
			writeErr = os.WriteFile(late, []byte("added during the download"), 0o600)
		}
	}})
	if !wrote || writeErr != nil || !errors.Is(err, ErrCloneTargetChanged) {
		t.Fatalf("clone did not detect the new file during download: wrote=%t writeErr=%v cloneErr=%v", wrote, writeErr, err)
	}
	if content, err := os.ReadFile(late); err != nil || string(content) != "added during the download" {
		t.Fatalf("concurrent file changed after clone: %q, %v", content, err)
	}
	assertNoCloneStage(t, root)
}

func TestCloneRejectsReplacingTheLocalSource(t *testing.T) {
	source := newFixture(t)
	for _, destination := range []string{source, filepath.Dir(source)} {
		target, err := PrepareClone(t.Context(), destination)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := target.Clone(t.Context(), source, true, CloneOptions{}); err == nil ||
			!strings.Contains(err.Error(), "contains the source") {
			t.Fatalf("clone accepted replacing part of its source: %v", err)
		}
	}
}

func TestCloneRejectsUnrepresentableCheckoutBeforeReplacement(t *testing.T) {
	source := newFixture(t)
	fixtureGit(t, source, "update-index", "--add", "--cacheinfo", "100644,"+gitValue(t, source, "rev-parse", "HEAD:mixed.txt")+",Report.txt")
	fixtureGit(t, source, "update-index", "--add", "--cacheinfo", "100644,"+gitValue(t, source, "rev-parse", "HEAD:conflict.txt")+",report.txt")
	fixtureGit(t, source, "commit", "-m", "Distinct paths differing only in case")
	root := t.TempDir()
	dir, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	caseInsensitive, _, err := probeCheckoutNames(dir)
	dir.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !caseInsensitive {
		t.Skip("the filesystem can represent both names")
	}
	dest := filepath.Join(root, "existing destination")
	if err := os.Mkdir(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dest, "untouched.txt", "keep original data\n")
	target, err := PrepareClone(t.Context(), dest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := target.Clone(t.Context(), source, true, CloneOptions{}); err == nil {
		t.Fatal("clone published a checkout with colliding paths")
	}
	if got, err := os.ReadFile(filepath.Join(dest, "untouched.txt")); err != nil || string(got) != "keep original data\n" {
		t.Fatalf("lossy clone changed the destination: %q, %v", got, err)
	}
	assertNoCloneStage(t, root)
}

func TestCloneCancellationCleansPrivateDirectory(t *testing.T) {
	source := newFixture(t)
	root := t.TempDir()
	dest := filepath.Join(root, "destination")
	target, err := PrepareClone(t.Context(), dest)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var progressSeen bool
	result, err := target.Clone(ctx, source, false, CloneOptions{Progress: func(p TransferProgress) {
		progressSeen = true
		cancel()
	}})
	if !progressSeen || !errors.Is(err, context.Canceled) || result.Repo != nil {
		t.Fatalf("cancel on clone progress: %+v, got progress %t, error %v", result, progressSeen, err)
	}
	if _, err := os.Lstat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancel left a published destination: %v", err)
	}
	assertNoCloneStage(t, root)
}

func TestCloneHTTPSRetriesWithScopedCredentials(t *testing.T) {
	source := newFixture(t)
	repo := openFixture(t, source)
	fixtureGit(t, source, "update-server-info")
	const username, password = "gitwave-test", "fixture-token"
	files := http.StripPrefix("/project.git/", http.FileServer(http.Dir(repo.GitDir)))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, token, ok := r.BasicAuth()
		if !ok || user != username || token != password {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"GitWave test\"")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		files.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	gitConfig := useTestTLSConfig(t, source, server)
	root := t.TempDir()
	dest := filepath.Join(root, "destination")
	target, err := PrepareClone(t.Context(), dest)
	if err != nil {
		t.Fatal(err)
	}
	remote := server.URL + "/project.git/"
	if _, err := target.Clone(t.Context(), remote, false, CloneOptions{}); !errors.Is(err, ErrHTTPSAuthentication) {
		t.Fatalf("expected an HTTPS authentication error: %v", err)
	}
	bad := CloneOptions{Credentials: &HTTPSCredential{Username: username, Password: "incorrect"}}
	if _, err := target.Clone(t.Context(), remote, false, bad); !errors.Is(err, ErrHTTPSAuthentication) {
		t.Fatalf("expected an HTTPS authentication error for wrong credentials: %v", err)
	}
	good := CloneOptions{Credentials: &HTTPSCredential{Username: username, Password: password, Remember: true}}
	result, err := target.Clone(t.Context(), remote, false, good)
	if err != nil {
		t.Fatal(err)
	}
	if result.Repo == nil || readFixtureStatus(t, result.Repo).HeadOID != gitValue(t, source, "rev-parse", "HEAD") {
		t.Fatalf("unexpected authenticated clone: %+v", result)
	}
	if result.Warning == "" {
		t.Fatal("clone claimed the credentials could be reused without a configured helper")
	}
	if origin := gitValue(t, dest, "remote", "get-url", "origin"); origin != remote {
		t.Fatalf("clone persisted an unexpected origin: %q", origin)
	}
	file := filepath.ToSlash(filepath.Join(filepath.Dir(gitConfig), "credentials"))
	helper := "store --file='" + strings.ReplaceAll(file, "'", "'\\''") + "'"
	fixtureGit(t, source, "config", "--file", gitConfig, "credential.helper", helper)
	saved, err := PrepareClone(t.Context(), filepath.Join(root, "with-helper"))
	if err != nil {
		t.Fatal(err)
	}
	if result, err := saved.Clone(t.Context(), remote, false, good); err != nil || result.Warning != "" {
		t.Fatalf("configured Git helper could not remember the credentials: %q, %v", result.Warning, err)
	}
	reuse, err := PrepareClone(t.Context(), filepath.Join(root, "reuse-helper"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reuse.Clone(t.Context(), remote, false, CloneOptions{}); err != nil {
		t.Fatalf("clone could not reuse the verified Git helper entry: %v", err)
	}
	assertNoCloneStage(t, root)
}

func useTestTLSConfig(t *testing.T, repo string, server *httptest.Server) string {
	t.Helper()
	config := t.TempDir()
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(filepath.Join(config, "fixture.pem"), cert, 0o600); err != nil {
		t.Fatal(err)
	}
	gitConfig := filepath.Join(config, "gitconfig")
	fixtureGit(t, repo, "config", "--file", gitConfig, "http.sslCAInfo", filepath.Join(config, "fixture.pem"))
	fixtureGit(t, repo, "config", "--file", gitConfig, "http.schannelUseSSLCAInfo", "true")
	t.Setenv("GIT_CONFIG_GLOBAL", gitConfig)
	return gitConfig
}

func assertNoCloneStage(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".gitwave-clone-") {
			t.Fatalf("clone stage was not cleaned: %s", entry.Name())
		}
	}
}
