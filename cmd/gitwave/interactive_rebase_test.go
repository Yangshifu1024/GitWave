package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

func TestNativeInteractiveRebaseReordersEditsAndContinues(t *testing.T) {
	path, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Initial base")
	git("", "branch", "upstream")
	for _, name := range []string{"first pick", "second pick", "third pick"} {
		if err := os.WriteFile(filepath.Join(path, name+".txt"), []byte(name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		git("", "add", "--all")
		git("", "commit", "-m", name)
	}
	original := git("", "rev-parse", "HEAD")
	app := refTestApp(t, path)
	const timeout = 12 * time.Second
	open := func() {
		app.WaitFor("label:upstream", timeout)
		app.RightClick("label:upstream")
		app.ChoosePopup("Interactive rebase")
		app.WaitFor("id:interactive-rebase-todos", timeout)
	}
	open()
	app.Press("id:interactive-rebase-cancel")
	app.WaitForGone("id:interactive-rebase-dialog", timeout)
	if head := git("", "rev-parse", "HEAD"); head != original {
		t.Fatalf("cancel changed HEAD: %s", head)
	}
	open()
	app.Drag("label:third pick >> id:interactive-rebase-grip", core.Point{Y: -105})
	app.WaitUntil(timeout, func() bool {
		rows := app.Query("id:interactive-rebase-todo")
		return len(rows) == 3 && rows[0].Props.Label == "third pick"
	})
	app.Focus("label:third pick >> id:interactive-rebase-grip")
	app.Key("down")
	app.Key("down")
	app.WaitUntil(timeout, func() bool {
		rows := app.Query("id:interactive-rebase-todo")
		return len(rows) == 3 && rows[2].Props.Label == "third pick"
	})
	app.Key("up")
	app.Key("up")
	app.WaitUntil(timeout, func() bool {
		rows := app.Query("id:interactive-rebase-todo")
		return len(rows) == 3 && rows[0].Props.Label == "third pick"
	})
	app.Press("label:third pick >> id:interactive-rebase-action")
	app.Press("id:interactive-rebase-action-2")
	app.Press("label:first pick >> id:interactive-rebase-action")
	app.Press("id:interactive-rebase-action-1")
	app.Type("id:interactive-rebase-message", "renamed first")
	app.Press("id:interactive-rebase-start")
	app.WaitForGone("id:interactive-rebase-dialog", timeout)
	app.WaitFor("id:interactive-rebase-continue", timeout)
	if got := git("", "show", "-s", "--format=%s", "HEAD"); got != "third pick" {
		t.Fatalf("expected edit pause on reordered commit: %q", got)
	}
	app.Press("id:interactive-rebase-continue")
	app.WaitForGone("id:interactive-rebase-continue", timeout)
	if got := git("", "log", "-3", "--format=%s", "HEAD"); got != strings.Join([]string{"second pick", "renamed first", "third pick"}, "\n") {
		t.Fatalf("unexpected history: %q", got)
	}
}
