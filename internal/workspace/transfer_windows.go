package workspace

import (
	"strings"

	"golang.org/x/sys/windows"
)

func publishNewFile(temp, path string) error {
	from, err := transferWindowsPath(temp)
	if err != nil {
		return err
	}
	to, err := transferWindowsPath(path)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH)
}

func transferWindowsPath(path string) (*uint16, error) {
	switch {
	case strings.HasPrefix(path, `\\?\`), strings.HasPrefix(path, `\\.\`):
	case strings.HasPrefix(path, `\\`):
		path = `\\?\UNC\` + path[2:]
	default:
		path = `\\?\` + path
	}
	return windows.UTF16PtrFromString(path)
}
