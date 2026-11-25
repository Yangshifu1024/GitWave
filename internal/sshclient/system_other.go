//go:build !windows

package sshclient

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
)

func systemCommand(name string) (string, error) { return exec.LookPath(name) }

func dialAgent(ctx context.Context) (io.ReadWriteCloser, error) {
	return dialAgentSocket(ctx, os.Getenv("SSH_AUTH_SOCK"))
}

// Git and the settings panel can both use PATH on Unix.
func PreferredGitSSH() string { return "" }

func StartWindowsAgent(context.Context) error {
	return errors.New("the OpenSSH agent service is available on Windows")
}
