package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestNativeSubmoduleInitUpdateAndDeactivate(t *testing.T) {
	parent, git := historyRepository(t)
	source, sourceGit := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Superproject base")
	sourceGit("", "commit", "--allow-empty", "-m", "Submodule commit")
	git("", "-c", "protocol.file.allow=always", "submodule", "add", "--name", "library", "--", source, "deps/library")
	git("", "commit", "-m", "Register library")
	repo, cloneGit := historyRepository(t)
	cloneGit("", "fetch", "--", parent, "main")
	cloneGit("", "reset", "--hard", "FETCH_HEAD")

	app := refTestApp(t, repo)
	const timeout = 12 * time.Second
	app.WaitFor("id:submodule-row", timeout)
	app.WaitFor("id:submodule-init", timeout)
	app.Press("id:submodule-init")
	app.WaitFor("id:submodule-update", timeout)
	app.Press("id:submodule-update")
	app.WaitFor("text:Updated submodule \"library\"", timeout)
	checkout := filepath.Join(repo, "deps", "library")
	marker := filepath.Join(checkout, "local.txt")
	if err := os.WriteFile(marker, []byte("local content"), 0o600); err != nil {
		t.Fatal(err)
	}
	app.Press("id:submodule-deinit")
	app.WaitFor("id:submodule-dialog", timeout)
	app.Press("label:Cancel")
	app.WaitForGone("id:submodule-dialog", timeout)
	app.WaitFor("id:submodule-deinit", timeout)
	app.Press("id:submodule-deinit")
	app.WaitFor("id:submodule-dialog", timeout)
	app.Press("id:submodule-confirm")
	app.WaitFor("id:submodule-init", timeout)
	if got, err := os.ReadFile(marker); err != nil || string(got) != "local content" {
		t.Fatalf("deactivation lost untracked content: %q, %v", got, err)
	}
}

func TestNativeSubmoduleAddStagesSelectedPath(t *testing.T) {
	repo, git := historyRepository(t)
	source, sourceGit := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Superproject base")
	sourceGit("", "commit", "--allow-empty", "-m", "Library commit")
	head := git("", "rev-parse", "HEAD")

	app := refTestApp(t, repo)
	const timeout = 12 * time.Second
	const firstAdd = "id:submodules-toggle >> id:submodule-open-add"
	app.WaitFor(firstAdd, timeout)
	app.WaitUntil(timeout, func() bool { return app.Enabled(firstAdd) })
	app.Press(firstAdd)
	app.WaitFor("id:submodule-add-form", timeout)
	app.Type("id:submodule-url", source)
	app.Type("id:submodule-path", "deps/module")
	app.Press("id:submodule-add")
	app.WaitForGone("id:submodule-add-form", timeout)
	app.WaitFor("id:submodule-deinit", timeout)
	if staged := git("", "diff", "--cached", "--name-only"); staged != ".gitmodules\ndeps/module" {
		t.Fatalf("unexpected staged paths: %q", staged)
	}
	if got := git("", "rev-parse", "HEAD"); got != head {
		t.Fatalf("adding a submodule moved the branch: %q", got)
	}
}

func TestNativeSubmoduleReadErrorClearsAfterRefresh(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Superproject base")
	config := filepath.Join(repo, ".gitmodules")
	if err := os.WriteFile(config, []byte("[submodule \"unterminated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := refTestApp(t, repo)
	app.WaitFor("id:submodule-list-error", refTestTimeout)
	valid := "[submodule \"library\"]\n\tpath = deps/library\n\turl = ./library.git\n"
	if err := os.WriteFile(config, []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	app.Key("mod+r")
	app.WaitFor("id:submodule-init", refTestTimeout)
	app.WaitForGone("id:submodule-list-error", refTestTimeout)
}

func TestNativeSubmoduleBareRepositoryHasNoAdd(t *testing.T) {
	_, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Bare repository base")
	bare := t.TempDir()
	git("", "clone", "--bare", "--local", ".", bare)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.open(bare)
		return core.Show{When: w.submodulesReady.Get, Then: func(*core.Scope) core.Node { return w.view() }}
	}, nsgotest.Size(1200, 800))
	app.WaitFor("label:/^Bare repository base,/", refTestTimeout)
	app.WaitFor("id:submodules-toggle", refTestTimeout)
	if app.Exists("id:submodule-open-add") {
		t.Fatal("a bare repository has no worktree to add a submodule to")
	}
}
