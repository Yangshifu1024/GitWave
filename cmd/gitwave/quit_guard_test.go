package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestQuitChecksRequireConfirmationForUnverifiedRepositories(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	dirty := dirtyRepositories(ctx, []workspace.Repo{{Path: "/unverified", Nickname: "Unverified"}, {Path: "/missing", Missing: true}})
	if len(dirty) != 1 || dirty[0] != (dirtyRepository{name: "Unverified", incomplete: true}) {
		t.Fatalf("quit confirmation after deadline = %+v", dirty)
	}
}

func TestNativeQuitChecksInactiveRepositoryAndCancelKeepsWindow(t *testing.T) {
	clean, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Active clean repository")
	dirty, gitDirty := historyRepository(t)
	source, gitSource := historyRepository(t)
	gitSource("", "commit", "--allow-empty", "-m", "Submodule base")
	gitDirty("", "-c", "protocol.file.allow=always", "submodule", "add", "--", source, "child")
	gitDirty("", "commit", "-m", "Add child")
	child := filepath.Join(dirty, "child")
	other, _ := historyRepository(t)
	for _, path := range []string{child, other} {
		if err := os.WriteFile(filepath.Join(path, "unsaved.txt"), []byte("Keep my work\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	profile := filepath.Join(t.TempDir(), "state.db")
	store, err := workspace.Open(t.Context(), profile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.CreateWithRepos(t.Context(), "Another workspace", []string{other}); err != nil {
		t.Fatal(err)
	}
	ws, err := store.CreateWithRepos(t.Context(), "Active workspace", []string{clean, dirty})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SelectOrAddRepo(t.Context(), ws.ID, clean); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", profile)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(t.Context(), `UPDATE repos SET nickname = CASE path WHEN ? THEN 'Dirty in this workspace' ELSE 'Dirty in another workspace' END WHERE path IN (?, ?)`, dirty, dirty, other); err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.connectWorkspace(store, nil)
		return w.view()
	}, nsgotest.Size(1200, 800), nsgotest.Environment(core.Environment{
		OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en", ReduceMotion: true,
	}))
	const timeout = 5 * time.Second
	app.WaitFor("label:/^Active clean repository,/", timeout)
	assertPrompt := func() {
		t.Helper()
		app.WaitFor("id:quit-guard", timeout)
		app.Find("id:quit-guard >> text:Dirty in this workspace")
		app.Find("id:quit-guard >> text:1 uncommitted file(s)")
		app.WaitUntil(timeout, func() bool { return app.Find("id:quit-guard >> label:Cancel").Focused })
		if app.Exists("id:quit-guard >> text:Dirty in another workspace") || app.Quits != 0 {
			t.Fatal("quit guard left the active workspace or allowed quit before confirmation")
		}
	}
	onTarget := os.Getenv("NSGO_TEST_TARGET") != ""
	if !onTarget {
		app.ChooseMenu("GitWave", "Quit GitWave")
		assertPrompt()
		app.Press("id:quit-guard >> label:Cancel")
		app.WaitForGone("id:quit-guard", timeout)
		app.WaitFor("label:/^Active clean repository,/", timeout)
	}
	if onTarget {
		// The AppKit test host owns its window and app termination handlers.
		app.Key("mod+w")
	} else if app.CloseWindow() {
		t.Fatal("the dirty window closed before confirmation")
	}
	assertPrompt()
	if onTarget {
		app.Key("escape")
		app.WaitForGone("id:quit-guard", timeout)
		app.WaitFor("label:/^Active clean repository,/", timeout)
	} else {
		app.Press("id:quit-guard >> label:Quit anyway")
		app.WaitUntil(timeout, func() bool { return app.Quits == 1 })
	}
	if bytes, err := os.ReadFile(filepath.Join(child, "unsaved.txt")); err != nil || string(bytes) != "Keep my work\n" {
		t.Fatalf("quit changed uncommitted work: %q, %v", bytes, err)
	}
}
