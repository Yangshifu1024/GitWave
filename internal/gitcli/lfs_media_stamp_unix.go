//go:build darwin || linux

package gitcli

import "os"

func lfsMediaChangeTime(_ *os.File, info os.FileInfo) (int64, bool) {
	stamp := worktreeDirectoryChangeTime(info)
	changed := stamp.Nano()
	return changed, changed != 0
}
