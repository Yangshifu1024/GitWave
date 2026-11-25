package gitcli

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type legacyRemoteFile struct {
	path string
	data string
	info fs.FileInfo
}

func (f legacyRemoteFile) same(other legacyRemoteFile) bool {
	return f.path == other.path && f.data == other.data && sameDiscardMetadata(f.info, other.info)
}

type legacyRemote struct {
	files  []legacyRemoteFile
	config []configValue
}

func (r legacyRemote) hasURL() bool {
	for i := len(r.config) - 1; i >= 0; i-- {
		if r.config[i].key == "url" {
			return r.config[i].value != ""
		}
	}
	return false
}

type legacyRemotes map[string]legacyRemote

func (r legacyRemotes) same(other legacyRemotes) bool {
	if len(r) != len(other) {
		return false
	}
	for name, before := range r {
		after, found := other[name]
		if !found || !slices.Equal(before.config, after.config) ||
			!slices.EqualFunc(before.files, after.files, legacyRemoteFile.same) {
			return false
		}
	}
	return true
}

func (r *Repository) readLegacyRemotes(ctx context.Context) (legacyRemotes, error) {
	common, err := os.OpenRoot(r.CommonDir)
	if err != nil {
		return nil, err
	}
	defer common.Close()
	result := make(legacyRemotes)
	defaultBranch := ""
	for _, group := range []string{"remotes", "branches"} {
		err := r.visitLegacyRemoteFiles(ctx, common, group, func(name string, file legacyRemoteFile) error {
			item := result[name]
			var config []configValue
			if group == "remotes" {
				config = parseLegacyRemotesFile(file)
			} else {
				line, _, _ := strings.Cut(file.data, "\n")
				address, head, given := strings.Cut(trimLegacyRemoteSpace(line), "#")
				if address == "" {
					return nil
				}
				if !given && !item.hasURL() {
					if defaultBranch == "" {
						data, err := r.readLimited(ctx, "", 64<<10, "var", "GIT_DEFAULT_BRANCH")
						if err != nil {
							return err
						}
						defaultBranch = strings.TrimSuffix(string(data), "\n")
					}
					head = defaultBranch
				}
				config = []configValue{
					{file.path, "url", address},
					{file.path, "fetch", "refs/heads/" + head + ":refs/heads/" + name},
					{file.path, "push", "HEAD:refs/heads/" + head},
				}
			}
			if len(config) == 0 {
				return nil
			}
			item.files = append(item.files, file)
			if !item.hasURL() {
				item.config = append(item.config, config...)
			}
			result[name] = item
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	for name, item := range result {
		if !item.hasURL() {
			delete(result, name)
		}
	}
	return result, nil
}

func (r *Repository) visitLegacyRemoteFiles(ctx context.Context, common *os.Root, group string,
	visit func(string, legacyRemoteFile) error) error {
	info, err := common.Lstat(group)
	if errors.Is(err, fs.ErrNotExist) || err == nil && !info.IsDir() {
		return nil
	}
	if err != nil {
		return err
	}
	dir, err := common.OpenRoot(group)
	if err != nil {
		return err
	}
	defer dir.Close()
	current, err := dir.Stat(".")
	if err != nil || !os.SameFile(current, info) {
		return fmt.Errorf("Git's %s directory changed while opening it", group)
	}
	folder, err := dir.Open(".")
	if err != nil {
		return err
	}
	entries, err := folder.ReadDir(-1)
	if err := errors.Join(err, folder.Close()); err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		if !entry.Type().IsRegular() {
			continue
		}
		if _, err := r.checkedRefName(ctx, "remotes", name+"/probe"); err != nil {
			continue
		}
		stamp, err := dir.Lstat(name)
		if err != nil {
			return err
		}
		data, _, err := readGitStorageFile(dir, name, 1<<20)
		if err != nil {
			return err
		}
		current, err := dir.Lstat(name)
		if err != nil || !sameDiscardMetadata(stamp, current) || strings.ContainsRune(string(data), 0) {
			return fmt.Errorf("invalid or changed Git legacy remote file %q", name)
		}
		if err := visit(name, legacyRemoteFile{filepath.Join(group, name), string(data), stamp}); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func parseLegacyRemotesFile(file legacyRemoteFile) []configValue {
	var config []configValue
	for line := range strings.SplitSeq(file.data, "\n") {
		key, value, _ := strings.Cut(line, ":")
		value = trimLegacyRemoteSpace(value)
		switch key {
		case "URL":
			config = append(config, configValue{file.path, "url", value})
		case "Pull":
			config = append(config, configValue{file.path, "fetch", value})
		case "Push":
			config = append(config, configValue{file.path, "push", value})
		}
	}
	return config
}

func trimLegacyRemoteSpace(value string) string {
	return strings.Trim(value, " \t\r\n\v\f")
}

type legacyRemoteRetirement struct {
	root  *os.Root
	dir   string
	moved []legacyRemoteFile
}

func (m *legacyRemoteRetirement) park(files []legacyRemoteFile) error {
	if len(files) == 0 {
		return nil
	}
	m.dir = ".gitwave-legacy-remote-" + rand.Text()
	if err := m.root.Mkdir(m.dir, 0o700); err != nil {
		m.dir = ""
		return err
	}
	for _, file := range files {
		data, _, err := readGitStorageFile(m.root, file.path, 1<<20)
		if err != nil || string(data) != file.data {
			return fmt.Errorf("Git legacy remote file %q changed before publication", file.path)
		}
		info, err := m.root.Lstat(file.path)
		if err != nil || !sameDiscardMetadata(info, file.info) {
			return fmt.Errorf("Git legacy remote file %q was replaced before publication", file.path)
		}
		backup := m.backupPath(file)
		if err := m.root.Rename(file.path, backup); err != nil {
			return err
		}
		m.moved = append(m.moved, file)
		info, err = m.root.Lstat(backup)
		if err != nil || !sameDiscardMetadata(info, file.info) {
			return fmt.Errorf("Git legacy remote file %q was replaced during publication", file.path)
		}
	}
	return nil
}

func (m *legacyRemoteRetirement) backupPath(file legacyRemoteFile) string {
	return filepath.Join(m.dir, filepath.Base(filepath.Dir(file.path)))
}

func (m *legacyRemoteRetirement) finish(operationErr error) error {
	if m.dir == "" {
		return operationErr
	}
	var cleanupErr error
	for _, file := range m.moved {
		backup := m.backupPath(file)
		if operationErr != nil {
			if restoreErr := m.root.Link(backup, file.path); restoreErr != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("could not restore %q, preserved at %s: %w",
					file.path, filepath.Join(m.root.Name(), backup), restoreErr))
				continue
			}
		}
		if removeErr := m.root.Remove(backup); removeErr != nil {
			cleanupErr = errors.Join(cleanupErr, removeErr)
		}
	}
	if cleanupErr == nil {
		cleanupErr = m.root.Remove(m.dir)
	}
	return errors.Join(operationErr, cleanupErr)
}
