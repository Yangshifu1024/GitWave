//go:build windows

package appupdate

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	windowsAppName    = "GitWave.exe"
	windowsSetupName  = "setup.exe"
	windowsWorkerName = "update-worker.exe"
	windowsStageName  = "gitwave-update-"
	windowsUninstall  = `Software\Microsoft\Windows\CurrentVersion\Uninstall\desktop.gitwave.work`
)

var errWindowsInstallChanged = errors.New("the registered GitWave installation changed")

func InstallerForRunningApp() Installer {
	path, err := os.Executable()
	if err != nil {
		return nil
	}
	installed, _, err := registeredWindowsApp(windowsUninstall)
	if err != nil || !sameWindowsFile(path, installed) {
		return nil
	}
	return func(ctx context.Context, c *Candidate, report func(int64, int64)) (Restart, error) {
		return stageWindowsUpdate(ctx, installed, c, report)
	}
}

func registeredWindowsApp(keyPath string) (path, version string, err error) {
	// NSIS is a 32-bit installer even when its payload targets Windows x64.
	key, err := registry.OpenKey(registry.CURRENT_USER, keyPath, registry.QUERY_VALUE|registry.WOW64_32KEY)
	if err != nil {
		return "", "", err
	}
	defer key.Close()
	dir, _, err := key.GetStringValue("InstallLocation")
	if err != nil {
		return "", "", err
	}
	if !filepath.IsAbs(dir) || strings.ContainsAny(dir, "\"\r\n\x00") {
		return "", "", errWindowsInstallChanged
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return "", "", err
	}
	version, _, err = key.GetStringValue("DisplayVersion")
	if err != nil || !validVersion(version) {
		return "", "", errWindowsInstallChanged
	}
	return filepath.Join(dir, windowsAppName), version, nil
}

func sameWindowsFile(a, b string) bool {
	left, err := windowsFileSnapshot(a)
	if err != nil || !left.Mode().IsRegular() {
		return false
	}
	return windowsPathStillIs(b, left)
}

func windowsFileSnapshot(path string) (os.FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// The identity from File.Stat is bound to the open handle. On Windows,
	// os.Stat can defer reading it until the first os.SameFile comparison.
	return file.Stat()
}

func windowsPathStillIs(path string, expected os.FileInfo) bool {
	current, err := windowsFileSnapshot(path)
	return err == nil && os.SameFile(current, expected)
}

func stageWindowsUpdate(ctx context.Context, installed string, c *Candidate, report func(int64, int64)) (Restart, error) {
	if c == nil {
		return nil, ErrPackage
	}
	name, err := ArtifactName(c.Version(), WindowsX64)
	if err != nil || name != c.artifact {
		return nil, ErrPackage
	}
	current, currentVersion, err := registeredWindowsApp(windowsUninstall)
	if err != nil || !sameWindowsFile(current, installed) {
		return nil, errWindowsInstallChanged
	}
	original, err := windowsFileSnapshot(installed)
	if err != nil {
		return nil, err
	}
	stage, err := makeWindowsUpdateStage()
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			removeWindowsUpdateStage(stage)
		}
	}()
	setup := filepath.Join(stage, windowsSetupName)
	file, err := os.OpenFile(setup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := errors.Join(c.downloadTo(ctx, file, report), file.Close()); err != nil {
		return nil, err
	}
	payload, err := openVerifiedWindowsSetup(setup, c.packageInfo)
	if err != nil {
		return nil, err
	}
	versionErr := verifyWindowsProductVersion(setup, c.Version())
	payload.Close()
	if versionErr != nil {
		return nil, versionErr
	}
	worker := filepath.Join(stage, windowsWorkerName)
	if err := copyWindowsUpdateWorker(installed, worker); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var committed atomic.Bool
	context.AfterFunc(ctx, func() {
		if !committed.Load() {
			removeWindowsUpdateStage(stage)
		}
	})
	keep = true
	request := windowsUpdateRequest{Install: installed, Old: currentVersion, Version: c.Version(), Package: c.packageInfo, Args: os.Args[1:]}
	return func(ctx context.Context) (func(bool) error, error) {
		return startWindowsUpdateWorker(ctx, worker, request, original, &committed)
	}, nil
}

func makeWindowsUpdateStage() (string, error) {
	base, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		return "", err
	}
	return makeWindowsPrivateDirectory(base, windowsStageName)
}

func makeWindowsPrivateDirectory(base, prefix string) (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	security, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return "", err
	}
	path := filepath.Join(base, prefix+rand.Text())
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	attrs := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: security}
	if err := windows.CreateDirectory(name, &attrs); err != nil {
		return "", err
	}
	return path, nil
}

func removeWindowsUpdateStage(stage string) {
	_ = os.Remove(filepath.Join(stage, windowsSetupName))
	_ = os.Remove(filepath.Join(stage, windowsWorkerName))
	_ = os.Remove(stage)
}

func copyWindowsUpdateWorker(installed, worker string) error {
	old, err := lockWindowsUpdateFile(installed)
	if err != nil {
		return err
	}
	defer old.Close()
	if err := verifyWindowsAuthenticode(old); err != nil {
		return err
	}
	if _, err := old.Seek(0, io.SeekStart); err != nil {
		return err
	}
	copy, err := os.OpenFile(worker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(copy, old)
	return errors.Join(err, copy.Close())
}

func lockWindowsUpdateFile(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 {
		windows.CloseHandle(h)
		return nil, errors.Join(errWindowsInstallChanged, err)
	}
	return os.NewFile(uintptr(h), path), nil
}

func openVerifiedWindowsSetup(path string, expected Package) (*os.File, error) {
	if expected.Size <= 0 || expected.Size > maxPackageSize {
		return nil, ErrPackage
	}
	file, err := lockWindowsUpdateFile(path)
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			file.Close()
		}
	}()
	digest := sha256.New()
	n, err := io.Copy(digest, io.LimitReader(file, expected.Size+1))
	if err != nil || n != expected.Size || hex.EncodeToString(digest.Sum(nil)) != expected.SHA256 {
		return nil, errors.Join(ErrPackage, err)
	}
	image, err := pe.NewFile(file)
	if err != nil || image.Characteristics&pe.IMAGE_FILE_EXECUTABLE_IMAGE == 0 || image.Characteristics&pe.IMAGE_FILE_DLL != 0 {
		return nil, errors.Join(ErrPackage, err)
	}
	if err := verifyWindowsAuthenticode(file); err != nil {
		return nil, err
	}
	keep = true
	return file, nil
}

func verifyWindowsExecutableDigest(file *os.File, expected string) error {
	if !validSHA256(expected) {
		return ErrPackage
	}
	digest := sha256.New()
	n, err := io.Copy(digest, io.LimitReader(file, maxPackageSize+1))
	if err != nil || n <= 0 || n > maxPackageSize || hex.EncodeToString(digest.Sum(nil)) != expected {
		return errors.Join(ErrPackage, err)
	}
	return nil
}

func verifyWindowsAuthenticode(file *os.File) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	path, err := windows.UTF16PtrFromString(file.Name())
	if err != nil {
		return err
	}
	info := windows.WinTrustFileInfo{Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})), FilePath: path, File: windows.Handle(file.Fd())}
	data := windows.WinTrustData{
		Size: uint32(unsafe.Sizeof(windows.WinTrustData{})), UIChoice: windows.WTD_UI_NONE,
		UnionChoice: windows.WTD_CHOICE_FILE, FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(&info),
		StateAction: windows.WTD_STATEACTION_VERIFY, ProvFlags: windows.WTD_DISABLE_MD2_MD4 | windows.WTD_CACHE_ONLY_URL_RETRIEVAL,
	}
	err = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &data)
	data.StateAction = windows.WTD_STATEACTION_CLOSE
	closeErr := windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &data)
	if err := errors.Join(err, closeErr); err != nil {
		return fmt.Errorf("could not verify the Windows publisher signature: %w", err)
	}
	return nil
}

func verifyWindowsProductVersion(path, expected string) error {
	var zero windows.Handle
	size, err := windows.GetFileVersionInfoSize(path, &zero)
	if err != nil || size == 0 || size > 1<<20 {
		return errors.Join(ErrPackage, err)
	}
	data := make([]byte, size)
	block := unsafe.Pointer(&data[0])
	if err := windows.GetFileVersionInfo(path, 0, size, block); err != nil {
		return err
	}
	var translation *[2]uint16
	var length uint32
	if err := windows.VerQueryValue(block, `\VarFileInfo\Translation`, unsafe.Pointer(&translation), &length); err != nil || length < 4 {
		return errors.Join(ErrPackage, err)
	}
	key := fmt.Sprintf(`\StringFileInfo\%04x%04x\ProductVersion`, translation[0], translation[1])
	var version *uint16
	if err := windows.VerQueryValue(block, key, unsafe.Pointer(&version), &length); err != nil || length == 0 || length > 128 {
		return errors.Join(ErrPackage, err)
	}
	if got := windows.UTF16ToString(unsafe.Slice(version, length)); got != expected {
		return fmt.Errorf("%w: installed GitWave executable reports version %q, expected %s", ErrPackage, got, expected)
	}
	return nil
}
