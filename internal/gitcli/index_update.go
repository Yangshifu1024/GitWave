package gitcli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

func (g *workingGit) stageSelection(ctx context.Context, add, remove []string, gitlinks map[string]string, moves ...WorkingMove) error {
	root, err := os.OpenRoot(g.repo.WorkTree)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, path := range remove {
		if err := checkRemovedWorkingFile(root, path); err != nil {
			return err
		}
	}
	if len(gitlinks) != 0 {
		add, err = g.stageGitlinks(ctx, root, add, gitlinks)
		if err != nil {
			return err
		}
	}
	if len(add) > 0 {
		if err := g.snapshotStageFiles(ctx, root, add); err != nil {
			return err
		}
		if len(moves) != 0 {
			if err := g.verifyMoveDestinations(ctx, moves); err != nil {
				return err
			}
		}
		if err := g.freezeWorktreeAttributes(ctx); err != nil {
			return err
		}
		if err := g.configureLocalLFS(true, false); err != nil {
			return err
		}
		if err := g.checkWorktreeFilters(ctx, add, "staging", false); err != nil {
			return err
		}
	}
	if len(remove) > 0 {
		if _, err := g.run(ctx, nulPaths(remove), 64<<10, "update-index", "--force-remove", "-z", "--stdin"); err != nil {
			return err
		}
	}
	if len(add) == 0 {
		return nil
	}
	pathspecs, err := g.stagePathspecs(add)
	if err != nil {
		return err
	}
	// check-attr uses these same words both for its states and for
	// identically named drivers. Required drivers with no commands make
	// Git reject the string form without running anything.
	g.settings = append(g.settings, "-c", "filter.set.required=true", "-c", "filter.unset.required=true",
		"-c", "filter.unspecified.required=true")
	_, err = g.run(ctx, nulPaths(pathspecs), 64<<10, "add", "--all", "--pathspec-from-file=-", "--pathspec-file-nul")
	return err
}

func (g *workingGit) stageGitlinks(ctx context.Context, root *os.Root, paths []string, expected map[string]string) ([]string, error) {
	var ordinary []string
	var records strings.Builder
	for _, path := range paths {
		want, gitlink := expected[path]
		if !gitlink {
			ordinary = append(ordinary, path)
			continue
		}
		child, err := g.repo.openSubmodule(ctx, root, path)
		if err != nil {
			return nil, err
		}
		if child == nil {
			return nil, fmt.Errorf("the submodule checkout is missing: %q", path)
		}
		head, err := child.optionalRef(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
		if err != nil {
			return nil, err
		}
		if !validOID(head) || len(head) != g.oidSize || (want != "" && head != want) {
			return nil, fmt.Errorf("the submodule commit changed before staging: %q", path)
		}
		// Clear any unmerged stages before writing the selected gitlink.
		fmt.Fprintf(&records, "0 %s\t%s\x00160000 %s\t%s\x00", strings.Repeat("0", g.oidSize), path, head, path)
	}
	if records.Len() != 0 {
		if _, err := g.run(ctx, records.String(), 64<<10, "update-index", "-z", "--index-info"); err != nil {
			return nil, err
		}
	}
	return ordinary, nil
}

func checkRemovedWorkingFile(root *os.Root, path string) error {
	info, err := root.Lstat(filepath.FromSlash(path))
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) || (err == nil && info.IsDir()) {
		return nil
	}
	return fmt.Errorf("the removed working file has changed: %q", path)
}

func (g *workingGit) stagePathspecs(paths []string) ([]string, error) {
	root, err := os.OpenRoot(g.workTree)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	specs := make([]string, 0, 2*len(paths))
	for _, path := range paths {
		info, err := root.Lstat(filepath.FromSlash(path))
		if err != nil {
			return nil, err
		}
		specs = append(specs, ":(top,literal)"+path)
		// Git rejects even an excluded pathspec beyond a symlink.
		if info.Mode()&os.ModeSymlink == 0 {
			specs = append(specs, ":(top,literal,exclude)"+path+"/")
		}
	}
	return specs, nil
}

func (g *workingGit) refreshResolvedStats(ctx context.Context, paths []string) error {
	// Git's abort checks the live worktree against index stat information.
	// The staged object has already been captured from the private snapshot.
	live := *g
	live.workTree = g.repo.WorkTree
	if len(live.lfsEnvironment) != 0 {
		live.lfsEnvironment = append(slices.Clone(live.lfsEnvironment), lfsFilterStage+"=")
	}
	pathspecs, err := live.stagePathspecs(paths)
	if err != nil {
		return err
	}
	_, err = live.run(ctx, nulPaths(pathspecs), 64<<10, "add", "--refresh", "--sparse", "--pathspec-from-file=-", "--pathspec-file-nul")
	return err
}

func (g *workingGit) refreshStagedLFSStats(ctx context.Context, paths []string) {
	if len(g.lfsEnvironment) == 0 || len(paths) == 0 {
		return
	}
	values, err := g.filterValues(ctx, paths)
	if err != nil {
		return
	}
	var assets []string
	for i, value := range values {
		if value == "lfs" {
			assets = append(assets, paths[i])
		}
	}
	if len(assets) != 0 {
		// A concurrent edit can prevent recording stat data for a path.
		// The private index still holds the staged pointer in that case.
		_ = g.refreshResolvedStats(ctx, assets)
	}
}

func (g *workingGit) checkWorktreeFilters(ctx context.Context, paths []string, action string, cached bool) error {
	var flags []string
	if cached {
		flags = append(flags, "--cached")
	}
	return g.checkFilters(ctx, paths, action, flags...)
}

func (g *workingGit) checkFilters(ctx context.Context, paths []string, action string, flags ...string) error {
	values, err := g.filterValues(ctx, paths, flags...)
	if err != nil {
		return err
	}
	for i, value := range values {
		if value != "unspecified" && value != "unset" && value != "set" && !(value == "lfs" && len(g.lfsEnvironment) != 0) {
			return fmt.Errorf("%s %q requires the external Git filter %q", action, paths[i], value)
		}
	}
	return nil
}

func (g *workingGit) filterValues(ctx context.Context, paths []string, flags ...string) ([]string, error) {
	args := append([]string{"check-attr", "-z", "--stdin"}, flags...)
	output, err := g.run(ctx, nulPaths(paths), 32<<20, append(args, "filter")...)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(string(output), "\x00")
	if len(fields) != len(paths)*3+1 || fields[len(fields)-1] != "" {
		return nil, fmt.Errorf("invalid Git filter information")
	}
	values := make([]string, len(paths))
	for i, path := range paths {
		name, attribute, value := fields[3*i], fields[3*i+1], fields[3*i+2]
		if name != path || attribute != "filter" {
			return nil, fmt.Errorf("invalid Git filter information for %q", path)
		}
		values[i] = value
	}
	return values, nil
}

func (g *workingGit) unstageSelection(ctx context.Context, head string, paths []string) error {
	removed := "0 " + strings.Repeat("0", g.oidSize) + "\t"
	var records strings.Builder
	for _, path := range paths {
		records.WriteString(removed)
		records.WriteString(path)
		records.WriteByte(0)
	}
	if head != "" {
		wanted := make(map[string]bool, len(paths))
		for _, path := range paths {
			wanted[path] = true
		}
		tree, err := g.read(ctx, 128<<20, "ls-tree", "-r", "-z", "--full-tree", head)
		if err != nil {
			return err
		}
		for record := range strings.SplitSeq(strings.TrimSuffix(string(tree), "\x00"), "\x00") {
			if record == "" {
				continue
			}
			meta, path, found := strings.Cut(record, "\t")
			if !found {
				return fmt.Errorf("invalid Git tree entry")
			}
			if !wanted[path] {
				continue
			}
			fields := strings.Fields(meta)
			if len(fields) != 3 || !validOID(fields[2]) {
				return fmt.Errorf("invalid Git tree entry for %q", path)
			}
			fmt.Fprintf(&records, "%s %s\t%s\x00", fields[0], fields[2], path)
		}
	}
	_, err := g.run(ctx, records.String(), 64<<10, "update-index", "-z", "--index-info")
	return err
}

func (g *workingGit) otherIndexEntries(ctx context.Context, paths []string, moves ...WorkingMove) ([]byte, error) {
	selected := make(map[string]bool, len(paths))
	for _, path := range paths {
		selected[path] = true
	}
	sources := make(map[string]bool, len(moves))
	destinations := make(map[string]WorkingMove, len(moves))
	for _, move := range moves {
		sources[move.Source.Path] = true
		destinations[move.Destination.Path] = move
	}
	output, err := g.read(ctx, 128<<20, "ls-files", "--stage", "-z", "--")
	if err != nil {
		return nil, err
	}
	var unchanged []byte
	for record := range strings.SplitSeq(strings.TrimSuffix(string(output), "\x00"), "\x00") {
		if record == "" {
			continue
		}
		meta, path, found := strings.Cut(record, "\t")
		if !found {
			return nil, fmt.Errorf("invalid Git index entry")
		}
		if !selected[path] {
			unchanged = append(unchanged, record...)
			unchanged = append(unchanged, 0)
		}
		if sources[path] {
			return nil, fmt.Errorf("the move source is still in the index: %q", path)
		}
		if move, ok := destinations[path]; ok {
			fields := strings.Fields(meta)
			if len(fields) != 3 || fields[1] != move.cleanOID || fields[2] != "0" ||
				(fields[0] == "120000") != (move.snapshot.mode&os.ModeSymlink != 0) {
				return nil, fmt.Errorf("the Git conversion for the moved file has changed: %q", path)
			}
			delete(destinations, path)
		}
	}
	if len(destinations) != 0 {
		return nil, fmt.Errorf("a move destination was not added to the index")
	}
	return unchanged, nil
}

func nulPaths(paths []string) string { return strings.Join(paths, "\x00") + "\x00" }
