package sshclient

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitwave.work/internal/process"
)

func TestMSYSAgentAddressUsesWindowsSocketDescriptor(t *testing.T) {
	port, cookie, err := msysAgentAddress([]byte("!<socket >13330 s 04030201-08070605-0c0b0a09-100f0e0d\x00"))
	if err != nil || port != 0x1234 || cookie != [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16} {
		t.Fatalf("unexpected descriptor: port %d, cookie %x, error %v", port, cookie, err)
	}
	if _, _, err := msysAgentAddress([]byte("!<socket >13330 d 04030201-08070605-0c0b0a09-100f0e0d\x00")); err == nil {
		t.Fatal("accepted a datagram endpoint")
	}
}

func TestGitBashSSHAgentPreservesMultilineIdentity(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		if os.Getenv("GITWAVE_TEST_REQUIRE_MSYS_AGENT") == "1" {
			t.Fatal(err)
		}
		t.Skip("Git for Windows is needed for this fixture")
	}
	root := filepath.Dir(filepath.Dir(git))
	if strings.HasPrefix(strings.ToLower(filepath.Base(root)), "mingw") {
		root = filepath.Dir(root)
	}
	bin := filepath.Join(root, "usr", "bin")
	for _, name := range []string{"ssh-agent.exe", "ssh-keygen.exe", "ssh-add.exe", "cygpath.exe", "id.exe"} {
		if _, err := os.Stat(filepath.Join(bin, name)); err != nil {
			if os.Getenv("GITWAVE_TEST_REQUIRE_MSYS_AGENT") == "1" {
				t.Fatal(err)
			}
			t.Skip("the Git for Windows installation has no full MSYS OpenSSH toolset")
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	ctx, stop := context.WithTimeout(t.Context(), 20*time.Second)
	defer stop()
	dir, err := os.MkdirTemp("", "gw-ssh-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	posixDir, err := msysAgentValue(ctx, filepath.Join(bin, "cygpath.exe"), "-au", "--", dir)
	if err != nil {
		t.Fatal(err)
	}
	const socketName = "a"
	t.Setenv("SSH_AUTH_SOCK", posixDir+"/"+socketName)
	command := exec.CommandContext(ctx, filepath.Join(bin, "ssh-agent.exe"), "-D", "-a", os.Getenv("SSH_AUTH_SOCK"))
	process.Hide(command)
	logFile, err := os.Create(filepath.Join(dir, "agent.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	command.Stderr = logFile
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var agentErr error
	go func() {
		agentErr = command.Wait()
		close(done)
	}()
	defer func() { stop(); <-done }()
	for {
		if _, err := os.Stat(filepath.Join(dir, socketName)); err == nil {
			break
		}
		select {
		case <-done:
			logBytes, _ := os.ReadFile(logFile.Name())
			t.Fatalf("the isolated Git Bash ssh-agent exited: %v: %s", agentErr, logBytes)
		case <-ctx.Done():
			t.Fatal("the isolated Git Bash ssh-agent did not create its socket")
		case <-time.After(25 * time.Millisecond):
		}
	}
	key := filepath.Join(dir, "key")
	const comment = "First line\nSecond line"
	if _, err := msysAgentValue(ctx, filepath.Join(bin, "ssh-keygen.exe"), "-q", "-t", "ed25519", "-N", "", "-C", comment, "-f", key); err != nil {
		t.Fatal(err)
	}
	if err := Add(ctx, key); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{posixDir + "/" + socketName, filepath.Join(dir, socketName)} {
		t.Setenv("SSH_AUTH_SOCK", path)
		listing, err := List(ctx)
		if err != nil || !listing.Running || len(listing.Keys) != 1 || listing.Keys[0].Comment != comment {
			t.Fatalf("list Git Bash agent using %q: %+v, %v", path, listing, err)
		}
		if path == filepath.Join(dir, socketName) {
			if err := Remove(ctx, listing.Keys[0].Fingerprint); err != nil {
				t.Fatal(fmt.Errorf("remove Git Bash identity: %w", err))
			}
		}
	}
	if listing, err := List(ctx); err != nil || !listing.Running || len(listing.Keys) != 0 {
		t.Fatalf("Git Bash agent after removing identity: %+v, %v", listing, err)
	}
}
