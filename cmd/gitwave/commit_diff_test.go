package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCommitDiffHandlesFirstParentExactPathsAndPreviewLimits(t *testing.T) {
	repo, git := historyRepository(t)
	put := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	put("mixed.txt", "first\r\n\r\nold tail")
	put("[sample].txt", "flat file\n")
	put("sample.png", "\x00old bytes")
	git("", "add", ".")
	git("", "commit", "-m", "Root")
	root := git("", "rev-parse", "HEAD")
	git("", "switch", "-c", "topic")
	put("topic.txt", "topic\n")
	git("", "add", ".")
	git("", "commit", "-m", "Topic")
	git("", "switch", "main")
	git("", "merge", "--no-ff", "topic", "-m", "Merge topic")
	merge := git("", "rev-parse", "HEAD")
	if err := os.Remove(filepath.Join(repo, "[sample].txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(repo, "[sample].txt"), 0o700); err != nil {
		t.Fatal(err)
	}
	put("[sample].txt/child", "nested content\n")
	put("mixed.txt", "first\r\n\r\nnew tail")
	put("large.txt", " large\n"+strings.Repeat("a", 2<<20))
	put("many.txt", strings.Repeat("line\n", 5_010))
	put("sample.png", "\x00new bytes")
	git("", "add", "-A")
	git("", "commit", "-m", "Update")
	tip := git("", "rev-parse", "HEAD")
	ctx := t.Context()
	reader, err := startBackend(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	blob := func(sha, path string) string {
		return git("", "rev-parse", sha+":"+path)
	}
	rootFiles := []fileDiff{
		{Path: "[sample].txt", NewSHA: blob(root, "[sample].txt")},
		{Path: "mixed.txt", NewSHA: blob(root, "mixed.txt")},
		{Path: "sample.png", NewSHA: blob(root, "sample.png")},
	}
	mergeFile := fileDiff{Path: "topic.txt", NewSHA: blob(merge, "topic.txt")}
	tipFiles := []fileDiff{
		{Path: "[sample].txt", OldSHA: rootFiles[0].NewSHA},
		{Path: "[sample].txt/child", NewSHA: blob(tip, "[sample].txt/child")},
		{Path: "large.txt", NewSHA: blob(tip, "large.txt")},
		{Path: "many.txt", NewSHA: blob(tip, "many.txt")},
		{Path: "mixed.txt", OldSHA: rootFiles[1].NewSHA, NewSHA: blob(tip, "mixed.txt")},
		{Path: "sample.png", OldSHA: rootFiles[2].NewSHA, NewSHA: blob(tip, "sample.png")},
	}
	withHunk := func(file fileDiff, adds, deletes int, hunk diffHunk) fileDiff {
		file.Additions, file.Deletions, file.Hunks = adds, deletes, []diffHunk{hunk}
		return file
	}
	addedLines := func(contents ...string) diffHunk {
		hunk := diffHunk{NewStart: 1, NewLines: len(contents)}
		for i, content := range contents {
			hunk.Lines = append(hunk.Lines, diffLine{Kind: "added", Content: content, New: i + 1})
		}
		return hunk
	}
	many := diffHunk{NewStart: 1, NewLines: 5_010, Lines: make([]diffLine, 5_000)}
	for i := range many.Lines {
		many.Lines[i] = diffLine{Kind: "added", Content: "line", New: i + 1}
	}
	for _, test := range []struct {
		name, sha, path string
		expanded        bool
		want            preview
	}{
		{name: "root file list", sha: root, want: preview{Files: rootFiles}},
		{name: "root CRLF patch", sha: root, path: "mixed.txt", want: preview{
			Files: []fileDiff{withHunk(rootFiles[1], 3, 0, addedLines("first", "", "old tail"))},
		}},
		{name: "merge file list against first parent", sha: merge, want: preview{Files: []fileDiff{mergeFile}}},
		{name: "merge patch against first parent", sha: merge, path: "topic.txt", want: preview{
			Files: []fileDiff{withHunk(mergeFile, 1, 0, addedLines("topic"))},
		}},
		{name: "update file list", sha: tip, want: preview{Files: tipFiles}},
		{name: "updated CRLF patch", sha: tip, path: "mixed.txt", want: preview{
			Files: []fileDiff{withHunk(tipFiles[4], 1, 1, crlfTailHunk("old tail", "new tail"))},
		}},
		{name: "deleted file without its replacement directory", sha: tip, path: "[sample].txt", want: preview{
			Files: []fileDiff{withHunk(tipFiles[0], 0, 1, diffHunk{OldStart: 1, OldLines: 1,
				Lines: []diffLine{{Kind: "removed", Content: "flat file", Old: 1}}})},
		}},
		{name: "new child without the deleted parent file", sha: tip, path: "[sample].txt/child", want: preview{
			Files: []fileDiff{withHunk(tipFiles[1], 1, 0, addedLines("nested content"))},
		}},
		{name: "large patch needs expansion", sha: tip, path: "large.txt", want: preview{
			Files: []fileDiff{tipFiles[2]}, Truncated: true, TooLarge: true,
		}},
		{name: "expanded patch still caps characters per line", sha: tip, path: "large.txt", expanded: true, want: preview{
			Files: []fileDiff{withHunk(tipFiles[2], 2, 0, addedLines(" large", strings.Repeat("a", 200_000)))}, Truncated: true,
		}},
		{name: "line limit preserves total addition count", sha: tip, path: "many.txt", want: preview{
			Files: []fileDiff{withHunk(tipFiles[3], 5_010, 0, many)}, Truncated: true,
		}},
		{name: "binary patch preserves both blob IDs", sha: tip, path: "sample.png", want: preview{Files: []fileDiff{tipFiles[5]}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			details, err := reader.loadCommit(ctx, test.sha)
			if err != nil {
				t.Fatal(err)
			}
			got, err := reader.repo.CommitDiff(ctx, details, test.path, test.expanded)
			if err != nil {
				t.Fatal(err)
			}
			if got.Truncated != test.want.Truncated || got.TooLarge != test.want.TooLarge || len(got.Files) != len(test.want.Files) {
				t.Fatalf("diff truncated=%t tooLarge=%t files=%d, want %t/%t/%d",
					got.Truncated, got.TooLarge, len(got.Files), test.want.Truncated, test.want.TooLarge, len(test.want.Files))
			}
			for i, file := range got.Files {
				want := test.want.Files[i]
				if !reflect.DeepEqual(file, want) {
					t.Fatalf("delta %d: path %q, old/new blobs %q/%q, +%d -%d, hunks %d; want %q %q/%q +%d -%d hunks %d",
						i, file.Path, file.OldSHA, file.NewSHA, file.Additions, file.Deletions, len(file.Hunks),
						want.Path, want.OldSHA, want.NewSHA, want.Additions, want.Deletions, len(want.Hunks))
				}
			}
		})
	}
	image, err := reader.repo.Image(ctx, tipFiles[5].NewSHA)
	if err != nil || string(image) != "\x00new bytes" {
		t.Fatalf("new image blob = %q, %v", image, err)
	}
}
