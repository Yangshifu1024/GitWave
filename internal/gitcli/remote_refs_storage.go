package gitcli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func (r *Repository) checkTrackingRefPaths(ctx context.Context, affected map[string]bool) error {
	format, err := r.optionalRef(ctx, "rev-parse", "--show-ref-format")
	if err != nil || format != "files" {
		return err
	}
	common, err := os.OpenRoot(r.CommonDir)
	if err != nil {
		return err
	}
	defer common.Close()
	for name := range affected {
		if _, err := fetchFileRefIsSymbolic(common, name); err != nil {
			return err
		}
	}
	return nil
}

func (s remoteConfigEdit) appendUnresolvedRemoteRefs(from, to string, refs *[]remoteTrackingRef) error {
	r := s.repo
	format, err := r.optionalRef(s.ctx, "rev-parse", "--show-ref-format")
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(*refs))
	for _, ref := range *refs {
		seen[ref.name] = true
	}
	appendSymbolic := func(name string) error {
		if seen[name] {
			return nil
		}
		target, err := r.optionalRef(s.ctx, "symbolic-ref", "--no-recurse", "-q", name)
		if err != nil {
			return err
		}
		if target != "" {
			*refs = append(*refs, remoteTrackingRef{name: name, symbolic: target})
			seen[name] = true
		}
		return nil
	}
	if format != "files" {
		// for-each-ref omits dangling references. Remote HEAD can be
		// addressed directly even if no branch has been fetched yet.
		if err := appendSymbolic(from + "HEAD"); err != nil {
			return err
		}
		return appendSymbolic(to + "HEAD")
	}
	common, err := os.OpenRoot(r.CommonDir)
	if err != nil {
		return err
	}
	defer common.Close()
	for _, base := range []string{strings.TrimSuffix(from, "/"), strings.TrimSuffix(to, "/")} {
		if err := fs.WalkDir(common.FS(), base, func(path string, item fs.DirEntry, err error) error {
			if errors.Is(err, fs.ErrNotExist) && path == base {
				return nil
			}
			if err != nil {
				return err
			}
			if err := s.ctx.Err(); err != nil {
				return err
			}
			if item.IsDir() || strings.HasSuffix(path, ".lock") || seen[path] {
				return nil
			}
			if !item.Type().IsRegular() {
				return fmt.Errorf("unsupported file in remote-tracking refs at %q", path)
			}
			return appendSymbolic(path)
		}); err != nil {
			return err
		}
	}
	return nil
}

type remoteReflogCopies struct {
	root      *os.Root
	copies    []remoteReflogCopy
	published bool
}

type remoteReflogCopy struct {
	path   string
	info   os.FileInfo
	digest [sha256.Size]byte
}

const remoteReflogLimit = 64 << 20

func (l *remoteReflogCopies) install(ctx context.Context, r *Repository, moves []remoteRefMove) error {
	if len(moves) == 0 {
		return nil
	}
	format, err := r.optionalRef(ctx, "rev-parse", "--show-ref-format")
	if err != nil {
		return err
	}
	if format != "files" {
		return fmt.Errorf("unsupported Git reference storage %q", format)
	}
	l.root, err = os.OpenRoot(r.CommonDir)
	if err != nil {
		return err
	}
	for _, move := range moves {
		if err := ctx.Err(); err != nil {
			return err
		}
		oldPath := filepath.FromSlash("logs/" + move.from.name)
		data, mode, err := readGitStorageFile(l.root, oldPath, remoteReflogLimit)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if err := l.installFile(filepath.FromSlash("logs/"+move.to.name), data, mode.Perm()); err != nil {
			return err
		}
	}
	return nil
}

func (l *remoteReflogCopies) installFile(path string, data []byte, mode os.FileMode) error {
	parent, err := openWorktreeParent(l.root, path, true)
	if err != nil {
		return err
	}
	if parent != l.root {
		defer parent.Close()
	}
	file, err := parent.OpenFile(filepath.Base(path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("could not preserve the tracking reflog at %q: %w", path, err)
	}
	info, statErr := file.Stat()
	if statErr != nil {
		file.Close()
		return statErr
	}
	_, writeErr := file.Write(data)
	modeErr := file.Chmod(mode)
	closeErr := file.Close()
	if err := errors.Join(writeErr, modeErr, closeErr); err != nil {
		if current, statErr := l.root.Lstat(path); statErr == nil && os.SameFile(info, current) {
			_ = l.root.Remove(path)
		}
		return err
	}
	l.copies = append(l.copies, remoteReflogCopy{path, info, sha256.Sum256(data)})
	return nil
}

func (l *remoteReflogCopies) close() {
	if l.root == nil {
		return
	}
	defer l.root.Close()
	if !l.published {
		for _, copy := range l.copies {
			if now, err := l.root.Lstat(copy.path); err == nil && os.SameFile(now, copy.info) {
				data, _, err := readGitStorageFile(l.root, copy.path, remoteReflogLimit)
				if err == nil && sha256.Sum256(data) == copy.digest {
					_ = l.root.Remove(copy.path)
				}
			}
		}
	}
}
