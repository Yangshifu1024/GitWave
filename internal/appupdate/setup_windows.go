//go:build windows

package appupdate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"gitwave.work/internal/gittool"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	windowsInstallArg    = "--gitwave-install"
	windowsUninstallArg  = "--gitwave-prepare-uninstall"
	installedUninstaller = "Uninstall.exe"
	uninstallParent      = `Software\Microsoft\Windows\CurrentVersion\Uninstall`
	legacyRegistration   = "GitWave"
	stableRegistration   = "desktop.gitwave.work"
	previewRegistration  = "desktop.gitwave.work.native-preview"
)

func installWindowsPayload(args []string) error {
	if len(args) != 3 || (args[1] != stableRegistration && args[1] != previewRegistration) || !validVersion(args[2]) {
		return errors.New("invalid GitWave installer arguments")
	}
	destination := filepath.Clean(args[0])
	if !filepath.IsAbs(destination) || destination == filepath.VolumeName(destination)+`\` || strings.ContainsAny(destination, "\x00\r\n\"") {
		return errors.New("select a GitWave installation directory")
	}
	worker, err := os.Executable()
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Base(worker), windowsAppName) {
		return errors.New("invalid GitWave setup executable")
	}
	payload := filepath.Dir(worker)
	if strings.EqualFold(payload, destination) {
		return errors.New("GitWave setup requires a separate installation directory")
	}
	if err := verifyWindowsProductVersion(worker, args[2]); err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	destination, err = filepath.EvalSymlinks(destination)
	if err != nil {
		return err
	}
	if strings.EqualFold(payload, destination) || destination == filepath.VolumeName(destination)+`\` {
		return errors.New("invalid GitWave installation directory")
	}
	root, err := holdWindowsInstallDirectory(destination)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(root)
	operation, err := lockWindowsInstallOperation(destination, "Setup")
	if err != nil {
		return err
	}
	defer windows.CloseHandle(operation)
	if err := waitForWindowsAppToClose(filepath.Join(destination, windowsAppName)); err != nil {
		return err
	}
	registration, closeRegistration, err := prepareWindowsRegistration(args[1], destination, args[2])
	if err != nil {
		return err
	}
	defer closeRegistration()
	stage, err := makeWindowsPrivateDirectory(destination, ".gitwave-setup-")
	if err != nil {
		return err
	}
	return installFiles(payload, destination, stage, []string{windowsAppName, gittool.Directory, installedUninstaller},
		moveWindowsSetupFile, registration)
}

func prepareWindowsUninstall(destination string) error {
	if !filepath.IsAbs(destination) || filepath.Clean(destination) == filepath.VolumeName(destination)+`\` {
		return errors.New("invalid GitWave installation directory")
	}
	destination, err := filepath.EvalSymlinks(destination)
	if err != nil {
		return err
	}
	root, err := holdWindowsInstallDirectory(destination)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(root)
	operation, err := lockWindowsInstallOperation(destination, "Setup")
	if err != nil {
		return err
	}
	defer windows.CloseHandle(operation)
	if err := waitForWindowsAppToClose(filepath.Join(destination, windowsAppName)); err != nil {
		return err
	}
	bundle := filepath.Join(destination, gittool.Directory)
	identity, err := installFileIdentity(bundle, true)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := gittool.CheckWindowsBundle(bundle, false); err != nil {
		return err
	}
	stage, err := makeWindowsPrivateDirectory(destination, ".gitwave-uninstall-")
	if err != nil {
		return err
	}
	saved := filepath.Join(stage, gittool.Directory)
	if err := moveWindowsSetupFile(bundle, saved); err != nil {
		_ = os.Remove(stage)
		return err
	}
	if !installFileIs(saved, identity) {
		err = errWindowsInstallChanged
	} else {
		err = gittool.CheckWindowsBundle(saved, false)
	}
	if err != nil {
		if restoreErr := moveWindowsSetupFile(saved, bundle); restoreErr != nil {
			return errors.Join(err, fmt.Errorf("recover the Git bundle from %s: %w", stage, restoreErr))
		}
		_ = os.Remove(stage)
		return err
	}
	_ = os.RemoveAll(stage)
	return nil
}

func waitForWindowsAppToClose(path string) error {
	deadline := time.Now().Add(6 * time.Second)
	for {
		running, err := windowsAppRunning(path)
		if err != nil || !running {
			return err
		}
		if time.Now().After(deadline) {
			return errors.New("close GitWave before starting setup again")
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func windowsAppRunning(path string) (bool, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		if entry.ProcessID == uint32(os.Getpid()) || !strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), windowsAppName) {
			continue
		}
		process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if err != nil {
			continue
		}
		image, err := windowsProcessImage(process)
		windows.CloseHandle(process)
		if err == nil && sameWindowsFile(image, path) {
			return true, nil
		}
	}
	if errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return false, nil
	}
	return false, err
}

func holdWindowsInstallDirectory(path string) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	file, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return 0, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(file, &info); err != nil ||
		info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		windows.CloseHandle(file)
		return 0, errors.Join(errWindowsInstallChanged, err)
	}
	return file, nil
}

func moveWindowsSetupFile(source, destination string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH)
}

func prepareWindowsRegistration(id, destination, version string) (func() error, func(), error) {
	parent, _, err := registry.CreateKey(registry.CURRENT_USER, uninstallParent, registry.ALL_ACCESS|registry.WOW64_32KEY)
	if err != nil {
		return nil, nil, err
	}
	close := func() { _ = parent.Close() }
	keep := false
	defer func() {
		if !keep {
			close()
		}
	}()
	if exists, matches, err := windowsRegistrationMatches(parent, id, destination); err != nil || (exists && !matches) {
		return nil, nil, errors.Join(errors.New("GitWave is registered at a different installation directory"), err)
	}
	legacy := false
	if id == stableRegistration {
		_, legacy, err = windowsRegistrationMatches(parent, legacyRegistration, destination)
		if err != nil {
			return nil, nil, err
		}
	}
	keep = true
	return func() error { return publishWindowsRegistration(parent, id, destination, version, legacy) }, close, nil
}

func windowsRegistrationMatches(parent registry.Key, id, destination string) (exists, matches bool, err error) {
	key, err := registry.OpenKey(parent, id, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	defer key.Close()
	path, _, err := key.GetStringValue("InstallLocation")
	if errors.Is(err, registry.ErrNotExist) {
		return true, false, nil
	}
	if err != nil {
		return true, false, err
	}
	path = strings.Trim(strings.TrimSpace(path), `"`)
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return true, false, nil
	}
	return true, strings.EqualFold(destination, canonical), nil
}

type windowsRegistrationValue struct {
	name, value string
}

func publishWindowsRegistration(parent registry.Key, id, destination, version string, legacy bool) (err error) {
	if exists, matches, err := windowsRegistrationMatches(parent, id, destination); err != nil || (exists && !matches) {
		return errors.Join(errWindowsInstallChanged, err)
	}
	key, existing, err := registry.CreateKey(parent, id, registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	uninstall := `"` + filepath.Join(destination, installedUninstaller) + `"`
	values := []windowsRegistrationValue{
		{"InstallLocation", destination}, {"DisplayName", "GitWave"}, {"DisplayVersion", version},
		{"DisplayIcon", filepath.Join(destination, windowsAppName)}, {"UninstallString", uninstall}, {"QuietUninstallString", uninstall + " /S"},
	}
	var undo []func() error
	defer func() {
		if err != nil && existing {
			for i := len(undo) - 1; i >= 0; i-- {
				err = errors.Join(err, undo[i]())
			}
		}
		_ = key.Close()
		if err != nil && !existing {
			err = errors.Join(err, registry.DeleteKey(parent, id))
		}
	}()
	for _, value := range values {
		old, kind, err := key.GetStringValue(value.name)
		if err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		undo = append(undo, func() error {
			if errors.Is(err, registry.ErrNotExist) {
				return deleteWindowsValue(key, value.name)
			}
			if kind == registry.EXPAND_SZ {
				return key.SetExpandStringValue(value.name, old)
			}
			return key.SetStringValue(value.name, old)
		})
		if err := key.SetStringValue(value.name, value.value); err != nil {
			return err
		}
	}
	for _, name := range []string{"NoModify", "NoRepair"} {
		old, kind, err := key.GetIntegerValue(name)
		if err != nil && !errors.Is(err, registry.ErrNotExist) {
			return err
		}
		undo = append(undo, func() error {
			if errors.Is(err, registry.ErrNotExist) {
				return deleteWindowsValue(key, name)
			}
			if kind == registry.QWORD {
				return key.SetQWordValue(name, old)
			}
			return key.SetDWordValue(name, uint32(old))
		})
		if err := key.SetDWordValue(name, 1); err != nil {
			return err
		}
	}
	if legacy {
		_, matches, err := windowsRegistrationMatches(parent, legacyRegistration, destination)
		if err != nil || !matches {
			return errors.Join(errWindowsInstallChanged, err)
		}
		if err := registry.DeleteKey(parent, legacyRegistration); err != nil {
			return fmt.Errorf("retire the previous GitWave uninstall entry: %w", err)
		}
	}
	return nil
}

func deleteWindowsValue(key registry.Key, name string) error {
	err := key.DeleteValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}
