package gitcli

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

type RebaseAction string

const (
	RebasePick   RebaseAction = "pick"
	RebaseReword RebaseAction = "reword"
	RebaseEdit   RebaseAction = "edit"
	RebaseSquash RebaseAction = "squash"
	RebaseFixup  RebaseAction = "fixup"
	RebaseDrop   RebaseAction = "drop"
)

type RebaseTodo struct {
	OID     string       `json:"oid"`
	Summary string       `json:"summary"`
	Action  RebaseAction `json:"action"`
	Message string       `json:"message,omitempty"`
}

type InteractiveRebasePlan struct {
	repo            *Repository
	upstream, local CheckoutTarget
	todos           []RebaseTodo
}

type InteractiveRebaseResult struct {
	OID       string
	Conflicts []string
	Paused    bool
}

const interactiveRebaseLimit = 5 << 20

func (r *Repository) PrepareInteractiveRebase(ctx context.Context, upstream, local CheckoutTarget) (*InteractiveRebasePlan, error) {
	if r.WorkTree == "" || !validOID(upstream.OID) || !validOID(local.OID) || upstream.Ref == local.Ref ||
		!strings.HasPrefix(upstream.Ref, "refs/heads/") || !strings.HasPrefix(local.Ref, "refs/heads/") {
		return nil, fmt.Errorf("select another local branch as the rebase target")
	}
	p := &InteractiveRebasePlan{repo: r, upstream: upstream, local: local}
	if err := p.verify(ctx); err != nil {
		return nil, err
	}
	output, err := r.readLimited(ctx, "", interactiveRebaseLimit, "log", "--reverse", "--topo-order",
		"--encoding=UTF-8", "--format=%H%x00%s", "-z", "--no-show-signature", upstream.OID+".."+local.OID, "--")
	if err != nil {
		return nil, err
	}
	for records := string(output); records != ""; {
		oid, rest, ok := strings.Cut(records, "\x00")
		summary, remaining, ended := strings.Cut(rest, "\x00")
		if !ok || !ended || !validOID(oid) {
			return nil, fmt.Errorf("invalid interactive rebase commit list")
		}
		p.todos = append(p.todos, RebaseTodo{OID: oid, Summary: summary, Action: RebasePick})
		records = remaining
	}
	return p, nil
}

func (p *InteractiveRebasePlan) Todos() []RebaseTodo { return slices.Clone(p.todos) }

func (p *InteractiveRebasePlan) Apply(ctx context.Context, todos []RebaseTodo) (InteractiveRebaseResult, error) {
	active, err := p.validateTodos(todos)
	if err != nil {
		return InteractiveRebaseResult{}, err
	}
	if len(todos) == 0 {
		return InteractiveRebaseResult{}, fmt.Errorf("there are no local commits to reorder")
	}
	r := p.repo
	plan, err := r.prepareCheckoutPlan(ctx, p.upstream, p.upstream.OID, nil, &interactiveAdvance{}, false)
	if err != nil {
		return InteractiveRebaseResult{}, err
	}
	if plan.source.head != p.local.OID || plan.source.attached != p.local.Ref {
		return InteractiveRebaseResult{}, fmt.Errorf("the checked-out branch changed since the rebase was planned")
	}
	if plan.DirtyFiles != 0 {
		return InteractiveRebaseResult{}, &RebaseDirtyError{Files: plan.DirtyFiles}
	}
	plan.rebase = true
	state := interactivePause{Branch: p.local.Ref, Original: p.local.OID, Upstream: p.upstream.OID}
	return plan.replayInteractive(ctx, state, p.upstream.OID, active)
}

func (r *Repository) ContinueInteractiveRebase(ctx context.Context, expected string) (InteractiveRebaseResult, error) {
	state, expected, err := r.openInteractivePause(ctx, expected)
	if err != nil {
		return InteractiveRebaseResult{}, err
	}
	plan, err := r.preparePausedCheckout(ctx, state, expected)
	if err != nil {
		return InteractiveRebaseResult{}, err
	}
	base, err := plan.stagedInteractiveAmend(ctx)
	if err != nil {
		return InteractiveRebaseResult{}, err
	}
	return plan.replayInteractive(ctx, state, base, state.Remaining)
}

func (r *Repository) AbortInteractiveRebase(ctx context.Context, expected string) error {
	if strings.HasPrefix(expected, legacyPausePrefix) {
		cleared, err := r.discardLegacyPauseWithoutOrigin(ctx, expected)
		if cleared || err != nil {
			return err
		}
	}
	state, expected, err := r.openInteractivePause(ctx, expected)
	if err != nil {
		return err
	}
	plan, err := r.preparePausedCheckout(ctx, state, expected)
	if err != nil {
		return err
	}
	if plan.DirtyFiles != 0 {
		return &RebaseDirtyError{Files: plan.DirtyFiles}
	}
	plan.dest.head = state.Original
	_, err = plan.Apply(ctx)
	return err
}

func (p *CheckoutPlan) replayInteractive(ctx context.Context, state interactivePause, base string, todos []RebaseTodo) (InteractiveRebaseResult, error) {
	result, remaining, err := p.repo.replayInteractive(ctx, base, todos)
	if err != nil {
		return InteractiveRebaseResult{}, err
	}
	if len(result.Conflicts) != 0 {
		if p.interactive.before == "" {
			return result, nil
		}
		if p.DirtyFiles != 0 {
			// Keep the edit pause and staged changes if the remaining
			// sequence cannot land with the proposed amendment.
			result.OID, result.Paused = p.source.head, true
			return result, nil
		}
		result.OID = state.Original
	}
	if result.Paused {
		state.Head, state.Remaining = result.OID, remaining
		p.interactive.after, err = p.repo.saveInteractivePause(ctx, state)
		if err != nil {
			return InteractiveRebaseResult{}, err
		}
	}
	p.dest.head = result.OID
	mode := CheckoutClean
	if p.DirtyFiles != 0 {
		mode = CheckoutDiscard // the captured staged changes are included in result.OID
	}
	if _, err := p.apply(ctx, mode, "interactive rebase"); err != nil {
		return InteractiveRebaseResult{}, err
	}
	return result, nil
}

func (p *InteractiveRebasePlan) verify(ctx context.Context) error {
	if err := p.repo.checkCheckoutOperation(ctx); err != nil {
		return err
	}
	if !p.repo.headMatches(ctx, p.local.Ref, p.local.OID) {
		return fmt.Errorf("the checked-out branch changed since the rebase was planned")
	}
	refs, err := p.repo.References(ctx)
	if err != nil {
		return err
	}
	return checkRefTip(refs, p.upstream.Ref, p.upstream.OID)
}

func (p *InteractiveRebasePlan) validateTodos(todos []RebaseTodo) ([]RebaseTodo, error) {
	if len(todos) != len(p.todos) {
		return nil, fmt.Errorf("the rebase todo list no longer matches the selected commits")
	}
	allowed := make(map[string]string, len(p.todos))
	for _, todo := range p.todos {
		allowed[todo.OID] = todo.Summary
	}
	var active []RebaseTodo
	for _, todo := range todos {
		summary, found := allowed[todo.OID]
		if !found {
			return nil, fmt.Errorf("the rebase todo list contains a missing or duplicate commit")
		}
		delete(allowed, todo.OID)
		todo.Summary = summary
		switch todo.Action {
		case RebaseDrop:
			continue
		case RebasePick, RebaseReword, RebaseEdit:
		case RebaseSquash, RebaseFixup:
			if len(active) == 0 {
				return nil, fmt.Errorf("the first selected commit cannot be squashed or fixed up")
			}
		default:
			return nil, fmt.Errorf("invalid rebase action %q", todo.Action)
		}
		if len(todo.Message) > interactiveRebaseLimit || strings.ContainsRune(todo.Message, 0) {
			return nil, fmt.Errorf("invalid rebase commit message")
		}
		active = append(active, todo)
	}
	return active, nil
}
