//go:build !darwin && !linux && !windows

package gitcli

import "errors"

func renameWithoutReplace(uintptr, string, uintptr, string) error {
	return errors.ErrUnsupported
}
