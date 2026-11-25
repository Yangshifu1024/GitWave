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
	"strings"
)

type MergeAbortPlan struct {
	repo     *Repository
	local    CheckoutTarget
	state    CommitState
	original string
	autoTree string
	index    string
	undo     string
	paths    []string
	skip     map[string]bool
	groups   []abortGroup
	stateDir abortGroup
}

type abortIndex struct {
	files    map[string]string
	modes    map[string]string
	symlinks bool
}

var abortStateNames = []string{"MERGE_MSG", "MERGE_MODE", "MERGE_RR", "MERGE_HEAD"}

// PrepareMergeAbort snapshots the operation, index and affected worktree
// paths shown for confirmation. Independent staged entries stay in the index.
func (r *Repository) PrepareMergeAbort(ctx context.Context) (*MergeAbortPlan, error) {
	if r.WorkTree == "" {
		return nil, fmt.Errorf("this repository has no working copy")
	}
	status, err := r.Status(ctx)
	if err != nil {
		return nil, err
	}
	if !validOID(status.HeadOID) || status.Commit.MergeHeads == "" {
		return nil, fmt.Errorf("there is no merge in progress")
	}
	if status.Commit.Continuation != "" && status.Commit.Continuation != mergeAutostashOperation {
		return nil, fmt.Errorf("cannot abort this merge while %s is active", status.Commit.Continuation)
	}
	p := &MergeAbortPlan{repo: r, local: CheckoutTarget{Ref: status.HeadRef, OID: status.HeadOID}, state: status.Commit}
	if p.SavesAutostash() {
		snapshot, err := r.readCommitHeader(ctx, p.state.MergeAutostash)
		if err != nil || len(snapshot.parents) != 2 || snapshot.parents[0] != p.local.OID {
			return nil, errors.Join(err, fmt.Errorf("MERGE_AUTOSTASH is not a snapshot of this checkout"))
		}
	}
	if p.original, err = r.exactRootRef(ctx, "ORIG_HEAD"); err != nil {
		return nil, err
	}
	if p.original != "" && p.original != p.local.OID {
		return nil, fmt.Errorf("HEAD has changed since the merge started")
	}
	if p.autoTree, err = r.exactRootRef(ctx, "AUTO_MERGE"); err != nil {
		return nil, err
	}
	git, err := r.workingGit(ctx, p.local.OID)
	if err != nil {
		return nil, err
	}
	defer git.close()
	if p.autoTree != "" {
		kind, err := git.read(ctx, 64, "cat-file", "-t", p.autoTree)
		if err != nil || strings.TrimSpace(string(kind)) != "tree" {
			return nil, errors.Join(err, fmt.Errorf("AUTO_MERGE does not name a Git tree"))
		}
	}
	gitDir, err := os.OpenRoot(r.GitDir)
	if err != nil {
		return nil, err
	}
	defer gitDir.Close()
	if p.stateDir, err = inspectMergeState(ctx, gitDir); err != nil {
		return nil, err
	}
	if err := p.planPaths(ctx, git, status); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(r.WorkTree)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := p.planGroups(ctx, root); err != nil {
		return nil, err
	}
	if err := p.verify(ctx, git, gitDir); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *MergeAbortPlan) SavesAutostash() bool { return p.state.MergeAutostash != "" }

func (p *MergeAbortPlan) mergePaths(ctx context.Context, git *workingGit) (map[string]string, error) {
	tree := p.autoTree
	_, gitMerge := p.stateDir.before["MERGE_MODE"]
	if tree == "" && !gitMerge && p.original == p.local.OID {
		// Older native builds saved only MERGE_HEAD and MERGE_MSG. Their
		// ort preview can be replayed from the original commit IDs.
		if err := copyGitMetadata(filepath.Join(p.repo.CommonDir, "shallow"), filepath.Join(git.dir, "shallow"), 16<<20); err != nil {
			return nil, err
		}
		head := p.state.MergeHeads
		if !validOID(head) {
			return nil, fmt.Errorf("invalid native merge head")
		}
		out, err := git.run(ctx, p.local.OID+" "+head+"\n", 32<<20, "merge-tree", "--write-tree", "--stdin", "-z")
		if err != nil {
			return nil, fmt.Errorf("could not identify the changes from this merge: %w", err)
		}
		var preview MergePlan
		if err := preview.readPreview(out); err != nil {
			return nil, err
		}
		tree = preview.tree
	}
	result := make(map[string]string)
	if tree == "" {
		return result, nil
	}
	output, err := git.read(ctx, 64<<20, "diff-tree", "-r", "--raw", "--no-abbrev", "--no-renames", "--no-commit-id", "-z", p.local.OID, tree, "--")
	if err != nil {
		return nil, err
	}
	records := strings.SplitSeq(strings.TrimSuffix(string(output), "\x00"), "\x00")
	var entry []string
	for field := range records {
		if entry == nil {
			if field == "" && len(output) == 0 {
				break
			}
			entry = strings.Fields(field)
			if len(entry) != 5 || !strings.HasPrefix(entry[0], ":") {
				return nil, fmt.Errorf("invalid merge tree entry")
			}
			continue
		}
		result[field] = ""
		if entry[1] != "000000" {
			result[field] = entry[1] + " " + entry[3]
		}
		entry = nil
	}
	if entry != nil {
		return nil, fmt.Errorf("invalid merge tree paths")
	}
	return result, nil
}

func (p *MergeAbortPlan) planPaths(ctx context.Context, git *workingGit, status Status) error {
	index, err := git.read(ctx, 128<<20, "ls-files", "--stage", "-v", "-z", "--")
	if err != nil {
		return err
	}
	p.index = string(index)
	owned, err := p.mergePaths(ctx, git)
	if err != nil {
		return err
	}
	conflicts := make(map[string]bool)
	for _, change := range status.Changes {
		if change.Kind == 'u' {
			conflicts[change.Path] = true
		}
	}
	undo, err := git.read(ctx, 32<<20, "ls-files", "--resolve-undo", "-z", "--")
	if err != nil {
		return err
	}
	p.undo = string(undo)
	_, gitMerge := p.stateDir.before["MERGE_MODE"]
	knownTree := p.autoTree != "" || (!gitMerge && p.original == p.local.OID)
	if knownTree {
		for record := range strings.SplitSeq(p.undo, "\x00") {
			if _, name, ok := strings.Cut(record, "\t"); ok {
				conflicts[name] = true
			}
		}
	}
	stage := make(map[string]string)
	for record := range strings.SplitSeq(p.index, "\x00") {
		meta, name, ok := strings.Cut(record, "\t")
		if ok && len(meta) > 4 && strings.HasSuffix(meta, " 0") {
			stage[name] = meta[2 : len(meta)-2]
		}
	}
	staged, err := git.read(ctx, 32<<20, "diff", "--cached", "--name-only", "--no-renames", "--no-ext-diff", "-z", p.local.OID, "--")
	if err != nil {
		return err
	}
	for name := range strings.SplitSeq(string(staged), "\x00") {
		expected, merged := owned[name]
		if name != "" && !conflicts[name] && !knownTree {
			return fmt.Errorf("the merge has no recorded result tree to distinguish staged merge changes from independent edits")
		}
		if merged || conflicts[name] {
			if !validIndexPath(name) {
				return fmt.Errorf("invalid merge path %q", name)
			}
			if !conflicts[name] && stage[name] != expected {
				return fmt.Errorf("staged edits overlap the merge at %q", name)
			}
			p.paths = append(p.paths, name)
		}
	}
	slices.Sort(p.paths)
	p.paths = slices.Compact(p.paths)
	p.skip, err = git.abortSparsePaths(ctx, p.paths)
	return err
}

func (g *workingGit) abortSparsePaths(ctx context.Context, paths []string) (map[string]bool, error) {
	excluded := make(map[string]bool)
	enabled, err := g.repo.optionalRef(ctx, "config", "--type=bool", "--get", "core.sparsecheckout")
	if err != nil || enabled != "true" || len(paths) == 0 {
		return excluded, err
	}
	matches, err := g.repo.readLimited(ctx, nulPaths(paths), 32<<20, "sparse-checkout", "check-rules", "-z")
	if err != nil {
		return nil, err
	}
	included := make(map[string]bool)
	for name := range strings.SplitSeq(string(matches), "\x00") {
		included[name] = true
	}
	for _, name := range paths {
		if !included[name] {
			excluded[name] = true
		}
	}
	return excluded, nil
}

func (p *MergeAbortPlan) planGroups(ctx context.Context, root *os.Root) error {
	tracked := make(map[string]bool)
	gitlinks := make(map[string]bool)
	hidden := make(map[string]bool)
	for entry := range strings.SplitSeq(p.index, "\x00") {
		meta, name, ok := strings.Cut(entry, "\t")
		if !ok {
			continue
		}
		tracked[name] = true
		if _, selected := slices.BinarySearch(p.paths, name); selected && len(meta) > 2 && meta[0] >= 'a' && meta[0] <= 'z' {
			return fmt.Errorf("cannot verify local edits to an assume-unchanged merge path: %q", name)
		}
		if strings.Contains(meta, " 160000 ") {
			gitlinks[name] = true
		}
		if strings.HasPrefix(meta, "S ") {
			hidden[name] = true
		}
	}
	groups := make(map[string][]string)
	for _, name := range p.paths {
		if gitlinks[name] {
			continue
		}
		group := name
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if _, ok := slices.BinarySearch(p.paths, parent); ok {
				group = parent
			}
		}
		groups[group] = append(groups[group], name)
	}
	for group, names := range groups {
		entry, err := inspectAbortGroup(ctx, root, group, names)
		if err != nil {
			return err
		}
		for name, item := range entry.before {
			if hidden[name] {
				return fmt.Errorf("cannot verify local edits to a materialized skip-worktree merge path: %q", name)
			}
			owned := tracked[name] && slices.Contains(names, name)
			if item.info.IsDir() {
				owned = slices.ContainsFunc(names, func(file string) bool { return strings.HasPrefix(file, name+"/") })
			}
			if !owned {
				return fmt.Errorf("aborting the merge would replace an independent file: %q", name)
			}
		}
		p.groups = append(p.groups, entry)
	}
	slices.SortFunc(p.groups, func(a, b abortGroup) int { return strings.Compare(a.path, b.path) })
	return nil
}

func (p *MergeAbortPlan) verify(ctx context.Context, git *workingGit, gitDir *os.Root) error {
	if !p.repo.headMatches(ctx, p.local.Ref, p.local.OID) {
		return fmt.Errorf("HEAD changed since the merge was shown")
	}
	state, err := p.repo.readCommitState(ctx, gitDir)
	if err != nil {
		return err
	}
	if state != p.state {
		return fmt.Errorf("the merge changed since it was shown")
	}
	for _, ref := range []struct{ name, oid string }{{"ORIG_HEAD", p.original}, {"AUTO_MERGE", p.autoTree}, {"MERGE_AUTOSTASH", p.state.MergeAutostash}} {
		current, err := p.repo.exactRootRef(ctx, ref.name)
		if err != nil || current != ref.oid {
			return errors.Join(err, fmt.Errorf("%s changed since the merge was shown", ref.name))
		}
	}
	index, err := git.read(ctx, 128<<20, "ls-files", "--stage", "-v", "-z", "--")
	if err != nil {
		return err
	}
	if string(index) != p.index {
		return fmt.Errorf("the Git index changed since the merge was shown")
	}
	undo, err := git.read(ctx, 32<<20, "ls-files", "--resolve-undo", "-z", "--")
	if err != nil || string(undo) != p.undo {
		return errors.Join(err, fmt.Errorf("the conflict resolution index changed since the merge was shown"))
	}
	if len(p.paths) != 0 {
		status, err := git.checkoutStatus(ctx)
		if err != nil {
			return err
		}
		for _, change := range status.Changes {
			_, affected := slices.BinarySearch(p.paths, change.Path)
			if affected && change.Kind != 'u' && change.Kind != '?' && change.WorkTree != '.' {
				return fmt.Errorf("unstaged edits overlap the merge at %q", change.Path)
			}
		}
	}
	skip, err := git.abortSparsePaths(ctx, p.paths)
	if err != nil || !maps.Equal(p.skip, skip) {
		return errors.Join(err, fmt.Errorf("sparse checkout changed since the merge was shown"))
	}
	metadata, err := inspectMergeState(ctx, gitDir)
	if err != nil || !matchingAbortFiles(p.stateDir.before, metadata.before) {
		return errors.Join(err, fmt.Errorf("merge state files changed since the merge was shown"))
	}
	return ctx.Err()
}

func (p *MergeAbortPlan) prepareIndex(ctx context.Context, lock *checkoutIndexLock, root *os.Root, git *workingGit) (abortIndex, error) {
	git.index = filepath.Join(git.dir, "index")
	git.settings = append(git.settings, "-c", "core.splitIndex=false")
	if err := copyOriginalIndex(lock.root, git.index); err != nil {
		return abortIndex{}, err
	}
	before, err := git.otherIndexEntries(ctx, p.paths)
	if err != nil {
		return abortIndex{}, err
	}
	if len(p.paths) != 0 {
		if err := git.unstageSelection(ctx, p.local.OID, p.paths); err != nil {
			return abortIndex{}, err
		}
	}
	if _, err := git.read(ctx, 64<<10, "update-index", "--clear-resolve-undo"); err != nil {
		return abortIndex{}, err
	}
	after, err := git.otherIndexEntries(ctx, p.paths)
	if err != nil || !bytes.Equal(before, after) {
		return abortIndex{}, errors.Join(err, fmt.Errorf("the abort would change an independent index entry"))
	}
	if err := git.isolateWorkTree(); err != nil {
		return abortIndex{}, err
	}
	var renderPaths, skipPaths, allPaths []string
	result := abortIndex{modes: make(map[string]string)}
	grouped := make(map[string]bool, len(p.paths))
	var replacingAttributes []os.FileInfo
	for _, group := range p.groups {
		for _, name := range group.files {
			grouped[name] = true
		}
		for name, entry := range group.before {
			if path.Base(name) == ".gitattributes" {
				replacingAttributes = append(replacingAttributes, entry.info)
			}
		}
	}
	index, err := git.read(ctx, 128<<20, "ls-files", "--stage", "-z", "--")
	if err != nil {
		return abortIndex{}, err
	}
	for entry := range strings.SplitSeq(string(index), "\x00") {
		meta, name, ok := strings.Cut(entry, "\t")
		if entry == "" {
			continue
		}
		if !ok || !strings.HasSuffix(meta, " 0") {
			return abortIndex{}, fmt.Errorf("the abort would leave an unmerged index entry at %q", name)
		}
		allPaths = append(allPaths, name)
		if _, ok := slices.BinarySearch(p.paths, name); !ok || strings.HasPrefix(meta, "160000 ") {
			continue
		}
		if !grouped[name] {
			return abortIndex{}, fmt.Errorf("aborting the merge would replace a submodule at %q", name)
		}
		if p.skip[name] {
			skipPaths = append(skipPaths, name)
		} else {
			renderPaths = append(renderPaths, name)
			result.modes[name], _, _ = strings.Cut(meta, " ")
		}
	}
	if err := checkCheckoutAliases(root, allPaths); err != nil {
		return abortIndex{}, err
	}
	if len(skipPaths) != 0 {
		if _, err := git.run(ctx, nulPaths(skipPaths), 64<<10, "update-index", "--skip-worktree", "-z", "--stdin"); err != nil {
			return abortIndex{}, err
		}
	}
	if len(renderPaths) != 0 {
		shadow, err := os.OpenRoot(git.workTree)
		if err != nil {
			return abortIndex{}, err
		}
		defer shadow.Close()
		result.files, result.symlinks, err = git.renderIndexFiles(ctx, root, shadow, renderPaths, nil, func(info os.FileInfo) bool {
			return slices.ContainsFunc(replacingAttributes, func(before os.FileInfo) bool { return os.SameFile(info, before) })
		})
		if err != nil {
			return abortIndex{}, err
		}
	}
	return result, nil
}
