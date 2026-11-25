package appupdate

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"gitwave.work/internal/gittool"
)

type installFile struct {
	target, incoming, previous string
	newFile                    os.FileInfo
	saved, installed           bool
	bundle                     bool
}

// installFiles publishes only the listed package files. The caller creates a
// private stage on the destination volume and supplies a non-overwriting move.
func installFiles(payload, destination, stage string, names []string, move func(string, string) error, commit func() error) (err error) {
	files := make([]installFile, 0, len(names))
	defer func() {
		if err != nil {
			if restoreErr := restoreInstallFiles(files, move); restoreErr != nil {
				err = errors.Join(err, fmt.Errorf("installation files were retained at %s: %w", stage, restoreErr))
				return
			}
		}
		_ = os.RemoveAll(stage)
	}()
	for _, name := range names {
		file := installFile{
			target:   filepath.Join(destination, name),
			incoming: filepath.Join(stage, name+".new"),
			previous: filepath.Join(stage, name+".old"),
			bundle:   name == gittool.Directory,
		}
		source := filepath.Join(payload, name)
		if file.bundle {
			file.newFile, err = copyInstallBundle(source, file.incoming)
		} else {
			file.newFile, err = copyInstallFile(source, file.incoming)
		}
		if err != nil {
			return err
		}
		files = append(files, file)
	}
	for i := range files {
		file := &files[i]
		previous, err := installFileIdentity(file.target, file.bundle)
		if err == nil {
			if file.bundle {
				if err := gittool.CheckWindowsBundle(file.target, false); err != nil {
					return err
				}
			}
			if err := move(file.target, file.previous); err != nil {
				return fmt.Errorf("save %s: %w", filepath.Base(file.target), err)
			}
			file.saved = true
			if !installFileIs(file.previous, previous) {
				return fmt.Errorf("the installed file %s changed while being saved", filepath.Base(file.target))
			}
			if file.bundle {
				if err := gittool.CheckWindowsBundle(file.previous, false); err != nil {
					return err
				}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := move(file.incoming, file.target); err != nil {
			return fmt.Errorf("publish %s: %w", filepath.Base(file.target), err)
		}
		file.installed = true
		if !installFileIs(file.target, file.newFile) {
			return fmt.Errorf("the installed file %s changed during publication", filepath.Base(file.target))
		}
	}
	for _, file := range files {
		if !installFileIs(file.target, file.newFile) {
			return fmt.Errorf("the installed file %s changed before registration", filepath.Base(file.target))
		}
		if file.bundle && file.saved {
			if err := gittool.CheckWindowsBundle(file.previous, false); err != nil {
				return err
			}
		}
	}
	return commit()
}

func restoreInstallFiles(files []installFile, move func(string, string) error) error {
	var restoreErr error
	for i := len(files) - 1; i >= 0; i-- {
		file := files[i]
		if file.installed {
			if !installFileIs(file.target, file.newFile) {
				restoreErr = errors.Join(restoreErr, fmt.Errorf("%s changed before rollback", file.target))
				continue
			}
			if err := move(file.target, file.incoming); err != nil {
				restoreErr = errors.Join(restoreErr, err)
				continue
			}
			if !installFileIs(file.incoming, file.newFile) {
				err := move(file.incoming, file.target)
				restoreErr = errors.Join(restoreErr, fmt.Errorf("%s changed during rollback", file.target), err)
				continue
			}
			if file.bundle {
				restoreErr = errors.Join(restoreErr, gittool.CheckWindowsBundle(file.incoming, false))
			}
		}
		if file.saved {
			restoreErr = errors.Join(restoreErr, move(file.previous, file.target))
		}
	}
	return restoreErr
}

func openInstallEntry(path string, directory bool) (*os.File, os.FileInfo, error) {
	entry, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if directory && !entry.IsDir() || !directory && !entry.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("invalid installation entry %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil || !os.SameFile(entry, info) {
		file.Close()
		return nil, nil, errors.Join(fmt.Errorf("%s changed while being opened", path), err)
	}
	return file, info, nil
}

func copyInstallFile(source, destination string) (os.FileInfo, error) {
	from, info, err := openInstallEntry(source, false)
	if err != nil {
		return nil, err
	}
	defer from.Close()
	return copyInstallData(from, info, destination)
}

func copyInstallData(from *os.File, info os.FileInfo, destination string) (os.FileInfo, error) {
	to, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return nil, err
	}
	written, err := io.Copy(to, io.LimitReader(from, maxPackageSize+1))
	if err == nil && (written != info.Size() || written > maxPackageSize) {
		err = ErrPackage
	}
	copied, statErr := to.Stat()
	return copied, errors.Join(err, to.Sync(), statErr, to.Close())
}

func copyInstallBundle(source, destination string) (os.FileInfo, error) {
	if err := gittool.CheckWindowsBundle(source, true); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(source)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	remaining := int64(maxPackageSize)
	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(destination, filepath.FromSlash(path))
		if entry.IsDir() {
			return os.Mkdir(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return ErrPackage
		}
		file, err := root.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Size() > remaining {
			return ErrPackage
		}
		remaining -= info.Size()
		_, err = copyInstallData(file, info, target)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := gittool.CheckWindowsBundle(destination, true); err != nil {
		return nil, err
	}
	return installFileIdentity(destination, true)
}

func installFileIdentity(path string, directory bool) (os.FileInfo, error) {
	file, info, err := openInstallEntry(path, directory)
	if err != nil {
		return nil, err
	}
	return info, file.Close()
}

func installFileIs(path string, other os.FileInfo) bool {
	actual, err := installFileIdentity(path, other.IsDir())
	return err == nil && os.SameFile(actual, other)
}
