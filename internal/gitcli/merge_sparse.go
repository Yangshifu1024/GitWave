package gitcli

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type sparseMergeFile struct {
	path  string
	stamp discardStamp
}

// Conflicts outside the sparse selection must be editable, even when
// the regular checkout of the merge tree left those paths on disk absent.
func (m *mergeCheckout) materializeSparse(ctx context.Context, git *workingGit) (*os.Root, []sparseMergeFile, error) {
	if len(m.paths) == 0 {
		return nil, nil, nil
	}
	var sparse bool
	for _, value := range git.settings {
		if value == "core.sparsecheckout=true" {
			sparse = true
		}
	}
	if !sparse {
		return nil, nil, nil
	}
	output, err := git.read(ctx, 64<<20, "ls-files", "--stage", "-t", "-z", "--")
	if err != nil {
		return nil, nil, err
	}
	paths := make(map[string]bool, len(m.paths))
	for _, path := range m.paths {
		paths[path] = true
	}
	var selected []string
	modes := make(map[string]string)
	for record := range strings.SplitSeq(string(output), "\x00") {
		if !strings.HasPrefix(record, "S ") {
			continue
		}
		entry, path, _ := strings.Cut(record[2:], "\t")
		if !paths[path] {
			continue
		}
		mode, _, _ := strings.Cut(entry, " ")
		if mode != "160000" {
			selected = append(selected, path)
			modes[path] = mode
		}
	}
	if len(selected) == 0 {
		return nil, nil, nil
	}
	shadow := *git
	shadow.workTree = filepath.Join(git.dir, "merge-sparse")
	if err := os.Mkdir(shadow.workTree, 0o700); err != nil {
		return nil, nil, err
	}
	rendered, symlinks, err := shadow.extractIndexFiles(ctx, selected)
	if err != nil {
		return nil, nil, err
	}
	from, err := os.OpenRoot(shadow.workTree)
	if err != nil {
		return nil, nil, err
	}
	defer from.Close()
	root, err := os.OpenRoot(git.repo.WorkTree)
	if err != nil {
		return nil, nil, err
	}
	var files []sparseMergeFile
	for _, path := range selected {
		local := filepath.FromSlash(path)
		stamp, err := publishSparseMergeFile(ctx, from, rendered[path], modes[path], symlinks, root, local)
		if err != nil {
			return root, files, fmt.Errorf("could not check out conflicted path %q: %w", path, err)
		}
		files = append(files, sparseMergeFile{path: local, stamp: stamp})
	}
	return root, files, nil
}

func publishSparseMergeFile(ctx context.Context, source *os.Root, rendered, mode string, symlinks bool, root *os.Root, path string) (discardStamp, error) {
	parent, err := openWorktreeParent(root, path, true)
	if err != nil {
		return discardStamp{}, err
	}
	if parent != root {
		defer parent.Close()
	}
	temp := ".gitwave-merge-" + rand.Text()
	defer parent.Remove(temp)
	if err := copyDiscardReplacement(ctx, source, rendered, parent, temp, mode, symlinks); err != nil {
		return discardStamp{}, err
	}
	stamp, err := readDiscardStamp(ctx, parent, temp)
	if err != nil {
		return discardStamp{}, err
	}
	return stamp, moveDiscardFile(parent, temp, parent, filepath.Base(path))
}

func removeSparseMergeFiles(ctx context.Context, root *os.Root, files []sparseMergeFile) error {
	var recoveryErr error
	for _, file := range files {
		recoveryErr = errors.Join(recoveryErr, removeSparseMergeFile(ctx, root, file))
	}
	return recoveryErr
}

func removeSparseMergeFile(ctx context.Context, root *os.Root, file sparseMergeFile) error {
	parent, err := openWorktreeParent(root, file.path, false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if parent != root {
		defer parent.Close()
	}
	name := filepath.Base(file.path)
	current, err := readDiscardStamp(ctx, parent, name)
	if err != nil || current.info == nil {
		return err
	}
	changed := fmt.Errorf("kept a concurrently modified conflict file %q", file.path)
	if !current.matches(file.stamp) {
		return changed
	}
	temp := ".gitwave-merge-recovery-" + rand.Text()
	if err := moveDiscardFile(parent, name, parent, temp); err != nil {
		return err
	}
	moved, err := readDiscardStamp(ctx, parent, temp)
	if err == nil && moved.matches(file.stamp) {
		return parent.Remove(temp)
	}
	if restoreErr := moveDiscardFile(parent, temp, parent, name); restoreErr != nil {
		saved := filepath.Join(filepath.Dir(file.path), temp)
		return errors.Join(err, fmt.Errorf("concurrent conflict file saved at %q: %w", saved, restoreErr))
	}
	return errors.Join(err, changed)
}
