package desktopgtk

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestPackagedAppRejectsBrokenGTKBeforeUpdateHandoff(t *testing.T) {
	executable := os.Getenv("GITWAVE_TEST_PACKAGED_GTK")
	if executable == "" {
		t.Skip("requires the packaged executable on a system with GTK older than 4.14")
	}
	var exit *exec.ExitError
	if err := exec.CommandContext(t.Context(), executable, probeArg).Run(); !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("the fixture must have an older system GTK: %v", err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, message string }{
		{"no runtime", "the bundled GTK runtime is missing"},
		{"no library", "load the bundled GTK runtime"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			child := filepath.Join(root, "GitWave")
			if err := os.WriteFile(child, data, 0o700); err != nil {
				t.Fatal(err)
			}
			if test.name == "no library" {
				schemas := filepath.Join(root, Directory, "share", "glib-2.0", "schemas")
				if err := os.MkdirAll(schemas, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(schemas, "gschemas.compiled"), []byte("fixture"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			pid, err := unix.PidfdOpen(os.Getpid(), 0)
			if err != nil {
				t.Fatal(err)
			}
			parent := os.NewFile(uintptr(pid), "previous GitWave process")
			defer parent.Close()
			image, err := os.Open(executable)
			if err != nil {
				t.Fatal(err)
			}
			defer image.Close()
			ready, signal, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer ready.Close()
			defer signal.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, child)
			cmd.ExtraFiles = []*os.File{parent, image, signal}
			cmd.Env = append(os.Environ(), "GITWAVE_UPDATE_WAIT_FOR_PARENT=1")
			output, err := cmd.CombinedOutput()
			_ = signal.Close()
			if ctx.Err() != nil || err == nil || !strings.Contains(string(output), test.message) {
				t.Fatalf("replacement did not reject broken GTK: %v\n%s", err, output)
			}
			if err := ready.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if received, err := io.ReadAll(ready); err != nil || len(received) != 0 {
				t.Fatalf("replacement acknowledged readiness: %v, %v", received, err)
			}
		})
	}
}
