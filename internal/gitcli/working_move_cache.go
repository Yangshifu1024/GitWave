package gitcli

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type moveAttribute struct {
	path string
	hash [sha256.Size]byte
}

type workingMoveScan struct {
	git            os.FileInfo
	workTree       string
	gitDir         string
	head           string
	settings       []string
	sources        []Change
	destinations   []WorkingMove
	textCandidates []WorkingMove
	attributes     []moveAttribute
	infoAttrs      [sha256.Size]byte
	moves          []WorkingMove
}

func (g *workingGit) moveAttributePaths(ctx context.Context) []string {
	output, err := g.read(ctx, 8<<10, "var", "GIT_ATTR_SYSTEM")
	system := strings.TrimSuffix(string(output), "\n")
	if err != nil || system == "" {
		return nil
	}
	global, err := g.globalAttributesPath(ctx)
	if err != nil {
		return nil
	}
	return []string{system, global}
}

// Cache only a comparison with fully captured attributes. Check the external
// paths just before comparison and freeze the observed absence for Git.
func (g *workingGit) moveScan(head string, sources []Change, paths []string, candidates map[string]WorkingMove, textCandidates []WorkingMove, attrs []moveAttribute, externalPaths []string) *workingMoveScan {
	if len(externalPaths) == 0 {
		return nil
	}
	for _, path := range externalPaths {
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) || strings.ContainsAny(path, "\r\n") {
			return nil
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			return nil
		}
	}
	git, err := os.Stat(g.repo.git)
	if err != nil || !git.Mode().IsRegular() {
		return nil
	}
	infoAttrs, err := os.ReadFile(filepath.Join(g.dir, "info", "attributes"))
	if err != nil && !os.IsNotExist(err) {
		return nil
	}
	destinations := make([]WorkingMove, 0, len(paths))
	for _, path := range paths {
		destinations = append(destinations, candidates[path])
	}
	// Keep the observed absence stable during Git's comparison.
	g.settings = append(g.settings, "-c", "core.attributesfile="+os.DevNull)
	g.frozenAttributes = true
	return &workingMoveScan{
		git: git, workTree: g.repo.WorkTree, gitDir: g.repo.GitDir, head: head, settings: slices.Clone(g.settings), sources: sources,
		destinations: destinations, textCandidates: textCandidates, attributes: attrs, infoAttrs: sha256.Sum256(infoAttrs),
	}
}

func (s *workingMoveScan) matches(previous *workingMoveScan) bool {
	return previous != nil && sameDiscardMetadata(s.git, previous.git) && s.workTree == previous.workTree &&
		s.gitDir == previous.gitDir && s.head == previous.head &&
		slices.Equal(s.settings, previous.settings) && slices.Equal(s.sources, previous.sources) &&
		slices.Equal(s.destinations, previous.destinations) && slices.Equal(s.attributes, previous.attributes) &&
		slices.Equal(s.textCandidates, previous.textCandidates) &&
		s.infoAttrs == previous.infoAttrs
}

func (r *Repository) rememberMoveScan(ctx context.Context, scan *workingMoveScan, moves []WorkingMove) {
	if scan != nil && ctx.Err() == nil {
		scan.moves = slices.Clone(moves)
		r.workingMoveCache.Store(scan)
	}
}
