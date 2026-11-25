package distasset

import "golang.org/x/sys/unix"

// PublishDirectory moves a staged directory into place without replacing an existing path.
func PublishDirectory(stage, output string) error {
	return unix.Renameat2(unix.AT_FDCWD, stage, unix.AT_FDCWD, output, unix.RENAME_NOREPLACE)
}
