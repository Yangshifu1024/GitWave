package gitcli

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"gitwave.work/internal/process"
)

var ErrCloneTargetChanged = errors.New("the clone destination changed")

type CloneTarget struct {
	path         string
	ancestor     string
	ancestorInfo os.FileInfo
	git          string
	dir          os.FileInfo
	nonempty     bool
}

type CloneResult struct {
	Repo    *Repository
	Backup  string
	Warning string
}

type CloneOptions struct {
	Progress         func(TransferProgress)
	Credentials      *HTTPSCredential
	MediaCredentials *HTTPSCredential
	Challenge        *RemoteAuthError
	Vault            HTTPSCredentialVault
	BeforePublish    func(context.Context, string) error
	gitChallenge     *RemoteAuthError
	mediaChallenge   *RemoteAuthError
}

func (t CloneTarget) Path() string           { return t.path }
func (t CloneTarget) NeedsReplacement() bool { return t.nonempty }

func PrepareClone(ctx context.Context, destination string) (CloneTarget, error) {
	if strings.TrimSpace(destination) == "" {
		return CloneTarget{}, fmt.Errorf("select a clone destination")
	}
	path, err := filepath.Abs(destination)
	if err != nil {
		return CloneTarget{}, err
	}
	// A symlink at the destination is distinct from the directory it names.
	dir, nonempty, err := inspectCloneDirectory(path)
	if err != nil {
		return CloneTarget{}, err
	}
	git, err := findGit()
	if err != nil {
		return CloneTarget{}, err
	}
	ancestor, path, err := initLocation(ctx, git, path)
	if err != nil {
		return CloneTarget{}, err
	}
	if ancestor == path {
		ancestor = filepath.Dir(path)
	}
	if ancestor == path {
		return CloneTarget{}, fmt.Errorf("select a destination below the filesystem root")
	}
	ancestorInfo, err := os.Stat(ancestor)
	if err != nil {
		return CloneTarget{}, err
	}
	return CloneTarget{path: path, ancestor: ancestor, ancestorInfo: ancestorInfo, git: git, dir: dir, nonempty: nonempty}, nil
}

func inspectCloneDirectory(path string) (os.FileInfo, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.IsDir() {
		return nil, false, fmt.Errorf("clone destination must be a directory: %q", path)
	}
	dir, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer dir.Close()
	opened, err := dir.Stat()
	if err != nil {
		return nil, false, err
	}
	if !os.SameFile(info, opened) {
		return nil, false, ErrCloneTargetChanged
	}
	_, err = dir.Readdirnames(1)
	if errors.Is(err, io.EOF) {
		return opened, false, nil
	}
	return opened, err == nil, err
}

func (t CloneTarget) verify() error {
	dir, nonempty, err := inspectCloneDirectory(t.path)
	if err != nil {
		return err
	}
	if !sameFileIdentity(dir, t.dir) || nonempty != t.nonempty ||
		dir != nil && !dir.ModTime().Equal(t.dir.ModTime()) {
		return ErrCloneTargetChanged
	}
	return nil
}

// Clone stages the complete repository on the target filesystem. Publication
// never replaces an unconfirmed destination or deletes the previous contents.
func (t CloneTarget) Clone(ctx context.Context, source string, replace bool, requested CloneOptions) (CloneResult, error) {
	credentials := requested.Credentials
	if requested.Challenge != nil && requested.Challenge.media != "" {
		credentials = requested.MediaCredentials
	}
	transfer := FetchOptions{Challenge: requested.Challenge, Credentials: credentials, Vault: requested.Vault}
	return retryWithHTTPSVault(ctx, t.git, t.ancestor, transfer, func(attempt FetchOptions) (CloneResult, error) {
		if attempt.Challenge != nil {
			if attempt.Challenge.media != "" {
				requested.MediaCredentials = attempt.Credentials
				requested.mediaChallenge = attempt.Challenge
			} else {
				requested.Credentials = attempt.Credentials
				requested.gitChallenge = attempt.Challenge
			}
		}
		return t.clone(ctx, source, replace, requested)
	})
}

func (t CloneTarget) clone(ctx context.Context, source string, replace bool, requested CloneOptions) (CloneResult, error) {
	if t.path == "" || t.nonempty && !replace {
		return CloneResult{}, fmt.Errorf("confirm replacement of the clone destination")
	}
	if err := t.verify(); err != nil {
		return CloneResult{}, err
	}
	original, _, err := parseTransportSource(source, "")
	if err != nil {
		return CloneResult{}, err
	}
	root, err := os.OpenRoot(t.ancestor)
	if err != nil {
		return CloneResult{}, err
	}
	defer root.Close()
	ancestor, err := root.Stat(".")
	if err != nil || !sameFileIdentity(ancestor, t.ancestorInfo) {
		return CloneResult{}, ErrCloneTargetChanged
	}
	temp := ".gitwave-clone-" + rand.Text()
	if err := root.Mkdir(temp, 0o700); err != nil {
		return CloneResult{}, err
	}
	stageInfo, err := root.Lstat(temp)
	if err != nil {
		return CloneResult{}, err
	}
	defer func() {
		if current, err := root.Lstat(temp); err == nil && sameFileIdentity(current, stageInfo) {
			_ = root.RemoveAll(temp)
		}
	}()
	stageRoot, err := root.OpenRoot(temp)
	if err != nil {
		return CloneResult{}, err
	}
	defer stageRoot.Close()
	stage := filepath.Join(t.ancestor, temp)
	route := filepath.Join(stage, "source")
	if _, err := readGit(ctx, t.git, stage, nil, "", 64<<10,
		"init", "--quiet", "--bare", "--template=", "--", route); err != nil {
		return CloneResult{}, err
	}
	remote, protocol, err := t.resolveCloneSource(ctx, route, original)
	if err != nil {
		return CloneResult{}, err
	}
	if protocol == "file" && t.dir != nil && localCloneSourceWithin(remote, t.dir) {
		return CloneResult{}, fmt.Errorf("the clone destination contains the source repository")
	}
	for _, challenge := range []*RemoteAuthError{requested.gitChallenge, requested.mediaChallenge} {
		if challenge != nil && (challenge.operation != "clone" || challenge.endpoint != remote) {
			return CloneResult{}, fmt.Errorf("the clone source changed since authentication was requested")
		}
	}
	if requested.MediaCredentials != nil && requested.mediaChallenge == nil {
		return CloneResult{}, fmt.Errorf("retry authentication for the selected Git LFS endpoint")
	}
	const worktree = "repository"
	location := filepath.Join(stage, worktree)
	options := []string{"-c", "core.hooksPath=" + stage, "-c", "init.defaultRefFormat=files"}
	transport, err := prepareTransport(remote, protocol, requested.Credentials, requested.Progress)
	if err != nil {
		return CloneResult{}, err
	}
	defer transport.close()
	options = append(options, transport.options...)
	alias := "gitwave-clone:" + rand.Text()
	options = append(options, "-c", "url."+remote+".insteadOf="+alias)
	var configuredProxy string
	if protocol == "http" || protocol == "https" {
		configuration := Repository{git: t.git, GitDir: route}
		configuredProxy, _, err = configuration.readProxyConfig(ctx, "--get-urlmatch", "http.proxy", remote)
		if err != nil {
			return CloneResult{}, err
		}
	}
	cmd := gitCommand(ctx, t.git, route, options, "", "clone", "--no-checkout", "--no-local", "--progress", "--", alias, location)
	if err := transport.run(ctx, cmd, "clone", configuredProxy); err != nil {
		if errors.Is(err, ErrHTTPSAuthentication) {
			return CloneResult{}, remoteAuthError("clone", remote, "clone")
		}
		return CloneResult{}, err
	}
	stagedRepo, err := OpenRoot(ctx, location)
	if err != nil {
		return CloneResult{}, fmt.Errorf("could not open the cloned repository: %w", err)
	}
	origin, err := stagedRepo.restoreCloneURL(ctx, alias, original)
	if err != nil {
		return CloneResult{}, err
	}
	checkout, err := t.prepareCloneCheckout(ctx, stagedRepo, stage, origin, transport, requested)
	if err != nil {
		return CloneResult{}, err
	}
	defer checkout.close(transport)
	if err := checkout.apply(ctx, stagedRepo, stage, transport); err != nil {
		return CloneResult{}, err
	}
	if requested.BeforePublish != nil {
		if err := requested.BeforePublish(ctx, t.path); err != nil {
			return CloneResult{}, err
		}
	}
	// Validate with the same user-configured filters Git used to materialize
	// the checkout. The Changes reader deliberately isolates those drivers.
	verify := stagedRepo.command(ctx, "", checkout.gitArguments(stage,
		"status", "--porcelain=v2", "-z", "--untracked-files=all", "--ignore-submodules=dirty")...)
	status := checkout.statusCheck(stagedRepo)
	verify.Stdout = status
	if err := process.Run(verify); err != nil {
		if ctx.Err() != nil {
			return CloneResult{}, ctx.Err()
		}
		return CloneResult{}, fmt.Errorf("could not verify the cloned checkout: %w", err)
	}
	if status.changed || len(status.incomplete) != 0 {
		return CloneResult{}, fmt.Errorf("the cloned checkout contains unexpected changes")
	}
	if err := ctx.Err(); err != nil {
		return CloneResult{}, err
	}
	if err := checkout.verify(ctx, t, stagedRepo, origin); err != nil {
		return CloneResult{}, err
	}
	backup, err := t.publishDirectory(root, stageRoot, worktree, false)
	result := CloneResult{Backup: backup}
	if err != nil {
		return result, err
	}
	// Once published, complete repository discovery even if cancellation races
	// with the final rename. A completed clone can still be added to Workspace.
	result.Repo, err = OpenRoot(context.WithoutCancel(ctx), t.path)
	if err == nil && !os.SameFile(stagedRepo.gitDirInfo, result.Repo.gitDirInfo) {
		result.Repo = nil
		return result, fmt.Errorf("the cloned repository was replaced during publication")
	}
	if err == nil {
		result.Warning = transport.remember(ctx, t.git, stage, requested.Vault)
		if checkout.media != nil && checkout.media != transport {
			result.Warning = joinCheckoutWarning(result.Warning, checkout.media.remember(ctx, t.git, stage, requested.Vault))
		}
	}
	return result, err
}

func (t CloneTarget) resolveCloneSource(ctx context.Context, stage, source string) (string, string, error) {
	raw, err := readGit(ctx, t.git, stage, nil, "", 64<<10, "ls-remote", "--get-url", "--", source)
	if err != nil {
		return "", "", err
	}
	return parseTransportSource(strings.TrimSuffix(string(raw), "\n"), stage)
}

func (r *Repository) restoreCloneURL(ctx context.Context, alias, source string) (string, error) {
	raw, err := r.readLimited(ctx, "", 64<<10, "config", "--local", "--null", "--get-regexp", `^remote\..+\.url$`)
	if err != nil {
		return "", err
	}
	for record := range strings.SplitSeq(strings.TrimSuffix(string(raw), "\x00"), "\x00") {
		key, value, ok := strings.Cut(record, "\n")
		if ok && value == alias {
			if _, err := r.readLimited(ctx, "", 64<<10, "config", "--local", "--replace-all", key, source); err != nil {
				return "", err
			}
			return strings.TrimSuffix(strings.TrimPrefix(key, "remote."), ".url"), nil
		}
	}
	return "", fmt.Errorf("Git did not register the selected clone source")
}

func localCloneSourceWithin(remote string, target os.FileInfo) bool {
	path, err := localTransportPath(remote)
	if err != nil {
		return false
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	for ; ; path = filepath.Dir(path) {
		if info, err := os.Stat(path); err == nil && os.SameFile(info, target) {
			return true
		}
		if filepath.Dir(path) == path {
			return false
		}
	}
}

func localTransportPath(remote string) (string, error) {
	path := remote
	if strings.HasPrefix(remote, "file://") {
		address, err := url.Parse(remote)
		if err != nil {
			return "", err
		}
		path = filepath.FromSlash(address.Path)
		if runtime.GOOS == "windows" {
			if address.Host != "" && !strings.EqualFold(address.Host, "localhost") {
				path = "\\\\" + address.Host + path
			} else if filepath.VolumeName(strings.TrimPrefix(path, "\\")) != "" {
				path = strings.TrimPrefix(path, "\\")
			}
		}
	}
	return path, nil
}

func (t CloneTarget) publishDirectory(root, private *os.Root, worktree string, keepEmpty bool) (string, error) {
	ancestor, err := os.Stat(t.ancestor)
	if err != nil || !sameFileIdentity(ancestor, t.ancestorInfo) {
		return "", ErrCloneTargetChanged
	}
	if err := t.verify(); err != nil {
		return "", err
	}
	parentPath := filepath.Dir(t.path)
	local, err := filepath.Rel(t.ancestor, t.path)
	if err != nil {
		return "", err
	}
	parent, err := openWorktreeParent(root, local, true)
	if err != nil {
		return "", err
	}
	if parent != root {
		defer parent.Close()
	}
	name := filepath.Base(t.path)
	saved := ""
	if t.dir != nil {
		saved = ".gitwave-original-" + rand.Text()
		if err := moveDiscardFile(parent, name, parent, saved); err != nil {
			return "", err
		}
	}
	restore := func(cause error) (string, error) {
		if saved == "" {
			return "", cause
		}
		if err := moveDiscardFile(parent, saved, parent, name); err != nil {
			backup := filepath.Join(parentPath, saved)
			return backup, errors.Join(cause, fmt.Errorf("original directory remains at %q: %w", backup, err))
		}
		return "", cause
	}
	if saved != "" {
		info, err := parent.Lstat(saved)
		if err != nil || !sameFileIdentity(info, t.dir) || !info.ModTime().Equal(t.dir.ModTime()) ||
			keepEmpty && info.Mode() != t.dir.Mode() {
			return restore(ErrCloneTargetChanged)
		}
	}
	if err := moveDiscardFile(private, worktree, parent, name); err != nil {
		return restore(err)
	}
	if saved == "" || !keepEmpty && !t.nonempty && parent.Remove(saved) == nil {
		return "", nil
	}
	return filepath.Join(parentPath, saved), nil
}

var scpCloneSource = regexp.MustCompile("^(?:[^@/\\\\:]+@)?(?:[A-Za-z0-9_.-]+|\\[[^\\]]+\\]):.+$")
var remoteHelperSource = regexp.MustCompile("^[A-Za-z0-9+.-]+::")

func CloneProtocol(source string) string {
	_, protocol, _ := parseTransportSource(source, "")
	return protocol
}

func parseTransportSource(source, base string) (remote, protocol string, err error) {
	if base == "" {
		source = strings.TrimSpace(source)
	}
	switch {
	case strings.TrimSpace(source) == "" || strings.ContainsAny(source, "\x00\r\n"):
		return "", "", fmt.Errorf("enter a Git repository URL or path")
	case filepath.IsAbs(source) || filepath.VolumeName(source) != "":
		path, err := filepath.Abs(source)
		return path, "file", err
	case remoteHelperSource.MatchString(source):
		return "", "", fmt.Errorf("unsupported Git transport")
	case strings.Contains(source, "://"):
		address, err := url.Parse(source)
		if err != nil || address.Fragment != "" || address.RawQuery != "" {
			return "", "", fmt.Errorf("enter a Git URL without a fragment or query")
		}
		protocol := strings.ToLower(address.Scheme)
		switch protocol {
		case "http", "https", "git", "ssh":
			if address.Hostname() == "" {
				return "", "", fmt.Errorf("the Git URL must include a host")
			}
		case "file":
			if address.Path == "" {
				return "", "", fmt.Errorf("the file URL must include a path")
			}
		default:
			return "", "", fmt.Errorf("unsupported Git transport")
		}
		if address.User != nil {
			if _, hasPassword := address.User.Password(); hasPassword || protocol != "ssh" {
				return "", "", fmt.Errorf("remove credentials from the Git URL")
			}
		}
		address.Scheme = protocol
		return address.String(), protocol, nil
	case scpCloneSource.MatchString(source):
		return source, "ssh", nil
	default:
		if base != "" {
			source = filepath.Join(base, source)
		}
		path, err := filepath.Abs(source)
		return path, "file", err
	}
}
