package gitcli

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type SubmoduleInfo struct {
	Name, Path, URL, Head, IndexOID string
	Initialized, InSync             bool
	registeredURL                   string
	config                          [sha256.Size]byte
}

const submoduleConfigPattern = "^submodule[.].*[.][^.]+$"
const gitmodulesLimit = 4 << 20

func (r *Repository) Submodules(ctx context.Context) ([]SubmoduleInfo, error) {
	if r.WorkTree == "" {
		return nil, nil
	}
	root, err := os.OpenRoot(r.WorkTree)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	data, _, err := readGitStorageFile(root, ".gitmodules", gitmodulesLimit)
	if errors.Is(err, fs.ErrNotExist) {
		oid, lookupErr := r.trackedGitmodules(ctx)
		if lookupErr != nil || oid == "" {
			return nil, lookupErr
		}
		data, err = r.readLimited(ctx, "", gitmodulesLimit, "cat-file", "blob", oid)
	}
	if err != nil {
		return nil, err
	}
	items, err := r.parseSubmoduleEntries(ctx, data)
	if err != nil || len(items) == 0 {
		return nil, err
	}
	gitlinks, err := r.indexGitlinks(ctx, false)
	if err != nil {
		return nil, err
	}
	effective, err := r.readConfigValues(ctx, submoduleConfigPattern)
	if err != nil {
		return nil, err
	}
	byName := make(map[string][]configValue)
	for _, entry := range effective {
		name, _, ok := submoduleConfigKey(entry.key)
		if ok {
			byName[name] = append(byName[name], entry)
		}
	}
	for i := range items {
		item := &items[i]
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		config := sha256.New()
		for _, entry := range byName[item.Name] {
			config.Write([]byte(entry.origin + "\x00" + entry.key + "\x00" + entry.value + "\x00"))
			if entry.key == "submodule."+item.Name+".url" {
				item.registeredURL, item.Initialized = entry.value, true
			}
		}
		copy(item.config[:], config.Sum(nil))
		item.IndexOID = gitlinks[item.Path]
		child, err := r.openSubmodule(ctx, root, item.Path)
		if err != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if child != nil {
			item.Head, err = child.optionalRef(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
			if err != nil {
				return nil, err
			}
			item.InSync = item.Head != "" && item.Head == item.IndexOID
		}
	}
	return items, nil
}

func (r *Repository) trackedGitmodules(ctx context.Context) (string, error) {
	for _, ref := range []string{":.gitmodules", "HEAD:.gitmodules"} {
		oid, err := r.optionalRef(ctx, "rev-parse", "--verify", "--quiet", ref)
		if err != nil || oid != "" {
			return oid, err
		}
	}
	return "", nil
}

func submoduleConfigKey(key string) (name, field string, ok bool) {
	key, ok = strings.CutPrefix(key, "submodule.")
	if !ok {
		return "", "", false
	}
	end := strings.LastIndexByte(key, '.')
	if end <= 0 {
		return "", "", false
	}
	return key[:end], key[end+1:], true
}

func (r *Repository) parseSubmoduleEntries(ctx context.Context, data []byte) ([]SubmoduleInfo, error) {
	result, err := r.readLimited(ctx, string(data), gitmodulesLimit,
		"config", "--file", "-", "--no-includes", "--null", "--get-regexp", "^submodule[.].*[.](path|url)$")
	if gitExitCode(err, 1) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("could not read .gitmodules: %w", err)
	}
	modules := make(map[string]SubmoduleInfo)
	for record := range strings.SplitSeq(strings.TrimSuffix(string(result), "\x00"), "\x00") {
		key, value, _ := strings.Cut(record, "\n")
		name, field, ok := submoduleConfigKey(key)
		if !ok {
			return nil, fmt.Errorf("invalid key in .gitmodules")
		}
		item := modules[name]
		item.Name = name
		if field == "path" {
			item.Path = value
		} else {
			item.URL = value
		}
		modules[name] = item
	}
	items := make([]SubmoduleInfo, 0, len(modules))
	paths := make(map[string]bool)
	for _, item := range modules {
		if item.Path == "" {
			continue
		}
		if !validIndexPath(item.Path) || paths[item.Path] || !validIndexPath(item.Name) {
			return nil, fmt.Errorf("invalid or repeated submodule name or path in .gitmodules: %q", item.Path)
		}
		paths[item.Path] = true
		items = append(items, item)
	}
	slices.SortFunc(items, func(a, b SubmoduleInfo) int { return strings.Compare(a.Name, b.Name) })
	return items, nil
}

func (r *Repository) indexGitlinks(ctx context.Context, visibleOnly bool) (map[string]string, error) {
	args := []string{"ls-files", "--stage", "-z", "-v"}
	if visibleOnly {
		args = append(args, "--sparse")
	}
	cmd := r.command(ctx, "", append(args, "--")...)
	output, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr gitDiagnostic
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	// Status scans the index on refresh. Keep only gitlinks from the stream.
	reader := bufio.NewReaderSize(output, 64<<10)
	items := make(map[string]string)
	var readErr error
	for {
		record, err := reader.ReadSlice(0)
		if err != nil {
			if err != io.EOF || len(record) != 0 {
				readErr = fmt.Errorf("incomplete Git index entry: %w", err)
				_ = cmd.Process.Kill()
			}
			break
		}
		const gitlinkMode = "160000 "
		if len(record) <= 2+len(gitlinkMode) || (visibleOnly && record[0] != 'H') {
			// Git status skips assume-unchanged and skip-worktree entries.
			continue
		}
		record = record[2:]
		if bytes.HasPrefix(record, []byte(gitlinkMode)) {
			oid, path, _ := strings.Cut(string(record[len(gitlinkMode):len(record)-1]), "\t")
			if tip, stage, _ := strings.Cut(oid, " "); stage == "0" && validOID(tip) {
				items[path] = tip
			}
		}
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if readErr != nil {
		return nil, readErr
	}
	if waitErr != nil {
		return nil, fmt.Errorf("git ls-files: %w: %s", waitErr, strings.TrimSpace(stderr.buffer.String()))
	}
	return items, nil
}

func (r *Repository) openSubmodule(ctx context.Context, root *os.Root, path string) (*Repository, error) {
	local, err := filepath.Localize(path)
	if err != nil || !validIndexPath(path) {
		return nil, fmt.Errorf("invalid submodule path %q", path)
	}
	parent, err := openWorktreeParent(root, local, false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if parent != root {
		defer parent.Close()
	}
	name := filepath.Base(local)
	info, err := parent.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("the submodule path is not a directory: %q", path)
	}
	dir, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	opened, err := dir.Stat(".")
	if err != nil || !os.SameFile(opened, info) {
		return nil, fmt.Errorf("the submodule directory changed while opening it")
	}
	marker, err := dir.Lstat(".git")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !marker.IsDir() && !marker.Mode().IsRegular() {
		return nil, fmt.Errorf("invalid submodule checkout at %q", path)
	}
	child, err := OpenRoot(ctx, filepath.Join(r.WorkTree, local))
	if err != nil {
		return nil, err
	}
	if child.WorkTree == "" || child.CommonDir == r.CommonDir {
		return nil, fmt.Errorf("invalid submodule checkout at %q", path)
	}
	if err := verifyStageSource(root, parent, local, info); err != nil {
		return nil, err
	}
	return child, nil
}

func (r *Repository) verifySubmodule(ctx context.Context, expected SubmoduleInfo, registrationOnly bool) error {
	items, err := r.Submodules(ctx)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(items, func(current SubmoduleInfo) bool {
		if registrationOnly {
			current.Head, current.InSync = expected.Head, expected.InSync
		}
		return current == expected
	}) {
		return fmt.Errorf("the submodule changed since it was selected: %q", expected.Name)
	}
	paths := make([]string, 0, len(items))
	for _, item := range items {
		paths = append(paths, item.Path)
	}
	root, err := os.OpenRoot(r.WorkTree)
	if err != nil {
		return err
	}
	defer root.Close()
	return checkCheckoutAliases(root, paths)
}
