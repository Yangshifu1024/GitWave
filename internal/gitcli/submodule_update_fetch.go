package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func (s submoduleUpdate) materialize(ctx context.Context, index *checkoutIndexLock, endpoint, protocol string,
	options FetchOptions) (*Repository, string, error) {
	r := s.parent
	target, err := PrepareClone(ctx, filepath.Join(r.WorkTree, filepath.FromSlash(s.item.Path)))
	if err != nil {
		return nil, "", err
	}
	if target.nonempty || target.path != filepath.Join(r.WorkTree, filepath.FromSlash(s.item.Path)) {
		return nil, "", fmt.Errorf("the submodule destination contains local files or a symbolic directory")
	}
	dir, err := os.MkdirTemp(target.ancestor, ".gitwave-submodule-update-")
	if err != nil {
		return nil, "", err
	}
	private, err := os.OpenRoot(dir)
	if err != nil {
		_ = os.Remove(dir)
		return nil, "", err
	}
	info, err := private.Stat(".")
	if err != nil {
		_ = private.Close()
		return nil, "", err
	}
	defer func() {
		if current, err := os.Lstat(dir); err == nil && sameFileIdentity(current, info) {
			_ = os.RemoveAll(dir)
		}
	}()
	defer private.Close()
	location := filepath.Join(dir, "checkout")
	var checkPermissions func() error
	if target.dir != nil {
		checkPermissions, err = prepareExistingWorktreeDirectory(ctx, target, private, "checkout")
		if err != nil {
			return nil, "", err
		}
	}
	format := "sha1"
	if len(s.item.IndexOID) == 64 {
		format = "sha256"
	}
	if _, err := readGit(ctx, r.git, dir, nil, "", 64<<10, "init", "--quiet", "--template=", "--object-format="+format,
		"--initial-branch=main", "--", location); err != nil {
		return nil, "", err
	}
	child, err := OpenRoot(ctx, location)
	if err != nil {
		return nil, "", err
	}
	source, _, err := parseTransportSource(s.item.registeredURL, r.WorkTree)
	if err != nil {
		return nil, "", err
	}
	if err := child.writeRefCommand(ctx, "remote", "add", "--", "origin", source); err != nil {
		return nil, "", err
	}
	warning, err := s.fetchCommit(ctx, child, endpoint, protocol, "origin", options)
	if err != nil {
		return nil, "", err
	}
	if err := s.checkout(ctx, child, ""); err != nil {
		return nil, "", err
	}
	if err := r.verifySubmodule(ctx, s.item, false); err != nil {
		return nil, "", err
	}
	if !index.ownsLock() {
		return nil, "", fmt.Errorf("the superproject index lock was replaced")
	}
	if checkPermissions != nil {
		if err := checkPermissions(); err != nil {
			return nil, "", err
		}
	}
	parent, err := os.OpenRoot(target.ancestor)
	if err != nil {
		return nil, "", err
	}
	defer parent.Close()
	if _, err := target.publishDirectory(parent, private, "checkout", false); err != nil {
		return nil, "", err
	}
	live, err := OpenRoot(context.WithoutCancel(ctx), target.path)
	if err != nil || !sameFileIdentity(child.gitDirInfo, live.gitDirInfo) {
		return nil, "", fmt.Errorf("the submodule checkout changed during publication")
	}
	return live, warning, nil
}

func (s submoduleUpdate) fetchCommit(ctx context.Context, child *Repository, endpoint, protocol, remote string,
	options FetchOptions) (string, error) {
	if err := child.checkIndexCheckout(ctx); err != nil {
		return "", err
	}
	state, err := child.References(ctx)
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "gitwave-submodule-fetch-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	staged, err := child.prepareTransportRepo(ctx, dir, state.Shallow)
	if err != nil {
		return "", err
	}
	if err := staged.bindCredentialContext(ctx, endpoint); err != nil {
		return "", err
	}
	if err := staged.bindProxyContext(ctx, remote, endpoint); err != nil {
		return "", err
	}
	alias, err := staged.bindEndpoint(ctx, endpoint)
	if err != nil {
		return "", err
	}
	transfer, err := prepareTransport(endpoint, protocol, options.Credentials, func(progress TransferProgress) {
		if options.Progress != nil {
			options.Progress(s.path, progress)
		}
	})
	if err != nil {
		return "", err
	}
	defer transfer.close()
	args := append([]string{}, transfer.options...)
	args = append(args, "fetch", "--no-all", "--no-write-fetch-head", "--no-recurse-submodules",
		"--no-auto-maintenance", "--no-write-commit-graph", "--progress", "--refmap=",
		"--upload-pack=git-upload-pack", "--", alias)
	if err := transfer.run(ctx, staged.command(ctx, "", append(args, "+refs/heads/*:refs/remotes/origin/*")...), "submodule fetch", staged.proxyURLs...); err != nil {
		if errors.Is(err, ErrHTTPSAuthentication) {
			return "", remoteAuthError(s.path, endpoint, submoduleUpdateOperation)
		}
		return "", err
	}
	if err := child.requireCommit(ctx, s.item.IndexOID); err != nil {
		if err := transfer.run(ctx, staged.command(ctx, "", append(args, s.item.IndexOID)...), "submodule fetch", staged.proxyURLs...); err != nil {
			if errors.Is(err, ErrHTTPSAuthentication) {
				return "", remoteAuthError(s.path, endpoint, submoduleUpdateOperation)
			}
			return "", err
		}
	}
	if err := child.requireCommit(ctx, s.item.IndexOID); err != nil {
		return "", err
	}
	return transfer.remember(ctx, child.git, dir, options.Vault), nil
}
