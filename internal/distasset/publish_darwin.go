package distasset

import "golang.org/x/sys/unix"

// PublishDirectory moves a staged directory into place without replacing an existing path.
func PublishDirectory(stage, output string) error {
	return unix.RenamexNp(stage, output, unix.RENAME_EXCL)
}
