package gitcli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

type StashApplyResult struct {
	Conflicts []string
	Warning   error
	Dropped   bool
}

// ApplyStash uses the immutable stash object. The reflog selection, HEAD and
// index are checked again before the private Git result is published.
func (r *Repository) ApplyStash(ctx context.Context, entry StashEntry, shown Status) (StashApplyResult, error) {
	if !validOID(entry.OID) || !validOID(shown.HeadOID) {
		return StashApplyResult{}, fmt.Errorf("select a stash and an existing checkout")
	}
	lock, err := r.lockCheckoutIndex(ctx)
	if err != nil {
		return StashApplyResult{}, err
	}
	defer lock.close()
	current, err := r.Status(ctx)
	if err != nil {
		return StashApplyResult{}, err
	}
	if current.HeadOID != shown.HeadOID || current.HeadRef != shown.HeadRef || current.Commit != shown.Commit ||
		!slices.Equal(current.Changes, shown.Changes) {
		return StashApplyResult{}, fmt.Errorf("the working copy changed since applying the stash was requested")
	}
	if err := r.checkStashOperation(ctx, shown); err != nil {
		return StashApplyResult{}, err
	}
	for _, change := range current.Changes {
		if change.Kind == 'u' || change.Index != '.' && change.Index != 0 {
			return StashApplyResult{}, fmt.Errorf("clear staged changes and resolve index conflicts before applying a stash")
		}
	}
	localChanges := make(map[string]bool, len(current.Changes))
	for _, change := range current.Changes {
		localChanges[change.Path] = true
	}
	entries, err := r.checkedStashEntry(ctx, entry)
	if err != nil {
		return StashApplyResult{}, err
	}
	stash, err := r.StashDiff(ctx, entry.OID, "", false)
	if err != nil {
		return StashApplyResult{}, err
	}
	git, err := r.workingGit(ctx, current.HeadOID)
	if err != nil {
		return StashApplyResult{}, err
	}
	defer git.close()
	if err := git.prepareCheckoutConfig(ctx, current.HeadOID, entry.OID, nil); err != nil {
		return StashApplyResult{}, err
	}
	paths := make([]string, 0, len(stash.Files))
	for _, file := range stash.Files {
		paths = append(paths, file.Path)
	}
	if len(paths) != 0 {
		if err := git.configureLocalLFS(false, true); err != nil {
			return StashApplyResult{}, err
		}
		if err := git.checkFilters(ctx, paths, "applying the stash at", "--source="+entry.OID); err != nil {
			return StashApplyResult{}, err
		}
		if err := git.checkWorktreeFilters(ctx, paths, "applying the stash at", false); err != nil {
			return StashApplyResult{}, err
		}
	}
	git.settings = append(git.settings, "-c", "core.hooksPath="+git.dir, "-c", "rerere.enabled=false",
		"-c", "advice.sparseIndexExpanded=false",
		"-c", "filter.set.required=true", "-c", "filter.unset.required=true", "-c", "filter.unspecified.required=true")
	liveGit := *git
	git.index = filepath.Join(git.dir, "stash-apply-index")
	if err := copyOriginalIndex(lock.root, git.index); err != nil {
		return StashApplyResult{}, err
	}
	root, err := os.OpenRoot(r.WorkTree)
	if err != nil {
		return StashApplyResult{}, err
	}
	defer root.Close()
	if err := git.isolateWorkTree(); err != nil {
		return StashApplyResult{}, err
	}
	shadow, err := os.OpenRoot(git.workTree)
	if err != nil {
		return StashApplyResult{}, err
	}
	defer shadow.Close()
	snapshot, err := git.copyStashWorktree(ctx, &liveGit, root, shadow, paths)
	if err != nil {
		return StashApplyResult{}, err
	}
	applyErr := git.applyStashToSnapshot(ctx, entry.OID)
	conflicts, err := git.replayConflicts(ctx)
	if err != nil {
		return StashApplyResult{}, errors.Join(applyErr, err)
	}
	if applyErr != nil && len(conflicts) == 0 {
		return StashApplyResult{}, applyErr
	}
	after, err := readStashWorktree(ctx, shadow, snapshot.copied)
	if err != nil {
		return StashApplyResult{}, err
	}
	groups, replacements, modes, changed, err := snapshot.changes(ctx, root, after, localChanges)
	if err != nil {
		return StashApplyResult{}, err
	}
	if len(changed) != 0 {
		if err := liveGit.checkWorktreeFilters(ctx, changed, "applying the stash at", false); err != nil {
			return StashApplyResult{}, err
		}
		if err := git.checkWorktreeFilters(ctx, changed, "restoring the stash at", false); err != nil {
			return StashApplyResult{}, err
		}
		if _, err := liveGit.preserveWorkingLFSObjects(ctx, changed); err != nil {
			return StashApplyResult{}, err
		}
	}
	for _, file := range conflicts {
		if snapshot.hidden[file] {
			return StashApplyResult{}, fmt.Errorf("the stash conflicts with hidden index path %q", file)
		}
	}
	warning, err := r.publishStashApply(ctx, entry, entries[0].OID, shown, lock, git, root, shadow, groups, replacements, modes)
	if err != nil {
		return StashApplyResult{}, err
	}
	if applyErr != nil {
		var exit *exec.ExitError
		if !errors.As(applyErr, &exit) || exit.ExitCode() != 1 {
			warning = errors.Join(warning, applyErr)
		}
	}
	return StashApplyResult{Conflicts: conflicts, Warning: warning}, nil
}

func (g *workingGit) applyStashToSnapshot(ctx context.Context, oid string) error {
	cmd := g.command(ctx, "", "stash", "apply", "--no-index", "--quiet", oid)
	var diagnostic gitDiagnostic
	cmd.Stderr = &diagnostic
	runErr := cmd.Run()
	message := strings.TrimSpace(diagnostic.buffer.String())
	if runErr == nil && message != "" {
		// Git applies a file/directory replacement successfully but reports
		// that it cannot unstage the incompatible original and new paths.
		// The private index correctly stages the replacement in this case.
		shapeOnly := true
		for line := range strings.SplitSeq(message, "\n") {
			if !strings.HasPrefix(line, "error: '") || !strings.HasSuffix(line, "' appears as both a file and as a directory") {
				shapeOnly = false
				break
			}
		}
		if shapeOnly {
			return ctx.Err()
		}
	}
	if message != "" {
		return fmt.Errorf("Git stash: %s", message)
	}
	return errors.Join(ctx.Err(), runErr)
}

func (r *Repository) checkedStashEntry(ctx context.Context, entry StashEntry) ([]StashEntry, error) {
	entries, err := r.Stashes(ctx)
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(entries, entry.SameSelection) {
		return nil, fmt.Errorf("the stash list changed since this entry was shown")
	}
	return entries, nil
}

type stashWorktreeSnapshot struct {
	original map[string]discardStamp
	copied   map[string]discardStamp
	tracked  map[string]bool
	hidden   map[string]bool
}

func (g *workingGit) copyStashWorktree(ctx context.Context, live *workingGit, root, shadow *os.Root, affected []string) (stashWorktreeSnapshot, error) {
	snapshot := stashWorktreeSnapshot{
		original: make(map[string]discardStamp), copied: make(map[string]discardStamp),
		tracked: make(map[string]bool), hidden: make(map[string]bool),
	}
	index, err := live.read(ctx, 128<<20, "ls-files", "--stage", "-v", "-z", "--")
	if err != nil {
		return snapshot, err
	}
	paths := make(map[string]bool)
	skipWorktree := make(map[string]bool)
	for record := range strings.SplitSeq(string(index), "\x00") {
		if record == "" {
			continue
		}
		meta, name, ok := strings.Cut(record, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 4 || len(fields[0]) != 1 || !validIndexPath(name) {
			return snapshot, fmt.Errorf("invalid Git index entry for %q", name)
		}
		if fields[1] == "160000" {
			continue
		}
		snapshot.tracked[name] = true
		tag := fields[0][0]
		if tag >= 'a' && tag <= 'z' {
			snapshot.hidden[name] = true
		}
		if tag == 'S' {
			skipWorktree[name] = true
		}
		paths[name] = true
	}
	others, err := live.read(ctx, 64<<20, "ls-files", "--others", "--exclude-standard", "-z", "--")
	if err != nil {
		return snapshot, err
	}
	for name := range strings.SplitSeq(string(others), "\x00") {
		if name != "" && !strings.HasSuffix(name, "/") {
			paths[name] = true
		}
	}
	// Ignored attributes can still affect the paths Git has to restore.
	for _, name := range affected {
		for dir := path.Dir(name); ; dir = path.Dir(dir) {
			paths[path.Join(dir, ".gitattributes")] = true
			if dir == "." {
				break
			}
		}
	}
	names := make([]string, 0, len(paths))
	for name := range paths {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if !validIndexPath(name) {
			return snapshot, fmt.Errorf("unsupported stash path %q", name)
		}
		local := filepath.FromSlash(name)
		info, err := root.Lstat(local)
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) || err == nil && info.IsDir() {
			continue
		}
		if err != nil {
			return snapshot, err
		}
		if skipWorktree[name] {
			snapshot.hidden[name] = true
		}
		if !info.Mode().IsRegular() && info.Mode()&fs.ModeSymlink == 0 {
			continue
		}
		if err := g.copyStageFile(ctx, root, shadow, local, info, true); err != nil {
			return snapshot, err
		}
		_, copied, err := inspectDiscardFile(ctx, shadow, local)
		if err != nil {
			return snapshot, err
		}
		// Each changed source path is compared with these exact copied bytes
		// again before publishing anything to the live worktree.
		original := discardStamp{info: info, sum: copied.sum}
		snapshot.original[name], snapshot.copied[name] = original, copied
	}
	return snapshot, nil
}

func sameStashFile(a, b discardStamp) bool {
	if a.info == nil || b.info == nil {
		return a.info == nil && b.info == nil
	}
	const comparableMode = fs.ModeType | 0o100
	return a.sum == b.sum && a.info.Mode()&comparableMode == b.info.Mode()&comparableMode
}

func readStashWorktree(ctx context.Context, root *os.Root, before map[string]discardStamp) (map[string]discardStamp, error) {
	files := make(map[string]discardStamp, len(before))
	err := fs.WalkDir(root.FS(), ".", func(name string, item fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if item.IsDir() {
			return nil
		}
		if !validIndexPath(name) {
			return fmt.Errorf("invalid restored path %q", name)
		}
		info, err := item.Info()
		if err != nil {
			return err
		}
		previous := before[name]
		if previous.info != nil && sameDiscardMetadata(previous.info, info) {
			files[name] = previous
			return nil
		}
		_, stamp, err := inspectDiscardFile(ctx, root, filepath.FromSlash(name))
		files[name] = stamp
		return err
	})
	return files, err
}

func (s stashWorktreeSnapshot) changes(ctx context.Context, root *os.Root, after map[string]discardStamp,
	localChanges map[string]bool) ([]abortGroup, map[string]string, map[string]string, []string, error) {
	var changed []string
	replacements, modes := make(map[string]string), make(map[string]string)
	for name, file := range after {
		if sameStashFile(s.copied[name], file) {
			continue
		}
		changed = append(changed, name)
		replacements[name] = filepath.FromSlash(name)
		switch {
		case file.info.Mode()&fs.ModeSymlink != 0:
			modes[name] = "120000"
		case file.info.Mode()&0o100 != 0:
			modes[name] = "100755"
		default:
			modes[name] = "100644"
		}
	}
	for name := range s.copied {
		if _, exists := after[name]; !exists {
			changed = append(changed, name)
		}
	}
	slices.Sort(changed)
	if err := checkCheckoutAliases(root, changed); err != nil {
		return nil, nil, nil, nil, err
	}
	byRoot := make(map[string][]string)
	for _, name := range changed {
		if s.hidden[name] {
			return nil, nil, nil, nil, fmt.Errorf("cannot verify edits to hidden index path %q", name)
		}
		if localChanges[name] || s.original[name].info != nil && !s.tracked[name] {
			return nil, nil, nil, nil, fmt.Errorf("applying the stash would replace a local change at %q", name)
		}
		group := name
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if _, found := slices.BinarySearch(changed, parent); found {
				group = parent
				continue
			}
			// Stage a whole new directory under its nearest existing parent.
			// Preparing replacements must leave the real path absent until
			// the reference transaction verifies the selected stash.
			if _, err := root.Lstat(filepath.FromSlash(parent)); errors.Is(err, fs.ErrNotExist) {
				group = parent
			}
		}
		byRoot[group] = append(byRoot[group], name)
	}
	groups := make([]abortGroup, 0, len(byRoot))
	for name, files := range byRoot {
		group, err := inspectAbortGroup(ctx, root, name, files)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		for file, stamp := range group.before {
			if stamp.info.IsDir() && slices.ContainsFunc(files, func(path string) bool { return strings.HasPrefix(path, file+"/") }) {
				continue
			}
			if stamp.info.IsDir() || !s.original[file].matches(stamp) || !slices.Contains(files, file) {
				return nil, nil, nil, nil, fmt.Errorf("applying the stash would replace an independent or changed path: %q", file)
			}
		}
		for _, file := range files {
			if original := s.original[file]; original.info != nil && !original.matches(group.before[file]) {
				return nil, nil, nil, nil, fmt.Errorf("the working path changed while preparing the stash: %q", file)
			}
		}
		groups = append(groups, group)
	}
	slices.SortFunc(groups, func(a, b abortGroup) int { return strings.Compare(a.path, b.path) })
	return groups, replacements, modes, changed, nil
}

func (r *Repository) publishStashApply(ctx context.Context, entry StashEntry, tip string, shown Status, lock *checkoutIndexLock,
	git *workingGit, root, shadow *os.Root, groups []abortGroup, replacements, modes map[string]string) (warning error, err error) {
	var swaps []*abortSwap
	defer func() {
		for _, swap := range swaps {
			swap.close()
		}
	}()
	for _, group := range groups {
		swap, err := group.prepare(ctx, root, shadow, replacements, modes, true)
		if err != nil {
			return nil, err
		}
		if swap != nil {
			swaps = append(swaps, swap)
		}
	}
	if err := writeLockedIndex(lock.root, lock.file, git.index); err != nil {
		return nil, err
	}
	finalize := context.WithoutCancel(ctx)
	cmd := r.command(finalize, "", "-c", "core.hooksPath="+git.dir, "update-ref", "--stdin", "-z")
	statement := "verify HEAD\x00" + shown.HeadOID + "\x00" + verifyCheckoutRef(stashRef, tip)
	if pause := shown.Commit.InteractiveRebase; validOID(pause) {
		statement += verifyCheckoutRef(interactiveRebaseRefName, pause)
	}
	applied, installed := 0, false
	_, _, err = runReferenceTransaction(cmd, statement, func() error {
		if err := r.verifyStashApplyTarget(ctx, lock, root, shown); err != nil {
			return err
		}
		if _, err := r.checkedStashEntry(ctx, entry); err != nil {
			return err
		}
		if err := git.verifyLocalLFSConfig(ctx); err != nil {
			return err
		}
		for _, swap := range swaps {
			applied++
			if err := swap.apply(finalize, root); err != nil {
				return err
			}
		}
		if err := r.verifyStashApplyTarget(finalize, lock, root, shown); err != nil {
			return err
		}
		if err := installLockedIndex(lock.root, lock.info); err != nil {
			return err
		}
		installed = true
		return nil
	})
	if !installed {
		for i := applied - 1; i >= 0; i-- {
			err = errors.Join(err, swaps[i].restore(finalize))
		}
		return nil, err
	}
	for _, swap := range swaps {
		warning = errors.Join(warning, swap.finish(finalize))
	}
	return warning, nil
}

func (r *Repository) verifyStashApplyTarget(ctx context.Context, lock *checkoutIndexLock, root *os.Root, shown Status) error {
	if err := r.verifyStashTarget(ctx, lock, shown); err != nil {
		return err
	}
	openRoot, err := root.Stat(".")
	if err != nil {
		return err
	}
	currentRoot, err := os.Stat(r.WorkTree)
	if err != nil {
		return err
	}
	if !sameFileIdentity(openRoot, currentRoot) {
		return fmt.Errorf("the checkout directory changed while applying the stash")
	}
	return nil
}
