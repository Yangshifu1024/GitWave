package gitcli

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func fixtureStashEntry(t *testing.T, repo *Repository) StashEntry {
	t.Helper()
	stashes, err := repo.Stashes(t.Context())
	if err != nil || len(stashes) == 0 {
		t.Fatalf("expected a stash: %+v, %v", stashes, err)
	}
	return stashes[0]
}

func TestApplyStashRestoresFilesAndKeepsIndependentEdits(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	writeFixture(t, dir, "mixed.txt", "stashed edit\n")
	writeFixture(t, dir, "tracked add.txt", "index version\n")
	fixtureGit(t, dir, "add", "tracked add.txt")
	writeFixture(t, dir, "tracked add.txt", "worktree version\n")
	writeFixture(t, dir, "new file.txt", "untracked stash file\n")
	fixtureGit(t, dir, "stash", "push", "-u", "-m", "restore")
	entry := fixtureStashEntry(t, repo)
	writeFixture(t, dir, "old name.txt", "independent edit\n")
	writeFixture(t, dir, "local new.txt", "independent untracked\n")
	result, err := repo.ApplyStash(t.Context(), entry, readFixtureStatus(t, repo))
	if err != nil || len(result.Conflicts) != 0 || result.Warning != nil {
		t.Fatalf("apply: %+v, %v", result, err)
	}
	for name, expected := range map[string]string{
		"mixed.txt": "stashed edit\n", "tracked add.txt": "worktree version\n", "new file.txt": "untracked stash file\n",
		"old name.txt": "independent edit\n", "local new.txt": "independent untracked\n",
	} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(data) != expected {
			t.Fatalf("%s: %q, %v", name, data, err)
		}
	}
	if got := gitValue(t, dir, "show", ":tracked add.txt"); got != "worktree version" {
		t.Fatalf("new tracked file index entry: %q", got)
	}
	if got := gitValue(t, dir, "show", ":mixed.txt"); got != "original version" {
		t.Fatalf("tracked modification changed staging: %q", got)
	}
	if got := fixtureStashEntry(t, repo); !got.SameSelection(entry) {
		t.Fatalf("Apply changed the stash: %+v", got)
	}
}

func TestApplyStashKeepsStashAndOpensConflicts(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	writeFixture(t, dir, "conflict.txt", "stashed version\n")
	writeFixture(t, dir, "from-stash.txt", "untracked content\n")
	fixtureGit(t, dir, "stash", "push", "-u", "-m", "will conflict")
	entry := fixtureStashEntry(t, repo)
	writeFixture(t, dir, "conflict.txt", "new base version\n")
	fixtureGit(t, dir, "commit", "-am", "advance the branch")
	result, err := repo.ApplyStash(t.Context(), entry, readFixtureStatus(t, repo))
	if err != nil || !slices.Equal(result.Conflicts, []string{"conflict.txt"}) {
		t.Fatalf("conflict: %+v, %v", result, err)
	}
	if stages := readFixtureStatus(t, repo).Changes; len(stages) != 2 || stages[0].Kind != 'u' || stages[1].Path != "from-stash.txt" {
		t.Fatalf("partial application: %+v", stages)
	}
	data, err := os.ReadFile(filepath.Join(dir, "conflict.txt"))
	if err != nil || !strings.Contains(string(data), "<<<<<<<") || !strings.Contains(string(data), "stashed version") ||
		!strings.Contains(string(data), "new base version") {
		t.Fatalf("conflict markers: %q, %v", data, err)
	}
	if !fixtureStashEntry(t, repo).SameSelection(entry) {
		t.Fatal("conflict removed the stash")
	}
}

func TestApplyStashRejectsStaleSelectionAndDirtyCollision(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	writeFixture(t, dir, "mixed.txt", "earlier stash\n")
	fixtureGit(t, dir, "stash", "push", "-m", "earlier")
	earlier := fixtureStashEntry(t, repo)
	writeFixture(t, dir, "conflict.txt", "newer stash\n")
	fixtureGit(t, dir, "stash", "push", "-m", "newer")
	clean := readFixtureStatus(t, repo)
	if _, err := repo.ApplyStash(t.Context(), earlier, clean); err == nil || !strings.Contains(err.Error(), "stash list changed") {
		t.Fatalf("stale stash selection: %v", err)
	}
	writeFixture(t, dir, "conflict.txt", "local edit\n")
	newer := fixtureStashEntry(t, repo)
	if _, err := repo.ApplyStash(t.Context(), newer, readFixtureStatus(t, repo)); err == nil {
		t.Fatal("applied over an unstaged edit")
	}
	if data, err := os.ReadFile(filepath.Join(dir, "conflict.txt")); err != nil || string(data) != "local edit\n" {
		t.Fatalf("local edit changed: %q, %v", data, err)
	}
	fixtureGit(t, dir, "add", "conflict.txt")
	if _, err := repo.ApplyStash(t.Context(), newer, readFixtureStatus(t, repo)); err == nil ||
		!strings.Contains(err.Error(), "staged changes") {
		t.Fatalf("staged changes were not protected: %v", err)
	}
}

func TestApplyStashProtectsIgnoredCollision(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	writeFixture(t, dir, "ignored.txt", "stash value\n")
	writeFixture(t, dir, "mixed.txt", "stash tracked edit\n")
	fixtureGit(t, dir, "stash", "push", "-u", "-m", "ignored collision")
	entry := fixtureStashEntry(t, repo)
	if err := os.MkdirAll(filepath.Join(repo.CommonDir, "info"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, repo.CommonDir, "info/exclude", "ignored.txt\n")
	writeFixture(t, dir, "ignored.txt", "local ignored value\n")
	if _, err := repo.ApplyStash(t.Context(), entry, readFixtureStatus(t, repo)); err == nil || !strings.Contains(err.Error(), "ignored.txt") {
		t.Fatalf("expected an ignored collision: %v", err)
	}
	for name, expected := range map[string]string{"ignored.txt": "local ignored value\n", "mixed.txt": "original version\n"} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(data) != expected {
			t.Fatalf("file %s changed: %q, %v", name, data, err)
		}
	}
}

func TestApplyStashRestoresSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation requires developer mode")
	}
	dir := newFixture(t)
	repo := openFixture(t, dir)
	fixtureGit(t, dir, "config", "core.symlinks", "true")
	if err := os.Symlink("missing target.txt", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "stash", "push", "-u", "-m", "link")
	if result, err := repo.ApplyStash(t.Context(), fixtureStashEntry(t, repo), readFixtureStatus(t, repo)); err != nil || result.Warning != nil {
		t.Fatalf("restore symlink: %+v, %v", result, err)
	}
	target, err := os.Readlink(filepath.Join(dir, "link"))
	if err != nil || target != "missing target.txt" {
		t.Fatalf("restored link: %q, %v", target, err)
	}
}

func TestApplyStashRestoresFileToDirectory(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	fixtureGit(t, dir, "rm", "mixed.txt")
	if err := os.Mkdir(filepath.Join(dir, "mixed.txt"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "mixed.txt/item", "new directory entry\n")
	fixtureGit(t, dir, "add", "--", "mixed.txt/item")
	fixtureGit(t, dir, "stash", "push", "-m", "directory replacement")
	if result, err := repo.ApplyStash(t.Context(), fixtureStashEntry(t, repo), readFixtureStatus(t, repo)); err != nil || result.Warning != nil {
		t.Fatalf("apply directory replacement: %+v, %v", result, err)
	}
	path := filepath.Join(dir, "mixed.txt", "item")
	if data, err := os.ReadFile(path); err != nil || string(data) != "new directory entry\n" {
		t.Fatalf("restored file: %q, %v", data, err)
	}
}

func TestApplyStashRestoresOutsideSparseCone(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	for _, p := range []string{"included", "excluded"} {
		if err := os.Mkdir(filepath.Join(dir, p), 0o700); err != nil {
			t.Fatal(err)
		}
		writeFixture(t, dir, p+"/tracked", "base\n")
		writeFixture(t, dir, p+"/untouched", "untouched\n")
	}
	fixtureGit(t, dir, "add", ".")
	fixtureGit(t, dir, "commit", "-m", "sparse paths")
	writeFixture(t, dir, "included/tracked", "included edit\n")
	writeFixture(t, dir, "excluded/tracked", "excluded edit\n")
	fixtureGit(t, dir, "stash", "push", "-m", "sparse stash")
	fixtureGit(t, dir, "sparse-checkout", "set", "--cone", "--sparse-index", "included")
	result, err := repo.ApplyStash(t.Context(), fixtureStashEntry(t, repo), readFixtureStatus(t, repo))
	if err != nil || len(result.Conflicts) != 0 || result.Warning != nil {
		t.Fatalf("sparse apply: %+v, %v", result, err)
	}
	for name, expected := range map[string]string{"included/tracked": "included edit\n", "excluded/tracked": "excluded edit\n"} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(data) != expected {
			t.Fatalf("%s: %q, %v", name, data, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "excluded", "untouched")); !os.IsNotExist(err) {
		t.Fatalf("sparse apply materialized an unrelated file: %v", err)
	}
}

func TestApplyStashReportsUntrackedCollisionAlongsideConflict(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	writeFixture(t, dir, "conflict.txt", "stash edit\n")
	writeFixture(t, dir, "new.txt", "stash untracked\n")
	fixtureGit(t, dir, "stash", "push", "-u", "-m", "two issues")
	entry := fixtureStashEntry(t, repo)
	writeFixture(t, dir, "conflict.txt", "branch edit\n")
	fixtureGit(t, dir, "commit", "-am", "new base")
	writeFixture(t, dir, "new.txt", "independent edit\n")
	result, err := repo.ApplyStash(t.Context(), entry, readFixtureStatus(t, repo))
	if err != nil || !slices.Equal(result.Conflicts, []string{"conflict.txt"}) ||
		result.Warning == nil || !strings.Contains(result.Warning.Error(), "could not restore untracked files") {
		t.Fatalf("mixed failure should report both issues: %+v, %v", result, err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "new.txt")); err != nil || string(data) != "independent edit\n" {
		t.Fatalf("independent file changed: %q, %v", data, err)
	}
	if got := fixtureStashEntry(t, repo); !got.SameSelection(entry) {
		t.Fatalf("stash was changed: %+v", got)
	}
}

func TestApplyStashRechecksReflogBeforeWritingFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a shell wrapper")
	}
	dir := newFixture(t)
	if err := os.MkdirAll(filepath.Join(dir, "new", "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "new/sub/file.txt", "old stash contents\n")
	fixtureGit(t, dir, "stash", "push", "-u", "-m", "old entry")
	writeFixture(t, dir, "mixed.txt", "new stash contents\n")
	fixtureGit(t, dir, "stash", "push", "-m", "new entry")
	repo := openFixture(t, dir)
	entries, err := repo.Stashes(t.Context())
	if err != nil || len(entries) != 2 {
		t.Fatalf("setup: %+v, %v", entries, err)
	}
	t.Setenv("GITWAVE_STASH_REAL_GIT", repo.git)
	t.Setenv("GITWAVE_STASH_REPO", dir)
	wrapper := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(wrapper, []byte(`#!/bin/sh
for arg do
  if [ "$arg" = update-ref ]; then
    "$GITWAVE_STASH_REAL_GIT" -C "$GITWAVE_STASH_REPO" stash drop 'stash@{1}' > /dev/null || exit "$?"
    break
  fi
done
exec "$GITWAVE_STASH_REAL_GIT" "$@"
`), 0o700); err != nil {
		t.Fatal(err)
	}
	shown := readFixtureStatus(t, repo)
	repo.git = wrapper
	if _, err := repo.ApplyStash(t.Context(), entries[1], shown); err == nil || !strings.Contains(err.Error(), "stash list changed") {
		t.Fatalf("stale stash survived final verification: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "new")); !os.IsNotExist(err) {
		t.Fatalf("rejected apply left directories in the worktree: %v", err)
	}
	if got := fixtureStashEntry(t, repo); !got.sameRecord(entries[0]) {
		t.Fatalf("newer stash was changed: %+v", got)
	}
}
