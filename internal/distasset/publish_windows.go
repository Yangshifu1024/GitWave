package distasset

import "golang.org/x/sys/windows"

// PublishDirectory moves a staged directory into place without replacing an existing path.
func PublishDirectory(stage, output string) error {
	from, err := windows.UTF16PtrFromString(stage)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(output)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH)
}
