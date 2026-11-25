package gitcli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openFixtureConflict(t *testing.T, repo *Repository, local string) *Conflict {
	t.Helper()
	status := readFixtureStatus(t, repo)
	for _, change := range status.Changes {
		if change.Kind == 'u' && change.Path == local {
			conflict, err := repo.OpenConflict(t.Context(), status, change)
			if err != nil {
				t.Fatal(err)
			}
			return conflict
		}
	}
	t.Fatalf("no unresolved path %q: %+v", local, status.Changes)
	return nil
}

func openTextConflict(t *testing.T) (string, *Repository, *Conflict) {
	t.Helper()
	dir, repo, source, target := mergeFixture(t, true, true)
	plan, err := repo.PrepareMerge(t.Context(), source, target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	return dir, repo, openFixtureConflict(t, repo, "conflict.txt")
}

func TestConflictEditorLoadsAllSidesAndSavesText(t *testing.T) {
	dir, _, conflict := openTextConflict(t)
	if !conflict.CanEdit() || conflict.Base.Text != "common base\n" || conflict.Ours.Text != "ours\n" || conflict.Theirs.Text != "theirs\n" ||
		!strings.Contains(conflict.Working.Text, "<<<<<<<") {
		t.Fatalf("incorrect conflict snapshot: %+v", conflict)
	}
	writeFixture(t, dir, "mixed.txt", "unrelated staged content\n")
	fixtureGit(t, dir, "add", "mixed.txt")
	const resolved = "保留双方修改\n"
	if err := conflict.Resolve(t.Context(), ConflictText, resolved); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "conflict.txt")); err != nil || string(got) != resolved {
		t.Fatalf("wrong working file: %q, %v", got, err)
	}
	if got := gitValue(t, dir, "show", ":conflict.txt"); got != strings.TrimSpace(resolved) {
		t.Fatalf("wrong staged text: %q", got)
	}
	if got := gitValue(t, dir, "show", ":mixed.txt"); got != "unrelated staged content" {
		t.Fatalf("resolution changed another index entry: %s", got)
	}
}

func TestConflictEditorRejectsChangedWorkingFile(t *testing.T) {
	dir, _, conflict := openTextConflict(t)
	writeFixture(t, dir, "conflict.txt", strings.Repeat("x", len(conflict.Working.Text)))
	if err := os.Chtimes(filepath.Join(dir, "conflict.txt"), conflict.stamp.info.ModTime(), conflict.stamp.info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := conflict.Resolve(t.Context(), ConflictOurs, ""); err == nil || !strings.Contains(err.Error(), "changed on disk") {
		t.Fatalf("resolution accepted a stale working file: %v", err)
	}
	if got := gitValue(t, dir, "status", "--porcelain"); got != "UU conflict.txt" {
		t.Fatalf("stale save changed conflict stages: %s", got)
	}
}

func TestConflictEditorUsingMissingSideStagesDeletion(t *testing.T) {
	dir := newFixture(t)
	fixtureGit(t, dir, "checkout", "-b", "feature")
	fixtureGit(t, dir, "rm", "conflict.txt")
	fixtureGit(t, dir, "commit", "-m", "Delete file on feature")
	fixtureGit(t, dir, "checkout", "main")
	writeFixture(t, dir, "conflict.txt", "ours\n")
	fixtureGit(t, dir, "commit", "-am", "Edit file on main")
	fixtureConflict(t, dir, "merge", "feature")
	conflict := openFixtureConflict(t, openFixture(t, dir), "conflict.txt")
	if conflict.Theirs.Kind != "missing" {
		t.Fatalf("deletion preview was not represented: %+v", conflict.Theirs)
	}
	if err := conflict.Resolve(t.Context(), ConflictTheirs, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "conflict.txt")); !os.IsNotExist(err) {
		t.Fatalf("an empty replacement was created: %v", err)
	}
	if got := gitValue(t, dir, "status", "--porcelain"); got != "D  conflict.txt" {
		t.Fatalf("deletion was not staged: %s", got)
	}
}

func TestConflictPublicationFailureRestoresWorkingFile(t *testing.T) {
	dir, _, conflict := openTextConflict(t)
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	cause := errors.New("forced index publication failure")
	err = conflict.publishFile(context.Background(), root, "conflict.txt", func(temp *os.Root, name string) error {
		return temp.WriteFile(name, []byte("resolved\n"), 0o600)
	}, func(os.FileInfo, discardStamp) error { return cause })
	if !errors.Is(err, cause) {
		t.Fatalf("index failure lost: %v", err)
	}
	if actual, err := root.ReadFile("conflict.txt"); err != nil || string(actual) != conflict.Working.Text {
		t.Fatalf("working file was not restored: %q, %v", actual, err)
	}
	if got := gitValue(t, dir, "status", "--porcelain"); got != "UU conflict.txt" {
		t.Fatalf("rollback left unexpected changes: %s", got)
	}
}

func TestConflictPublicationKeepsConcurrentEditsToOriginal(t *testing.T) {
	dir, _, conflict := openTextConflict(t)
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	var saved string
	err = conflict.publishFile(t.Context(), root, "conflict.txt", func(temp *os.Root, name string) error {
		return temp.WriteFile(name, []byte("resolved\n"), 0o600)
	}, func(os.FileInfo, discardStamp) error {
		paths, err := filepath.Glob(filepath.Join(dir, ".gitwave-resolution-*", "original"))
		if err != nil || len(paths) != 1 {
			t.Fatalf("missing protected original: %v, %v", paths, err)
		}
		saved = paths[0]
		return os.WriteFile(saved, []byte("later external edit\n"), 0o600)
	})
	if err == nil || !strings.Contains(err.Error(), "preserved at") {
		t.Fatalf("expected recovery notice: %v", err)
	}
	if actual, err := os.ReadFile(saved); err != nil || string(actual) != "later external edit\n" {
		t.Fatalf("lost the concurrent original edit: %q, %v", actual, err)
	}
	if actual, err := root.ReadFile("conflict.txt"); err != nil || string(actual) != "resolved\n" {
		t.Fatalf("lost the staged resolution: %q, %v", actual, err)
	}
}

func TestConflictEditorRecognizesConfiguredMarkerSize(t *testing.T) {
	dir := newFixture(t)
	writeFixture(t, dir, ".gitattributes", "conflict.txt conflict-marker-size=12\n")
	fixtureGit(t, dir, "add", ".gitattributes")
	fixtureGit(t, dir, "commit", "-m", "Configure wide conflict markers")
	fixtureGit(t, dir, "checkout", "-b", "feature")
	writeFixture(t, dir, "conflict.txt", "theirs\n")
	fixtureGit(t, dir, "commit", "-am", "Feature edit")
	fixtureGit(t, dir, "checkout", "main")
	writeFixture(t, dir, "conflict.txt", "ours\n")
	fixtureGit(t, dir, "commit", "-am", "Main edit")
	fixtureConflict(t, dir, "merge", "feature")
	conflict := openFixtureConflict(t, openFixture(t, dir), "conflict.txt")
	if conflict.MarkerSize != 12 || len(ConflictRegions(conflict.Working.Text, conflict.MarkerSize)) != 1 ||
		len(ConflictRegions("<<<<<<<<<< literal text\n>>>>>>>>> literal text\n", conflict.MarkerSize)) != 0 {
		t.Fatalf("incorrect marker detection for size %d: %q", conflict.MarkerSize, conflict.Working.Text)
	}
	writeFixture(t, dir, ".gitattributes", "conflict.txt conflict-marker-size=15\n")
	conflict = openFixtureConflict(t, openFixture(t, dir), "conflict.txt")
	if conflict.MarkerSize != 15 || len(ConflictRegions(conflict.Working.Text, conflict.MarkerSize)) != 1 {
		t.Fatal("an attribute change hid conflict markers already present in the worktree")
	}
}

func TestConflictEditorKeepsBinarySideBytes(t *testing.T) {
	dir := newFixture(t)
	writeFixture(t, dir, "conflict.txt", "\x00base\xff")
	fixtureGit(t, dir, "commit", "-am", "Binary base")
	fixtureGit(t, dir, "checkout", "-b", "feature")
	const payload = "\x00theirs\xff\r\n"
	writeFixture(t, dir, "conflict.txt", payload)
	fixtureGit(t, dir, "commit", "-am", "Binary change on feature")
	fixtureGit(t, dir, "checkout", "main")
	writeFixture(t, dir, "conflict.txt", "\x00ours\x80")
	fixtureGit(t, dir, "commit", "-am", "Binary change on main")
	fixtureConflict(t, dir, "merge", "feature")
	conflict := openFixtureConflict(t, openFixture(t, dir), "conflict.txt")
	if conflict.CanEdit() || conflict.Theirs.Kind != "binary" {
		t.Fatalf("binary file was treated as text: %+v", conflict)
	}
	if err := conflict.Resolve(t.Context(), ConflictTheirs, "unused"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "conflict.txt")); err != nil || string(data) != payload {
		t.Fatalf("selected file bytes changed: %q, %v", data, err)
	}
	if data := fixtureGit(t, dir, "show", ":conflict.txt"); data != payload {
		t.Fatalf("staged binary bytes changed: %q", data)
	}
}

func TestConflictEditorPreservesIndexExecutableModeWhenWorktreeModeIsIgnored(t *testing.T) {
	dir := newFixture(t)
	fixtureGit(t, dir, "config", "core.filemode", "false")
	fixtureGit(t, dir, "update-index", "--chmod=+x", "conflict.txt")
	fixtureGit(t, dir, "commit", "-m", "Track an executable file")
	fixtureGit(t, dir, "checkout", "-b", "feature")
	writeFixture(t, dir, "conflict.txt", "feature text\n")
	fixtureGit(t, dir, "commit", "-am", "Feature edit")
	fixtureGit(t, dir, "checkout", "main")
	writeFixture(t, dir, "conflict.txt", "main text\n")
	fixtureGit(t, dir, "commit", "-am", "Main edit")
	fixtureConflict(t, dir, "merge", "feature")
	if err := os.Chmod(filepath.Join(dir, "conflict.txt"), 0o600); err != nil {
		t.Fatal(err)
	}
	conflict := openFixtureConflict(t, openFixture(t, dir), "conflict.txt")
	if err := conflict.Resolve(t.Context(), ConflictText, "resolved\n"); err != nil {
		t.Fatal(err)
	}
	if got := gitValue(t, dir, "ls-files", "--stage", "conflict.txt"); !strings.HasPrefix(got, "100755 ") {
		t.Fatalf("the resolution stripped executable mode: %s", got)
	}
}
