package gitcli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func (r *Repository) CreateBranch(ctx context.Context, name, base string) error {
	return r.withRefEdit(ctx, func() error {
		if _, err := r.checkedRefName(ctx, "heads", name); err != nil {
			return err
		}
		if err := r.requireCommit(ctx, base); err != nil {
			return err
		}
		return r.writeRefCommand(ctx, "branch", "--no-track", "--", name, base)
	})
}

func (r *Repository) RenameBranch(ctx context.Context, name, next, expected string) error {
	return r.editExistingBranch(ctx, name, expected, func(full string, _ References) error {
		if _, err := r.checkedRefName(ctx, "heads", next); err != nil {
			return err
		}
		if name == next {
			return nil
		}
		if err := r.protectBranchWorktrees(ctx, full, true); err != nil {
			return err
		}
		if err := r.protectInteractiveBranch(ctx, full); err != nil {
			return err
		}
		if _, err := os.Lstat(filepath.Join(r.CommonDir, "config.lock")); err == nil {
			return fmt.Errorf("the Git configuration is locked by another operation")
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		// Git also moves branch configuration and updates the attached HEAD.
		return r.writeRefCommand(ctx, "branch", "--move", "--", name, next)
	})
}

func (r *Repository) DeleteBranch(ctx context.Context, name, expected string) error {
	return r.editExistingBranch(ctx, name, expected, func(full string, _ References) error {
		if err := r.protectBranchWorktrees(ctx, full, false); err != nil {
			return err
		}
		if err := r.protectInteractiveBranch(ctx, full); err != nil {
			return err
		}
		unlock, err := r.lockBranchWorktreeIndices(ctx, nil)
		if err != nil {
			return err
		}
		defer unlock()
		return r.editDirectRef(ctx, full, expected, "", "branch: delete "+name, func() error {
			return r.protectBranchWorktrees(ctx, full, false)
		})
	})
}

func (r *Repository) protectInteractiveBranch(ctx context.Context, full string) error {
	return r.visitWorktreeGitDirs(ctx, func(dir *os.Root) error {
		worktree := *r
		worktree.GitDir = dir.Name()
		pause, err := worktree.interactiveRebaseRef(ctx)
		if err != nil {
			return err
		}
		branch := ""
		if pause != "" {
			state, err := worktree.loadInteractivePause(ctx, pause)
			if err != nil {
				return err
			}
			branch = state.Branch
		} else if _, err := dir.Lstat(legacyInteractiveRebaseFile); err == nil {
			head, err := readOperationFile(dir, "HEAD")
			if err != nil {
				return err
			}
			branch, _ = strings.CutPrefix(strings.TrimSpace(head), "ref: ")
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if r.sameBranchPath(branch, full) {
			return fmt.Errorf("branch %q is in use by a paused interactive rebase", strings.TrimPrefix(full, "refs/heads/"))
		}
		return nil
	})
}

func (r *Repository) SetBranchUpstream(ctx context.Context, name, expected, oldUpstream, upstream string) error {
	return r.editExistingBranch(ctx, name, expected, func(full string, refs References) error {
		if findRef(refs, full).Upstream != oldUpstream {
			return fmt.Errorf("the upstream changed since it was shown")
		}
		if upstream == oldUpstream {
			return nil
		}
		if upstream == "" {
			return r.writeRefCommand(ctx, "branch", "--unset-upstream", "--", name)
		}
		ref := findRef(refs, upstream)
		if !strings.HasPrefix(upstream, "refs/remotes/") || ref.OID == "" || ref.Symbolic != "" {
			return fmt.Errorf("the selected upstream branch is no longer available")
		}
		return r.writeRefCommand(ctx, "branch", "--set-upstream-to="+upstream, "--", name)
	})
}

func (r *Repository) editExistingBranch(ctx context.Context, name, expected string, edit func(string, References) error) error {
	if !validOID(expected) {
		return fmt.Errorf("select a local branch with a commit")
	}
	return r.withRefEdit(ctx, func() error {
		full, err := r.checkedRefName(ctx, "heads", name)
		if err != nil {
			return err
		}
		refs, err := r.References(ctx)
		if err != nil {
			return err
		}
		if err := checkRefTip(refs, full, expected); err != nil {
			return err
		}
		return edit(full, refs)
	})
}

func (r *Repository) protectBranchWorktrees(ctx context.Context, full string, allowOwn bool) error {
	out, err := r.readLimited(ctx, "", 4<<20, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return err
	}
	var path string
	for item := range strings.SplitSeq(string(out), "\x00") {
		field, value, _ := strings.Cut(item, " ")
		switch field {
		case "worktree":
			path = value
		case "branch":
			if r.sameBranchPath(value, full) && !(value == full && allowOwn && isOwnWorktree(path, r.WorkTree)) {
				return fmt.Errorf("branch %q is checked out in a worktree: %s", strings.TrimPrefix(full, "refs/heads/"), path)
			}
		}
	}
	return r.visitWorktreeGitDirs(ctx, func(dir *os.Root) error { return r.checkPausedWorktree(dir, full) })
}

func (r *Repository) sameBranchPath(actual, expected string) bool {
	if actual == expected {
		return true
	}
	if !strings.HasPrefix(actual, "refs/heads/") || !validIndexPath(actual) {
		return false
	}
	from, err := os.Stat(filepath.Join(r.CommonDir, filepath.FromSlash(actual)))
	to, toErr := os.Stat(filepath.Join(r.CommonDir, filepath.FromSlash(expected)))
	return err == nil && toErr == nil && os.SameFile(from, to)
}

func (r *Repository) lockBranchWorktreeIndices(ctx context.Context, alreadyLocked os.FileInfo) (func(), error) {
	var locks []*checkoutIndexLock
	release := func() {
		for _, lock := range locks {
			lock.close()
		}
	}
	err := r.visitWorktreeGitDirs(ctx, func(dir *os.Root) error {
		if alreadyLocked != nil {
			info, err := dir.Stat(".")
			if err != nil {
				return err
			}
			if os.SameFile(info, alreadyLocked) {
				return nil
			}
		}
		root, err := dir.OpenRoot(".")
		if err != nil {
			return err
		}
		// The common-directory Go lock is already held by withRefEdit.
		lock := &checkoutIndexLock{root: root, release: func() {}}
		locks = append(locks, lock)
		return lock.openFile()
	})
	if err != nil {
		release()
		return nil, err
	}
	return release, nil
}

func (r *Repository) visitWorktreeGitDirs(ctx context.Context, visit func(*os.Root) error) error {
	common, err := os.OpenRoot(r.CommonDir)
	if err != nil {
		return err
	}
	defer common.Close()
	if err := visit(common); err != nil {
		return err
	}
	entries, err := fs.ReadDir(common.FS(), "worktrees")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.IsDir() {
			continue
		}
		dir, err := common.OpenRoot(filepath.Join("worktrees", entry.Name()))
		if err != nil {
			return err
		}
		err = visit(dir)
		dir.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) checkPausedWorktree(dir *os.Root, full string) error {
	for _, name := range []string{"rebase-merge/head-name", "rebase-apply/head-name", "rebase-merge/update-refs", "BISECT_START"} {
		data, err := readOperationFile(dir, name)
		if err != nil {
			return err
		}
		for line := range strings.SplitSeq(strings.TrimRight(data, "\r\n"), "\n") {
			if r.sameBranchPath(strings.TrimSuffix(line, "\r"), full) ||
				(name == "BISECT_START" && r.sameBranchPath("refs/heads/"+line, full)) {
				return fmt.Errorf("branch %q is in use by a paused Git operation at %q", strings.TrimPrefix(full, "refs/heads/"), dir.Name())
			}
		}
	}
	return nil
}

func isOwnWorktree(path, worktree string) bool {
	if path == worktree && worktree != "" {
		return true
	}
	if path == "" || worktree == "" {
		return false
	}
	actual, err := os.Stat(path)
	own, ownErr := os.Stat(worktree)
	return err == nil && ownErr == nil && os.SameFile(actual, own)
}
