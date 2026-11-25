package gitcli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

const stashRef = "refs/stash"

var ErrStashTargetChanged = errors.New("HEAD or the Git operation changed since the stash was requested")

type StashEntry struct {
	Index      int
	OID        string
	Message    string
	reflogDate string
	actor      string
	generation stashGeneration
}

// SameSelection distinguishes reflog records even when Git stores the same
// commit, message, actor and timestamp more than once.
func (s StashEntry) SameSelection(other StashEntry) bool {
	return s.Index == other.Index && s.sameRecord(other) && s.generation.matches(other.generation)
}

func (s StashEntry) sameRecord(other StashEntry) bool {
	return s.OID == other.OID && s.Message == other.Message && s.reflogDate == other.reflogDate && s.actor == other.actor
}

type stashGeneration [2]os.FileInfo

func (s stashGeneration) matches(other stashGeneration) bool {
	for i, info := range s {
		if info == nil || other[i] == nil {
			if !sameFileIdentity(info, other[i]) {
				return false
			}
		} else if !sameDiscardMetadata(info, other[i]) {
			return false
		}
	}
	return true
}

func stashGenerationAt(common *os.Root) (stashGeneration, error) {
	var result stashGeneration
	for i, path := range [...]string{stashLogPath, reftableListPath} {
		local := filepath.FromSlash(path)
		observed, err := common.Lstat(local)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return result, err
		}
		if !observed.Mode().IsRegular() {
			return result, fmt.Errorf("invalid Git stash metadata %q", path)
		}
		file, err := common.OpenFile(local, os.O_RDONLY|workFileOpenFlags, 0)
		if err != nil {
			return result, err
		}
		info, err := file.Stat()
		file.Close()
		if err != nil {
			return result, err
		}
		if !sameDiscardMetadata(observed, info) {
			return result, fmt.Errorf("Git stash metadata %q changed while opening it", path)
		}
		result[i] = info
	}
	return result, nil
}

func (r *Repository) Stashes(ctx context.Context) ([]StashEntry, error) {
	common, err := os.OpenRoot(r.CommonDir)
	if err != nil {
		return nil, err
	}
	defer common.Close()
	before, err := stashGenerationAt(common)
	if err != nil {
		return nil, err
	}
	symbolic, err := r.optionalRef(ctx, "symbolic-ref", "--no-recurse", "-q", stashRef)
	if err != nil {
		return nil, err
	}
	if symbolic != "" {
		return nil, fmt.Errorf("%s is a symbolic reference", stashRef)
	}
	entries, err := readStashLog(ctx, func(ctx context.Context, limit int64, args ...string) ([]byte, error) {
		return r.readLimited(ctx, "", limit, args...)
	})
	if err != nil {
		return nil, err
	}
	after, err := stashGenerationAt(common)
	if err != nil {
		return nil, err
	}
	if !before.matches(after) || len(entries) != 0 && (before[0] == nil && before[1] == nil) {
		return nil, fmt.Errorf("the stash list changed while reading it")
	}
	for i := range entries {
		entries[i].generation = before
	}
	return entries, nil
}

func readStashLog(ctx context.Context, read func(context.Context, int64, ...string) ([]byte, error)) ([]StashEntry, error) {
	// A revision expression alone can DWIM to a branch named refs/stash.
	if _, err := read(ctx, 4096, "show-ref", "--exists", stashRef); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 2 {
			return nil, nil
		}
		return nil, err
	}
	const format = "--format=%H%x00%gD%x00%gs%x00%gn%x00%ge"
	logArgs := []string{"reflog", "show", "--no-color", "--no-show-signature", format, "-z"}
	out, err := read(ctx, 8<<20, append(slices.Clone(logArgs), stashRef, "--")...)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	dated, err := read(ctx, 8<<20, append(logArgs, "--date=raw", stashRef, "--")...)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	dates := strings.Split(strings.TrimSuffix(string(dated), "\x00"), "\x00")
	const width = 5
	if len(fields)%width != 0 || len(fields) != len(dates) {
		return nil, fmt.Errorf("invalid Git stash list")
	}
	entries := make([]StashEntry, 0, len(fields)/width)
	for i := 0; i < len(fields); i += width {
		if fields[i] != dates[i] || !slices.Equal(fields[i+2:i+width], dates[i+2:i+width]) {
			return nil, fmt.Errorf("the stash list changed while reading it")
		}
		ordinal, ok := stashSelector(fields[i+1])
		index, numberErr := strconv.Atoi(ordinal)
		stamp, datedOK := stashSelector(dates[i+1])
		if !validOID(fields[i]) || !ok || !datedOK || numberErr != nil || index < 0 {
			return nil, fmt.Errorf("invalid Git stash entry")
		}
		entries = append(entries, StashEntry{Index: index, OID: fields[i], Message: fields[i+2], reflogDate: stamp,
			actor: fields[i+3] + "\x00" + fields[i+4]})
	}
	return entries, nil
}

func stashSelector(label string) (string, bool) {
	selector, found := strings.CutPrefix(label, stashRef+"@{")
	if !found {
		return "", false
	}
	return strings.CutSuffix(selector, "}")
}

// StashDiff includes untracked files from the third parent's tree. A tracked
// delta takes precedence if an external stash contains the same path twice.
func (r *Repository) StashDiff(ctx context.Context, oid, path string, expanded bool) (DiffPreview, error) {
	if !validOID(oid) {
		return DiffPreview{}, fmt.Errorf("invalid Git stash ID")
	}
	stash, err := r.readCommitHeader(ctx, oid)
	if err != nil {
		return DiffPreview{}, err
	}
	if len(stash.parents) < 2 || len(stash.parents) > 3 {
		return DiffPreview{}, fmt.Errorf("invalid Git stash commit")
	}
	tracked, err := r.treeDiff(ctx, stash.parents[0], oid, path, expanded)
	if err != nil || len(stash.parents) < 3 || path != "" && len(tracked.Files) > 0 {
		return tracked, err
	}
	empty, err := r.readLimited(ctx, "", 4096, "hash-object", "-t", "tree", "--stdin")
	if err != nil {
		return DiffPreview{}, err
	}
	untracked, err := r.treeDiff(ctx, strings.TrimSpace(string(empty)), stash.parents[2], path, expanded)
	if err != nil {
		return DiffPreview{}, err
	}
	seen := make(map[string]bool, len(tracked.Files))
	for _, file := range tracked.Files {
		seen[file.Path] = true
	}
	for _, file := range untracked.Files {
		if !seen[file.Path] {
			file.Untracked = true
			tracked.Files = append(tracked.Files, file)
		}
	}
	tracked.Truncated = tracked.Truncated || untracked.Truncated
	tracked.TooLarge = tracked.TooLarge || untracked.TooLarge
	return tracked, nil
}

func untrackedChange(change Change) bool { return change.Kind == '?' }

// SaveStash makes the snapshot reachable before altering any working files.
// HEAD, the index and refs/stash are locked while clearing saved changes.
func (r *Repository) SaveStash(ctx context.Context, shown Status, message string, includeUntracked bool) (oid string, err error) {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "WIP"
	}
	if strings.ContainsAny(message, "\x00\r\n") {
		return "", fmt.Errorf("invalid Git stash message")
	}
	if !validOID(shown.HeadOID) {
		return "", fmt.Errorf("create an initial commit before saving a stash")
	}
	lock, err := r.lockCheckoutIndex(ctx)
	if err != nil {
		return "", err
	}
	defer lock.close()
	head, err := r.optionalRef(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return "", err
	}
	if head != shown.HeadOID {
		return "", ErrStashTargetChanged
	}
	if err := r.verifyStashTarget(ctx, lock, shown); err != nil {
		return "", err
	}
	pause := shown.Commit.InteractiveRebase
	if err := r.checkStashOperation(ctx, shown); err != nil {
		return "", err
	}
	git, err := r.workingGit(ctx, shown.HeadOID)
	if err != nil {
		return "", err
	}
	defer git.close()
	git.index = filepath.Join(git.dir, "stash-save-index")
	git.settings = append(git.settings, "-c", "core.splitIndex=false")
	if err := copyOriginalIndex(lock.root, git.index); err != nil {
		return "", err
	}
	status, err := git.checkoutStatus(ctx)
	if err != nil {
		return "", err
	}
	if err := git.configureLocalLFS(false, true); err != nil {
		return "", err
	}
	if !includeUntracked {
		status.Changes = slices.DeleteFunc(status.Changes, untrackedChange)
	}
	if len(status.Changes) == 0 {
		return "", fmt.Errorf("there are no changes to stash")
	}
	if err := git.checkSnapshotChanges(ctx, status.Changes); err != nil {
		return "", err
	}
	dirty, err := r.captureCheckoutDirty(ctx, status.Changes)
	if err != nil {
		return "", err
	}
	dirty.trackedOnly = !includeUntracked
	if err := dirty.protectUntrackedReplacements(); err != nil {
		return "", err
	}
	branch := strings.TrimPrefix(shown.HeadRef, "refs/heads/")
	if branch == "" {
		branch = "(no branch)"
	}
	note := "On " + branch + ": " + message
	plan := CheckoutPlan{repo: r, dirty: dirty}
	plan.source.head = shown.HeadOID
	cleanup, err := plan.prepareCheckoutCleanup(ctx, lock, git, CheckoutDiscard, note)
	if err != nil {
		return "", err
	}
	defer cleanup.close()
	var cleared bool
	defer func() {
		cleanCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err != nil && cleared {
			entries, readErr := r.Stashes(cleanCtx)
			if readErr == nil && slices.ContainsFunc(entries, func(entry StashEntry) bool { return entry.OID == cleanup.stash.oid }) {
				oid, err = cleanup.stash.oid, nil
			}
		}
		if err != nil && cleanup.started && oid == "" {
			if storeErr := cleanup.stash.storeMessage(cleanCtx, r, note); storeErr != nil {
				err = fmt.Errorf("%w. Saved changes can be recovered from %s: %v", err, cleanup.stash.ref, storeErr)
				return
			}
			oid = cleanup.stash.oid
			if cleared {
				err = nil
			} else {
				err = fmt.Errorf("stash %s was saved, but the working copy could not be fully cleared: %w", oid, err)
			}
		}
		if warning := cleanup.stash.remove(cleanCtx, r); warning != "" {
			err = errors.Join(err, errors.New(warning))
		}
	}()
	if err := cleanup.protectIgnored(ctx, git, shown.HeadOID, nil); err != nil {
		return "", err
	}
	old, err := r.exactRootRef(ctx, stashRef)
	if err != nil {
		return "", err
	}
	if old == "" {
		old = strings.Repeat("0", len(cleanup.stash.oid))
	}
	cmd := r.command(ctx, "", "-c", "core.hooksPath="+git.dir, "update-ref", "--stdin", "-z", "--create-reflog", "-m", note)
	statement := "verify HEAD\x00" + shown.HeadOID + "\x00option no-deref\x00update " + stashRef + "\x00" + cleanup.stash.oid + "\x00" + old + "\x00"
	if validOID(pause) {
		statement += verifyCheckoutRef(interactiveRebaseRefName, pause)
	}
	_, _, err = runReferenceTransaction(cmd, statement, func() error {
		if err := r.verifyStashTarget(ctx, lock, shown); err != nil {
			return err
		}
		symbolic, err := r.optionalRef(ctx, "symbolic-ref", "--no-recurse", "-q", stashRef)
		if err != nil {
			return err
		}
		if symbolic != "" {
			return fmt.Errorf("%s is a symbolic reference", stashRef)
		}
		if err := cleanup.reset(ctx, git, shown.HeadOID); err != nil {
			return err
		}
		if len(cleanup.indexPaths) != 0 {
			if err := publishIndex(lock.root, lock.file, lock.info, git.index, func() error {
				return r.verifyStashTarget(ctx, lock, shown)
			}); err != nil {
				return err
			}
		}
		cleared = true
		return nil
	})
	if err == nil {
		oid = cleanup.stash.oid
	}
	return oid, err
}

func (r *Repository) checkStashOperation(ctx context.Context, shown Status) error {
	pause := shown.Commit.InteractiveRebase
	if err := r.checkCheckoutOperationDuring(ctx, pause); err != nil {
		return err
	}
	if !validOID(pause) {
		return nil
	}
	state, err := r.loadInteractivePause(ctx, pause)
	if err != nil {
		return err
	}
	if state.Head != shown.HeadOID || state.Branch != shown.HeadRef {
		return fmt.Errorf("the paused interactive rebase belongs to a different checkout")
	}
	return nil
}

func (r *Repository) verifyStashTarget(ctx context.Context, lock *checkoutIndexLock, shown Status) error {
	if !lock.ownsLock() {
		return fmt.Errorf("the Git index lock was replaced")
	}
	if err := r.checkIndexCheckout(ctx); err != nil {
		return err
	}
	ref, err := r.optionalRef(ctx, "symbolic-ref", "-q", "HEAD")
	if err != nil {
		return err
	}
	state, err := r.readCommitState(ctx, lock.root)
	if err != nil {
		return err
	}
	if ref != shown.HeadRef || state != shown.Commit {
		return ErrStashTargetChanged
	}
	return ctx.Err()
}

func (g *workingGit) checkSnapshotChanges(ctx context.Context, changes []Change) error {
	tracked := make(map[string]bool, len(changes))
	for _, change := range changes {
		if change.Kind == 'u' {
			return fmt.Errorf("resolve the conflict at %q first", change.Path)
		}
		if change.Kind != '?' {
			tracked[change.Path] = true
		}
	}
	out, err := g.read(ctx, 64<<20, "ls-files", "-v", "-z", "--")
	if err != nil {
		return err
	}
	for record := range strings.SplitSeq(strings.TrimSuffix(string(out), "\x00"), "\x00") {
		if len(record) > 2 && tracked[record[2:]] && (record[0] == 'S' || record[0] >= 'a' && record[0] <= 'z') {
			return fmt.Errorf("cannot verify changes to index path %q", record[2:])
		}
	}
	return nil
}

func (d checkoutDirty) protectUntrackedReplacements() error {
	untracked := make(map[string]bool)
	for _, change := range d.changes {
		if untrackedChange(change) {
			untracked[change.Path] = true
		}
	}
	for _, file := range d.files {
		if file.change.Index == 'D' && file.stamp.info != nil && !untracked[file.change.Path] {
			return fmt.Errorf("the working copy has an unselected replacement at %q", file.change.Path)
		}
	}
	return nil
}
