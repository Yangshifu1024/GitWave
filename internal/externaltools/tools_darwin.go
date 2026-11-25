package externaltools

import (
	"os"
	"os/exec"
	"path/filepath"
)

func application(name string) string {
	paths := []string{filepath.Join("/Applications", name+".app")}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, "Applications", name+".app"))
	}
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			return path
		}
	}
	return ""
}

func editorCommand(editor Editor, directory string) *exec.Cmd {
	if path := application(editor.Name); path != "" {
		return detachedCommand("/usr/bin/open", "-a", path, directory)
	}
	return nil
}

func terminalCommand(directory string) *exec.Cmd {
	path := application("iTerm")
	if path == "" {
		path = "/System/Applications/Utilities/Terminal.app"
	}
	return detachedCommand("/usr/bin/open", "-a", path, directory)
}
