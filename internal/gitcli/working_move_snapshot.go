package gitcli

import (
	"os"
	"path/filepath"
)

const moveSnapshotBufferBytes = 4 << 20

type moveSnapshotFile struct {
	path string
	workFileContent
}

// Keep a small candidate snapshot in memory until Git needs it. Larger
// snapshots spill into the private worktree while they are collected.
type moveSnapshotFiles struct {
	root      *os.Root
	parent    *os.Root
	directory string
	remaining int64
	pending   []moveSnapshotFile
}

func (s *moveSnapshotFiles) add(path string, file workFileContent) error {
	if size := int64(cap(file.data)); s.remaining > 0 && size <= s.remaining {
		s.pending = append(s.pending, moveSnapshotFile{path, file})
		s.remaining -= size
		return nil
	}
	if err := s.materialize(); err != nil {
		return err
	}
	return s.write(path, file)
}

func (s *moveSnapshotFiles) materialize() error {
	s.remaining = 0
	for _, file := range s.pending {
		if err := s.write(file.path, file.workFileContent); err != nil {
			return err
		}
	}
	s.pending = nil
	return nil
}

func (s *moveSnapshotFiles) write(path string, file workFileContent) error {
	directory := filepath.Dir(path)
	if s.parent == nil || s.directory != directory {
		s.close()
		parent, err := openWorktreeParent(s.root, path, true)
		if err != nil {
			return err
		}
		s.parent, s.directory = parent, directory
	}
	name := filepath.Base(path)
	if file.mode&os.ModeSymlink != 0 {
		return s.parent.Symlink(string(file.data), name)
	}
	output, err := s.parent.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600|(file.mode.Perm()&0o100))
	if err != nil {
		return err
	}
	_, err = output.Write(file.data)
	closeErr := output.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func (s *moveSnapshotFiles) close() {
	if s.parent != nil && s.parent != s.root {
		_ = s.parent.Close()
	}
	s.parent = nil
}
