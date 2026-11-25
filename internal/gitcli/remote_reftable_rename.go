package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func (s *remoteConfigEdit) publishReftableRename(common *os.Root, before, after []byte, publishConfig func() error) error {
	r, ctx := s.repo, s.ctx
	proposal := s.reftableRename
	lock, err := lockGitStorageFile(common, reftableListPath)
	if err != nil {
		return err
	}
	defer lock.close()
	manifest, mode, err := readGitStorageFile(lock.parent, lock.name, 8<<20)
	if err != nil {
		return err
	}
	original, err := parseReftableList(manifest)
	if err != nil {
		return err
	}
	originalFiles := make(map[string]os.FileInfo, len(original))
	for _, name := range original {
		info, err := lock.parent.Lstat(name)
		if err != nil {
			return err
		}
		originalFiles[name] = info
	}
	refs, oidSize, lastIndex, err := readReftableRefStack(ctx, lock.parent, original, proposal.from, proposal.to)
	if err != nil {
		return err
	}
	items, err := r.Remotes(ctx)
	if err != nil {
		return err
	}
	slices.SortFunc(refs, func(a, b remoteTrackingRef) int { return strings.Compare(a.name, b.name) })
	if err := s.planRemoteRefMoves(proposal.remote, proposal.from, proposal.to, items, refs); err != nil {
		return err
	}
	if len(s.moves) == 0 {
		return publishConfig()
	}
	if lastIndex == math.MaxUint64 {
		return fmt.Errorf("the Git reference transaction index is exhausted")
	}
	statement, affected := s.trackingRefTransaction()
	if err := r.protectFetchHeads(ctx, affected); err != nil {
		return err
	}
	logs, err := readMovingReftableLogs(ctx, lock.parent, original, s.moves)
	if err != nil {
		return err
	}

	workspace, err := os.MkdirTemp("", "gitwave-remote-reftable-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workspace)
	shadow, err := r.initRemoteRefShadow(ctx, workspace, oidSize)
	if err != nil {
		return err
	}
	private, err := os.OpenRoot(filepath.Join(shadow.dir, "reftable"))
	if err != nil {
		return err
	}
	defer private.Close()

	for _, name := range original {
		info, _, err := copyReftableTable(ctx, lock.parent, private, name)
		if err != nil {
			return err
		}
		if !sameDiscardMetadata(info, originalFiles[name]) {
			return fmt.Errorf("Git reftable %q changed before copying it", name)
		}
	}
	stack := slices.Clone(manifest)
	if err := writeReftableLogCopies(logs, lastIndex+1, oidSize, func(name string, contents []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := private.WriteFile(name, contents, mode.Perm()); err != nil {
			return err
		}
		if err := private.Chmod(name, mode.Perm()); err != nil {
			return err
		}
		stack = append(stack, name...)
		stack = append(stack, '\n')
		return nil
	}); err != nil {
		return err
	}
	if err := private.WriteFile(lock.name, stack, mode.Perm()); err != nil {
		return err
	}

	// Only Git changes the refs. It appends the new operation after the
	// historical log records we copied into this private stack.
	identity, err := r.commitIdentity(ctx)
	if err != nil {
		return err
	}
	settings := []string{"-c", "core.logAllRefUpdates=true"}
	shared, err := r.optionalRef(ctx, "config", "--get", "core.sharedRepository")
	if err != nil {
		return err
	}
	if shared != "" {
		settings = append(settings, "-c", "core.sharedRepository="+shared)
	}
	cmd := shadow.command(ctx, statement, append(settings, "update-ref", "--stdin", "-z", "-m", "remote: update tracking references")...)
	// Git can otherwise compact old, larger log records into its default
	// block size and fail even though the new ref transaction fits.
	cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_SYSTEM="+os.DevNull, "GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_TEST_REFTABLE_AUTOCOMPACTION=0")
	withRefIdentity(cmd, identity)
	if _, err := readGitOutput(ctx, cmd, 64<<10, "update private references"); err != nil {
		return err
	}

	next, _, err := readGitStorageFile(private, lock.name, 8<<20)
	if err != nil {
		return err
	}
	replacement, err := parseReftableList(next)
	if err != nil {
		return err
	}
	added := make(map[string]os.FileInfo)
	installed := false
	defer func() {
		if !installed {
			removeReftableTables(lock.parent, added)
		}
	}()
	for _, name := range replacement {
		if _, found := originalFiles[name]; found {
			continue
		}
		_, info, err := copyReftableTable(ctx, private, lock.parent, name)
		if err != nil {
			return err
		}
		added[name] = info
	}
	if err := lock.prepare(next, mode); err != nil {
		return err
	}
	live, err := OpenRoot(ctx, r.Path())
	if err != nil || !r.SameCheckout(live) {
		return fmt.Errorf("the Git reference store changed before publishing the remote rename")
	}
	if err := r.verifyGitStorageLocks(ctx, common, lock); err != nil {
		return err
	}
	current, _, err := readGitStorageFile(lock.parent, lock.name, 8<<20)
	if err != nil || !bytes.Equal(current, manifest) {
		return fmt.Errorf("the reftable manifest changed before publishing the remote rename")
	}
	for name, expected := range originalFiles {
		info, err := lock.parent.Lstat(name)
		if err != nil || !sameDiscardMetadata(info, expected) {
			return fmt.Errorf("Git reftable %q changed before publishing the remote rename", name)
		}
	}
	if err := r.protectFetchHeads(ctx, affected); err != nil {
		return err
	}
	if err := publishConfig(); err != nil {
		return err
	}
	if err := lock.publish(); err != nil {
		return errors.Join(err, r.restoreRemoteConfig(before, after))
	}
	installed = true
	for _, name := range replacement {
		delete(originalFiles, name)
	}
	removeReftableTables(lock.parent, originalFiles)
	return nil
}

func (r *Repository) initRemoteRefShadow(ctx context.Context, workspace string, oidSize int) (*transportRepo, error) {
	format := "sha1"
	if oidSize == 32 {
		format = "sha256"
	}
	path := filepath.Join(workspace, "git")
	cmd := gitCommand(ctx, r.git, workspace, nil, "",
		"init", "--quiet", "--bare", "--template=", "--ref-format=reftable", "--object-format="+format, "--", path)
	cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_SYSTEM="+os.DevNull, "GIT_CONFIG_GLOBAL="+os.DevNull)
	if _, err := readGitOutput(ctx, cmd, 64<<10, "initialize private reference store"); err != nil {
		return nil, err
	}
	return &transportRepo{repo: r, dir: path, objects: filepath.Join(r.CommonDir, "objects")}, nil
}

func readMovingReftableLogs(ctx context.Context, root *os.Root, tables []string, moves []remoteRefMove) ([]reftableLogCopy, error) {
	destinations := make(map[string]string, len(moves))
	targetNames := make(map[string]bool, len(moves))
	for _, move := range moves {
		destinations[move.from.name] = move.to.name
		targetNames[move.to.name] = true
	}
	type logKey struct {
		name  string
		index uint64
	}
	copies := make(map[logKey]reftableLogCopy)
	targetLogs := make(map[logKey]bool)
	for _, name := range tables {
		err := readReftableLogFile(ctx, root, name, func(record reftableLogRecord) {
			if targetNames[record.name] {
				key := logKey{record.name, record.index}
				if record.raw == nil {
					delete(targetLogs, key)
				} else {
					targetLogs[key] = true
				}
			}
			if dest := destinations[record.name]; dest != "" {
				key := logKey{dest, record.index}
				if record.raw == nil {
					delete(copies, key)
				} else {
					copies[key] = reftableLogCopy{dest, record.index, bytes.Clone(record.raw)}
				}
			}
		})
		if err != nil {
			return nil, err
		}
	}
	for key := range targetLogs {
		return nil, fmt.Errorf("the destination %q already has a reflog", key.name)
	}
	result := make([]reftableLogCopy, 0, len(copies))
	for _, copy := range copies {
		result = append(result, copy)
	}
	return result, nil
}
