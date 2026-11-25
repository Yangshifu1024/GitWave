package gitcli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

const legacyPausePrefix = "legacy:"

type legacyInteractivePause struct {
	Original  string       `json:"original_head"`
	Remaining []RebaseTodo `json:"remaining"`
}

func legacyPauseVersion(raw string) string {
	return fmt.Sprintf("%s%x", legacyPausePrefix, sha256.Sum256([]byte(raw)))
}

func (r *Repository) openInteractivePause(ctx context.Context, expected string) (interactivePause, string, error) {
	if strings.HasPrefix(expected, legacyPausePrefix) {
		id, err := r.migrateLegacyPause(ctx, expected)
		if err != nil {
			return interactivePause{}, "", err
		}
		expected = id
	}
	state, err := r.loadInteractivePause(ctx, expected)
	if err == nil && state.Legacy != "" {
		err = r.removeLegacyPause(ctx, expected, state.Legacy)
	}
	return state, expected, err
}

func (r *Repository) migrateLegacyPause(ctx context.Context, expected string) (string, error) {
	lock, err := r.lockCheckoutIndex(ctx)
	if err != nil {
		return "", err
	}
	defer lock.close()
	operation, err := r.readCommitState(ctx, lock.root)
	if err != nil {
		return "", err
	}
	if operation.InteractiveRebase != expected {
		return "", fmt.Errorf("the legacy interactive rebase changed since it was shown")
	}
	metadata, err := lock.root.Lstat(legacyInteractiveRebaseFile)
	if err != nil {
		return "", err
	}
	raw, err := readOperationFile(lock.root, legacyInteractiveRebaseFile)
	if err != nil {
		return "", err
	}
	if legacyPauseVersion(raw) != expected {
		return "", fmt.Errorf("the legacy interactive rebase changed while reading")
	}
	var legacy legacyInteractivePause
	if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
		return "", fmt.Errorf("invalid legacy interactive rebase data: %w", err)
	}
	refs, err := r.References(ctx)
	if err != nil {
		return "", err
	}
	if !validOID(refs.Head) || !strings.HasPrefix(refs.HeadRef, "refs/heads/") {
		return "", fmt.Errorf("the legacy interactive rebase requires its checked-out local branch")
	}
	if legacy.Original != "" {
		if err := r.requireCommit(ctx, legacy.Original); err != nil {
			return "", err
		}
	}
	base, err := r.legacyRebaseBase(ctx, refs, legacy.Original, metadata.ModTime().Unix())
	if err != nil {
		return "", err
	}
	state := interactivePause{Branch: refs.HeadRef, Head: refs.Head, Original: legacy.Original, Upstream: base, Legacy: expected}
	for _, todo := range legacy.Remaining {
		if !validOID(todo.OID) || strings.ContainsRune(todo.Message, 0) {
			return "", fmt.Errorf("invalid commit in legacy interactive rebase")
		}
		switch todo.Action {
		case RebaseDrop:
		case RebasePick, RebaseReword, RebaseEdit, RebaseSquash, RebaseFixup:
			state.Remaining = append(state.Remaining, todo)
		default:
			return "", fmt.Errorf("invalid action in legacy interactive rebase")
		}
	}
	id, err := r.saveInteractivePause(ctx, state)
	if err != nil {
		return "", err
	}
	git, err := r.workingGit(ctx, refs.Head)
	if err != nil {
		return "", err
	}
	defer git.close()
	cmd := r.command(context.WithoutCancel(ctx), "", "-c", "core.hooksPath="+git.dir, "update-ref", "--stdin", "-z")
	identity, err := r.commitIdentity(ctx)
	if err != nil {
		return "", err
	}
	withRefIdentity(cmd, identity)
	statement := "verify HEAD\x00" + refs.Head + "\x00" + (interactiveAdvance{after: id}).statement(len(refs.Head))
	published, sent, err := runReferenceTransaction(cmd, statement, func() error {
		current, err := r.readCommitState(ctx, lock.root)
		if err != nil {
			return err
		}
		if current != operation || !lock.ownsLock() || !r.headMatches(ctx, refs.HeadRef, refs.Head) {
			return fmt.Errorf("the legacy interactive rebase or its checkout changed")
		}
		return ctx.Err()
	})
	if !published && sent {
		actual, checkErr := r.interactiveRebaseRef(context.WithoutCancel(ctx))
		published = checkErr == nil && actual == id
	}
	if !published {
		return "", err
	}
	return id, nil
}

// The old pause file named no branch. Require the same replay transition
// in the worktree's HEAD and the attached branch reflogs.
func (r *Repository) legacyRebaseBase(ctx context.Context, refs References, original string, recordedAt int64) (string, error) {
	unverified := fmt.Errorf("cannot verify the original checkout of this legacy interactive rebase")
	if !validOID(original) {
		return "", unverified
	}
	type entry struct {
		oid, message string
		when         int64
	}
	log := func(ref string) ([]entry, error) {
		output, err := r.readLimited(ctx, "", interactiveRebaseLimit,
			"reflog", "show", "-n", "8192", "--date=unix", "--format=%H%x00%gs%x00%gD", ref, "--")
		if err != nil {
			return nil, err
		}
		var entries []entry
		for line := range strings.SplitSeq(strings.TrimSuffix(string(output), "\n"), "\n") {
			if line == "" {
				continue
			}
			fields := strings.Split(line, "\x00")
			if len(fields) != 3 || !validOID(fields[0]) {
				return nil, unverified
			}
			_, elapsed, _ := strings.Cut(fields[2], "@{")
			when, err := strconv.ParseInt(strings.TrimSuffix(elapsed, "}"), 10, 64)
			if err != nil {
				return nil, unverified
			}
			entries = append(entries, entry{oid: fields[0], message: fields[1], when: when})
		}
		return entries, nil
	}
	head, err := log("HEAD")
	if err != nil {
		return "", errors.Join(unverified, err)
	}
	branch, err := log(refs.HeadRef)
	if err != nil {
		return "", errors.Join(unverified, err)
	}
	if len(head) != 0 && len(branch) != 0 && head[0] != branch[0] {
		if head[0].oid != refs.Head || !returnedToLegacyBranch(head[0].message, refs.HeadRef) {
			return "", unverified
		}
		// A recent checkout back to the unchanged owning branch may
		// precede the matching sequence in the worktree's HEAD log.
		for len(head) != 0 && head[0] != branch[0] {
			head = head[1:]
		}
	}
	for i, item := range branch {
		if i >= len(head) || item != head[i] || (i == 0 && item.oid != refs.Head) {
			break
		}
		if i > 0 && item.message == "reset: moving to "+item.oid && i+1 < len(branch) && i+1 < len(head) {
			if item.when <= recordedAt && branch[i+1].oid == original && head[i+1].oid == original {
				return item.oid, nil
			}
			break
		}
		if !strings.HasPrefix(item.message, "commit (amend): ") &&
			(!strings.HasPrefix(item.message, "commit: ") || item.when > recordedAt) {
			break
		}
	}
	return "", unverified
}

func returnedToLegacyBranch(message, ref string) bool {
	checkout, ok := strings.CutPrefix(message, "checkout: moving from ")
	_, target, found := strings.Cut(checkout, " to ")
	return ok && found && (target == ref || target == strings.TrimPrefix(ref, "refs/heads/"))
}

// Keep the old record until the new ref is durable. If the app exits
// between writes, reopening the native pause can finish this cleanup.
func (r *Repository) removeLegacyPause(ctx context.Context, nativeID, legacyVersion string) error {
	lock, err := r.lockCheckoutIndex(ctx)
	if err != nil {
		return err
	}
	defer lock.close()
	current, err := r.readCommitState(ctx, lock.root)
	if err != nil {
		return err
	}
	if current.InteractiveRebase != nativeID {
		return fmt.Errorf("the migrated interactive rebase changed")
	}
	return removeLegacyPauseFile(ctx, lock.root, legacyVersion)
}

func (r *Repository) discardLegacyPauseWithoutOrigin(ctx context.Context, expected string) (bool, error) {
	lock, err := r.lockCheckoutIndex(ctx)
	if err != nil {
		return false, err
	}
	defer lock.close()
	operation, err := r.readCommitState(ctx, lock.root)
	if err != nil {
		return false, err
	}
	if operation.InteractiveRebase != expected {
		return false, fmt.Errorf("the legacy interactive rebase changed since it was shown")
	}
	raw, err := readOperationFile(lock.root, legacyInteractiveRebaseFile)
	if err != nil {
		return false, err
	}
	if legacyPauseVersion(raw) != expected {
		return false, fmt.Errorf("the legacy interactive rebase changed while reading")
	}
	var legacy legacyInteractivePause
	if err := json.Unmarshal([]byte(raw), &legacy); err != nil {
		return false, err
	}
	if legacy.Original != "" {
		return false, nil
	}
	return true, removeLegacyPauseFile(ctx, lock.root, expected)
}

func removeLegacyPauseFile(ctx context.Context, root *os.Root, legacyVersion string) error {
	item, err := inspectAbortGroup(ctx, root, legacyInteractiveRebaseFile, nil)
	if err != nil {
		return err
	}
	if len(item.before) == 0 {
		return nil
	}
	stamp := item.before[legacyInteractiveRebaseFile]
	if stamp.info == nil || !stamp.info.Mode().IsRegular() || fmt.Sprintf("%s%x", legacyPausePrefix, stamp.sum) != legacyVersion {
		return fmt.Errorf("the legacy interactive rebase file changed after migration")
	}
	swap, err := item.prepare(ctx, root, nil, nil, nil, false)
	if err != nil {
		return err
	}
	defer swap.close()
	if err := swap.apply(ctx, root); err != nil {
		return errors.Join(err, swap.restore(context.WithoutCancel(ctx)))
	}
	if err := swap.finish(context.WithoutCancel(ctx)); err != nil {
		return err
	}
	if _, err := root.Lstat(legacyInteractiveRebaseFile); !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("the legacy interactive rebase file was replaced during migration")
	}
	return nil
}
