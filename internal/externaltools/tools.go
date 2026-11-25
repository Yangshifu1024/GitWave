// Package externaltools opens the working directory in a desktop application.
package externaltools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type Editor struct{ ID, Name string }

var supportedEditors = []Editor{
	{"vscode", "Visual Studio Code"},
	{"zed", "Zed"},
	{"vscodium", "VSCodium"},
}

func Editors() []Editor {
	var installed []Editor
	for _, editor := range supportedEditors {
		if editorCommand(editor, "") != nil {
			installed = append(installed, editor)
		}
	}
	return installed
}

func OpenEditor(ctx context.Context, directory, id string) error {
	for _, editor := range supportedEditors {
		if editor.ID == id {
			if cmd := editorCommand(editor, directory); cmd != nil {
				return start(ctx, cmd, directory)
			}
			break
		}
	}
	return fmt.Errorf("editor not found: %s", id)
}

func OpenTerminal(ctx context.Context, directory string) error {
	cmd := terminalCommand(directory)
	if cmd == nil {
		return errors.New("no terminal emulator found")
	}
	return start(ctx, cmd, directory)
}

func start(ctx context.Context, cmd *exec.Cmd, directory string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if cmd.Err != nil {
		return cmd.Err
	}
	if !filepath.IsAbs(cmd.Path) {
		return fmt.Errorf("not an absolute program path: %s", cmd.Path)
	}
	if !filepath.IsAbs(directory) {
		return fmt.Errorf("not an absolute directory: %s", directory)
	}
	info, err := os.Stat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", directory)
	}
	if cmd.Dir == "" {
		cmd.Dir = directory
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch %s: %w", filepath.Base(cmd.Path), err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
