package externaltools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestEditorShimUsesWorkingDirectoryWithoutParsingItsName(t *testing.T) {
	system, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(t.TempDir(), "sample repo & notes %pending%")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "editor shim.cmd")
	if err := os.WriteFile(script, []byte("@echo off\r\necho \"%~1\"\r\necho \"%CD%\"\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := editorShimCommand(filepath.Join(system, "cmd.exe"), script, repo)
	cmd.Dir = repo
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run editor shim: %v\n%s", err, output)
	}
	want := "\".\"\r\n\"" + repo + "\""
	if !strings.EqualFold(strings.TrimSpace(string(output)), want) {
		t.Fatalf("launch recorded %q, want %q", output, want)
	}

	// The shim only echoes its argument, so this path does not need a share to exist.
	share := `\\server\share\sample repo & notes %pending% !draft!`
	cmd = editorShimCommand(filepath.Join(system, "cmd.exe"), script, share)
	output, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run editor shim for a network directory: %v\n%s", err, output)
	}
	want = `"` + share + "\"\r\n\"" + system + `"`
	if !strings.EqualFold(strings.TrimSpace(string(output)), want) {
		t.Fatalf("network launch recorded %q, want %q", output, want)
	}
}
