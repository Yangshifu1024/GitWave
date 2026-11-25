package gitcli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCommitDiffScopesFileReplacedByDirectory(t *testing.T) {
	dir := newFixture(t)
	const path = "[sample].txt"
	writeFixture(t, dir, path, "wanted old line\n")
	fixtureGit(t, dir, "add", ".")
	fixtureGit(t, dir, "commit", "-m", "Flat file")
	if err := os.Remove(filepath.Join(dir, path)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, path), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, path+"/nested", strings.Repeat("a", previewBytes+1))
	writeFixture(t, dir, "s.txt", "globbing would match me\n")
	fixtureGit(t, dir, "add", "-A")
	fixtureGit(t, dir, "commit", "-m", "Directory")
	fixtureGit(t, dir, "config", "diff.orderFile", "missing-order-file")
	repo := openFixture(t, dir)
	heads, err := repo.Commits(t.Context(), []string{strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := repo.CommitDiff(t.Context(), heads[0], path, false)
	if err != nil || len(preview.Files) != 1 || preview.Truncated || preview.TooLarge {
		t.Fatalf("selected file diff: %+v, %v", preview, err)
	}
	file := preview.Files[0]
	if file.Path != path || file.NewSHA != "" || file.Deletions != 1 || len(file.Hunks) != 1 ||
		!slices.Equal(file.Hunks[0].Lines, []DiffLine{{Kind: "removed", Content: "wanted old line", Old: 1}}) {
		t.Fatalf("unexpected diff: %+v", file)
	}
}

func TestPatchTruncatesRowsButCountsFullHunks(t *testing.T) {
	patch := "@@ -1,2 +1,3 @@\n before\n-old\n+新内容\n+new tail\n\\ No newline at end of file\n"
	var file FileDiff
	truncated, err := parsePatch(&file, patch, previewBytes, 3, 2)
	if err != nil || !truncated || file.Additions != 2 || file.Deletions != 1 || len(file.Hunks) != 1 {
		t.Fatalf("bounded patch: %+v, truncated=%v, %v", file, truncated, err)
	}
	want := []DiffLine{
		{Kind: "context", Content: "be", Old: 1, New: 1},
		{Kind: "removed", Content: "ol", Old: 2},
		{Kind: "added", Content: "新内", New: 2},
	}
	if !slices.Equal(file.Hunks[0].Lines, want) {
		t.Fatalf("unexpected diff lines: %+v", file.Hunks[0].Lines)
	}
	if _, err := parsePatch(&FileDiff{}, "@@ -1 +1 @@\n-old\n", previewBytes, 10, 10); err == nil {
		t.Fatal("incomplete patch accepted")
	}
	if got, want := lossyGitText("\x80\x81:\xe1\x80!\xe0\x80:\xf1\x80"), "��:�!��:�"; got != want {
		t.Fatalf("invalid bytes: got %q, want %q", got, want)
	}
}

func TestCommitDiffDoesNotTreatShallowBoundaryAsRoot(t *testing.T) {
	dir := newFixture(t)
	writeFixture(t, dir, "mixed.txt", "second revision\n")
	fixtureGit(t, dir, "commit", "-am", "Second")
	clone := filepath.Join(t.TempDir(), "shallow")
	fixtureGit(t, dir, "clone", "--no-local", "--depth=1", dir, clone)
	repo := openFixture(t, clone)
	heads, err := repo.Commits(t.Context(), []string{strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))})
	if err != nil {
		t.Fatal(err)
	}
	if diff, err := repo.CommitDiff(t.Context(), heads[0], "", false); err == nil {
		t.Fatalf("missing parent reported as root diff: %+v", diff)
	}
}

func TestCommitDiffShowsBothSidesOfFileTypeChange(t *testing.T) {
	dir := newFixture(t)
	writeFixture(t, dir, "target", "target contents\n")
	fixtureGit(t, dir, "add", "target")
	// Stage the symlink by object ID so the fixture needs no symlink privileges.
	oid := strings.TrimSpace(fixtureGit(t, dir, "hash-object", "-w", "target"))
	fixtureGit(t, dir, "update-index", "--cacheinfo", "120000,"+oid+",mixed.txt")
	fixtureGit(t, dir, "commit", "-m", "Change file type")
	repo := openFixture(t, dir)
	heads, err := repo.Commits(t.Context(), []string{strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))})
	if err != nil {
		t.Fatal(err)
	}
	diff, err := repo.CommitDiff(t.Context(), heads[0], "mixed.txt", false)
	if err != nil || len(diff.Files) != 1 {
		t.Fatalf("type change: %+v, %v", diff, err)
	}
	file := diff.Files[0]
	if file.Path != "mixed.txt" || file.Additions != 1 || file.Deletions != 1 ||
		file.OldSHA == "" || file.NewSHA != oid || len(file.Hunks) != 2 {
		t.Fatalf("type change omitted a side: %+v", file)
	}
}

func TestImageRejectsOversizedBlob(t *testing.T) {
	dir := newFixture(t)
	image := filepath.Join(dir, "sample.png")
	writeFixture(t, dir, "sample.png", "image")
	if err := os.Truncate(image, MaxImageBytes+1); err != nil {
		t.Fatal(err)
	}
	oid := strings.TrimSpace(fixtureGit(t, dir, "hash-object", "-w", "sample.png"))
	repo := openFixture(t, dir)
	data, err := repo.Image(t.Context(), oid)
	if err == nil || !strings.Contains(err.Error(), "preview size limit") || len(data) != 0 {
		t.Fatalf("oversized image returned %d bytes, %v", len(data), err)
	}
}
