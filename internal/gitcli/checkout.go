package gitcli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// CheckoutTarget identifies the requested ref and the exact object shown in
// the UI. An empty Ref names a commit. A tag's OID may be an annotated object.
type CheckoutTarget struct{ Ref, OID string }

type CheckoutPlan struct {
	repo          *Repository
	request       CheckoutTarget
	source        checkoutSource
	dest          checkoutDestination
	dirty         checkoutDirty
	advance       bool
	rebase        bool
	hardReset     bool
	keepLocal     bool
	advanceNote   string
	merge         *mergeCheckout
	interactive   *interactiveAdvance
	downloadedLFS *lfsDownloadReceipt
	DirtyFiles    int
	Current       bool
}

type checkoutSource struct {
	head, resolved, attached string
}

type checkoutDestination struct {
	head, branch, name, upstream string
}

type CheckoutResult struct {
	Name, OID, Warning string
	Created            bool
	Reapplied          bool
}

func (p CheckoutPlan) Name() string { return p.dest.name }

func (p CheckoutPlan) StartsAt(source CheckoutTarget) bool {
	return source.OID == p.source.head && source.Ref == p.source.resolved
}

func (r *Repository) PrepareCheckout(ctx context.Context, request CheckoutTarget) (*CheckoutPlan, error) {
	return r.prepareCheckout(ctx, request, "")
}

func (r *Repository) prepareCheckout(ctx context.Context, request CheckoutTarget, advanceTo string) (*CheckoutPlan, error) {
	return r.prepareCheckoutPlan(ctx, request, advanceTo, nil, nil, false)
}

func (r *Repository) prepareCheckoutPlan(ctx context.Context, request CheckoutTarget, advanceTo string, merge *mergeCheckout, interactive *interactiveAdvance, hardReset bool) (*CheckoutPlan, error) {
	if r.WorkTree == "" {
		return nil, fmt.Errorf("this repository has no working copy")
	}
	if !validOID(request.OID) {
		return nil, fmt.Errorf("select a commit to check out")
	}
	refs, err := r.References(ctx)
	if err != nil {
		return nil, err
	}
	attached, err := r.optionalRef(ctx, "symbolic-ref", "--no-recurse", "-q", "HEAD")
	if err != nil {
		return nil, err
	}
	plan := &CheckoutPlan{repo: r, request: request, advance: advanceTo != "", merge: merge, interactive: interactive, hardReset: hardReset,
		source: checkoutSource{head: refs.Head, resolved: refs.HeadRef, attached: attached}}
	if plan.advance {
		if request.Ref != "" {
			if err := checkRefTip(refs, request.Ref, request.OID); err != nil {
				return nil, err
			}
		} else if err := r.requireCommit(ctx, request.OID); err != nil {
			return nil, err
		}
		if err := r.requireCommit(ctx, advanceTo); err != nil {
			return nil, err
		}
		detachedAdvance := attached == "" && (merge != nil || request.Ref == "")
		if attached != refs.HeadRef || (!strings.HasPrefix(attached, "refs/heads/") && !detachedAdvance) {
			return nil, fmt.Errorf("the target is no longer the expected checkout")
		}
		plan.dest = checkoutDestination{head: advanceTo, branch: attached, name: strings.TrimPrefix(attached, "refs/heads/")}
		if err := plan.captureWorkingCopy(ctx, true); err != nil {
			return nil, err
		}
		return plan, nil
	}
	dest, err := r.resolveCheckout(ctx, request, refs)
	if err != nil {
		return nil, err
	}
	plan.dest = dest
	if (dest.branch != "" && dest.branch == refs.HeadRef && dest.upstream == "") ||
		(dest.branch == "" && refs.HeadRef == "" && dest.head == refs.Head) {
		plan.Current = true
		return plan, nil
	}
	if err := plan.captureWorkingCopy(ctx, true); err != nil {
		return nil, err
	}
	return plan, nil
}

func (p *CheckoutPlan) captureWorkingCopy(ctx context.Context, captureFiles bool) error {
	r := p.repo
	if err := p.checkOperation(ctx); err != nil {
		return err
	}
	if p.dest.branch != "" {
		if err := r.protectBranchWorktrees(ctx, p.dest.branch, p.advance); err != nil {
			return err
		}
	}
	working, err := r.workingGit(ctx, p.source.head)
	if err != nil {
		return err
	}
	defer working.close()
	status, err := working.checkoutStatus(ctx)
	if err != nil {
		return err
	}
	if p.hardReset {
		status.Changes = slices.DeleteFunc(status.Changes, untrackedChange)
		if err := working.checkSnapshotChanges(ctx, status.Changes); err != nil {
			return err
		}
	}
	for _, change := range status.Changes {
		if change.Kind == 'u' {
			return fmt.Errorf("resolve all index conflicts before changing the checkout")
		}
	}
	if len(status.Changes) != 0 && captureFiles {
		p.dirty, err = r.captureCheckoutDirty(ctx, status.Changes)
		if err != nil {
			return err
		}
		if p.hardReset {
			p.dirty.trackedOnly = true
			if err := p.dirty.protectUntrackedReplacements(); err != nil {
				return err
			}
		}
		p.DirtyFiles = len(p.dirty.files)
	} else {
		p.DirtyFiles = len(status.Changes)
	}
	return nil
}

func (r *Repository) resolveCheckout(ctx context.Context, request CheckoutTarget, refs References) (checkoutDestination, error) {
	if request.Ref == "" {
		if err := r.requireCommit(ctx, request.OID); err != nil {
			return checkoutDestination{}, err
		}
		return checkoutDestination{head: request.OID, name: request.OID[:7]}, nil
	}
	if err := checkRefTip(refs, request.Ref, request.OID); err != nil {
		return checkoutDestination{}, err
	}
	ref := findRef(refs, request.Ref)
	tip, err := r.ReferenceCommit(ctx, ref)
	if err != nil {
		return checkoutDestination{}, err
	}
	if tip == "" {
		return checkoutDestination{}, fmt.Errorf("the selected reference does not point to a commit")
	}
	switch {
	case strings.HasPrefix(ref.Name, "refs/tags/"):
		return checkoutDestination{head: tip, name: strings.TrimPrefix(ref.Name, "refs/tags/")}, nil
	case strings.HasPrefix(ref.Name, "refs/heads/"):
		return checkoutDestination{head: tip, branch: ref.Name, name: strings.TrimPrefix(ref.Name, "refs/heads/")}, nil
	case strings.HasPrefix(ref.Name, "refs/remotes/"):
		name, err := r.localNameForRemote(ctx, strings.TrimPrefix(ref.Name, "refs/remotes/"))
		if err != nil {
			return checkoutDestination{}, err
		}
		full, err := r.checkedRefName(ctx, "heads", name)
		if err != nil {
			return checkoutDestination{}, err
		}
		if local := findRef(refs, full); local.OID != "" {
			if local.Symbolic != "" || local.Type != "commit" {
				return checkoutDestination{}, fmt.Errorf("local branch %q does not directly point to a commit", name)
			}
			return checkoutDestination{head: local.OID, branch: full, name: name}, nil
		}
		return checkoutDestination{head: tip, branch: full, name: name, upstream: ref.Name}, nil
	default:
		return checkoutDestination{}, fmt.Errorf("this reference cannot be checked out")
	}
}

func (r *Repository) localNameForRemote(ctx context.Context, name string) (string, error) {
	out, err := r.readLimited(ctx, "", 1<<20, "remote")
	if err != nil {
		return "", err
	}
	prefix := ""
	for remote := range strings.SplitSeq(string(out), "\n") {
		if remote != "" && strings.HasPrefix(name, remote+"/") && len(remote) > len(prefix) {
			prefix = remote
		}
	}
	if prefix == "" {
		return "", fmt.Errorf("no configured remote owns %q", name)
	}
	return strings.TrimPrefix(name, prefix+"/"), nil
}

func (p *CheckoutPlan) checkOperation(ctx context.Context) error {
	expected := ""
	if p.interactive != nil {
		expected = p.interactive.before
	}
	if expected != "" {
		if _, err := os.Lstat(filepath.Join(p.repo.GitDir, legacyInteractiveRebaseFile)); err == nil {
			return fmt.Errorf("a legacy interactive rebase file appeared during checkout")
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return p.repo.checkCheckoutOperationDuring(ctx, expected)
}

func (r *Repository) checkCheckoutOperation(ctx context.Context) error {
	return r.checkCheckoutOperationDuring(ctx, "")
}

func (r *Repository) checkCheckoutOperationDuring(ctx context.Context, pause string) error {
	state, err := r.commitState(ctx)
	if err != nil {
		return err
	}
	if state.MergeHeads != "" {
		return fmt.Errorf("finish or abort the merge before switching")
	}
	if state.Continuation != "" && (pause == "" || state.Continuation != interactiveRebaseOperation || state.InteractiveRebase != pause) {
		return fmt.Errorf("finish or abort the %s before switching", state.Continuation)
	}
	if pause != "" && state.InteractiveRebase != pause {
		return fmt.Errorf("the interactive rebase changed since it was shown")
	}
	root, err := os.OpenRoot(r.GitDir)
	if err != nil {
		return err
	}
	defer root.Close()
	bisect, err := readOperationFile(root, "BISECT_START")
	if err != nil {
		return err
	}
	if bisect != "" {
		return fmt.Errorf("finish or reset the bisect before switching")
	}
	return ctx.Err()
}

func (g *workingGit) checkoutStatus(ctx context.Context) (Status, error) {
	if err := g.configureLocalLFS(false, false); err != nil {
		return Status{}, err
	}
	out, err := g.read(ctx, 32<<20, "status", "--porcelain=v2", "-z", "--branch",
		"--untracked-files=all", "--no-renames", "--ignore-submodules=dirty")
	if err != nil {
		return Status{}, err
	}
	return parseStatus(string(out), true)
}

// Apply executes a clean checkout after revalidating both ends of the plan.
func (p CheckoutPlan) Apply(ctx context.Context) (CheckoutResult, error) {
	return p.apply(ctx, CheckoutClean, "")
}

type CheckoutMode uint8

const (
	CheckoutClean CheckoutMode = iota
	CheckoutDiscard
	CheckoutStash
)

func (p CheckoutPlan) ApplyDirty(ctx context.Context, mode CheckoutMode, message string) (CheckoutResult, error) {
	if mode != CheckoutDiscard && mode != CheckoutStash {
		return CheckoutResult{}, fmt.Errorf("select how to handle uncommitted changes")
	}
	return p.apply(ctx, mode, message)
}

func (p CheckoutPlan) apply(ctx context.Context, mode CheckoutMode, message string) (CheckoutResult, error) {
	result := CheckoutResult{Name: p.dest.name, OID: p.dest.head}
	r := p.repo
	lock, err := r.lockCheckoutIndex(ctx)
	if err != nil {
		return CheckoutResult{}, err
	}
	defer lock.close()
	advanceTo := ""
	if p.advance {
		advanceTo = p.dest.head
	}
	current, err := r.prepareCheckoutPlan(ctx, p.request, advanceTo, p.merge, p.interactive, p.hardReset)
	if err != nil {
		return CheckoutResult{}, err
	}
	if p.source != current.source || p.dest != current.dest {
		return CheckoutResult{}, fmt.Errorf("checkout source or target changed since it was selected")
	}
	if current.Current {
		return result, nil
	}
	if mode == CheckoutClean && !p.keepLocal && current.DirtyFiles != 0 {
		return CheckoutResult{}, fmt.Errorf("commit or stash changes before switching")
	}
	if mode != CheckoutClean && (p.DirtyFiles == 0 || !p.dirty.matches(current.dirty)) {
		return CheckoutResult{}, fmt.Errorf("the working copy changed since switching was requested")
	}
	if p.dest.branch != "" && !p.advance {
		if err := r.requireBranchCheckoutGit(ctx); err != nil {
			return CheckoutResult{}, err
		}
	}
	working, err := r.workingGit(ctx, p.source.head)
	if err != nil {
		return CheckoutResult{}, err
	}
	defer working.close()
	working.index = filepath.Join(working.dir, "index")
	if err := copyOriginalIndex(lock.root, working.index); err != nil {
		return CheckoutResult{}, err
	}
	paths, err := working.checkoutPaths(ctx, p.source.head, p.dest.head)
	if err != nil {
		return CheckoutResult{}, err
	}
	if err := working.prepareCheckoutConfig(ctx, p.source.head, p.dest.head, paths); err != nil {
		return CheckoutResult{}, err
	}
	if err := p.downloadedLFS.verify(ctx); err != nil {
		return CheckoutResult{}, err
	}
	var lfsConfig lfsConfigSource
	if mode != CheckoutClean && len(working.lfsEnvironment) != 0 {
		lfsConfig, err = r.lfsConfigAfterRestore(ctx, p.dirty.files, p.source.head, false)
		if err != nil {
			return CheckoutResult{}, err
		}
	}
	target, err := working.checkoutLFSTarget(ctx, lfsConfig, paths, p.dest.head, p.downloadedLFS)
	if err != nil {
		return CheckoutResult{}, err
	}
	source := working
	if p.source.head != p.dest.head {
		pointers, err := working.preserveWorkingLFSObjects(ctx, paths)
		if err != nil {
			return CheckoutResult{}, err
		}
		if len(pointers) != 0 {
			source, err = working.restrictLFSCheckout(paths, func(path string) bool { return !pointers[path] })
			if err != nil {
				return CheckoutResult{}, err
			}
		}
		if err := target.materializeCheckoutTarget(ctx, p.dest.head, paths); err != nil {
			return CheckoutResult{}, err
		}
		if err := source.materializeCheckoutTarget(ctx, p.source.head, paths); err != nil {
			return CheckoutResult{}, err
		}
	}
	if p.dest.branch != "" {
		unlock, err := r.lockBranchWorktreeIndices(ctx, r.gitDirInfo)
		if err != nil {
			return CheckoutResult{}, err
		}
		defer unlock()
	}
	var dirty *checkoutCleanup
	if mode != CheckoutClean {
		dirty, err = p.prepareCheckoutCleanup(ctx, lock, working, mode, message)
		if err != nil {
			return CheckoutResult{}, err
		}
		defer dirty.close()
	}
	publishErr := p.publishCheckout(ctx, lock, working, source, target, paths, dirty)
	if dirty != nil {
		warning, err := dirty.finish(context.WithoutCancel(ctx), publishErr)
		if err != nil {
			return CheckoutResult{}, err
		}
		result.Reapplied = mode == CheckoutStash && dirty.replayErr == nil
		result.Warning = warning
	} else if publishErr != nil {
		return CheckoutResult{}, publishErr
	}
	if p.dest.upstream != "" {
		result.Created = true
		if err := r.writeRefCommand(ctx, "branch", "--set-upstream-to="+p.dest.upstream, "--", p.dest.name); err != nil {
			result.Warning = joinCheckoutWarning(result.Warning, fmt.Sprintf("Could not configure upstream %q: %v", p.dest.upstream, err))
		}
	}
	return result, nil
}

func joinCheckoutWarning(first, second string) string {
	if first == "" {
		return second
	}
	if second == "" {
		return first
	}
	return first + "\n" + second
}

func (r *Repository) requireBranchCheckoutGit(ctx context.Context) error {
	out, err := r.readLimited(ctx, "", 4096, "version")
	if err != nil {
		return err
	}
	var major, minor int
	if _, err := fmt.Sscanf(string(out), "git version %d.%d", &major, &minor); err != nil {
		return fmt.Errorf("could not determine the Git version for branch checkout: %q", strings.TrimSpace(string(out)))
	}
	if major < 2 || (major == 2 && minor < 46) {
		return fmt.Errorf("branch checkout requires Git 2.46 or newer")
	}
	return nil
}
