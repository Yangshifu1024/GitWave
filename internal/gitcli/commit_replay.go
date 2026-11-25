package gitcli

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

type CommitReplayAction string

const (
	CherryPickCommit CommitReplayAction = "cherry-pick"
	RevertCommit     CommitReplayAction = "revert"
)

type CommitReplayConflicts struct {
	Action CommitReplayAction
	Paths  []string
}

func (e *CommitReplayConflicts) Error() string {
	return fmt.Sprintf("%s hit conflicts in: %s", e.Action, strings.Join(e.Paths, ", "))
}

// ReplayCommit applies one explicitly selected commit to the checkout
// captured when the user opened the action. Conflict state stays private.
func (r *Repository) ReplayCommit(ctx context.Context, action CommitReplayAction, oid string, local CheckoutTarget) (string, error) {
	if action != CherryPickCommit && action != RevertCommit {
		return "", fmt.Errorf("unknown commit action %q", action)
	}
	if !validOID(oid) || !validOID(local.OID) {
		return "", fmt.Errorf("select a commit and an existing checkout")
	}
	plan, err := r.prepareCheckout(ctx, CheckoutTarget{OID: oid}, local.OID)
	if err != nil {
		return "", err
	}
	if plan.source.head != local.OID || plan.source.attached != local.Ref {
		return "", fmt.Errorf("the checked-out branch or HEAD changed since the action was requested")
	}
	if plan.DirtyFiles != 0 {
		return "", fmt.Errorf("the working copy is not clean; commit or stash your changes first")
	}
	source, err := r.readCommitHeader(ctx, oid)
	if err != nil {
		return "", err
	}
	if len(source.parents) > 1 {
		return "", fmt.Errorf("%s requires a mainline parent for a merge commit", action)
	}
	current, err := r.readCommitHeader(ctx, local.OID)
	if err != nil {
		return "", err
	}
	git, err := r.privateReplay(ctx, local.OID)
	if err != nil {
		return "", err
	}
	defer git.close()
	if err := git.runCheckoutCommand(ctx, "", "read-tree", "--reset", "-u", local.OID); err != nil {
		return "", err
	}
	conflicts, err := git.applyCommitChange(ctx, action, oid, len(source.parents) != 0)
	if err != nil {
		return "", err
	}
	if len(conflicts) != 0 {
		return "", &CommitReplayConflicts{Action: action, Paths: conflicts}
	}
	tree, err := git.writeIndexTree(ctx)
	if err != nil {
		return "", err
	}
	if action == CherryPickCommit && tree == current.tree {
		return "", fmt.Errorf("%s produced no changes", action)
	}
	messageData, err := git.read(ctx, 32<<20, "show", "--no-patch", "--no-show-signature", "--encoding=UTF-8", "--format=%s%x00%B", oid, "--")
	if err != nil {
		return "", err
	}
	subject, body, ok := strings.Cut(string(messageData), "\x00")
	if !ok {
		return "", fmt.Errorf("Git returned an invalid commit message")
	}
	committer, err := r.commitIdentity(ctx)
	if err != nil {
		return "", err
	}
	author := source.author
	message := strings.TrimRightFunc(body, unicode.IsSpace) + "\n\n(cherry picked from commit " + oid + ")"
	if action == RevertCommit {
		author = committer
		subject = "Revert \"" + subject + "\""
		message = subject + "\n\nThis reverts commit " + oid + "."
	}
	next, err := r.createCommitObject(ctx, tree, []string{local.OID}, author, committer, message)
	if err != nil {
		return "", err
	}
	plan.dest.head = next
	plan.advanceNote = string(action) + ": " + subject
	if _, err := plan.Apply(ctx); err != nil {
		return "", err
	}
	return next, nil
}
