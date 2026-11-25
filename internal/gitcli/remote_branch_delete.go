package gitcli

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

const deleteRemoteOperation = "delete-remote-branch"

type RemoteBranchDeletion struct {
	Branch      string
	Destination string
	Tip         string
}

type RemoteBranchDeleteResult struct {
	Local     string
	Deleted   []RemoteBranchDeletion
	Warnings  []string
	Attempted bool
}

type remoteBranchDeletionTarget struct {
	remote        RemoteInfo
	ref           string
	tracking      CheckoutTarget
	push          []remoteSource
	localPush     []*Repository
	fetchEndpoint string
	fetchLocal    *Repository
}

type remoteDeleteKey struct {
	endpoint string
	ref      string
}

type RemoteBranchDeletePlan struct {
	mu          sync.Mutex
	repo        *Repository
	local       CheckoutTarget
	targets     []remoteBranchDeletionTarget
	remoteIndex int
	pushIndex   int
	result      RemoteBranchDeleteResult
	done        map[remoteDeleteKey]bool
	pruned      map[string]bool
}

func (p *RemoteBranchDeletePlan) Destinations() []RemoteBranchDeletion {
	var items []RemoteBranchDeletion
	seen := make(map[remoteDeleteKey]bool)
	for _, target := range p.targets {
		for i, source := range target.push {
			key := target.key(i)
			if !seen[key] {
				seen[key] = true
				items = append(items, target.destination(source))
			}
		}
	}
	return items
}

func (t remoteBranchDeletionTarget) key(i int) remoteDeleteKey {
	endpoint := t.push[i].endpoint
	if repo := t.localPush[i]; repo != nil {
		endpoint = repo.CommonDir
	}
	return remoteDeleteKey{endpoint: endpoint, ref: t.ref}
}

func (t remoteBranchDeletionTarget) destination(source remoteSource) RemoteBranchDeletion {
	address := source.endpoint
	if endpoint, err := url.Parse(address); err == nil && endpoint.Scheme != "" {
		address = endpoint.Redacted()
	}
	return RemoteBranchDeletion{Branch: t.remote.Name + "/" + strings.TrimPrefix(t.ref, "refs/heads/"),
		Destination: address, Tip: t.tracking.OID}
}

// PrepareRemoteBranchDelete takes either the exact remote-tracking ref shown
// in History or the local branch whose same-named remote counterparts the
// user can opt to delete. It does not contact a remote.
func (r *Repository) PrepareRemoteBranchDelete(ctx context.Context, shown CheckoutTarget) (*RemoteBranchDeletePlan, error) {
	local := strings.HasPrefix(shown.Ref, "refs/heads/")
	if !validOID(shown.OID) || (!local && !strings.HasPrefix(shown.Ref, "refs/remotes/")) {
		return nil, fmt.Errorf("select a branch with a commit")
	}
	refs, err := r.References(ctx)
	if err != nil {
		return nil, err
	}
	if err := checkRefTip(refs, shown.Ref, shown.OID); err != nil {
		return nil, err
	}
	remotes, err := r.Remotes(ctx)
	if err != nil {
		return nil, err
	}
	plan := &RemoteBranchDeletePlan{repo: r, done: make(map[remoteDeleteKey]bool), pruned: make(map[string]bool)}
	if local {
		plan.local = shown
	}
	for _, remote := range remotes {
		specs := remote.urls("fetch")
		target := remoteBranchDeletionTarget{remote: remote, tracking: shown}
		if local {
			target.ref = shown.Ref
			name, err := pullTrackingReference(remote.Name, shown.Ref, specs)
			if err != nil || !uniqueTrackingSource(shown.Ref, name, specs) {
				continue
			}
			ref := findRef(refs, name)
			if !validOID(ref.OID) || ref.Symbolic != "" {
				continue
			}
			target.tracking = CheckoutTarget{Ref: name, OID: ref.OID}
		} else {
			source, err := branchForTrackingReference(shown.Ref, specs)
			if err != nil {
				return nil, err
			}
			if source == "" {
				continue
			}
			target.ref = source
		}
		if _, err := r.checkedRefName(ctx, "heads", strings.TrimPrefix(target.ref, "refs/heads/")); err != nil {
			return nil, err
		}
		target.push, err = r.pushEndpoints(ctx, remote.Name)
		if err != nil {
			return nil, err
		}
		target.localPush = make([]*Repository, len(target.push))
		for i, source := range target.push {
			if source.protocol == "file" {
				target.localPush[i], err = openLocalPushRepository(ctx, source.endpoint)
				if err != nil {
					return nil, fmt.Errorf("open the local push repository for %q: %w", remote.Name, err)
				}
			}
		}
		if endpoint, protocol, err := parseTransportSource(remote.endpoint, r.Path()); err == nil {
			target.fetchEndpoint = endpoint
			if protocol == "file" {
				target.fetchEndpoint = ""
				if fetch, err := openLocalPushRepository(ctx, endpoint); err == nil {
					target.fetchEndpoint = fetch.CommonDir
					target.fetchLocal = fetch
				}
			}
		}
		plan.targets = append(plan.targets, target)
	}
	if !local && len(plan.targets) != 1 {
		return nil, fmt.Errorf("the tracking reference does not identify a single remote branch")
	}
	leases := make(map[remoteDeleteKey]string)
	for _, target := range plan.targets {
		for i := range target.push {
			key := target.key(i)
			if old := leases[key]; old != "" && old != target.tracking.OID {
				return nil, fmt.Errorf("tracking branches disagree about the tip at a shared destination")
			}
			leases[key] = target.tracking.OID
		}
	}
	return plan, nil
}

func openLocalPushRepository(ctx context.Context, endpoint string) (*Repository, error) {
	path, err := localTransportPath(endpoint)
	if err != nil {
		return nil, err
	}
	repo, openErr := OpenRoot(ctx, path)
	if openErr == nil {
		return repo, nil
	}
	// Git also accepts an explicit .git path as a file transport.
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return nil, openErr
	}
	git, err := findGit()
	if err != nil {
		return nil, openErr
	}
	gitDir, err := readPath(ctx, git, path, "--absolute-git-dir")
	if err != nil || gitDir != path {
		return nil, openErr
	}
	common, err := readPath(ctx, git, path, "--git-common-dir")
	if err != nil {
		return nil, err
	}
	return openedRepository("", path, common, git)
}

func (t remoteBranchDeletionTarget) verifyLocalDestination(ctx context.Context, i int) error {
	if shown := t.localPush[i]; shown != nil {
		fresh, err := openLocalPushRepository(ctx, t.push[i].endpoint)
		if err != nil || !shown.SameCheckout(fresh) {
			return fmt.Errorf("the local push repository changed since the branch was shown")
		}
	}
	return nil
}

func branchForTrackingReference(tracking string, specs []string) (string, error) {
	var source string
	for _, spec := range specs {
		from, to, ok := strings.Cut(strings.TrimPrefix(spec, "+"), ":")
		middle, matches := matchFetchSource(to, tracking)
		if !ok || !matches || strings.HasPrefix(from, "^") {
			continue
		}
		candidate := strings.Replace(from, "*", middle, 1)
		if fetchSourceExcluded(specs, candidate) {
			continue
		}
		if source != "" && source != candidate {
			return "", fmt.Errorf("the remote-tracking reference maps to more than one branch")
		}
		source = candidate
	}
	if !strings.HasPrefix(source, "refs/heads/") {
		return "", nil
	}
	return source, nil
}

func (p *RemoteBranchDeletePlan) Apply(ctx context.Context, transfer FetchOptions) (RemoteBranchDeleteResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return retryWithHTTPSVault(ctx, p.repo.git, p.repo.Path(), transfer, func(attempt FetchOptions) (RemoteBranchDeleteResult, error) {
		return p.apply(ctx, attempt)
	})
}

func (p *RemoteBranchDeletePlan) apply(ctx context.Context, transfer FetchOptions) (RemoteBranchDeleteResult, error) {
	if len(p.targets) == 0 {
		return p.result, fmt.Errorf("select a remote branch to delete")
	}
	if transfer.Credentials != nil {
		challenge := transfer.Challenge
		if p.remoteIndex >= len(p.targets) || challenge == nil || challenge.operation != deleteRemoteOperation {
			return p.result, fmt.Errorf("the authentication prompt belongs to another operation")
		}
		target := p.targets[p.remoteIndex]
		if p.pushIndex >= len(target.push) || challenge.Remote != target.remote.Name ||
			challenge.endpoint != target.push[p.pushIndex].endpoint {
			return p.result, fmt.Errorf("the authentication prompt belongs to another remote destination")
		}
	}
	if p.local.Ref != "" && p.result.Local == "" {
		for _, target := range p.targets {
			if err := p.verify(ctx, target); err != nil {
				return p.result, err
			}
		}
		local := strings.TrimPrefix(p.local.Ref, "refs/heads/")
		if err := p.repo.DeleteBranch(ctx, local, p.local.OID); err != nil {
			return p.result, err
		}
		p.result.Local = local
	}
	err := p.repo.withRefEdit(ctx, func() error {
		for p.remoteIndex < len(p.targets) {
			target := p.targets[p.remoteIndex]
			for p.pushIndex < len(target.push) && p.done[target.key(p.pushIndex)] {
				p.pushIndex++
			}
			if p.pushIndex == len(target.push) {
				p.remoteIndex++
				p.pushIndex = 0
				continue
			}
			if err := p.verify(ctx, target); err != nil {
				return err
			}
			for p.pushIndex < len(target.push) {
				i := p.pushIndex
				key := target.key(i)
				if p.done[key] {
					p.pushIndex++
					continue
				}
				source := target.push[i]
				destinationSource := source
				if local := target.localPush[i]; local != nil {
					destinationSource.endpoint = local.Path()
				}
				var credentials *HTTPSCredential
				if challenge := transfer.Challenge; transfer.Credentials != nil && challenge != nil &&
					challenge.operation == deleteRemoteOperation && challenge.Remote == target.remote.Name &&
					challenge.endpoint == source.endpoint {
					credentials = transfer.Credentials
				}
				lease := map[string]string{target.ref: target.tracking.OID}
				accepted := false
				err := p.repo.withPushDestination(ctx, destinationSource, "", target.remote.Name, deleteRemoteOperation,
					lfsRemoteTarget{}, localLFSTransport{}, pushAuthentication{git: credentials}, transfer, func(destination pushDestination) error {
						if err := p.verify(ctx, target); err != nil {
							return err
						}
						p.result.Attempted = true
						report, err := destination.push(ctx, []CheckoutTarget{{Ref: target.ref}}, lease)
						accepted = len(report.accepted) != 0
						if accepted {
							if warning := destination.transport.remember(ctx, p.repo.git, destination.scratch, transfer.Vault); warning != "" {
								p.result.Warnings = append(p.result.Warnings, warning)
							}
						}
						return err
					})
				if accepted {
					p.result.Deleted = append(p.result.Deleted, target.destination(source))
					p.done[key] = true
					p.pruneConfirmedTracking(ctx, key)
					p.pushIndex++
				}
				if err != nil {
					return err
				}
				if !accepted {
					return fmt.Errorf("Git did not confirm deletion of %q", target.ref)
				}
			}
			p.remoteIndex++
			p.pushIndex = 0
		}
		return nil
	})
	result := p.result
	result.Deleted = slices.Clone(result.Deleted)
	result.Warnings = slices.Clone(result.Warnings)
	return result, err
}

func (p *RemoteBranchDeletePlan) pruneConfirmedTracking(ctx context.Context, key remoteDeleteKey) {
	for _, target := range p.targets {
		if target.ref != key.ref || target.fetchEndpoint != key.endpoint || p.pruned[target.tracking.Ref] {
			continue
		}
		for i := range target.push {
			if target.key(i) != key {
				continue
			}
			removed, warning := p.removeTracking(ctx, target)
			p.pruned[target.tracking.Ref] = removed
			if warning != "" {
				p.result.Warnings = append(p.result.Warnings, warning)
			}
			break
		}
	}
}

func (p *RemoteBranchDeletePlan) verify(ctx context.Context, target remoteBranchDeletionTarget) error {
	r := p.repo
	if err := r.checkRemoteDeleteCheckout(ctx); err != nil {
		return err
	}
	if _, err := r.matchRemote(ctx, target.remote); err != nil {
		return err
	}
	sources, err := r.pushEndpoints(ctx, target.remote.Name)
	if err != nil {
		return err
	}
	if !slices.Equal(sources, target.push) {
		return fmt.Errorf("the push destination changed since the branch was shown")
	}
	for i := range target.push {
		if p.done[target.key(i)] {
			continue
		}
		if err := target.verifyLocalDestination(ctx, i); err != nil {
			return err
		}
	}
	if !r.remoteRefMatches(ctx, remoteTrackingRef{name: target.tracking.Ref, oid: target.tracking.OID}) {
		if p.pruned[target.tracking.Ref] && r.remoteRefMatches(ctx, remoteTrackingRef{name: target.tracking.Ref}) {
			return nil
		}
		return fmt.Errorf("the remote-tracking branch changed since it was shown")
	}
	return nil
}

func (p *RemoteBranchDeletePlan) removeTracking(ctx context.Context, target remoteBranchDeletionTarget) (bool, string) {
	r := p.repo
	finalize, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	removed := false
	err := func() error {
		if err := r.checkRemoteDeleteCheckout(finalize); err != nil {
			return err
		}
		removable, err := r.trackingRemovable(finalize, target)
		if err != nil || !removable {
			return err
		}
		name := target.tracking.Ref
		affected := map[string]bool{name: true}
		if err := r.checkTrackingRefPaths(finalize, affected); err != nil {
			return err
		}
		err = r.editDirectRef(finalize, name, target.tracking.OID, "", "delete remote branch: remove tracking reference", func() error {
			if err := r.checkRemoteDeleteCheckout(finalize); err != nil {
				return err
			}
			if err := r.checkTrackingRefPaths(finalize, affected); err != nil {
				return err
			}
			removable, err := r.trackingRemovable(finalize, target)
			if err != nil {
				return err
			}
			if !removable {
				return fmt.Errorf("another remote or local reference now uses this tracking branch")
			}
			return r.protectFetchHeads(finalize, affected)
		})
		removed = err == nil
		return err
	}()
	if err != nil {
		return false, fmt.Sprintf("Remote deletion succeeded, but its local tracking reference could not be removed: %v", err)
	}
	return removed, ""
}

func (r *Repository) checkRemoteDeleteCheckout(ctx context.Context) error {
	fresh, err := OpenRoot(ctx, r.Path())
	if err != nil || !r.SameCheckout(fresh) {
		return fmt.Errorf("the local checkout changed during remote branch deletion")
	}
	return nil
}

func (r *Repository) trackingRemovable(ctx context.Context, target remoteBranchDeletionTarget) (bool, error) {
	items, err := r.Remotes(ctx)
	if err != nil {
		return false, err
	}
	if !slices.ContainsFunc(items, target.remote.SameSelection) {
		return false, fmt.Errorf("the remote changed before its tracking branch could be removed")
	}
	if target.fetchLocal != nil {
		endpoint, _, err := parseTransportSource(target.remote.endpoint, r.Path())
		if err != nil {
			return false, err
		}
		fresh, err := openLocalPushRepository(ctx, endpoint)
		if err != nil || !target.fetchLocal.SameCheckout(fresh) {
			return false, fmt.Errorf("the local fetch repository changed before its tracking branch could be removed")
		}
	}
	name := target.tracking.Ref
	for _, other := range items {
		if other.Name != target.remote.Name && remoteTracksRef(other, name) {
			return false, nil
		}
	}
	refs, err := r.remoteTrackingRefs(ctx)
	if err != nil {
		return false, err
	}
	return !slices.ContainsFunc(refs, func(ref remoteTrackingRef) bool { return ref.symbolic == name }), nil
}
