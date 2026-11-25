//go:build unix

package gitcli

import (
	"fmt"
	"os"
	"syscall"
)

func lfsDirectoryIdentity(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() {
		return "", fmt.Errorf("could not identify the LFS destination directory")
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino), nil
}
