package gitcli

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func configureLFSFixture(t *testing.T, dir string) {
	t.Helper()
	configureLFSFixtureFilters(t, dir)
	writeFixture(t, dir, ".gitattributes", "*.asset "+lfsAttributeValues+"\n")
	fixtureGit(t, dir, "add", "--", ".gitattributes")
	fixtureGit(t, dir, "commit", "-m", "Track assets")
}

func configureLFSFixtureFilters(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	fixtureGit(t, dir, "config", "filter.lfs.clean", "git-lfs clean -- %f")
	fixtureGit(t, dir, "config", "filter.lfs.smudge", "git-lfs smudge -- %f")
	fixtureGit(t, dir, "config", "filter.lfs.process", "git-lfs filter-process")
	fixtureGit(t, dir, "config", "filter.lfs.required", "true")
}

func lfsFixtureChange(t *testing.T, status Status, path string) Change {
	t.Helper()
	for _, change := range status.Changes {
		if change.Path == path {
			return change
		}
	}
	t.Fatalf("changed path %q is missing from %+v", path, status.Changes)
	return Change{}
}

func TestLFSWorktreeStageCommitAndCheckout(t *testing.T) {
	dir := newFixture(t)
	configureLFSFixture(t, dir)
	base := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "branch", "plain")
	repo := openFixture(t, dir)
	const name = "large file.asset"
	const data = "raw LFS object\x00with\r\nbinary content\n"
	writeFixture(t, dir, name, data)
	shown := readFixtureStatus(t, repo)
	if err := repo.SetStaged(t.Context(), shown, []Change{lfsFixtureChange(t, shown, name)}, true); err != nil {
		t.Fatal(err)
	}
	oid := fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
	wantPointer := fmt.Sprintf("version %s\noid sha256:%s\nsize %d\n", lfsPointerVersion, oid, len(data))
	if got := fixtureGit(t, dir, "show", ":"+name); got != wantPointer {
		t.Fatalf("the private index did not stage the canonical LFS pointer: %q", got)
	}
	storage := filepath.Join(repo.CommonDir, "lfs")
	objectPath := filepath.Join(storage, lfsObjectPath(oid))
	if got := lfsRead(t, filepath.Dir(objectPath), filepath.Base(objectPath)); got != data {
		t.Fatalf("stage did not publish the real LFS object: %q", got)
	}
	if got := lfsRead(t, dir, name); got != data {
		t.Fatalf("stage rewrote the working file: %q", got)
	}
	shown = readFixtureStatus(t, repo)
	staged := lfsFixtureChange(t, shown, name)
	if staged.Index != 'A' || staged.WorkTree != '.' {
		t.Fatalf("hydrated LFS content was reported as an unstaged edit: %+v", staged)
	}
	commit, err := repo.Commit(t.Context(), shown, "Commit LFS asset", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := readFixtureStatus(t, repo); len(got.Changes) != 0 {
		t.Fatalf("hydrated committed LFS file did not compare clean: %+v", got.Changes)
	}

	switchTo := func(ref, oid string) error {
		t.Helper()
		plan, err := repo.PrepareCheckout(t.Context(), CheckoutTarget{Ref: ref, OID: oid})
		if err != nil {
			return err
		}
		_, err = plan.Apply(t.Context())
		return err
	}
	if err := switchTo("refs/heads/plain", base); err != nil {
		t.Fatal(err)
	}
	if err := switchTo("refs/heads/main", commit.SHA); err != nil {
		t.Fatal(err)
	}
	if got := lfsRead(t, dir, name); got != data {
		t.Fatalf("checkout failed to hydrate a cached object: %q", got)
	}
	if err := os.Remove(objectPath); err != nil {
		t.Fatal(err)
	}
	if err := switchTo("refs/heads/plain", base); err != nil {
		t.Fatal(err)
	}
	if got := lfsRead(t, filepath.Dir(objectPath), filepath.Base(objectPath)); got != data {
		t.Fatalf("checkout discarded the only hydrated copy of the asset: %q", got)
	}
	writeFixture(t, filepath.Dir(objectPath), filepath.Base(objectPath), strings.Repeat("x", len(data)))
	if err := switchTo("refs/heads/main", commit.SHA); err == nil {
		t.Fatal("checkout accepted a corrupt same-size LFS object")
	}
	if got := gitValue(t, dir, "symbolic-ref", "-q", "HEAD"); got != "refs/heads/plain" {
		t.Fatalf("failed checkout changed HEAD: %q", got)
	}
	if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
		t.Fatalf("failed checkout changed the working path: %v", err)
	}
	if err := os.Remove(objectPath); err != nil {
		t.Fatal(err)
	}
	if err := switchTo("refs/heads/main", commit.SHA); err != nil {
		t.Fatal(err)
	}
	if got := lfsRead(t, dir, name); got != wantPointer {
		t.Fatalf("offline checkout did not preserve the missing object pointer: %q", got)
	}
	if got := readFixtureStatus(t, repo); len(got.Changes) != 0 {
		t.Fatalf("offline pointer showed a dirty change: %+v", got.Changes)
	}
}

func TestLFSWorktreeReadDoesNotPopulateCustomStorage(t *testing.T) {
	dir := newFixture(t, "--object-format=sha256")
	configureLFSFixture(t, dir)
	cache := filepath.Join(t.TempDir(), "custom cache %f")
	fixtureGit(t, dir, "config", "lfs.storage", cache)
	linkedDir := filepath.Join(t.TempDir(), "other checkout")
	fixtureGit(t, dir, "worktree", "add", "-b", "linked", "--", linkedDir)
	repo := openFixture(t, linkedDir)
	const name = "file.asset"
	const first = "stage and commit an LFS object\n"
	writeFixture(t, linkedDir, name, first)
	shown := readFixtureStatus(t, repo)
	if err := repo.SetStaged(t.Context(), shown, []Change{lfsFixtureChange(t, shown, name)}, true); err != nil {
		t.Fatal(err)
	}
	firstOID := fmt.Sprintf("%x", sha256.Sum256([]byte(first)))
	if got := lfsRead(t, cache, lfsObjectPath(firstOID)); got != first {
		t.Fatalf("stage ignored the custom storage directory: %q", got)
	}
	commit, err := repo.Commit(t.Context(), readFixtureStatus(t, repo), "Save linked worktree asset", "")
	if err != nil {
		t.Fatal(err)
	}
	const edited = "user modified the checked-out content\x00"
	writeFixture(t, linkedDir, name, edited)
	shown = readFixtureStatus(t, repo)
	change := lfsFixtureChange(t, shown, name)
	if change.WorkTree != 'M' || change.Index != '.' {
		t.Fatalf("edited LFS asset not shown in Changes: %+v", change)
	}
	if _, err := repo.WorkingDiff(t.Context(), change, false, false); err != nil {
		t.Fatal(err)
	}
	editedOID := fmt.Sprintf("%x", sha256.Sum256([]byte(edited)))
	if _, err := os.Lstat(filepath.Join(cache, lfsObjectPath(editedOID))); !os.IsNotExist(err) {
		t.Fatalf("reading a changed LFS object wrote the cache: %v", err)
	}
	if got := gitValue(t, dir, "rev-parse", "linked"); got != commit.SHA {
		t.Fatalf("linked branch commit was not visible in the main repository: %q", got)
	}
}

func TestLFSNativeCleanMatchesOfficialCLI(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	requireLFSCLI(t, repo)
	for _, data := range []string{"", "content with\x00binary\r\n", strings.Repeat("long object", 2000),
		"version https://hawser.github.com/spec/v1\noid sha256:" + strings.Repeat("2", 64) + "\nsize 42\n"} {
		cmd := fixtureCommand(t, dir, "lfs", "clean", "--", "file.asset")
		cmd.Stdin = strings.NewReader(data)
		want, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		var got bytes.Buffer
		if err := cleanLFS(strings.NewReader(data), &got, ""); err != nil || !bytes.Equal(got.Bytes(), want) {
			t.Fatalf("LFS clean differs from the CLI for %d bytes: GitWave=%q, CLI=%q, err=%v", len(data), got.Bytes(), want, err)
		}
	}
}

func TestLFSWorktreeStashRetainsTrackedAndUntrackedContent(t *testing.T) {
	dir := newFixture(t)
	configureLFSFixture(t, dir)
	repo := openFixture(t, dir)
	const name = "existing.asset"
	const original = "original content\x00"
	writeFixture(t, dir, name, original)
	shown := readFixtureStatus(t, repo)
	if err := repo.SetStaged(t.Context(), shown, []Change{lfsFixtureChange(t, shown, name)}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Commit(t.Context(), readFixtureStatus(t, repo), "Save tracked LFS asset", ""); err != nil {
		t.Fatal(err)
	}
	const changed = "changed content\x00"
	const fresh = "untracked content\x00"
	writeFixture(t, dir, name, changed)
	writeFixture(t, dir, "fresh.asset", fresh)
	oid, err := repo.SaveStash(t.Context(), readFixtureStatus(t, repo), "Save LFS bytes", true)
	if err != nil {
		t.Fatal(err)
	}
	if got := lfsRead(t, dir, name); got != original {
		t.Fatalf("stash did not restore the committed bytes: %q", got)
	}
	if _, err := os.Lstat(filepath.Join(dir, "fresh.asset")); !os.IsNotExist(err) {
		t.Fatalf("stash did not remove the saved untracked path: %v", err)
	}
	for _, item := range []struct{ ref, data string }{{oid + ":" + name, changed}, {oid + "^3:fresh.asset", fresh}} {
		ptr, ok := parseLFSPointer([]byte(fixtureGit(t, dir, "show", item.ref)))
		if !ok || ptr.oid != fmt.Sprintf("%x", sha256.Sum256([]byte(item.data))) {
			t.Fatalf("stash did not store the correct pointer for %s: %+v", item.ref, ptr)
		}
		if got := lfsRead(t, filepath.Join(repo.CommonDir, "lfs"), lfsObjectPath(ptr.oid)); got != item.data {
			t.Fatalf("stash lost the referenced LFS object: %q", got)
		}
	}
	entries, err := repo.Stashes(t.Context())
	if err != nil || len(entries) == 0 || entries[0].OID != oid {
		t.Fatalf("stash not found: %+v, %v", entries, err)
	}
	const renamed = "renamed.asset"
	fixtureGit(t, dir, "mv", "--", name, renamed)
	if _, err := repo.Commit(t.Context(), readFixtureStatus(t, repo), "Rename the asset", ""); err != nil {
		t.Fatal(err)
	}
	originalOID := fmt.Sprintf("%x", sha256.Sum256([]byte(original)))
	cache := filepath.Join(repo.CommonDir, "lfs")
	if err := os.Remove(filepath.Join(cache, lfsObjectPath(originalOID))); err != nil {
		t.Fatal(err)
	}
	if result, err := repo.ApplyStash(t.Context(), entries[0], readFixtureStatus(t, repo)); err != nil || len(result.Conflicts) != 0 {
		t.Fatalf("apply stashed LFS assets: %+v, %v", result, err)
	}
	if got := lfsRead(t, cache, lfsObjectPath(originalOID)); got != original {
		t.Fatalf("apply lost the last hydrated copy of the original asset: %q", got)
	}
	if got := lfsRead(t, dir, renamed); got != changed {
		t.Fatalf("apply did not restore the modified bytes at the renamed path: %q", got)
	}
	if got := lfsRead(t, dir, "fresh.asset"); got != fresh {
		t.Fatalf("apply restored pointer bytes instead of untracked content: %q", got)
	}
}

func TestLFSCheckoutTracksTargetAttributeRemoval(t *testing.T) {
	dir := newFixture(t)
	configureLFSFixture(t, dir)
	fixtureGit(t, dir, "switch", "-c", "without-lfs")
	fixtureGit(t, dir, "rm", "--", ".gitattributes")
	oid := strings.Repeat("2", 64)
	text := fmt.Sprintf("version %s\noid sha256:%s\nsize 5\n", lfsPointerVersion, oid)
	writeFixture(t, dir, "ordinary.asset", text)
	fixtureGit(t, dir, "add", "--", "ordinary.asset")
	fixtureGit(t, dir, "commit", "-m", "Store pointer-like ordinary file")
	plain := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "switch", "main")
	repo := openFixture(t, dir)
	cachePath := filepath.Join(repo.CommonDir, "lfs", lfsObjectPath(oid))
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Dir(cachePath), filepath.Base(cachePath), "bad!!")
	plan, err := repo.PrepareCheckout(t.Context(), CheckoutTarget{Ref: "refs/heads/without-lfs", OID: plain})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := lfsRead(t, dir, "ordinary.asset"); got != text {
		t.Fatalf("checkout interpreted an ordinary file as an LFS pointer: %q", got)
	}
}

func TestLFSWorktreeRejectsRepositoryFormatItCannotDecode(t *testing.T) {
	dir := newFixture(t)
	configureLFSFixture(t, dir)
	fixtureGit(t, dir, "config", lfsFormatKey, "99")
	repo := openFixture(t, dir)
	writeFixture(t, dir, "file.asset", "unencoded source")
	shown := readFixtureStatus(t, repo)
	if err := repo.SetStaged(t.Context(), shown, []Change{lfsFixtureChange(t, shown, "file.asset")}, true); err == nil {
		t.Fatal("a repository with an unknown LFS format was staged")
	}
	if cached := gitValue(t, dir, "diff", "--cached", "--name-only"); cached != "" {
		t.Fatalf("staging with an unknown LFS format modified the index: %q", cached)
	}
}

func TestLFSCheckoutHonorsSkipSmudgeEnvironment(t *testing.T) {
	dir := newFixture(t)
	configureLFSFixture(t, dir)
	base := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "branch", "without-assets")
	repo := openFixture(t, dir)
	const path = "file.asset"
	writeFixture(t, dir, path, "materialized asset\n")
	status := readFixtureStatus(t, repo)
	if err := repo.SetStaged(t.Context(), status, []Change{lfsFixtureChange(t, status, path)}, true); err != nil {
		t.Fatal(err)
	}
	pointer := fixtureGit(t, dir, "show", ":"+path)
	commit, err := repo.Commit(t.Context(), readFixtureStatus(t, repo), "Track cached asset", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_LFS_SKIP_SMUDGE", "1")
	for _, info := range []CheckoutTarget{{Ref: "refs/heads/without-assets", OID: base}, {Ref: "refs/heads/main", OID: commit.SHA}} {
		plan, err := repo.PrepareCheckout(t.Context(), info)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := plan.Apply(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if got := lfsRead(t, dir, path); got != pointer {
		t.Fatalf("checkout ignored the skip-smudge environment setting: %q", got)
	}
}

func TestLFSCheckoutPreservesLastCopyWithLocalOnlyAttributes(t *testing.T) {
	dir := newFixture(t)
	configureLFSFixture(t, dir)
	fixtureGit(t, dir, "rm", "--cached", "--", ".gitattributes")
	fixtureGit(t, dir, "commit", "-m", "Keep attributes local")
	repo := openFixture(t, dir)
	if err := os.MkdirAll(filepath.Join(repo.CommonDir, "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, repo.CommonDir, filepath.Join("info", "exclude"), ".gitattributes\n")
	base := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "branch", "without-asset")
	const path = "last-copy.asset"
	const contents = "last hydrated contents\x00\n"
	writeFixture(t, dir, path, contents)
	status := readFixtureStatus(t, repo)
	if err := repo.SetStaged(t.Context(), status, []Change{lfsFixtureChange(t, status, path)}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Commit(t.Context(), readFixtureStatus(t, repo), "Save local-only LFS asset", ""); err != nil {
		t.Fatal(err)
	}
	oid := fmt.Sprintf("%x", sha256.Sum256([]byte(contents)))
	object := filepath.Join(repo.CommonDir, "lfs", lfsObjectPath(oid))
	if err := os.Remove(object); err != nil {
		t.Fatal(err)
	}
	plan, err := repo.PrepareCheckout(t.Context(), CheckoutTarget{Ref: "refs/heads/without-asset", OID: base})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := lfsRead(t, filepath.Dir(object), filepath.Base(object)); got != contents {
		t.Fatalf("checkout lost the last copy of the LFS asset: %q", got)
	}
}
