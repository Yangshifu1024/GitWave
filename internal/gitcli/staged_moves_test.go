package gitcli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const stagedMoveSource, stagedMoveDestination = "old/item.txt", "new/item.txt"

func newStagedTextMoveFixture(t *testing.T) (string, string) {
	t.Helper()
	dir := newFixture(t)
	for _, folder := range []string{"old", "new", "other"} {
		if err := os.Mkdir(filepath.Join(dir, folder), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	var original, edited strings.Builder
	for i := range 12 {
		line := fmt.Sprintf("Context line with unique content %d", i)
		fmt.Fprintln(&original, "\t"+line)
		fmt.Fprintln(&edited, "    "+line)
	}
	original.WriteString("Committed line\n")
	edited.WriteString("Index only\n")
	writeFixture(t, dir, stagedMoveSource, original.String())
	fixtureGit(t, dir, "add", "--", stagedMoveSource)
	fixtureGit(t, dir, "commit", "-m", "Original text")
	if err := os.Remove(filepath.Join(dir, stagedMoveSource)); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, stagedMoveDestination, edited.String())
	writeFixture(t, dir, "unrelated.txt", "Unrelated staged file\n")
	fixtureGit(t, dir, "add", "-A")
	return dir, edited.String()
}

func TestStagedTextMovePreviewAndUnstageFollowTheIndex(t *testing.T) {
	dir, indexed := newStagedTextMoveFixture(t)
	fixtureGit(t, dir, "update-index", "--split-index")
	repo := openFixture(t, dir)
	status := readFixtureStatus(t, repo)
	var original []Change
	for _, change := range status.Changes {
		if change.Path != "unrelated.txt" {
			original = append(original, change)
		}
	}
	if len(original) != 2 || original[0].Index != 'A' || original[1].Index != 'D' {
		t.Fatalf("Git already paired the fixture: %+v", original)
	}
	moves := repo.StagedMoves(t.Context(), status)
	if len(moves) != 1 || !moves[0].Staged() || moves[0].Source.Path != stagedMoveSource || moves[0].Destination.Path != stagedMoveDestination {
		t.Fatalf("staged text pairs: %+v", moves)
	}
	move := moves[0]
	scan := repo.stagedMoveCache.Load()
	moves[0].Source.Path = "mutated by caller"
	if got := repo.StagedMoves(t.Context(), status); len(got) != 1 || got[0] != move || scan == nil || repo.stagedMoveCache.Load() != scan {
		t.Fatalf("the staged pairing was not cached independently: %+v", got)
	}
	readIndex := func() []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(repo.GitDir, "index"))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	checkPreview := func() {
		t.Helper()
		index, objects := readIndex(), fixtureGit(t, dir, "count-objects")
		preview, err := repo.WorkingMoveDiff(t.Context(), move, false)
		if err != nil || len(preview.Files) != 1 {
			t.Fatalf("preview failed: %+v, %v", preview, err)
		}
		file := preview.Files[0]
		if file.Path != stagedMoveDestination || file.OldSHA != move.Source.HeadOID || file.NewSHA != move.Destination.IndexOID ||
			file.Additions != 13 || file.Deletions != 13 || file.WorkTreePath != "" {
			t.Fatalf("preview used the wrong staged content: %+v", file)
		}
		if !bytes.Equal(index, readIndex()) || fixtureGit(t, dir, "count-objects") != objects {
			t.Fatal("staged preview changed the live index or object store")
		}
	}

	writeFixture(t, dir, stagedMoveDestination, "Worktree edit only\n")
	writeFixture(t, dir, stagedMoveSource, "Recreated source\n")
	if got := repo.StagedMoves(t.Context(), readFixtureStatus(t, repo)); len(got) != 1 || got[0].Destination.WorkTree != 'M' {
		t.Fatalf("working edit interfered with staged pairing: %+v", got)
	}
	checkPreview()
	fixtureGit(t, dir, "reset", "-q", "HEAD", "--", stagedMoveSource)
	if _, err := repo.WorkingMoveDiff(t.Context(), move, false); err == nil {
		t.Fatal("preview accepted a staged move after its source returned to the index")
	}
	fixtureGit(t, dir, "update-index", "--force-remove", "--", stagedMoveSource)
	if err := os.Remove(filepath.Join(dir, stagedMoveDestination)); err != nil {
		t.Fatal(err)
	}
	if got := repo.StagedMoves(t.Context(), readFixtureStatus(t, repo)); len(got) != 1 || got[0].Destination.WorkTree != 'D' {
		t.Fatalf("working deletion interfered with staged pairing: %+v", got)
	}
	checkPreview()

	// A different blob in the index makes the old selection stale. The
	// unrelated file must remain staged even when selected in the same click.
	writeFixture(t, dir, stagedMoveDestination, "Externally staged replacement\n")
	fixtureGit(t, dir, "add", "--", stagedMoveDestination)
	before := readIndex()
	if _, err := repo.WorkingMoveDiff(t.Context(), move, false); err == nil {
		t.Fatal("preview presented a replaced index blob as the old pair")
	}
	if err := repo.SetStaged(t.Context(), status, fixtureChanges(t, status, "unrelated.txt"), false, move); err == nil || !bytes.Equal(before, readIndex()) {
		t.Fatalf("unstaging a stale pair succeeded or partially updated the index: %v", err)
	}
	writeFixture(t, dir, stagedMoveDestination, indexed)
	fixtureGit(t, dir, "add", "--", stagedMoveDestination)
	if err := os.Remove(filepath.Join(dir, stagedMoveDestination)); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetStaged(t.Context(), status, nil, false, move); err != nil {
		t.Fatal(err)
	}
	if got := fixtureGit(t, dir, "diff", "--cached", "--name-only"); got != "unrelated.txt\n" {
		t.Fatalf("unstage changed other staged paths: %q", got)
	}
	if data, err := os.ReadFile(filepath.Join(dir, stagedMoveSource)); err != nil || string(data) != "Recreated source\n" {
		t.Fatalf("unstage rewrote the recreated source: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dir, stagedMoveDestination)); !os.IsNotExist(err) {
		t.Fatalf("unstage restored a worktree file: %v", err)
	}
}

func TestStagedTextMoveLeavesCompetingTargetsUnpaired(t *testing.T) {
	for _, test := range []struct{ path, suffix string }{
		{"other/item.txt", "Competing file with different contents\n"},
		{"other/copy.txt", ""},
	} {
		t.Run(test.path, func(t *testing.T) {
			dir, indexed := newStagedTextMoveFixture(t)
			writeFixture(t, dir, test.path, indexed+test.suffix)
			fixtureGit(t, dir, "add", "--", test.path)
			repo := openFixture(t, dir)
			status := readFixtureStatus(t, repo)
			if moves := repo.StagedMoves(t.Context(), status); len(moves) != 0 {
				t.Fatalf("ambiguous staged paths were paired: %+v", moves)
			}
		})
	}
}
