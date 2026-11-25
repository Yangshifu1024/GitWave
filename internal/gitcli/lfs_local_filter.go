package gitcli

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const lfsFilterArg = "--gitwave-lfs-filter"

var errLFSObjectMismatch = errors.New("LFS object does not match its pointer")

// LocalLFSFilter handles Git's private filter command before starting the UI.
func LocalLFSFilter() bool {
	if len(os.Args) < 2 || os.Args[1] != lfsFilterArg {
		return false
	}
	var err error
	switch {
	case len(os.Args) == 3 && os.Args[2] == "clean":
		err = cleanLFS(os.Stdin, os.Stdout, os.Getenv(lfsFilterStage))
	case len(os.Args) == 4 && os.Args[2] == "smudge":
		cache := os.Getenv(lfsFilterCache)
		if dir := os.Getenv(lfsFilterAllowed); dir != "" {
			if _, statErr := os.Stat(filepath.Join(dir, lfsPathKey(os.Args[3]))); errors.Is(statErr, fs.ErrNotExist) {
				cache = ""
			} else if statErr != nil {
				err = statErr
				break
			}
		}
		err = smudgeLFS(os.Stdin, os.Stdout, cache, os.Getenv(lfsFilterTemp))
	default:
		err = fmt.Errorf("invalid GitWave LFS filter invocation")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return true
}

func lfsPathKey(name string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(filepath.ToSlash(name))))
}

func lfsInput(in io.Reader) ([]byte, io.Reader, error) {
	prefix := make([]byte, lfsPointerLimit)
	length, err := io.ReadFull(in, prefix)
	prefix = prefix[:length]
	if err == nil {
		return prefix, io.MultiReader(bytes.NewReader(prefix), in), nil
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return prefix, bytes.NewReader(prefix), nil
	}
	return nil, nil, err
}

// cleanLFS computes pointer bytes for reads and retains content only for an
// explicit write to a private staging directory.
func cleanLFS(in io.Reader, out io.Writer, stagedDir string) error {
	prefix, contents, err := lfsInput(in)
	if err != nil {
		return err
	}
	_, pointer := parseLFSPointer(prefix)
	if len(prefix) == 0 || pointer {
		_, err := out.Write(prefix)
		return err
	}
	hash := sha256.New()
	var writer io.Writer = hash
	var temp *os.File
	if stagedDir != "" {
		temp, err = os.CreateTemp(stagedDir, "incoming-")
		if err != nil {
			return err
		}
		defer os.Remove(temp.Name())
		writer = io.MultiWriter(hash, temp)
	}
	length, copyErr := io.Copy(writer, contents)
	result := lfsPointer{oid: hex.EncodeToString(hash.Sum(nil)), size: length}
	if temp != nil {
		if err := errors.Join(copyErr, temp.Close()); err != nil {
			return err
		}
		// Multiple files with identical contents produce the same object.
		if err := os.Rename(temp.Name(), filepath.Join(stagedDir, result.oid)); err != nil {
			return err
		}
	} else if copyErr != nil {
		return copyErr
	}
	return result.writeTo(out)
}

func smudgeLFS(in io.Reader, out io.Writer, storage, tempDir string) error {
	prefix, contents, err := lfsInput(in)
	if err != nil {
		return err
	}
	pointer, found := parseLFSPointer(prefix)
	if storage == "" || !found || pointer.extended {
		_, err := io.Copy(out, contents)
		return err
	}
	if pointer.size == 0 {
		return nil
	}
	object, err := openLFSObject(storage, pointer.oid)
	if errors.Is(err, fs.ErrNotExist) {
		// Like git lfs checkout, only materialize objects already available
		// locally. Preserve the pointer until an explicit download.
		_, err := out.Write(prefix)
		return err
	}
	if err != nil {
		return err
	}
	defer object.Close()
	verified, err := os.CreateTemp(tempDir, "lfs-smudge-")
	if err != nil {
		return err
	}
	defer os.Remove(verified.Name())
	defer verified.Close()
	if err := copyLFSObject(context.Background(), verified, object, pointer); err != nil {
		return err
	}
	if _, err := verified.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, err = io.Copy(out, verified)
	return err
}

// openLFSObject anchors lookups below the configured cache and rejects
// symlinked entries and substituted directories on the object path.
func openLFSObject(storage, oid string) (*os.File, error) {
	root, err := os.OpenRoot(storage)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return openLFSObjectIn(root, oid)
}

func openLFSObjectIn(root *os.Root, oid string) (*os.File, error) {
	objectPath := lfsObjectPath(oid)
	parent, err := openWorktreeParent(root, objectPath, false)
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	return openLFSRegular(parent, oid)
}

func lfsObjectPath(oid string) string { return filepath.Join("objects", oid[:2], oid[2:4], oid) }

func openLFSRegular(root *os.Root, name string) (*os.File, error) {
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("LFS object %s is not a regular file", name)
	}
	file, err := root.OpenFile(name, os.O_RDONLY|workFileOpenFlags, 0)
	if err != nil {
		return nil, err
	}
	actual, err := file.Stat()
	if err != nil || !sameDiscardMetadata(actual, before) {
		file.Close()
		return nil, fmt.Errorf("LFS object %s changed while opening it", name)
	}
	return file, nil
}

func copyLFSObject(ctx context.Context, target io.Writer, object *os.File, pointer lfsPointer) error {
	info, err := object.Stat()
	if err != nil {
		return err
	}
	hash := sha256.New()
	if info.Size() != pointer.size {
		return fmt.Errorf("%s: %w (size)", pointer.oid, errLFSObjectMismatch)
	}
	reader := stageReader{ctx, object}
	length, err := io.CopyN(io.MultiWriter(target, hash), reader, pointer.size)
	if err != nil {
		return err
	}
	var extra [1]byte
	more, readErr := io.ReadFull(reader, extra[:])
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}
	if more != 0 || length != pointer.size || hex.EncodeToString(hash.Sum(nil)) != pointer.oid {
		return fmt.Errorf("%s: %w (SHA-256)", pointer.oid, errLFSObjectMismatch)
	}
	return nil
}

func storeLFSObject(ctx context.Context, root *os.Root, object *os.File, oid string) error {
	info, err := object.Stat()
	if err != nil {
		return err
	}
	pointer := lfsPointer{oid: oid, size: info.Size()}
	parent, err := openWorktreeParent(root, lfsObjectPath(oid), true)
	if err != nil {
		return err
	}
	defer parent.Close()
	tempName := "gitwave-lfs-" + rand.Text()
	temp, err := parent.OpenFile(tempName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer parent.Remove(tempName)
	if err := copyLFSObject(ctx, temp, object, pointer); err != nil {
		temp.Close()
		return err
	}
	if err := errors.Join(temp.Sync(), temp.Close()); err != nil {
		return err
	}
	if err := moveDiscardFile(parent, tempName, parent, oid); !errors.Is(err, fs.ErrExist) {
		return err
	}
	existing, err := openLFSRegular(parent, oid)
	if err != nil {
		return err
	}
	defer existing.Close()
	return copyLFSObject(ctx, io.Discard, existing, pointer)
}
