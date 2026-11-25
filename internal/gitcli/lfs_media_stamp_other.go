//go:build !darwin && !linux && !windows

package gitcli

import "os"

func lfsMediaChangeTime(*os.File, os.FileInfo) (int64, bool) {
	return 0, false
}
