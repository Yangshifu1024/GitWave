package gitcli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

var clientHooks = []string{
	"pre-commit", "prepare-commit-msg", "commit-msg", "post-commit",
	"pre-rebase", "post-merge", "pre-push", "post-checkout",
}

type HookInfo struct {
	Name, Path         string
	Exists, Executable bool
}

type HooksEditor struct {
	repo      *Repository
	directory editorDirectory
}

type HookFile struct {
	panel *HooksEditor
	file  repositoryEditor
	name  string
	link  os.FileInfo
	path  string
}

func (r *Repository) hooksEditorPath(ctx context.Context) (string, error) {
	data, err := r.readLimited(ctx, "", 64<<10, "rev-parse", "--path-format=absolute", "--git-path", "hooks")
	if err != nil {
		return "", err
	}
	path := strings.TrimSuffix(string(data), "\n")
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("Git did not return an absolute hooks directory")
	}
	return filepath.Clean(path), nil
}

func (r *Repository) OpenHooksEditor(ctx context.Context) (*HooksEditor, error) {
	if r.WorkTree == "" {
		return nil, fmt.Errorf("editing client-side hooks requires a working copy")
	}
	if err := r.checkIndexCheckout(ctx); err != nil {
		return nil, err
	}
	path, err := r.hooksEditorPath(ctx)
	if err != nil {
		return nil, err
	}
	directory, err := bindEditorDirectory(path)
	if err != nil {
		return nil, err
	}
	return &HooksEditor{repo: r, directory: directory}, nil
}

func (h *HooksEditor) verify(ctx context.Context) error {
	if err := h.repo.checkIndexCheckout(ctx); err != nil {
		return err
	}
	path, err := h.repo.hooksEditorPath(ctx)
	if err != nil {
		return err
	}
	if path != h.directory.path {
		return fmt.Errorf("the configured hooks directory changed since it was opened")
	}
	return nil
}

func (h *HooksEditor) List(ctx context.Context) ([]HookInfo, error) {
	if err := h.verify(ctx); err != nil {
		return nil, err
	}
	root, err := h.directory.open(false)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if root != nil {
		defer root.Close()
	}
	hooks := make([]HookInfo, 0, len(clientHooks))
	for _, name := range clientHooks {
		info := HookInfo{Name: name, Path: filepath.Join(h.directory.path, name)}
		if root != nil {
			file, err := root.Lstat(name)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
			if file != nil {
				if file.Mode()&os.ModeSymlink != 0 {
					// Leave unresolved links inactive. Opening the hook reports the error.
					file, _ = os.Stat(info.Path)
				}
				info.Exists = file != nil && file.Mode().IsRegular()
				info.Executable = info.Exists && runtime.GOOS != "windows" && file.Mode().Perm()&0o111 != 0
			}
		}
		hooks = append(hooks, info)
	}
	return hooks, nil
}

func (h *HooksEditor) Open(ctx context.Context, name string) (*HookFile, error) {
	if !slices.Contains(clientHooks, name) {
		return nil, fmt.Errorf("unknown client-side Git hook %q", name)
	}
	if err := h.verify(ctx); err != nil {
		return nil, err
	}
	path := filepath.Join(h.directory.resolved, name)
	directory, filename := h.directory, name
	linked, err := h.hookLink(name)
	if err != nil {
		return nil, err
	}
	if linked != nil {
		path, err = filepath.EvalSymlinks(path)
		if err != nil {
			return nil, err
		}
		directory, err = bindEditorDirectory(filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		filename = filepath.Base(path)
	}
	editor, err := directory.read(h.repo, filename)
	if err != nil {
		return nil, err
	}
	if linked != nil && editor.before.info == nil {
		return nil, fmt.Errorf("the linked hook target disappeared while opening it")
	}
	hook := &HookFile{panel: h, file: editor, name: name, path: path, link: linked}
	if err := hook.verify(ctx); err != nil {
		return nil, err
	}
	return hook, nil
}

func (e *HookFile) Text() string { return string(e.file.before.data) }

func (e *HookFile) Path() string { return e.path }

func (h *HooksEditor) hookLink(name string) (os.FileInfo, error) {
	root, err := h.directory.open(false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) || err == nil && info.Mode()&os.ModeSymlink == 0 {
		return nil, nil
	}
	return info, err
}

func (e *HookFile) verify(ctx context.Context) error {
	if err := e.panel.verify(ctx); err != nil {
		return err
	}
	link, err := e.panel.hookLink(e.name)
	if err != nil {
		return err
	}
	if !sameFileIdentity(e.link, link) {
		return fmt.Errorf("the hook link changed since it was opened")
	}
	if link != nil {
		actual, err := filepath.EvalSymlinks(filepath.Join(e.panel.directory.resolved, e.name))
		if err != nil || actual != e.path {
			return fmt.Errorf("the linked hook target changed since it was opened")
		}
	}
	return nil
}

func (e *HookFile) Save(ctx context.Context, text string) error {
	return e.file.repo.withRefEdit(ctx, func() error {
		mode := e.file.before.mode
		if runtime.GOOS != "windows" {
			if mode == 0 {
				mode = 0o755
			} else {
				mode |= 0o100
			}
		}
		return e.file.save(ctx, text, mode, func() error { return e.verify(ctx) })
	})
}
