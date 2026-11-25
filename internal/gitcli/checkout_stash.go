package gitcli

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type checkoutStash struct{ ref, oid string }

// The staging, worktree and untracked trees are built in private indices.
// Install the recovery ref before clearing any real paths.
func (p CheckoutPlan) createCheckoutStash(ctx context.Context, lock *checkoutIndexLock, index *workingGit, note string) (checkoutStash, error) {
	r := p.repo
	who, err := r.commitIdentity(ctx)
	if err != nil {
		return checkoutStash{}, err
	}
	base := p.source.head
	if base == "" {
		out, err := index.read(ctx, 4096, "mktree")
		if err != nil {
			return checkoutStash{}, err
		}
		base, err = r.createCommitObject(ctx, strings.TrimSpace(string(out)), nil, who, who, "GitWave checkout recovery base")
		if err != nil {
			return checkoutStash{}, err
		}
	}
	stagedTree, err := index.checkoutStashTree(ctx)
	if err != nil {
		return checkoutStash{}, err
	}
	staged, err := r.createCommitObject(ctx, stagedTree, []string{base}, who, who, "index for "+note)
	if err != nil {
		return checkoutStash{}, err
	}
	var add, remove, untracked []string
	for _, change := range p.dirty.changes {
		switch {
		case change.Kind == '?':
			untracked = append(untracked, change.Path)
		case change.WorkTree == 'D':
			remove = append(remove, change.Path)
		case change.WorkTree != '.' && change.WorkTree != 0:
			add = append(add, change.Path)
		}
	}
	worktree, err := p.snapshotCheckoutTree(ctx, lock, stagedTree, add, remove, true)
	if err != nil {
		return checkoutStash{}, err
	}
	parents := []string{base, staged}
	if len(untracked) != 0 {
		tree, err := p.snapshotCheckoutTree(ctx, lock, "", untracked, nil, false)
		if err != nil {
			return checkoutStash{}, err
		}
		commit, err := r.createCommitObject(ctx, tree, nil, who, who, "untracked files for "+note)
		if err != nil {
			return checkoutStash{}, err
		}
		parents = append(parents, commit)
	}
	oid, err := r.createCommitObject(ctx, worktree, parents, who, who, note)
	if err != nil {
		return checkoutStash{}, err
	}
	ref := "refs/gitwave/checkout-recovery/" + rand.Text()
	if err := r.editDirectRef(ctx, ref, strings.Repeat("0", len(oid)), oid, note, nil); err != nil {
		return checkoutStash{}, err
	}
	return checkoutStash{ref: ref, oid: oid}, nil
}

func (p CheckoutPlan) snapshotCheckoutTree(ctx context.Context, lock *checkoutIndexLock, stagedTree string,
	add, remove []string, tracked bool) (string, error) {
	if len(add)+len(remove) == 0 {
		return stagedTree, nil
	}
	shadow, err := p.repo.workingGit(ctx, p.source.head)
	if err != nil {
		return "", err
	}
	defer shadow.close()
	shadow.index = filepath.Join(shadow.dir, "stash-index")
	shadow.settings = append(shadow.settings, "-c", "core.splitIndex=false")
	if tracked {
		if err := copyOriginalIndex(lock.root, shadow.index); err != nil {
			return "", err
		}
	}
	affected := append(slices.Clone(add), remove...)
	before, err := shadow.otherIndexEntries(ctx, affected)
	if err != nil {
		return "", err
	}
	if err := shadow.stageSelection(ctx, add, remove, nil); err != nil {
		return "", err
	}
	after, err := shadow.otherIndexEntries(ctx, affected)
	if err != nil {
		return "", err
	}
	if !slices.Equal(before, after) {
		return "", fmt.Errorf("snapshot would affect paths outside the confirmed changes")
	}
	if len(add) != 0 {
		root, err := os.OpenRoot(shadow.workTree)
		if err != nil {
			return "", err
		}
		defer root.Close()
		if err := checkCheckoutAliases(root, add); err != nil {
			return "", err
		}
		selected := make(map[string]bool, len(add))
		for _, path := range add {
			selected[path] = true
		}
		for _, file := range p.dirty.files {
			if !selected[file.change.Path] {
				continue
			}
			_, actual, err := inspectDiscardFile(ctx, root, file.local)
			if err != nil {
				return "", err
			}
			if actual.info == nil || file.stamp.info == nil || actual.sum != file.stamp.sum ||
				actual.info.Mode().Type() != file.stamp.info.Mode().Type() {
				return "", fmt.Errorf("the working file changed while saving it: %q", file.change.Path)
			}
		}
	}
	tree, err := shadow.checkoutStashTree(ctx)
	if err != nil {
		return "", err
	}
	if err := shadow.publishLFSObjects(ctx); err != nil {
		return "", err
	}
	return tree, nil
}

func (g *workingGit) checkoutStashTree(ctx context.Context) (string, error) {
	out, err := g.read(ctx, 4096, "write-tree")
	if err != nil {
		return "", err
	}
	tree := strings.TrimSpace(string(out))
	if !validOID(tree) {
		return "", fmt.Errorf("could not write a Git tree for the saved changes")
	}
	return tree, nil
}

func (s checkoutStash) store(ctx context.Context, repo *Repository, note string) error {
	return s.storeMessage(ctx, repo, "On checkout: "+note)
}

func (s checkoutStash) storeMessage(ctx context.Context, repo *Repository, message string) error {
	const ref = "refs/stash"
	var last error
	for range 3 {
		old, err := repo.exactRootRef(ctx, ref)
		if err != nil {
			return err
		}
		if old == s.oid {
			return nil
		}
		if old == "" {
			old = strings.Repeat("0", len(s.oid))
		}
		last = repo.editDirectRef(ctx, ref, old, s.oid, message, nil)
		if last == nil {
			return nil
		}
	}
	return last
}

func (s checkoutStash) remove(ctx context.Context, repo *Repository) string {
	if err := repo.editDirectRef(ctx, s.ref, s.oid, "", "checkout recovery complete", nil); err != nil {
		return fmt.Sprintf("The recovery reference %s could not be removed: %v", s.ref, err)
	}
	return ""
}
