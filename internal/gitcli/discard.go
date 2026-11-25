package gitcli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// DiscardPlan binds an explicit confirmation to worktree contents and index entries.
type DiscardPlan struct {
	repo  *Repository
	shown Status
	root  os.FileInfo
	files []discardFile
}

type discardFile struct {
	change Change
	local  string
	parent os.FileInfo
	stamp  discardStamp
}

func (c Change) CanDiscard() bool {
	if !validIndexPath(c.Path) {
		return false
	}
	if c.Kind == '?' {
		return true
	}
	switch c.IndexMode {
	case "100644", "100755", "120000":
		return c.Kind != 'u' && (c.WorkTree == 'M' || c.WorkTree == 'D' || c.WorkTree == 'T')
	default:
		return false
	}
}

func (r *Repository) PrepareDiscard(ctx context.Context, shown Status, changes []Change) (*DiscardPlan, error) {
	if len(changes) == 0 {
		return nil, fmt.Errorf("select unstaged files to discard")
	}
	if r.WorkTree == "" {
		return nil, fmt.Errorf("this repository has no working copy")
	}
	if err := r.checkIndexCheckout(ctx); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(r.WorkTree)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	identity, err := root.Stat(".")
	if err != nil {
		return nil, err
	}
	plan := &DiscardPlan{repo: r, shown: Status{HeadRef: shown.HeadRef, HeadOID: shown.HeadOID, Commit: shown.Commit}, root: identity}
	seen := make(map[string]bool)
	for _, change := range changes {
		if !change.CanDiscard() || seen[change.Path] {
			return nil, fmt.Errorf("this working change cannot be discarded: %q", change.Path)
		}
		seen[change.Path] = true
		file := discardFile{change: change, local: filepath.FromSlash(change.Path)}
		file.parent, file.stamp, err = inspectDiscardFile(ctx, root, file.local)
		if err != nil {
			return nil, err
		}
		if (file.stamp.info == nil) != (change.WorkTree == 'D') {
			return nil, fmt.Errorf("the working file changed before confirmation: %q", change.Path)
		}
		plan.files = append(plan.files, file)
	}
	current, err := r.Status(ctx)
	if err != nil {
		return nil, err
	}
	if err := plan.checkStatus(current); err != nil {
		return nil, err
	}
	return plan, nil
}

func (p *DiscardPlan) checkStatus(current Status) error {
	if current.HeadRef != p.shown.HeadRef || current.HeadOID != p.shown.HeadOID || current.Commit != p.shown.Commit {
		return fmt.Errorf("HEAD or the repository operation changed before discarding")
	}
	available := make(map[Change]bool, len(current.Changes))
	for _, change := range current.Changes {
		available[change] = true
	}
	for _, file := range p.files {
		if !available[file.change] {
			return fmt.Errorf("the selected change is no longer current: %q", file.change.Path)
		}
	}
	return nil
}

func (p *DiscardPlan) Apply(ctx context.Context) error {
	repo := p.repo
	lock, err := repo.lockCheckoutIndex(ctx)
	if err != nil {
		return err
	}
	defer lock.close()
	current, err := repo.Status(ctx)
	if err != nil {
		return err
	}
	if err := p.checkStatus(current); err != nil {
		return err
	}
	root, err := os.OpenRoot(repo.WorkTree)
	if err != nil {
		return err
	}
	defer root.Close()
	identity, err := root.Stat(".")
	if err != nil || !os.SameFile(p.root, identity) {
		return fmt.Errorf("the working tree changed before discarding")
	}
	var tracked []string
	for _, file := range p.files {
		if file.change.Kind != '?' {
			tracked = append(tracked, file.change.Path)
		}
	}
	head := current.HeadOID
	if head == "(initial)" {
		head = ""
	}
	var restored *os.Root
	var paths map[string]string
	symlinks := false
	git, err := repo.workingGit(ctx, head)
	if err != nil {
		return err
	}
	defer git.close()
	git.index = filepath.Join(git.dir, "index")
	if err := copyOriginalIndex(lock.root, git.index); err != nil {
		return err
	}
	if err := p.checkPathAliases(ctx, root, git); err != nil {
		return err
	}
	if len(tracked) != 0 {
		if err := git.isolateWorkTree(); err != nil {
			return err
		}
		restored, err = os.OpenRoot(git.workTree)
		if err != nil {
			return err
		}
		defer restored.Close()
		config, err := repo.lfsConfigAfterRestore(ctx, p.files, head, true)
		if err != nil {
			return err
		}
		paths, symlinks, err = git.renderIndexFiles(ctx, root, restored, tracked, &config, nil)
		if err != nil {
			return err
		}
	}
	for _, file := range p.files {
		parent, stamp, err := inspectDiscardFile(ctx, root, file.local)
		if err != nil {
			return err
		}
		if !sameFileIdentity(parent, file.parent) || !stamp.matches(file.stamp) {
			return discardChanged(file.local)
		}
	}
	if err := repo.checkIndexCheckout(ctx); err != nil {
		return err
	}
	ref, err := repo.optionalRef(ctx, "symbolic-ref", "-q", "HEAD")
	if err != nil {
		return err
	}
	tip, err := repo.optionalRef(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return err
	}
	operation, err := p.repo.readCommitState(ctx, lock.root)
	if err != nil {
		return err
	}
	if !lock.ownsLock() || ref != p.shown.HeadRef || tip != head || operation != p.shown.Commit {
		return fmt.Errorf("the Git index, HEAD or repository operation changed before discarding")
	}
	if err := git.verifyLocalLFSConfig(ctx); err != nil {
		return err
	}
	if err := git.verifyLFSPathScope(ctx); err != nil {
		return err
	}
	for i, file := range p.files {
		if err := file.apply(ctx, root, restored, paths[file.change.Path], symlinks); err != nil {
			if i != 0 {
				return fmt.Errorf("discarded %d of %d files: %w", i, len(p.files), err)
			}
			return err
		}
	}
	return nil
}

func (p *DiscardPlan) checkPathAliases(ctx context.Context, root *os.Root, git *workingGit) error {
	fold := cases.Fold()
	key := func(path string) string { return norm.NFC.String(fold.String(path)) }
	groups := make(map[string][]discardFile)
	for _, file := range p.files {
		group := key(file.change.Path)
		for _, other := range groups[group] {
			if file.stamp.info != nil && sameFileIdentity(other.stamp.info, file.stamp.info) {
				return fmt.Errorf("the selected Git paths refer to the same working file: %q and %q", other.change.Path, file.change.Path)
			}
		}
		groups[group] = append(groups[group], file)
	}
	untracked := make(map[string][]discardFile)
	for _, file := range p.files {
		if file.change.Kind == '?' {
			group := key(file.change.Path)
			untracked[group] = append(untracked[group], file)
		}
	}
	if len(untracked) == 0 {
		return nil
	}
	output, err := git.read(ctx, 128<<20, "ls-files", "--cached", "-z")
	if err != nil {
		return err
	}
	for path := range strings.SplitSeq(strings.TrimSuffix(string(output), "\x00"), "\x00") {
		for _, file := range untracked[key(path)] {
			if !validIndexPath(path) {
				return fmt.Errorf("invalid tracked path: %q", path)
			}
			info, err := root.Lstat(filepath.FromSlash(path))
			if err == nil && os.SameFile(info, file.stamp.info) {
				return fmt.Errorf("untracked path %q refers to tracked file %q", file.change.Path, path)
			}
		}
	}
	return nil
}

func (g *workingGit) renderIndexFiles(ctx context.Context, root, target *os.Root, paths []string,
	config *lfsConfigSource, skipAttributes func(os.FileInfo) bool) (map[string]string, bool, error) {
	if err := g.snapshotWorktreeMetadata(ctx, root, target, paths, skipAttributes, ".gitattributes"); err != nil {
		return nil, false, err
	}
	if err := g.freezeWorktreeAttributes(ctx); err != nil {
		return nil, false, err
	}
	if err := g.configureLocalLFS(false, true); err != nil {
		return nil, false, err
	}
	for _, cached := range []bool{true, false} {
		if err := g.checkWorktreeFilters(ctx, paths, "restoring", cached); err != nil {
			return nil, false, err
		}
	}
	g.settings = append(g.settings, "-c", "filter.set.required=true", "-c", "filter.unset.required=true",
		"-c", "filter.unspecified.required=true")
	if config != nil {
		scoped, err := g.checkoutLFSTarget(ctx, *config, paths, "", nil)
		if err != nil {
			return nil, false, err
		}
		g.lfsCheckoutScope = scoped.lfsCheckoutScope
		return scoped.extractIndexFiles(ctx, paths)
	}
	return g.extractIndexFiles(ctx, paths)
}

func (g *workingGit) extractIndexFiles(ctx context.Context, paths []string) (map[string]string, bool, error) {
	symlinks, err := g.read(ctx, 32, "config", "--bool", "--default=true", "--get", "core.symlinks")
	if err != nil {
		return nil, false, err
	}
	// Git assigns a separate temporary name per index entry, even when this
	// private filesystem cannot represent two original names independently.
	out, err := g.run(ctx, nulPaths(paths), 64<<20, "checkout-index", "--temp", "--ignore-skip-worktree-bits", "-z", "--stdin")
	if err != nil {
		return nil, false, err
	}
	rendered := make(map[string]string, len(paths))
	for record := range strings.SplitSeq(strings.TrimSuffix(string(out), "\x00"), "\x00") {
		temp, path, found := strings.Cut(record, "\t")
		if !found || !(strings.HasPrefix(temp, ".merge_file_") || strings.HasPrefix(temp, ".merge_link_")) || !validIndexPath(temp) ||
			strings.ContainsRune(temp, '/') || rendered[path] != "" {
			return nil, false, fmt.Errorf("invalid Git checkout-index response")
		}
		rendered[path] = temp
	}
	for _, path := range paths {
		if rendered[path] == "" {
			return nil, false, fmt.Errorf("Git did not restore the staged file %q", path)
		}
	}
	return rendered, strings.TrimSpace(string(symlinks)) == "true", nil
}
