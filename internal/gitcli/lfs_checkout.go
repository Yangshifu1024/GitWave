package gitcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Preserve a hydrated source even if the LFS cache was pruned or moved. A
// clean checkout can otherwise delete the only copy of an unpublished object.
// Return existing pointer paths so recovery can restore their original form.
func (g *workingGit) preserveWorkingLFSObjects(ctx context.Context, paths []string) (map[string]bool, error) {
	if len(g.lfsEnvironment) == 0 || len(paths) == 0 {
		return nil, nil
	}
	indexed, err := g.filterValues(ctx, paths, "--cached")
	if err != nil {
		return nil, err
	}
	working, err := g.filterValues(ctx, paths)
	if err != nil {
		return nil, err
	}
	selected := make(map[string]bool)
	for i, value := range indexed {
		if value == "lfs" || working[i] == "lfs" {
			selected[paths[i]] = true
		}
	}
	if len(selected) == 0 {
		return nil, nil
	}
	index, err := g.read(ctx, 128<<20, "ls-files", "--stage", "-z", "--")
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(g.repo.WorkTree)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	pointers := make(map[string]bool)
	for record := range strings.SplitSeq(strings.TrimSuffix(string(index), "\x00"), "\x00") {
		metadata, path, found := strings.Cut(record, "\t")
		if !found || !selected[path] {
			continue
		}
		fields := strings.Fields(metadata)
		if len(fields) != 3 || !validOID(fields[1]) {
			return nil, fmt.Errorf("invalid Git index entry for LFS path %q", path)
		}
		if fields[2] != "0" || fields[0] != "100644" && fields[0] != "100755" {
			continue
		}
		blob, err := g.read(ctx, lfsPointerLimit, "cat-file", "blob", fields[1])
		if errors.Is(err, errGitOutputLimit) {
			continue // Git already stores the full contents.
		}
		if err != nil {
			return nil, err
		}
		pointer, ok := parseLFSPointer(blob)
		if !ok || pointer.extended {
			continue
		}
		isPointer, err := g.preserveMissingLFSObject(ctx, root, filepath.FromSlash(path), pointer)
		if err != nil {
			return nil, fmt.Errorf("preserve the LFS object for %q: %w", path, err)
		}
		if isPointer {
			pointers[path] = true
		}
	}
	return pointers, nil
}

func (g *workingGit) preserveMissingLFSObject(ctx context.Context, root *os.Root, local string, pointer lfsPointer) (bool, error) {
	parent, err := openWorktreeParent(root, local, false)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if parent != root {
		defer parent.Close()
	}
	name := filepath.Base(local)
	info, err := parent.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() != pointer.size && info.Size() >= lfsPointerLimit {
		return false, err
	}
	source, err := openLFSRegular(parent, name)
	if err != nil {
		return false, err
	}
	defer source.Close()
	if info.Size() < lfsPointerLimit {
		data, err := io.ReadAll(io.LimitReader(source, lfsPointerLimit))
		if err != nil {
			return false, err
		}
		if old, ok := parseLFSPointer(data); ok {
			return old == pointer, nil
		}
		if int64(len(data)) != pointer.size {
			return false, nil
		}
		if _, err := source.Seek(0, io.SeekStart); err != nil {
			return false, err
		}
	}
	if pointer.size == 0 {
		return false, nil
	}
	cached, err := openLFSObject(g.localLFS.storage, pointer.oid)
	if err == nil {
		defer cached.Close()
		return false, copyLFSObject(ctx, io.Discard, cached, pointer)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	cache, err := g.openLFSStorage(ctx)
	if err != nil {
		return false, err
	}
	defer cache.Close()
	err = storeLFSObject(ctx, cache, source, pointer.oid)
	if errors.Is(err, errLFSObjectMismatch) {
		// These bytes are an independent edit, not the committed asset.
		// Leave them to the checkout's existing dirty-file protection.
		return false, nil
	}
	return false, err
}
