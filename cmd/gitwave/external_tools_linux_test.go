package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestNativeExternalToolsOpenTheActiveRepository(t *testing.T) {
	repo, _ := historyRepository(t)
	bin, launches := t.TempDir(), t.TempDir()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(git, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	const fixture = "#!/bin/sh\nprintf '%s\\n' \"$PWD\" \"$@\" > \"$GITWAVE_TEST_TOOL_DIR/${0##*/}\"\n"
	for _, program := range []string{"example-terminal", "code"} {
		if err := os.WriteFile(filepath.Join(bin, program), []byte(fixture), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	t.Setenv("TERMINAL", "example-terminal")
	t.Setenv("GITWAVE_TEST_TOOL_DIR", launches)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.open(repo)
		return w.view()
	}, nsgotest.Size(1200, 800), nsgotest.Environment(core.Environment{
		OS: core.OSLinux, Idiom: core.IdiomDesktop, Locale: "en", ReduceMotion: true,
	}))
	const timeout = 5 * time.Second
	app.WaitFor("id:repository-tabs", timeout)
	assertLaunch := func(program, want string) {
		t.Helper()
		app.WaitUntil(timeout, func() bool {
			data, _ := os.ReadFile(filepath.Join(launches, program))
			return string(data) == want
		})
	}
	app.Press("id:open-terminal")
	assertLaunch("example-terminal", repo+"\n")
	app.Press("id:open-editor")
	app.WaitFor("id:external-editor-vscode", timeout)
	app.Press("id:external-editor-vscode")
	assertLaunch("code", repo+"\n"+repo+"\n")
}
