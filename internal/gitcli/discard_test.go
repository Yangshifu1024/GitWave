package gitcli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDiscardRestoresIndexAndRemovesOnlySelectedUntrackedFiles(t *testing.T) {
	dir := newFixture(t)
	if err := os.Mkdir(filepath.Join(dir, "missing"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, ".gitattributes", "mixed.txt text eol=crlf\n")
	writeFixture(t, dir, "missing/[file].txt", "recreate parent\n")
	fixtureGit(t, dir, "add", ".")
	fixtureGit(t, dir, "commit", "-m", "Track attributes and nested file")
	writeFixture(t, dir, "mixed.txt", "staged version\n")
	fixtureGit(t, dir, "add", "mixed.txt")
	writeFixture(t, dir, "mixed.txt", "unstaged version\n")
	writeFixture(t, dir, "untracked.txt", "discard\n")
	writeFixture(t, dir, "keep.txt", "keep\n")
	if err := os.Remove(filepath.Join(dir, "missing", "[file].txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "missing")); err != nil {
		t.Fatal(err)
	}
	repo := openFixture(t, dir)
	state := readFixtureStatus(t, repo)
	head := state.HeadOID
	index := readDiscardFixture(t, repo.GitDir, "index")
	plan, err := repo.PrepareDiscard(t.Context(), state,
		fixtureChanges(t, state, "mixed.txt", "untracked.txt", "missing/[file].txt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		"mixed.txt": "staged version\r\n", "keep.txt": "keep\n", "missing/[file].txt": "recreate parent\n",
	} {
		if got := string(readDiscardFixture(t, dir, path)); got != want {
			t.Fatalf("%s: got %q, want %q", path, got, want)
		}
	}
	if _, err := os.Lstat(filepath.Join(dir, "untracked.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("selected untracked file remains: %v", err)
	}
	if after := readFixtureStatus(t, repo); after.HeadOID != head || !bytes.Equal(index, readDiscardFixture(t, repo.GitDir, "index")) {
		t.Fatalf("discard modified HEAD or the original index: %+v", after)
	}
}

func TestDiscardRejectsAnyChangedFileBeforeApplyingSelection(t *testing.T) {
	dir := newFixture(t)
	writeFixture(t, dir, "mixed.txt", "working\n")
	writeFixture(t, dir, "untracked.txt", "before\n")
	repo := openFixture(t, dir)
	state := readFixtureStatus(t, repo)
	plan, err := repo.PrepareDiscard(t.Context(), state, fixtureChanges(t, state, "mixed.txt", "untracked.txt"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "untracked.txt")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "untracked.txt", "latest\n")
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(t.Context()); err == nil || !strings.Contains(err.Error(), "working file changed") {
		t.Fatalf("discard accepted newer contents: %v", err)
	}
	if got := string(readDiscardFixture(t, dir, "mixed.txt")); got != "working\n" {
		t.Fatalf("stale batch discarded another file: %q", got)
	}
	if got := string(readDiscardFixture(t, dir, "untracked.txt")); got != "latest\n" {
		t.Fatalf("newer untracked version lost: %q", got)
	}
}

func TestDiscardRestoresAndDeletesSymlinksWithoutTouchingTargets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks on Windows requires Developer Mode or elevated privileges")
	}
	dir := newFixture(t)
	outside := t.TempDir()
	writeFixture(t, outside, "untouched", "original\n")
	target := filepath.Join(outside, "untouched")
	link := filepath.Join(dir, "tracked-link")
	if err := os.Symlink("mixed.txt", link); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "add", "tracked-link")
	fixtureGit(t, dir, "commit", "-m", "Track link")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{link, filepath.Join(dir, "untracked-link")} {
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
	}
	repo := openFixture(t, dir)
	state := readFixtureStatus(t, repo)
	plan, err := repo.PrepareDiscard(t.Context(), state, fixtureChanges(t, state, "tracked-link", "untracked-link"))
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(link); got != "mixed.txt" || err != nil {
		t.Fatalf("restored link: %q, %v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "untracked-link")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("untracked link remains: %v", err)
	}
	if got := string(readDiscardFixture(t, outside, "untouched")); got != "original\n" {
		t.Fatalf("discard modified a symlink target: %q", got)
	}
}

func TestDiscardPreservesTrackedFileReportedAsUntrackedAlias(t *testing.T) {
	dir := newFixture(t)
	fixtureGit(t, dir, "config", "core.ignorecase", "false")
	writeFixture(t, dir, "Upper.txt", "tracked\n")
	fixtureGit(t, dir, "add", "Upper.txt")
	fixtureGit(t, dir, "commit", "-m", "Track upper case")
	from, to := filepath.Join(dir, "Upper.txt"), filepath.Join(dir, "upper.txt")
	temporary := filepath.Join(dir, "intermediate.txt")
	if err := os.Rename(from, temporary); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporary, to); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(from); errors.Is(err, os.ErrNotExist) {
		t.Skip("this worktree distinguishes names by case")
	} else if err != nil {
		t.Fatal(err)
	}
	repo := openFixture(t, dir)
	state := readFixtureStatus(t, repo)
	plan, err := repo.PrepareDiscard(t.Context(), state, fixtureChanges(t, state, "upper.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(t.Context()); err == nil || !strings.Contains(err.Error(), "refers to tracked file") {
		t.Fatalf("discard accepted an untracked alias of a tracked file: %v", err)
	}
	if got := string(readDiscardFixture(t, dir, "upper.txt")); got != "tracked\n" {
		t.Fatalf("the tracked file changed: %q", got)
	}
}

func TestDiscardFileMovePreservesBothFilesOnNameCollision(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "before.txt", "confirmed copy\n")
	writeFixture(t, dir, "after.txt", "external edit\n")
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := moveDiscardFile(root, "before.txt", root, "after.txt"); err == nil {
		t.Fatal("the move replaced a file created by another editor")
	}
	if got := string(readDiscardFixture(t, dir, "before.txt")); got != "confirmed copy\n" {
		t.Fatalf("the old copy was lost: %q", got)
	}
	if got := string(readDiscardFixture(t, dir, "after.txt")); got != "external edit\n" {
		t.Fatalf("the new copy was lost: %q", got)
	}
}

func readDiscardFixture(t *testing.T, dir, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return data
}
