package gitcli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WorkingDiff previews one side of a change. The staged side compares HEAD
// with the index, while the unstaged side compares the index with the worktree.
func (r *Repository) WorkingDiff(ctx context.Context, change Change, staged, expanded bool) (DiffPreview, error) {
	return r.workingDiff(ctx, change, staged, expanded, WorkingMove{})
}

func (r *Repository) WorkingMoveDiff(ctx context.Context, move WorkingMove, expanded bool) (DiffPreview, error) {
	result, err := r.workingDiff(ctx, move.Change(), move.Staged(), expanded, move)
	if err == nil && move.Staged() {
		if err := r.verifyStagedMovePreview(ctx, move); err != nil {
			return DiffPreview{}, err
		}
	}
	return result, err
}

func (r *Repository) workingDiff(ctx context.Context, change Change, staged, expanded bool, move WorkingMove) (DiffPreview, error) {
	path := change.Path
	if r.WorkTree == "" || !validFilePath(path) {
		return DiffPreview{}, fmt.Errorf("invalid working copy path")
	}
	if change.Kind == 'u' {
		return DiffPreview{}, nil
	}
	if change.Kind == '?' && !staged {
		return r.untrackedDiff(ctx, path, expanded)
	}
	if !staged && change.hasSubmoduleFileChanges() && validOID(change.IndexOID) && validOID(change.WorkTreeOID) {
		lines := []DiffLine{
			{Kind: "removed", Content: "Subproject commit " + change.IndexOID, Old: 1},
			{Kind: "added", Content: "Subproject commit " + change.WorkTreeOID + "-dirty", New: 1},
		}
		file := FileDiff{Path: path, OldSHA: change.IndexOID, Additions: 1, Deletions: 1,
			Hunks: []DiffHunk{{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1, Lines: lines}}}
		return DiffPreview{Files: []FileDiff{file}}, nil
	}
	maxBytes, maxLines, maxChars := patchBudget(expanded)
	base := []string{"-c", "diff.suppressBlankEmpty=false", "diff", "--no-ext-diff", "--no-textconv",
		"--no-color", "--ignore-submodules=dirty", "--submodule=short", "-O" + os.DevNull}
	paths := []string{"--", ":(literal)" + path, ":(exclude,literal)" + path + "/"}
	side := change.WorkTree
	var workTreePath, version string
	head := ""
	if staged {
		var err error
		head, err = r.optionalRef(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
		if err != nil {
			return DiffPreview{}, err
		}
		if move.Staged() && (head != move.stagedHead || !validIndexPath(move.Source.Path)) {
			return DiffPreview{}, fmt.Errorf("the staged move has changed: %q", path)
		}
	}
	git, err := r.newWorkingGit(ctx, head, staged || move.Source.Path == "")
	if err != nil {
		return DiffPreview{}, err
	}
	defer git.close()
	if staged {
		base = append(base, "--cached")
		side = change.Index
		// Some Git versions can reuse matching worktree files even for a
		// cached diff. An empty private worktree keeps preview reads on blobs.
		if err := git.isolateWorkTree(); err != nil {
			return DiffPreview{}, err
		}
		if move.Staged() {
			tree, err := git.stagedMoveSourceTree(ctx, move)
			if err != nil {
				return DiffPreview{}, err
			}
			paths = append([]string{tree}, paths...)
		}
	} else if !strings.HasPrefix(change.Submodule, "S") {
		if err := git.configureLocalLFS(false, false); err != nil {
			return DiffPreview{}, err
		}
		content, err := git.snapshotFile(ctx, change, int64(maxBytes))
		if errors.Is(err, errWorkFileTooLarge) && content.stamp != "" {
			return oversizedDiff(FileDiff{Path: path, WorkTreePath: path, WorkTreeVersion: content.stamp}), nil
		}
		if err != nil {
			return DiffPreview{}, err
		}
		if move.Source.Path != "" {
			if content.moveSignature() != move.snapshot {
				return DiffPreview{}, fmt.Errorf("the moved working file has changed: %q", path)
			}
			source, destinations := move.Source, []string{path}
			if move.matchedText {
				// Compare the already paired source blob at the destination path
				// so Git can use the destination's attributes without re-pairing.
				source.Path, destinations = path, nil
			}
			if err := git.prepareMoveIndex(ctx, []Change{source}, destinations); err != nil {
				return DiffPreview{}, err
			}
		}
		if change.WorkTree != 'D' {
			workTreePath, version = path, content.stamp
		}
	}
	if (side == 'R' || side == 'C') && change.PreviousPath != path && !move.matchedText {
		if !validFilePath(change.PreviousPath) {
			return DiffPreview{}, fmt.Errorf("invalid previous working copy path")
		}
		paths = append(paths, ":(literal)"+change.PreviousPath, ":(exclude,literal)"+change.PreviousPath+"/")
		base = append(base, "--find-renames")
	} else {
		base = append(base, "--no-renames")
	}
	raw, err := git.read(ctx, 256<<10, diffArguments(base, paths, "--raw", "-z", "--no-abbrev")...)
	if err != nil {
		return DiffPreview{}, fmt.Errorf("read working copy delta: %w", err)
	}
	deltas, err := parseTreeDeltas(string(raw))
	if move.matchedText && !staged && err == nil && len(deltas) == 0 {
		return DiffPreview{Files: []FileDiff{{Path: path, OldSHA: move.Source.IndexOID, WorkTreePath: workTreePath, WorkTreeVersion: version}}}, nil
	}
	if err != nil || (len(deltas) == 0 && !move.Staged()) {
		return DiffPreview{}, err
	}
	if len(deltas) != 1 || deltas[0].file.Path != path {
		return DiffPreview{}, fmt.Errorf("working copy changed while reading the file")
	}
	delta := deltas[0]
	if move.Staged() && (delta.file.OldSHA != move.Source.HeadOID || delta.oldMode != move.Source.HeadMode ||
		delta.file.NewSHA != move.Destination.IndexOID || delta.newMode != move.Destination.IndexMode) {
		return DiffPreview{}, fmt.Errorf("the staged move has changed: %q", path)
	}
	if !staged {
		delta.file.NewSHA = ""
		delta.file.WorkTreePath = workTreePath
		delta.file.WorkTreeVersion = version
	}
	tooLarge, err := r.diffBlobsTooLarge(ctx, delta, maxBytes)
	if err != nil {
		return DiffPreview{}, err
	}
	if tooLarge {
		return oversizedDiff(delta.file), nil
	}
	args := append(base, "--raw", "-z", "--no-abbrev", "--full-index")
	output, err := git.read(ctx, int64(maxBytes)*8, diffPatchArgs(args, paths)...)
	if errors.Is(err, errGitOutputLimit) {
		return oversizedDiff(delta.file), nil
	}
	if err != nil {
		return DiffPreview{}, fmt.Errorf("read working copy patch: %w", err)
	}
	rawPatch, patch, found := bytes.Cut(output, []byte{0, 0})
	if !found || !bytes.Equal(rawPatch, raw[:len(raw)-1]) {
		return DiffPreview{}, fmt.Errorf("working copy changed while reading the file")
	}
	if version := workTreePatchVersion(patch); delta.file.WorkTreePath != "" && version != "" {
		delta.file.WorkTreeVersion = version
	}
	truncated, err := parsePatch(&delta.file, string(patch), maxBytes, maxLines, maxChars)
	if err != nil {
		return DiffPreview{}, err
	}
	return DiffPreview{Files: []FileDiff{delta.file}, Truncated: truncated}, nil
}

func (g *workingGit) stagedMoveSourceTree(ctx context.Context, move WorkingMove) (string, error) {
	// Place the captured HEAD blob at the destination in a one-file tree.
	// Comparing it with the live index keeps the destination's Git attributes
	// and lets the caller verify the exact staged blob in the diff header.
	index := g.index
	g.index = filepath.Join(g.dir, "preview-source-index")
	defer func() { g.index = index }()
	source := Change{Path: move.Destination.Path, IndexOID: move.Source.HeadOID, IndexMode: move.Source.HeadMode}
	if err := g.prepareMoveIndex(ctx, []Change{source}, nil); err != nil {
		return "", err
	}
	return g.writeIndexTree(ctx)
}

func (r *Repository) verifyStagedMovePreview(ctx context.Context, move WorkingMove) error {
	head, err := r.optionalRef(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return err
	}
	source, destination := move.Source.Path, move.Destination.Path
	if head != move.stagedHead {
		return fmt.Errorf("the staged move has changed: %q", destination)
	}
	output, err := r.readLimited(ctx, "", 256<<10, "ls-files", "--stage", "-z", "--", ":(literal)"+source, ":(literal)"+destination)
	if err != nil {
		return err
	}
	entry := move.Destination.IndexMode + " " + move.Destination.IndexOID + " 0"
	found := false
	for record := range strings.SplitSeq(strings.TrimSuffix(string(output), "\x00"), "\x00") {
		meta, path, _ := strings.Cut(record, "\t")
		if path == source || (path == destination && (found || meta != entry)) {
			return fmt.Errorf("the staged move has changed: %q", destination)
		}
		found = found || path == destination
	}
	if !found {
		return fmt.Errorf("the staged move has changed: %q", destination)
	}
	return nil
}

func workTreePatchVersion(patch []byte) string {
	for line := range bytes.SplitSeq(patch, []byte{'\n'}) {
		if bytes.HasPrefix(line, []byte("@@ ")) {
			break
		}
		index, ok := bytes.CutPrefix(line, []byte("index "))
		if !ok {
			continue
		}
		_, version, _ := bytes.Cut(index, []byte(".."))
		oid, _, _ := bytes.Cut(version, []byte{' '})
		if validOIDBytes(oid) {
			return string(oid)
		}
	}
	return ""
}

func (r *Repository) untrackedDiff(ctx context.Context, path string, expanded bool) (DiffPreview, error) {
	file := FileDiff{Path: path, WorkTreePath: path}
	maxBytes, maxLines, maxChars := patchBudget(expanded)
	input, err := r.workFile(ctx, path, int64(maxBytes))
	if errors.Is(err, errWorkFileTooLarge) {
		file.WorkTreeVersion = input.stamp
		return oversizedDiff(file), nil
	}
	if err != nil {
		return DiffPreview{}, err
	}
	contents := input.data
	file.WorkTreeVersion = fmt.Sprintf("%x", sha256.Sum256(contents))
	if len(contents) == 0 || bytes.IndexByte(contents[:min(len(contents), 8000)], 0) >= 0 {
		return DiffPreview{Files: []FileDiff{file}}, nil
	}
	text := string(contents)
	count := strings.Count(text, "\n")
	if text[len(text)-1] != '\n' {
		count++
	}
	// The file read already enforces the byte budget. Only decode lines that
	// can be displayed, while retaining the full addition count in the header.
	visible := min(count, maxLines)
	hunk := DiffHunk{NewStart: 1, NewLines: count, Lines: make([]DiffLine, 0, visible)}
	truncated := count > visible
	for i := range visible {
		line, rest, _ := strings.Cut(text, "\n")
		content, clipped := previewLineText(line, maxChars)
		hunk.Lines = append(hunk.Lines, DiffLine{Kind: "added", Content: content, New: i + 1})
		truncated = truncated || clipped
		text = rest
	}
	file.Additions = count
	file.Hunks = []DiffHunk{hunk}
	return DiffPreview{Files: []FileDiff{file}, Truncated: truncated}, nil
}
