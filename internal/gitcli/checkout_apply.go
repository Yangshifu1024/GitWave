package gitcli

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func (g *workingGit) prepareCheckoutConfig(ctx context.Context, old, next string, paths []string) error {
	g.settings = append(g.settings, "-c", "core.splitIndex=false")
	if old == next {
		return nil
	}
	for _, key := range []string{"core.sparsecheckout", "core.sparsecheckoutcone", "index.sparse"} {
		value, err := g.repo.optionalRef(ctx, "config", "--type=bool", "--get", key)
		if err != nil {
			return err
		}
		if value != "" {
			g.settings = append(g.settings, "-c", key+"="+value)
		}
		if key == "core.sparsecheckout" && value == "true" {
			path, err := g.repo.readLimited(ctx, "", 64<<10, "rev-parse", "--path-format=absolute", "--git-path", "info/sparse-checkout")
			if err != nil {
				return err
			}
			if err := copyGitMetadata(strings.TrimSuffix(string(path), "\n"), filepath.Join(g.dir, "info", "sparse-checkout"), 16<<20); err != nil {
				return err
			}
		}
	}
	if err := g.freezeWorktreeAttributes(ctx); err != nil {
		return err
	}
	if len(paths) == 0 {
		return nil
	}
	if err := g.configureLocalLFS(false, true); err != nil {
		return err
	}
	if err := g.checkFilters(ctx, paths, "switching to", "--source="+next); err != nil {
		return err
	}
	if err := g.checkWorktreeFilters(ctx, paths, "switching from", false); err != nil {
		return err
	}
	// These string-valued filter names are ambiguous in check-attr output.
	g.settings = append(g.settings, "-c", "filter.set.required=true", "-c", "filter.unset.required=true",
		"-c", "filter.unspecified.required=true")
	return nil
}

func (g *workingGit) checkoutPaths(ctx context.Context, old, next string) ([]string, error) {
	if old == next {
		return nil, nil
	}
	args := []string{"diff-tree", "-r", "--no-renames", "--no-commit-id", "--no-ext-diff", "--name-only", "-z", old, next, "--"}
	if old == "" {
		args = []string{"ls-tree", "-r", "--name-only", "-z", next, "--"}
	}
	out, err := g.read(ctx, 32<<20, args...)
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, nil
	}
	paths := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	for _, path := range paths {
		if !validIndexPath(path) {
			return nil, fmt.Errorf("unsupported checkout path %q", path)
		}
	}
	return paths, nil
}

func (p CheckoutPlan) referenceStatement(previousOriginal, previousAutoMerge string) string {
	statement := "option no-deref\x00"
	old := p.source.head
	if old == "" {
		old = strings.Repeat("0", len(p.dest.head))
	}
	if p.advance {
		// Dereferencing HEAD locks both its attachment and the current
		// branch. The prepared callback also verifies the attachment name.
		recordOriginal := ""
		if p.merge != nil || p.rebase || p.hardReset {
			if previousOriginal == "" {
				previousOriginal = strings.Repeat("0", len(old))
			}
			recordOriginal = "option no-deref\x00update ORIG_HEAD\x00" + old + "\x00" + previousOriginal + "\x00"
		}
		if p.merge != nil && p.merge.paused {
			if previousAutoMerge == "" {
				previousAutoMerge = strings.Repeat("0", len(old))
			}
			recordTree := "option no-deref\x00update AUTO_MERGE\x00" + p.merge.tree + "\x00" + previousAutoMerge + "\x00"
			return "verify HEAD\x00" + old + "\x00" + verifyCheckoutRef(p.request.Ref, p.request.OID) + recordOriginal + recordTree
		}
		statement := "update HEAD\x00" + p.dest.head + "\x00" + old + "\x00"
		if p.request.Ref != "" {
			statement += verifyCheckoutRef(p.request.Ref, p.request.OID)
		}
		if p.interactive != nil {
			statement += p.interactive.statement(len(old))
		}
		return statement + recordOriginal
	}
	if p.dest.branch == "" {
		// A dangling symbolic HEAD is present even though its branch has
		// no object. Verify it in the prepared callback instead.
		statement += "update HEAD\x00" + p.dest.head + "\x00" + p.source.head + "\x00"
	} else {
		statement += "symref-update HEAD\x00" + p.dest.branch + "\x00"
		if p.source.attached == "" {
			statement += "oid\x00" + old + "\x00"
		} else {
			statement += "ref\x00" + p.source.attached + "\x00"
		}
		if p.dest.upstream != "" {
			statement += "create " + p.dest.branch + "\x00" + p.dest.head + "\x00"
		} else {
			statement += verifyCheckoutRef(p.dest.branch, p.dest.head)
		}
	}
	if p.request.Ref != "" && p.request.Ref != p.dest.branch {
		statement += verifyCheckoutRef(p.request.Ref, p.request.OID)
	}
	return statement
}

func verifyCheckoutRef(ref, oid string) string {
	return "option no-deref\x00verify " + ref + "\x00" + oid + "\x00"
}

func (p CheckoutPlan) publishCheckout(ctx context.Context, lock *checkoutIndexLock, git, source, target *workingGit,
	paths []string, dirty *checkoutCleanup) error {
	r := p.repo
	applyTree := p.source.head != p.dest.head
	oldTree := p.source.head
	if oldTree == "" {
		output, err := git.read(ctx, 4096, "mktree")
		if err != nil {
			return err
		}
		oldTree = strings.TrimSpace(string(output))
		if !validOID(oldTree) {
			return fmt.Errorf("invalid empty Git tree")
		}
	}
	gitlinks, err := git.removedCheckoutGitlinks(ctx, oldTree, p.dest.head)
	if err != nil {
		return err
	}
	from := strings.TrimPrefix(p.source.resolved, "refs/heads/")
	if from == "" {
		from = p.source.head
	}
	// Cancellation remains effective through preparation. Once worktree
	// writes begin, finish publication or recovery before returning.
	finalize := context.WithoutCancel(ctx)
	note := "checkout: moving from " + from + " to " + p.dest.name
	if p.advance {
		note = "pull: Fast-forward"
		if p.request.OID != p.dest.head {
			note = "pull --rebase: Finished"
		}
	}
	if p.merge != nil {
		note = "merge: " + p.merge.message
	} else if p.interactive != nil {
		note = "rebase -i: onto " + p.request.OID
	} else if p.rebase {
		note = "rebase: onto " + p.request.OID
	} else if p.advanceNote != "" {
		note = p.advanceNote
	}
	var previousOriginal, previousAutoMerge string
	if p.merge != nil || p.rebase || p.hardReset {
		var err error
		previousOriginal, err = r.exactRootRef(ctx, "ORIG_HEAD")
		if err != nil {
			return err
		}
		if p.merge != nil && p.merge.paused {
			previousAutoMerge, err = r.exactRootRef(ctx, "AUTO_MERGE")
			if err != nil {
				return err
			}
		}
	}
	cmd := r.command(finalize, "", "-c", "core.hooksPath="+git.dir, "update-ref", "--stdin", "-z", "--create-reflog",
		"-m", note)
	identity, err := r.commitIdentity(ctx)
	if err != nil {
		return err
	}
	withRefIdentity(cmd, identity)

	treeAttempted, treeComplete, conflictSaved := false, false, false
	published, sentCommit, txErr := runReferenceTransaction(cmd, p.referenceStatement(previousOriginal, previousAutoMerge), func() error {
		if err := p.checkPreparedCheckout(ctx, lock, git, dirty); err != nil {
			return err
		}
		if !applyTree && dirty == nil {
			return nil
		}
		if applyTree && dirty == nil {
			// read-tree's dry run compares index stat data. Refresh the
			// private index as checkout does so it can compare LFS content
			// through the clean filter before declaring a path dirty.
			if len(git.lfsEnvironment) != 0 {
				if err := git.runCheckoutCommand(ctx, "", "update-index", "-q", "--refresh", "--ignore-missing"); err != nil {
					return err
				}
			}
			if err := git.runCheckoutCommand(ctx, "", "read-tree", "-n", "-m", "-u", oldTree, p.dest.head); err != nil {
				return err
			}
		}
		if applyTree {
			if err := git.protectCheckoutGitlinks(gitlinks); err != nil {
				return err
			}
		}
		if dirty != nil {
			if err := dirty.protectIgnored(ctx, git, p.dest.head, paths); err != nil {
				return err
			}
		}
		if applyTree && p.hardReset {
			untracked, err := git.read(ctx, 64<<20, "ls-files", "--others", "-z", "--")
			if err != nil {
				return err
			}
			if err := git.checkCheckoutPathCollisions(ctx, p.dest.head, paths, untracked, "untracked"); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := target.verifyLFSPathScope(ctx); err != nil {
			return err
		}
		if dirty != nil {
			if err := dirty.reset(finalize, git, p.source.head); err != nil {
				return err
			}
		}
		if applyTree {
			if dirty != nil {
				if err := git.protectCheckoutGitlinks(gitlinks); err != nil {
					return err
				}
			}
			if dirty != nil && p.hardReset {
				if err := git.runCheckoutCommand(finalize, "", "update-index", "--refresh"); err != nil {
					return err
				}
				if err := git.runCheckoutCommand(finalize, "", "read-tree", "-n", "-m", "-u", oldTree, p.dest.head); err != nil {
					return err
				}
			}
			treeAttempted = true
			// The private HEAD and index retain Git's checkout safety checks
			// without loading the live repository's executable settings.
			if err := target.switchPrivateHead(finalize, p.dest.head); err != nil {
				return fmt.Errorf("could not finish writing the checkout worktree: %w", err)
			}
			treeComplete = true
			if err := git.checkCheckoutResult(finalize, p.dest.head, paths); err != nil {
				return err
			}
			if err := p.downloadedLFS.verify(finalize); err != nil {
				return err
			}
		}
		if dirty != nil && dirty.mode == CheckoutStash {
			// Git may produce both useful changes and conflict index
			// entries even when applying the entry reports a conflict.
			dirty.replayed = true
			dirty.replayErr = git.runCheckoutCommand(finalize, "", "stash", "apply", "--quiet", "--index", dirty.stash.oid)
		}
		if p.merge != nil && p.merge.paused {
			if err := p.merge.publish(finalize, lock, git, p.request.OID); err != nil {
				return err
			}
			conflictSaved = true
			return nil
		}
		if err := writeLockedIndex(lock.root, lock.file, git.index); err != nil {
			return err
		}
		if !lock.ownsLock() {
			return fmt.Errorf("the Git index lock changed during checkout")
		}
		return nil
	})
	if conflictSaved {
		if !published && txErr != nil {
			return fmt.Errorf("merge conflicts are ready to resolve, but Git could not save the original HEAD and merge tree: %w", txErr)
		}
		return nil
	}

	recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if !published && sentCommit {
		// A child cancelled after receiving commit may have completed HEAD.
		published = r.headMatches(recoveryCtx, p.dest.branch, p.dest.head)
		if published && p.hardReset {
			observed, err := r.exactRootRef(recoveryCtx, "ORIG_HEAD")
			// A reset to the current commit can leave both references
			// unchanged. Those values cannot prove Git committed the reset.
			changedRef := p.source.head != p.dest.head || previousOriginal != p.source.head
			published = err == nil && observed == p.source.head && changedRef
		}
		if published && p.interactive != nil {
			observed, err := r.interactiveRebaseRef(recoveryCtx)
			published = err == nil && observed == p.interactive.after
		}
	}
	if published {
		if !applyTree && dirty == nil {
			return nil
		}
		if err := installLockedIndex(lock.root, lock.info); err != nil {
			if lock.ownsLock() {
				backup := "gitwave-checkout-index-" + rand.Text()
				if saveErr := lock.root.Rename("index.lock", backup); saveErr == nil {
					return fmt.Errorf("HEAD switched but its index could not be installed. New index saved at %s: %w", filepath.Join(r.GitDir, backup), err)
				}
			}
			return fmt.Errorf("HEAD switched but its index could not be installed: %w", err)
		}
		return nil
	}
	if treeAttempted && (dirty == nil || !dirty.replayed) {
		if !r.headMatches(recoveryCtx, p.source.attached, p.source.head) || !lock.ownsLock() {
			return fmt.Errorf("checkout did not finish and its worktree could not be safely restored: %w", txErr)
		}
		if err := p.restoreCheckoutTree(recoveryCtx, source, paths, oldTree, treeComplete); err != nil {
			return errors.Join(txErr, fmt.Errorf("could not restore the prior worktree: %w", err))
		}
	}
	if dirty != nil && dirty.cleaned && !dirty.replayed && p.source.head != "" &&
		lock.ownsLock() && r.headMatches(recoveryCtx, p.source.attached, p.source.head) {
		// Reapply the original state only when the failed checkout
		// left a clean source. Preserve the recovery entry otherwise.
		status, err := git.checkoutStatus(recoveryCtx)
		if err == nil && status.HeadOID == p.source.head && dirty.cleanSource(status) {
			if err := git.runCheckoutCommand(recoveryCtx, "", "stash", "apply", "--quiet", "--index", dirty.stash.oid); err != nil {
				return errors.Join(txErr, fmt.Errorf("could not restore the prior working changes: %w", err))
			}
			dirty.restoredSource = true
		}
	}
	return txErr
}

func (p CheckoutPlan) restoreCheckoutTree(ctx context.Context, git *workingGit, paths []string, oldTree string, complete bool) error {
	if !complete {
		if err := git.checkCheckoutResult(ctx, p.source.head, paths); err == nil {
			return nil
		}
	}
	gitlinks, err := git.removedCheckoutGitlinks(ctx, p.dest.head, oldTree)
	if err != nil {
		return err
	}
	if err := git.protectCheckoutGitlinks(gitlinks); err != nil {
		return err
	}
	if !complete {
		// Git may fail after writing paths but before advancing its
		// private HEAD. Seed the reverse checkout from the target.
		if err := git.runCheckoutCommand(ctx, "", "read-tree", p.dest.head); err != nil {
			return err
		}
		if _, err := git.read(ctx, 64<<10, "update-ref", "--no-deref", "HEAD", p.dest.head); err != nil {
			return err
		}
	}
	if p.source.head == "" {
		return git.runCheckoutCommand(ctx, "", "read-tree", "-m", "-u", p.dest.head, oldTree)
	}
	return git.switchPrivateHead(ctx, p.source.head)
}

func (g *workingGit) switchPrivateHead(ctx context.Context, oid string) error {
	return g.runCheckoutCommand(ctx, "", "checkout", "--quiet", "--detach", "--no-overwrite-ignore", "--no-recurse-submodules", oid)
}

func (g *workingGit) runCheckoutCommand(ctx context.Context, input string, args ...string) error {
	cmd := g.command(ctx, input, args...)
	var diagnostic gitDiagnostic
	cmd.Stderr = &diagnostic
	err := cmd.Run()
	if message := strings.TrimSpace(diagnostic.buffer.String()); message != "" {
		return fmt.Errorf("Git %s: %s", args[0], message)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func (g *workingGit) checkCheckoutResult(ctx context.Context, oid string, paths []string) error {
	changed := make(map[string]bool, len(paths))
	for _, path := range paths {
		changed[path] = true
	}
	status, err := g.checkoutStatus(ctx)
	if err != nil {
		return err
	}
	head := status.HeadOID
	if head == "(initial)" {
		head = ""
	}
	if head != oid {
		return fmt.Errorf("private HEAD did not match the checkout snapshot")
	}
	for _, change := range status.Changes {
		if changed[change.Path] && !strings.HasPrefix(change.Submodule, "S") {
			return fmt.Errorf("Git did not materialize the selected version of %q", change.Path)
		}
	}
	return nil
}

func (p CheckoutPlan) checkPreparedCheckout(ctx context.Context, lock *checkoutIndexLock, working *workingGit, dirty *checkoutCleanup) error {
	r := p.repo
	if !lock.ownsLock() {
		return fmt.Errorf("the Git index lock changed before checkout")
	}
	if err := r.checkIndexCheckout(ctx); err != nil {
		return err
	}
	if err := working.verifyLocalLFSConfig(ctx); err != nil {
		return err
	}
	if p.downloadedLFS != nil && p.downloadedLFS.directory != nil {
		if working.localLFS != p.downloadedLFS.config {
			return fmt.Errorf("the Git LFS checkout settings changed after downloading")
		}
		if err := p.downloadedLFS.verify(ctx); err != nil {
			return err
		}
	}
	if err := p.checkOperation(ctx); err != nil {
		return err
	}
	if !r.headMatches(ctx, p.source.attached, p.source.head) {
		return fmt.Errorf("HEAD changed before checkout")
	}
	if p.merge != nil || p.rebase || p.hardReset {
		original, err := r.optionalRef(ctx, "symbolic-ref", "--no-recurse", "-q", "ORIG_HEAD")
		if err != nil {
			return err
		}
		if original != "" {
			return fmt.Errorf("ORIG_HEAD is a symbolic reference")
		}
	}
	if p.request.Ref != "" {
		ref, err := r.optionalRef(ctx, "symbolic-ref", "--no-recurse", "-q", p.request.Ref)
		if err != nil {
			return err
		}
		if ref != "" {
			return fmt.Errorf("the checkout target changed into a symbolic ref")
		}
	}
	if p.dest.branch != "" {
		if err := r.protectBranchWorktrees(ctx, p.dest.branch, p.advance); err != nil {
			return err
		}
		ref, err := r.optionalRef(ctx, "symbolic-ref", "--no-recurse", "-q", p.dest.branch)
		if err != nil {
			return err
		}
		if ref != "" {
			return fmt.Errorf("the local checkout branch changed into a symbolic ref")
		}
	}
	if dirty != nil {
		return dirty.state.verify(ctx, dirty.root, working)
	}
	if p.keepLocal && p.DirtyFiles != 0 {
		root, err := os.OpenRoot(r.WorkTree)
		if err != nil {
			return err
		}
		defer root.Close()
		return p.dirty.verify(ctx, root, working)
	}
	status, err := working.checkoutStatus(ctx)
	if err != nil {
		return err
	}
	if p.hardReset {
		status.Changes = slices.DeleteFunc(status.Changes, untrackedChange)
	}
	if len(status.Changes) != 0 {
		return fmt.Errorf("commit or stash changes before switching")
	}
	return ctx.Err()
}

func (r *Repository) headMatches(ctx context.Context, ref, head string) bool {
	actualRef, err := r.optionalRef(ctx, "symbolic-ref", "--no-recurse", "-q", "HEAD")
	if err != nil || actualRef != ref {
		return false
	}
	actualHead, err := r.optionalRef(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	return err == nil && actualHead == head
}
