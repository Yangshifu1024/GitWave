package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/text/cases"
)

type checkoutDirty struct {
	root        os.FileInfo
	changes     []Change
	files       []discardFile
	trackedOnly bool
}

func (r *Repository) captureCheckoutDirty(ctx context.Context, changes []Change) (checkoutDirty, error) {
	root, err := os.OpenRoot(r.WorkTree)
	if err != nil {
		return checkoutDirty{}, err
	}
	defer root.Close()
	info, err := root.Stat(".")
	if err != nil {
		return checkoutDirty{}, err
	}
	result := checkoutDirty{root: info, changes: changes}
	seen := make(map[string]bool, len(changes))
	for _, change := range changes {
		if !validIndexPath(change.Path) || change.IndexMode == "160000" || strings.HasPrefix(change.Submodule, "S") {
			return checkoutDirty{}, fmt.Errorf("commit or stash the change at %q before switching", change.Path)
		}
		if seen[change.Path] {
			continue
		}
		seen[change.Path] = true
		local := filepath.FromSlash(change.Path)
		parent, stamp, err := inspectDiscardFile(ctx, root, local)
		if err != nil {
			return checkoutDirty{}, err
		}
		result.files = append(result.files, discardFile{change: change, local: local, parent: parent, stamp: stamp})
	}
	return result, nil
}

func (d checkoutDirty) matches(other checkoutDirty) bool {
	if !sameFileIdentity(d.root, other.root) || !slices.Equal(d.changes, other.changes) || len(d.files) != len(other.files) {
		return false
	}
	for i, file := range d.files {
		actual := other.files[i]
		if file.local != actual.local || !sameFileIdentity(file.parent, actual.parent) || !file.stamp.matches(actual.stamp) {
			return false
		}
	}
	return true
}

func (d checkoutDirty) verify(ctx context.Context, root *os.Root, git *workingGit) error {
	info, err := root.Stat(".")
	if err != nil || !sameFileIdentity(info, d.root) {
		return fmt.Errorf("the checkout directory changed since confirmation")
	}
	status, err := git.checkoutStatus(ctx)
	if err != nil {
		return err
	}
	if d.trackedOnly {
		status.Changes = slices.DeleteFunc(status.Changes, untrackedChange)
	}
	if !slices.Equal(status.Changes, d.changes) {
		return fmt.Errorf("the working copy or Git index changed since confirmation")
	}
	for _, file := range d.files {
		parent, stamp, err := inspectDiscardFile(ctx, root, file.local)
		if err != nil {
			return err
		}
		if !sameFileIdentity(file.parent, parent) || !file.stamp.matches(stamp) {
			return fmt.Errorf("the working file changed since switching was requested: %q", file.change.Path)
		}
	}
	return nil
}

type checkoutCleanup struct {
	repo           *Repository
	state          checkoutDirty
	mode           CheckoutMode
	note           string
	stash          checkoutStash
	root           *os.Root
	restoreGit     *workingGit
	restored       *os.Root
	modes          map[string]string
	sources        map[string]string
	indexPaths     []string
	ignored        map[string]bool
	symlinks       bool
	started        bool
	cleaned        bool
	replayed       bool
	restoredSource bool
	replayErr      error
}

func (p CheckoutPlan) prepareCheckoutCleanup(ctx context.Context, lock *checkoutIndexLock, working *workingGit,
	mode CheckoutMode, note string) (*checkoutCleanup, error) {
	note = strings.TrimSpace(note)
	if note == "" {
		note = "switch to " + p.dest.name
	}
	if strings.ContainsRune(note, 0) || strings.ContainsAny(note, "\r\n") {
		return nil, fmt.Errorf("invalid Git stash message")
	}
	root, err := os.OpenRoot(p.repo.WorkTree)
	if err != nil {
		return nil, err
	}
	cleanup := &checkoutCleanup{repo: p.repo, state: p.dirty, mode: mode, note: note, root: root}
	ok := false
	defer func() {
		if !ok {
			cleanup.close()
		}
	}()
	if err := p.dirty.verify(ctx, root, working); err != nil {
		return nil, err
	}
	// A staged deletion and its untracked replacement can share a path.
	untracked := make(map[string]bool)
	for _, change := range p.dirty.changes {
		if change.Kind == '?' {
			untracked[change.Path] = true
		}
	}
	aliases := DiscardPlan{files: slices.Clone(p.dirty.files)}
	for i := range aliases.files {
		if untracked[aliases.files[i].change.Path] {
			aliases.files[i].change.Kind = '?'
		}
	}
	if err := aliases.checkPathAliases(ctx, root, working); err != nil {
		return nil, err
	}
	if err := cleanup.prepareSourceRestore(ctx, p.source.head); err != nil {
		return nil, err
	}
	if mode == CheckoutStash {
		if !working.frozenAttributes {
			if err := working.freezeWorktreeAttributes(ctx); err != nil {
				return nil, err
			}
		}
		paths := make([]string, 0, len(p.dirty.files))
		for _, file := range p.dirty.files {
			paths = append(paths, file.change.Path)
		}
		if err := working.checkFilters(ctx, paths, "replaying changes at", "--source="+p.dest.head); err != nil {
			return nil, err
		}
	}
	for _, change := range p.dirty.changes {
		if change.Kind != '?' {
			cleanup.indexPaths = append(cleanup.indexPaths, change.Path)
		}
	}
	cleanup.stash, err = p.createCheckoutStash(ctx, lock, working, note)
	if err != nil {
		return nil, err
	}
	ok = true
	return cleanup, nil
}

func (d *checkoutCleanup) prepareSourceRestore(ctx context.Context, head string) error {
	if head == "" {
		return nil
	}
	git, err := d.repo.workingGit(ctx, head)
	if err != nil {
		return err
	}
	d.restoreGit = git
	if err := git.isolateWorkTree(); err != nil {
		return err
	}
	git.index = filepath.Join(git.dir, "source-index")
	if err := git.runCheckoutCommand(ctx, "", "read-tree", head); err != nil {
		return err
	}
	d.restored, err = os.OpenRoot(git.workTree)
	if err != nil {
		return err
	}
	wanted := make(map[string]bool, len(d.state.files))
	for _, file := range d.state.files {
		wanted[file.change.Path] = true
	}
	out, err := git.read(ctx, 128<<20, "ls-tree", "-r", "-z", head, "--")
	if err != nil {
		return err
	}
	d.modes = make(map[string]string)
	var paths []string
	for record := range strings.SplitSeq(strings.TrimSuffix(string(out), "\x00"), "\x00") {
		meta, path, found := strings.Cut(record, "\t")
		if !wanted[path] {
			continue
		}
		fields := strings.Fields(meta)
		if !found || len(fields) != 3 || fields[1] != "blob" {
			return fmt.Errorf("the source at %q requires a separate checkout operation", path)
		}
		d.modes[path] = fields[0]
		paths = append(paths, path)
	}
	if len(paths) != 0 {
		// Fallback attributes removed by the reset must not influence
		// the files we restore. Attributes from HEAD still come from its index.
		var changingAttributes []os.FileInfo
		for _, file := range d.state.files {
			if file.stamp.info != nil && strings.EqualFold(filepath.Base(file.local), ".gitattributes") {
				changingAttributes = append(changingAttributes, file.stamp.info)
			}
		}
		config, err := d.repo.lfsConfigAfterRestore(ctx, d.state.files, head, false)
		if err != nil {
			return err
		}
		d.sources, d.symlinks, err = git.renderIndexFiles(ctx, d.root, d.restored, paths, &config, func(info os.FileInfo) bool {
			return slices.ContainsFunc(changingAttributes, func(file os.FileInfo) bool { return os.SameFile(file, info) })
		})
		return err
	}
	return nil
}

func (d *checkoutCleanup) reset(ctx context.Context, git *workingGit, head string) error {
	if err := git.verifyLocalLFSConfig(ctx); err != nil {
		return err
	}
	if err := d.state.verify(ctx, d.root, git); err != nil {
		return err
	}
	if d.restoreGit != nil {
		if err := d.restoreGit.verifyLocalLFSConfig(ctx); err != nil {
			return err
		}
		if err := d.restoreGit.verifyLFSPathScope(ctx); err != nil {
			return err
		}
	}
	d.started = true
	for _, file := range d.state.files {
		path := file.change.Path
		mode := d.modes[path]
		if mode == "" && file.stamp.info == nil {
			continue
		}
		// Restore the original HEAD's contents rather than its index.
		file.change.Kind, file.change.IndexMode = '1', mode
		if mode == "" {
			file.change.Kind = '?'
		}
		if err := file.apply(ctx, d.root, d.restored, d.sources[path], d.symlinks); err != nil {
			return err
		}
	}
	if len(d.indexPaths) != 0 {
		if err := git.unstageSelection(ctx, head, d.indexPaths); err != nil {
			return err
		}
	}
	status, err := git.checkoutStatus(ctx)
	if err != nil {
		return err
	}
	if !d.cleanSource(status) {
		return fmt.Errorf("the working copy changed while preparing to switch")
	}
	d.cleaned = true
	return nil
}

func (d *checkoutCleanup) cleanSource(status Status) bool {
	for _, change := range status.Changes {
		if change.Kind != '?' || !(d.state.trackedOnly || d.ignored[change.Path]) {
			return false
		}
	}
	return true
}

func (d *checkoutCleanup) protectIgnored(ctx context.Context, g *workingGit, target string, paths []string) error {
	ignored, err := g.read(ctx, 64<<20, "ls-files", "--others", "--ignored", "--exclude-standard", "-z")
	if err != nil || len(ignored) == 0 {
		return err
	}
	d.ignored = make(map[string]bool)
	for name := range strings.SplitSeq(strings.TrimSuffix(string(ignored), "\x00"), "\x00") {
		d.ignored[name] = true
	}
	return g.checkCheckoutPathCollisions(ctx, target, paths, ignored, "ignored")
}

func (g *workingGit) checkCheckoutPathCollisions(ctx context.Context, target string, paths []string, local []byte, kind string) error {
	if len(paths) == 0 {
		return nil
	}
	tree, err := g.read(ctx, 128<<20, "ls-tree", "-r", "--name-only", "-z", target, "--")
	if err != nil {
		return err
	}
	set := make(map[string]bool, len(paths))
	for _, name := range paths {
		set[name] = true
	}
	normalize := func(name string) string { return name }
	value, err := g.read(ctx, 32, "config", "--bool", "--default=false", "--get", "core.ignorecase")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(value)) == "true" {
		fold := cases.Fold()
		normalize = fold.String
	}
	var targets []string
	for name := range strings.SplitSeq(strings.TrimSuffix(string(tree), "\x00"), "\x00") {
		if set[name] {
			targets = append(targets, normalize(name))
		}
	}
	slices.Sort(targets)
	for original := range strings.SplitSeq(strings.TrimSuffix(string(local), "\x00"), "\x00") {
		name := normalize(strings.TrimSuffix(original, "/"))
		index, _ := slices.BinarySearch(targets, name+"/")
		if index < len(targets) && strings.HasPrefix(targets[index], name+"/") {
			return fmt.Errorf("checkout would overwrite %s path %q", kind, original)
		}
		for prefix := name; prefix != "." && prefix != "/" && prefix != ""; prefix = path.Dir(prefix) {
			if _, found := slices.BinarySearch(targets, prefix); found {
				return fmt.Errorf("checkout would overwrite %s path %q", kind, original)
			}
		}
	}
	return nil
}

func (d *checkoutCleanup) close() {
	d.root.Close()
	if d.restored != nil {
		d.restored.Close()
	}
	if d.restoreGit != nil {
		d.restoreGit.close()
	}
}

func (d *checkoutCleanup) finish(ctx context.Context, checkoutErr error) (string, error) {
	warning := ""
	if d.started && !d.restoredSource && (checkoutErr != nil || d.replayErr != nil) {
		if err := d.stash.store(ctx, d.repo, d.note); err != nil {
			warning = fmt.Sprintf("Saved changes can be recovered from %s. Could not add the entry to the stash list: %v", d.stash.ref, err)
		} else {
			warning = fmt.Sprintf("Saved changes remain in the stash list (%s).", d.stash.oid[:7])
			warning = joinCheckoutWarning(warning, d.stash.remove(ctx, d.repo))
		}
	} else {
		warning = d.stash.remove(ctx, d.repo)
	}
	if checkoutErr != nil {
		if warning != "" {
			checkoutErr = errors.Join(checkoutErr, errors.New(warning))
		}
		return "", checkoutErr
	}
	if d.replayErr != nil {
		warning = joinCheckoutWarning(warning, fmt.Sprintf("Could not reapply all saved changes: %v", d.replayErr))
	}
	return warning, nil
}
