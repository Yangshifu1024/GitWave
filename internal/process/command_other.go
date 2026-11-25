//go:build !unix && !windows

package process

import "os/exec"

const SupportsGroupCancellation = false

func SetGroupCancellation(*exec.Cmd) {}

func Hide(*exec.Cmd) {}

func Run(cmd *exec.Cmd) error { return cmd.Run() }
