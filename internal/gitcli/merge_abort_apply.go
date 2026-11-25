package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Apply restores the merge paths without resetting the whole checkout. A
// warning identifies recovery files kept after a successful abort.
func (p *MergeAbortPlan) Apply(ctx context.Context) (warning error, err error) {
	r := p.repo
	lock, err := r.lockCheckoutIndex(ctx)
	if err != nil {
		return nil, err
	}
	defer lock.close()
	root, err := os.OpenRoot(r.WorkTree)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := checkCheckoutAliases(root, p.paths); err != nil {
		return nil, err
	}
	git, err := r.workingGit(ctx, p.local.OID)
	if err != nil {
		return nil, err
	}
	defer git.close()
	live := *git
	if err := p.verify(ctx, &live, lock.root); err != nil {
		return nil, err
	}
	index, err := p.prepareIndex(ctx, lock, root, git)
	if err != nil {
		return nil, err
	}
	source, err := os.OpenRoot(git.workTree)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	var worktree, metadata []*abortSwap
	defer func() {
		for _, list := range [][]*abortSwap{worktree, metadata} {
			for _, swap := range list {
				swap.close()
			}
		}
	}()
	for _, group := range p.groups {
		swap, err := group.prepare(ctx, root, source, index.files, index.modes, index.symlinks)
		if err != nil {
			return nil, err
		}
		if swap != nil {
			worktree = append(worktree, swap)
		}
	}
	stateParent, err := lock.root.Stat(".")
	if err != nil {
		return nil, err
	}
	for _, name := range abortStateNames {
		stamp, found := p.stateDir.before[name]
		if !found {
			continue
		}
		group := abortGroup{path: name, parent: stateParent, before: map[string]discardStamp{name: stamp}}
		swap, err := group.prepare(ctx, lock.root, nil, nil, nil, false)
		if err != nil {
			return nil, err
		}
		metadata = append(metadata, swap)
	}
	if err := writeLockedIndex(lock.root, lock.file, git.index); err != nil {
		return nil, err
	}
	finalize := context.WithoutCancel(ctx)
	if p.SavesAutostash() {
		if err := p.moveAutostash(ctx, finalize, &live, lock); err != nil {
			return nil, err
		}
		// A later error can leave a regular in-progress merge and a saved stash.
		next := *p
		next.state.MergeAutostash, next.state.Continuation = "", ""
		p = &next
	}
	cmd := r.command(finalize, "", "-c", "core.hooksPath="+git.dir, "update-ref", "--stdin", "-z")
	installed := false
	var applied []*abortSwap
	published, _, txErr := runReferenceTransaction(cmd, p.referenceStatement(), func() error {
		if err := p.verify(ctx, &live, lock.root); err != nil {
			return err
		}
		if err := r.checkIndexCheckout(ctx); err != nil {
			return err
		}
		if !lock.ownsLock() {
			return fmt.Errorf("the Git index lock changed before aborting")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		for _, swap := range worktree {
			applied = append(applied, swap)
			if err := swap.apply(finalize, root); err != nil {
				return err
			}
		}
		for _, swap := range metadata {
			applied = append(applied, swap)
			if err := swap.apply(finalize, lock.root); err != nil {
				return err
			}
		}
		remaining, err := inspectMergeState(finalize, lock.root)
		if err != nil || len(remaining.before) != 0 {
			return errors.Join(err, fmt.Errorf("the merge state changed during abort"))
		}
		if err := r.checkIndexCheckout(finalize); err != nil {
			return err
		}
		if err := installLockedIndex(lock.root, lock.info); err != nil {
			return err
		}
		installed = true
		return nil
	})
	if !installed {
		for i := len(applied) - 1; i >= 0; i-- {
			txErr = errors.Join(txErr, applied[i].restore(finalize))
		}
		return nil, txErr
	}
	if !published && p.autoTree != "" {
		current, err := r.exactRootRef(finalize, "AUTO_MERGE")
		if err != nil || current != "" {
			warning = errors.Join(txErr, err, fmt.Errorf("could not clear AUTO_MERGE in %q", filepath.Clean(r.GitDir)))
		}
	}
	for _, swap := range applied {
		warning = errors.Join(warning, swap.finish(finalize))
	}
	return warning, nil
}

// Keep the merge retryable if its checkout cannot be published. The original
// autostash is durable before the merge becomes a regular in-progress merge.
func (p *MergeAbortPlan) moveAutostash(ctx, finalize context.Context, git *workingGit, lock *checkoutIndexLock) error {
	r, oid := p.repo, p.state.MergeAutostash
	if err := p.verify(ctx, git, lock.root); err != nil {
		return err
	}
	tip, err := r.exactRootRef(ctx, stashRef)
	if err != nil {
		return err
	}
	entries, err := r.Stashes(ctx)
	if err != nil {
		return err
	}
	if tip != "" && (len(entries) == 0 || entries[0].OID != tip) {
		return fmt.Errorf("the existing stash reference does not match its history")
	}
	isAutostash := func(entry StashEntry) bool { return entry.OID == oid }
	if !slices.ContainsFunc(entries, isAutostash) {
		if tip == "" {
			tip = strings.Repeat("0", len(oid))
		}
		if err := r.editDirectRef(ctx, stashRef, tip, oid, "On merge abort: autostash", nil); err != nil {
			return fmt.Errorf("could not save the merge autostash: %w", err)
		}
		tip = oid
	}
	cmd := r.command(finalize, "", "-c", "core.hooksPath="+git.dir, "update-ref", "--stdin", "-z")
	statement := verifyCheckoutRef(stashRef, tip) + "option no-deref\x00delete MERGE_AUTOSTASH\x00" + oid + "\x00"
	moved, sent, err := runReferenceTransaction(cmd, statement, func() error {
		if !lock.ownsLock() {
			return fmt.Errorf("the Git index lock changed before saving the autostash")
		}
		entries, err := r.Stashes(ctx)
		if err != nil || !slices.ContainsFunc(entries, isAutostash) {
			return errors.Join(err, fmt.Errorf("the saved autostash is no longer in the stash history"))
		}
		return p.verify(ctx, git, lock.root)
	})
	if moved {
		return nil
	}
	if sent {
		current, readErr := r.exactRootRef(finalize, "MERGE_AUTOSTASH")
		if readErr == nil && current == "" {
			return nil
		}
		err = errors.Join(err, readErr)
	}
	return fmt.Errorf("the autostash was saved, but Git could not clear its merge reference: %w", err)
}

func (p *MergeAbortPlan) referenceStatement() string {
	zero := strings.Repeat("0", len(p.local.OID))
	original := p.original
	if original == "" {
		original = zero
	}
	statement := "verify HEAD\x00" + p.local.OID + "\x00" + verifyCheckoutRef("ORIG_HEAD", original)
	statement += verifyCheckoutRef("MERGE_AUTOSTASH", zero)
	if p.autoTree == "" {
		return statement + verifyCheckoutRef("AUTO_MERGE", zero)
	}
	return statement + deleteMergeTree(p.autoTree)
}
