package gitcli

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type WorktreeRemovePlan struct {
	repo      *Repository
	target    Worktree
	directory os.FileInfo
	admin     os.FileInfo
}

type WorktreeRemoveResult struct {
	Warning string
}

func (p WorktreeRemovePlan) MetadataOnly() bool { return p.directory == nil }

func (r *Repository) PrepareWorktreeRemoval(ctx context.Context, target Worktree) (WorktreeRemovePlan, error) {
	if target.Main || target.Current || target.ID == "" {
		return WorktreeRemovePlan{}, fmt.Errorf("switch to another worktree before removing this linked worktree")
	}
	if target.Locked {
		return WorktreeRemovePlan{}, fmt.Errorf("the linked worktree is locked")
	}
	if _, err := r.checkWorktree(ctx, target); err != nil {
		return WorktreeRemovePlan{}, err
	}
	if err := r.checkDetachedWorktreeRemoval(ctx, target); err != nil {
		return WorktreeRemovePlan{}, err
	}
	adminPath := filepath.Join(r.CommonDir, "worktrees", target.ID)
	admin, err := os.Lstat(adminPath)
	if err != nil || !admin.IsDir() {
		return WorktreeRemovePlan{}, fmt.Errorf("the linked worktree registration is missing")
	}
	plan := WorktreeRemovePlan{repo: r, target: target, admin: admin}
	if target.Prunable {
		// The path may have been reused. Treat a broken registration as
		// metadata only and leave whatever occupies that path alone.
		return plan, nil
	}
	linked, err := r.OpenWorktree(ctx, target)
	if err != nil {
		return WorktreeRemovePlan{}, err
	}
	if !os.SameFile(linked.gitDirInfo, admin) {
		return WorktreeRemovePlan{}, fmt.Errorf("the Git worktree registration was replaced")
	}
	info, err := os.Lstat(linked.WorkTree)
	if err != nil || !info.IsDir() {
		return WorktreeRemovePlan{}, fmt.Errorf("the linked worktree directory is missing")
	}
	if err := checkRemovableWorktree(ctx, linked, target); err != nil {
		return WorktreeRemovePlan{}, err
	}
	plan.directory = info
	return plan, nil
}

// Remove detaches the selected registration by renaming its admin directory.
// The working directory is first quarantined to make sure cleanup cannot
// remove a directory another process creates at the original path.
func (p WorktreeRemovePlan) Remove(ctx context.Context) (result WorktreeRemoveResult, err error) {
	if p.repo == nil {
		return result, fmt.Errorf("prepare the linked worktree removal first")
	}
	err = p.repo.withRefEdit(ctx, func() error {
		current, err := p.repo.PrepareWorktreeRemoval(ctx, p.target)
		if err != nil {
			return err
		}
		if !sameFileIdentity(p.admin, current.admin) || !sameFileIdentity(p.directory, current.directory) {
			return fmt.Errorf("the linked worktree changed since removal was requested")
		}
		result, err = p.detachWorktree(ctx)
		return err
	})
	return result, err
}

func (p WorktreeRemovePlan) detachWorktree(ctx context.Context) (result WorktreeRemoveResult, err error) {
	r, target := p.repo, p.target
	common, err := os.OpenRoot(r.CommonDir)
	if err != nil {
		return result, err
	}
	defer common.Close()
	registrations, err := common.OpenRoot("worktrees")
	if err != nil {
		return result, err
	}
	defer registrations.Close()
	admin, err := registrations.OpenRoot(target.ID)
	if err != nil {
		return result, err
	}
	defer admin.Close()
	info, err := admin.Stat(".")
	if err != nil || !sameFileIdentity(info, p.admin) {
		return result, fmt.Errorf("the linked worktree registration changed")
	}
	lock, err := claimWorktreeRemoval(admin)
	if err != nil {
		return result, err
	}
	defer func() {
		if current, err := admin.Lstat("locked"); err == nil && os.SameFile(current, lock) {
			_ = admin.Remove("locked")
		}
	}()
	linkData, _, err := readGitStorageFile(admin, "gitdir", 64<<10)
	if err != nil {
		return result, err
	}
	link := filepath.FromSlash(strings.TrimSuffix(string(linkData), "\n"))
	if !filepath.IsAbs(link) {
		link = filepath.Join(r.CommonDir, "worktrees", target.ID, link)
	}
	if filepath.Clean(link) != filepath.Join(target.Path, ".git") {
		return result, fmt.Errorf("the worktree link changed before removal")
	}
	var parent *os.Root
	var quarantine string
	cleanup := false
	if p.directory != nil {
		linked, err := OpenRoot(ctx, target.Path)
		if err != nil {
			return result, err
		}
		if !r.sameCommonWorktree(linked) || !os.SameFile(linked.gitDirInfo, p.admin) {
			return result, fmt.Errorf("the worktree was replaced before removal")
		}
		index, err := admin.OpenRoot(".")
		if err != nil {
			return result, err
		}
		indexGuard := checkoutIndexLock{root: index, release: func() {}}
		defer indexGuard.close()
		if err := indexGuard.openFile(); err != nil {
			return result, err
		}
		// Keep the lock marker while releasing its Windows file handle so
		// the registration directory can be renamed.
		if err := indexGuard.file.Close(); err != nil {
			return result, err
		}
		if err := checkRemovableWorktree(ctx, linked, target); err != nil {
			return result, err
		}
		parent, err = os.OpenRoot(filepath.Dir(target.Path))
		if err != nil {
			return result, err
		}
		defer parent.Close()
		name := filepath.Base(target.Path)
		if info, err := parent.Lstat(name); err != nil || !sameFileIdentity(info, p.directory) {
			return result, fmt.Errorf("the working directory changed before removal")
		}
		if err := ctx.Err(); err != nil {
			return result, err
		}
		quarantine = ".gitwave-removed-worktree-" + rand.Text()
		if err := moveDiscardFile(parent, name, parent, quarantine); err != nil {
			return result, err
		}
		defer func() {
			if !cleanup {
				archive := filepath.Join(parent.Name(), quarantine)
				// Moving it back never overwrites a newly created path.
				if restoreErr := moveDiscardFile(parent, quarantine, parent, name); restoreErr != nil {
					err = errors.Join(err, fmt.Errorf("the working directory remains at %q: %w", archive, restoreErr))
				}
			}
		}()
		moved, err := parent.Lstat(quarantine)
		if err != nil || !sameFileIdentity(moved, p.directory) {
			return result, fmt.Errorf("the working directory changed during removal")
		}
		away := *linked
		away.WorkTree = filepath.Join(parent.Name(), quarantine)
		if err := checkRemovableWorktree(ctx, &away, target); err != nil {
			return result, err
		}
		if !indexGuard.ownsLock() {
			return result, fmt.Errorf("the Git index lock changed during removal")
		}
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if current, err := admin.Lstat("locked"); err != nil || !os.SameFile(lock, current) {
		return result, fmt.Errorf("the worktree lock was replaced during removal")
	}
	if now, _, err := readGitStorageFile(admin, "gitdir", 64<<10); err != nil || string(now) != string(linkData) {
		return result, fmt.Errorf("the worktree link changed during removal")
	}
	if err := r.checkDetachedWorktreeRemoval(ctx, target); err != nil {
		return result, err
	}
	if p.directory == nil {
		if linked, err := OpenRoot(ctx, target.Path); err == nil && os.SameFile(linked.gitDirInfo, p.admin) {
			return result, fmt.Errorf("the broken worktree registration has been repaired")
		}
	}
	removed, err := quarantineWorktreeRegistration(common, registrations, target.ID, p.admin)
	if err != nil {
		return result, err
	}
	cleanup = true
	if err := common.RemoveAll(removed); err != nil {
		result.Warning = fmt.Sprintf("The registration remains at %s: %v", filepath.Join(r.CommonDir, removed), err)
	}
	if parent != nil {
		if info, err := parent.Lstat(quarantine); err != nil || !os.SameFile(info, p.directory) {
			result.Warning = joinCheckoutWarning(result.Warning, fmt.Sprintf("Working directory cleanup did not finish at %s.", filepath.Join(parent.Name(), quarantine)))
		} else if err := parent.RemoveAll(quarantine); err != nil {
			result.Warning = joinCheckoutWarning(result.Warning, fmt.Sprintf("The working directory remains at %s: %v", filepath.Join(parent.Name(), quarantine), err))
		}
	}
	return result, nil
}

func quarantineWorktreeRegistration(common, registrations *os.Root, id string, expected os.FileInfo) (string, error) {
	current, err := registrations.Lstat(id)
	if err != nil || !os.SameFile(current, expected) {
		return "", fmt.Errorf("the worktree registration was replaced")
	}
	removed := ".gitwave-removed-registration-" + rand.Text()
	if err := moveDiscardFile(registrations, id, common, removed); err != nil {
		return "", err
	}
	if info, err := common.Lstat(removed); err != nil || !os.SameFile(info, expected) {
		changed := fmt.Errorf("the worktree registration was replaced during removal")
		if err := moveDiscardFile(common, removed, registrations, id); err != nil {
			return "", errors.Join(changed, fmt.Errorf("the replacement registration remains at %q: %w",
				filepath.Join(common.Name(), removed), err))
		}
		return "", changed
	}
	return removed, nil
}

func (r *Repository) checkDetachedWorktreeRemoval(ctx context.Context, target Worktree) error {
	if target.Branch != "" || strings.Trim(target.Head, "0") == "" {
		return nil
	}
	refs, err := r.readLimited(ctx, "", 64<<10, "for-each-ref", "--count=1", "--contains="+target.Head,
		"--format=%(refname)", "refs/heads", "refs/tags", "refs/remotes")
	if err != nil {
		return err
	}
	if len(refs) == 0 {
		return fmt.Errorf("create a branch or tag for the detached worktree commit before removing it")
	}
	return nil
}

func claimWorktreeRemoval(admin *os.Root) (os.FileInfo, error) {
	file, err := admin.OpenFile("locked", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if errors.Is(err, fs.ErrExist) {
		return nil, fmt.Errorf("the linked worktree is locked")
	}
	if err != nil {
		return nil, err
	}
	info, statErr := file.Stat()
	_, writeErr := file.WriteString("GitWave removing worktree\n")
	err = errors.Join(statErr, writeErr, file.Close())
	if err != nil {
		if current, statErr := admin.Lstat("locked"); statErr == nil && sameFileIdentity(info, current) {
			_ = admin.Remove("locked")
		}
		return nil, err
	}
	return info, nil
}

func checkRemovableWorktree(ctx context.Context, linked *Repository, expected Worktree) error {
	ref := ""
	if expected.Branch != "" {
		ref = "refs/heads/" + expected.Branch
	}
	head := expected.Head
	if strings.Trim(head, "0") == "" {
		head = ""
	}
	if !linked.headMatches(ctx, ref, head) {
		return fmt.Errorf("the linked worktree HEAD changed")
	}
	if err := linked.checkCheckoutOperation(ctx); err != nil {
		return err
	}
	git, err := linked.workingGit(ctx, head)
	if err != nil {
		return err
	}
	defer git.close()
	// Even a clean populated submodule may be a repository in its own
	// right. Git refuses deleting these worktrees without a force flag.
	index, err := git.read(ctx, 64<<20, "ls-files", "--stage", "-v", "-z", "--")
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(linked.WorkTree)
	if err != nil {
		return err
	}
	defer root.Close()
	var entries, sparse strings.Builder
	for entry := range strings.SplitSeq(string(index), "\x00") {
		if entry == "" {
			continue
		}
		stage, name, ok := strings.Cut(entry, "\t")
		if !ok || len(stage) < 9 {
			return fmt.Errorf("Git returned an invalid index record")
		}
		local := filepath.FromSlash(name)
		flag := entry[0]
		if flag == 'S' || flag == 's' {
			if _, err := root.Lstat(local); errors.Is(err, fs.ErrNotExist) {
				sparse.WriteString(name)
				sparse.WriteByte(0)
			} else if err != nil {
				return err
			}
		}
		if strings.HasPrefix(stage[2:], "160000 ") {
			if err := checkRemovableGitlink(root, local); err != nil {
				return err
			}
		}
		entries.WriteString(entry[2:])
		entries.WriteByte(0)
	}
	if entries.Len() > 0 {
		// The real index can hide file changes through cached stat data and
		// assume-unchanged. Recreate entries without either before removal.
		git.index = filepath.Join(git.dir, "worktree-removal-index")
		if _, err := git.run(ctx, entries.String(), 64<<10, "update-index", "-z", "--index-info"); err != nil {
			return err
		}
		if sparse.Len() > 0 {
			if _, err := git.run(ctx, sparse.String(), 64<<10, "update-index", "--skip-worktree", "-z", "--stdin"); err != nil {
				return err
			}
		}
	}
	out, err := git.read(ctx, 32<<20, "status", "--porcelain=v2", "-z", "--untracked-files=all", "--ignored=matching",
		"--ignore-submodules=none", "--no-renames")
	if err != nil {
		return err
	}
	if len(out) != 0 {
		return fmt.Errorf("the linked worktree contains local changes, untracked files or ignored files")
	}
	// A nested repository can be invisible to Git even when its working
	// files are tracked here. Its own objects and refs belong to the user.
	return fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		if !strings.EqualFold(name, ".git") {
			return nil
		}
		if path == ".git" && !entry.IsDir() && entry.Type().IsRegular() {
			return nil
		}
		return fmt.Errorf("the linked worktree contains separate Git metadata at %q", path)
	})
}

func checkRemovableGitlink(root *os.Root, path string) error {
	info, err := root.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	changed := fmt.Errorf("the linked worktree contains local data at submodule path %q", path)
	if !info.IsDir() {
		return changed
	}
	dir, err := root.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	opened, err := dir.Stat()
	if err != nil || !sameFileIdentity(info, opened) {
		return changed
	}
	if _, err := dir.Readdirnames(1); !errors.Is(err, io.EOF) {
		return changed
	}
	return nil
}
