package sshclient

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf16"

	"gitwave.work/internal/gittool"
	"gitwave.work/internal/process"
	"golang.org/x/sys/windows"
)

const localAgentPipe = `\\.\pipe\`

func windowsOpenSSH(name string) string {
	if system, err := windows.GetSystemDirectory(); err == nil {
		path := filepath.Join(system, "OpenSSH", name+".exe")
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path
		}
	}
	return ""
}

func systemCommand(name string) (string, error) {
	if filepath.Base(name) != name {
		return exec.LookPath(name)
	}
	socket := os.Getenv("SSH_AUTH_SOCK")
	useWindowsAgent := socket == "" || strings.HasPrefix(strings.ToLower(socket), localAgentPipe)
	system := windowsOpenSSH(name)
	if useWindowsAgent && system != "" {
		return system, nil
	}

	path, err := exec.LookPath(name)
	if err != nil && !errors.Is(err, exec.ErrNotFound) {
		return path, err
	}
	if !useWindowsAgent && (path == "" || strings.EqualFold(path, system)) {
		// A Git Bash socket can use a runtime-specific /tmp or /home mount.
		// Reuse the Git for Windows installation that started that agent.
		if path := systemGitSSH(name); path != "" {
			return path, nil
		}
	}
	if err == nil {
		return path, nil
	}
	if bundled, bundleErr := gittool.WindowsSSH(name); bundleErr == nil {
		return bundled, nil
	}
	return "", err
}

func systemGitSSH(name string) string {
	git, err := exec.LookPath("git")
	if err != nil {
		return ""
	}
	root := filepath.Dir(filepath.Dir(git))
	switch strings.ToLower(filepath.Base(root)) {
	case "mingw32", "mingw64", "ucrt64":
		root = filepath.Dir(root)
	}
	path, _ := exec.LookPath(filepath.Join(root, "usr", "bin", name+".exe"))
	return path
}

func dialAgent(ctx context.Context) (io.ReadWriteCloser, error) {
	path := os.Getenv("SSH_AUTH_SOCK")
	if path == "" {
		path = localAgentPipe + "openssh-ssh-agent"
	}
	if !strings.HasPrefix(strings.ToLower(path), localAgentPipe) {
		if strings.HasPrefix(filepath.VolumeName(path), `\`) {
			return nil, errors.New("the SSH agent must use a local pipe or socket")
		}
		return dialWindowsAgentSocket(ctx, path)
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	flags := uint32(windows.FILE_FLAG_OVERLAPPED | windows.SECURITY_SQOS_PRESENT | windows.SECURITY_IDENTIFICATION)
	for {
		handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, flags, 0)
		switch {
		case err == nil:
			return os.NewFile(uintptr(handle), path), nil
		case errors.Is(err, windows.ERROR_FILE_NOT_FOUND), errors.Is(err, windows.ERROR_PATH_NOT_FOUND):
			return nil, ErrAgentUnavailable
		case !errors.Is(err, windows.ERROR_PIPE_BUSY):
			return nil, fmt.Errorf("connect to ssh-agent: %w", err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(25 * time.Millisecond):
		}
	}
}

// Use the same client as the SSH panel, with core.sshCommand taking precedence.
func PreferredGitSSH() string {
	path, _ := systemCommand("ssh")
	return path
}

// StartWindowsAgent asks for one UAC approval for the two SCM operations.
// The elevated script uses the system sc.exe without a command shell.
func StartWindowsAgent(parent context.Context) error {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	powershell := filepath.Join(system, "WindowsPowerShell", "v1.0", "powershell.exe")
	sc := quotePowerShell(filepath.Join(system, "sc.exe"))
	service := fmt.Sprintf(`& %s config ssh-agent start= auto
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
& %s start ssh-agent
if ($LASTEXITCODE -eq 1056) { exit 0 }
exit $LASTEXITCODE`, sc, sc)
	script := fmt.Sprintf(`$ErrorActionPreference = 'Stop'
$worker = Microsoft.PowerShell.Management\Start-Process -FilePath %s -WorkingDirectory %s -Verb RunAs -WindowStyle Hidden -Wait -PassThru -ArgumentList '-NoLogo -NoProfile -NonInteractive -EncodedCommand %s'
exit $worker.ExitCode`, quotePowerShell(powershell), quotePowerShell(system), encodePowerShell(service))
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, powershell, "-NoLogo", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodePowerShell(script))
	cmd.Dir = system
	cmd.Env = append(os.Environ(), "PSModulePath="+filepath.Join(filepath.Dir(powershell), "Modules"))
	process.Hide(cmd)
	cmd.WaitDelay = time.Second
	var output boundedOutput
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("start the OpenSSH agent service: %w", withDiagnostic(err, output.String()))
	}
	return nil
}

func quotePowerShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func encodePowerShell(script string) string {
	var bytes []byte
	for _, unit := range utf16.Encode([]rune(script)) {
		bytes = binary.LittleEndian.AppendUint16(bytes, unit)
	}
	return base64.StdEncoding.EncodeToString(bytes)
}
