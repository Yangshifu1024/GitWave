package gitcli

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type renderResolution func(*os.Root, string) error

func (c *Conflict) prepareResolution(ctx context.Context, git *workingGit, live *os.Root, choice ConflictChoice, text string) (IndexStage, renderResolution, error) {
	if err := git.isolateWorkTree(); err != nil {
		return IndexStage{}, nil, err
	}
	shadow, err := os.OpenRoot(git.workTree)
	if err != nil {
		return IndexStage{}, nil, err
	}
	defer shadow.Close()
	path := c.change.Path
	paths := []string{path}
	if err := git.snapshotWorktreeMetadata(ctx, live, shadow, paths, nil, ".gitattributes"); err != nil {
		return IndexStage{}, nil, err
	}
	if err := git.freezeWorktreeAttributes(ctx); err != nil {
		return IndexStage{}, nil, err
	}
	if err := git.checkWorktreeFilters(ctx, paths, "resolving", false); err != nil {
		return IndexStage{}, nil, err
	}
	git.settings = append(git.settings, "-c", "filter.set.required=true", "-c", "filter.unset.required=true", "-c", "filter.unspecified.required=true")
	var stage IndexStage
	var render renderResolution
	switch choice {
	case ConflictText:
		if !c.CanEdit() || len(text) > maxConflictText || !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
			return IndexStage{}, nil, fmt.Errorf("the resolution is not supported text")
		}
		mode := os.FileMode(0o666)
		stage.Mode = "100644"
		if c.stamp.info != nil {
			mode = c.stamp.info.Mode().Perm()
			if mode&0o100 != 0 {
				stage.Mode = "100755"
			}
		} else if c.change.Conflict[1].Mode == "100755" ||
			(c.change.Conflict[1].Mode == "000000" && c.change.Conflict[2].Mode == "100755") {
			mode, stage.Mode = 0o777, "100755"
		}
		fileMode, err := git.read(ctx, 32, "config", "--bool", "--default=true", "--get", "core.filemode")
		if err != nil {
			return IndexStage{}, nil, err
		}
		if strings.TrimSpace(string(fileMode)) == "false" {
			previous := c.change.Conflict[1].Mode
			if previous != "100644" && previous != "100755" {
				previous = c.change.Conflict[2].Mode
			}
			if previous == "100644" || previous == "100755" {
				stage.Mode = previous
			}
		}
		out, err := git.run(ctx, text, 4096, "hash-object", "-w", "--path="+path, "--stdin")
		if err != nil {
			return IndexStage{}, nil, err
		}
		stage.OID = strings.TrimSpace(string(out))
		render = func(root *os.Root, name string) error {
			file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
			if err != nil {
				return err
			}
			_, err = io.WriteString(file, text)
			if c.stamp.info != nil {
				err = errors.Join(err, file.Chmod(mode))
			}
			return errors.Join(err, file.Sync(), file.Close())
		}
	case ConflictOurs, ConflictTheirs:
		stage = c.change.Conflict[int(choice)]
		if stage.Mode == "000000" {
			stage = IndexStage{}
		}
	default:
		return IndexStage{}, nil, fmt.Errorf("select a valid conflict resolution")
	}
	if stage.Mode != "" && (!validOID(stage.OID) || len(stage.OID) != git.oidSize) {
		return IndexStage{}, nil, fmt.Errorf("invalid Git object for the resolution")
	}
	if stage.Mode == "160000" {
		return IndexStage{}, nil, fmt.Errorf("resolve the submodule commit in Git before staging it")
	}
	before, err := git.otherIndexEntries(ctx, paths)
	if err != nil {
		return IndexStage{}, nil, err
	}
	records := fmt.Sprintf("0 %s\t%s\x00", strings.Repeat("0", git.oidSize), path)
	if stage.OID != "" {
		records += fmt.Sprintf("%s %s\t%s\x00", stage.Mode, stage.OID, path)
	}
	if _, err := git.run(ctx, records, 64<<10, "update-index", "-z", "--index-info"); err != nil {
		return IndexStage{}, nil, err
	}
	after, err := git.otherIndexEntries(ctx, paths)
	if err != nil {
		return IndexStage{}, nil, err
	}
	if !bytes.Equal(before, after) {
		return IndexStage{}, nil, fmt.Errorf("this path conflicts with another index entry")
	}
	if render == nil && stage.OID != "" {
		files, symlinks, err := git.extractIndexFiles(ctx, paths)
		if err != nil {
			return IndexStage{}, nil, err
		}
		render = func(root *os.Root, name string) error {
			from, err := os.OpenRoot(git.workTree)
			if err != nil {
				return err
			}
			defer from.Close()
			return copyDiscardReplacement(ctx, from, files[path], root, name, stage.Mode, symlinks)
		}
	}
	return stage, render, nil
}

// Retain the exact original until both the replacement and the private index
// have been installed. Failure recovery never replaces an external edit.
func (c *Conflict) publishFile(ctx context.Context, root *os.Root, local string, render renderResolution,
	install func(os.FileInfo, discardStamp) error) error {
	if err := c.verifyFile(ctx, root, local, c.parent, c.stamp); err != nil {
		return err
	}
	if render == nil && c.stamp.info == nil {
		return install(c.parent, discardStamp{})
	}
	parent, err := openWorktreeParent(root, local, c.parent == nil)
	if err != nil {
		return err
	}
	if parent != root {
		defer parent.Close()
	}
	identity, err := parent.Stat(".")
	if err != nil || (c.parent != nil && !sameFileIdentity(identity, c.parent)) {
		return fmt.Errorf("the conflict directory changed before saving %q", local)
	}
	tempName := ".gitwave-resolution-" + rand.Text()
	if err := parent.Mkdir(tempName, 0o700); err != nil {
		return err
	}
	tempInfo, err := parent.Lstat(tempName)
	if err != nil || !tempInfo.IsDir() {
		return fmt.Errorf("the resolution directory changed before opening")
	}
	temp, err := parent.OpenRoot(tempName)
	if err != nil {
		return err
	}
	if opened, err := temp.Stat("."); err != nil || !sameFileIdentity(opened, tempInfo) {
		temp.Close()
		return fmt.Errorf("the resolution directory changed while opening")
	}
	const next, original = "resolved", "original"
	defer func() {
		_ = temp.Remove(next)
		temp.Close()
		if current, err := parent.Lstat(tempName); err == nil && sameFileIdentity(tempInfo, current) {
			_ = parent.Remove(tempName)
		}
	}()
	var prepared discardStamp
	if render != nil {
		if err := render(temp, next); err != nil {
			return err
		}
		prepared, err = readDiscardStamp(ctx, temp, next)
		if err != nil {
			return err
		}
	}
	if err := c.verifyFile(ctx, root, local, identity, c.stamp); err != nil {
		return err
	}
	name := filepath.Base(local)
	saved := filepath.Join(c.repo.WorkTree, filepath.Dir(local), tempName, original)
	undoOriginal := func(cause error) error {
		if c.stamp.info != nil {
			if err := moveDiscardFile(temp, original, parent, name); err != nil {
				return errors.Join(cause, fmt.Errorf("original saved at %q: %w", saved, err))
			}
		}
		return cause
	}
	if c.stamp.info != nil {
		if err := moveDiscardFile(parent, name, temp, original); err != nil {
			return err
		}
		moved, err := readDiscardStamp(ctx, temp, original)
		if err != nil || !moved.matches(c.stamp) {
			return undoOriginal(errors.Join(err, fmt.Errorf("the working file changed on disk: %q", local)))
		}
	}
	if render != nil {
		if err := moveDiscardFile(temp, next, parent, name); err != nil {
			return undoOriginal(err)
		}
	}
	if err := install(identity, prepared); err != nil {
		if render != nil {
			file := sparseMergeFile{path: local, stamp: prepared}
			if cleanupErr := removeSparseMergeFile(ctx, root, file); cleanupErr != nil {
				return undoOriginal(errors.Join(err, cleanupErr))
			}
		}
		return undoOriginal(err)
	}
	if c.stamp.info != nil {
		current, err := readDiscardStamp(ctx, temp, original)
		if err != nil || !current.matches(c.stamp) {
			return errors.Join(err, fmt.Errorf("resolution staged; concurrent edits to the original were preserved at %q", saved))
		}
		if err := temp.Remove(original); err != nil {
			return fmt.Errorf("resolution staged, but original remains at %q: %w", saved, err)
		}
	}
	return nil
}
