package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type fetchStage struct {
	*transportRepo
	before           References
	fetchHead        []byte
	fetchHeadReadErr error
}

func (r *Repository) prepareFetchStage(ctx context.Context, dir string) (*fetchStage, error) {
	refs, err := r.References(ctx)
	if err != nil {
		return nil, err
	}
	isolated, err := r.prepareTransportRepo(ctx, dir, refs.Shallow)
	if err != nil {
		return nil, err
	}
	stage := &fetchStage{transportRepo: isolated, before: refs}
	stage.fetchHead, stage.fetchHeadReadErr = readFetchHead(r.GitDir)
	var seed strings.Builder
	for _, ref := range refs.Refs {
		if ref.Symbolic == "" {
			seed.WriteString("create " + ref.Name + "\x00" + ref.OID + "\x00")
		}
	}
	if seed.Len() != 0 {
		cmd := stage.command(ctx, seed.String(), "update-ref", "--stdin", "-z")
		if _, err := readGitOutput(ctx, cmd, 64<<10, "prepare fetch refs"); err != nil {
			return nil, err
		}
	}
	return stage, nil
}

func (s *fetchStage) commitTip(ctx context.Context, ref string) (string, error) {
	cmd := s.command(ctx, "", "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	out, err := readGitOutput(ctx, cmd, 1024, "read the fetched branch tip")
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return "", nil
	}
	return strings.TrimSuffix(string(out), "\n"), err
}

func (s *fetchStage) publish(ctx context.Context, remote string, expected *RemoteInfo, refspecs []string,
	verifyMedia func(context.Context) error) (string, error) {
	raw, err := readGitOutput(ctx, s.command(ctx, "", "for-each-ref", "--format=%(refname)%00%(objectname)",
		"refs/remotes", "refs/tags", "refs/heads"), 32<<20, "read fetched refs")
	if err != nil {
		return "", err
	}
	shallow, err := os.ReadFile(filepath.Join(s.dir, "shallow"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if string(shallow) != s.before.Shallow {
		return "", fmt.Errorf("fetch changed the shallow boundary of this checkout")
	}
	fresh, err := OpenRoot(ctx, s.repo.Path())
	if err != nil {
		return "", err
	}
	if !s.repo.SameCheckout(fresh) {
		return "", fmt.Errorf("the checkout changed before publishing fetched refs")
	}
	currentShallow, err := os.ReadFile(filepath.Join(fresh.CommonDir, "shallow"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if string(currentShallow) != s.before.Shallow {
		return "", fmt.Errorf("the checkout's shallow boundary changed during fetch")
	}
	verifyRemote := func() error {
		if expected == nil {
			return nil
		}
		_, err := fresh.matchRemote(ctx, *expected)
		return err
	}
	if err := verifyRemote(); err != nil {
		return "", err
	}
	var statement strings.Builder
	updated := make(map[string]bool)
	seen := make(map[string]bool)
	local := make(map[string]bool)
	var updatedLocal []string
	for _, spec := range refspecs {
		if ref := localFetchDestination(spec); ref != "" {
			local[ref] = true
		}
	}
	for line := range strings.SplitSeq(strings.TrimSuffix(string(raw), "\n"), "\n") {
		if line == "" {
			continue
		}
		name, oid, ok := strings.Cut(line, "\x00")
		if !ok || !validOID(oid) {
			return "", fmt.Errorf("invalid fetched reference")
		}
		if !fetchDestination(name) && !local[name] {
			continue
		}
		seen[name] = true
		old := findRef(s.before, name)
		if old.Symbolic != "" {
			return "", fmt.Errorf("fetch destination %q is a symbolic reference", name)
		}
		if old.OID == "" {
			statement.WriteString("option no-deref\x00create " + name + "\x00" + oid + "\x00")
		} else if old.OID != oid && (strings.HasPrefix(name, "refs/remotes/") || local[name]) {
			statement.WriteString("option no-deref\x00update " + name + "\x00" + oid + "\x00" + old.OID + "\x00")
		} else {
			continue
		}
		updated[name] = true
		if local[name] {
			updatedLocal = append(updatedLocal, name)
		}
	}
	for _, old := range s.before.Refs {
		if strings.HasPrefix(old.Name, "refs/remotes/") && old.Symbolic == "" && !seen[old.Name] {
			statement.WriteString("option no-deref\x00delete " + old.Name + "\x00" + old.OID + "\x00")
			updated[old.Name] = true
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if statement.Len() != 0 {
		format, err := fresh.optionalRef(ctx, "rev-parse", "--show-ref-format")
		if err != nil {
			return "", err
		}
		// Git versions before reftable may echo an unknown option.
		files := format != "reftable"
		if err := fresh.protectFetchRefPaths(ctx, updated, files); err != nil {
			return "", err
		}
		if len(updatedLocal) != 0 {
			release, err := fresh.lockBranchWorktreeIndices(ctx, nil)
			if err != nil {
				return "", err
			}
			defer release()
		}
		finalize, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		cmd := fresh.command(finalize, "", "-c", "core.hooksPath="+s.dir,
			"update-ref", "--stdin", "-z", "-m", "fetch: "+remote)
		if _, _, err := runReferenceTransaction(cmd, statement.String(), func() error {
			if err := verifyRemote(); err != nil {
				return err
			}
			if err := fresh.protectFetchHeads(finalize, updated); err != nil {
				return err
			}
			for _, branch := range updatedLocal {
				if err := fresh.protectBranchWorktrees(finalize, branch, false); err != nil {
					return err
				}
				if err := fresh.protectInteractiveBranch(finalize, branch); err != nil {
					return err
				}
			}
			if err := fresh.protectFetchRefPaths(finalize, updated, files); err != nil {
				return err
			}
			if verifyMedia != nil {
				if err := verifyMedia(finalize); err != nil {
					return err
				}
			}
			return ctx.Err()
		}); err != nil {
			return "", err
		}
	} else if verifyMedia != nil {
		if err := verifyMedia(ctx); err != nil {
			return "", err
		}
	}
	if err := s.publishFetchHead(); err != nil {
		return fmt.Sprintf("Fetched refs, but could not update FETCH_HEAD: %v", err), nil
	}
	return "", nil
}

func (r *Repository) protectFetchHeads(ctx context.Context, updated map[string]bool) error {
	if len(updated) == 0 {
		return nil
	}
	var paths []string
	for name := range updated {
		paths = append(paths, name)
	}
	err := r.visitWorktreeGitDirs(ctx, func(dir *os.Root) error {
		seen := make(map[string]bool)
		for branch := "HEAD"; ; {
			if seen[branch] || len(seen) >= 16 {
				return fmt.Errorf("a worktree has an invalid symbolic HEAD chain")
			}
			seen[branch] = true
			out, err := readGit(ctx, r.git, r.Path(), []string{"--git-dir=" + dir.Name()},
				"", 64<<10, "symbolic-ref", "--no-recurse", "-q", branch)
			if err != nil {
				if gitExitCode(err, 1) {
					return nil
				}
				return err
			}
			branch = strings.TrimSuffix(string(out), "\n")
			if updated[branch] {
				return fmt.Errorf("the operation would change a checked-out reference")
			}
			lower := strings.ToLower(branch)
			if fetchDestination(lower) || strings.HasPrefix(lower, "refs/heads/") {
				paths = append(paths, branch)
			}
		}
	})
	if err != nil {
		return err
	}
	common, err := os.OpenRoot(r.CommonDir)
	if err != nil {
		return err
	}
	defer common.Close()
	return checkCheckoutAliases(common, paths)
}

func (r *Repository) protectFetchRefPaths(ctx context.Context, updated map[string]bool, files bool) error {
	common, err := os.OpenRoot(r.CommonDir)
	if err != nil {
		return err
	}
	defer common.Close()
	for name := range updated {
		var symbolic bool
		if files {
			symbolic, err = fetchFileRefIsSymbolic(common, name)
		} else {
			var target string
			target, err = r.optionalRef(ctx, "symbolic-ref", "--no-recurse", "-q", name)
			symbolic = target != ""
		}
		if err != nil {
			return err
		}
		if symbolic {
			return fmt.Errorf("fetch destination %q is a symbolic reference", name)
		}
	}
	return nil
}

func fetchFileRefIsSymbolic(common *os.Root, ref string) (bool, error) {
	p := filepath.FromSlash(ref)
	parent, err := openWorktreeParent(common, p, false)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if parent != common {
		defer parent.Close()
	}
	name := filepath.Base(p)
	info, err := parent.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return err == nil, err
	}
	data, err := readOperationFile(parent, name)
	return strings.HasPrefix(data, "ref:"), err
}

func readFetchHead(dir string) ([]byte, error) {
	path := filepath.Join(dir, "FETCH_HEAD")
	before, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("unsupported FETCH_HEAD file")
	}
	file, err := os.OpenFile(path, os.O_RDONLY|workFileOpenFlags, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	const limit = 8 << 20
	if !os.SameFile(before, info) || info.Size() > limit {
		return nil, fmt.Errorf("unsupported FETCH_HEAD file")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if len(data) > limit {
		return nil, fmt.Errorf("FETCH_HEAD exceeds the size limit")
	}
	return data, nil
}

func (s *fetchStage) publishFetchHead() error {
	if s.fetchHeadReadErr != nil {
		return s.fetchHeadReadErr
	}
	head, err := readFetchHead(s.dir)
	if err != nil || head == nil {
		return err
	}
	root, err := os.OpenRoot(s.repo.GitDir)
	if err != nil {
		return err
	}
	defer root.Close()
	info, err := root.Stat(".")
	if err != nil || !os.SameFile(info, s.repo.gitDirInfo) {
		return fmt.Errorf("the Git directory changed before updating FETCH_HEAD")
	}
	const lock = "FETCH_HEAD.lock"
	file, err := root.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o666)
	if err != nil {
		return err
	}
	owned, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	ownsLock := func() bool {
		info, err := root.Lstat(lock)
		return err == nil && os.SameFile(info, owned)
	}
	defer func() {
		if ownsLock() {
			_ = root.Remove(lock)
		}
	}()
	_, writeErr := file.Write(head)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	current, err := readFetchHead(s.repo.GitDir)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, s.fetchHead) {
		return fmt.Errorf("FETCH_HEAD changed during fetch")
	}
	if !ownsLock() {
		return fmt.Errorf("FETCH_HEAD lock changed during fetch")
	}
	return root.Rename(lock, "FETCH_HEAD")
}
