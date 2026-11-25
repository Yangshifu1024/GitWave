package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type PullOptions struct {
	Remote, Branch string
	Local          CheckoutTarget
	Rebase, Stash  bool
}

type PullPlan struct {
	options  PullOptions
	checkout CheckoutPlan
	lfs      lfsDownloadPlan
	gitAuth  *HTTPSCredential
	lfsAuth  *HTTPSCredential
}

type PullResult struct {
	OID, Warning string
	Fetched      bool
	UpToDate     bool
	Rebased      bool
}

type pullSource struct {
	branch  string
	local   string
	target  CheckoutTarget
	lfs     lfsDownloadPlan
	lfsAuth *HTTPSCredential
	receipt lfsDownloadReceipt
}

func (r *Repository) PreparePull(ctx context.Context, options PullOptions) (*PullPlan, error) {
	if r.WorkTree == "" || !strings.HasPrefix(options.Local.Ref, "refs/heads/") {
		return nil, fmt.Errorf("pull requires an attached local branch")
	}
	if options.Remote == "" {
		return nil, fmt.Errorf("select a remote to pull from")
	}
	branch, err := r.checkedRefName(ctx, "heads", options.Branch)
	if err != nil {
		return nil, err
	}
	attached, err := r.optionalRef(ctx, "symbolic-ref", "--no-recurse", "-q", "HEAD")
	if err != nil {
		return nil, err
	}
	refs, err := r.References(ctx)
	if err != nil {
		return nil, err
	}
	if attached != options.Local.Ref || attached != refs.HeadRef || refs.Head != options.Local.OID {
		return nil, fmt.Errorf("the selected local branch changed before pulling")
	}
	plan := &PullPlan{options: options, checkout: CheckoutPlan{
		repo: r, advance: true, source: checkoutSource{head: refs.Head, resolved: refs.HeadRef, attached: attached},
		dest: checkoutDestination{branch: attached, name: strings.TrimPrefix(attached, "refs/heads/")},
	}}
	plan.options.Branch = branch
	if err := plan.checkout.captureWorkingCopy(ctx, options.Stash); err != nil {
		return nil, err
	}
	plan.lfs, err = r.prepareLFSDownload(ctx, options.Remote)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

// Apply fetches and integrates only the selected branch. An authentication
// retry can reuse the same plan without authorizing newer working changes.
func (p *PullPlan) Apply(ctx context.Context, transfer FetchOptions) (PullResult, error) {
	r := p.checkout.repo
	return retryWithHTTPSVault(ctx, r.git, r.Path(), transfer, func(attempt FetchOptions) (PullResult, error) {
		return p.apply(ctx, attempt)
	})
}

func (p *PullPlan) apply(ctx context.Context, transfer FetchOptions) (PullResult, error) {
	r := p.checkout.repo
	remote := p.options.Remote
	if (transfer.Remote != "" && transfer.Remote != remote) ||
		(transfer.Challenge != nil && transfer.Challenge.Remote != remote) ||
		(transfer.Credentials != nil && transfer.Challenge == nil) {
		return PullResult{}, fmt.Errorf("the authentication prompt belongs to another pull")
	}
	local := p.checkout.source
	selected := pullSource{branch: p.options.Branch, local: local.attached, lfs: p.lfs}
	result := PullResult{OID: local.head}
	err := r.withRefEdit(ctx, func() error {
		if err := r.checkCheckoutOperation(ctx); err != nil {
			return err
		}
		if !r.headMatches(ctx, local.attached, local.head) {
			return fmt.Errorf("the selected local branch changed before pulling")
		}
		if transfer.Challenge != nil && transfer.Credentials != nil {
			auth := *transfer.Credentials
			if transfer.Challenge.media == "" {
				p.gitAuth = &auth
			} else {
				p.lfsAuth = &auth
			}
		}
		transfer.Credentials, selected.lfsAuth = p.gitAuth, p.lfsAuth
		warning, err := r.fetchRemote(ctx, remote, transfer, &selected)
		var challenge *RemoteAuthError
		switch {
		case !errors.As(err, &challenge):
			p.gitAuth, p.lfsAuth = nil, nil
		case challenge.media == "":
			p.gitAuth = nil
		default:
			p.lfsAuth = nil
		}
		if err == nil {
			result.Fetched = true
			result.Warning = warning
		}
		return err
	})
	if err != nil {
		return result, err
	}
	if !validOID(selected.target.OID) {
		return result, fmt.Errorf("the selected remote branch no longer exists")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if !r.headMatches(ctx, local.attached, local.head) {
		return result, fmt.Errorf("the local branch changed while fetching")
	}
	next := selected.target.OID
	if local.head != "" {
		upToDate, err := r.isAncestor(ctx, next, local.head)
		if err != nil {
			return result, err
		}
		if upToDate {
			result.UpToDate = true
			return result, nil
		}
	}
	if !p.options.Stash && p.checkout.DirtyFiles != 0 {
		return result, fmt.Errorf("pull needs a clean worktree; select Stash and reapply or commit first")
	}
	if local.head != "" {
		fastForward, err := r.isAncestor(ctx, local.head, next)
		if err != nil {
			return result, err
		}
		if !fastForward {
			if !p.options.Rebase {
				return result, fmt.Errorf("pull would require a merge; select Rebase or merge from Branches")
			}
			next, err = r.rebasePull(ctx, local.head, next)
			if err != nil {
				return result, err
			}
			result.Rebased = true
		}
	}
	plan := p.checkout
	plan.request = selected.target
	if plan.request.Ref == local.attached {
		plan.request.Ref = ""
	}
	plan.dest.head = next
	if selected.receipt.filtered && next != selected.target.OID {
		selected.receipt.paths, err = p.lfs.checkoutPaths(ctx, r, next)
		if err != nil {
			return result, err
		}
	}
	plan.downloadedLFS = &selected.receipt
	mode := CheckoutClean
	if p.options.Stash && plan.DirtyFiles != 0 {
		mode = CheckoutStash
	}
	integrated, err := plan.apply(ctx, mode, "pull: auto stash")
	if err != nil {
		return result, err
	}
	result.OID = integrated.OID
	result.Warning = joinCheckoutWarning(result.Warning, integrated.Warning)
	return result, nil
}

func (r *Repository) isAncestor(ctx context.Context, older, newer string) (bool, error) {
	_, err := r.readLimited(ctx, "", 1024, "merge-base", "--is-ancestor", older, newer)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, nil
	}
	return err == nil, err
}

func pullTrackingReference(remote, source string, refspecs []string) (string, error) {
	var dests []string
	for _, spec := range refspecs {
		local := localFetchDestination(spec)
		spec = strings.TrimPrefix(spec, "+")
		if neg, ok := strings.CutPrefix(spec, "^"); ok {
			if _, matches := matchFetchSource(neg, source); matches {
				return "", fmt.Errorf("the selected branch is excluded by the remote's fetch refspec")
			}
			continue
		}
		from, dest, _ := strings.Cut(spec, ":")
		middle, matches := matchFetchSource(from, source)
		if !matches || !strings.HasPrefix(dest, "refs/remotes/") && local == "" {
			continue
		}
		dest = strings.Replace(dest, "*", middle, 1)
		for _, seen := range dests {
			if dest == seen {
				dest = ""
				break
			}
		}
		if dest != "" {
			dests = append(dests, dest)
		}
	}
	direct := "refs/remotes/" + remote + "/" + strings.TrimPrefix(source, "refs/heads/")
	for _, dest := range dests {
		if dest == direct {
			return dest, nil
		}
	}
	if len(dests) == 1 {
		return dests[0], nil
	}
	return "", fmt.Errorf("the remote does not map the selected branch to a unique tracking reference")
}

func matchFetchSource(spec, ref string) (string, bool) {
	before, after, wild := strings.Cut(spec, "*")
	if !wild {
		return "", spec == ref
	}
	if len(ref) < len(before)+len(after) || !strings.HasPrefix(ref, before) || !strings.HasSuffix(ref, after) {
		return "", false
	}
	return ref[len(before) : len(ref)-len(after)], true
}
