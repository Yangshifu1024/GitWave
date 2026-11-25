package gitcli

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func renameWithoutReplace(from uintptr, old string, to uintptr, name string) error {
	oldName, err := windows.NewNTUnicodeString(old)
	if err != nil {
		return err
	}
	object := windows.OBJECT_ATTRIBUTES{
		RootDirectory: windows.Handle(from),
		ObjectName:    oldName,
		Attributes:    windows.OBJ_CASE_INSENSITIVE,
	}
	object.Length = uint32(unsafe.Sizeof(object))
	var file windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&file, windows.DELETE|windows.SYNCHRONIZE, &object, &status, nil, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, windows.FILE_OPEN,
		windows.FILE_OPEN_REPARSE_POINT|windows.FILE_OPEN_FOR_BACKUP_INTENT|windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		return windowsRenameError(err)
	}
	defer windows.CloseHandle(file)

	// FILE_RENAME_INFORMATION with ReplaceIfExists left at zero.
	info := struct {
		ReplaceIfExists uint8
		RootDirectory   windows.Handle
		FileNameLength  uint32
		FileName        [256]uint16
	}{RootDirectory: windows.Handle(to)}
	units, err := windows.UTF16FromString(name)
	if err != nil {
		return err
	}
	if len(units) > len(info.FileName) {
		return syscall.ENAMETOOLONG
	}
	copy(info.FileName[:], units)
	info.FileNameLength = uint32((len(units) - 1) * 2)
	err = windows.NtSetInformationFile(file, &status, (*byte)(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)), windows.FileRenameInformation)
	return windowsRenameError(err)
}

func windowsRenameError(err error) error {
	if status, ok := err.(windows.NTStatus); ok {
		return status.Errno()
	}
	return err
}
