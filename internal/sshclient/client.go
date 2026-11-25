package sshclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"gitwave.work/internal/process"
)

const agentTimeout = 15 * time.Second

var (
	ErrAgentUnavailable = errors.New("ssh-agent is not running or cannot be reached")
	ErrPassphrase       = errors.New("load this passphrase-protected key in a terminal with ssh-add first")
	ErrPublicKey        = errors.New("choose the private key without the .pub extension")
	ErrKeyChanged       = errors.New("the selected key is no longer loaded in ssh-agent")
)

type Key struct {
	Fingerprint string
	Comment     string
	public      []byte
}

type Listing struct {
	Running bool
	Keys    []Key
}

func Add(ctx context.Context, path string) error {
	path, err := resolveKeyPath(path)
	if err != nil {
		return err
	}
	if strings.EqualFold(filepath.Ext(path), ".pub") {
		return ErrPublicKey
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("open private key: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("choose a private key file")
	}
	_, diagnostic, err := run(ctx, agentTimeout, "ssh-add", "--", path)
	if err == nil {
		return nil
	}
	if agentUnavailable(err, diagnostic) {
		return ErrAgentUnavailable
	}
	if strings.Contains(strings.ToLower(diagnostic), "passphrase") {
		return ErrPassphrase
	}
	return withDiagnostic(err, diagnostic)
}

func resolveKeyPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || strings.ContainsAny(path, "\x00\r\n") {
		return "", errors.New("enter the path to a private key")
	}
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimLeft(strings.TrimPrefix(path, "~"), "/\\"))
	}
	return filepath.Abs(path)
}

func agentUnavailable(err error, diagnostic string) bool {
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 2 {
		return true
	}
	diagnostic = strings.ToLower(diagnostic)
	return strings.Contains(diagnostic, "could not open a connection to your authentication agent") ||
		strings.Contains(diagnostic, "error connecting to agent")
}

func run(parent context.Context, timeout time.Duration, name string, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	exe, err := systemCommand(name)
	if err != nil {
		return "", "", fmt.Errorf("find %s: %w", name, err)
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	process.Hide(cmd)
	cmd.WaitDelay = time.Second
	cmd.Env = append(os.Environ(), "LC_ALL=C", "SSH_ASKPASS_REQUIRE=never", "SSH_ASKPASS=")
	var out, diagnostic boundedOutput
	cmd.Stdout, cmd.Stderr = &out, &diagnostic
	err = process.Run(cmd)
	if ctx.Err() != nil {
		return "", "", ctx.Err()
	}
	if out.truncated && err == nil {
		return "", "", errors.New("OpenSSH returned too much output")
	}
	return out.String(), diagnostic.String(), err
}

type boundedOutput struct {
	buffer    bytes.Buffer
	truncated bool
}

func (o *boundedOutput) String() string { return o.buffer.String() }

func (o *boundedOutput) Write(p []byte) (int, error) {
	const maxOutput = 4 << 20
	n := min(len(p), max(0, maxOutput-o.buffer.Len()))
	_, _ = o.buffer.Write(p[:n])
	o.truncated = o.truncated || n != len(p)
	return len(p), nil
}

func withDiagnostic(err error, diagnostic string) error {
	if err == nil {
		return nil
	}
	if text := displayText(diagnostic); text != "" {
		return fmt.Errorf("%w: %s", err, text)
	}
	return err
}

func displayText(s string) string {
	const maxRunes = 4096
	var text strings.Builder
	count := 0
	for _, r := range strings.TrimSpace(s) {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			continue
		}
		if count == maxRunes {
			text.WriteRune('…')
			break
		}
		text.WriteRune(r)
		count++
	}
	return text.String()
}
