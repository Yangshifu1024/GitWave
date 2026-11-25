package gitcli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type Worktree struct {
	ID          string
	Name        string
	Path        string
	Head        string
	Branch      string
	Main        bool
	Bare        bool
	Locked      bool
	Prunable    bool
	Provisional bool
	Current     bool
}

const provisionalWorktreeLock = "GitWave creating .gitwave-worktree-"

func (r *Repository) Worktrees(ctx context.Context) ([]Worktree, error) {
	data, err := r.readLimited(ctx, "", 16<<20, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	trees, err := parseWorktrees(string(data))
	if err != nil {
		return nil, err
	}
	admin, err := r.worktreeLocations(ctx)
	if err != nil {
		return nil, err
	}
	for i := range trees {
		tree := &trees[i]
		tree.Current = isOwnWorktree(tree.Path, r.Path())
		if tree.Main {
			tree.Name = "(main)"
			continue
		}
		entry := admin[filepath.Clean(tree.Path)]
		tree.ID, tree.Name = entry.id, entry.name
		if tree.Name == "" {
			tree.Name = filepath.Base(tree.Path)
		}
	}
	return trees, nil
}

func parseWorktrees(output string) ([]Worktree, error) {
	var result []Worktree
	var tree Worktree
	for entry := range strings.SplitSeq(output, "\x00") {
		field, value, _ := strings.Cut(entry, " ")
		switch field {
		case "":
			if tree.Path != "" {
				tree.Main = len(result) == 0
				result = append(result, tree)
				tree = Worktree{}
			}
		case "worktree":
			if tree.Path != "" || !filepath.IsAbs(value) {
				return nil, fmt.Errorf("invalid Git worktree path")
			}
			tree.Path = filepath.Clean(value)
		case "HEAD":
			if !validOID(value) {
				return nil, fmt.Errorf("invalid Git worktree HEAD at %q", tree.Path)
			}
			tree.Head = value
		case "branch":
			tree.Branch = strings.TrimPrefix(value, "refs/heads/")
		case "bare":
			tree.Bare = true
		case "locked":
			tree.Locked = true
			tree.Provisional = strings.HasPrefix(value, provisionalWorktreeLock)
		case "prunable":
			tree.Prunable = true
		}
	}
	if tree.Path != "" {
		return nil, fmt.Errorf("incomplete Git worktree list")
	}
	return result, nil
}

type worktreeLocation struct {
	id, name string
}

const worktreeNameFile = "gitwave-worktree-name"

func (r *Repository) worktreeLocations(ctx context.Context) (map[string]worktreeLocation, error) {
	common, err := os.OpenRoot(r.CommonDir)
	if err != nil {
		return nil, err
	}
	defer common.Close()
	entries, err := fs.ReadDir(common.FS(), "worktrees")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	result := make(map[string]worktreeLocation, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		base := filepath.Join("worktrees", id)
		data, _, err := readGitStorageFile(common, filepath.Join(base, "gitdir"), 64<<10)
		if errors.Is(err, fs.ErrNotExist) {
			continue // An external Git process is creating or removing this worktree.
		}
		if err != nil {
			return nil, err
		}
		link := strings.TrimSuffix(string(data), "\n")
		if !filepath.IsAbs(link) {
			link = filepath.Join(r.CommonDir, base, link)
		}
		if filepath.Base(link) != ".git" {
			continue
		}
		name := id
		label, _, err := readGitStorageFile(common, filepath.Join(base, worktreeNameFile), 4096)
		if err == nil && len(label) != 0 {
			name = strings.TrimSuffix(string(label), "\n")
		}
		result[filepath.Clean(filepath.Dir(link))] = worktreeLocation{id, name}
	}
	return result, nil
}

func (r *Repository) checkWorktree(ctx context.Context, expected Worktree) (Worktree, error) {
	trees, err := r.Worktrees(ctx)
	if err != nil {
		return Worktree{}, err
	}
	i := slices.IndexFunc(trees, func(item Worktree) bool { return item == expected })
	if i < 0 {
		return Worktree{}, fmt.Errorf("the Git worktree changed since it was shown")
	}
	return trees[i], nil
}

func (r *Repository) OpenWorktree(ctx context.Context, expected Worktree) (*Repository, error) {
	if expected.Provisional {
		return nil, fmt.Errorf("the linked worktree is still being created")
	}
	if _, err := r.checkWorktree(ctx, expected); err != nil {
		return nil, err
	}
	target, err := OpenRoot(ctx, expected.Path)
	if err != nil {
		return nil, err
	}
	if !r.sameCommonWorktree(target) || (!expected.Main && filepath.Base(target.GitDir) != expected.ID) {
		return nil, fmt.Errorf("the selected path no longer belongs to this repository's worktree")
	}
	return target, nil
}

func (r *Repository) sameCommonWorktree(target *Repository) bool {
	first, err := os.Stat(r.CommonDir)
	second, otherErr := os.Stat(target.CommonDir)
	return err == nil && otherErr == nil && os.SameFile(first, second)
}
