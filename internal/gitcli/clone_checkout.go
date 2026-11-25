package gitcli

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

type cloneCheckout struct {
	tip     string
	source  remoteSource
	filters []string
	lfs     lfsDownloadPlan
	receipt lfsDownloadReceipt
	media   *transportAttempt
}

type cloneStatusCheck struct {
	cacheFiles map[string]bool
	incomplete []byte
	changed    bool
}

func (c *cloneCheckout) statusCheck(repo *Repository) *cloneStatusCheck {
	check := &cloneStatusCheck{}
	if len(c.receipt.files) == 0 {
		return check
	}
	relative, err := filepath.Rel(repo.WorkTree, c.receipt.storage)
	if err != nil || !filepath.IsLocal(relative) {
		return check
	}
	check.cacheFiles = make(map[string]bool, len(c.receipt.files))
	for pointer := range c.receipt.files {
		name := filepath.ToSlash(filepath.Join(relative, lfsObjectPath(pointer.oid)))
		check.cacheFiles[name] = true
	}
	return check
}

func (s *cloneStatusCheck) Write(data []byte) (int, error) {
	if s.changed {
		return len(data), nil
	}
	s.incomplete = append(s.incomplete, data...)
	for {
		record, rest, complete := bytes.Cut(s.incomplete, []byte{0})
		if !complete {
			if len(s.incomplete) > 1<<20 {
				s.changed = true
			}
			break
		}
		s.incomplete = rest
		path, untracked := strings.CutPrefix(string(record), "? ")
		if !untracked || !s.cacheFiles[path] {
			s.changed = true
			break
		}
	}
	return len(data), nil
}

func (t CloneTarget) prepareCloneCheckout(ctx context.Context, repo *Repository, stage, origin string,
	git *transportAttempt, requested CloneOptions) (_ *cloneCheckout, err error) {
	source := remoteSource{endpoint: git.remote, protocol: git.protocol}
	prepared := &cloneCheckout{source: source}
	endpoint, err := repo.remoteEndpoint(ctx, origin)
	if err != nil {
		return nil, err
	}
	if endpoint != source {
		return nil, fmt.Errorf("the configured clone source changed during transfer")
	}
	prepared.tip, err = repo.optionalRef(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return nil, err
	}
	if prepared.tip == "" {
		if requested.MediaCredentials != nil {
			return nil, fmt.Errorf("the clone has no checkout for the selected Git LFS endpoint")
		}
		return prepared, nil
	}
	if err := checkCloneTreePaths(ctx, repo, prepared.tip); err != nil {
		return nil, err
	}
	_, config, err := repo.workingGitSettings(ctx)
	if err != nil {
		return nil, err
	}
	storage, err := t.cloneLFSStorage(ctx, repo, config)
	if err != nil {
		return nil, err
	}
	prepared.lfs, err = repo.prepareLFSDownloadAt(ctx, origin, storage, true)
	if err != nil {
		return nil, err
	}
	if challenge := requested.mediaChallenge; challenge != nil &&
		(!prepared.lfs.enabled() || challenge.media != prepared.lfs.httpEndpoint()) {
		return nil, fmt.Errorf("the Git LFS endpoint changed since authentication was requested")
	}
	if prepared.lfs.enabled() {
		prepared.media, err = prepared.lfs.mediaTransport(git, requested.MediaCredentials, requested.Progress)
		if err != nil {
			return nil, err
		}
		defer func() {
			if err != nil {
				prepared.close(git)
			}
		}()
		isolated, err := repo.prepareTransportRepo(ctx, stage, "")
		if err != nil {
			return nil, err
		}
		if err := prepared.lfs.download(ctx, isolated, origin, prepared.tip, prepared.media, nil, &prepared.receipt); err != nil {
			if errors.Is(err, ErrHTTPSAuthentication) {
				challenge := lfsAuthError("clone", git.remote, prepared.media.remote)
				challenge.operation = "clone"
				return nil, challenge
			}
			return nil, err
		}
	}
	prepared.filters = cloneLFSFilters(stage, config, prepared.lfs, prepared.receipt.paths != nil)
	return prepared, nil
}

func (t CloneTarget) cloneLFSStorage(ctx context.Context, repo *Repository, config localLFSConfig) (string, error) {
	if config.standardDriver() == "" {
		return "", nil
	}
	raw, err := repo.optionalRef(ctx, "config", "--get", "lfs.storage")
	if err != nil || raw == "" {
		return "", err
	}
	configured := config.storage
	if !filepath.IsAbs(raw) {
		configured = filepath.Join(t.path, ".git", raw)
	}
	// Paths inside the final checkout must move with the new directory.
	// Apply absolute or parent-relative storage paths as seen after the rename.
	relative, inside, err := t.cloneCacheRelative(repo.WorkTree, configured)
	if err != nil {
		return "", err
	}
	if inside {
		return filepath.Join(repo.WorkTree, relative), nil
	}
	return configured, nil
}

func (t CloneTarget) cloneCacheRelative(stage, configured string) (string, bool, error) {
	relative, err := filepath.Rel(t.path, configured)
	if err == nil && filepath.IsLocal(relative) {
		return relative, true, nil
	}
	// An existing destination can also be reached through a case variant,
	// a normalization variant, or a symlink. Compare directory identities.
	if t.dir != nil {
		for candidate := configured; ; candidate = filepath.Dir(candidate) {
			if info, err := os.Stat(candidate); err == nil && os.SameFile(info, t.dir) {
				relative, err := filepath.Rel(candidate, configured)
				return relative, err == nil, err
			}
			if filepath.Dir(candidate) == candidate {
				break
			}
		}
	}
	_, target, err := resolveExistingPath(t.path)
	if err != nil {
		return "", false, err
	}
	_, physical, err := resolveExistingPath(configured)
	if err != nil {
		return "", false, err
	}
	if relative, err := filepath.Rel(target, physical); err == nil && filepath.IsLocal(relative) {
		return relative, true, nil
	}
	root, err := os.OpenRoot(stage)
	if err != nil {
		return "", false, err
	}
	defer root.Close()
	foldsCase, normalizes, err := probeCheckoutNames(root)
	if err != nil || !foldsCase && !normalizes {
		return "", false, err
	}
	prefix := strings.Split(target, string(os.PathSeparator))
	actual := strings.Split(physical, string(os.PathSeparator))
	if len(actual) < len(prefix) {
		return "", false, nil
	}
	fold := cases.Fold()
	key := func(name string) string {
		if foldsCase {
			name = fold.String(name)
		}
		if normalizes {
			name = norm.NFC.String(name)
		}
		return name
	}
	for i, name := range prefix {
		if key(name) != key(actual[i]) {
			return "", false, nil
		}
	}
	return filepath.Join(append([]string{"."}, actual[len(prefix):]...)...), true, nil
}

func checkCloneTreePaths(ctx context.Context, repo *Repository, tip string) error {
	out, err := repo.readLimited(ctx, "", 128<<20, "ls-tree", "-r", "--name-only", "-z", tip, "--")
	if err != nil {
		return err
	}
	var paths []string
	if len(out) != 0 {
		paths = strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	}
	for _, path := range paths {
		if !validIndexPath(path) {
			return fmt.Errorf("unsupported clone checkout path %q", path)
		}
	}
	root, err := os.OpenRoot(repo.WorkTree)
	if err != nil {
		return err
	}
	defer root.Close()
	return checkCheckoutAliases(root, paths)
}

func cloneLFSFilters(stage string, config localLFSConfig, download lfsDownloadPlan, scanned bool) []string {
	if config.standardDriver() == "" {
		return nil
	}
	if !scanned {
		return []string{"-c", "filter.lfs.process=", "-c", "filter.lfs.clean=", "-c", "filter.lfs.smudge=", "-c", "filter.lfs.required=false"}
	}
	executable := "'" + strings.ReplaceAll(filepath.ToSlash(download.executable), "'", "'\\''") + "'"
	localPath := filepath.ToSlash(filepath.Join(stage, "git"))
	if !strings.HasPrefix(localPath, "/") {
		localPath = "/" + localPath
	}
	local := (&url.URL{Scheme: "file", Path: localPath}).String()
	alias := "gitwave-clone-lfs:" + rand.Text()
	// Checkout may only materialize the validated cache. A cache miss
	// can reach the private file endpoint, never a network LFS server.
	return []string{
		"-c", "filter.lfs.process=" + executable + " filter-process", "-c", "filter.lfs.required=true",
		"-c", "lfs.storage=" + download.storage, "-c", "lfs.fetchinclude=" + download.include, "-c", "lfs.fetchexclude=" + download.exclude,
		"-c", "url." + local + ".insteadOf=" + alias, "-c", "lfs.url=" + alias, "-c", "lfs.standalonetransferagent=",
		"-c", "lfs." + local + ".standalonetransferagent=", "-c", "lfs.basictransfersonly=true", "-c", "lfs.skipdownloaderrors=false",
	}
}

func (c *cloneCheckout) gitArguments(stage string, args ...string) []string {
	options := append([]string{"-c", "core.hooksPath=" + stage}, c.filters...)
	return append(options, args...)
}

func (c *cloneCheckout) apply(ctx context.Context, repo *Repository, stage string, git *transportAttempt) error {
	if c.tip == "" {
		return nil
	}
	if report := git.output.report; report != nil {
		report(TransferProgress{Phase: "Checking out files"})
	}
	args := c.gitArguments(stage, "read-tree", "--reset", "-u", c.tip)
	args = append(append([]string{}, git.options...), args...)
	cmd := repo.command(ctx, "", args...)
	if err := git.run(ctx, cmd, "read-tree"); errors.Is(err, ErrHTTPSAuthentication) {
		return remoteAuthError("clone", git.remote, "clone")
	} else if err != nil {
		return err
	}
	if git.output.collision {
		return fmt.Errorf("the destination filesystem cannot represent all paths in the cloned repository")
	}
	return c.receipt.verify(ctx)
}

func (c *cloneCheckout) verifyMaterializedLFS(ctx context.Context, repo *Repository) error {
	if len(c.receipt.paths) == 0 {
		return nil
	}
	worktree, err := os.OpenRoot(repo.WorkTree)
	if err != nil {
		return err
	}
	defer worktree.Close()
	for name, object := range c.receipt.paths {
		if c.lfs.config.extensions {
			// The extension transforms the media before it reaches disk.
			// Verify that the checkout converts back to the indexed blob.
			want, err := repo.optionalRef(ctx, "rev-parse", "--verify", "--quiet", c.tip+":"+name)
			if err != nil {
				return err
			}
			args := c.gitArguments(filepath.Dir(repo.WorkTree), "hash-object", "--path="+name, "--", name)
			raw, err := repo.readLimited(ctx, "", 1024, args...)
			if err != nil || !validOID(want) || strings.TrimSpace(string(raw)) != want {
				return fmt.Errorf("verify cloned LFS extension file %q: pointer changed", name)
			}
			continue
		}
		if err := verifyClonedLFSPath(ctx, worktree, name, object); err != nil {
			return err
		}
	}
	return nil
}

func verifyClonedLFSPath(ctx context.Context, worktree *os.Root, name string, pointer lfsPointer) error {
	if !validIndexPath(name) {
		return fmt.Errorf("invalid cloned LFS path %q", name)
	}
	path := filepath.FromSlash(name)
	parent, err := openWorktreeParent(worktree, path, false)
	if err != nil {
		return err
	}
	if parent != worktree {
		defer parent.Close()
	}
	file, err := openLFSRegular(parent, filepath.Base(path))
	if err != nil {
		return err
	}
	defer file.Close()
	if err := copyLFSObject(ctx, io.Discard, file, pointer); err != nil {
		return fmt.Errorf("verify cloned LFS file %q: %w", name, err)
	}
	return nil
}

func (c *cloneCheckout) verify(ctx context.Context, target CloneTarget, repo *Repository, origin string) error {
	fresh, err := OpenRoot(ctx, repo.Path())
	if err != nil {
		return err
	}
	if !repo.SameCheckout(fresh) {
		return fmt.Errorf("the clone checkout changed before publication")
	}
	tip, err := fresh.optionalRef(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return err
	}
	if tip != c.tip {
		return fmt.Errorf("the clone HEAD changed before publication")
	}
	endpoint, err := fresh.remoteEndpoint(ctx, origin)
	if err != nil {
		return err
	}
	if endpoint != c.source {
		return fmt.Errorf("the clone source changed before publication")
	}
	if c.lfs.enabled() {
		if err := c.lfs.verify(ctx, fresh, origin, c.source); err != nil {
			return err
		}
		storage, err := target.cloneLFSStorage(ctx, fresh, c.lfs.config)
		if err != nil {
			return err
		}
		if storage == "" {
			storage = c.lfs.config.storage
		}
		if storage != c.lfs.storage {
			return fmt.Errorf("the final Git LFS cache changed before clone publication")
		}
	}
	if err := c.receipt.verify(ctx); err != nil {
		return err
	}
	return c.verifyMaterializedLFS(ctx, fresh)
}

func (c *cloneCheckout) close(git *transportAttempt) {
	if c.media != nil && c.media != git {
		c.media.close()
	}
}
