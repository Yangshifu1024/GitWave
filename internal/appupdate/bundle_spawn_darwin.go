//go:build darwin && !ios

package appupdate

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/unix"
)

func spawnVerifiedMacExecutable(path, expectedHash string, args []string) (*os.Process, error) {
	argv, err := syscall.SlicePtrFromStrings(append([]string{path}, args...))
	if err != nil {
		return nil, err
	}
	env := os.Environ()
	filtered := env[:0]
	for _, entry := range env {
		if !strings.HasPrefix(entry, "DYLD_") {
			filtered = append(filtered, entry)
		}
	}
	envp, err := syscall.SlicePtrFromStrings(filtered)
	if err != nil {
		return nil, err
	}

	var initAttributes, destroyAttributes, initActions, destroyActions func(*uintptr) int32
	var setFlags func(*uintptr, int16) int32
	var openFile func(*uintptr, int32, string, int32, uint16) int32
	var spawn func(*int32, string, *uintptr, *uintptr, **byte, **byte) int32
	purego.RegisterLibFunc(&initAttributes, purego.RTLD_DEFAULT, "posix_spawnattr_init")
	purego.RegisterLibFunc(&destroyAttributes, purego.RTLD_DEFAULT, "posix_spawnattr_destroy")
	purego.RegisterLibFunc(&setFlags, purego.RTLD_DEFAULT, "posix_spawnattr_setflags")
	purego.RegisterLibFunc(&initActions, purego.RTLD_DEFAULT, "posix_spawn_file_actions_init")
	purego.RegisterLibFunc(&destroyActions, purego.RTLD_DEFAULT, "posix_spawn_file_actions_destroy")
	purego.RegisterLibFunc(&openFile, purego.RTLD_DEFAULT, "posix_spawn_file_actions_addopen")
	purego.RegisterLibFunc(&spawn, purego.RTLD_DEFAULT, "posix_spawn")

	var attributes, actions uintptr
	if result := initAttributes(&attributes); result != 0 {
		return nil, unix.Errno(result)
	}
	defer destroyAttributes(&attributes)
	const (
		spawnStartSuspended  = 0x0080
		spawnNewSession      = 0x0400
		spawnCloseFdsDefault = 0x4000
	)
	if result := setFlags(&attributes, spawnStartSuspended|spawnNewSession|spawnCloseFdsDefault); result != 0 {
		return nil, unix.Errno(result)
	}
	if result := initActions(&actions); result != 0 {
		return nil, unix.Errno(result)
	}
	defer destroyActions(&actions)
	for fd := int32(0); fd < 3; fd++ {
		if result := openFile(&actions, fd, os.DevNull, unix.O_RDWR, 0); result != 0 {
			return nil, unix.Errno(result)
		}
	}
	var pid int32
	if result := spawn(&pid, path, &actions, &attributes, &argv[0], &envp[0]); result != 0 {
		return nil, fmt.Errorf("launch GitWave: %w", unix.Errno(result))
	}
	process, err := os.FindProcess(int(pid))
	if err != nil {
		_ = unix.Kill(int(pid), unix.SIGKILL)
		return nil, err
	}
	resume := false
	defer func() {
		if !resume {
			_ = process.Kill()
			_, _ = process.Wait()
		}
	}()
	// Darwin selects the executable and its code signature before returning from
	// the suspended spawn. Compare that process's identity before it can run.
	hash, err := macProcessCodeHash(process.Pid)
	if err != nil || hash != expectedHash {
		return nil, errors.Join(errors.New("the executable selected for restart does not match the verified replacement"), err)
	}
	if err := process.Signal(unix.SIGCONT); err != nil {
		return nil, err
	}
	resume = true
	return process, nil
}
