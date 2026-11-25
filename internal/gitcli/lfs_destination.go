package gitcli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// An existing cache is bound to the directory it resolved to at selection.
// A missing cache can be created beneath its nearest existing ancestor.
type lfsCacheBinding struct {
	Anchor, Identity string
}

func bindLFSCache(path string) (lfsCacheBinding, error) {
	for candidate := path; ; candidate = filepath.Dir(candidate) {
		identity, err := lfsDirectoryIdentity(candidate)
		if err == nil {
			return lfsCacheBinding{Anchor: candidate, Identity: identity}, nil
		}
		if !errors.Is(err, fs.ErrNotExist) || candidate == filepath.Dir(candidate) {
			return lfsCacheBinding{}, err
		}
		if _, err := os.Lstat(candidate); !errors.Is(err, fs.ErrNotExist) {
			return lfsCacheBinding{}, fmt.Errorf("the LFS cache path contains an unavailable link")
		}
	}
}

func (b lfsCacheBinding) open(path string, create bool) (*os.Root, error) {
	relative, err := filepath.Rel(b.Anchor, path)
	if err != nil || !filepath.IsLocal(relative) || b.Identity == "" {
		return nil, fmt.Errorf("the selected LFS cache is outside its bound directory")
	}
	root, err := os.OpenRoot(b.Anchor)
	if err != nil {
		return nil, fmt.Errorf("the selected LFS cache ancestor is unavailable: %v", err)
	}
	opened, err := root.Stat(".")
	if err != nil {
		root.Close()
		return nil, err
	}
	identity, err := lfsDirectoryIdentity(b.Anchor)
	actual, statErr := os.Stat(b.Anchor)
	if err != nil || statErr != nil || identity != b.Identity || !sameFileIdentity(opened, actual) {
		root.Close()
		return nil, fmt.Errorf("the selected LFS cache directory was replaced")
	}
	cache, err := openWorktreeParent(root, filepath.Join(relative, "object"), create)
	if cache != root {
		root.Close()
	}
	return cache, err
}

func (b lfsCacheBinding) stillAt(path string) bool {
	root, err := b.open(path, false)
	if root != nil {
		root.Close()
	}
	return err == nil || errors.Is(err, fs.ErrNotExist)
}
