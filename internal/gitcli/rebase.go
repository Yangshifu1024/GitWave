package gitcli

import (
	"context"
	"fmt"
	"strings"
)

type RebaseKind uint8

const (
	RebaseUpToDate RebaseKind = iota + 1
	RebaseFastForward
	RebaseClean
	RebaseConflicts
)

type RebaseResult struct {
	Kind      RebaseKind
	OID       string
	Conflicts []string
}

type RebaseDirtyError struct{ Files int }

func (e *RebaseDirtyError) Error() string {
	return fmt.Sprintf("rebase needs a clean worktree; %d uncommitted file(s)", e.Files)
}

// RebaseBranch rebases the shown local checkout onto the shown upstream.
// A conflict leaves the live branch and working copy at their original state.
func (r *Repository) RebaseBranch(ctx context.Context, upstream, local CheckoutTarget) (RebaseResult, error) {
	if r.WorkTree == "" || !validOID(upstream.OID) || !validOID(local.OID) ||
		!strings.HasPrefix(upstream.Ref, "refs/heads/") || !strings.HasPrefix(local.Ref, "refs/heads/") {
		return RebaseResult{}, fmt.Errorf("select a local branch and a checked-out local branch to rebase")
	}
	if err := r.checkCheckoutOperation(ctx); err != nil {
		return RebaseResult{}, err
	}
	if !r.headMatches(ctx, local.Ref, local.OID) {
		return RebaseResult{}, fmt.Errorf("the checked-out branch changed since rebase was requested")
	}
	refs, err := r.References(ctx)
	if err != nil {
		return RebaseResult{}, err
	}
	if err := checkRefTip(refs, upstream.Ref, upstream.OID); err != nil {
		return RebaseResult{}, err
	}
	status, err := r.Status(ctx)
	if err != nil {
		return RebaseResult{}, err
	}
	if status.HeadRef != local.Ref || status.HeadOID != local.OID || status.Commit != (CommitState{}) {
		return RebaseResult{}, fmt.Errorf("HEAD or the repository operation changed before rebasing")
	}
	if len(status.Changes) != 0 {
		return RebaseResult{}, &RebaseDirtyError{Files: len(status.Changes)}
	}
	upToDate, err := r.isAncestor(ctx, upstream.OID, local.OID)
	if err != nil {
		return RebaseResult{}, err
	}
	if upToDate {
		return RebaseResult{Kind: RebaseUpToDate, OID: local.OID}, nil
	}
	fastForward, err := r.isAncestor(ctx, local.OID, upstream.OID)
	if err != nil {
		return RebaseResult{}, err
	}
	result := RebaseResult{Kind: RebaseFastForward, OID: upstream.OID}
	if !fastForward {
		oid, conflicts, err := r.replayRebase(ctx, local.OID, upstream.OID)
		if err != nil {
			return RebaseResult{}, err
		}
		if len(conflicts) != 0 {
			return RebaseResult{Kind: RebaseConflicts, Conflicts: conflicts}, nil
		}
		result = RebaseResult{Kind: RebaseClean, OID: oid}
	}
	plan, err := r.prepareCheckout(ctx, upstream, result.OID)
	if err != nil {
		return RebaseResult{}, err
	}
	if plan.source.attached != local.Ref || plan.source.head != local.OID {
		return RebaseResult{}, fmt.Errorf("the checked-out branch changed since rebase was requested")
	}
	if plan.DirtyFiles != 0 {
		return RebaseResult{}, &RebaseDirtyError{Files: plan.DirtyFiles}
	}
	plan.rebase = true
	if _, err := plan.Apply(ctx); err != nil {
		return RebaseResult{}, err
	}
	return result, nil
}
