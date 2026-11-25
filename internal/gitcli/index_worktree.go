package gitcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Stage reads files from a private worktree. Live paths are opened through the
// original root and copied without letting Git follow a path replaced during
// selection. This also keeps the attribute check and Git's add on one snapshot.
func (g *workingGit) snapshotStageFiles(ctx context.Context, root *os.Root, paths []string) error {
	if err := g.isolateWorkTree(); err != nil {
		return err
	}
	target, err := os.OpenRoot(g.workTree)
	if err != nil {
		return err
	}
	defer target.Close()
	if err := g.snapshotWorktreeMetadata(ctx, root, target, paths, nil, ".gitattributes", ".gitignore"); err != nil {
		return err
	}
	for _, path := range paths {
		local := filepath.FromSlash(path)
		info, err := root.Lstat(local)
		if err != nil {
			return err
		}
		if err := g.copyStageFile(ctx, root, target, local, info, false); err != nil {
			return err
		}
	}
	return nil
}

func (g *workingGit) snapshotWorktreeMetadata(ctx context.Context, root, target *os.Root, paths []string,
	skip func(os.FileInfo) bool, names ...string) error {
	copied := make(map[string]bool)
	for _, path := range paths {
		for parent := filepath.Dir(filepath.FromSlash(path)); ; parent = filepath.Dir(parent) {
			for _, name := range names {
				metadata := filepath.Join(parent, name)
				if copied[metadata] {
					continue
				}
				copied[metadata] = true
				info, err := root.Lstat(metadata)
				if errors.Is(err, fs.ErrNotExist) || (err == nil && !info.Mode().IsRegular()) {
					continue
				}
				if err != nil {
					return err
				}
				if skip != nil && skip(info) {
					continue
				}
				if err := g.copyStageFile(ctx, root, target, metadata, info, true); err != nil {
					return fmt.Errorf("copy Git metadata %q: %w", metadata, err)
				}
			}
			if parent == "." {
				break
			}
		}
	}
	return nil
}

func (g *workingGit) copyStageFile(ctx context.Context, root, target *os.Root, local string, before fs.FileInfo, exclusive bool) error {
	parent, err := openWorktreeParent(root, local, false)
	if err != nil {
		return err
	}
	if parent != root {
		defer parent.Close()
	}
	name := filepath.Base(local)
	current, err := parent.Lstat(name)
	if err != nil || !os.SameFile(before, current) {
		return fmt.Errorf("the working file changed before staging it: %q", local)
	}
	if err := target.MkdirAll(filepath.Dir(local), 0o700); err != nil {
		return err
	}
	switch {
	case before.Mode()&os.ModeSymlink != 0:
		link, err := parent.Readlink(name)
		if err != nil {
			return err
		}
		if err := target.Symlink(link, local); err != nil {
			return err
		}
		return verifyStageSource(root, parent, local, before)
	case !before.Mode().IsRegular():
		return fmt.Errorf("the working file cannot be staged: %q", local)
	}

	in, err := parent.OpenFile(name, os.O_RDONLY|workFileOpenFlags, 0)
	if err != nil {
		return err
	}
	defer in.Close()
	opened, err := in.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(before, opened) || !opened.Mode().IsRegular() {
		return fmt.Errorf("the working file changed while opening it: %q", local)
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC | workFileOpenFlags
	if exclusive {
		flags |= os.O_EXCL
	}
	out, err := target.OpenFile(local, flags, 0o600|(opened.Mode().Perm()&0o100))
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, stageReader{ctx, in})
	if err := errors.Join(copyErr, out.Close()); err != nil {
		return err
	}
	return verifyStageSource(root, parent, local, opened)
}

func openWorktreeParent(root *os.Root, local string, create bool) (*os.Root, error) {
	parent := root
	dir := filepath.Dir(local)
	if dir == "." {
		return parent, nil
	}
	for part := range strings.SplitSeq(dir, string(filepath.Separator)) {
		found, err := parent.Lstat(part)
		if create && errors.Is(err, fs.ErrNotExist) {
			err = parent.Mkdir(part, 0o777)
			if err == nil || errors.Is(err, fs.ErrExist) {
				found, err = parent.Lstat(part)
			}
		}
		if err != nil {
			if parent != root {
				parent.Close()
			}
			return nil, fmt.Errorf("open working path %q: %w", local, err)
		}
		if !found.IsDir() {
			if parent != root {
				parent.Close()
			}
			return nil, fmt.Errorf("the working path passes through a replaced directory: %q", local)
		}
		next, err := parent.OpenRoot(part)
		if parent != root {
			parent.Close()
		}
		if err != nil {
			return nil, err
		}
		opened, err := next.Stat(".")
		if err != nil || !os.SameFile(found, opened) {
			next.Close()
			return nil, fmt.Errorf("the working directory changed while opening %q", local)
		}
		parent = next
	}
	return parent, nil
}

func verifyStageSource(root, parent *os.Root, local string, before fs.FileInfo) error {
	fresh, err := openWorktreeParent(root, local, false)
	if err != nil {
		return err
	}
	if fresh != root {
		defer fresh.Close()
	}
	parentInfo, err := parent.Stat(".")
	if err != nil {
		return err
	}
	actualParent, err := fresh.Stat(".")
	if err != nil {
		return err
	}
	after, err := fresh.Lstat(filepath.Base(local))
	if err != nil || !os.SameFile(parentInfo, actualParent) || !os.SameFile(before, after) ||
		before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return fmt.Errorf("the working file changed while staging it: %q", local)
	}
	return nil
}

type stageReader struct {
	ctx context.Context
	in  io.Reader
}

func (r stageReader) Read(buf []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.in.Read(buf)
}

func (g *workingGit) freezeWorktreeAttributes(ctx context.Context) error {
	combined := filepath.Join(g.dir, "worktree-attributes")
	file, err := os.OpenFile(combined, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	system, err := g.repo.optionalRef(ctx, "var", "GIT_ATTR_SYSTEM")
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 129 {
			return fmt.Errorf("Git 2.42 or later is needed to read worktree attributes")
		}
		return fmt.Errorf("read system Git attributes path: %w", err)
	}
	global, err := g.globalAttributesPath(ctx)
	if err != nil {
		return err
	}
	for _, path := range []string{system, global} {
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = g.repo.WorkTree + string(os.PathSeparator) + path
		}
		if err := appendStageAttributes(ctx, file, path); err != nil {
			return err
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	g.settings = append(g.settings, "-c", "core.attributesfile="+combined)
	g.frozenAttributes = true
	return nil
}

func (g *workingGit) globalAttributesPath(ctx context.Context) (string, error) {
	// git var normalizes paths lexically, which can change symlink/.. targets.
	output, err := g.read(ctx, 64<<10, "config", "--null", "--path", "--get", "core.attributesfile")
	if err == nil {
		if name, ok := strings.CutSuffix(string(output), "\x00"); ok && !strings.ContainsRune(name, '\x00') {
			return name, nil
		}
		return "", fmt.Errorf("invalid global Git attributes path")
	}
	if !gitExitCode(err, 1) {
		return "", fmt.Errorf("read global Git attributes path: %w", err)
	}
	if directory := os.Getenv("XDG_CONFIG_HOME"); directory != "" {
		return directory + "/git/attributes", nil
	}
	if home, ok := os.LookupEnv("HOME"); ok {
		return home + "/.config/git/attributes", nil
	}
	return g.repo.optionalRef(ctx, "var", "GIT_ATTR_GLOBAL")
}

func appendStageAttributes(ctx context.Context, out io.Writer, source string) error {
	path, err := filepath.EvalSymlinks(source)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	in, err := os.OpenFile(path, os.O_RDONLY|workFileOpenFlags, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	const limit = 16 << 20
	if !info.Mode().IsRegular() || info.Size() > limit {
		return fmt.Errorf("invalid or oversized Git attributes file")
	}
	size, err := io.Copy(out, io.LimitReader(stageReader{ctx, in}, limit+1))
	if err != nil {
		return err
	}
	if size > limit {
		return fmt.Errorf("Git attributes file exceeds the read limit")
	}
	_, err = io.WriteString(out, "\n")
	return err
}
