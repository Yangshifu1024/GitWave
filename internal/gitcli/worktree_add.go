package gitcli

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type WorktreeAddRequest struct {
	Name        string
	Destination string
	Start       CheckoutTarget
}

type WorktreeAddResult struct {
	Path    string
	Warning string
}

func (r *Repository) AddWorktree(ctx context.Context, request WorktreeAddRequest) (result WorktreeAddResult, err error) {
	target, err := PrepareClone(ctx, request.Destination)
	if err != nil {
		return result, err
	}
	if target.nonempty {
		return result, fmt.Errorf("select an empty location for the new worktree")
	}
	if err := r.requireBranchCheckoutGit(ctx); err != nil {
		return result, err
	}
	err = r.withRefEdit(ctx, func() error {
		name, err := r.checkedRefName(ctx, "heads", request.Name)
		if err != nil {
			return err
		}
		if err := r.requireCommit(ctx, request.Start.OID); err != nil {
			return err
		}
		if request.Start.Ref != "" && !strings.HasPrefix(request.Start.Ref, "refs/heads/") {
			return fmt.Errorf("select a local branch or HEAD as the starting point")
		}
		if start := request.Start.Ref; start != "" {
			oid, err := r.exactRootRef(ctx, start)
			if err != nil || oid != request.Start.OID {
				return fmt.Errorf("the starting branch changed since the worktree was requested")
			}
		}
		if existing, err := r.exactRootRef(ctx, name); err != nil || existing != "" {
			if err != nil {
				return err
			}
			return fmt.Errorf("branch %q already exists", request.Name)
		}
		result, err = r.stageWorktree(ctx, target, request, name)
		return err
	})
	return result, err
}

func (r *Repository) stageWorktree(ctx context.Context, target CloneTarget, request WorktreeAddRequest, branch string) (result WorktreeAddResult, err error) {
	root, err := os.OpenRoot(target.ancestor)
	if err != nil {
		return result, err
	}
	defer root.Close()
	ancestor, err := root.Stat(".")
	if err != nil || !sameFileIdentity(ancestor, target.ancestorInfo) {
		return result, fmt.Errorf("the worktree destination changed")
	}
	suffix := rand.Text()
	stage := ".gitwave-worktree-" + suffix
	if err := root.Mkdir(stage, 0o700); err != nil {
		return result, err
	}
	stageRoot, err := root.OpenRoot(stage)
	if err != nil {
		return result, err
	}
	defer stageRoot.Close()
	stageInfo, err := stageRoot.Stat(".")
	if err != nil {
		return result, err
	}
	base := filepath.Base(target.path)
	stagePath := filepath.Join(target.ancestor, stage, base)
	reason := provisionalWorktreeLock + suffix
	cleanupPath := stagePath
	preserveStage := false
	defer func() {
		if preserveStage {
			err = errors.Join(err, fmt.Errorf("provisional files are kept at %q", stagePath))
			return
		}
		current, statErr := root.Lstat(stage)
		if statErr != nil || !sameFileIdentity(current, stageInfo) {
			if cleanupPath != "" {
				err = errors.Join(err, fmt.Errorf("the staging directory at %q changed during worktree creation", stagePath))
			}
			return
		}
		cleanCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		if cleanupPath != "" {
			cleanupErr := r.cleanStagedWorktree(cleanCtx, cleanupPath, reason)
			if cleanupErr != nil {
				err = errors.Join(err, cleanupErr)
				return
			}
		}
		if now, infoErr := root.Lstat(stage); infoErr == nil && sameFileIdentity(now, stageInfo) {
			if removeErr := root.RemoveAll(stage); removeErr != nil {
				message := fmt.Sprintf("The staging directory remains at %s: %v", filepath.Dir(stagePath), removeErr)
				if err == nil {
					result.Warning = joinCheckoutWarning(result.Warning, message)
				} else {
					err = errors.Join(err, fmt.Errorf("%s", message))
				}
			}
		}
	}()
	var checkPermissions func() error
	if target.dir != nil {
		checkPermissions, err = prepareExistingWorktreeDirectory(ctx, target, stageRoot, base)
		if err != nil {
			return result, fmt.Errorf("preserve the destination directory permissions: %w", err)
		}
		if err := checkPermissions(); err != nil {
			return result, err
		}
	}
	if err := r.runWorktreeCommand(ctx, "add", "--quiet", "--detach", "--no-checkout", "--lock", "--reason", reason,
		"--", stagePath, request.Start.OID); err != nil {
		return result, err
	}
	staged, err := OpenRoot(ctx, stagePath)
	if err != nil {
		return result, err
	}
	if !r.sameCommonWorktree(staged) {
		return result, fmt.Errorf("the new worktree is attached to a different repository")
	}
	stagedInfo, err := stageRoot.Lstat(base)
	if err != nil || !stagedInfo.IsDir() {
		return result, fmt.Errorf("the provisional worktree directory changed")
	}
	if err := prepareNewWorktree(ctx, staged, request.Start.OID); err != nil {
		return result, err
	}
	admin, err := os.OpenRoot(staged.GitDir)
	if err != nil {
		return result, err
	}
	defer admin.Close()
	if err := admin.WriteFile(worktreeNameFile, []byte(request.Name+"\n"), 0o666); err != nil {
		return result, err
	}
	originalLink, linkMode, err := readGitStorageFile(admin, "gitdir", 64<<10)
	if err != nil {
		return result, err
	}
	if !filepath.IsAbs(strings.TrimSuffix(string(originalLink), "\n")) {
		return result, fmt.Errorf("the provisional worktree has a relative Git link")
	}
	link, err := lockGitStorageFile(admin, "gitdir")
	if err != nil {
		return result, err
	}
	defer link.close()
	if err := link.prepare([]byte(filepath.ToSlash(filepath.Join(target.path, ".git"))+"\n"), linkMode); err != nil {
		return result, err
	}

	finalize, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	hooks := filepath.Join(target.ancestor, stage, "empty-hooks")
	if err := stageRoot.Mkdir("empty-hooks", 0o700); err != nil {
		return result, err
	}
	note := "worktree: add " + request.Name
	cmd := gitCommand(finalize, staged.git, target.ancestor, []string{"--git-dir=" + staged.GitDir}, "", "-c", "core.hooksPath="+hooks,
		"update-ref", "--stdin", "-z", "--create-reflog", "-m", note)
	statement := "symref-update HEAD\x00" + branch + "\x00oid\x00" + request.Start.OID + "\x00" +
		"create " + branch + "\x00" + request.Start.OID + "\x00"
	if request.Start.Ref != "" {
		statement += verifyCheckoutRef(request.Start.Ref, request.Start.OID)
	}
	var moved, linked bool
	var backup string
	published, sent, txErr := runReferenceTransaction(cmd, statement, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !staged.headMatches(ctx, "", request.Start.OID) {
			return fmt.Errorf("the provisional worktree HEAD changed")
		}
		if start := request.Start.Ref; start != "" {
			symbolic, err := r.optionalRef(ctx, "symbolic-ref", "--no-recurse", "-q", start)
			if err != nil || symbolic != "" {
				return fmt.Errorf("the starting branch changed since the worktree was requested")
			}
		}
		if now, err := stageRoot.Lstat(base); err != nil || !sameFileIdentity(now, stagedInfo) {
			return fmt.Errorf("the provisional worktree directory changed")
		}
		if !link.owns() {
			return fmt.Errorf("the worktree Git link was locked by another operation")
		}
		lockNote, err := readOperationFile(admin, "locked")
		if err != nil || strings.TrimSpace(lockNote) != reason {
			return fmt.Errorf("the provisional worktree lock changed")
		}
		if checkPermissions != nil {
			if err := checkPermissions(); err != nil {
				return err
			}
		}
		backup, err = target.publishDirectory(root, stageRoot, base, true)
		if err != nil {
			return err
		}
		moved = true
		if err := link.publish(); err != nil {
			return err
		}
		linked = true
		return nil
	})
	if !published && sent && linked {
		if actual, err := OpenRoot(finalize, target.path); err == nil && os.SameFile(staged.gitDirInfo, actual.gitDirInfo) {
			published = actual.headMatches(finalize, branch, request.Start.OID)
		}
	}
	if !published {
		if moved {
			if err := restoreStagedWorktree(finalize, root, target, stageRoot, base, staged, stagedInfo,
				request.Start.OID, admin, originalLink, linkMode, linked, backup); err != nil {
				preserveStage = true
				result.Path = target.path
				txErr = errors.Join(txErr, fmt.Errorf("provisional worktree could not be restored to its staging area: %w", err))
			} else {
				backup = ""
			}
		}
		if backup != "" {
			txErr = errors.Join(txErr, fmt.Errorf("the original destination directory remains at %q", backup))
		}
		return result, txErr
	}
	cleanupPath = ""
	result.Path = target.path
	if backup != "" {
		if err := target.releaseOriginalWorktreeDirectory(root, backup); err != nil {
			result.Warning = fmt.Sprintf("The original directory is kept at %s.", backup)
		}
	}
	if lockNote, err := readOperationFile(admin, "locked"); err != nil || strings.TrimSpace(lockNote) != reason {
		result.Warning = joinCheckoutWarning(result.Warning, "The worktree lock changed during creation.")
	} else if err := r.runWorktreeCommand(finalize, "unlock", "--", target.path); err != nil {
		result.Warning = joinCheckoutWarning(result.Warning, fmt.Sprintf("The new worktree is still locked: %v", err))
	}
	return result, nil
}

func prepareNewWorktree(ctx context.Context, repo *Repository, oid string) error {
	git, err := repo.workingGit(ctx, oid)
	if err != nil {
		return err
	}
	defer git.close()
	paths, err := git.checkoutPaths(ctx, "", oid)
	if err != nil {
		return err
	}
	if err := git.prepareCheckoutConfig(ctx, "", oid, paths); err != nil {
		return err
	}
	root, err := os.OpenRoot(repo.WorkTree)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := checkCheckoutAliases(root, paths); err != nil {
		return err
	}
	if err := git.runCheckoutCommand(ctx, "", "read-tree", "--reset", "-u", oid); err != nil {
		return err
	}
	status, err := git.checkoutStatus(ctx)
	if err != nil {
		return err
	}
	if status.HeadOID != oid || len(status.Changes) != 0 {
		return fmt.Errorf("Git did not materialize all paths in the new worktree")
	}
	return nil
}

func restoreStagedWorktree(ctx context.Context, root *os.Root, target CloneTarget, stage *os.Root, base string,
	repo *Repository, stagedInfo os.FileInfo, oid string, admin *os.Root, linkData []byte,
	mode os.FileMode, linked bool, backup string) error {
	local, err := filepath.Rel(target.ancestor, target.path)
	if err != nil {
		return err
	}
	parent, err := openWorktreeParent(root, local, false)
	if err != nil {
		return err
	}
	if parent != root {
		defer parent.Close()
	}
	final := filepath.Base(local)
	if now, err := parent.Lstat(final); err != nil || !sameFileIdentity(now, stagedInfo) {
		return fmt.Errorf("the path at %q has been replaced", target.path)
	}
	published := *repo
	published.WorkTree = target.path
	if err := checkRemovableWorktree(ctx, &published, Worktree{Head: oid}); err != nil {
		return fmt.Errorf("the provisional worktree remains at %q: %w", target.path, err)
	}
	if err := moveDiscardFile(parent, final, stage, base); err != nil {
		return err
	}
	if backup != "" {
		backupName := filepath.Base(backup)
		if info, err := parent.Lstat(backupName); err != nil || !sameFileIdentity(info, target.dir) {
			return fmt.Errorf("the original destination directory changed")
		}
		if err := moveDiscardFile(parent, backupName, parent, final); err != nil {
			return err
		}
	}
	if !linked {
		return nil
	}
	lock, err := lockGitStorageFile(admin, "gitdir")
	if err != nil {
		return err
	}
	defer lock.close()
	if err := lock.prepare(linkData, mode); err != nil {
		return err
	}
	return lock.publish()
}

func (t CloneTarget) releaseOriginalWorktreeDirectory(root *os.Root, backup string) error {
	local, err := filepath.Rel(t.ancestor, backup)
	if err != nil {
		return err
	}
	parent, err := openWorktreeParent(root, local, false)
	if err != nil {
		return err
	}
	if parent != root {
		defer parent.Close()
	}
	name := filepath.Base(local)
	if info, err := parent.Lstat(name); err != nil || !sameFileIdentity(info, t.dir) {
		return fmt.Errorf("the original destination directory changed")
	}
	return parent.Remove(name)
}

func (r *Repository) cleanStagedWorktree(ctx context.Context, path, reason string) error {
	known, err := r.Worktrees(ctx)
	if err != nil {
		return fmt.Errorf("could not check the provisional worktree at %q: %w", path, err)
	}
	for _, entry := range known {
		if entry.Path != path || entry.Main || entry.ID == "" || !entry.Locked {
			continue
		}
		common, err := os.OpenRoot(r.CommonDir)
		if err != nil {
			return err
		}
		defer common.Close()
		registrations, err := common.OpenRoot("worktrees")
		if err != nil {
			return err
		}
		defer registrations.Close()
		admin, err := registrations.OpenRoot(entry.ID)
		if err != nil {
			return err
		}
		defer admin.Close()
		note, err := readOperationFile(admin, "locked")
		if err != nil || strings.TrimSpace(note) != reason {
			return fmt.Errorf("provisional worktree at %q has a different lock", path)
		}
		link, _, err := readGitStorageFile(admin, "gitdir", 64<<10)
		if err != nil || filepath.Clean(strings.TrimSuffix(string(link), "\n")) != filepath.Join(path, ".git") {
			return fmt.Errorf("provisional worktree at %q has a different location", path)
		}
		info, err := admin.Stat(".")
		if err != nil {
			return err
		}
		// Remove the registration independently. Deletion of staged files
		// stays with the caller that opened and checked the private directory.
		quarantine, err := quarantineWorktreeRegistration(common, registrations, entry.ID, info)
		if err != nil {
			return err
		}
		return common.RemoveAll(quarantine)
	}
	return nil
}

func (r *Repository) runWorktreeCommand(ctx context.Context, args ...string) error {
	return r.writeRefCommand(ctx, append([]string{"-c", "worktree.useRelativePaths=false", "worktree"}, args...)...)
}
