package gitcli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"unicode/utf8"
)

const repositoryEditorLimit = 100<<20 - 1

type editorDirectory struct {
	path, resolved, ancestor string
	identity                 os.FileInfo
}

func bindEditorDirectory(path string) (editorDirectory, error) {
	anchor, physical, err := resolveExistingPath(path)
	if err != nil {
		return editorDirectory{}, err
	}
	info, err := os.Stat(anchor)
	if err != nil {
		return editorDirectory{}, err
	}
	if !info.IsDir() {
		return editorDirectory{}, fmt.Errorf("the editor directory is not a directory")
	}
	return editorDirectory{path: path, resolved: physical, ancestor: anchor, identity: info}, nil
}

func (d editorDirectory) open(create bool) (*os.Root, error) {
	_, physical, err := resolveExistingPath(d.path)
	if err != nil {
		return nil, err
	}
	if physical != d.resolved {
		return nil, fmt.Errorf("the editor directory changed since it was opened")
	}
	root, err := os.OpenRoot(d.ancestor)
	if err != nil {
		return nil, err
	}
	info, err := root.Stat(".")
	if err != nil || !sameFileIdentity(info, d.identity) {
		root.Close()
		return nil, fmt.Errorf("the editor directory was replaced")
	}
	relative, err := filepath.Rel(d.ancestor, physical)
	if err != nil || !filepath.IsLocal(relative) {
		root.Close()
		return nil, fmt.Errorf("the editor directory is outside the selected path")
	}
	parent, err := openWorktreeParent(root, filepath.Join(relative, "file"), create)
	if parent != root {
		root.Close()
	}
	return parent, err
}

type editorSnapshot struct {
	gitStorageSnapshot
	info os.FileInfo
}

func readEditorSnapshot(root *os.Root, name string) (editorSnapshot, error) {
	var snapshot editorSnapshot
	before, err := root.Lstat(name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return snapshot, err
	}
	snapshot.gitStorageSnapshot, err = readGitStorageSnapshot(root, name, repositoryEditorLimit)
	if err != nil {
		return snapshot, err
	}
	after, err := root.Lstat(name)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return snapshot, err
	}
	if !sameFileIdentity(before, after) || (after == nil) != (snapshot.mode == 0) ||
		after != nil && !sameDiscardMetadata(before, after) {
		return snapshot, fmt.Errorf("%s changed while it was read", name)
	}
	snapshot.info = after
	return snapshot, nil
}

type repositoryEditor struct {
	repo      *Repository
	directory editorDirectory
	name      string
	before    editorSnapshot
}

func (d editorDirectory) read(repo *Repository, name string) (repositoryEditor, error) {
	editor := repositoryEditor{repo: repo, directory: d, name: name}
	root, err := d.open(false)
	if errors.Is(err, fs.ErrNotExist) {
		return editor, nil
	}
	if err != nil {
		return editor, err
	}
	defer root.Close()
	editor.before, err = readEditorSnapshot(root, name)
	if err != nil {
		return editor, err
	}
	if !utf8.Valid(editor.before.data) {
		return editor, fmt.Errorf("%s is not UTF-8 text", name)
	}
	return editor, nil
}

func (e repositoryEditor) save(ctx context.Context, text string, mode fs.FileMode, verify func() error) error {
	if len(text) > repositoryEditorLimit || !utf8.ValidString(text) {
		return fmt.Errorf("%s must be valid UTF-8 text below 100 MiB", e.name)
	}
	if err := verify(); err != nil {
		return err
	}
	root, err := e.directory.open(true)
	if err != nil {
		return err
	}
	defer root.Close()
	lock, err := lockGitStorageFile(root, e.name)
	if err != nil {
		return err
	}
	defer lock.close()
	checkSnapshot := func() error {
		current, err := readEditorSnapshot(root, e.name)
		if err != nil {
			return err
		}
		if !sameFileIdentity(e.before.info, current.info) || !e.before.gitStorageSnapshot.same(current.gitStorageSnapshot) {
			return fmt.Errorf("%s changed since it was opened", e.name)
		}
		return nil
	}
	if err := checkSnapshot(); err != nil {
		return err
	}
	if err := lock.prepare([]byte(text), mode); err != nil {
		return err
	}
	if err := verify(); err != nil {
		return err
	}
	fresh, err := e.directory.open(false)
	if err != nil {
		return err
	}
	defer fresh.Close()
	oldInfo, err := root.Stat(".")
	if err != nil {
		return err
	}
	newInfo, err := fresh.Stat(".")
	if err != nil || !sameFileIdentity(oldInfo, newInfo) {
		return fmt.Errorf("the editor directory was replaced before saving %s", e.name)
	}
	if err := checkSnapshot(); err != nil {
		return err
	}
	prepared, err := readGitStorageSnapshot(lock.parent, lock.name+".lock", repositoryEditorLimit)
	if err != nil || !lock.owns() || string(prepared.data) != text ||
		mode != 0 && prepared.mode.Perm() != mode.Perm() {
		return fmt.Errorf("the prepared edit for %s changed before publication", e.name)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return lock.publish()
}
