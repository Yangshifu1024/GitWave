package gitcli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSetStagedBatchesPathsWithoutReplacingOtherIndexChanges(t *testing.T) {
	dir := newFixture(t)
	const literal = "[mixed].txt"
	writeFixture(t, dir, literal, "literal\n")
	writeFixture(t, dir, "m.txt", "unrelated glob match\n")
	writeFixture(t, dir, "mixed.txt", "stage one\n")
	fixtureGit(t, dir, "add", "mixed.txt")
	writeFixture(t, dir, "mixed.txt", "stage two\n")
	writeFixture(t, dir, "conflict.txt", "other staged change\n")
	fixtureGit(t, dir, "add", "conflict.txt")
	fixtureGit(t, dir, "update-index", "--split-index")
	if err := os.Remove(filepath.Join(dir, "old name.txt")); err != nil {
		t.Fatal(err)
	}
	repo := openFixture(t, dir)
	before := readFixtureStatus(t, repo)
	paths := []string{literal, "mixed.txt", "old name.txt"}
	if err := repo.SetStaged(t.Context(), before, fixtureChanges(t, before, paths...), true); err != nil {
		t.Fatal(err)
	}
	if got := fixtureGit(t, dir, "show", ":mixed.txt"); got != "stage two\n" {
		t.Fatalf("index contents: %q", got)
	}
	names := strings.Fields(fixtureGit(t, dir, "diff", "--cached", "--name-only", "--no-renames"))
	if slices.Contains(names, "m.txt") || !slices.Contains(names, literal) {
		t.Fatalf("nonliteral path selection: %v", names)
	}
	after := readFixtureStatus(t, repo)
	if err := repo.SetStaged(t.Context(), after, fixtureChanges(t, after, paths...), false); err != nil {
		t.Fatal(err)
	}
	if got := fixtureGit(t, dir, "diff", "--cached", "--name-only"); got != "conflict.txt\n" {
		t.Fatalf("unrelated staged changes were altered: %q", got)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "mixed.txt")); err != nil || string(data) != "stage two\n" {
		t.Fatalf("unstage changed the working file: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "old name.txt")); !os.IsNotExist(err) {
		t.Fatalf("unstage restored the deleted working file: %v", err)
	}
}

func TestUnstageRenameRestoresBothIndexNames(t *testing.T) {
	dir := newFixture(t)
	fixtureGit(t, dir, "mv", "old name.txt", "new name.txt")
	repo := openFixture(t, dir)
	status := readFixtureStatus(t, repo)
	if err := repo.SetStaged(t.Context(), status, fixtureChanges(t, status, "new name.txt"), false); err != nil {
		t.Fatal(err)
	}
	if diff := fixtureGit(t, dir, "diff", "--cached", "--name-only"); diff != "" {
		t.Fatalf("partial rename remained staged: %q", diff)
	}
	if _, err := os.Stat(filepath.Join(dir, "new name.txt")); err != nil {
		t.Fatalf("working file was renamed back: %v", err)
	}
}

func TestUnstageUnbornPreservesWorktree(t *testing.T) {
	dir := t.TempDir()
	fixtureGit(t, dir, "init", "--template=", "-b", "main")
	writeFixture(t, dir, "-new.txt", "unborn\n")
	repo := openFixture(t, dir)
	before := readFixtureStatus(t, repo)
	if err := repo.SetStaged(t.Context(), before, before.Changes, true); err != nil {
		t.Fatal(err)
	}
	staged := readFixtureStatus(t, repo)
	if len(staged.Changes) != 1 || staged.Changes[0].Index != 'A' {
		t.Fatalf("staged in unborn repo: %+v", staged)
	}
	if err := repo.SetStaged(t.Context(), staged, staged.Changes, false); err != nil {
		t.Fatal(err)
	}
	after := readFixtureStatus(t, repo)
	if len(after.Changes) != 1 || after.Changes[0].Kind != '?' {
		t.Fatalf("unstaged in unborn repo: %+v", after)
	}
}

func TestIndexWriteRejectsStaleSelectionAndExistingLock(t *testing.T) {
	dir := newFixture(t)
	writeFixture(t, dir, "mixed.txt", "first stage\n")
	fixtureGit(t, dir, "add", "mixed.txt")
	writeFixture(t, dir, "mixed.txt", "second stage\n")
	repo := openFixture(t, dir)
	old := readFixtureStatus(t, repo)
	fixtureGit(t, dir, "add", "mixed.txt")
	writeFixture(t, dir, "mixed.txt", "uncommitted\n")
	if err := repo.SetStaged(t.Context(), old, old.Changes, false); err == nil || !strings.Contains(err.Error(), "no longer current") {
		t.Fatalf("unstaging a replaced selection: %v", err)
	}
	current := readFixtureStatus(t, repo)
	lock := filepath.Join(repo.GitDir, "index.lock")
	if err := os.WriteFile(lock, []byte("external lock"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetStaged(t.Context(), current, current.Changes, true); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("index lock ignored: %v", err)
	}
	if got, err := os.ReadFile(lock); err != nil || string(got) != "external lock" {
		t.Fatalf("external lock replaced: %q, %v", got, err)
	}
	if got := fixtureGit(t, dir, "show", ":mixed.txt"); got != "second stage\n" {
		t.Fatalf("operation changed the live index: %q", got)
	}
}

func TestIndexWriteRejectsFilterWithoutStagingOtherFiles(t *testing.T) {
	dir := newFixture(t)
	writeFixture(t, dir, ".gitattributes", "mixed.txt filter=fixture\n")
	writeFixture(t, dir, "mixed.txt", "filtered\n")
	writeFixture(t, dir, "unfiltered.txt", "new\n")
	fixtureGit(t, dir, "config", "filter.fixture.clean", "gitwave-nonexistent-clean-filter")
	fixtureGit(t, dir, "config", "filter.fixture.required", "true")
	repo := openFixture(t, dir)
	before := readFixtureStatus(t, repo)
	if err := repo.SetStaged(t.Context(), before, before.Changes, true); err == nil || !strings.Contains(err.Error(), "external Git filter") {
		t.Fatalf("filtered content was accepted: %v", err)
	}
	if got := fixtureGit(t, dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("failed batch partly staged files: %q", got)
	}
	if _, err := os.Stat(filepath.Join(repo.GitDir, "index.lock")); !os.IsNotExist(err) {
		t.Fatalf("app lock remained after error: %v", err)
	}
}

func TestIndexWriteRejectsStaleFileMode(t *testing.T) {
	dir := newFixture(t)
	fixtureGit(t, dir, "config", "core.filemode", "false")
	writeFixture(t, dir, "mixed.txt", "staged content\n")
	fixtureGit(t, dir, "add", "mixed.txt")
	repo := openFixture(t, dir)
	before := readFixtureStatus(t, repo)
	fixtureGit(t, dir, "add", "--chmod=+x", "mixed.txt")
	if err := repo.SetStaged(t.Context(), before, before.Changes, false); err == nil || !strings.Contains(err.Error(), "no longer current") {
		t.Fatalf("mode-only replacement was accepted: %v", err)
	}
	if got := fixtureGit(t, dir, "ls-files", "--stage", "mixed.txt"); !strings.HasPrefix(got, "100755 ") {
		t.Fatalf("external executable bit was lost: %q", got)
	}
}

func TestIndexWriteFreezesGlobalAttributesAndDistinguishesFilterState(t *testing.T) {
	dir := newFixture(t)
	attrs := filepath.Join(t.TempDir(), "attributes")
	setAttrs := func(value string) {
		t.Helper()
		if err := os.WriteFile(attrs, []byte("mixed.txt "+value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fixtureGit(t, dir, "config", "core.attributesfile", attrs)
	fixtureGit(t, dir, "config", "filter.unset.clean", "gitwave-nonexistent-clean-filter")
	writeFixture(t, dir, "mixed.txt", "outside attributes\n")
	setAttrs("filter=unset")
	repo := openFixture(t, dir)
	status := readFixtureStatus(t, repo)
	if err := repo.SetStaged(t.Context(), status, status.Changes, true); err == nil {
		t.Fatal("an external filter named unset was bypassed")
	}
	if got := fixtureGit(t, dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("external filter rejection changed the index: %q", got)
	}
	setAttrs("-filter")
	status = readFixtureStatus(t, repo)
	if err := repo.SetStaged(t.Context(), status, status.Changes, true); err != nil {
		t.Fatalf("an unset filter shouldn't require a driver: %v", err)
	}
	if got := fixtureGit(t, dir, "show", ":mixed.txt"); got != "outside attributes\n" {
		t.Fatalf("unexpected index blob: %q", got)
	}
}

func TestUnstageFileDirectoryCollisionKeepsOtherEntries(t *testing.T) {
	dir := newFixture(t)
	path := filepath.Join(dir, "mixed.txt")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "mixed.txt/new.txt", "unrelated child\n")
	repo := openFixture(t, dir)
	shown := readFixtureStatus(t, repo)
	if err := repo.SetStaged(t.Context(), shown, shown.Changes, true); err != nil {
		t.Fatal(err)
	}
	staged := readFixtureStatus(t, repo)
	if err := repo.SetStaged(t.Context(), staged, fixtureChanges(t, staged, "mixed.txt"), false); err == nil {
		t.Fatal("unstage replaced an unselected child")
	}
	if got := fixtureGit(t, dir, "show", ":mixed.txt/new.txt"); got != "unrelated child\n" {
		t.Fatalf("stage entry was altered after rollback: %q", got)
	}
	if err := repo.SetStaged(t.Context(), staged, staged.Changes, false); err != nil {
		t.Fatal(err)
	}
	if got := fixtureGit(t, dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("unstage all left staged changes: %q", got)
	}
}

func TestIndexWriteStagesDirectoryReplacedWithFile(t *testing.T) {
	dir := newFixture(t)
	parent := filepath.Join(dir, "parent")
	if err := os.Mkdir(parent, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "parent/old.txt", "original contents inside directory\n")
	fixtureGit(t, dir, "add", "parent")
	fixtureGit(t, dir, "commit", "-m", "Directory")
	if err := os.Remove(filepath.Join(parent, "old.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(parent); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "parent", "different replacement\n")
	repo := openFixture(t, dir)
	shown := readFixtureStatus(t, repo)
	if err := repo.SetStaged(t.Context(), shown, shown.Changes, true); err != nil {
		t.Fatal(err)
	}
	if got := fixtureGit(t, dir, "show", ":parent"); got != "different replacement\n" {
		t.Fatalf("replacement wasn't staged: %q", got)
	}
	staged := readFixtureStatus(t, repo)
	if err := repo.SetStaged(t.Context(), staged, staged.Changes, false); err != nil {
		t.Fatal(err)
	}
	if got := fixtureGit(t, dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("changes remained staged: %q", got)
	}
}

func fixtureChanges(t *testing.T, status Status, paths ...string) []Change {
	t.Helper()
	var selected []Change
	for _, path := range paths {
		index := slices.IndexFunc(status.Changes, func(change Change) bool { return change.Path == path })
		if index < 0 {
			t.Fatalf("missing change for %q: %+v", path, status.Changes)
		}
		selected = append(selected, status.Changes[index])
	}
	return selected
}
