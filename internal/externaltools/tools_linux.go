package externaltools

import (
	"os"
	"os/exec"
)

func editorCommand(editor Editor, directory string) *exec.Cmd {
	program := map[string]string{"vscode": "code", "zed": "zed", "vscodium": "codium"}[editor.ID]
	if path, err := exec.LookPath(program); err == nil {
		return detachedCommand(path, directory)
	}
	return nil
}

func terminalCommand(directory string) *exec.Cmd {
	if configured := os.Getenv("TERMINAL"); configured != "" {
		return detachedCommand(configured)
	}
	for _, terminal := range []struct {
		name string
		args []string
	}{
		{"gnome-terminal", []string{"--working-directory", directory}},
		{"konsole", []string{"--workdir", directory}},
		{"xfce4-terminal", []string{"--working-directory", directory}},
		{"kgx", []string{"--working-directory", directory}},
		{"kitty", nil},
		{"alacritty", nil},
		{"foot", nil},
		{"wezterm", []string{"start", "--cwd", directory}},
	} {
		if path, err := exec.LookPath(terminal.name); err == nil {
			return detachedCommand(path, terminal.args...)
		}
	}
	return nil
}
