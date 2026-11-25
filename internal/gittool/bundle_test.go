package gittool

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFindGitPrefersTheInstalledBundle(t *testing.T) {
	directory := t.TempDir()
	root := filepath.Join(directory, Directory)
	git := gitPath(root, runtime.GOOS)
	shadow := filepath.Join(directory, "path")
	for _, name := range []string{git, filepath.Join(shadow, filepath.Base(git))} {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", shadow)
	for _, fallbackPath := range []string{shadow, ""} {
		t.Setenv("PATH", fallbackPath)
		if got, err := findGit(root, runtime.GOOS); err != nil || got != git {
			t.Fatalf("find Git with PATH %q: %q, %v", fallbackPath, got, err)
		}
	}
	env := Environment([]string{"PATH=" + shadow}, git, "")
	if want := "PATH=" + filepath.Dir(git) + string(os.PathListSeparator); !strings.HasPrefix(env[len(env)-1], want) {
		t.Fatalf("Git subprocess PATH = %s, want prefix %s", env[len(env)-1], want)
	}
	if err := os.Remove(git); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", shadow)
	if got, err := findGit(root, runtime.GOOS); err == nil || got != "" {
		t.Fatalf("incomplete bundle fell back to PATH: %q, %v", got, err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if got, err := findGit(root, runtime.GOOS); err != nil || got != filepath.Join(shadow, filepath.Base(git)) {
		t.Fatalf("developer PATH fallback: %q, %v", got, err)
	}
}
