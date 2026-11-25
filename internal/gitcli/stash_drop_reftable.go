package gitcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

const reftableListPath = "reftable/tables.list"

// Reftable files are immutable. Let Git perform the log rewrite on a private
// copy of the table stack. Publish the new tables before atomically switching
// the manifest, using Git's own lock to serialize this with other writers.
func (r *Repository) dropReftableStash(ctx context.Context, common *os.Root, entry StashEntry) error {
	lock, err := lockGitStorageFile(common, reftableListPath)
	if err != nil {
		return err
	}
	defer lock.close()
	before, err := r.checkedStashEntry(ctx, entry)
	if err != nil {
		return err
	}
	manifest, manifestMode, err := readGitStorageFile(lock.parent, lock.name, 8<<20)
	if err != nil {
		return err
	}
	tables, err := parseReftableList(manifest)
	if err != nil {
		return err
	}
	objectFormat := "sha1"
	if len(entry.OID) == 64 {
		objectFormat = "sha256"
	}
	temp, err := os.MkdirTemp("", "gitwave-stash-drop-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	git := &workingGit{repo: r, dir: filepath.Join(temp, ".git"), workTree: temp,
		index: filepath.Join(temp, ".git", "index")}
	init := gitCommand(ctx, r.git, temp, nil, "", "init", "--quiet", "--template=",
		"--ref-format=reftable", "--object-format="+objectFormat, "--", temp)
	init.Env = append(init.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_SYSTEM="+os.DevNull, "GIT_CONFIG_GLOBAL="+os.DevNull)
	if _, err := readGitOutput(ctx, init, 4096, "init"); err != nil {
		return err
	}
	git.settings = []string{"-c", "core.hooksPath=" + git.dir, "-c", "reftable.autoCompact=false"}
	shared, err := r.optionalRef(ctx, "config", "--get", "core.sharedRepository")
	if err != nil {
		return err
	}
	if shared != "" {
		git.settings = append(git.settings, "-c", "core.sharedRepository="+shared)
	}
	scratch, err := os.OpenRoot(filepath.Join(git.dir, "reftable"))
	if err != nil {
		return err
	}
	defer scratch.Close()
	original := make(map[string]os.FileInfo, len(tables))
	for _, name := range tables {
		sourceInfo, _, err := copyReftableTable(ctx, lock.parent, scratch, name)
		if err != nil {
			return err
		}
		original[name] = sourceInfo
	}
	if err := scratch.WriteFile("tables.list", manifest, 0o600); err != nil {
		return err
	}
	selector := "stash@{" + strconv.Itoa(entry.Index) + "}"
	if _, err := git.read(ctx, 64<<10, "stash", "drop", "--quiet", selector); err != nil {
		return err
	}
	after, err := readStashLog(ctx, git.read)
	if err != nil {
		return err
	}
	expected := slices.DeleteFunc(slices.Clone(before), func(item StashEntry) bool { return item.Index == entry.Index })
	if !slices.EqualFunc(expected, after, StashEntry.sameRecord) {
		return fmt.Errorf("Git rewrote an unexpected set of stash entries")
	}
	next, _, err := readGitStorageFile(scratch, "tables.list", 8<<20)
	if err != nil {
		return err
	}
	nextTables, err := parseReftableList(next)
	if err != nil {
		return err
	}
	newTables := make(map[string]os.FileInfo)
	published := false
	defer func() {
		if !published {
			removeReftableTables(lock.parent, newTables)
		}
	}()
	for _, name := range nextTables {
		if _, kept := original[name]; kept {
			delete(original, name)
			continue
		}
		_, createdInfo, err := copyReftableTable(ctx, scratch, lock.parent, name)
		if err != nil {
			return err
		}
		newTables[name] = createdInfo
	}
	if err := lock.prepare(next, manifestMode); err != nil {
		return err
	}
	if err := r.verifyGitStorageLocks(ctx, common, lock); err != nil {
		return err
	}
	if err := lock.publish(); err != nil {
		return err
	}
	published = true
	removeReftableTables(lock.parent, original)
	return nil
}

func parseReftableList(manifest []byte) ([]string, error) {
	var result []string
	seen := make(map[string]bool)
	if len(manifest) == 0 {
		return result, nil
	}
	if manifest[len(manifest)-1] != '\n' {
		return nil, fmt.Errorf("invalid Git reftable list")
	}
	for name := range strings.SplitSeq(strings.TrimSuffix(string(manifest), "\n"), "\n") {
		if seen[name] || !validIndexPath(name) || strings.ContainsAny(name, "/\\") || strings.HasPrefix(name, ".") ||
			(!strings.HasSuffix(name, ".ref") && !strings.HasSuffix(name, ".log")) {
			return nil, fmt.Errorf("invalid Git reftable name %q", name)
		}
		seen[name] = true
		result = append(result, name)
	}
	return result, nil
}

func copyReftableTable(ctx context.Context, from, to *os.Root, name string) (srcInfo, dstInfo os.FileInfo, err error) {
	srcInfo, err = from.Lstat(name)
	if err != nil {
		return nil, nil, err
	}
	if !srcInfo.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("invalid Git reftable %q", name)
	}
	reader, err := from.OpenFile(name, os.O_RDONLY|workFileOpenFlags, 0)
	if err != nil {
		return nil, nil, err
	}
	defer reader.Close()
	info, err := reader.Stat()
	if err != nil || !sameDiscardMetadata(info, srcInfo) {
		return nil, nil, fmt.Errorf("Git reftable %q changed while opening it", name)
	}
	writer, err := to.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, srcInfo.Mode().Perm())
	if err != nil {
		return nil, nil, err
	}
	created, statErr := writer.Stat()
	defer func() {
		if err != nil && created != nil {
			removeReftableTables(to, map[string]os.FileInfo{name: created})
		}
	}()
	if statErr != nil {
		return nil, nil, errors.Join(statErr, writer.Close())
	}
	if err := writer.Chmod(srcInfo.Mode().Perm()); err != nil {
		return nil, nil, errors.Join(err, writer.Close())
	}
	size, copyErr := io.Copy(writer, stageReader{ctx, reader})
	if copyErr != nil || size != srcInfo.Size() {
		return nil, nil, errors.Join(copyErr, writer.Close(), fmt.Errorf("Git reftable %q was not copied completely", name))
	}
	syncErr, closeErr := writer.Sync(), writer.Close()
	return srcInfo, created, errors.Join(syncErr, closeErr)
}

func removeReftableTables(root *os.Root, tables map[string]os.FileInfo) {
	for name, info := range tables {
		current, err := root.Lstat(name)
		if err == nil && os.SameFile(info, current) {
			_ = root.Remove(name)
		}
	}
}
