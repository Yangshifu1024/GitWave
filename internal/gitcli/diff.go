package gitcli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type DiffPreview struct {
	Files     []FileDiff
	Truncated bool
	TooLarge  bool
}

type FileDiff struct {
	Path            string
	WorkTreePath    string `json:"-"`
	WorkTreeVersion string `json:"-"`
	OldSHA          string `json:"old_sha"`
	NewSHA          string `json:"new_sha"`
	Additions       int
	Deletions       int
	Untracked       bool `json:"untracked,omitempty"`
	Hunks           []DiffHunk
}

type DiffHunk struct {
	OldStart int `json:"old_start"`
	OldLines int `json:"old_lines"`
	NewStart int `json:"new_start"`
	NewLines int `json:"new_lines"`
	Lines    []DiffLine
}

type DiffLine struct {
	Kind, Content string
	Old           int `json:"old_line_no"`
	New           int `json:"new_line_no"`
}

type treeDelta struct {
	file             FileDiff
	oldMode, newMode string
}

const (
	previewBytes  = 2 << 20
	expandedBytes = 8 << 20
	MaxImageBytes = 20 << 20
)

// CommitDiff compares a commit with its first parent. An empty path requests
// the changed-file list without reading patch text or blob content.
func (r *Repository) CommitDiff(ctx context.Context, commit Commit, path string, expanded bool) (DiffPreview, error) {
	if !validOID(commit.SHA) {
		return DiffPreview{}, fmt.Errorf("invalid Git commit ID")
	}
	parent, err := r.firstParent(ctx, commit)
	if err != nil {
		return DiffPreview{}, err
	}
	return r.treeDiff(ctx, parent, commit.SHA, path, expanded)
}

func (r *Repository) treeDiff(ctx context.Context, parent, tip, path string, expanded bool) (DiffPreview, error) {
	base := treeDiffArgs()
	revisions := []string{tip, "--"}
	if parent != "" {
		revisions = append([]string{parent}, revisions...)
	}
	if path != "" {
		if !validFilePath(path) {
			return DiffPreview{}, fmt.Errorf("invalid Git file path")
		}
		// Literal pathspecs still match a directory's descendants. Exclude those
		// explicitly when a commit replaces a file with a directory or vice versa.
		revisions = append(revisions, ":(literal)"+path, ":(exclude,literal)"+path+"/")
	}
	raw, err := r.read(ctx, "", diffArguments(base, revisions, "--raw", "-z", "--no-abbrev")...)
	if err != nil {
		return DiffPreview{}, fmt.Errorf("read changed files: %w", err)
	}
	deltas, err := parseTreeDeltas(string(raw))
	if err != nil {
		return DiffPreview{}, err
	}
	var result DiffPreview
	if path == "" {
		for _, delta := range deltas {
			result.Files = append(result.Files, delta.file)
		}
		return result, nil
	}
	if len(deltas) == 0 {
		return result, nil
	}
	if len(deltas) != 1 || deltas[0].file.Path != path {
		return DiffPreview{}, fmt.Errorf("Git returned an unexpected diff path")
	}
	delta := deltas[0]
	byteLimit, lineLimit, charLimit := patchBudget(expanded)
	tooLarge, err := r.diffBlobsTooLarge(ctx, delta, byteLimit)
	if err != nil {
		return DiffPreview{}, err
	}
	if tooLarge {
		return oversizedDiff(delta.file), nil
	}
	patch, err := r.read(ctx, "", diffPatchArgs(base, revisions)...)
	if err != nil {
		return DiffPreview{}, fmt.Errorf("read file diff: %w", err)
	}
	truncated, err := parsePatch(&delta.file, string(patch), byteLimit, lineLimit, charLimit)
	if err != nil {
		return DiffPreview{}, err
	}
	return DiffPreview{Files: []FileDiff{delta.file}, Truncated: truncated}, nil
}

func treeDiffArgs() []string {
	return []string{"-c", "diff.suppressBlankEmpty=false", "diff-tree", "-r", "--root", "--no-commit-id", "--no-renames",
		"--no-ext-diff", "--no-textconv", "--no-color", "--ignore-submodules=none", "--submodule=short", "-O" + os.DevNull}
}

func patchBudget(expanded bool) (bytes, lines, chars int) {
	if expanded {
		return expandedBytes, 50_000, 200_000
	}
	return previewBytes, 5_000, 50_000
}

func oversizedDiff(file FileDiff) DiffPreview {
	return DiffPreview{Files: []FileDiff{file}, Truncated: true, TooLarge: true}
}

func (r *Repository) diffBlobsTooLarge(ctx context.Context, delta treeDelta, limit int) (bool, error) {
	for _, side := range [...]struct{ oid, mode string }{
		{delta.file.OldSHA, delta.oldMode}, {delta.file.NewSHA, delta.newMode},
	} {
		if side.oid == "" || side.mode == "160000" {
			continue
		}
		size, err := r.blobSize(ctx, side.oid)
		if err != nil {
			return false, err
		}
		if size > limit {
			return true, nil
		}
	}
	return false, nil
}

func diffPatchArgs(base, revisions []string) []string {
	return diffArguments(base, revisions, "--patch", "--unified=3", "--inter-hunk-context=0",
		"--diff-algorithm=myers", "--no-indent-heuristic", "--output-indicator-new=+", "--output-indicator-old=-",
		"--output-indicator-context= ")
}

func validFilePath(path string) bool {
	for part := range strings.SplitSeq(path, "/") {
		if part == "" || part == "." || part == ".." || strings.ContainsRune(part, 0) {
			return false
		}
	}
	return path != ""
}

func diffArguments(base, revisions []string, options ...string) []string {
	args := make([]string, 0, len(base)+len(options)+len(revisions))
	args = append(args, base...)
	args = append(args, options...)
	return append(args, revisions...)
}

func (r *Repository) firstParent(ctx context.Context, commit Commit) (string, error) {
	var parent string
	if len(commit.Parents) > 0 {
		parent = commit.Parents[0]
	} else {
		// Pretty logs omit parents at a shallow boundary. Do not show the whole
		// snapshot as additions for commits whose first-parent tree is missing.
		object, err := r.read(ctx, "", "cat-file", "commit", commit.SHA)
		if err != nil {
			return "", err
		}
		_, rest, _ := strings.Cut(string(object), "\n")
		first, _, _ := strings.Cut(rest, "\n")
		if oid, ok := strings.CutPrefix(first, "parent "); ok {
			parent = oid
		}
	}
	if parent != "" && !validOID(parent) {
		return "", fmt.Errorf("invalid Git parent ID")
	}
	return parent, nil
}

func parseTreeDeltas(raw string) ([]treeDelta, error) {
	var deltas []treeDelta
	for raw != "" {
		meta, rest, ok := strings.Cut(raw, "\x00")
		path, remaining, pathOK := strings.Cut(rest, "\x00")
		fields := strings.Fields(meta)
		if !ok || !pathOK || len(fields) != 5 || !strings.HasPrefix(fields[0], ":") ||
			!validOID(fields[2]) || !validOID(fields[3]) || path == "" {
			return nil, fmt.Errorf("invalid Git tree delta")
		}
		if strings.HasPrefix(fields[4], "R") || strings.HasPrefix(fields[4], "C") {
			path, remaining, pathOK = strings.Cut(remaining, "\x00")
			if !pathOK || path == "" {
				return nil, fmt.Errorf("invalid Git rename destination")
			}
		}
		deltas = append(deltas, treeDelta{oldMode: fields[0][1:], newMode: fields[1], file: FileDiff{
			Path: path, OldSHA: nonzeroOID(fields[2]), NewSHA: nonzeroOID(fields[3]),
		}})
		raw = remaining
	}
	return deltas, nil
}

func nonzeroOID(oid string) string {
	if strings.Trim(oid, "0") == "" {
		return ""
	}
	return oid
}

func (r *Repository) blobSize(ctx context.Context, oid string) (int, error) {
	if !validOID(oid) {
		return 0, fmt.Errorf("invalid Git blob ID")
	}
	output, err := r.read(ctx, oid+"\n", "cat-file", "--batch-check=%(objecttype) %(objectsize)")
	if err != nil {
		return 0, err
	}
	kind, size, _ := strings.Cut(strings.TrimSuffix(string(output), "\n"), " ")
	n, err := strconv.Atoi(size)
	if kind != "blob" || err != nil || n < 0 {
		return 0, fmt.Errorf("Git blob is unavailable")
	}
	return n, nil
}

func (r *Repository) Image(ctx context.Context, oid string) ([]byte, error) {
	size, err := r.blobSize(ctx, oid)
	if err != nil {
		return nil, err
	}
	if size > MaxImageBytes {
		return nil, fmt.Errorf("image exceeds the preview size limit")
	}
	return r.read(ctx, "", "cat-file", "blob", oid)
}
