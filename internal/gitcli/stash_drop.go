package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const stashLogPath = "logs/refs/stash"

// PopStash only removes an entry after Apply has published a clean result.
// If deletion is blocked after applying, the result still records the apply.
func (r *Repository) PopStash(ctx context.Context, entry StashEntry, shown Status) (StashApplyResult, error) {
	result, err := r.ApplyStash(ctx, entry, shown)
	if err != nil || len(result.Conflicts) != 0 || result.Warning != nil {
		return result, err
	}
	if err := r.DropStash(ctx, entry); err != nil {
		result.Warning = fmt.Errorf("the stash was applied but could not be removed: %w", err)
		return result, nil
	}
	result.Dropped = true
	return result, nil
}

// DropStash verifies the reflog entry while holding the lock used by Git's
// reference backend. Resolving an ordinal and invoking stash drop separately
// could delete a different entry after another Git process updates the log.
func (r *Repository) DropStash(ctx context.Context, entry StashEntry) error {
	if entry.Index < 0 || !validOID(entry.OID) {
		return fmt.Errorf("select a valid Git stash")
	}
	return r.withRefEdit(ctx, func() error {
		format, err := r.readLimited(ctx, "", 4096, "rev-parse", "--show-ref-format")
		if err != nil {
			return err
		}
		common, err := os.OpenRoot(r.CommonDir)
		if err != nil {
			return err
		}
		defer common.Close()
		switch strings.TrimSpace(string(format)) {
		case "files":
			return r.dropFilesStash(ctx, common, entry)
		case "reftable":
			return r.dropReftableStash(ctx, common, entry)
		default:
			return fmt.Errorf("unsupported Git reference format %q", strings.TrimSpace(string(format)))
		}
	})
}

// Git's files backend uses the direct ref lock to serialize reflog expiry and
// appends. For the last entry, packed-refs must be locked as well so a packed
// copy cannot survive deleting the loose tip.
func (r *Repository) dropFilesStash(ctx context.Context, common *os.Root, entry StashEntry) error {
	ref, err := lockGitStorageFile(common, stashRef)
	if err != nil {
		return err
	}
	defer ref.close()
	log, err := lockGitStorageFile(common, stashLogPath)
	if err != nil {
		return err
	}
	defer log.close()
	entries, err := r.checkedStashEntry(ctx, entry)
	if err != nil {
		return err
	}
	tip, err := r.exactRootRef(ctx, stashRef)
	if err != nil {
		return err
	}
	logData, logMode, err := readGitStorageFile(log.parent, log.name, 32<<20)
	if err != nil {
		return err
	}
	nextLog, nextTip, err := rewriteStashLog(logData, tip, entry, entries)
	if err != nil {
		return err
	}
	if entries[0].Index != 0 || entries[0].OID != tip {
		return fmt.Errorf("the stash reference and its reflog disagree")
	}
	var packed *gitStorageLock
	var packedChanged bool
	var looseData []byte
	var looseMode fs.FileMode
	switch {
	case nextTip == "":
		packed, err = lockGitStorageFile(common, "packed-refs")
		if err != nil {
			return err
		}
		defer packed.close()
		packedData, packedMode, readErr := readGitStorageFile(packed.parent, packed.name, 512<<20)
		if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
			return readErr
		}
		out := removePackedStash(packedData)
		packedChanged = !bytes.Equal(out, packedData)
		if packedChanged {
			if err := packed.prepare(out, packedMode); err != nil {
				return err
			}
		}
		looseData, looseMode, err = readGitStorageFile(ref.parent, ref.name, 4096)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if looseData != nil && strings.TrimSpace(string(looseData)) != tip {
			return fmt.Errorf("the loose stash reference changed before deletion")
		}
		if looseData == nil && !packedChanged {
			return fmt.Errorf("the stash reference disappeared before deletion")
		}
	case nextTip != tip:
		if len(entries) < 2 || entries[1].Index != 1 || entries[1].OID != nextTip {
			return fmt.Errorf("the next stash entry is not readable")
		}
		mode, err := looseStashMode(common, ref)
		if err != nil {
			return err
		}
		if err := ref.prepare([]byte(nextTip+"\n"), mode); err != nil {
			return err
		}
	}
	if err := log.prepare(nextLog, logMode); err != nil {
		return err
	}
	if err := r.verifyGitStorageLocks(ctx, common, ref, log, packed); err != nil {
		return err
	}
	// Like Git's files backend, publish the reflog while the ref lock is
	// still held. If publishing the tip fails, restore the original log.
	if err := log.publish(); err != nil {
		return err
	}
	restoreLog := func(cause error) error {
		undo, err := lockGitStorageFile(common, stashLogPath)
		if err == nil {
			defer undo.close()
			if err = undo.prepare(logData, logMode); err == nil {
				err = undo.publish()
			}
		}
		if err != nil {
			return errors.Join(cause, fmt.Errorf("restore the previous stash reflog: %w", err))
		}
		return cause
	}
	switch {
	case nextTip == "":
		if looseData != nil {
			if err := ref.parent.Remove(ref.name); err != nil {
				return restoreLog(err)
			}
		}
		if packedChanged {
			if err := packed.publish(); err != nil {
				err = restoreLog(err)
				if looseData != nil {
					restoreErr := ref.prepare(looseData, looseMode)
					if restoreErr == nil {
						restoreErr = ref.publish()
					}
					err = errors.Join(err, restoreErr)
				}
				return err
			}
		}
		// An empty file is already a valid empty reflog. Removing it is
		// cleanup, and cannot make a completed Drop fail on Windows.
		if info, err := log.parent.Lstat(log.name); err == nil && os.SameFile(info, log.identity) {
			_ = log.parent.Remove(log.name)
		}
	case nextTip != tip:
		if err := ref.publish(); err != nil {
			return restoreLog(err)
		}
	}
	return nil
}

func looseStashMode(common *os.Root, ref *gitStorageLock) (fs.FileMode, error) {
	info, err := ref.parent.Lstat(ref.name)
	if errors.Is(err, fs.ErrNotExist) {
		// A packed-only ref can require a new loose tip. Keep the access
		// permissions of the packed refs it replaces, even under a stricter umask.
		info, err = common.Lstat("packed-refs")
	}
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("invalid Git stash reference files")
	}
	return info.Mode(), nil
}

func rewriteStashLog(data []byte, tip string, entry StashEntry, visible []StashEntry) ([]byte, string, error) {
	records := bytes.Split(data, []byte{'\n'})
	if len(records) < 2 || len(records[len(records)-1]) != 0 {
		return nil, "", fmt.Errorf("invalid Git stash reflog")
	}
	records = records[:len(records)-1]
	// Git's log walker skips malformed or unreadable records. Refuse to
	// interpret an ordinal in raw storage unless both views agree exactly.
	if len(records) != len(visible) {
		return nil, "", fmt.Errorf("the Git stash reflog contains unreadable records")
	}
	selected := len(records) - entry.Index - 1
	oidSize := len(tip)
	if !validOID(tip) || len(entry.OID) != oidSize || selected < 0 {
		return nil, "", fmt.Errorf("the stash list changed before deletion")
	}
	var out bytes.Buffer
	previous := strings.Repeat("0", oidSize)
	for i, record := range records {
		oldEnd, newEnd := oidSize, 2*oidSize+1
		if len(record) <= newEnd || record[oldEnd] != ' ' || record[newEnd] != ' ' ||
			!validOID(string(record[:oldEnd])) || !validOID(string(record[oldEnd+1:newEnd])) {
			return nil, "", fmt.Errorf("invalid Git stash reflog record")
		}
		next := string(record[oldEnd+1 : newEnd])
		index := len(records) - i - 1
		if visible[index].Index != index || visible[index].OID != next {
			return nil, "", fmt.Errorf("the Git stash reflog entries disagree with the stash list")
		}
		if i == len(records)-1 && next != tip {
			return nil, "", fmt.Errorf("the stash reference and its reflog disagree")
		}
		if i == selected {
			if next != entry.OID {
				return nil, "", fmt.Errorf("the stash list changed before deletion")
			}
			continue
		}
		out.WriteString(previous)
		out.Write(record[oldEnd:])
		out.WriteByte('\n')
		previous = next
	}
	if out.Len() == 0 {
		return nil, "", nil
	}
	return out.Bytes(), previous, nil
}

func removePackedStash(data []byte) []byte {
	var out bytes.Buffer
	dropped := false
	for _, line := range bytes.SplitAfter(data, []byte{'\n'}) {
		name := bytes.TrimRight(line, "\r\n")
		oid, ref, refLine := bytes.Cut(name, []byte{' '})
		if refLine && string(ref) == stashRef && validOID(string(oid)) {
			dropped = true
			continue
		}
		if dropped && bytes.HasPrefix(name, []byte{'^'}) {
			dropped = false
			continue
		}
		dropped = false
		out.Write(line)
	}
	return out.Bytes()
}

type gitStorageLock struct {
	parent   *os.Root
	file     *os.File
	identity os.FileInfo
	relative string
	name     string
}

func lockGitStorageFile(root *os.Root, relative string) (*gitStorageLock, error) {
	local := filepath.FromSlash(relative)
	parent, err := openWorktreeParent(root, local, false)
	if err != nil {
		return nil, err
	}
	if parent == root {
		parent, err = root.OpenRoot(".")
		if err != nil {
			return nil, err
		}
	}
	name := filepath.Base(local)
	file, err := parent.OpenFile(name+".lock", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
	if err != nil {
		parent.Close()
		if errors.Is(err, fs.ErrExist) {
			return nil, fmt.Errorf("Git has locked %s for another operation", relative)
		}
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		_ = parent.Remove(name + ".lock")
		parent.Close()
		return nil, err
	}
	return &gitStorageLock{parent: parent, file: file, identity: info, relative: local, name: name}, nil
}

func (l *gitStorageLock) owns() bool {
	info, err := l.parent.Lstat(l.name + ".lock")
	return err == nil && os.SameFile(info, l.identity)
}

func (l *gitStorageLock) prepare(data []byte, mode fs.FileMode) error {
	if mode != 0 {
		if err := l.file.Chmod(mode.Perm()); err != nil {
			return err
		}
	}
	if _, err := l.file.Write(data); err != nil {
		return err
	}
	if err := l.file.Sync(); err != nil {
		return err
	}
	return l.file.Close()
}

func (l *gitStorageLock) publish() error {
	if !l.owns() {
		return fmt.Errorf("the Git lock for %s was replaced", l.relative)
	}
	return l.parent.Rename(l.name+".lock", l.name)
}

func (l *gitStorageLock) close() {
	l.file.Close()
	if l.owns() {
		_ = l.parent.Remove(l.name + ".lock")
	}
	l.parent.Close()
}

func readGitStorageFile(parent *os.Root, name string, limit int64) ([]byte, fs.FileMode, error) {
	info, err := parent.Lstat(name)
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() || info.Size() > limit {
		return nil, 0, fmt.Errorf("invalid or oversized Git file %q", name)
	}
	file, err := parent.OpenFile(name, os.O_RDONLY|workFileOpenFlags, 0)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	fstat, err := file.Stat()
	if err != nil || !sameDiscardMetadata(fstat, info) {
		return nil, 0, fmt.Errorf("Git file %q changed while opening it", name)
	}
	result, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, 0, err
	}
	if int64(len(result)) != info.Size() {
		return nil, 0, fmt.Errorf("Git file %q changed while reading it", name)
	}
	return result, info.Mode(), nil
}

type gitStorageSnapshot struct {
	data []byte
	mode fs.FileMode
}

func readGitStorageSnapshot(root *os.Root, name string, limit int64) (gitStorageSnapshot, error) {
	data, mode, err := readGitStorageFile(root, name, limit)
	if errors.Is(err, fs.ErrNotExist) {
		return gitStorageSnapshot{}, nil
	}
	return gitStorageSnapshot{data, mode}, err
}

func (b gitStorageSnapshot) same(other gitStorageSnapshot) bool {
	return b.mode == other.mode && bytes.Equal(b.data, other.data)
}

func restoreGitStorageFile(root *os.Root, name string, before gitStorageSnapshot, published []byte, limit int64) error {
	lock, err := lockGitStorageFile(root, name)
	if err != nil {
		return err
	}
	defer lock.close()
	current, err := readGitStorageSnapshot(root, name, limit)
	if err != nil || !bytes.Equal(current.data, published) {
		return fmt.Errorf("%s changed before the operation could be rolled back", name)
	}
	if before.mode == 0 {
		return root.Remove(name)
	}
	if err := lock.prepare(before.data, before.mode); err != nil {
		return err
	}
	return lock.publish()
}

func (r *Repository) verifyGitStorageLocks(ctx context.Context, common *os.Root, locks ...*gitStorageLock) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now, err := os.Stat(r.CommonDir)
	if err != nil {
		return err
	}
	opened, err := common.Stat(".")
	if err != nil || !sameFileIdentity(opened, now) {
		return fmt.Errorf("the Git reference directory changed during the operation")
	}
	for _, lock := range locks {
		if lock == nil {
			continue
		}
		info, err := lock.parent.Stat(".")
		if err != nil {
			return err
		}
		if !lock.owns() {
			return fmt.Errorf("the Git lock for %s was replaced", lock.relative)
		}
		if err := checkDiscardParent(common, lock.relative, info); err != nil {
			return err
		}
	}
	return nil
}
