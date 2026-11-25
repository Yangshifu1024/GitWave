package gitcli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func prepareExistingWorktreeDirectory(ctx context.Context, target CloneTarget, stage *os.Root, name string) (func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	security, err := worktreeDirectorySecurity(target)
	if err != nil {
		return nil, err
	}
	descriptor := security.String()
	if descriptor == "" {
		return nil, fmt.Errorf("could not read the destination security descriptor")
	}
	if err := stage.Mkdir(name, target.dir.Mode().Perm()); err != nil {
		return nil, err
	}
	path, err := windows.UTF16PtrFromString(filepath.Join(stage.Name(), name))
	if err != nil {
		return nil, err
	}
	const share = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE
	const flags = windows.FILE_FLAG_OPEN_REPARSE_POINT | windows.FILE_FLAG_BACKUP_SEMANTICS
	handle, err := windows.CreateFile(path, windows.READ_CONTROL|windows.WRITE_DAC, share, nil, windows.OPEN_EXISTING, flags, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(handle)
	existing, err := windows.GetSecurityInfo(handle, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return nil, err
	}
	owner, _, err := security.Owner()
	if err != nil {
		return nil, err
	}
	currentOwner, _, err := existing.Owner()
	if err != nil {
		return nil, err
	}
	if !owner.Equals(currentOwner) {
		return nil, fmt.Errorf("the existing worktree directory has a different owner")
	}
	dacl, _, err := security.DACL()
	if err != nil {
		return nil, err
	}
	// Copy the original access entries as written, without combining them
	// with entries inherited from the private staging directory.
	if err := windows.SetSecurityInfo(handle, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil); err != nil {
		return nil, err
	}
	return func() error {
		now, err := worktreeDirectorySecurity(target)
		if err != nil || now.String() != descriptor {
			return fmt.Errorf("the destination permissions changed during worktree creation")
		}
		return nil
	}, nil
}

func worktreeDirectorySecurity(target CloneTarget) (*windows.SECURITY_DESCRIPTOR, error) {
	original, err := os.Open(target.path)
	if err != nil {
		return nil, err
	}
	defer original.Close()
	info, err := original.Stat()
	if err != nil || !sameFileIdentity(info, target.dir) {
		return nil, ErrCloneTargetChanged
	}
	const permissions = windows.OWNER_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION
	return windows.GetSecurityInfo(windows.Handle(original.Fd()), windows.SE_FILE_OBJECT, permissions)
}
