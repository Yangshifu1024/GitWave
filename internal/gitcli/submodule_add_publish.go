package gitcli

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type submoduleRecoveryError struct {
	directory string
	cause     error
}

func (e *submoduleRecoveryError) Error() string {
	return fmt.Sprintf("the submodule checkout was preserved at %q: %v", e.directory, e.cause)
}

func (e *submoduleRecoveryError) Unwrap() error { return e.cause }

func (p *SubmoduleAddPlan) publish(ctx context.Context, root, private *os.Root, child *Repository, checkPermissions func() error) error {
	r := p.repo
	index, err := r.lockCheckoutIndex(ctx)
	if err != nil {
		return err
	}
	defer index.close()
	if err := r.checkCheckoutOperation(ctx); err != nil {
		return err
	}
	common, err := os.OpenRoot(r.CommonDir)
	if err != nil {
		return err
	}
	defer common.Close()
	modulesLock, err := lockGitStorageFile(root, ".gitmodules")
	if err != nil {
		return err
	}
	defer modulesLock.close()
	configLock, err := lockGitStorageFile(common, "config")
	if err != nil {
		return err
	}
	defer configLock.close()
	if err := p.verify(ctx, root); err != nil {
		return err
	}
	conflicts, err := r.readLimited(ctx, "", 64<<10, "ls-files", "--unmerged", "-z", "--", ".gitmodules")
	if err != nil || len(conflicts) != 0 {
		return fmt.Errorf("resolve .gitmodules conflicts before adding a submodule")
	}
	original, err := readGitStorageSnapshot(common, "config", remoteConfigLimit)
	if err != nil {
		return err
	}
	nextModules, err := p.draft(ctx, private, ".gitmodules", p.modules,
		"submodule."+p.path+".path", p.path, "submodule."+p.path+".url", p.url)
	if err != nil {
		return err
	}
	nextConfig, err := p.draft(ctx, private, "config", original,
		"submodule."+p.path+".url", p.resolvedURL, "submodule."+p.path+".active", "true")
	if err != nil {
		return err
	}
	tip, err := child.optionalRef(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil || !validOID(tip) {
		return fmt.Errorf("the submodule checkout has no commit")
	}
	git, err := p.stageIndex(ctx, root, index.root, nextModules, tip)
	if err != nil {
		return err
	}
	defer git.close()
	if err := writeLockedIndex(index.root, index.file, git.index); err != nil {
		return err
	}
	if err := configLock.prepare(nextConfig, original.mode); err != nil {
		return err
	}
	mode := p.modules.mode
	if mode == 0 {
		mode = 0o644
	}
	if err := modulesLock.prepare(nextModules, mode); err != nil {
		return err
	}
	if err := r.verifyGitStorageLocks(ctx, common, configLock); err != nil {
		return err
	}
	if current, err := readGitStorageSnapshot(common, "config", remoteConfigLimit); err != nil || !original.same(current) {
		return fmt.Errorf("the Git configuration changed before adding the submodule")
	}
	if !index.ownsLock() || !modulesLock.owns() {
		return fmt.Errorf("a Git lock was replaced before adding the submodule")
	}
	if err := p.verify(ctx, root); err != nil {
		return err
	}

	var backup string
	if !p.target.nonempty {
		if checkPermissions != nil {
			if err := checkPermissions(); err != nil {
				return err
			}
		}
		ancestor, err := os.OpenRoot(p.target.ancestor)
		if err != nil {
			return err
		}
		backup, err = p.target.publishDirectory(ancestor, private, "checkout", true)
		ancestor.Close()
		if err != nil {
			return err
		}
	}
	configPublished, modulesPublished := false, false
	rollback := func(cause error) error {
		if modulesPublished {
			cause = errors.Join(cause, restoreGitStorageFile(root, ".gitmodules", p.modules, nextModules, gitmodulesLimit))
		}
		if configPublished {
			cause = errors.Join(cause, restoreGitStorageFile(common, "config", original, nextConfig, remoteConfigLimit))
		}
		if !p.target.nonempty {
			cause = errors.Join(cause, p.rollbackCheckout(context.WithoutCancel(ctx), root, private, index.root, child, backup))
		}
		return cause
	}
	if err := configLock.publish(); err != nil {
		return rollback(err)
	}
	configPublished = true
	if err := modulesLock.publish(); err != nil {
		return rollback(err)
	}
	modulesPublished = true
	if err := installLockedIndex(index.root, index.info); err != nil {
		return rollback(err)
	}
	if backup != "" {
		// Only remove the empty directory we confirmed and replaced.
		if original, err := os.Lstat(backup); err == nil && sameFileIdentity(original, p.target.dir) {
			_ = os.Remove(backup)
		}
	}
	return nil
}

func (p *SubmoduleAddPlan) draft(ctx context.Context, private *os.Root, name string, file gitStorageSnapshot, fields ...string) ([]byte, error) {
	if err := private.WriteFile(name, file.data, 0o600); err != nil {
		return nil, err
	}
	for i := 0; i < len(fields); i += 2 {
		if _, err := p.repo.readLimited(ctx, "", 64<<10, "config", "--file", filepath.Join(private.Name(), name),
			"--add", fields[i], fields[i+1]); err != nil {
			return nil, err
		}
	}
	return private.ReadFile(name)
}

func (p *SubmoduleAddPlan) stageIndex(ctx context.Context, working, index *os.Root, modules []byte, tip string) (*workingGit, error) {
	git, err := p.repo.workingGit(ctx, p.head)
	if err != nil {
		return nil, err
	}
	if err := p.writeSubmoduleIndex(ctx, git, working, index, modules, tip); err != nil {
		git.close()
		return nil, err
	}
	return git, nil
}

func (p *SubmoduleAddPlan) writeSubmoduleIndex(ctx context.Context, git *workingGit, working, index *os.Root, modules []byte, tip string) error {
	if len(tip) != git.oidSize {
		return fmt.Errorf("the submodule uses a different Git object format")
	}
	git.index = filepath.Join(git.dir, "index")
	git.settings = append(git.settings, "-c", "core.splitIndex=false")
	if err := copyOriginalIndex(index, git.index); err != nil {
		return err
	}
	affected := []string{".gitmodules", p.path}
	before, err := git.otherIndexEntries(ctx, affected)
	if err != nil {
		return err
	}
	if err := git.isolateWorkTree(); err != nil {
		return err
	}
	copyRoot, err := os.OpenRoot(git.workTree)
	if err != nil {
		return err
	}
	defer copyRoot.Close()
	modulePath := []string{".gitmodules"}
	if err := git.snapshotWorktreeMetadata(ctx, working, copyRoot, modulePath, nil, ".gitattributes"); err != nil {
		return err
	}
	if err := git.freezeWorktreeAttributes(ctx); err != nil {
		return err
	}
	if err := git.checkWorktreeFilters(ctx, modulePath, "staging", false); err != nil {
		return err
	}
	git.settings = append(git.settings, "-c", "filter.set.required=true", "-c", "filter.unset.required=true",
		"-c", "filter.unspecified.required=true")
	mode := "100644"
	if p.modules.mode&0o100 != 0 {
		mode = "100755"
	}
	staged, err := git.read(ctx, 512, "ls-files", "--stage", "-z", "--", ".gitmodules")
	if err != nil {
		return err
	}
	if len(staged) != 0 {
		meta, name, ok := strings.Cut(string(staged), "\t")
		fields := strings.Fields(meta)
		if !ok || name != ".gitmodules\x00" || len(fields) != 3 || fields[2] != "0" ||
			(fields[0] != "100644" && fields[0] != "100755") {
			return fmt.Errorf("the staged .gitmodules entry is not a regular file")
		}
		work, err := git.run(ctx, string(p.modules.data), 256, "hash-object", "--path=.gitmodules", "--stdin")
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(work)) != fields[1] {
			return fmt.Errorf("stage or discard the existing .gitmodules edits before adding a submodule")
		}
		mode = fields[0]
	}
	out, err := git.run(ctx, string(modules), 256, "hash-object", "-w", "--path=.gitmodules", "--stdin")
	if err != nil {
		return err
	}
	blob := strings.TrimSpace(string(out))
	if len(blob) != git.oidSize || !validOID(blob) {
		return fmt.Errorf("invalid staged .gitmodules object")
	}
	input := mode + " " + blob + "\t.gitmodules\x00" + "160000 " + tip + "\t" + p.path + "\x00"
	if _, err := git.run(ctx, input, 64<<10, "update-index", "-z", "--index-info"); err != nil {
		return err
	}
	after, err := git.otherIndexEntries(ctx, affected)
	if err != nil {
		return err
	}
	if !bytes.Equal(before, after) {
		return fmt.Errorf("adding the submodule would change unrelated index entries")
	}
	return nil
}

func (p *SubmoduleAddPlan) rollbackCheckout(ctx context.Context, root, private, gitDir *os.Root, staged *Repository, backup string) error {
	child, err := p.repo.openSubmodule(ctx, root, p.path)
	if err != nil || child == nil || !sameFileIdentity(child.gitDirInfo, staged.gitDirInfo) {
		return fmt.Errorf("the submodule checkout changed; retained at %q", p.target.path)
	}
	parent, err := openWorktreeParent(root, filepath.FromSlash(p.path), false)
	if err != nil {
		return err
	}
	if parent != root {
		defer parent.Close()
	}
	name := filepath.Base(p.target.path)
	if err := moveDiscardFile(parent, name, private, "checkout"); err != nil {
		return err
	}
	moved, err := OpenRoot(ctx, filepath.Join(private.Name(), "checkout"))
	if err != nil || !sameFileIdentity(moved.gitDirInfo, staged.gitDirInfo) {
		if err := moveDiscardFile(private, "checkout", parent, name); err != nil {
			return &submoduleRecoveryError{filepath.Join(private.Name(), "checkout"), err}
		}
		return fmt.Errorf("the replacement checkout was retained at %q", p.target.path)
	}
	var restoreErr error
	if backup != "" {
		old := filepath.Base(backup)
		if info, err := parent.Lstat(old); err != nil || !sameFileIdentity(info, p.target.dir) {
			restoreErr = fmt.Errorf("the original submodule directory backup changed at %q", backup)
		} else {
			restoreErr = moveDiscardFile(parent, old, parent, name)
		}
	}
	// Once published, the checkout may contain new local files or Git metadata.
	// Recover the whole repository instead of deleting it on rollback.
	if err := p.repo.checkIndexCheckout(ctx); err != nil {
		return errors.Join(restoreErr, &submoduleRecoveryError{filepath.Join(private.Name(), "checkout"), err})
	}
	recovery := "gitwave-submodule-recovery-" + rand.Text()
	if err := moveDiscardFile(private, "checkout", gitDir, recovery); err != nil {
		return errors.Join(restoreErr, &submoduleRecoveryError{filepath.Join(private.Name(), "checkout"), err})
	}
	return errors.Join(restoreErr, fmt.Errorf("the submodule checkout was saved at %q", filepath.Join(gitDir.Name(), recovery)))
}
