package appupdate

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const waitForUpdateParentEnv = "GITWAVE_UPDATE_WAIT_FOR_PARENT"
const updateOriginalTmpEnv = "GITWAVE_UPDATE_ORIGINAL_TMPDIR"

func (a *AppImage) relaunch(ctx context.Context, args []string, pkg Package) (func(bool) error, error) {
	root, err := os.OpenRoot(a.directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	name := filepath.Base(a.path)
	image, err := openAppImage(root, name)
	if err != nil {
		return nil, err
	}
	defer image.Close()
	info, err := image.Stat()
	if err != nil || !os.SameFile(info, a.file) || !imageStillAt(root, name, a.file) {
		return nil, ErrInstallationChanged
	}
	if err := verifyAppImageBytes(ctx, image, pkg); err != nil {
		return nil, err
	}
	if !imageUnmodifiedAt(root, name, a.file) {
		return nil, ErrInstallationChanged
	}
	fd, err := unix.PidfdOpen(os.Getpid(), 0)
	if err != nil {
		return nil, err
	}
	parent := os.NewFile(uintptr(fd), "gitwave-update-parent")
	defer parent.Close()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ready, childReady, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer ready.Close()
	defer childReady.Close()
	launchDir, err := os.MkdirTemp("", ".gitwave-update-launch-")
	if err != nil {
		return nil, err
	}
	// Extraction reads the verified inode via /proc/self/exe. The runtime's
	// FUSE path would resolve the executable back to its mutable on-disk name.
	cmd := exec.Command("/proc/self/fd/4", args...)
	cmd.Args[0] = a.path
	cmd.ExtraFiles = []*os.File{parent, image, childReady}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Env = slices.DeleteFunc(os.Environ(), func(entry string) bool {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "APPIMAGE", "APPDIR", "ARGV0", "OWD", "TARGET_APPIMAGE", "TMPDIR", "APPIMAGE_EXTRACT_AND_RUN",
			waitForUpdateParentEnv, updateOriginalTmpEnv:
			return true
		default:
			return false
		}
	})
	cmd.Env = append(cmd.Env, waitForUpdateParentEnv+"=1", "APPIMAGE_EXTRACT_AND_RUN=1", "TMPDIR="+launchDir,
		updateOriginalTmpEnv+"="+os.Getenv("TMPDIR"))
	if err := cmd.Start(); err != nil {
		_ = os.Remove(launchDir)
		return nil, err
	}
	_ = childReady.Close()
	abort := sync.OnceFunc(func() {
		_ = unix.Kill(-cmd.Process.Pid, unix.SIGKILL)
		go func() { _ = cmd.Wait(); _ = os.RemoveAll(launchDir) }()
	})
	if err := ready.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
		abort()
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = ready.Close() })
	defer stop()
	var signal [1]byte
	if _, err := io.ReadFull(ready, signal[:]); err != nil || signal[0] != 1 || ctx.Err() != nil {
		abort()
		return nil, errors.Join(errors.New("the replacement AppImage did not start"), err, ctx.Err())
	}
	return func(quit bool) error {
		if !quit {
			abort()
		}
		return nil
	}, nil
}

// WaitForUpdateParent keeps the replacement from opening the workspace while
// the old process still has its SQLite profile and repository operations open.
func WaitForUpdateParent() error {
	if os.Getenv(waitForUpdateParentEnv) != "1" {
		return nil
	}
	_ = os.Unsetenv(waitForUpdateParentEnv)
	kind, err := os.Readlink("/proc/self/fd/3")
	if err != nil || (kind != "anon_inode:[pidfd]" && !strings.HasPrefix(kind, "pidfd:[")) {
		return errors.New("the update launch did not receive the previous process handle")
	}
	defer unix.Close(3)
	if inheritedImage, err := os.Readlink("/proc/self/fd/4"); err == nil && inheritedImage == os.Getenv("APPIMAGE") {
		_ = unix.Close(4)
	}
	originalTemp := os.Getenv(updateOriginalTmpEnv)
	_ = os.Unsetenv(updateOriginalTmpEnv)
	_ = os.Unsetenv("APPIMAGE_EXTRACT_AND_RUN")
	if originalTemp == "" {
		_ = os.Unsetenv("TMPDIR")
	} else {
		_ = os.Setenv("TMPDIR", originalTemp)
	}
	ready := os.NewFile(5, "gitwave-update-ready")
	info, err := ready.Stat()
	if err != nil || info.Mode()&os.ModeNamedPipe == 0 {
		return errors.New("the update launch did not receive the readiness pipe")
	}
	_, err = ready.Write([]byte{1})
	_ = ready.Close()
	if err != nil {
		return err
	}
	deadline := time.Now().Add(30 * time.Second)
	fds := []unix.PollFd{{Fd: 3, Events: unix.POLLIN}}
	for time.Now().Before(deadline) {
		_, err := unix.Poll(fds, int(time.Until(deadline).Milliseconds()))
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if fds[0].Revents&(unix.POLLIN|unix.POLLHUP) != 0 {
			return nil
		}
		break
	}
	return errors.New("the previous GitWave process did not exit")
}
