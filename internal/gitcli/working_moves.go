package gitcli

import (
	"context"
	"crypto/sha256"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"gitwave.work/internal/process"
)

const (
	maxMoveFileBytes = 16 << 20
	moveScanBytes    = 64 << 20
	maxMovePaths     = 1000
)

// WorkingMove pairs two porcelain entries for presentation. Writes still
// validate and apply both original entries under the ordinary index lock.
type WorkingMove struct {
	Source, Destination Change
	snapshot            moveSignature
	cleanOID            string
	stagedHead          string
	matchedText         bool
}

type moveSignature struct {
	hash [sha256.Size]byte
	mode os.FileMode
}

func (m WorkingMove) Staged() bool {
	return m.Source.Index == 'D' && m.Destination.Index == 'A'
}

func (m WorkingMove) Change() Change {
	if m.Staged() {
		change := m.Destination
		change.Kind, change.Index, change.PreviousPath = '2', 'R', m.Source.Path
		change.HeadOID, change.HeadMode = m.Source.HeadOID, m.Source.HeadMode
		return change
	}
	return Change{Kind: '2', Index: '.', WorkTree: 'R', Path: m.Destination.Path, PreviousPath: m.Source.Path,
		IndexOID: m.Source.IndexOID, IndexMode: m.Source.IndexMode}
}

func (c workFileContent) moveSignature() moveSignature {
	return moveSignature{hash: sha256.Sum256(c.data), mode: c.mode & (os.ModeSymlink | 0o100)}
}

// WorkingMoves detects renames on a private index and worktree. An inconclusive
// scan leaves the ordinary delete and untracked rows usable.
func (r *Repository) WorkingMoves(ctx context.Context, status Status) []WorkingMove {
	var sources, destinations []Change
	for _, c := range status.Changes {
		if !validIndexPath(c.Path) {
			continue
		}
		if c.Kind == '?' {
			destinations = append(destinations, c)
		} else if c.Kind == '1' && c.Index == '.' && c.WorkTree == 'D' &&
			(regularMoveMode(c.IndexMode) || c.IndexMode == "120000") && validOID(c.IndexOID) {
			sources = append(sources, c)
		}
	}
	if len(sources) == 0 || len(destinations) == 0 || len(sources) > maxMovePaths {
		return nil
	}
	textDestinations := uniqueTextMoveDestinations(sources, destinations)
	git, err := r.newWorkingGit(ctx, status.HeadOID, false)
	if err != nil {
		return nil
	}
	defer git.close()
	// Cancel the whole process group when a speculative Git read is dropped.
	// Without group cancellation, a launcher child can hold the output pipe.
	git.groupCancellation = process.SupportsGroupCancellation
	sources, sizes := git.moveSourcesWithinBudget(ctx, sources)
	if len(sources) == 0 {
		return nil
	}
	if err := git.isolateWorkTree(); err != nil {
		return nil
	}
	return git.findWorkingMoves(ctx, status.HeadOID, sources, destinations, sizes, textDestinations)
}

func (g *workingGit) moveSourcesWithinBudget(ctx context.Context, sources []Change) ([]Change, map[string]int64) {
	var ids strings.Builder
	for _, c := range sources {
		ids.WriteString(c.IndexOID)
		ids.WriteByte('\n')
	}
	output, err := g.run(ctx, ids.String(), 128<<10, "cat-file", "--batch-check=%(objecttype) %(objectsize)")
	if err != nil {
		return nil, nil
	}
	records := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if len(records) != len(sources) {
		return nil, nil
	}
	var eligible []Change
	sizes := make(map[string]int64, len(sources))
	remaining := int64(moveScanBytes)
	for i, record := range records {
		value, ok := strings.CutPrefix(record, "blob ")
		size, err := strconv.ParseInt(value, 10, 64)
		if !ok || err != nil || size < 0 || size > maxMoveFileBytes {
			continue
		}
		remaining -= size
		if remaining < 0 {
			return nil, nil
		}
		eligible = append(eligible, sources[i])
		sizes[sources[i].IndexOID] = size
	}
	return eligible, sizes
}

func (g *workingGit) findWorkingMoves(ctx context.Context, head string, sources, destinations []Change, sourceSizes map[string]int64, textDestinations map[string]string) []WorkingMove {
	root, err := os.OpenRoot(g.repo.WorkTree)
	if err != nil {
		return nil
	}
	defer root.Close()
	target, err := os.OpenRoot(g.workTree)
	if err != nil {
		return nil
	}
	defer target.Close()
	var previous *workingMoveScan
	if g.repo.workingMoveCache != nil {
		previous = g.repo.workingMoveCache.Load()
	}
	snapshot := moveSnapshotFiles{root: target}
	if previous != nil {
		snapshot.remaining = moveSnapshotBufferBytes
	}
	defer snapshot.close()
	var paths []string
	byPath := make(map[string]WorkingMove)
	var externalPaths []string
	var attributeProbe sync.WaitGroup
	probeCtx, stopProbe := context.WithCancel(ctx)
	defer func() {
		stopProbe()
		attributeProbe.Wait()
	}()
	sizes := slices.Sorted(maps.Values(sourceSizes))
	remaining := int64(moveScanBytes)
	for _, c := range destinations {
		if ctx.Err() != nil {
			return nil
		}
		local := filepath.FromSlash(c.Path)
		info, err := root.Lstat(local)
		if err != nil || (!info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0) {
			continue
		}
		size := info.Size()
		if size > maxMoveFileBytes {
			continue
		}
		// Allow extra room for built-in line-ending and encoding conversions.
		at, _ := slices.BinarySearch(sizes, (size+3)/4)
		if info.Mode()&os.ModeSymlink == 0 && (at == len(sizes) || sizes[at] > 4*size) {
			continue
		}
		if size > remaining || len(paths) == maxMovePaths {
			return nil
		}
		if g.groupCancellation && len(paths) == 0 && g.repo.workingMoveCache != nil {
			attributeProbe.Go(func() { externalPaths = g.moveAttributePaths(probeCtx) })
		}
		contents, err := readMoveFile(ctx, root, local, min(remaining, maxMoveFileBytes), info)
		if err != nil || snapshot.add(local, contents) != nil {
			return nil
		}
		remaining -= int64(len(contents.data))
		paths = append(paths, c.Path)
		signature := contents.moveSignature()
		byPath[c.Path] = WorkingMove{Destination: c, snapshot: signature}
	}
	if len(paths) == 0 {
		return nil
	}
	attributes, err := snapshotMoveAttributes(ctx, root, &snapshot, paths, remaining)
	if err != nil {
		return nil
	}
	attributeProbe.Wait()
	if !g.groupCancellation && g.repo.workingMoveCache != nil {
		externalPaths = g.moveAttributePaths(ctx)
	}
	if ctx.Err() != nil {
		return nil
	}
	textCandidates := textMoveCandidates(sources, textDestinations, byPath, sourceSizes)
	scan := g.moveScan(head, sources, paths, byPath, textCandidates, attributes, externalPaths)
	if scan != nil && scan.matches(previous) && ctx.Err() == nil {
		return slices.Clone(previous.moves)
	}
	if err := snapshot.materialize(); err != nil {
		return nil
	}
	if err := g.prepareMoveIndex(ctx, sources, paths); err != nil {
		return nil
	}
	output, cleanIDs, err := g.compareWorkingMoves(ctx, paths, byPath)
	if err != nil {
		return nil
	}
	bySource := make(map[string]Change, len(sources))
	byOID := make(map[string]int, len(sources))
	for _, c := range sources {
		bySource[c.Path] = c
		byOID[c.IndexOID]++
	}
	var pairs []WorkingMove
	claimed := make(map[string]bool)
	for raw := string(output); raw != ""; {
		kind, rest, kindOK := strings.Cut(raw, "\x00")
		source, rest, sourceOK := strings.Cut(rest, "\x00")
		destination, remaining, destinationOK := strings.Cut(rest, "\x00")
		from, move := bySource[source], byPath[destination]
		if !kindOK || !sourceOK || !destinationOK || !strings.HasPrefix(kind, "R") || from.Path == "" || move.Destination.Path == "" {
			return nil
		}
		move.Source = from
		pairs = append(pairs, move)
		claimed[source], claimed[destination] = true, true
		raw = remaining
	}
	textMoves, err := g.matchTextMoves(ctx, target, textCandidates, claimed, sourceSizes)
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		// A failed supplemental read does not invalidate Git's matches. Retry
		// the supplemental scan next time instead of caching an incomplete result.
		scan = nil
	} else if len(textMoves) != 0 {
		if cleanIDs == nil {
			cleanIDs, err = g.uniqueMoveDestinations(ctx, paths, byPath)
			if err != nil {
				return nil
			}
		}
		pairs = append(pairs, textMoves...)
	}
	var moves []WorkingMove
	for _, move := range pairs {
		if oid := cleanIDs[move.Destination.Path]; byOID[move.Source.IndexOID] == 1 && oid != "" {
			move.cleanOID = oid
			if move.snapshot.mode&os.ModeSymlink != 0 {
				// Git only recognizes exact symlink matches.
				move.cleanOID = move.Source.IndexOID
			}
			moves = append(moves, move)
		}
	}
	g.repo.rememberMoveScan(ctx, scan, moves)
	return moves
}

func (g *workingGit) compareWorkingMoves(ctx context.Context, paths []string, candidates map[string]WorkingMove) ([]byte, map[string]string, error) {
	var cleanIDs map[string]string
	var hashErr error
	var hashes sync.WaitGroup
	hashCtx, stopHash := context.WithCancel(ctx)
	defer stopHash()
	parallel := g.groupCancellation && g.frozenAttributes
	if parallel {
		// Both commands read the completed private index and worktree.
		hashes.Go(func() { cleanIDs, hashErr = g.uniqueMoveDestinations(hashCtx, paths, candidates) })
	}
	output, err := g.read(ctx, 8<<20, "diff", "--no-ext-diff", "--no-textconv", "--find-renames=50%", "-l200", "--diff-filter=R", "--name-status", "-z", "--")
	if err != nil || len(output) == 0 {
		stopHash()
		hashes.Wait()
		return output, nil, err
	}
	if !parallel {
		cleanIDs, hashErr = g.uniqueMoveDestinations(ctx, paths, candidates)
	}
	hashes.Wait()
	return output, cleanIDs, hashErr
}

func (g *workingGit) uniqueMoveDestinations(ctx context.Context, paths []string, candidates map[string]WorkingMove) (map[string]string, error) {
	var input strings.Builder
	var regular []string
	content := make(map[string]string, len(paths))
	for _, path := range paths {
		signature := candidates[path].snapshot
		if signature.mode&os.ModeSymlink != 0 {
			content[path] = fmt.Sprintf("link:%x", signature.hash)
			continue
		}
		regular = append(regular, path)
		input.WriteString(quoteGitPath(path))
		input.WriteByte('\n')
	}
	if len(regular) != 0 {
		output, err := g.run(ctx, input.String(), 128<<10, "hash-object", "--stdin-paths")
		if err != nil {
			return nil, err
		}
		ids := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
		if len(ids) != len(regular) {
			return nil, fmt.Errorf("invalid Git move contents")
		}
		for i, path := range regular {
			content[path] = ids[i]
		}
	}
	counts := make(map[string]int, len(content))
	for _, id := range content {
		counts[id]++
	}
	for path, id := range content {
		if counts[id] != 1 {
			delete(content, path)
		}
	}
	return content, nil
}

func readMoveFile(ctx context.Context, root *os.Root, local string, limit int64, info os.FileInfo) (workFileContent, error) {
	parent, err := openWorktreeParent(root, local, false)
	if err != nil {
		return workFileContent{}, err
	}
	if parent != root {
		defer parent.Close()
	}
	name := filepath.Base(local)
	if info != nil && info.Mode().IsRegular() {
		return readWorkFileWithInfo(ctx, parent, name, limit, info)
	}
	return readWorkFile(ctx, parent, name, limit)
}

func snapshotMoveAttributes(ctx context.Context, root *os.Root, snapshot *moveSnapshotFiles, paths []string, remaining int64) ([]moveAttribute, error) {
	var attributes []moveAttribute
	copied := make(map[string]bool, len(paths))
	for _, path := range paths {
		copied[filepath.FromSlash(path)] = true
	}
	for _, path := range paths {
		for parent := filepath.Dir(filepath.FromSlash(path)); ; parent = filepath.Dir(parent) {
			name := filepath.Join(parent, ".gitattributes")
			if !copied[name] {
				copied[name] = true
				file, err := readMoveFile(ctx, root, name, min(remaining, maxMoveFileBytes), nil)
				if err != nil && !os.IsNotExist(err) {
					return nil, err
				}
				if err == nil && file.mode.IsRegular() {
					remaining -= int64(len(file.data))
					if err := snapshot.add(name, file); err != nil {
						return nil, err
					}
					attributes = append(attributes, moveAttribute{path: name, hash: sha256.Sum256(file.data)})
				}
			}
			if parent == "." {
				break
			}
		}
	}
	return attributes, nil
}

func (g *workingGit) prepareMoveIndex(ctx context.Context, sources []Change, destinations []string) error {
	if err := os.Mkdir(filepath.Join(g.dir, "objects"), 0o700); err != nil {
		return err
	}
	g.privateObjects = true
	g.settings = append(g.settings, "-c", "core.ignorecase=false", "-c", "core.splitIndex=false")
	var entries strings.Builder
	for _, c := range sources {
		fmt.Fprintf(&entries, "%s %s\t%s\x00", c.IndexMode, c.IndexOID, c.Path)
	}
	if _, err := g.run(ctx, entries.String(), 64<<10, "update-index", "-z", "--index-info"); err != nil {
		return err
	}
	if len(destinations) == 0 {
		return nil
	}
	pathspecs := make([]string, len(destinations))
	for i, path := range destinations {
		pathspecs[i] = ":(top,literal)" + path
	}
	_, err := g.run(ctx, nulPaths(pathspecs), 64<<10, "add", "--intent-to-add", "--force", "--pathspec-from-file=-", "--pathspec-file-nul")
	return err
}

func (g *workingGit) verifyMoveDestinations(ctx context.Context, moves []WorkingMove) error {
	root, err := os.OpenRoot(g.workTree)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, move := range moves {
		path := move.Destination.Path
		file, err := readMoveFile(ctx, root, filepath.FromSlash(path), maxMoveFileBytes, nil)
		if err != nil || file.moveSignature() != move.snapshot {
			return fmt.Errorf("the moved working file has changed: %q", path)
		}
	}
	return nil
}
