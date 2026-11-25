package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/core"
)

func TestNativeWorktreeCreateSwitchAndRemove(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Base for linked tree")
	git("", "branch", "start-here")
	git("", "switch", "start-here")
	git("", "commit", "--allow-empty", "-m", "Alternate beginning")
	start := git("", "rev-parse", "HEAD")
	git("", "switch", "main")

	root := t.TempDir()
	destination := filepath.Join(root, "Sibling checkout")
	store, err := workspace.Open(t.Context(), filepath.Join(root, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.CreateWithRepos(t.Context(), "Worktree project", []string{repo}); err != nil {
		t.Fatal(err)
	}
	app := workspaceApp(t, store)
	const timeout = 8 * time.Second
	app.WaitFor("label:/^Base for linked tree,/", timeout)
	app.WaitUntil(timeout, func() bool { return app.Find("id:worktree-toggle").Props.Role == core.RoleButton })
	if app.Exists("id:worktree-row") {
		t.Fatal("lone main worktree should start collapsed")
	}
	openCreate := func() {
		app.ChooseMenu("Repository", "New worktree")
		app.WaitFor("id:worktree-create-dialog", timeout)
	}
	openCreate()
	app.Type("id:worktree-create-name", "feature/sibling")
	app.Type("id:worktree-create-path", destination)
	app.Press("label:Cancel")
	app.WaitForGone("id:worktree-create-dialog", timeout)
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("cancel created a worktree: %v", err)
	}

	openCreate()
	app.Type("id:worktree-create-name", "feature/sibling")
	app.Type("id:worktree-create-path", destination)
	app.Press("id:worktree-create-start")
	app.WaitFor("role:option start-here", timeout)
	app.Press("role:option start-here")
	app.Press("id:worktree-create-confirm")
	app.WaitForGone("id:worktree-create-dialog", timeout)
	waitSwitch := func() {
		t.Helper()
		app.WaitUntil(timeout, func() bool {
			return app.Exists("id:worktree-switch") && app.Enabled("id:worktree-switch")
		})
	}
	waitSwitch()
	if got := git("", "rev-parse", "refs/heads/feature/sibling"); got != start {
		t.Fatalf("worktree started at %s, want %s", got, start)
	}

	app.Press("id:worktree-switch")
	app.WaitFor("label:/^Alternate beginning,/", timeout)
	waitRepo := func(name string, count int) {
		t.Helper()
		app.WaitUntil(timeout, func() bool {
			state, err := store.Load(t.Context())
			return err == nil && len(state.Repos) == count && filepath.Base(state.Repo.Path) == name
		})
	}
	waitRepo(filepath.Base(destination), 2)
	waitSwitch()
	app.Press("id:worktree-switch")
	app.WaitFor("label:/^Base for linked tree,/", timeout)
	waitRepo(filepath.Base(repo), 2)

	local := filepath.Join(destination, "local-only")
	if err := os.WriteFile(local, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	app.WaitFor("id:worktree-remove", timeout)
	app.Press("id:worktree-remove")
	app.WaitFor("text:/linked worktree contains local changes/", timeout)
	if app.Enabled("id:worktree-remove-confirm") {
		t.Fatal("remove should be disabled while the worktree is dirty")
	}
	app.Press("label:Cancel")
	app.WaitForGone("id:worktree-remove-dialog", timeout)
	if err := os.Remove(local); err != nil {
		t.Fatal(err)
	}

	app.Press("id:worktree-remove")
	app.WaitUntil(timeout, func() bool { return app.Enabled("id:worktree-remove-confirm") })
	app.Press("label:Cancel")
	app.WaitForGone("id:worktree-remove-dialog", timeout)
	if _, err := os.Stat(destination); err != nil {
		t.Fatalf("cancel removed the worktree: %v", err)
	}

	app.Press("id:worktree-remove")
	app.WaitUntil(timeout, func() bool { return app.Enabled("id:worktree-remove-confirm") })
	app.Press("id:worktree-remove-confirm")
	app.WaitForGone("id:worktree-remove-dialog", timeout)
	app.WaitForGone("id:worktree-row", timeout)
	app.WaitUntil(timeout, func() bool {
		state, err := store.Load(t.Context())
		for _, item := range state.Repos {
			if filepath.Base(item.Path) == filepath.Base(destination) {
				return err == nil && item.Missing
			}
		}
		return false
	})
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("confirmed removal left the worktree: %v", err)
	}
	if got := git("", "rev-parse", "refs/heads/feature/sibling"); got != start {
		t.Fatalf("removing the worktree changed the branch: %s", got)
	}
}
