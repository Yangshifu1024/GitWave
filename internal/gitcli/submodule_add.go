package gitcli

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type SubmoduleAddPlan struct {
	mu            sync.Mutex
	repo          *Repository
	path, url     string
	resolvedURL   string
	endpoint      string
	head, headRef string
	modules       gitStorageSnapshot
	target        CloneTarget
	challenge     *RemoteAuthError
	added         bool
}

const submoduleAddOperation = "submodule-add"

func (r *Repository) PrepareSubmoduleAdd(ctx context.Context, source, relative string) (*SubmoduleAddPlan, error) {
	if r.WorkTree == "" || !validIndexPath(relative) || strings.EqualFold(strings.Split(relative, "/")[0], ".gitmodules") {
		return nil, fmt.Errorf("enter a submodule path inside the working copy")
	}
	if strings.TrimSpace(source) == "" || strings.ContainsAny(source, "\x00\r\n") {
		return nil, fmt.Errorf("enter a submodule URL")
	}
	target, err := PrepareClone(ctx, filepath.Join(r.WorkTree, filepath.FromSlash(relative)))
	if err != nil {
		return nil, err
	}
	if target.path != filepath.Join(r.WorkTree, filepath.FromSlash(relative)) {
		return nil, fmt.Errorf("the submodule path traverses a symbolic directory")
	}
	root, err := os.OpenRoot(r.WorkTree)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	modules, err := readGitStorageSnapshot(root, ".gitmodules", gitmodulesLimit)
	if err != nil {
		return nil, err
	}
	refs, err := r.References(ctx)
	if err != nil {
		return nil, err
	}
	p := &SubmoduleAddPlan{repo: r, path: relative, url: source, head: refs.Head, headRef: refs.HeadRef,
		modules: modules, target: target}
	if err := p.checkAvailable(ctx, root); err != nil {
		return nil, err
	}
	p.resolvedURL, p.endpoint, err = r.submoduleAddURL(ctx, source, refs.HeadRef)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func (r *Repository) submoduleAddURL(ctx context.Context, source, headRef string) (resolved, endpoint string, err error) {
	resolved = source
	if strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") {
		resolved, err = r.resolveRelativeSubmoduleURL(ctx, source, headRef)
		if err != nil {
			return "", "", err
		}
	}
	normalized, _, err := parseTransportSource(resolved, r.WorkTree)
	if err != nil {
		return "", "", err
	}
	raw, err := r.readLimited(ctx, "", 64<<10, "ls-remote", "--get-url", "--", normalized)
	if err != nil {
		return "", "", err
	}
	endpoint, _, err = parseTransportSource(strings.TrimSuffix(string(raw), "\n"), r.WorkTree)
	return normalized, endpoint, err
}

// Ask Git to resolve ./ and ../ against the superproject's default remote.
func (r *Repository) resolveRelativeSubmoduleURL(ctx context.Context, source, headRef string) (string, error) {
	name := "origin"
	if branch, ok := strings.CutPrefix(headRef, "refs/heads/"); ok {
		value, err := r.optionalRef(ctx, "config", "--get", "branch."+branch+".remote")
		if err != nil {
			return "", err
		}
		if value != "" {
			name = value
		}
	}
	origin, err := r.optionalRef(ctx, "config", "--get", "remote."+name+".url")
	if err != nil {
		return "", err
	}
	if origin == "" {
		origin = r.WorkTree
	}
	git, err := r.workingGit(ctx, "")
	if err != nil {
		return "", err
	}
	defer git.close()
	if err := git.isolateWorkTree(); err != nil {
		return "", err
	}
	git.index = filepath.Join(git.dir, "index")
	for _, args := range [][]string{
		{"config", "--local", "remote.origin.url", origin},
		{"config", "--file", ".gitmodules", "submodule.target.path", "target"},
		{"config", "--file", ".gitmodules", "submodule.target.url", source},
		{"update-index", "--add", "--cacheinfo", "160000," + strings.Repeat("1", git.oidSize) + ",target"},
		{"submodule", "init", "--", "target"},
	} {
		if _, err := git.read(ctx, 64<<10, args...); err != nil {
			return "", fmt.Errorf("could not resolve the relative submodule URL: %w", err)
		}
	}
	out, err := git.read(ctx, 64<<10, "config", "--local", "--get", "submodule.target.url")
	return strings.TrimSuffix(string(out), "\n"), err
}

func (p *SubmoduleAddPlan) checkAvailable(ctx context.Context, root *os.Root) error {
	if err := p.target.verify(); err != nil {
		return err
	}
	parent, err := openWorktreeParent(root, filepath.FromSlash(p.path), false)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if parent != nil && parent != root {
		parent.Close()
	}
	if p.target.nonempty {
		child, err := p.repo.openSubmodule(ctx, root, p.path)
		if err != nil {
			return err
		}
		if child == nil {
			return fmt.Errorf("the submodule destination contains local files")
		}
	}
	items, err := p.repo.parseSubmoduleEntries(ctx, p.modules.data)
	if err != nil {
		return err
	}
	paths := []string{p.path}
	for _, item := range items {
		if item.Name == p.path || item.Path == p.path || strings.HasPrefix(item.Path, p.path+"/") ||
			strings.HasPrefix(p.path, item.Path+"/") {
			return fmt.Errorf("the path already belongs to a submodule: %q", item.Path)
		}
		paths = append(paths, item.Path)
	}
	gitlinks, err := p.repo.indexGitlinks(ctx, false)
	if err != nil {
		return err
	}
	for path := range gitlinks {
		paths = append(paths, path)
	}
	if err := checkCheckoutAliases(root, paths); err != nil {
		return err
	}
	entries, err := p.repo.readConfigValues(ctx, submoduleConfigPattern)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name, _, _ := submoduleConfigKey(entry.key)
		if name == p.path {
			return fmt.Errorf("a submodule named %q is already registered", p.path)
		}
	}
	data, err := p.repo.readLimited(ctx, "", 64<<10, "--literal-pathspecs", "ls-files", "-z", "--", p.path)
	if err != nil {
		return err
	}
	if len(data) != 0 {
		return fmt.Errorf("the submodule path already contains indexed files")
	}
	_, err = p.repo.readLimited(ctx, "", 64<<10, "check-ignore", "-q", "--", "./"+p.path)
	if gitExitCode(err, 1) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("the submodule path is ignored by Git")
}

func (p *SubmoduleAddPlan) Apply(ctx context.Context, options FetchOptions) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return retryWithHTTPSVault(ctx, p.repo.git, p.repo.Path(), options, func(attempt FetchOptions) (string, error) {
		return p.apply(ctx, attempt)
	})
}

func (p *SubmoduleAddPlan) apply(ctx context.Context, options FetchOptions) (string, error) {
	if p.added {
		return "", nil
	}
	if (p.challenge != nil && !p.challenge.SameChallenge(options.Challenge)) ||
		(p.challenge == nil && (options.Challenge != nil || options.Credentials != nil)) {
		return "", fmt.Errorf("the authentication prompt belongs to another submodule")
	}
	r := p.repo
	root, err := os.OpenRoot(r.WorkTree)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if err := p.verify(ctx, root); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(p.target.ancestor, ".gitwave-submodule-add-")
	if err != nil {
		return "", err
	}
	private, err := os.OpenRoot(dir)
	if err != nil {
		_ = os.Remove(dir)
		return "", err
	}
	info, err := private.Stat(".")
	if err != nil {
		_ = private.Close()
		return "", err
	}
	retain := false
	defer func() {
		if current, err := os.Lstat(dir); !retain && err == nil && sameFileIdentity(current, info) {
			_ = os.RemoveAll(dir)
		}
	}()
	defer private.Close()
	var child *Repository
	var warning string
	var checkPermissions func() error
	if p.target.nonempty {
		child, err = r.openSubmodule(ctx, root, p.path)
		if err != nil || child == nil {
			return "", fmt.Errorf("the existing submodule checkout is unavailable")
		}
	} else {
		if p.target.dir != nil {
			checkPermissions, err = prepareExistingWorktreeDirectory(ctx, p.target, private, "checkout")
			if err != nil {
				return "", err
			}
		}
		child, warning, err = p.clone(ctx, private.Name(), options)
		if errors.Is(err, ErrHTTPSAuthentication) {
			p.challenge = remoteAuthError(p.path, p.endpoint, submoduleAddOperation)
			return "", p.challenge
		}
		if err != nil {
			return "", err
		}
	}
	if err := p.publish(ctx, root, private, child, checkPermissions); err != nil {
		var recovery *submoduleRecoveryError
		retain = errors.As(err, &recovery)
		return "", err
	}
	p.added = true
	p.challenge = nil
	return warning, nil
}

func (p *SubmoduleAddPlan) clone(ctx context.Context, dir string, options FetchOptions) (*Repository, string, error) {
	_, protocol, err := parseTransportSource(p.endpoint, p.repo.WorkTree)
	if err != nil {
		return nil, "", err
	}
	transfer, err := prepareTransport(p.endpoint, protocol, options.Credentials, func(progress TransferProgress) {
		if options.Progress != nil {
			options.Progress(p.path, progress)
		}
	})
	if err != nil {
		return nil, "", err
	}
	defer transfer.close()
	checkout := filepath.Join(dir, "checkout")
	alias := "gitwave-submodule:" + rand.Text()
	config := filepath.Join(dir, "clone.config")
	if _, err := readGit(ctx, p.target.git, dir, nil, "", 64<<10, "config", "--file", config,
		"--add", "url."+p.endpoint+".insteadOf", alias); err != nil {
		return nil, "", err
	}
	settings := append([]string{"-c", "core.hooksPath=" + dir, "-c", "include.path=" + config}, transfer.options...)
	credentialContext, err := p.repo.credentialContextFile(ctx, dir, p.endpoint)
	if err != nil {
		return nil, "", err
	}
	if credentialContext != "" {
		settings = append(settings, "-c", "include.path="+credentialContext)
	}
	proxyContext, proxyURLs, err := p.repo.proxyContextFile(ctx, dir, "", p.endpoint)
	if err != nil {
		return nil, "", err
	}
	if proxyContext != "" {
		settings = append(settings, "-c", "include.path="+proxyContext)
	}
	cmd := gitCommand(ctx, p.target.git, dir, settings, "", "clone", "--no-local", "--no-checkout", "--progress", "--", alias, checkout)
	if err := transfer.run(ctx, cmd, "clone submodule", proxyURLs...); err != nil {
		return nil, "", err
	}
	child, err := OpenRoot(ctx, checkout)
	if err != nil {
		return nil, "", err
	}
	if err := child.writeRefCommand(ctx, "remote", "set-url", "--", "origin", p.resolvedURL); err != nil {
		return nil, "", err
	}
	refs, err := child.References(ctx)
	if err != nil {
		return nil, "", err
	}
	if !validOID(refs.Head) {
		return nil, "", fmt.Errorf("the cloned submodule has no default commit")
	}
	if err := child.writeRefCommand(ctx, "symbolic-ref", "HEAD", "refs/heads/gitwave-"+rand.Text()); err != nil {
		return nil, "", err
	}
	plan, err := child.PrepareCheckout(ctx, CheckoutTarget{Ref: refs.HeadRef, OID: refs.Head})
	if err != nil {
		return nil, "", err
	}
	if _, err := plan.Apply(ctx); err != nil {
		return nil, "", err
	}
	return child, transfer.remember(ctx, p.target.git, dir, options.Vault), nil
}

func (p *SubmoduleAddPlan) verify(ctx context.Context, root *os.Root) error {
	r := p.repo
	if err := r.checkIndexCheckout(ctx); err != nil {
		return err
	}
	if !r.headMatches(ctx, p.headRef, p.head) {
		return fmt.Errorf("the superproject HEAD changed during submodule setup")
	}
	data, err := readGitStorageSnapshot(root, ".gitmodules", gitmodulesLimit)
	if err != nil || !p.modules.same(data) {
		return fmt.Errorf(".gitmodules changed during submodule setup")
	}
	if p.modules.mode == 0 {
		tracked, err := r.trackedGitmodules(ctx)
		if err != nil {
			return err
		}
		if tracked != "" {
			return fmt.Errorf("restore the tracked .gitmodules before adding a submodule")
		}
	}
	resolved, endpoint, err := r.submoduleAddURL(ctx, p.url, p.headRef)
	if err != nil {
		return err
	}
	if resolved != p.resolvedURL || endpoint != p.endpoint {
		return fmt.Errorf("the resolved submodule URL changed during setup")
	}
	return p.checkAvailable(ctx, root)
}
