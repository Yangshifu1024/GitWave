package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

type SubmoduleUpdatePlan struct {
	mutex     sync.Mutex
	pending   []submoduleUpdate
	challenge *RemoteAuthError
	result    SubmoduleUpdateResult
}

type SubmoduleUpdateResult struct {
	Updated []string
	Warning string
}

type submoduleUpdate struct {
	parent   *Repository
	selected *Repository
	item     SubmoduleInfo
	path     string
}

const submoduleUpdateOperation = "submodule-update"

func (r *Repository) PrepareSubmoduleUpdate(ctx context.Context, item SubmoduleInfo) (*SubmoduleUpdatePlan, error) {
	if item.IndexOID == "" {
		return nil, fmt.Errorf("select a submodule with a staged gitlink")
	}
	if err := r.verifySubmodule(ctx, item, false); err != nil {
		return nil, err
	}
	step, err := r.submoduleUpdateStep(ctx, item, item.Path)
	if err != nil {
		return nil, err
	}
	return &SubmoduleUpdatePlan{pending: []submoduleUpdate{step}}, nil
}

func (r *Repository) submoduleUpdateStep(ctx context.Context, item SubmoduleInfo, relative string) (submoduleUpdate, error) {
	root, err := os.OpenRoot(r.WorkTree)
	if err != nil {
		return submoduleUpdate{}, err
	}
	defer root.Close()
	child, err := r.openSubmodule(ctx, root, item.Path)
	return submoduleUpdate{parent: r, selected: child, item: item, path: relative}, err
}

// Apply advances the selected subtree. An authentication retry resumes at
// the failing child and supplies credentials only to that child.
func (p *SubmoduleUpdatePlan) Apply(ctx context.Context, options FetchOptions) (SubmoduleUpdateResult, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	if (p.challenge != nil && !p.challenge.SameChallenge(options.Challenge)) ||
		(p.challenge == nil && (options.Challenge != nil || options.Credentials != nil)) {
		return p.snapshot(), fmt.Errorf("the authentication prompt belongs to another submodule update")
	}
	for len(p.pending) != 0 {
		step := &p.pending[0]
		if err := ctx.Err(); err != nil {
			return p.snapshot(), err
		}
		if !step.item.Initialized {
			if err := step.parent.InitSubmodule(ctx, step.item); err != nil {
				return p.snapshot(), err
			}
			items, err := step.parent.Submodules(ctx)
			if err != nil {
				return p.snapshot(), err
			}
			i := slices.IndexFunc(items, func(item SubmoduleInfo) bool {
				return item.Name == step.item.Name && item.Path == step.item.Path &&
					item.IndexOID == step.item.IndexOID && item.URL == step.item.URL
			})
			if i < 0 || !items[i].Initialized {
				return p.snapshot(), fmt.Errorf("the submodule changed during registration: %q", step.path)
			}
			step.item = items[i]
		}
		var fetched *Repository
		config := step.selected
		if config == nil {
			config = step.parent
		}
		warning, err := retryWithHTTPSVault(ctx, config.git, config.Path(), options, func(attempt FetchOptions) (string, error) {
			child, warning, err := step.apply(ctx, attempt)
			fetched = child
			return warning, err
		})
		if err != nil {
			var challenge *RemoteAuthError
			if errors.As(err, &challenge) {
				p.challenge = challenge
			}
			return p.snapshot(), fmt.Errorf("submodule %q: %w", step.path, err)
		}
		p.challenge = nil
		options.Challenge, options.Credentials = nil, nil
		p.result.Updated = append(p.result.Updated, step.path)
		p.result.Warning = joinCheckoutWarning(p.result.Warning, warning)
		nested, err := fetched.Submodules(ctx)
		if err != nil {
			return p.snapshot(), err
		}
		following := make([]submoduleUpdate, 0, len(nested))
		for _, item := range nested {
			if item.IndexOID == "" {
				continue
			}
			nestedStep, err := fetched.submoduleUpdateStep(ctx, item, path.Join(step.path, item.Path))
			if err != nil {
				return p.snapshot(), err
			}
			following = append(following, nestedStep)
		}
		p.pending = append(following, p.pending[1:]...)
	}
	return p.snapshot(), nil
}

func (p *SubmoduleUpdatePlan) snapshot() SubmoduleUpdateResult {
	result := p.result
	result.Updated = slices.Clone(result.Updated)
	return result
}

func (s submoduleUpdate) apply(ctx context.Context, options FetchOptions) (*Repository, string, error) {
	var child *Repository
	var warning string
	r := s.parent
	err := r.withRefEdit(ctx, func() error {
		lock := &checkoutIndexLock{release: func() {}}
		defer lock.close()
		if err := lock.open(ctx, r); err != nil {
			return err
		}
		if err := r.verifySubmodule(ctx, s.item, false); err != nil {
			return err
		}
		root, err := os.OpenRoot(r.WorkTree)
		if err != nil {
			return err
		}
		defer root.Close()
		child, err = r.openSubmodule(ctx, root, s.item.Path)
		if err != nil {
			return err
		}
		if (s.selected == nil) != (child == nil) ||
			child != nil && !s.selected.SameCheckout(child) {
			return fmt.Errorf("the submodule checkout was replaced since it was selected")
		}
		endpoint, protocol, remote, err := s.endpoint(ctx, child)
		if err != nil {
			return err
		}
		if options.Challenge != nil && (endpoint != options.Challenge.endpoint || s.path != options.Challenge.Remote ||
			options.Challenge.operation != submoduleUpdateOperation) {
			return fmt.Errorf("the submodule URL changed since authentication was requested")
		}
		if child == nil {
			child, warning, err = s.materialize(ctx, lock, endpoint, protocol, options)
			return err
		}
		if endpoint != "" {
			warning, err = s.fetchCommit(ctx, child, endpoint, protocol, remote, options)
			if err != nil {
				return err
			}
		}
		if err := r.verifySubmodule(ctx, s.item, false); err != nil {
			return err
		}
		if !lock.ownsLock() {
			return fmt.Errorf("the superproject index lock was replaced")
		}
		if err := child.checkIndexCheckout(ctx); err != nil {
			return err
		}
		if s.item.Head == s.item.IndexOID {
			return nil
		}
		return s.checkout(ctx, child, s.item.Head)
	})
	return child, warning, err
}

func (s submoduleUpdate) checkout(ctx context.Context, child *Repository, oldHead string) error {
	plan, err := child.PrepareCheckout(ctx, CheckoutTarget{OID: s.item.IndexOID})
	if err != nil {
		return err
	}
	if plan.source.head != oldHead {
		return fmt.Errorf("the submodule HEAD changed during the update")
	}
	plan.keepLocal = true
	plan.advanceNote = "submodule: update"
	_, err = plan.Apply(ctx)
	return err
}

func (s submoduleUpdate) endpoint(ctx context.Context, child *Repository) (endpoint, protocol, remoteName string, err error) {
	repo, source := s.parent, s.item.registeredURL
	if child != nil {
		have, err := child.optionalRef(ctx, "rev-parse", "--verify", "--quiet", s.item.IndexOID+"^{commit}")
		if err != nil {
			return "", "", "", err
		}
		if have == s.item.IndexOID {
			return "", "", "", nil
		}
		repo, source = child, "origin"
		branch, err := child.optionalRef(ctx, "symbolic-ref", "-q", "HEAD")
		if err != nil {
			return "", "", "", err
		}
		if name, ok := strings.CutPrefix(branch, "refs/heads/"); ok {
			remote, err := child.optionalRef(ctx, "config", "--get", "branch."+name+".remote")
			if err != nil {
				return "", "", "", err
			}
			if remote != "" {
				source = remote
			}
		}
		remoteName = source
	}
	if source == "" {
		return "", "", "", fmt.Errorf("the submodule has no registered URL")
	}
	effective, err := repo.readLimited(ctx, "", 64<<10, "ls-remote", "--get-url", "--", source)
	if err != nil {
		return "", "", "", fmt.Errorf("could not resolve the submodule URL: %w", err)
	}
	endpoint, protocol, err = parseTransportSource(strings.TrimSuffix(string(effective), "\n"), filepath.Clean(repo.WorkTree))
	return endpoint, protocol, remoteName, err
}
