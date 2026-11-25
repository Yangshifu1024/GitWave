package gitcli

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func lfsMediaChangeTime(file *os.File, _ os.FileInfo) (int64, bool) {
	var information struct {
		Created, Accessed, Written, Changed int64
		Attributes, Padding                 uint32
	}
	err := windows.GetFileInformationByHandleEx(windows.Handle(file.Fd()), windows.FileBasicInfo,
		(*byte)(unsafe.Pointer(&information)), uint32(unsafe.Sizeof(information)))
	return information.Changed, err == nil && information.Changed != 0
}
