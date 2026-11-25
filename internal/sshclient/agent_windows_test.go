package sshclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestPreferredGitSSHUsesTheOpenSSHOfTheAgent(t *testing.T) {
	root := t.TempDir()
	git := filepath.Join(root, "cmd", "git.exe")
	ssh := filepath.Join(root, "usr", "bin", "ssh.exe")
	for _, executable := range []string{git, ssh} {
		if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(executable, nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", filepath.Dir(git))
	t.Setenv("SSH_AUTH_SOCK", "/tmp/git-bash-agent")
	if got := PreferredGitSSH(); got != ssh {
		t.Fatalf("Git Bash agent client = %q, want %q", got, ssh)
	}
	if ssh := windowsOpenSSH("ssh"); ssh != "" {
		t.Setenv("SSH_AUTH_SOCK", `\\.\pipe\gitwave-custom-agent`)
		if got := PreferredGitSSH(); got != ssh {
			t.Fatalf("named-pipe agent client = %q, want %q", got, ssh)
		}
	}
}

func TestWindowsAgentListsAndCancelsOverLocalNamedPipe(t *testing.T) {
	path := fmt.Sprintf(`\\.\pipe\gitwave-test-ssh-agent-%d`, os.Getpid())
	t.Setenv("SSH_AUTH_SOCK", path)
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateNamedPipe(name, windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_OVERLAPPED,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_REJECT_REMOTE_CLIENTS, 1, 4096, 4096, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := os.NewFile(uintptr(handle), path)
	defer server.Close()
	if err := server.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	serverResult := make(chan error, 1)
	err = useAgent(ctx, func(conn io.ReadWriter) error {
		go func() {
			var request [5]byte
			if _, err := io.ReadFull(server, request[:]); err != nil {
				serverResult <- err
				return
			}
			if !bytes.Equal(request[:], []byte{0, 0, 0, 1, agentRequestIdentities}) {
				serverResult <- fmt.Errorf("unexpected request: %x", request)
				return
			}
			_, err := server.Write([]byte{0, 0, 0, 5, agentIdentitiesAnswer, 0, 0, 0, 0})
			serverResult <- err
		}()
		keys, err := agentIdentities(conn)
		if err != nil || len(keys) != 0 {
			return fmt.Errorf("list identities: %v, %v", keys, err)
		}
		cancel()
		// The server leaves the pipe open without writing any more bytes.
		_, err = io.ReadFull(conn, make([]byte, 1))
		return err
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected a canceled agent request, got %v", err)
	}
	if err := <-serverResult; err != nil {
		t.Fatal(err)
	}
}
