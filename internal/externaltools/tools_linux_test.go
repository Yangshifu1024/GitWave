package externaltools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestExternalToolsUseTheSelectedWorkingDirectory(t *testing.T) {
	bin := t.TempDir()
	repo := filepath.Join(t.TempDir(), "sample repo & notes")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	const fixture = "#!/bin/sh\nprintf '%s\\n' \"$PWD\" \"$@\" > \"$GITWAVE_TOOL_RESULT\"\n"
	for _, tool := range []string{"code", "zed", "codium", "gnome-terminal", "kitty"} {
		if err := os.WriteFile(filepath.Join(bin, tool), []byte(fixture), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	t.Setenv("TERMINAL", "")
	if editors := Editors(); !slices.Equal(editors, supportedEditors) {
		t.Fatalf("unexpected editor order: %+v", editors)
	}
	for _, test := range []struct {
		name, terminal, want string
		open                 func(context.Context, string) error
	}{
		{"editor", "", repo + "\n" + repo + "\n", func(ctx context.Context, dir string) error { return OpenEditor(ctx, dir, "vscode") }},
		{"terminal", "", repo + "\n--working-directory\n" + repo + "\n", OpenTerminal},
		{"configured terminal", "kitty", repo + "\n", OpenTerminal},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := filepath.Join(t.TempDir(), "launch")
			t.Setenv("GITWAVE_TOOL_RESULT", result)
			t.Setenv("TERMINAL", test.terminal)
			if err := test.open(t.Context(), repo); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if data, err := os.ReadFile(result); err == nil && len(data) != 0 {
					if string(data) != test.want {
						t.Fatalf("launch recorded %q, want %q", data, test.want)
					}
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatal("external tool did not start")
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := OpenTerminal(ctx, repo); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled launch returned %v", err)
	}
}
