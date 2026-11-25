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
	"slices"
	"strings"
	"sync"
)

var indexWriters = struct {
	sync.Mutex
	repos map[string]*indexWriter
}{repos: make(map[string]*indexWriter)}

type indexWriter struct {
	ready chan struct{}
	users int
}

func lockIndexWriter(ctx context.Context, commonDir string) (func(), error) {
	indexWriters.Lock()
	writer := indexWriters.repos[commonDir]
	if writer == nil {
		writer = &indexWriter{ready: make(chan struct{}, 1)}
		indexWriters.repos[commonDir] = writer
	}
	writer.users++
	indexWriters.Unlock()
	releaseRef := func() {
		indexWriters.Lock()
		writer.users--
		if writer.users == 0 {
			delete(indexWriters.repos, commonDir)
		}
		indexWriters.Unlock()
	}
	select {
	case writer.ready <- struct{}{}:
		return func() {
			<-writer.ready
			releaseRef()
		}, nil
	case <-ctx.Done():
		releaseRef()
		return nil, ctx.Err()
	}
}

type checkoutIndexLock struct {
	root    *os.Root
	file    *os.File
	info    os.FileInfo
	release func()
}

func (r *Repository) lockCheckoutIndex(ctx context.Context) (*checkoutIndexLock, error) {
	if r.WorkTree == "" {
		return nil, fmt.Errorf("this repository has no working copy")
	}
	release, err := lockIndexWriter(ctx, r.CommonDir)
	if err != nil {
		return nil, err
	}
	lock := &checkoutIndexLock{release: release}
	if err := lock.open(ctx, r); err != nil {
		lock.close()
		return nil, err
	}
	return lock, nil
}

func (l *checkoutIndexLock) open(ctx context.Context, r *Repository) error {
	if err := r.checkIndexCheckout(ctx); err != nil {
		return err
	}
	root, err := os.OpenRoot(r.GitDir)
	if err != nil {
		return err
	}
	l.root = root
	identity, err := root.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(identity, r.gitDirInfo) {
		return fmt.Errorf("the Git directory was replaced before locking the index")
	}
	return l.openFile()
}

func (l *checkoutIndexLock) openFile() error {
	var err error
	l.file, err = l.root.OpenFile("index.lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o666)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("the Git index is locked by another operation")
	}
	if err != nil {
		return err
	}
	l.info, err = l.file.Stat()
	if err != nil {
		l.file.Close()
		_ = l.root.Remove("index.lock")
	}
	return err
}

func (l *checkoutIndexLock) ownsLock() bool {
	info, err := l.root.Lstat("index.lock")
	return l.info != nil && err == nil && os.SameFile(info, l.info)
}

func (l *checkoutIndexLock) close() {
	if l.file != nil {
		l.file.Close()
		if l.ownsLock() {
			_ = l.root.Remove("index.lock")
		}
	}
	if l.root != nil {
		l.root.Close()
	}
	l.release()
}

// SetStaged applies one explicit selection to the index. All intermediate
// changes use a private index. Git's index.lock protects the original checkout
// while the selection is checked and the result is published.
func (r *Repository) SetStaged(ctx context.Context, shown Status, selected []Change, stage bool, moves ...WorkingMove) error {
	if len(selected) == 0 && len(moves) == 0 {
		return nil
	}
	lock, err := r.lockCheckoutIndex(ctx)
	if err != nil {
		return err
	}
	defer lock.close()

	current, err := r.Status(ctx)
	if err != nil {
		return err
	}
	if current.HeadOID != shown.HeadOID || current.HeadRef != shown.HeadRef {
		return fmt.Errorf("HEAD changed since the working copy was shown")
	}
	if current.Commit != shown.Commit {
		return fmt.Errorf("the Git operation changed since the working copy was shown")
	}
	if len(moves) != 0 {
		selected = slices.Clone(selected)
		for _, move := range moves {
			if move.Staged() == stage {
				return fmt.Errorf("the selected move is on the wrong side of the index")
			}
			if stage {
				selected = append(selected, move.Source, move.Destination)
				continue
			}
			if move.stagedHead != current.HeadOID {
				return fmt.Errorf("HEAD changed since the staged move was shown")
			}
			for _, side := range [...]Change{move.Source, move.Destination} {
				i := slices.IndexFunc(current.Changes, func(change Change) bool { return sameStagedEntry(change, side) })
				if i < 0 {
					return fmt.Errorf("the selected change is no longer current: %q", side.Path)
				}
				selected = append(selected, current.Changes[i])
			}
		}
		if !stage {
			moves = nil
		}
	}
	add, remove, err := indexPaths(current, selected, stage)
	if err != nil {
		return err
	}
	head := current.HeadOID
	if head == "(initial)" {
		head = ""
	}
	git, err := r.workingGit(ctx, head)
	if err != nil {
		return err
	}
	defer git.close()
	git.index = filepath.Join(git.dir, "index")
	git.settings = append(git.settings, "-c", "core.splitIndex=false")
	if err := copyOriginalIndex(lock.root, git.index); err != nil {
		return err
	}
	affected := append(slices.Clone(add), remove...)
	before, err := git.otherIndexEntries(ctx, affected)
	if err != nil {
		return err
	}
	if stage {
		gitlinks := make(map[string]string)
		var resolutions []string
		for _, change := range selected {
			if change.WorkTreeMode == "160000" {
				gitlinks[change.Path] = change.WorkTreeOID
			}
			if change.Kind == 'u' && change.WorkTreeMode != "000000" {
				resolutions = append(resolutions, change.Path)
			}
		}
		err = git.stageSelection(ctx, add, remove, gitlinks, moves...)
		if err == nil && len(resolutions) != 0 {
			err = git.refreshResolvedStats(ctx, resolutions)
		}
		if err == nil {
			git.refreshStagedLFSStats(ctx, add)
		}
	} else {
		err = git.unstageSelection(ctx, head, remove)
	}
	if err != nil {
		return err
	}
	after, err := git.otherIndexEntries(ctx, affected, moves...)
	if err != nil {
		return err
	}
	if !bytes.Equal(before, after) {
		return fmt.Errorf("this file and its child paths must be updated together")
	}
	verify := func() error {
		if err := r.checkIndexCheckout(ctx); err != nil {
			return err
		}
		ref, err := r.optionalRef(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
		if err != nil {
			return err
		}
		headRef, err := r.optionalRef(ctx, "symbolic-ref", "-q", "HEAD")
		if err != nil {
			return err
		}
		if ref != head || headRef != current.HeadRef {
			return fmt.Errorf("HEAD changed during the index update")
		}
		state, err := r.readCommitState(ctx, lock.root)
		if err != nil {
			return err
		}
		if state != current.Commit {
			return fmt.Errorf("the Git operation changed during the index update")
		}
		if len(moves) != 0 {
			root, err := os.OpenRoot(r.WorkTree)
			if err != nil {
				return err
			}
			defer root.Close()
			for _, move := range moves {
				if err := checkRemovedWorkingFile(root, move.Source.Path); err != nil {
					return err
				}
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return git.publishLFSObjects(ctx)
	}
	return publishIndex(lock.root, lock.file, lock.info, git.index, verify)
}

func sameStagedEntry(a, b Change) bool {
	return a.Kind == b.Kind && a.Index == b.Index && a.Path == b.Path && a.PreviousPath == b.PreviousPath &&
		a.HeadMode == b.HeadMode && a.HeadOID == b.HeadOID && a.IndexMode == b.IndexMode && a.IndexOID == b.IndexOID
}

func copyOriginalIndex(root *os.Root, target string) error {
	original, err := root.OpenFile("index", os.O_RDONLY|workFileOpenFlags, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer original.Close()
	return copyGitRegularFile(original, target, 512<<20)
}

func (r *Repository) checkIndexCheckout(ctx context.Context) error {
	fresh, err := OpenRoot(ctx, r.WorkTree)
	if err != nil {
		return err
	}
	if !r.SameCheckout(fresh) {
		return fmt.Errorf("the checkout changed since the working copy was opened")
	}
	return nil
}

func publishIndex(root *os.Root, lock *os.File, lockInfo os.FileInfo, source string, verify func() error) error {
	if err := writeLockedIndex(root, lock, source); err != nil {
		return err
	}
	if err := verify(); err != nil {
		return err
	}
	return installLockedIndex(root, lockInfo)
}

func writeLockedIndex(root *os.Root, lock *os.File, source string) error {
	index, err := os.Open(source)
	if err != nil {
		return err
	}
	defer index.Close()
	if previous, err := root.Lstat("index"); err == nil {
		if err := lock.Chmod(previous.Mode().Perm()); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if _, err := io.Copy(lock, index); err != nil {
		return err
	}
	if err := lock.Sync(); err != nil {
		return err
	}
	return lock.Close()
}

func installLockedIndex(root *os.Root, lockInfo os.FileInfo) error {
	if info, err := root.Lstat("index.lock"); err != nil || !os.SameFile(info, lockInfo) {
		return fmt.Errorf("the Git index lock was replaced")
	}
	return root.Rename("index.lock", "index")
}

func indexPaths(current Status, requested []Change, stage bool) (add, remove []string, err error) {
	available := make(map[Change]bool, len(current.Changes))
	for _, change := range current.Changes {
		available[change] = true
	}
	for _, change := range requested {
		side := change.Index
		if stage {
			if change.SubmoduleFilesOnly() {
				return nil, nil, fmt.Errorf("stage the files inside submodule %q", change.Path)
			}
			side = change.WorkTree
			if change.Kind == '?' {
				side = '?'
			} else if change.Kind == 'u' {
				side = 'M'
				if change.WorkTreeMode == "000000" {
					side = 'D'
				}
			}
		}
		if (change.Kind == 'u' && !stage) || side == 0 || side == '.' || !available[change] {
			return nil, nil, fmt.Errorf("the selected change is no longer current: %q", change.Path)
		}
		if !validIndexPath(change.Path) {
			return nil, nil, fmt.Errorf("invalid index path: %q", change.Path)
		}
		if !stage || side == 'D' {
			remove = append(remove, change.Path)
		} else {
			add = append(add, change.Path)
		}
		if side == 'R' {
			if !validIndexPath(change.PreviousPath) {
				return nil, nil, fmt.Errorf("invalid rename source: %q", change.PreviousPath)
			}
			remove = append(remove, change.PreviousPath)
		}
	}
	slices.Sort(add)
	slices.Sort(remove)
	return slices.Compact(add), slices.Compact(remove), nil
}

func validIndexPath(path string) bool {
	if !validFilePath(path) {
		return false
	}
	if _, err := filepath.Localize(path); err != nil {
		return false
	}
	for part := range strings.SplitSeq(path, "/") {
		if strings.EqualFold(part, ".git") {
			return false
		}
	}
	return true
}
