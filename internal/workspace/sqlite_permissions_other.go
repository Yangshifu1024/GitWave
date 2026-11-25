//go:build !darwin && !linux && !windows

package workspace

import "os"

func protectDatabaseFiles(string) error { return nil }

func protectRecoveryDirectory(string) error { return nil }

func syncRecoveryDirectory(string) error { return nil }

func lockStableProfile(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
}
