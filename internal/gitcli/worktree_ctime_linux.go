package gitcli

import (
	"os"
	"syscall"
)

func worktreeDirectoryChangeTime(info os.FileInfo) syscall.Timespec {
	return info.Sys().(*syscall.Stat_t).Ctim
}
