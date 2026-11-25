package appupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

var ErrInstallationChanged = errors.New("the installed GitWave AppImage changed")

const appImageBackupName = "previous.AppImage"

type AppImage struct {
	path, directory string
	file, parent    os.FileInfo
}

func InstallerForRunningApp() Installer {
	if image := CurrentAppImage(); image != nil {
		return image.Install
	}
	return nil
}

// CurrentAppImage binds the update to the image supplying this executable.
func CurrentAppImage() *AppImage {
	appDir, image := os.Getenv("APPDIR"), os.Getenv("APPIMAGE")
	if !filepath.IsAbs(appDir) || !filepath.IsAbs(image) || os.Getenv("TARGET_APPIMAGE") != "" {
		return nil
	}
	executable, err := os.Executable()
	if err != nil || executable != filepath.Join(appDir, "GitWave") {
		return nil
	}
	image, err = filepath.EvalSymlinks(image)
	if err != nil {
		return nil
	}
	bound, _ := bindAppImage(image)
	return bound
}

func bindAppImage(path string) (*AppImage, error) {
	dir := filepath.Dir(path)
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	parent, err := root.Stat(".")
	if err != nil {
		return nil, err
	}
	file, err := openAppImage(root, filepath.Base(path))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !ownedExecutableImage(info) {
		return nil, errors.New("the AppImage must be an executable file owned by the current user")
	}
	return &AppImage{path: path, directory: dir, file: info, parent: parent}, nil
}

// Install downloads to the same filesystem and publishes only a verified image.
// The returned function relaunches that installation after the app's old process exits.
func (a *AppImage) Install(ctx context.Context, candidate *Candidate, progress func(int64, int64)) (Restart, error) {
	if candidate == nil {
		return nil, ErrPackage
	}
	name, err := ArtifactName(candidate.Version(), LinuxX64)
	if err != nil || name != candidate.artifact {
		return nil, ErrPackage
	}
	root, err := os.OpenRoot(a.directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	parent, err := root.Stat(".")
	if err != nil || !os.SameFile(parent, a.parent) {
		return nil, ErrInstallationChanged
	}
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	stagePath, err := os.MkdirTemp(procFilePath(dir), ".gitwave-update-")
	if err != nil {
		return nil, err
	}
	stageName := filepath.Base(stagePath)
	stageFD, err := unix.Openat(int(dir.Fd()), stageName, unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	stageDir := os.NewFile(uintptr(stageFD), stagePath)
	defer stageDir.Close()
	stageInfo, err := stageDir.Stat()
	if err != nil {
		return nil, err
	}
	stage, err := os.OpenRoot(procFilePath(stageDir))
	if err != nil {
		return nil, err
	}
	defer stage.Close()
	var restore string
	defer func() {
		if restore != "" {
			return
		}
		_ = stage.Remove(name)
		_ = stage.Remove(appImageBackupName)
		if live, err := root.Lstat(stageName); err == nil && os.SameFile(live, stageInfo) {
			_ = root.Remove(stageName)
		}
	}()
	file, err := stage.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	err = candidate.downloadTo(ctx, file, progress)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return nil, errors.Join(err, closeErr)
	}
	previous := a.file
	restore, err = a.publish(ctx, root, stage, dir, stageDir, name, candidate.packageInfo)
	if err != nil {
		if restore != "" {
			return nil, fmt.Errorf("%w; the previous file remains at %s", err, filepath.Join(a.directory, stageName, restore))
		}
		return nil, err
	}
	return func(ctx context.Context) (func(bool) error, error) {
		finish, err := a.relaunch(ctx, os.Args[1:], candidate.packageInfo)
		if err != nil {
			return nil, fmt.Errorf("%w; the previous AppImage remains at %s", err, filepath.Join(a.directory, stageName, restore))
		}
		return func(quit bool) error {
			if err := finish(quit); err != nil {
				return err
			}
			if quit {
				a.removeAppImageBackup(stageName, restore, stageInfo, previous)
			}
			return nil
		}, nil
	}, nil
}

func (a *AppImage) removeAppImageBackup(stageName, backup string, expectedStage, previous os.FileInfo) {
	root, err := os.OpenRoot(a.directory)
	if err != nil {
		return
	}
	defer root.Close()
	if dir, err := root.Stat("."); err != nil || !os.SameFile(dir, a.parent) || !imageUnmodifiedAt(root, filepath.Base(a.path), a.file) {
		return
	}
	stage, err := root.OpenRoot(stageName)
	if err != nil {
		return
	}
	defer stage.Close()
	if info, err := stage.Stat("."); err != nil || !os.SameFile(info, expectedStage) || !imageStillAt(stage, backup, previous) {
		return
	}
	_ = stage.Remove(backup)
	if live, err := root.Lstat(stageName); err == nil && os.SameFile(live, expectedStage) {
		_ = root.Remove(stageName)
	}
}

func procFilePath(file *os.File) string { return fmt.Sprintf("/proc/self/fd/%d", file.Fd()) }

// publish returns a recovery location if an unexpected concurrent replacement
// prevents restoring the previously installed file.
func (a *AppImage) publish(ctx context.Context, root, stage *os.Root, dir, stageDir *os.File, artifact string, pkg Package) (string, error) {
	name := filepath.Base(a.path)
	old, err := openAppImage(root, name)
	if err != nil {
		return "", err
	}
	defer old.Close()
	oldInfo, err := old.Stat()
	if err != nil || !os.SameFile(oldInfo, a.file) {
		return "", ErrInstallationChanged
	}
	if err := unix.Flock(int(old.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return "", fmt.Errorf("lock the installed AppImage: %w", err)
	}
	if !imageStillAt(root, name, a.file) {
		return "", ErrInstallationChanged
	}
	updated, err := openAppImage(stage, artifact)
	if err != nil {
		return "", err
	}
	defer updated.Close()
	if err := verifyAppImageBytes(ctx, updated, pkg); err != nil {
		return "", err
	}
	if err := updated.Chmod(a.file.Mode().Perm()); err != nil {
		return "", err
	}
	if err := updated.Sync(); err != nil {
		return "", err
	}
	newInfo, err := updated.Stat()
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !imageUnmodifiedAt(root, name, a.file) || !imageUnmodifiedAt(stage, artifact, newInfo) {
		return "", ErrInstallationChanged
	}
	fd, stageFD := int(dir.Fd()), int(stageDir.Fd())
	err = unix.Renameat2(stageFD, artifact, fd, name, unix.RENAME_EXCHANGE)
	backup := artifact
	switch {
	case err == nil:
		if !imageStillAt(stage, artifact, a.file) || !imageStillAt(root, name, newInfo) {
			if imageStillAt(root, name, newInfo) && unix.Renameat2(stageFD, artifact, fd, name, unix.RENAME_EXCHANGE) == nil {
				return "", ErrInstallationChanged
			}
			return artifact, ErrInstallationChanged
		}
	case errors.Is(err, unix.ENOSYS), errors.Is(err, unix.EOPNOTSUPP), errors.Is(err, unix.EINVAL):
		if restore, err := a.publishWithoutExchange(ctx, stage, fd, stageFD, name, artifact); err != nil {
			return restore, err
		}
		backup = appImageBackupName
	default:
		return "", err
	}
	if err := errors.Join(dir.Sync(), stageDir.Sync()); err != nil {
		return backup, fmt.Errorf("sync the installed AppImage: %w", err)
	}
	a.file, err = updated.Stat()
	if err != nil {
		return backup, err
	}
	return backup, nil
}

func (a *AppImage) publishWithoutExchange(ctx context.Context, stage *os.Root, fd, stageFD int, name, artifact string) (string, error) {
	// Move the old name out of the way first. Either rename can fail if another
	// process owns the target name, and neither rename can overwrite it.
	if err := unix.Renameat2(fd, name, stageFD, appImageBackupName, unix.RENAME_NOREPLACE); err != nil {
		return "", err
	}
	err := ctx.Err()
	if err == nil && !imageStillAt(stage, appImageBackupName, a.file) {
		err = ErrInstallationChanged
	}
	if err == nil {
		err = unix.Renameat2(stageFD, artifact, fd, name, unix.RENAME_NOREPLACE)
	}
	if err == nil {
		return "", nil
	}
	if restoreErr := unix.Renameat2(stageFD, appImageBackupName, fd, name, unix.RENAME_NOREPLACE); restoreErr != nil {
		return appImageBackupName, errors.Join(err, restoreErr)
	}
	return "", err
}

func openAppImage(root *os.Root, name string) (*os.File, error) {
	file, err := root.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = ErrPackage
	}
	if err == nil {
		var header [20]byte
		if _, readErr := file.ReadAt(header[:], 0); readErr != nil ||
			!bytes.Equal(header[:7], []byte{0x7f, 'E', 'L', 'F', 2, 1, 1}) ||
			!bytes.Equal(header[8:11], []byte{'A', 'I', 2}) || binary.LittleEndian.Uint16(header[18:20]) != 62 {
			err = ErrPackage
		}
	}
	if err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

func ownedExecutableImage(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid()) && info.Mode().Perm()&0o100 != 0
}

func imageStillAt(root *os.Root, name string, expected os.FileInfo) bool {
	info, err := root.Lstat(name)
	return err == nil && sameImage(info, expected)
}

func imageUnmodifiedAt(root *os.Root, name string, expected os.FileInfo) bool {
	info, err := root.Lstat(name)
	if err != nil || !sameImage(info, expected) {
		return false
	}
	return info.Sys().(*syscall.Stat_t).Ctim == expected.Sys().(*syscall.Stat_t).Ctim
}

func sameImage(info, expected os.FileInfo) bool {
	return info.Mode().IsRegular() && os.SameFile(info, expected) &&
		info.Size() == expected.Size() && info.ModTime().Equal(expected.ModTime()) && info.Mode() == expected.Mode()
}

func verifyAppImageBytes(ctx context.Context, file *os.File, pkg Package) error {
	digest := sha256.New()
	size, err := io.Copy(digest, io.LimitReader(file, pkg.Size+1))
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if size != pkg.Size || hex.EncodeToString(digest.Sum(nil)) != pkg.SHA256 {
		return ErrPackage
	}
	return nil
}
