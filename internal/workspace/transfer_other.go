//go:build !darwin && !linux && !windows

package workspace

import "os"

func publishNewFile(temp, path string) error {
	return os.Link(temp, path)
}
