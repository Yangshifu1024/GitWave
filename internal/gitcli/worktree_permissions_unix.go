//go:build darwin || linux

package gitcli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"gitwave.work/internal/process"
)

func prepareExistingWorktreeDirectory(ctx context.Context, target CloneTarget, stage *os.Root, name string) (func() error, error) {
	// The original is empty. A preserved copy carries the owner, mode and
	// access ACL into the new checkout before Git starts writing its contents.
	path := filepath.Join(stage.Name(), name)
	cmd := exec.CommandContext(ctx, "/bin/cp", "-pRP", "-x", target.path, path)
	cmd.Env = slices.DeleteFunc(os.Environ(), func(variable string) bool {
		return strings.HasPrefix(variable, "COPYFILE_DISABLE=") || strings.HasPrefix(variable, "COPY_EXTENDED_ATTRIBUTES_DISABLE=")
	})
	process.Hide(cmd)
	_, err := gitOutput(cmd, 64<<10)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	info, nonempty, err := inspectCloneDirectory(path)
	if err != nil {
		return nil, err
	}
	if info == nil || nonempty || info.Mode() != target.dir.Mode() {
		return nil, fmt.Errorf("the destination changed while copying its permissions")
	}
	return func() error {
		now, err := os.Lstat(target.path)
		if err != nil || !sameFileIdentity(now, target.dir) ||
			worktreeDirectoryChangeTime(now) != worktreeDirectoryChangeTime(target.dir) {
			return fmt.Errorf("the destination permissions or contents changed during worktree creation")
		}
		return nil
	}, nil
}
