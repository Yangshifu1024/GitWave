package gitcli

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

type discardStamp struct {
	info os.FileInfo
	sum  [sha256.Size]byte
}

func sameFileIdentity(a, b os.FileInfo) bool {
	return a == nil && b == nil || a != nil && b != nil && os.SameFile(a, b)
}

func (s discardStamp) matches(other discardStamp) bool {
	return sameFileIdentity(s.info, other.info) && s.sum == other.sum &&
		(s.info == nil || sameDiscardMetadata(s.info, other.info))
}

func sameDiscardMetadata(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.Mode() == b.Mode() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

func discardChanged(path string) error {
	return fmt.Errorf("the working file changed since discard was requested: %q", path)
}

func inspectDiscardFile(ctx context.Context, root *os.Root, path string) (os.FileInfo, discardStamp, error) {
	parent, err := openWorktreeParent(root, path, false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, discardStamp{}, nil
	}
	if err != nil {
		return nil, discardStamp{}, err
	}
	if parent != root {
		defer parent.Close()
	}
	identity, err := parent.Stat(".")
	if err != nil {
		return nil, discardStamp{}, err
	}
	stamp, err := readDiscardStamp(ctx, parent, filepath.Base(path))
	return identity, stamp, err
}

func readDiscardStamp(ctx context.Context, root *os.Root, path string) (discardStamp, error) {
	if err := ctx.Err(); err != nil {
		return discardStamp{}, err
	}
	info, err := root.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return discardStamp{}, nil
	}
	if err != nil {
		return discardStamp{}, err
	}
	stamp := discardStamp{info: info}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := root.Readlink(path)
		if err != nil {
			return discardStamp{}, err
		}
		stamp.sum = sha256.Sum256([]byte(target))
	case info.Mode().IsRegular():
		file, err := root.OpenFile(path, os.O_RDONLY|workFileOpenFlags, 0)
		if err != nil {
			return discardStamp{}, err
		}
		defer file.Close()
		opened, err := file.Stat()
		if err != nil || !sameDiscardMetadata(info, opened) {
			return discardStamp{}, discardChanged(path)
		}
		hash := sha256.New()
		if _, err := io.Copy(hash, stageReader{ctx, file}); err != nil {
			return discardStamp{}, err
		}
		hash.Sum(stamp.sum[:0])
	default:
		return discardStamp{}, fmt.Errorf("discard only supports files and symbolic links: %q", path)
	}
	after, err := root.Lstat(path)
	if err != nil || !sameDiscardMetadata(info, after) {
		return discardStamp{}, discardChanged(path)
	}
	return stamp, nil
}

func (f discardFile) apply(ctx context.Context, root, restored *os.Root, source string, symlinks bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	parent, err := openWorktreeParent(root, f.local, f.parent == nil)
	if err != nil {
		return err
	}
	if parent != root {
		defer parent.Close()
	}
	name := filepath.Base(f.local)
	identity, err := parent.Stat(".")
	if err != nil {
		return err
	}
	if f.parent != nil && !os.SameFile(f.parent, identity) {
		return discardChanged(f.local)
	}
	tmpName := ".gitwave-discard-" + rand.Text()
	if err := parent.Mkdir(tmpName, 0o700); err != nil {
		return err
	}
	tmpInfo, err := parent.Lstat(tmpName)
	if err != nil || !tmpInfo.IsDir() {
		return fmt.Errorf("the discard backup directory changed before opening")
	}
	tmp, err := parent.OpenRoot(tmpName)
	if err != nil {
		return err
	}
	opened, err := tmp.Stat(".")
	if err != nil || !os.SameFile(tmpInfo, opened) {
		tmp.Close()
		return fmt.Errorf("the discard backup directory changed while opening")
	}
	const replacement, original = "restored", "original"
	defer func() {
		_ = tmp.Remove(replacement)
		tmp.Close()
		if current, err := parent.Lstat(tmpName); err == nil && os.SameFile(tmpInfo, current) {
			_ = parent.Remove(tmpName)
		}
	}()
	if f.change.Kind != '?' {
		if err := copyDiscardReplacement(ctx, restored, source, tmp, replacement, f.change.IndexMode, symlinks); err != nil {
			return err
		}
	}
	actualParent, current, err := inspectDiscardFile(ctx, root, f.local)
	if err != nil {
		return err
	}
	if !sameFileIdentity(actualParent, identity) || !current.matches(f.stamp) {
		return discardChanged(f.local)
	}
	if f.stamp.info == nil {
		return moveDiscardFile(tmp, replacement, parent, name)
	}
	// Move the exact candidate out of the way, then verify what actually moved.
	// Publishing the replacement or putting the original back never overwrites a
	// file an editor creates at the destination during this operation.
	if err := moveDiscardFile(parent, name, tmp, original); err != nil {
		return err
	}
	saved := filepath.Join(filepath.Dir(f.local), tmpName, original)
	undo := func(cause error) error {
		if err := moveDiscardFile(tmp, original, parent, name); err != nil {
			return fmt.Errorf("original saved at %q. %w (%v)", saved, cause, err)
		}
		return cause
	}
	moved, err := readDiscardStamp(ctx, tmp, original)
	if err != nil {
		return undo(err)
	}
	if !moved.matches(f.stamp) {
		return undo(discardChanged(f.local))
	}
	if err := checkDiscardParent(root, f.local, identity); err != nil {
		return undo(err)
	}
	if f.change.Kind != '?' {
		if err := moveDiscardFile(tmp, replacement, parent, name); err != nil {
			return undo(err)
		}
		if err := checkDiscardParent(root, f.local, identity); err != nil {
			return fmt.Errorf("original saved at %q in the moved directory. %w", saved, err)
		}
	}
	if err := tmp.Remove(original); err != nil {
		return fmt.Errorf("original remains at %q after discarding %q: %w", saved, f.local, err)
	}
	return nil
}

func checkDiscardParent(root *os.Root, local string, expected os.FileInfo) error {
	parent, err := openWorktreeParent(root, local, false)
	if err != nil {
		return err
	}
	if parent != root {
		defer parent.Close()
	}
	identity, err := parent.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(identity, expected) {
		return discardChanged(local)
	}
	return nil
}

func moveDiscardFile(source *os.Root, old string, target *os.Root, newName string) error {
	sourceDir, err := source.Open(".")
	if err != nil {
		return err
	}
	defer sourceDir.Close()
	targetDir, err := target.Open(".")
	if err != nil {
		return err
	}
	defer targetDir.Close()
	if err := renameWithoutReplace(sourceDir.Fd(), old, targetDir.Fd(), newName); err != nil {
		return fmt.Errorf("move %q to %q without replacing an existing file: %w", old, newName, err)
	}
	return nil
}

func copyDiscardReplacement(ctx context.Context, source *os.Root, path string, target *os.Root, replacement, mode string, symlinks bool) error {
	if mode == "120000" && symlinks {
		info, err := source.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			data, err := source.Readlink(path)
			if err != nil {
				return err
			}
			return target.Symlink(data, replacement)
		}
	}
	in, err := source.OpenFile(path, os.O_RDONLY|workFileOpenFlags, 0)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("invalid staged file output")
	}
	if mode == "120000" && symlinks {
		const maxTarget = 32 << 10
		data, err := io.ReadAll(io.LimitReader(in, maxTarget+1))
		if err != nil {
			return err
		}
		if len(data) > maxTarget {
			return fmt.Errorf("the staged symbolic link target is too long")
		}
		return target.Symlink(string(data), replacement)
	}
	permissions := os.FileMode(0o666)
	if mode == "100755" {
		permissions = 0o777
	}
	out, err := target.OpenFile(replacement, os.O_WRONLY|os.O_CREATE|os.O_EXCL, permissions)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, stageReader{ctx, in})
	return errors.Join(copyErr, out.Close())
}
