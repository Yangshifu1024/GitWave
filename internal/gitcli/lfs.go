package gitcli

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"slices"
	"strings"
)

const lfsAttributes = ".gitattributes"
const lfsAttributesLimit = 100<<20 - 1 // Git ignores files at and above 100 MiB.

var lfsFilterKeys = []string{"filter.lfs.clean", "filter.lfs.smudge", "filter.lfs.process", "filter.lfs.required"}

const lfsFormatKey = "lfs.repositoryformatversion"

var lfsConfigKeys = slices.Concat(lfsFilterKeys, []string{lfsFormatKey})

type LFSStatus struct {
	Available, Installed bool
	Patterns             []string
}

func (r *Repository) LFSStatus(ctx context.Context) (LFSStatus, error) {
	if r.WorkTree == "" {
		return LFSStatus{}, fmt.Errorf("Git LFS requires a working copy")
	}
	status := LFSStatus{}
	if executable, err := r.lfsExecutable(ctx); err == nil {
		version, err := r.runLocalLFS(ctx, executable, os.TempDir(), "version")
		status.Available = err == nil && strings.HasPrefix(string(version), "git-lfs/")
	}
	if err := ctx.Err(); err != nil {
		return status, err
	}
	filters, err := r.lfsConfig(ctx)
	if err != nil {
		return status, err
	}
	status.Installed = true
	for _, key := range lfsFilterKeys {
		local, worktree := filters.local[key], filters.worktree[key]
		status.Installed = status.Installed && len(local) > 0 && local[len(local)-1] != "" &&
			(len(worktree) == 0 || worktree[len(worktree)-1] == local[len(local)-1])
	}
	if status.Installed {
		value, err := r.readLimited(ctx, "", 128, "config", "--type=bool", "--get", "filter.lfs.required")
		status.Installed = err == nil && strings.TrimSpace(string(value)) == "true"
	}
	root, err := os.OpenRoot(r.WorkTree)
	if err != nil {
		return status, err
	}
	defer root.Close()
	attributes, err := r.readLFSAttributes(ctx, root)
	if err == nil {
		status.Patterns = lfsPatterns(string(attributes.data))
	}
	return status, err
}

type lfsConfig struct {
	local, worktree map[string][]string
}

func (r *Repository) lfsConfig(ctx context.Context) (lfsConfig, error) {
	data, err := r.readLimited(ctx, "", 64<<10, "config", "--includes", "--show-scope", "--null",
		"--get-regexp", `^(filter\.lfs\.(clean|smudge|process|required)|lfs\.repositoryformatversion)$`)
	if gitExitCode(err, 1) {
		return lfsConfig{}, nil
	}
	if err != nil {
		return lfsConfig{}, err
	}
	filters := lfsConfig{local: make(map[string][]string), worktree: make(map[string][]string)}
	fields := strings.Split(string(data), "\x00")
	if fields[len(fields)-1] != "" || len(fields)%2 != 1 {
		return lfsConfig{}, fmt.Errorf("Git returned malformed LFS configuration")
	}
	for i := 0; i < len(fields)-1; i += 2 {
		key, value, hasValue := strings.Cut(fields[i+1], "\n")
		if !slices.Contains(lfsConfigKeys, key) {
			return lfsConfig{}, fmt.Errorf("Git returned an invalid LFS configuration key")
		}
		if !hasValue && key == "filter.lfs.required" {
			value = "true"
		}
		switch fields[i] {
		case "local":
			filters.local[key] = append(filters.local[key], value)
		case "worktree":
			if key != lfsFormatKey {
				filters.worktree[key] = append(filters.worktree[key], value)
			}
		}
	}
	return filters, nil
}

type lfsAttributesFile struct {
	gitStorageSnapshot
	indexedOID string
}

func (r *Repository) readLFSAttributes(ctx context.Context, root *os.Root) (lfsAttributesFile, error) {
	file, err := readGitStorageSnapshot(root, lfsAttributes, lfsAttributesLimit)
	if err != nil || file.mode != 0 {
		return lfsAttributesFile{gitStorageSnapshot: file}, err
	}
	// Git uses the index's attributes when no worktree file is present.
	staged, err := r.readLimited(ctx, "", 4096, "ls-files", "--stage", "-z", "--", lfsAttributes)
	if err != nil || len(staged) == 0 {
		return lfsAttributesFile{}, err
	}
	meta, path, _ := strings.Cut(string(staged), "\t")
	fields := strings.Fields(meta)
	if path != lfsAttributes+"\x00" || len(fields) != 3 || fields[2] != "0" || !validOID(fields[1]) ||
		(fields[0] != "100644" && fields[0] != "100755") {
		return lfsAttributesFile{}, fmt.Errorf("the index has no regular %s entry", lfsAttributes)
	}
	file.data, err = r.readLimited(ctx, "", lfsAttributesLimit, "cat-file", "blob", fields[1])
	return lfsAttributesFile{file, fields[1]}, err
}

func ValidateLFSPattern(pattern string) error {
	_, err := encodeLFSPattern(pattern)
	return err
}

func (r *Repository) TrackLFS(ctx context.Context, pattern string) error {
	encoded, err := encodeLFSPattern(pattern)
	if err != nil {
		return err
	}
	return r.editLFSAttributes(ctx, func(data string) string {
		if slices.Contains(lfsPatterns(data), pattern) {
			return data
		}
		return appendLFSRule(data, encoded)
	})
}

func (r *Repository) UntrackLFS(ctx context.Context, pattern string) error {
	return r.editLFSAttributes(ctx, func(data string) string { return removeLFSRules(data, pattern) })
}

func (r *Repository) editLFSAttributes(ctx context.Context, edit func(string) string) error {
	if r.WorkTree == "" {
		return fmt.Errorf("Git LFS requires a working copy")
	}
	index, err := r.lockCheckoutIndex(ctx)
	if err != nil {
		return err
	}
	defer index.close()
	root, err := os.OpenRoot(r.WorkTree)
	if err != nil {
		return err
	}
	defer root.Close()
	conflicted, err := r.readLimited(ctx, "", 4096, "ls-files", "--unmerged", "-z", "--", lfsAttributes)
	if err != nil {
		return err
	}
	if len(conflicted) != 0 {
		return fmt.Errorf("resolve %s conflicts before editing LFS patterns", lfsAttributes)
	}
	lock, err := lockGitStorageFile(root, lfsAttributes)
	if err != nil {
		return err
	}
	defer lock.close()
	before, err := r.readLFSAttributes(ctx, root)
	if err != nil {
		return err
	}
	stamp, err := readDiscardStamp(ctx, root, lfsAttributes)
	if err != nil || (stamp.info == nil) != (before.mode == 0) ||
		stamp.info != nil && stamp.sum != sha256.Sum256(before.data) {
		return fmt.Errorf("%s changed while it was read", lfsAttributes)
	}
	data := string(before.data)
	next := edit(data)
	if next == data {
		return nil
	}
	if len(next) > lfsAttributesLimit {
		return fmt.Errorf("%s exceeds Git's size limit", lfsAttributes)
	}
	if err := lock.prepare([]byte(next), before.mode); err != nil {
		return err
	}
	if err := r.checkIndexCheckout(ctx); err != nil {
		return err
	}
	parent, err := lock.parent.Stat(".")
	if err != nil {
		return err
	}
	onDisk, err := os.Stat(r.WorkTree)
	if err != nil || !sameFileIdentity(parent, onDisk) || !index.ownsLock() {
		return fmt.Errorf("the checkout changed while editing LFS patterns")
	}
	current, err := r.readLFSAttributes(ctx, root)
	if err != nil || current.indexedOID != before.indexedOID || !current.same(before.gitStorageSnapshot) {
		return fmt.Errorf("%s changed while editing LFS patterns", lfsAttributes)
	}
	now, err := readDiscardStamp(ctx, root, lfsAttributes)
	if err != nil || !stamp.matches(now) {
		return fmt.Errorf("%s changed while editing LFS patterns", lfsAttributes)
	}
	return lock.publish()
}
