//go:build windows

package appupdate

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/mod/semver"
	"golang.org/x/sys/windows"
)

const (
	windowsWorkerArg  = "--gitwave-update-worker="
	windowsCleanupArg = "--gitwave-update-cleanup="
)

type windowsUpdateRequest struct {
	Install, Old, Version string
	Package               Package
	Args                  []string
}

type windowsInstallSnapshot struct {
	file, directory os.FileInfo
}

func (s windowsInstallSnapshot) matches(path string) bool {
	return windowsPathStillIs(path, s.file) && windowsPathStillIs(filepath.Dir(path), s.directory)
}

func startWindowsUpdateWorker(parent context.Context, worker string, request windowsUpdateRequest, original os.FileInfo, committed *atomic.Bool) (func(bool) error, error) {
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	defer cancel()
	if !windowsPathStillIs(request.Install, original) {
		return nil, errWindowsInstallChanged
	}
	copy, err := lockWindowsUpdateFile(worker)
	if err != nil {
		return nil, err
	}
	defer copy.Close()
	if err := verifyWindowsAuthenticode(copy); err != nil {
		return nil, err
	}
	fromParent, toWorker, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer fromParent.Close()
	handedOff := false
	defer func() {
		if !handedOff {
			toWorker.Close()
		}
	}()
	fromWorker, toParent, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer fromWorker.Close()
	defer toParent.Close()
	cmd, old, err := windowsHandoffCommand(worker, windowsWorkerArg)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(old)
	cmd.Stdin, cmd.Stdout = fromParent, toParent
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	fromParent.Close()
	toParent.Close()
	go func() { _ = cmd.Wait() }()
	unblock := context.AfterFunc(ctx, func() {
		_ = fromWorker.Close()
		_ = toWorker.Close()
		_ = cmd.Process.Kill()
	})
	defer unblock()
	if err := json.NewEncoder(toWorker).Encode(request); err != nil {
		return nil, errors.Join(ctx.Err(), err)
	}
	var message string
	if err := json.NewDecoder(io.LimitReader(fromWorker, 4096)).Decode(&message); err != nil {
		return nil, errors.Join(errors.New("the Windows update process did not become ready"), ctx.Err(), err)
	}
	if message != "" {
		return nil, errors.New(message)
	}
	if !unblock() || ctx.Err() != nil {
		return nil, ctx.Err()
	}
	handedOff = true
	return func(quit bool) error {
		defer toWorker.Close()
		if !quit {
			return nil
		}
		installed, version, err := registeredWindowsApp(windowsUninstall)
		if err != nil || version != request.Old || !windowsPathStillIs(installed, original) || !windowsPathStillIs(request.Install, original) {
			return errWindowsInstallChanged
		}
		committed.Store(true)
		if _, err := toWorker.Write([]byte{1}); err != nil {
			committed.Store(false)
			return err
		}
		return nil
	}, nil
}

func windowsHandoffCommand(path, flag string, args ...string) (*exec.Cmd, windows.Handle, error) {
	var handle windows.Handle
	current := windows.CurrentProcess()
	if err := windows.DuplicateHandle(current, current, current, &handle, windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, true, 0); err != nil {
		return nil, 0, err
	}
	arguments := append([]string{flag + strconv.FormatUint(uint64(handle), 10)}, args...)
	cmd := exec.Command(path, arguments...)
	cmd.Dir = filepath.Dir(path)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags:              windows.CREATE_NEW_PROCESS_GROUP,
		AdditionalInheritedHandles: []syscall.Handle{syscall.Handle(handle)},
	}
	return cmd, handle, nil
}

func WaitForUpdateParent() error {
	if len(os.Args) < 2 {
		return nil
	}
	arg := os.Args[1]
	if arg == windowsInstallArg {
		if err := installWindowsPayload(os.Args[2:]); err != nil {
			if path, pathErr := os.Executable(); pathErr == nil {
				_ = os.WriteFile(filepath.Join(filepath.Dir(path), "install-error.txt"), []byte(err.Error()), 0o600)
			}
			return err
		}
		os.Exit(0)
	}
	if arg == windowsUninstallArg {
		path, err := os.Executable()
		if err != nil {
			return err
		}
		directory := filepath.Dir(path)
		if len(os.Args) == 3 {
			directory = os.Args[2]
		}
		if err := prepareWindowsUninstall(directory); err != nil {
			return err
		}
		os.Exit(0)
	}
	if value, ok := strings.CutPrefix(arg, windowsWorkerArg); ok {
		handle, err := windowsInheritedProcess(value)
		if err != nil {
			return err
		}
		err = runWindowsUpdateWorker(handle)
		windows.CloseHandle(handle)
		if err != nil {
			return err
		}
		os.Exit(0)
	}
	if value, ok := strings.CutPrefix(arg, windowsCleanupArg); ok {
		if len(os.Args) < 3 {
			return errors.New("missing Windows update staging directory")
		}
		token := os.Args[2]
		os.Args = append(os.Args[:1], os.Args[3:]...)
		cleanupWindowsUpdateWorker(value, token)
	}
	return nil
}

func windowsInheritedProcess(value string) (windows.Handle, error) {
	parsed, err := strconv.ParseUint(value, 10, strconv.IntSize)
	if err != nil || parsed == 0 {
		return 0, errors.New("invalid Windows update process handle")
	}
	handle := windows.Handle(parsed)
	if err := windows.SetHandleInformation(handle, windows.HANDLE_FLAG_INHERIT, 0); err != nil {
		return 0, err
	}
	return handle, nil
}

func runWindowsUpdateWorker(old windows.Handle) error {
	reader := bufio.NewReader(os.Stdin)
	line, err := bufio.NewReader(io.LimitReader(reader, 256<<10)).ReadBytes('\n')
	if err != nil {
		return err
	}
	var request windowsUpdateRequest
	if err := json.Unmarshal(line, &request); err != nil {
		return err
	}
	stage, pkg, original, err := prepareWindowsUpdateWorker(old, request)
	if pkg != nil {
		defer pkg.Close()
	}
	if err == nil {
		var operation windows.Handle
		operation, err = lockWindowsInstallationUpdate(request.Install)
		if operation != 0 {
			defer windows.CloseHandle(operation)
		}
	}
	response := ""
	if err != nil {
		response = err.Error()
	}
	if replyErr := json.NewEncoder(os.Stdout).Encode(response); replyErr != nil || err != nil {
		return errors.Join(err, replyErr)
	}
	decision, err := reader.ReadByte()
	if errors.Is(err, io.EOF) {
		return nil // The old window kept running and abandoned this attempt.
	}
	if err != nil || decision != 1 {
		return errors.Join(errors.New("invalid Windows update confirmation"), err)
	}
	if err := awaitWindowsUpdateProcess(old, 45*time.Second); err != nil {
		return windowsUpdateFailure(stage, err)
	}
	if err := applyWindowsSetup(stage, request, original, pkg); err != nil {
		return windowsUpdateFailure(stage, err)
	}
	return nil
}

func lockWindowsInstallationUpdate(path string) (windows.Handle, error) {
	return lockWindowsInstallOperation(path, "Update")
}

func lockWindowsInstallOperation(path, operation string) (windows.Handle, error) {
	identity := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(path))))
	name := windows.StringToUTF16Ptr(fmt.Sprintf(`Local\GitWave.%s.%x`, operation, identity))
	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		if handle != 0 {
			windows.CloseHandle(handle)
		}
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
			return 0, errors.New("another GitWave process is already preparing an update for this installation")
		}
		return 0, err
	}
	return handle, nil
}

func prepareWindowsUpdateWorker(old windows.Handle, request windowsUpdateRequest) (string, *os.File, windowsInstallSnapshot, error) {
	var original windowsInstallSnapshot
	worker, err := os.Executable()
	if err != nil {
		return "", nil, original, err
	}
	stage := filepath.Dir(worker)
	if filepath.Base(worker) != windowsWorkerName || !validWindowsStageName(filepath.Base(stage)) ||
		!validSHA256(request.Package.ExecutableSHA256) || !validVersion(request.Old) || !validVersion(request.Version) ||
		semver.Compare("v"+request.Version, "v"+request.Old) <= 0 {
		return "", nil, original, ErrPackage
	}
	oldPath, err := windowsProcessImage(old)
	installed, version, registrationErr := registeredWindowsApp(windowsUninstall)
	if err != nil || registrationErr != nil || version != request.Old ||
		!sameWindowsFile(oldPath, request.Install) || !sameWindowsFile(installed, request.Install) {
		return "", nil, original, errWindowsInstallChanged
	}
	original.file, err = windowsFileSnapshot(installed)
	if err != nil {
		return "", nil, original, err
	}
	original.directory, err = windowsFileSnapshot(filepath.Dir(installed))
	if err != nil {
		return "", nil, original, err
	}
	setup := filepath.Join(stage, windowsSetupName)
	pkg, err := openVerifiedWindowsSetup(setup, request.Package)
	if err != nil {
		return "", nil, original, err
	}
	if err := verifyWindowsProductVersion(setup, request.Version); err != nil {
		pkg.Close()
		return "", nil, original, err
	}
	return stage, pkg, original, nil
}

func applyWindowsSetup(stage string, request windowsUpdateRequest, original windowsInstallSnapshot, pkg *os.File) error {
	installed, version, err := registeredWindowsApp(windowsUninstall)
	if err != nil || version != request.Old || !original.matches(installed) || !original.matches(request.Install) {
		return errWindowsInstallChanged
	}
	// NSIS requires /D to be the last parameter, with the path left unquoted.
	setup := exec.Command(pkg.Name())
	setup.Dir = stage
	setup.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: windows.ComposeCommandLine([]string{pkg.Name(), "/P"}) + " /D=" + filepath.Dir(installed),
	}
	if err := setup.Run(); err != nil {
		return fmt.Errorf("Windows setup failed: %w", err)
	}
	updated, version, err := registeredWindowsApp(windowsUninstall)
	if err != nil || version != request.Version || !strings.EqualFold(updated, installed) {
		return errors.New("the installer did not publish the requested GitWave version")
	}
	replacement, err := lockWindowsUpdateFile(updated)
	if err != nil {
		return err
	}
	defer replacement.Close()
	if err := verifyWindowsExecutableDigest(replacement, request.Package.ExecutableSHA256); err != nil {
		return err
	}
	if err := verifyWindowsAuthenticode(replacement); err != nil {
		return err
	}
	if err := verifyWindowsProductVersion(updated, request.Version); err != nil {
		return err
	}
	cmd, workerHandle, err := windowsHandoffCommand(updated, windowsCleanupArg, append([]string{filepath.Base(stage)}, request.Args...)...)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(workerHandle)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func cleanupWindowsUpdateWorker(value, stageName string) {
	if !validWindowsStageName(stageName) {
		return
	}
	stage := filepath.Join(os.TempDir(), stageName)
	handle, err := windowsInheritedProcess(value)
	if err != nil {
		return
	}
	defer windows.CloseHandle(handle)
	workerImage, err := windowsProcessImage(handle)
	if err == nil && sameWindowsFile(workerImage, filepath.Join(stage, windowsWorkerName)) && awaitWindowsUpdateProcess(handle, 5*time.Second) == nil {
		removeWindowsUpdateStage(stage)
	}
}

func validWindowsStageName(name string) bool {
	token, ok := strings.CutPrefix(name, windowsStageName)
	return ok && len(token) == 26 && !strings.ContainsFunc(token, func(c rune) bool {
		return (c < 'A' || c > 'Z') && (c < '2' || c > '7')
	})
}

func windowsProcessImage(handle windows.Handle) (string, error) {
	var path [32768]uint16
	length := uint32(len(path))
	if err := windows.QueryFullProcessImageName(handle, 0, &path[0], &length); err != nil {
		return "", err
	}
	return windows.UTF16ToString(path[:length]), nil
}

func awaitWindowsUpdateProcess(handle windows.Handle, timeout time.Duration) error {
	state, err := windows.WaitForSingleObject(handle, uint32(timeout.Milliseconds()))
	if err != nil || state != windows.WAIT_OBJECT_0 {
		return errors.Join(errors.New("the previous GitWave process did not exit"), err)
	}
	return nil
}

func windowsUpdateFailure(stage string, err error) error {
	message := fmt.Sprintf("GitWave could not complete the update.\r\nGitWave 无法完成更新。\r\n\r\n%s\r\n\r\n%s", err, stage)
	_, _ = windows.MessageBox(0, windows.StringToUTF16Ptr(message), windows.StringToUTF16Ptr("GitWave"), windows.MB_OK|windows.MB_ICONERROR)
	return err
}
