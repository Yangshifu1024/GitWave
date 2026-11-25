package gitcli

import (
	"context"
	"fmt"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"
)

type stagedMoveScan struct {
	candidates, moves []WorkingMove
}

// StagedMoves supplements Git's staged renames with unambiguous text pairs.
// The status entries stay intact for index validation and worktree changes.
func (r *Repository) StagedMoves(ctx context.Context, status Status) []WorkingMove {
	if !validOID(status.HeadOID) || ctx.Err() != nil {
		return nil
	}
	var sources, destinations []Change
	for _, change := range status.Changes {
		if change.Kind != '1' || (change.Index != 'A' && change.Index != 'D') || !validIndexPath(change.Path) {
			continue
		}
		switch change.Index {
		case 'D':
			sources = append(sources, change)
		case 'A':
			destinations = append(destinations, change)
		}
	}
	if len(sources) == 0 || len(destinations) == 0 || len(sources) > maxMovePaths {
		return nil
	}
	unique := uniqueTextMoveDestinations(sources, destinations)
	if len(unique) == 0 {
		return nil
	}
	sourceCounts, destinationCounts := make(map[string]int), make(map[string]int)
	byPath := make(map[string]Change, len(destinations))
	for _, change := range sources {
		sourceCounts[change.HeadOID]++
	}
	for _, change := range destinations {
		byPath[change.Path] = change
		destinationCounts[change.IndexOID]++
	}
	var pairs []WorkingMove
	oids := make(map[string]bool)
	for _, source := range sources {
		destination, ok := byPath[unique[path.Base(source.Path)]]
		if !ok || !regularMoveMode(source.HeadMode) || !regularMoveMode(destination.IndexMode) ||
			!validOID(source.HeadOID) || !validOID(destination.IndexOID) || nonzeroOID(source.HeadOID) == "" || nonzeroOID(destination.IndexOID) == "" ||
			source.HeadOID == destination.IndexOID || sourceCounts[source.HeadOID] != 1 || destinationCounts[destination.IndexOID] != 1 {
			continue
		}
		pairs = append(pairs, WorkingMove{Source: source, Destination: destination, stagedHead: status.HeadOID, matchedText: true})
		oids[source.HeadOID], oids[destination.IndexOID] = true, true
	}
	if len(pairs) == 0 {
		return nil
	}
	if r.stagedMoveCache != nil {
		if previous := r.stagedMoveCache.Load(); previous != nil && slices.Equal(pairs, previous.candidates) {
			return slices.Clone(previous.moves)
		}
	}
	moves, err := r.matchStagedMoveBlobs(ctx, pairs, slices.Sorted(maps.Keys(oids)))
	if err != nil || ctx.Err() != nil {
		return nil
	}
	if r.stagedMoveCache != nil {
		r.stagedMoveCache.Store(&stagedMoveScan{candidates: pairs, moves: slices.Clone(moves)})
	}
	return moves
}

func (r *Repository) matchStagedMoveBlobs(ctx context.Context, pairs []WorkingMove, ids []string) ([]WorkingMove, error) {
	output, err := r.readLimited(ctx, strings.Join(ids, "\n")+"\n", int64(len(ids))*128, "cat-file", "--batch-check=%(objecttype) %(objectsize)")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
	if len(lines) != len(ids) {
		return nil, fmt.Errorf("invalid Git blob sizes in staged move scan")
	}
	sizes := make(map[string]int64, len(ids))
	for i, line := range lines {
		text, ok := strings.CutPrefix(line, "blob ")
		size, err := strconv.ParseInt(text, 10, 64)
		if !ok || err != nil || size < 0 {
			return nil, fmt.Errorf("a staged move blob is unavailable")
		}
		if size > 0 && size <= previewBytes {
			sizes[ids[i]] = size
		}
	}
	requested := make(map[string]int64)
	for _, move := range pairs {
		source, destination := sizes[move.Source.HeadOID], sizes[move.Destination.IndexOID]
		if source > 0 && destination > 0 {
			requested[move.Source.HeadOID], requested[move.Destination.IndexOID] = source, destination
		}
	}
	blobs, err := readMoveBlobs(ctx, r.readLimited, requested)
	if err != nil {
		return nil, err
	}
	var result []WorkingMove
	for _, move := range pairs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if matchesMoveText(blobs[move.Source.HeadOID], blobs[move.Destination.IndexOID]) {
			result = append(result, move)
		}
	}
	return result, nil
}

func regularMoveMode(mode string) bool {
	return mode == "100644" || mode == "100755"
}
