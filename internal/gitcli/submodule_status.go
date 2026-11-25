package gitcli

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
)

func (r *Repository) addSubmoduleFileChanges(ctx context.Context, status *Status) error {
	gitlinks, err := r.indexGitlinks(ctx, true)
	if err != nil || len(gitlinks) == 0 {
		return err
	}
	entries := make(map[string]int, len(gitlinks))
	for i, change := range status.Changes {
		if oid, exists := gitlinks[change.Path]; exists {
			if change.IndexOID != oid || change.WorkTreeMode != "160000" {
				delete(gitlinks, change.Path)
			} else {
				entries[change.Path] = i
			}
		}
	}
	if len(gitlinks) == 0 {
		return nil
	}
	root, err := os.OpenRoot(r.WorkTree)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, path := range slices.Sorted(maps.Keys(gitlinks)) {
		if err := ctx.Err(); err != nil {
			return err
		}
		change, err := r.submoduleWorktreeChange(ctx, root, path, gitlinks[path])
		if err != nil && status.SubmoduleWarning == "" {
			status.SubmoduleWarning = fmt.Sprintf("read submodule status at %q: %s", path, err)
		}
		if change.WorkTree != 'M' {
			continue
		}
		if i, exists := entries[path]; exists {
			current := &status.Changes[i]
			current.WorkTreeOID = change.WorkTreeOID
			current.WorkTree = 'M'
			current.Submodule = change.Submodule
		} else {
			status.Changes = append(status.Changes, change)
		}
	}
	return ctx.Err()
}

func (r *Repository) submoduleWorktreeChange(ctx context.Context, root *os.Root, path, indexOID string) (Change, error) {
	child, err := r.openSubmodule(ctx, root, path)
	if err != nil || child == nil {
		return Change{}, err
	}
	head, err := child.optionalRef(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return Change{}, err
	}
	if !validOID(head) || len(head) != len(indexOID) {
		return Change{}, fmt.Errorf("the submodule has no compatible HEAD commit")
	}
	// A parent's Git process would load the submodule's configured filters.
	// Inspect its worktree using the same isolated reader as the parent.
	status, err := child.workingStatus(ctx, head, "--untracked-files=normal", "--no-renames")
	if err != nil {
		return Change{}, err
	}
	flags := [4]byte{'S', '.', '.', '.'}
	if head != indexOID {
		flags[1] = 'C'
	}
	for _, change := range status.Changes {
		if change.Kind == '?' || (change.hasSubmoduleFileChanges() && change.Submodule[3] == 'U') {
			flags[3] = 'U'
		}
		if change.Kind != '?' && (!change.SubmoduleFilesOnly() || change.Index != '.' || change.Submodule[2] == 'M') {
			flags[2] = 'M'
		}
	}
	worktree := byte('.')
	if flags[1] != '.' || flags[2] != '.' || flags[3] != '.' {
		worktree = 'M'
	}
	if status.SubmoduleWarning != "" {
		err = errors.New(status.SubmoduleWarning)
	}
	return Change{Kind: '1', Index: '.', WorkTree: worktree, Path: path, Submodule: string(flags[:]),
		IndexOID: indexOID, WorkTreeOID: head, IndexMode: "160000", WorkTreeMode: "160000"}, err
}
