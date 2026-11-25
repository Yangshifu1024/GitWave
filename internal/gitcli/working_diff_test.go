package gitcli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWorkingGitSnapshotIsolatedFromLaterFilters(t *testing.T) {
	dir := newFixture(t)
	writeFixture(t, dir, ".gitattributes", "mixed.txt filter=race-fixture\n")
	fixtureGit(t, dir, "add", ".")
	fixtureGit(t, dir, "commit", "-m", "Attributes")
	fixtureGit(t, dir, "update-index", "--split-index")
	writeFixture(t, dir, "mixed.txt", "first worktree reading\n")
	repo := openFixture(t, dir)
	status := readFixtureStatus(t, repo)
	if len(status.Changes) != 1 || status.Changes[0].Path != "mixed.txt" {
		t.Fatalf("split-index status: %+v", status.Changes)
	}
	git, err := repo.workingGit(t.Context(), status.HeadOID)
	if err != nil {
		t.Fatal(err)
	}
	defer git.close()
	if _, err := git.snapshotFile(t.Context(), status.Changes[0], previewBytes); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "config", "filter.race-fixture.process", "gitwave-nonexistent-process-filter")
	fixtureGit(t, dir, "config", "filter.race-fixture.required", "true")
	writeFixture(t, dir, "mixed.txt", "replacement after snapshot\n")
	diff, err := git.read(t.Context(), previewBytes, "diff", "--no-ext-diff", "--no-textconv", "--", "mixed.txt")
	if err != nil || !strings.Contains(string(diff), "+first worktree reading") || strings.Contains(string(diff), "replacement") {
		t.Fatalf("private diff: %q, %v", diff, err)
	}
}

func TestWorkingDiffUsesBothSidesAndLiteralPaths(t *testing.T) {
	dir := newFixture(t)
	const path = "file[one].txt"
	writeFixture(t, dir, path, "kept\noriginal\n")
	writeFixture(t, dir, ".gitattributes", "*.txt filter=fixture\n")
	fixtureGit(t, dir, "add", ".")
	fixtureGit(t, dir, "commit", "-m", "Add fixture file")
	writeFixture(t, dir, path, "kept\nstaging!\n")
	fixtureGit(t, dir, "add", "--", path)
	writeFixture(t, dir, path, "kept\nunstaged\n")
	fixtureGit(t, dir, "config", "filter.fixture.clean", "gitwave-nonexistent-clean-filter")
	fixtureGit(t, dir, "config", "filter.fixture.process", "gitwave-nonexistent-process-filter")
	fixtureGit(t, dir, "config", "filter.fixture.required", "true")

	repo := openFixture(t, dir)
	status := readFixtureStatus(t, repo)
	if len(status.Changes) != 1 || status.Changes[0].Path != path ||
		status.Changes[0].Index != 'M' || status.Changes[0].WorkTree != 'M' {
		t.Fatalf("working copy status: %+v", status.Changes)
	}
	for _, tc := range []struct {
		staged                    bool
		oldText, newText, newPath string
	}{
		{true, "original", "staging!", ""},
		{false, "staging!", "unstaged", path},
	} {
		result, err := repo.WorkingDiff(t.Context(), status.Changes[0], tc.staged, false)
		if err != nil || len(result.Files) != 1 {
			t.Fatalf("staged=%v: %+v, %v", tc.staged, result, err)
		}
		file := result.Files[0]
		want := []DiffLine{
			{Kind: "context", Content: "kept", Old: 1, New: 1},
			{Kind: "removed", Content: tc.oldText, Old: 2},
			{Kind: "added", Content: tc.newText, New: 2},
		}
		if file.Path != path || file.WorkTreePath != tc.newPath || len(file.Hunks) != 1 ||
			!reflect.DeepEqual(file.Hunks[0].Lines, want) || file.Additions != 1 || file.Deletions != 1 {
			t.Fatalf("staged=%v: %+v", tc.staged, file)
		}
	}
}

func TestWorkingDiffRenameAndNewFiles(t *testing.T) {
	dir := newFixture(t)
	fixtureGit(t, dir, "mv", "old name.txt", "renamed.txt")
	writeFixture(t, dir, "new.txt", "added\r\n\r\nlast")
	writeFixture(t, dir, "empty.bin", "")
	writeFixture(t, dir, "binary.bin", "abc\x00def")
	writeFixture(t, dir, "large.txt", strings.Repeat("a", previewBytes+1))
	repo := openFixture(t, dir)
	status := readFixtureStatus(t, repo)
	for _, change := range status.Changes {
		result, err := repo.WorkingDiff(t.Context(), change, change.Index == 'R', false)
		if err != nil || len(result.Files) != 1 || result.Files[0].Path != change.Path {
			t.Fatalf("preview %q: %+v, %v", change.Path, result, err)
		}
		file := result.Files[0]
		switch change.Path {
		case "renamed.txt":
			if file.OldSHA == "" || file.OldSHA != file.NewSHA || len(file.Hunks) != 0 {
				t.Fatalf("renamed file: %+v", file)
			}
		case "new.txt":
			if file.Additions != 3 || len(file.Hunks) != 1 || file.Hunks[0].Lines[1].Content != "" ||
				file.Hunks[0].Lines[2].Content != "last" {
				t.Fatalf("untracked file: %+v", file)
			}
		case "binary.bin", "empty.bin":
			if len(file.Hunks) != 0 || file.WorkTreePath != change.Path {
				t.Fatalf("untracked non-text file: %+v", file)
			}
		case "large.txt":
			if !result.TooLarge {
				t.Fatalf("oversized file was accepted")
			}
		default:
			t.Fatalf("unexpected change %q", change.Path)
		}
	}
}

func TestUntrackedPreviewRetainsFullCountsAndTextLimits(t *testing.T) {
	repo := &Repository{WorkTree: t.TempDir()}
	const path = "preview.txt"
	_, lineLimit, charLimit := patchBudget(false)
	for _, tc := range []struct {
		name, contents string
		additions      int
		truncated      bool
	}{
		{"exact line limit", strings.Repeat("\n", lineLimit), lineLimit, false},
		{"over line limit", strings.Repeat("\n", lineLimit) + "last without newline", lineLimit + 1, true},
		{"long and invalid text", strings.Repeat("字", charLimit+1) + "\r\ninvalid \xff\r", 2, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeFixture(t, repo.WorkTree, path, tc.contents)
			preview, err := repo.WorkingDiff(t.Context(), Change{Kind: '?', Path: path}, false, false)
			if err != nil || preview.Truncated != tc.truncated || len(preview.Files) != 1 {
				t.Fatalf("unexpected preview: truncated=%t files=%d error=%v", preview.Truncated, len(preview.Files), err)
			}
			file := preview.Files[0]
			if file.Additions != tc.additions || len(file.Hunks) != 1 {
				t.Fatalf("unexpected count: additions=%d hunks=%d", file.Additions, len(file.Hunks))
			}
			hunk := file.Hunks[0]
			if visible := min(tc.additions, lineLimit); hunk.NewStart != 1 || hunk.NewLines != tc.additions ||
				len(hunk.Lines) != visible || hunk.Lines[visible-1].New != visible {
				t.Fatalf("unexpected hunk ranges: start=%d new=%d visible=%d", hunk.NewStart, hunk.NewLines, len(hunk.Lines))
			}
			if tc.additions == 2 && (hunk.Lines[0].Content != strings.Repeat("字", charLimit) ||
				hunk.Lines[1].Content != "invalid �") {
				t.Fatal("clipping and lossy decoding changed the visible text")
			}
		})
	}
}

func TestWorkingDiffIntentRenameAndImageVersion(t *testing.T) {
	dir := newFixture(t)
	const from, to = "old name.txt", "new name.txt"
	writeFixture(t, dir, from, strings.Repeat("lines that stay in both files\n", 16))
	writeFixture(t, dir, "picture.png", "first\x00image")
	fixtureGit(t, dir, "add", ".")
	fixtureGit(t, dir, "commit", "-m", "Original files")
	writeFixture(t, dir, to, strings.Repeat("lines that stay in both files\n", 16)+"extra line\n")
	if err := os.Remove(filepath.Join(dir, from)); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "add", "-N", "--", to)
	writeFixture(t, dir, "picture.png", "second\x00image")
	repo := openFixture(t, dir)
	status := readFixtureStatus(t, repo)
	for _, change := range status.Changes {
		result, err := repo.WorkingDiff(t.Context(), change, false, false)
		if err != nil || len(result.Files) != 1 {
			t.Fatalf("unstaged %s: %+v, %v", change.Path, result, err)
		}
		switch change.Path {
		case to:
			if change.WorkTree != 'R' || result.Files[0].Additions != 1 || result.Files[0].Deletions != 0 {
				t.Fatalf("unstaged rename: %+v, %+v", change, result)
			}
		case "picture.png":
			before := result.Files[0].WorkTreeVersion
			writeFixture(t, dir, change.Path, "third\x00image!")
			after, err := repo.WorkingDiff(t.Context(), change, false, false)
			if err != nil || len(after.Files) != 1 || before == "" || before == after.Files[0].WorkTreeVersion {
				t.Fatalf("working image version stayed at %q: %+v, %v", before, after, err)
			}
		default:
			t.Fatalf("unexpected change: %+v", change)
		}
	}
}
