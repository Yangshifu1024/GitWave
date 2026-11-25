package gitcli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gitwave.work/internal/process"
)

func TestWorkingMovesPreviewAndStageFromOneSelection(t *testing.T) {
	dir := newFixture(t)
	const source, destination = "old name.txt", "new dir/new [name].txt"
	var baseline strings.Builder
	for i := range 40 {
		fmt.Fprintf(&baseline, "Unique unchanged context at line %d\n", i)
	}
	writeFixture(t, dir, source, baseline.String())
	fixtureGit(t, dir, "add", "--", source)
	fixtureGit(t, dir, "commit", "-m", "Original file")
	writeFixture(t, dir, "mixed.txt", "Leave this staged\n")
	fixtureGit(t, dir, "add", "--", "mixed.txt")
	modified := strings.Replace(baseline.String(), "Unique unchanged context at line 12", "Edit after the move", 1)
	if err := os.Remove(filepath.Join(dir, source)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "new dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, destination, modified)
	writeFixture(t, dir, "new name.txt", "Unrelated untracked file\n")
	repo := openFixture(t, dir)
	status := readFixtureStatus(t, repo)
	indexPath := filepath.Join(repo.GitDir, "index")
	readIndex := func() []byte {
		t.Helper()
		data, err := os.ReadFile(indexPath)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	wantIndex := readIndex()
	wantObjects := fixtureGit(t, dir, "count-objects")
	moves := repo.WorkingMoves(t.Context(), status)
	if len(moves) != 1 || moves[0].Source.Path != source || moves[0].Destination.Path != destination {
		t.Fatalf("working moves: %+v from %+v", moves, status.Changes)
	}
	preview, err := repo.WorkingMoveDiff(t.Context(), moves[0], false)
	if err != nil || len(preview.Files) != 1 {
		t.Fatalf("move preview: %+v, %v", preview, err)
	}
	file := preview.Files[0]
	if file.Additions != 1 || file.Deletions != 1 || file.Path != destination || file.OldSHA != moves[0].Source.IndexOID {
		t.Fatalf("move preview: %+v", file)
	}
	if got := fixtureGit(t, dir, "count-objects"); got != wantObjects || !bytes.Equal(readIndex(), wantIndex) {
		t.Fatalf("read-only move changed index or object store: %q -> %q", wantObjects, got)
	}
	writeFixture(t, dir, destination, "Replaced outside the app\n")
	if err := repo.SetStaged(t.Context(), status, nil, true, moves...); err == nil || !strings.Contains(err.Error(), "moved working file has changed") || !bytes.Equal(readIndex(), wantIndex) {
		t.Fatalf("staging a stale move modified the index or returned an unexpected error: %v", err)
	}
	writeFixture(t, dir, destination, modified)
	writeFixture(t, dir, source, baseline.String())
	if err := repo.SetStaged(t.Context(), status, nil, true, moves...); err == nil || !bytes.Equal(readIndex(), wantIndex) {
		t.Fatalf("staging with a restored source modified the index or succeeded: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, source)); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetStaged(t.Context(), status, nil, true, moves...); err != nil {
		t.Fatal(err)
	}
	staged := readFixtureStatus(t, repo)
	pair := fixtureChanges(t, staged, destination)
	if pair[0].Index != 'R' || pair[0].PreviousPath != source || fixtureGit(t, dir, "show", ":"+destination) != modified ||
		fixtureGit(t, dir, "show", ":mixed.txt") != "Leave this staged\n" {
		t.Fatalf("staging move produced incorrect index: %+v", staged.Changes)
	}
	if err := repo.SetStaged(t.Context(), staged, pair, false); err != nil {
		t.Fatal(err)
	}
	if found := repo.WorkingMoves(t.Context(), readFixtureStatus(t, repo)); len(found) != 1 || found[0] != moves[0] {
		t.Fatalf("unstaged move or worktree differs: %+v", found)
	}
	if got := fixtureGit(t, dir, "diff", "--cached", "--name-only"); got != "mixed.txt\n" {
		t.Fatalf("unstaging rewrote other staged files: %q", got)
	}
}

func TestWorkingMovesLeavesAmbiguousCopiesUnpaired(t *testing.T) {
	dir := newFixture(t)
	writeFixture(t, dir, ".gitattributes", "*.txt text eol=lf\n")
	fixtureGit(t, dir, "add", ".gitattributes")
	fixtureGit(t, dir, "commit", "-m", "Normalize text")
	contents, err := os.ReadFile(filepath.Join(dir, "old name.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "old name.txt")); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "first candidate.txt", string(contents))
	writeFixture(t, dir, "second candidate.txt", strings.ReplaceAll(string(contents), "\n", "\r\n"))
	repo := openFixture(t, dir)
	status := readFixtureStatus(t, repo)
	if moves := repo.WorkingMoves(t.Context(), status); len(status.Changes) != 3 || len(moves) != 0 {
		t.Fatalf("ambiguous copies were paired: %+v from %+v", moves, status.Changes)
	}
}

func TestWorkingMovesFindsTextMatchBelowGitRenameThreshold(t *testing.T) {
	for _, test := range []struct {
		name, extra string
		shared      bool
	}{
		{name: "unique"},
		{name: "two destinations", extra: "other/source.txt"},
		{name: "repeated line only", shared: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := newFixture(t)
			const source, destination = "old/source.txt", "new/source.txt"
			for _, folder := range []string{"old", "new", "other"} {
				if err := os.Mkdir(filepath.Join(dir, folder), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			var original, edited strings.Builder
			for n := range 12 {
				line := fmt.Sprintf("A common line with unique content %d", n)
				if test.shared {
					line = "A common line repeated throughout the file"
				}
				fmt.Fprintln(&original, "\t"+line)
				fmt.Fprintln(&edited, "    "+line)
			}
			original.WriteString("Original final line\n")
			edited.WriteString("Updated final line\n")
			writeFixture(t, dir, source, original.String())
			fixtureGit(t, dir, "add", "--", source)
			fixtureGit(t, dir, "commit", "-m", "Original text")
			if err := os.Remove(filepath.Join(dir, source)); err != nil {
				t.Fatal(err)
			}
			writeFixture(t, dir, destination, edited.String())
			if test.extra != "" {
				writeFixture(t, dir, test.extra, edited.String()+"Other addition\n")
			}
			fixtureGit(t, dir, "add", "-N", "--", destination)
			if diff := fixtureGit(t, dir, "diff", "--find-renames=50%", "--diff-filter=R", "--name-only"); diff != "" {
				t.Fatalf("fixture unexpectedly passed Git's rename threshold: %q", diff)
			}
			fixtureGit(t, dir, "reset", "-q", "HEAD", "--", destination)
			repo := openFixture(t, dir)
			status := readFixtureStatus(t, repo)
			moves := repo.WorkingMoves(t.Context(), status)
			if test.extra != "" || test.shared {
				if len(moves) != 0 {
					t.Fatalf("inconclusive text was paired: %+v", moves)
				}
				return
			}
			if len(moves) != 1 || !moves[0].matchedText || moves[0].Source.Path != source || moves[0].Destination.Path != destination {
				t.Fatalf("expected paired text paths: %+v", moves)
			}
			const largeCandidate = "other/source.txt"
			writeFixture(t, dir, largeCandidate, strings.Repeat(original.String(), 8))
			if got := repo.WorkingMoves(t.Context(), readFixtureStatus(t, repo)); len(got) != 0 {
				t.Fatalf("a new same-name candidate outside the size range must invalidate the pairing: %+v", got)
			}
			if err := os.Remove(filepath.Join(dir, largeCandidate)); err != nil {
				t.Fatal(err)
			}
			if got := repo.WorkingMoves(t.Context(), readFixtureStatus(t, repo)); len(got) != 1 || got[0] != moves[0] {
				t.Fatalf("removing the competing path did not restore the unique pairing: %+v", got)
			}
			preview, err := repo.WorkingMoveDiff(t.Context(), moves[0], false)
			if err != nil || len(preview.Files) != 1 || preview.Files[0].Path != destination || preview.Files[0].OldSHA != moves[0].Source.IndexOID ||
				preview.Files[0].Additions != 13 || preview.Files[0].Deletions != 13 {
				t.Fatalf("preview did not compare the paired blobs: %+v, %v", preview, err)
			}
			if err := repo.SetStaged(t.Context(), status, nil, true, moves...); err != nil {
				t.Fatal(err)
			}
			if got := fixtureGit(t, dir, "diff", "--cached", "--no-renames", "--name-status"); got != "A\t"+destination+"\nD\t"+source+"\n" {
				t.Fatalf("staging did not write both paired paths: %q", got)
			}
			staged := readFixtureStatus(t, repo)
			if err := repo.SetStaged(t.Context(), staged, fixtureChanges(t, staged, source, destination), false); err != nil {
				t.Fatal(err)
			}
			if got := repo.WorkingMoves(t.Context(), readFixtureStatus(t, repo)); len(got) != 1 || got[0] != moves[0] {
				t.Fatalf("unstaging did not restore the same pairing: %+v", got)
			}
		})
	}
}

func TestWorkingMovesCancelsHashLauncherChildrenWhenDiffIsEmpty(t *testing.T) {
	if !process.SupportsGroupCancellation {
		t.Skip("parallel comparison requires process group cancellation")
	}
	dir := t.TempDir()
	launcher := filepath.Join(dir, "git")
	const script = `#!/bin/sh
set -eu
cd "$2"
case " $* " in
  *" hash-object "*)
    (
      : > hash-started
      /bin/sleep 1
      : > hash-finished
    ) &
    wait
    ;;
  *" diff "*)
    while [ ! -f hash-started ]; do /bin/sleep 0.01; done
    ;;
  *) exit 1 ;;
esac
`
	if err := os.WriteFile(launcher, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	git := &workingGit{
		repo: &Repository{git: launcher, CommonDir: dir}, dir: dir, workTree: dir,
		index: filepath.Join(dir, "index"), groupCancellation: true, frozenAttributes: true,
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if diff, _, err := git.compareWorkingMoves(ctx, []string{"candidate.txt"}, nil); err != nil || len(diff) != 0 {
		t.Fatalf("empty comparison produced %q, %v", diff, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "hash-finished")); !os.IsNotExist(err) {
		t.Fatalf("the launcher child was not canceled while holding the output pipe: %v", err)
	}
}

func TestWorkingMovesRejectsChangedConversion(t *testing.T) {
	dir := newFixture(t)
	const source, destination = "old name.txt", "new name.txt"
	writeFixture(t, dir, ".gitattributes", "*.txt text eol=lf\n")
	fixtureGit(t, dir, "add", ".gitattributes")
	fixtureGit(t, dir, "commit", "-m", "Normalize text")
	contents, err := os.ReadFile(filepath.Join(dir, source))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, source)); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, destination, strings.ReplaceAll(string(contents), "\n", "\r\n"))
	repo := openFixture(t, dir)
	status := readFixtureStatus(t, repo)
	moves := repo.WorkingMoves(t.Context(), status)
	if len(moves) != 1 {
		t.Fatalf("expected the CRLF working file to pair after normalization: %+v", moves)
	}
	writeFixture(t, dir, ".gitattributes", "*.txt -text\n")
	if err := repo.SetStaged(t.Context(), status, nil, true, moves...); err == nil || !strings.Contains(err.Error(), "Git conversion for the moved file has changed") {
		t.Fatalf("staging after a conversion change: %v", err)
	}
	if got := fixtureGit(t, dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("a stale move staged these paths: %q", got)
	}
}

func TestWorkingMovesCacheTracksCandidateContentsAndAttributes(t *testing.T) {
	dir := newFixture(t)
	const source, destination = "old name.txt", "nested/new.txt"
	external := filepath.Join(t.TempDir(), "optional-attributes")
	fixtureGit(t, dir, "config", "core.attributesfile", external)
	fixtureGit(t, dir, "config", "core.autocrlf", "false")
	text := strings.Repeat("A stable line of content\n", 30)
	writeFixture(t, dir, source, text)
	writeFixture(t, dir, ".gitattributes", "*.txt text eol=lf\n")
	fixtureGit(t, dir, "add", "--", source, ".gitattributes")
	fixtureGit(t, dir, "commit", "-m", "Normalize move source")
	if err := os.Remove(filepath.Join(dir, source)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	contents := strings.ReplaceAll(text, "\n", "\r\n")
	writeFixture(t, dir, destination, contents)
	repo := openFixture(t, dir)
	status := readFixtureStatus(t, repo)
	read := func() []WorkingMove { return repo.WorkingMoves(t.Context(), status) }
	original := read()
	if len(original) != 1 || original[0].cleanOID != original[0].Source.IndexOID {
		t.Fatalf("expected one normalized rename: %+v", original)
	}
	first := repo.workingMoveCache.Load()
	if first == nil {
		t.Skip("the installed Git has active system attributes or cannot report their path")
	}
	original[0].Destination.Path = "modified by the caller"
	if moves := read(); len(moves) != 1 || moves[0].Destination.Path != destination || repo.workingMoveCache.Load() != first {
		t.Fatalf("unchanged snapshot was not reused intact: %+v", moves)
	}
	file := filepath.Join(dir, destination)
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, destination, strings.Replace(contents, "stable", "edited", 1))
	if err := os.Chtimes(file, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if moves := read(); len(moves) != 1 || moves[0].cleanOID == first.moves[0].cleanOID || repo.workingMoveCache.Load() == first {
		t.Fatalf("same-size edit with restored modification time reused old contents: %+v", moves)
	}
	writeFixture(t, dir, destination, contents)
	read()
	beforeAttributes := repo.workingMoveCache.Load()
	writeFixture(t, dir, "nested/.gitattributes", "*.txt -text\n")
	if moves := read(); repo.workingMoveCache.Load() == beforeAttributes ||
		len(moves) == 1 && moves[0].cleanOID == first.moves[0].cleanOID {
		t.Fatalf("new directory attributes were not applied: %+v", moves)
	}
	beforeInfo := repo.workingMoveCache.Load()
	if err := os.MkdirAll(filepath.Join(repo.CommonDir, "info"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, repo.CommonDir, "info/attributes", "*.txt text eol=lf\n")
	if moves := read(); len(moves) != 1 || moves[0].cleanOID != first.moves[0].cleanOID || repo.workingMoveCache.Load() == beforeInfo {
		t.Fatalf("new info attributes were not applied: %+v", moves)
	}
	for _, name := range []string{filepath.Join(dir, ".gitattributes"), filepath.Join(dir, "nested", ".gitattributes"),
		filepath.Join(repo.CommonDir, "info", "attributes")} {
		if err := os.Remove(name); err != nil {
			t.Fatal(err)
		}
	}
	if moves := read(); len(moves) == 1 && moves[0].cleanOID == first.moves[0].cleanOID {
		t.Fatalf("removed attributes were still applied: %+v", moves)
	}
	beforeExternal := repo.workingMoveCache.Load()
	writeFixture(t, filepath.Dir(external), filepath.Base(external), "*.txt text eol=lf\n")
	if moves := read(); len(moves) != 1 || moves[0].cleanOID != first.moves[0].cleanOID || repo.workingMoveCache.Load() != beforeExternal {
		t.Fatalf("scanning with an external attributes file should bypass the cache: %+v", moves)
	}
}

func TestWorkingMovesPreservesGlobalAttributesThroughSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("this fixture relies on POSIX traversal of a symlink followed by a parent component")
	}
	for _, kind := range []string{"absolute setting", "relative setting", "XDG default"} {
		t.Run(kind, func(t *testing.T) {
			dir := newFixture(t)
			external := t.TempDir()
			for _, subdir := range []string{"git", "sub"} {
				if err := os.Mkdir(filepath.Join(external, subdir), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			alias := filepath.Join(t.TempDir(), "alias")
			if err := os.Symlink(filepath.Join(external, "sub"), alias); err != nil {
				t.Fatal(err)
			}
			base := filepath.ToSlash(alias)
			if kind == "relative setting" {
				relative, err := filepath.Rel(dir, alias)
				if err != nil {
					t.Fatal(err)
				}
				base = filepath.ToSlash(relative)
			}
			writeFixture(t, external, "git/attributes", "*.txt text eol=lf\n")
			if kind == "XDG default" {
				t.Setenv("XDG_CONFIG_HOME", base+"/..")
			} else {
				fixtureGit(t, dir, "config", "core.attributesfile", base+"/../git/attributes")
			}
			fixtureGit(t, dir, "config", "core.autocrlf", "false")
			const source, destination = "source.txt", "destination.txt"
			text := strings.Repeat("A line that should stay normalized\n", 30)
			writeFixture(t, dir, source, text)
			fixtureGit(t, dir, "add", "--", source)
			fixtureGit(t, dir, "commit", "-m", "Record move source")
			if err := os.Remove(filepath.Join(dir, source)); err != nil {
				t.Fatal(err)
			}
			writeFixture(t, dir, destination, strings.ReplaceAll(text, "\n", "\r\n"))
			repo := openFixture(t, dir)
			status := readFixtureStatus(t, repo)
			moves := repo.WorkingMoves(t.Context(), status)
			if len(moves) != 1 || moves[0].cleanOID != moves[0].Source.IndexOID || repo.workingMoveCache.Load() != nil {
				t.Fatalf("external attributes were bypassed during rename detection: %+v", moves)
			}
			if err := repo.SetStaged(t.Context(), status, nil, true, moves...); err != nil {
				t.Fatal(err)
			}
			if got := fixtureGit(t, dir, "show", ":"+destination); got != text {
				t.Fatal("the staged move did not use Git's global line-ending conversion")
			}
		})
	}
}

func TestWorkingMovesStagesSymlinkWithoutFollowingIt(t *testing.T) {
	dir := newFixture(t)
	const source, destination, link = "old-link", "new-link", "missing-link-target"
	if err := os.Symlink(link, filepath.Join(dir, source)); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("creating this symlink requires Windows developer mode: %v", err)
		}
		t.Fatal(err)
	}
	fixtureGit(t, dir, "config", "core.symlinks", "true")
	fixtureGit(t, dir, "add", "--", source)
	fixtureGit(t, dir, "commit", "-m", "Add symlink")
	if err := os.Rename(filepath.Join(dir, source), filepath.Join(dir, destination)); err != nil {
		t.Fatal(err)
	}
	repo := openFixture(t, dir)
	status := readFixtureStatus(t, repo)
	moves := repo.WorkingMoves(t.Context(), status)
	if len(moves) != 1 {
		t.Fatalf("moved symlink: %+v", moves)
	}
	if err := repo.SetStaged(t.Context(), status, nil, true, moves...); err != nil {
		t.Fatal(err)
	}
	if got := fixtureGit(t, dir, "diff", "--cached", "--name-status", "--find-renames"); got != "R100\told-link\tnew-link\n" {
		t.Fatalf("staging symlink produced %q", got)
	}
	if got := fixtureGit(t, dir, "show", ":"+destination); got != link {
		t.Fatalf("staged symlink content: %q", got)
	}
}
