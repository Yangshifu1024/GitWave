package gitcli

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

type abortGroup struct {
	path   string
	files  []string
	parent os.FileInfo
	before map[string]discardStamp
}

func inspectAbortGroup(ctx context.Context, root *os.Root, group string, files []string) (abortGroup, error) {
	result := abortGroup{path: group, files: files, before: make(map[string]discardStamp)}
	local := filepath.FromSlash(group)
	parent, err := openWorktreeParent(root, local, false)
	if errors.Is(err, fs.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return abortGroup{}, err
	}
	if parent != root {
		defer parent.Close()
	}
	result.parent, err = parent.Stat(".")
	if err != nil {
		return abortGroup{}, err
	}
	err = scanAbortFiles(ctx, parent, filepath.Base(local), group, result.before)
	return result, err
}

func scanAbortFiles(ctx context.Context, root *os.Root, leaf, name string, entries map[string]discardStamp) error {
	info, err := root.Lstat(leaf)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		stamp, err := readDiscardStamp(ctx, root, leaf)
		if err == nil && !sameFileIdentity(stamp.info, info) {
			return fmt.Errorf("the path changed while inspecting %q", name)
		}
		entries[name] = stamp
		return err
	}
	dir, err := root.OpenRoot(leaf)
	if err != nil {
		return err
	}
	defer dir.Close()
	if opened, err := dir.Stat("."); err != nil || !sameDiscardMetadata(info, opened) {
		return fmt.Errorf("the directory changed while inspecting %q", name)
	}
	entries[name] = discardStamp{info: info}
	listing, err := fs.ReadDir(dir.FS(), ".")
	if err != nil {
		return err
	}
	for _, item := range listing {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !validIndexPath(item.Name()) {
			return fmt.Errorf("the directory contains an independent entry: %q", path.Join(name, item.Name()))
		}
		if err := scanAbortFiles(ctx, dir, item.Name(), path.Join(name, item.Name()), entries); err != nil {
			return err
		}
	}
	return nil
}

func matchingAbortFiles(before, after map[string]discardStamp) bool {
	if len(before) != len(after) {
		return false
	}
	for name, stamp := range before {
		if current, found := after[name]; !found || !stamp.matches(current) {
			return false
		}
	}
	return true
}

func inspectMergeState(ctx context.Context, root *os.Root) (abortGroup, error) {
	result := abortGroup{path: ".", before: make(map[string]discardStamp)}
	for _, name := range abortStateNames {
		stamp, err := readDiscardStamp(ctx, root, name)
		if err != nil {
			return abortGroup{}, err
		}
		if stamp.info != nil {
			if !stamp.info.Mode().IsRegular() {
				return abortGroup{}, fmt.Errorf("invalid merge state file %s", name)
			}
			result.before[name] = stamp
		}
	}
	return result, nil
}

type abortSwap struct {
	group         abortGroup
	parent, temp  *os.Root
	tempName      string
	tempIdentity  os.FileInfo
	parentOwned   bool
	replacement   map[string]discardStamp
	originalMoved bool
	restoredMoved bool
}

func createAbortTemp(parent *os.Root) (string, *os.Root, os.FileInfo, error) {
	name := ".gitwave-abort-" + rand.Text()
	if err := parent.Mkdir(name, 0o700); err != nil {
		return "", nil, nil, err
	}
	before, err := parent.Lstat(name)
	if err != nil || !before.IsDir() {
		return "", nil, nil, fmt.Errorf("the working copy recovery directory changed before opening")
	}
	temp, err := parent.OpenRoot(name)
	if err != nil {
		return "", nil, nil, err
	}
	if after, err := temp.Stat("."); err != nil || !sameFileIdentity(before, after) {
		temp.Close()
		return "", nil, nil, fmt.Errorf("the working copy recovery directory changed while opening")
	}
	return name, temp, before, nil
}

func (g abortGroup) prepare(ctx context.Context, root, source *os.Root, files, modes map[string]string, symlinks bool) (*abortSwap, error) {
	current, err := inspectAbortGroup(ctx, root, g.path, nil)
	if err != nil || (g.parent != nil && !sameFileIdentity(g.parent, current.parent)) || !matchingAbortFiles(g.before, current.before) {
		return nil, errors.Join(err, fmt.Errorf("the working path changed since confirmation: %q", g.path))
	}
	if len(g.before) == 0 && !slices.ContainsFunc(g.files, func(name string) bool { return files[name] != "" }) {
		return nil, nil
	}
	local := filepath.FromSlash(g.path)
	parent, err := openWorktreeParent(root, local, g.parent == nil)
	if err != nil {
		return nil, err
	}
	s := &abortSwap{group: g, parent: parent, parentOwned: parent != root, replacement: make(map[string]discardStamp)}
	if info, err := parent.Stat("."); err != nil || (g.parent != nil && !sameFileIdentity(g.parent, info)) {
		s.close()
		return nil, fmt.Errorf("the working directory changed: %q", g.path)
	} else {
		s.group.parent = info
	}
	s.tempName, s.temp, s.tempIdentity, err = createAbortTemp(parent)
	if err != nil {
		s.close()
		return nil, err
	}
	for _, file := range g.files {
		if files[file] == "" {
			continue
		}
		within := strings.TrimPrefix(strings.TrimPrefix(file, g.path), "/")
		name := filepath.Join("restored", filepath.FromSlash(within))
		if err := s.temp.MkdirAll(filepath.Dir(name), 0o777); err != nil {
			s.close()
			return nil, err
		}
		if err := copyDiscardReplacement(ctx, source, files[file], s.temp, name, modes[file], symlinks); err != nil {
			s.close()
			return nil, err
		}
	}
	if err := scanAbortFiles(ctx, s.temp, "restored", g.path, s.replacement); err != nil {
		s.close()
		return nil, err
	}
	return s, nil
}

func (s *abortSwap) apply(ctx context.Context, root *os.Root) error {
	current, err := inspectAbortGroup(ctx, root, s.group.path, nil)
	if err != nil || !sameFileIdentity(s.group.parent, current.parent) || !matchingAbortFiles(s.group.before, current.before) {
		return errors.Join(err, fmt.Errorf("the working path changed before replacement: %q", s.group.path))
	}
	name := path.Base(s.group.path)
	if len(s.group.before) != 0 {
		if err := moveDiscardFile(s.parent, name, s.temp, "original"); err != nil {
			return err
		}
		s.originalMoved = true
		moved := make(map[string]discardStamp)
		if err := scanAbortFiles(ctx, s.temp, "original", s.group.path, moved); err != nil || !matchingAbortFiles(s.group.before, moved) {
			return errors.Join(err, fmt.Errorf("the working path changed during replacement: %q", s.group.path))
		}
	}
	if len(s.replacement) != 0 {
		if err := moveDiscardFile(s.temp, "restored", s.parent, name); err != nil {
			return err
		}
		s.restoredMoved = true
	}
	return checkDiscardParent(root, filepath.FromSlash(s.group.path), s.group.parent)
}

func (s *abortSwap) restore(ctx context.Context) error {
	name := path.Base(s.group.path)
	if s.restoredMoved {
		if err := moveDiscardFile(s.parent, name, s.temp, "restored"); err != nil {
			return s.recoveryError(err)
		}
		moved := make(map[string]discardStamp)
		if err := scanAbortFiles(ctx, s.temp, "restored", s.group.path, moved); err != nil || !matchingAbortFiles(s.replacement, moved) {
			restoreErr := moveDiscardFile(s.temp, "restored", s.parent, name)
			return s.recoveryError(errors.Join(err, restoreErr, fmt.Errorf("the restored path %q received an external edit", s.group.path)))
		}
		s.restoredMoved = false
	}
	if s.originalMoved {
		if err := moveDiscardFile(s.temp, "original", s.parent, name); err != nil {
			return s.recoveryError(err)
		}
		s.originalMoved = false
	}
	return nil
}

func (s *abortSwap) recoveryError(err error) error {
	return fmt.Errorf("working copy recovery files kept at %q: %w", filepath.Join(s.parent.Name(), s.tempName), err)
}

func (s *abortSwap) finish(ctx context.Context) error {
	if !s.originalMoved {
		return nil
	}
	moved := make(map[string]discardStamp)
	if err := scanAbortFiles(ctx, s.temp, "original", s.group.path, moved); err != nil || !matchingAbortFiles(s.group.before, moved) {
		return s.recoveryError(errors.Join(err, fmt.Errorf("the original received an external edit")))
	}
	var names []string
	for name := range moved {
		names = append(names, name)
	}
	slices.Sort(names)
	for i := len(names) - 1; i >= 0; i-- {
		name := filepath.Join("original", filepath.FromSlash(strings.TrimPrefix(strings.TrimPrefix(names[i], s.group.path), "/")))
		if err := s.temp.Remove(name); err != nil {
			return s.recoveryError(err)
		}
	}
	s.originalMoved = false
	return nil
}

func (s *abortSwap) close() {
	if s.temp != nil {
		if !s.restoredMoved {
			_ = s.temp.RemoveAll("restored")
		}
		s.temp.Close()
		if current, err := s.parent.Lstat(s.tempName); err == nil && sameFileIdentity(s.tempIdentity, current) {
			_ = s.parent.Remove(s.tempName)
		}
	}
	if s.parentOwned {
		s.parent.Close()
	}
}
