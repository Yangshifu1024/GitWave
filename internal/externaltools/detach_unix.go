//go:build linux || darwin

package externaltools

import (
	"os/exec"
	"syscall"
)

func detachedCommand(program string, args ...string) *exec.Cmd {
	cmd := exec.Command(program, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}
