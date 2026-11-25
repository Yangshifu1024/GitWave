package gitcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
)

type mergeCheckout struct {
	message   string
	tree      string
	indexInfo string
	paths     []string
	paused    bool
}

func (m *mergeCheckout) publish(ctx context.Context, lock *checkoutIndexLock, git *workingGit, head string) (err error) {
	root, files, err := m.materializeSparse(ctx, git)
	if root != nil {
		defer root.Close()
		defer func() {
			if err != nil {
				err = errors.Join(err, removeSparseMergeFiles(ctx, root, files))
			}
		}()
	}
	if err != nil {
		return err
	}
	// Keep the conflict-marker checkout index available for rollback if
	// preparing the unmerged index or publishing merge state fails.
	if m.indexInfo != "" {
		cleanIndex := git.index
		git.index += ".conflicts"
		defer func() { git.index = cleanIndex }()
		if err := copyGitMetadata(cleanIndex, git.index, 512<<20); err != nil {
			return err
		}
		if _, err := git.read(ctx, 64<<10, "update-index", "--clear-resolve-undo"); err != nil {
			return err
		}
		if _, err := git.run(ctx, m.indexInfo, 64<<10, "update-index", "-z", "--index-info"); err != nil {
			return err
		}
	}
	if err := writeLockedIndex(lock.root, lock.file, git.index); err != nil {
		return err
	}
	metadata := []struct{ name, content string }{
		{"MERGE_MSG", m.message + "\n"},
		{"MERGE_HEAD", head + "\n"},
	}
	var created []os.FileInfo
	var publishErr error
	for _, item := range metadata {
		info, err := createMergeStateFile(lock.root, item.name, item.content)
		if info != nil {
			created = append(created, info)
		}
		if err != nil {
			publishErr = fmt.Errorf("write Git %s: %w", item.name, err)
			break
		}
	}
	if publishErr == nil {
		publishErr = installLockedIndex(lock.root, lock.info)
	}
	if publishErr != nil {
		for i, info := range created {
			name := metadata[i].name
			if current, err := lock.root.Lstat(name); err == nil && os.SameFile(current, info) {
				publishErr = errors.Join(publishErr, lock.root.Remove(name))
			}
		}
	}
	return publishErr
}

func createMergeStateFile(root *os.Root, name, content string) (os.FileInfo, error) {
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		return nil, err
	}
	info, statErr := file.Stat()
	_, writeErr := io.WriteString(file, content)
	return info, errors.Join(statErr, writeErr, file.Sync(), file.Close())
}
