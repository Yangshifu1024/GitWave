//go:build unix

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitwave.work/internal/sshclient"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestNativeSSHSettingsLoadsAndRemovesTheSelectedAgentKey(t *testing.T) {
	startSSHAgentFixture(t)
	key := generateSSHFixtureKey(t, "gitwave fixture identity", "")
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node { return newWorkbench(cx).view() },
		nsgotest.Size(1200, 800), nsgotest.Environment(refreshEnvironment(core.LifecycleActive)))
	const timeout = 10 * time.Second
	app.Key("mod+,")
	app.WaitFor("id:settings-ssh", timeout)
	app.Press("id:settings-ssh")
	app.WaitFor("id:ssh-key-list-empty", timeout)
	app.Press("id:ssh-add-open")
	app.WaitFor("id:ssh-key-path", timeout)
	app.Type("id:ssh-key-path", key)
	app.Press("id:ssh-add-apply")
	app.WaitFor("text:gitwave fixture identity", timeout)
	app.WaitForGone("id:ssh-add-form", timeout)
	list, err := sshclient.List(t.Context())
	if err != nil || !list.Running || len(list.Keys) != 1 {
		t.Fatalf("key was not loaded in agent: %+v, %v", list, err)
	}
	app.Press("id:ssh-test-open")
	app.WaitFor("id:ssh-test-form", timeout)
	app.Key("escape")
	app.WaitForGone("id:ssh-test-form", timeout)
	app.WaitFor("id:settings-dialog", timeout)
	app.Press("id:ssh-key-remove")
	app.WaitFor("id:ssh-key-list-empty", timeout)
	if _, err := os.Stat(key); err != nil {
		t.Fatalf("removal affected key file: %v", err)
	}
}

func TestSSHAgentRemovesByPublicIdentityAndRejectsPublicAndEncryptedKeyInput(t *testing.T) {
	startSSHAgentFixture(t)
	second := generateSSHFixtureKey(t, "another identity", "")
	secondPublic, err := os.ReadFile(second + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	comment := second + "\n" + strings.TrimSpace(string(secondPublic))
	first := generateSSHFixtureKey(t, comment, "")
	if err := sshclient.Add(t.Context(), first+".pub"); !errors.Is(err, sshclient.ErrPublicKey) {
		t.Fatalf("public key input returned %v", err)
	}
	protected := generateSSHFixtureKey(t, "encrypted fixture", "fixture-passphrase")
	if err := sshclient.Add(t.Context(), protected); !errors.Is(err, sshclient.ErrPassphrase) {
		t.Fatalf("encrypted key returned %v", err)
	}
	for _, path := range []string{first, second} {
		if err := sshclient.Add(t.Context(), path); err != nil {
			t.Fatal(err)
		}
	}
	listing, err := sshclient.List(t.Context())
	if err != nil || len(listing.Keys) != 2 {
		t.Fatalf("keys = %+v, %v", listing, err)
	}
	var selected string
	for _, key := range listing.Keys {
		if key.Comment == comment {
			selected = key.Fingerprint
		}
	}
	if selected == "" {
		t.Fatal("agent did not preserve the path-shaped multiline comment")
	}
	if err := sshclient.Remove(t.Context(), selected); err != nil {
		t.Fatal(err)
	}
	remaining, err := sshclient.List(t.Context())
	if err != nil || len(remaining.Keys) != 1 || remaining.Keys[0].Comment != "another identity" {
		t.Fatalf("removed the wrong identity: %+v, %v", remaining, err)
	}
}

func startSSHAgentFixture(t *testing.T) {
	t.Helper()
	for _, name := range []string{"ssh-agent", "ssh-add", "ssh-keygen"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("OpenSSH fixture needs %s", name)
		}
	}
	dir, err := os.MkdirTemp("/tmp", "gws-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "agent")
	t.Setenv("SSH_AUTH_SOCK", socket)
	ctx, cancel := context.WithCancel(t.Context())
	command := exec.CommandContext(ctx, "ssh-agent", "-D", "-a", socket)
	if err := command.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = command.Wait() })
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if stat, err := os.Stat(socket); err == nil && stat.Mode()&os.ModeSocket != 0 {
			return
		}
	}
	t.Fatal("isolated ssh-agent did not open its socket")
}

func generateSSHFixtureKey(t *testing.T, comment, passphrase string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture key")
	cmd := exec.CommandContext(t.Context(), "ssh-keygen", "-q", "-t", "ed25519", "-N", passphrase, "-C", comment, "-f", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate fixture key: %v: %s", err, output)
	}
	return path
}
