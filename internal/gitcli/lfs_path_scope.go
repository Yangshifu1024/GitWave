package gitcli

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/git-lfs/wildmatch/v2"
)

type lfsConfigSource struct {
	target, targetPath, original, restoredPath string
	cleanSource, useIndex                      bool
}

type lfsPathScope struct {
	source           lfsConfigSource
	include, exclude string
	foldCase         bool
}

func (r *Repository) lfsConfigAfterRestore(ctx context.Context, files []discardFile, head string, useIndex bool) (lfsConfigSource, error) {
	var paths []string
	for _, file := range files {
		for _, name := range []string{file.change.Path, file.change.PreviousPath} {
			if strings.EqualFold(name, ".lfsconfig") && !slices.Contains(paths, name) {
				paths = append(paths, name)
			}
		}
	}
	names, err := r.lfsConfigCandidates(paths)
	if err != nil || len(names) == 0 {
		return lfsConfigSource{}, err
	}
	path := names[0]
	if len(names) > 1 && (head != "" || useIndex) {
		args := []string{"ls-tree", "--name-only", "-z", head, "--"}
		if useIndex {
			args = []string{"ls-files", "--cached", "-z", "--"}
		}
		data, err := r.readLimited(ctx, "", 4096, append(args, names...)...)
		if err != nil {
			return lfsConfigSource{}, err
		}
		if len(data) != 0 {
			path = strings.TrimSuffix(string(data), "\x00")
			if !slices.Contains(names, path) {
				return lfsConfigSource{}, fmt.Errorf("multiple .lfsconfig paths refer to the same working file")
			}
		}
	}
	return lfsConfigSource{original: head, restoredPath: path, cleanSource: !useIndex, useIndex: useIndex}, nil
}

func (r *Repository) lfsConfigCandidates(paths []string) ([]string, error) {
	var candidates []string
	caseAlias := false
	for _, name := range paths {
		if strings.EqualFold(name, ".lfsconfig") {
			candidates = append(candidates, name)
			caseAlias = caseAlias || name != ".lfsconfig"
		}
	}
	if caseAlias {
		root, err := os.OpenRoot(r.WorkTree)
		if err != nil {
			return nil, err
		}
		insensitive, _, err := probeCheckoutNames(root)
		root.Close()
		if err != nil {
			return nil, err
		}
		if !insensitive {
			candidates = slices.DeleteFunc(candidates, func(name string) bool { return name != ".lfsconfig" })
		}
	}
	return candidates, nil
}

func (g *workingGit) checkoutLFSConfigPath(ctx context.Context, tree string, changed []string) (string, error) {
	if tree == "" {
		return "", nil
	}
	candidates, err := g.repo.lfsConfigCandidates(changed)
	if err != nil || len(candidates) == 0 {
		return "", err
	}
	data, err := g.read(ctx, 4096, append([]string{"ls-tree", "--name-only", "-z", tree, "--"}, candidates...)...)
	if err != nil || len(data) == 0 {
		return "", err
	}
	name := strings.TrimSuffix(string(data), "\x00")
	if !slices.Contains(candidates, name) {
		return "", fmt.Errorf("multiple .lfsconfig paths refer to the same working file")
	}
	sparse, err := g.read(ctx, 32, "config", "--type=bool", "--default=false", "--get", "core.sparsecheckout")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(sparse)) == "true" {
		inScope, err := g.run(ctx, name+"\x00", 4096, "sparse-checkout", "check-rules", "-z")
		if err != nil || len(inScope) == 0 {
			return "", err
		}
	}
	return name, nil
}

func (r *Repository) lfsPathScope(ctx context.Context, source lfsConfigSource) (lfsPathScope, error) {
	keys := []string{"lfs.fetchinclude", "lfs.fetchexclude"}
	values, err := r.lfsConfigValuesAt(ctx, source, keys)
	if err != nil {
		return lfsPathScope{}, err
	}
	scope := lfsPathScope{source: source, include: strings.TrimSpace(values[keys[0]]), exclude: strings.TrimSpace(values[keys[1]])}
	if scope.include != "" || scope.exclude != "" {
		value, err := r.optionalRef(ctx, "config", "--type=bool", "--get", "core.ignorecase")
		if err != nil {
			return lfsPathScope{}, err
		}
		scope.foldCase = value == "true"
	}
	return scope, nil
}

func (g *workingGit) checkoutLFSTarget(ctx context.Context, config lfsConfigSource, paths []string,
	attributeTree string, receipt *lfsDownloadReceipt) (*workingGit, error) {
	if len(paths) == 0 || len(g.lfsEnvironment) == 0 || g.localLFS.skipSmudge {
		return g, nil
	}
	flag := "--cached"
	if attributeTree != "" {
		flag = "--source=" + attributeTree
	}
	filters, err := g.filterValues(ctx, paths, flag)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(filters, "lfs") {
		// A local .gitattributes can supply rules not present in either tree.
		filters, err = g.filterValues(ctx, paths)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(filters, "lfs") {
			return g, nil
		}
	}
	projectPath, err := g.checkoutLFSConfigPath(ctx, attributeTree, paths)
	if err != nil {
		return nil, err
	}
	if projectPath != "" {
		config.target, config.targetPath = attributeTree, projectPath
	}
	scope, err := g.repo.lfsPathScope(ctx, config)
	if err != nil {
		return nil, err
	}
	allowed, err := scope.matcher()
	if err != nil {
		return nil, err
	}
	if receipt != nil && receipt.filtered {
		matchesScope := allowed
		allowed = func(path string) bool {
			_, downloaded := receipt.paths[path]
			return downloaded && (matchesScope == nil || matchesScope(path))
		}
	}
	target, err := g.restrictLFSCheckout(paths, allowed)
	if err == nil {
		target.lfsCheckoutScope = &scope
	}
	return target, err
}

func (g *workingGit) verifyLFSPathScope(ctx context.Context) error {
	if g.lfsCheckoutScope == nil {
		return nil
	}
	current, err := g.repo.lfsPathScope(ctx, g.lfsCheckoutScope.source)
	if err != nil {
		return err
	}
	if current != *g.lfsCheckoutScope {
		return fmt.Errorf("the Git LFS path filters changed during checkout")
	}
	return nil
}

func (scope lfsPathScope) matcher() (func(string) bool, error) {
	if scope.include == "" && scope.exclude == "" {
		return nil, nil
	}
	includes, err := scope.patterns(scope.include)
	if err != nil {
		return nil, err
	}
	excludes, err := scope.patterns(scope.exclude)
	if err != nil {
		return nil, err
	}
	return func(name string) bool {
		matches := func(pattern *wildmatch.Wildmatch) bool { return pattern.Match(name) }
		return (len(includes) == 0 || slices.ContainsFunc(includes, matches)) && !slices.ContainsFunc(excludes, matches)
	}, nil
}

func (scope lfsPathScope) patterns(list string) (patterns []*wildmatch.Wildmatch, err error) {
	if list == "" {
		return nil, nil
	}
	var pattern string
	defer func() {
		if recover() != nil {
			patterns, err = nil, fmt.Errorf("invalid Git LFS path pattern %q", pattern)
		}
	}()
	for item := range strings.SplitSeq(list, ",") {
		pattern = strings.TrimSpace(item)
		if strings.HasSuffix(pattern, "/") || strings.HasSuffix(pattern, `\`) {
			pattern = pattern[:len(pattern)-1]
		}
		if scope.foldCase {
			patterns = append(patterns, wildmatch.NewWildmatch(pattern, wildmatch.CaseFold, wildmatch.Contents))
		} else {
			patterns = append(patterns, wildmatch.NewWildmatch(pattern, wildmatch.Contents))
		}
	}
	return patterns, nil
}
