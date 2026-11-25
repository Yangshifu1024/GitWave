//go:build darwin || linux

package workspace

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func lockStableProfile(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = unix.Close(fd)
		if errors.Is(err, unix.EWOULDBLOCK) {
			return nil, ErrStableProfileInUse
		}
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func protectRecoveryDirectory(string) error { return nil }

func syncRecoveryDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

func protectDatabaseFiles(path string) error {
	// An existing database may contain a proxy URL with embedded credentials.
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		name := path + suffix
		// Closing a second descriptor can release SQLite's POSIX file locks.
		info, err := os.Stat(name)
		if suffix != "" && os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s must be a regular file", name)
		}
		if info.Mode().Perm() != 0o600 {
			if err := os.Chmod(name, 0o600); err != nil && !(suffix != "" && os.IsNotExist(err)) {
				return err
			}
		}
	}
	return nil
}
