package gitcli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	interactiveRebaseOperation  = "interactive rebase"
	interactiveRebaseRefName    = "refs/worktree/gitwave/interactive-rebase"
	legacyInteractiveRebaseFile = "gitwave-interactive-rebase.json"
)

type interactiveAdvance struct{ before, after string }

func (a interactiveAdvance) statement(oidLength int) string {
	base := a.before
	if base == "" {
		base = strings.Repeat("0", oidLength)
	}
	if a.before == a.after {
		return verifyCheckoutRef(interactiveRebaseRefName, base)
	}
	const direct = "option no-deref\x00"
	if a.after == "" {
		return direct + "delete " + interactiveRebaseRefName + "\x00" + base + "\x00"
	}
	return direct + "update " + interactiveRebaseRefName + "\x00" + a.after + "\x00" + base + "\x00"
}

type interactivePause struct {
	Branch    string       `json:"branch"`
	Upstream  string       `json:"upstream_oid"`
	Original  string       `json:"original_head"`
	Head      string       `json:"paused_head"`
	Remaining []RebaseTodo `json:"remaining"`
	Legacy    string       `json:"legacy_version,omitempty"`
}

func (r *Repository) interactiveRebaseRef(ctx context.Context) (string, error) {
	output, err := r.readLimited(ctx, "", 8192, "for-each-ref", "--format=%(refname)%00%(objectname)%00%(symref)", interactiveRebaseRefName)
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(string(output), "\n") {
		name, rest, _ := strings.Cut(line, "\x00")
		if name != interactiveRebaseRefName {
			continue
		}
		oid, symbolic, ok := strings.Cut(rest, "\x00")
		if !ok || !validOID(oid) || symbolic != "" {
			return "", fmt.Errorf("invalid interactive rebase reference")
		}
		return oid, nil
	}
	return "", nil
}

func (r *Repository) saveInteractivePause(ctx context.Context, state interactivePause) (string, error) {
	data, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	if len(data) > interactiveRebaseLimit {
		return "", fmt.Errorf("interactive rebase pause data is too large")
	}
	output, err := r.readLimited(ctx, string(data), 256, "hash-object", "-w", "--stdin")
	if err != nil {
		return "", err
	}
	blob := strings.TrimSpace(string(output))
	if !validOID(blob) {
		return "", fmt.Errorf("invalid interactive rebase state object")
	}
	output, err = r.readLimited(ctx, "100644 blob "+blob+"\tstate.json\x00", 256, "mktree", "-z")
	if err != nil {
		return "", err
	}
	tree := strings.TrimSpace(string(output))
	if !validOID(tree) {
		return "", fmt.Errorf("invalid interactive rebase state tree")
	}
	identity, err := r.commitIdentity(ctx)
	if err != nil {
		return "", err
	}
	// Parent references retain the old tip and paused commit through GC.
	parents := []string{state.Head}
	for _, oid := range []string{state.Original, state.Upstream} {
		if oid != "" && !slices.Contains(parents, oid) {
			parents = append(parents, oid)
		}
	}
	return r.createCommitObject(ctx, tree, parents, identity, identity, "GitWave interactive rebase pause")
}

func (r *Repository) loadInteractivePause(ctx context.Context, expected string) (interactivePause, error) {
	if !validOID(expected) {
		return interactivePause{}, fmt.Errorf("no interactive rebase is paused for edit")
	}
	ref, err := r.interactiveRebaseRef(ctx)
	if err != nil {
		return interactivePause{}, err
	}
	if ref != expected {
		return interactivePause{}, fmt.Errorf("the interactive rebase changed since it was shown")
	}
	raw, err := r.readLimited(ctx, "", interactiveRebaseLimit, "cat-file", "blob", expected+":state.json")
	if err != nil {
		return interactivePause{}, err
	}
	var state interactivePause
	if err := json.Unmarshal(raw, &state); err != nil {
		return interactivePause{}, fmt.Errorf("invalid interactive rebase pause data: %w", err)
	}
	if !strings.HasPrefix(state.Branch, "refs/heads/") || !validOID(state.Original) ||
		!validOID(state.Head) || !validOID(state.Upstream) {
		return interactivePause{}, fmt.Errorf("invalid interactive rebase pause data")
	}
	for _, todo := range state.Remaining {
		if !validOID(todo.OID) || strings.ContainsRune(todo.Message, 0) {
			return interactivePause{}, fmt.Errorf("invalid commit in paused rebase")
		}
		switch todo.Action {
		case RebasePick, RebaseReword, RebaseEdit, RebaseSquash, RebaseFixup:
		default:
			return interactivePause{}, fmt.Errorf("invalid action in paused rebase")
		}
	}
	return state, nil
}

func (r *Repository) preparePausedCheckout(ctx context.Context, state interactivePause, expected string) (*CheckoutPlan, error) {
	plan, err := r.prepareCheckoutPlan(ctx, CheckoutTarget{OID: state.Upstream}, state.Head, nil, &interactiveAdvance{before: expected}, false)
	if err != nil {
		return nil, err
	}
	if plan.source.head != state.Head || plan.source.attached != state.Branch {
		return nil, fmt.Errorf("the paused interactive rebase belongs to a different checkout")
	}
	return plan, nil
}

func (p *CheckoutPlan) stagedInteractiveAmend(ctx context.Context) (string, error) {
	head := p.source.head
	if p.DirtyFiles == 0 {
		return head, nil
	}
	for _, change := range p.dirty.changes {
		if change.Kind == '?' || change.WorkTree != '.' {
			return "", fmt.Errorf("stage or stash unstaged changes before continuing the interactive rebase")
		}
	}
	r := p.repo
	root, err := os.OpenRoot(r.WorkTree)
	if err != nil {
		return "", err
	}
	defer root.Close()
	lock, err := r.lockCheckoutIndex(ctx)
	if err != nil {
		return "", err
	}
	defer lock.close()
	git, err := r.workingGit(ctx, head)
	if err != nil {
		return "", err
	}
	defer git.close()
	git.index = filepath.Join(git.dir, "index")
	if err := copyOriginalIndex(lock.root, git.index); err != nil {
		return "", err
	}
	changed := make(map[string]bool, len(p.dirty.changes))
	for _, entry := range p.dirty.changes {
		changed[entry.Path] = true
		if entry.PreviousPath != "" {
			changed[entry.PreviousPath] = true
		}
	}
	flags, err := git.read(ctx, 128<<20, "ls-files", "-v", "-z", "--")
	if err != nil {
		return "", err
	}
	for entry := range strings.SplitSeq(string(flags), "\x00") {
		if len(entry) >= 3 && changed[entry[2:]] && (entry[0] == 'S' || entry[0] >= 'a' && entry[0] <= 'z') {
			return "", fmt.Errorf("cannot verify unstaged changes at hidden index path %q", entry[2:])
		}
	}
	if err := p.dirty.verify(ctx, root, git); err != nil {
		return "", err
	}
	if err := git.isolateWorkTree(); err != nil {
		return "", err
	}
	tree, err := git.writeIndexTree(ctx)
	if err != nil {
		return "", err
	}
	previous, err := r.readCommitHeader(ctx, head)
	if err != nil {
		return "", err
	}
	committer, err := r.commitIdentity(ctx)
	if err != nil {
		return "", err
	}
	return r.createCommitObject(ctx, tree, previous.parents, previous.author, committer, previous.message)
}
