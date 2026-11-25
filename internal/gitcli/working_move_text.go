package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Count every candidate name before filtering by size. An oversized file
// can still make a smaller file with the same name ambiguous.
func uniqueTextMoveDestinations(sources, destinations []Change) map[string]string {
	oldNames := make(map[string]int, len(sources))
	for _, source := range sources {
		oldNames[path.Base(source.Path)]++
	}
	newNames := make(map[string]string)
	for _, destination := range destinations {
		name := path.Base(destination.Path)
		if oldNames[name] != 1 {
			continue
		}
		if _, exists := newNames[name]; exists {
			newNames[name] = ""
		} else {
			newNames[name] = destination.Path
		}
	}
	return newNames
}

func textMoveCandidates(sources []Change, destinations map[string]string, byPath map[string]WorkingMove, sourceSizes map[string]int64) []WorkingMove {
	var pairs []WorkingMove
	for _, source := range sources {
		size := sourceSizes[source.IndexOID]
		move, ok := byPath[destinations[path.Base(source.Path)]]
		if !ok || source.IndexMode == "120000" || size == 0 || size > previewBytes || move.snapshot.mode&os.ModeSymlink != 0 {
			continue
		}
		move.Source = source
		pairs = append(pairs, move)
	}
	return pairs
}

// Git's rename score measures copied bytes against the larger file. A file
// moved to a different directory can keep much of its original text while
// growing enough to miss that threshold. Compare only unclaimed text pairs.
func (g *workingGit) matchTextMoves(ctx context.Context, root *os.Root, candidates []WorkingMove, claimed map[string]bool, sourceSizes map[string]int64) ([]WorkingMove, error) {
	var pairs []WorkingMove
	sizes := make(map[string]int64)
	for _, move := range candidates {
		if claimed[move.Source.Path] || claimed[move.Destination.Path] {
			continue
		}
		pairs = append(pairs, move)
		sizes[move.Source.IndexOID] = sourceSizes[move.Source.IndexOID]
	}
	if len(pairs) == 0 {
		return nil, nil
	}
	blobs, err := readMoveBlobs(ctx, g.run, sizes)
	if err != nil {
		return nil, err
	}
	var moves []WorkingMove
	for _, move := range pairs {
		current, err := readMoveFile(ctx, root, filepath.FromSlash(move.Destination.Path), previewBytes, nil)
		if errors.Is(err, errWorkFileTooLarge) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if matchesMoveText(blobs[move.Source.IndexOID], current.data) {
			move.matchedText = true
			moves = append(moves, move)
		}
	}
	return moves, nil
}

type moveGitRead func(context.Context, string, int64, ...string) ([]byte, error)

func readMoveBlobs(ctx context.Context, read moveGitRead, sizes map[string]int64) (map[string][]byte, error) {
	if len(sizes) == 0 {
		return nil, nil
	}
	ids := slices.Sorted(maps.Keys(sizes))
	var outputLimit int64
	for _, size := range sizes {
		outputLimit += size + 128 // object ID, type, size and newlines
	}
	if outputLimit > moveScanBytes+int64(len(sizes))*128 {
		return nil, fmt.Errorf("Git text move scan exceeds the read limit")
	}
	output, err := read(ctx, strings.Join(ids, "\n")+"\n", outputLimit, "cat-file", "--batch")
	if err != nil {
		return nil, err
	}
	blobs := make(map[string][]byte, len(ids))
	for _, oid := range ids {
		header, rest, found := bytes.Cut(output, []byte{'\n'})
		fields := strings.Fields(string(header))
		if !found || len(fields) != 3 || fields[0] != oid || fields[1] != "blob" {
			return nil, fmt.Errorf("invalid Git blob in text move scan")
		}
		size, err := strconv.Atoi(fields[2])
		if err != nil || int64(size) != sizes[oid] || size < 0 || size >= len(rest) || rest[size] != '\n' {
			return nil, fmt.Errorf("truncated Git blob in text move scan")
		}
		blobs[oid], output = rest[:size], rest[size+1:]
	}
	if len(output) != 0 {
		return nil, fmt.Errorf("unexpected data in Git text move scan")
	}
	return blobs, nil
}

func matchesMoveText(previous, current []byte) bool {
	if bytes.IndexByte(previous, 0) >= 0 || bytes.IndexByte(current, 0) >= 0 {
		return false
	}
	oldLines, oldBytes := moveTextLines(previous)
	newLines, newBytes := moveTextLines(current)
	var commonLines, commonBytes int
	for line := range oldLines {
		if _, exists := newLines[line]; exists {
			commonLines++
			commonBytes += len(line)
		}
	}
	// Require both substantive overlap and several distinct shared lines.
	return commonLines >= 3 && 4*commonBytes >= oldBytes+newBytes
}

func moveTextLines(data []byte) (map[string]struct{}, int) {
	lines := make(map[string]struct{})
	var size int
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimLeft(strings.TrimSuffix(line, "\r"), " \t")
		if line == "" {
			continue
		}
		if _, exists := lines[line]; !exists {
			lines[line] = struct{}{}
			size += len(line)
		}
	}
	return lines, size
}
