//go:build !darwin && !linux && !windows

package gitcli

import (
	"context"
	"errors"
	"os"
)

func prepareExistingWorktreeDirectory(context.Context, CloneTarget, *os.Root, string) (func() error, error) {
	return nil, errors.ErrUnsupported
}
