package gitcli

import (
	"context"
	"fmt"
	"strings"
)

func (r *Repository) PrepareHardReset(ctx context.Context, shown CheckoutTarget, target string) (*CheckoutPlan, error) {
	if !strings.HasPrefix(shown.Ref, "refs/heads/") || !validOID(shown.OID) {
		return nil, fmt.Errorf("checkout a local branch before resetting it")
	}
	plan, err := r.prepareCheckoutPlan(ctx, CheckoutTarget{OID: target}, target, nil, nil, true)
	if err != nil {
		return nil, err
	}
	if plan.source.attached != shown.Ref || plan.source.head != shown.OID {
		return nil, fmt.Errorf("the current branch changed since the reset was requested")
	}
	plan.advanceNote = "reset: moving to " + target
	return plan, nil
}

func (p *CheckoutPlan) ApplyHardReset(ctx context.Context) (CheckoutResult, error) {
	if !p.hardReset {
		return CheckoutResult{}, fmt.Errorf("prepare a branch reset before applying it")
	}
	mode := CheckoutClean
	if p.DirtyFiles != 0 {
		mode = CheckoutDiscard
	}
	return p.apply(ctx, mode, "reset to "+p.dest.head)
}
