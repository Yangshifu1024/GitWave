package externaltools

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func editorCommand(editor Editor, directory string) *exec.Cmd {
	var candidates []string
	var executable, shim string
	switch editor.ID {
	case "vscode":
		candidates = []string{
			installedPath("LOCALAPPDATA", "Programs", "Microsoft VS Code", "Code.exe"),
			installedPath("ProgramFiles", "Microsoft VS Code", "Code.exe"),
			installedPath("ProgramFiles(x86)", "Microsoft VS Code", "Code.exe"),
		}
		executable, shim = "code.exe", "code.cmd"
	case "zed":
		candidates = []string{installedPath("LOCALAPPDATA", "Programs", "Zed", "Zed.exe")}
		executable = "zed.exe"
	case "vscodium":
		candidates = []string{
			installedPath("LOCALAPPDATA", "Programs", "VSCodium", "VSCodium.exe"),
			installedPath("ProgramFiles", "VSCodium", "VSCodium.exe"),
		}
		executable, shim = "codium.exe", "codium.cmd"
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return exec.Command(candidate, directory)
		}
	}
	if path, err := exec.LookPath(executable); err == nil {
		return exec.Command(path, directory)
	}
	if script, err := exec.LookPath(shim); err == nil && filepath.IsAbs(script) {
		if system, err := windows.GetSystemDirectory(); err == nil {
			return editorShimCommand(filepath.Join(system, "cmd.exe"), script, directory)
		}
	}
	return nil
}

func installedPath(env string, parts ...string) string {
	root := os.Getenv(env)
	if !filepath.IsAbs(root) {
		return ""
	}
	return filepath.Join(append([]string{root}, parts...)...)
}

func editorShimCommand(interpreter, script, directory string) *exec.Cmd {
	cmd := exec.Command(interpreter)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW,
		CmdLine: `cmd.exe /D /V:OFF /S /C ""%GITWAVE_EDITOR_SHIM%" ".""`}
	cmd.Env = append(os.Environ(), "GITWAVE_EDITOR_SHIM="+script)
	if isNetworkDirectory(directory) {
		// cmd.exe cannot keep a UNC working directory. Give the editor the UNC path directly.
		cmd.Dir = filepath.Dir(interpreter)
		cmd.SysProcAttr.CmdLine = `cmd.exe /D /V:OFF /S /C ""%GITWAVE_EDITOR_SHIM%" "%GITWAVE_REPO_DIR%""`
		cmd.Env = append(cmd.Env, "GITWAVE_REPO_DIR="+directory)
	}
	return cmd
}

func terminalCommand(directory string) *exec.Cmd {
	if path, err := exec.LookPath("wt.exe"); err == nil && filepath.IsAbs(path) {
		cmd := exec.Command(path, "-d", ".")
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
		return cmd
	}
	if system, err := windows.GetSystemDirectory(); err == nil {
		cmd := exec.Command(filepath.Join(system, "cmd.exe"))
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_CONSOLE}
		if isNetworkDirectory(directory) {
			cmd.Dir = system
			cmd.SysProcAttr.CmdLine = `cmd.exe /D /E:ON /V:OFF /S /K "pushd "%GITWAVE_REPO_DIR%""`
			cmd.Env = append(os.Environ(), "GITWAVE_REPO_DIR="+directory)
		}
		return cmd
	}
	return nil
}

func isNetworkDirectory(directory string) bool {
	volume := strings.ToUpper(filepath.VolumeName(directory))
	if strings.HasPrefix(volume, `\\?\`) || strings.HasPrefix(volume, `\\.\`) {
		return strings.HasPrefix(volume[4:], `UNC\`)
	}
	return strings.HasPrefix(volume, `\\`)
}
