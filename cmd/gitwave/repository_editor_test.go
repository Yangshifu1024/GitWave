package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeGitignoreEditorAndIgnoreScope(t *testing.T) {
	dir, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Repository tools fixture")
	folder := filepath.Join(dir, "generated")
	if err := os.Mkdir(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"generated/one.log", "generated/two.log", "keep.log"} {
		if err := os.WriteFile(filepath.Join(dir, path), []byte("untracked contents"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app := refTestApp(t, dir)
	const timeout = 10 * time.Second
	app.WaitFor("label:/^Repository tools fixture,/", timeout)
	app.Press("id:navigate-changes")
	app.WaitFor("label:Unstaged · generated/one.log", timeout)
	app.Press("id:edit-gitignore")
	app.WaitFor("id:gitignore-text", timeout)
	app.Type("id:gitignore-text", "# Keep this line")
	app.Press("label:Cancel")
	app.WaitForGone("id:gitignore-dialog", timeout)
	path := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("cancelled .gitignore edit wrote a file: %v", err)
	}
	app.Press("id:edit-gitignore")
	app.WaitFor("id:gitignore-text", timeout)
	app.Type("id:gitignore-text", "# Keep this line")
	app.Press("id:repo-editor-save")
	app.WaitForGone("id:gitignore-dialog", timeout)
	app.WaitFor("label:Unstaged · .gitignore", timeout)
	app.RightClick("label:Unstaged · generated/one.log")
	app.ChoosePopup("Add to .gitignore…")
	app.WaitFor("id:ignore-dialog", timeout)
	app.Press("id:ignore-directory")
	app.Press("id:ignore-add")
	app.WaitForGone("id:ignore-dialog", timeout)
	app.WaitForGone("label:Unstaged · generated/one.log", timeout)
	app.WaitForGone("label:Unstaged · generated/two.log", timeout)
	app.WaitFor("label:Unstaged · keep.log", timeout)
	if raw, err := os.ReadFile(path); err != nil || string(raw) != "# Keep this line\n/generated/\n" {
		t.Fatalf("unexpected .gitignore contents: %q, %v", raw, err)
	}
	if raw, err := os.ReadFile(filepath.Join(folder, "one.log")); err != nil || string(raw) != "untracked contents" {
		t.Fatalf("ignoring modified the untracked file: %q, %v", raw, err)
	}
}

func TestNativeHooksEditorSavesAndSwitchesScripts(t *testing.T) {
	dir, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Hooks fixture")
	git("", "config", "core.hooksPath", "configured/hooks")
	app := refTestApp(t, dir)
	const timeout = 10 * time.Second
	app.WaitFor("label:/^Hooks fixture,/", timeout)
	app.ChooseMenu("Repository", "Hooks")
	app.WaitFor("id:hooks-dialog", timeout)
	app.WaitFor("id:hook-pre-commit", timeout)
	app.Press("id:hook-pre-commit")
	app.WaitFor("id:hook-insert-sample", timeout)
	app.Press("id:hook-insert-sample")
	app.Press("id:repo-editor-save")
	path := filepath.Join(dir, "configured", "hooks", "pre-commit")
	app.WaitUntil(timeout, func() bool {
		data, err := os.ReadFile(path)
		return err == nil && strings.HasPrefix(string(data), "#!/bin/sh\n# GitWave sample pre-commit hook.")
	})
	app.WaitUntil(timeout, func() bool { return app.Enabled("id:hook-post-checkout") })
	app.Press("id:hook-post-checkout")
	app.WaitUntil(timeout, func() bool {
		return app.Exists("id:hook-text") && app.Find("id:hook-text").Text == ""
	})
	app.Type("id:hook-text", "# Unsaved second hook")
	app.Press("id:repo-editor-close")
	app.WaitForGone("id:hooks-dialog", timeout)
	if _, err := os.Stat(filepath.Join(dir, "configured", "hooks", "post-checkout")); !os.IsNotExist(err) {
		t.Fatalf("closing the editor saved an unsaved hook: %v", err)
	}
}
