package gitcli

import "golang.org/x/sys/unix"

func renameWithoutReplace(from uintptr, old string, to uintptr, name string) error {
	return unix.RenameatxNp(int(from), old, int(to), name, unix.RENAME_EXCL)
}
