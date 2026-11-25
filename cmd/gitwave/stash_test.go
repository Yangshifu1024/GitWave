package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

func TestNativeStashSaveAndPreview(t *testing.T) {
	path, git := historyRepository(t)
	tracked := filepath.Join(path, "saved.txt")
	untracked := filepath.Join(path, "new file.txt")
	write := func(path, value string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(tracked, "base")
	git("", "add", "saved.txt")
	git("", "commit", "-m", "Stash base")
	write(tracked, "tracked stash content")
	write(untracked, "untracked stash content")
	for i := range 32 {
		write(filepath.Join(path, fmt.Sprintf("middle-%02d.txt", i)), "additional stash file")
	}
	app := refTestApp(t, path)
	const timeout = 12 * time.Second
	heading := app.WaitFor("id:stash-toggle", timeout)
	if heading.Props.Role != core.RoleGroup || app.Exists("text:No stashes") {
		t.Fatal("an empty stash section should show only its heading")
	}
	app.WaitUntil(timeout, func() bool { return app.Enabled("id:stash-save") })
	app.Press("id:stash-save")
	app.WaitFor("id:stash-save-dialog", timeout)
	app.Press("id:cancel-stash-save")
	app.WaitForGone("id:stash-save-dialog", timeout)
	if out := git("", "stash", "list"); out != "" {
		t.Fatalf("cancel created a stash: %s", out)
	}
	app.Press("id:stash-save")
	app.WaitFor("id:stash-message", timeout)
	app.Type("id:stash-message", "native preview")
	app.Press("id:confirm-stash-save")
	app.WaitForGone("id:stash-save-dialog", timeout)
	app.WaitFor("text:Stash saved", timeout)
	app.WaitFor("id:stash-row", timeout)
	if got := git("", "stash", "list"); !strings.Contains(got, "native preview") {
		t.Fatalf("stash list: %q", got)
	}
	if got, err := os.ReadFile(tracked); err != nil || string(got) != "base\n" {
		t.Fatalf("stash did not restore tracked file: %q, %v", got, err)
	}
	if _, err := os.Stat(untracked); !os.IsNotExist(err) {
		t.Fatalf("stash did not clear the new file: %v", err)
	}
	app.Press("id:stash-row")
	app.WaitFor("id:stash-preview-dialog", timeout)
	app.WaitFor("id:stash-files", timeout)
	app.Scroll("id:stash-files", 1200)
	app.WaitFor("label:new file.txt · Untracked", timeout)
	app.Press("label:new file.txt · Untracked")
	app.WaitFor("text:/^Untracked$/", timeout)
	app.WaitFor("text:/^untracked stash content$/", timeout)
	if offset := app.Find("id:stash-files").Offset.Y; offset <= 0 {
		t.Fatalf("switching a stash file reset the list scroll position: %v", offset)
	}
	app.Press("label:Close")
	app.WaitForGone("id:stash-preview-dialog", timeout)
}

func TestNativeStashApplyConfirmsDirtyAndKeepsEntry(t *testing.T) {
	path, git := historyRepository(t)
	stashed := filepath.Join(path, "stashed.txt")
	independent := filepath.Join(path, "independent.txt")
	write := func(path, value string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write(stashed, "base")
	git("", "add", "stashed.txt")
	git("", "commit", "-m", "Before stash")
	write(stashed, "stashed edit")
	git("", "stash", "push", "-m", "apply test")
	write(independent, "independent edit")
	app := refTestApp(t, path)
	const timeout = 12 * time.Second
	app.WaitFor("id:stash-row", timeout)
	app.Press("id:stash-row")
	app.WaitFor("id:stash-preview-dialog", timeout)
	app.Press("id:stash-preview-apply")
	app.WaitFor("id:stash-apply-dialog", timeout)
	app.Press("id:stash-apply-dialog >> label:Cancel")
	app.WaitForGone("id:stash-apply-dialog", timeout)
	if data, err := os.ReadFile(stashed); err != nil || string(data) != "base\n" {
		t.Fatalf("cancel applied a stash: %q, %v", data, err)
	}
	app.Press("id:stash-preview-apply")
	app.WaitFor("id:stash-apply-dialog", timeout)
	app.Press("id:stash-apply-dialog >> label:Continue")
	app.WaitForGone("id:stash-apply-dialog", timeout)
	app.WaitUntil(timeout, func() bool {
		data, _ := os.ReadFile(stashed)
		return string(data) == "stashed edit\n"
	})
	app.WaitFor("id:stash-preview-dialog", timeout)
	if got, err := os.ReadFile(independent); err != nil || string(got) != "independent edit\n" {
		t.Fatalf("apply changed independent edit: %q, %v", got, err)
	}
	if got := git("", "stash", "list"); !strings.Contains(got, "apply test") {
		t.Fatalf("apply removed the stash: %q", got)
	}
}

func TestNativeStashApplyConflictKeepsEntry(t *testing.T) {
	for _, id := range []string{"stash-preview-apply", "stash-preview-pop"} {
		t.Run(id, func(t *testing.T) { testNativeStashConflictKeepsEntry(t, id) })
	}
}

func testNativeStashConflictKeepsEntry(t *testing.T, id string) {
	t.Helper()
	path, git := historyRepository(t)
	file := filepath.Join(path, "conflicting.txt")
	write := func(value string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("base")
	git("", "add", "conflicting.txt")
	git("", "commit", "-m", "Base")
	write("stash version")
	git("", "stash", "push", "-m", "conflicting stash")
	write("current branch version")
	git("", "commit", "-am", "Advance branch")
	app := refTestApp(t, path)
	const timeout = 12 * time.Second
	app.WaitFor("id:stash-row", timeout)
	app.Press("id:stash-row")
	app.WaitFor("id:stash-preview-dialog", timeout)
	app.Press("id:" + id)
	if message := app.WaitFor("id:stash-action-error", timeout).Text; !strings.Contains(message, "Stash kept") ||
		!strings.Contains(message, "conflicting.txt") {
		t.Fatalf("unexpected conflict feedback: %q", message)
	}
	if got := git("", "stash", "list"); !strings.Contains(got, "conflicting stash") {
		t.Fatalf("lost stash after conflict: %q", got)
	}
	if got := git("", "ls-files", "--unmerged"); !strings.Contains(got, "conflicting.txt") {
		t.Fatalf("conflict missing from index: %q", got)
	}
	app.Press("label:Close")
	app.WaitForGone("id:stash-preview-dialog", timeout)
	app.WaitFor("id:open-conflicts", timeout)
}

func TestNativeStashDropCancelsAndDeletesOnlySelected(t *testing.T) {
	path, git := historyRepository(t)
	file := filepath.Join(path, "drop-file.txt")
	write := func(value string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("base")
	git("", "add", "drop-file.txt")
	git("", "commit", "-m", "Before stashes")
	write("older content")
	git("", "stash", "push", "-m", "older entry")
	write("latest content")
	git("", "stash", "push", "-m", "latest entry")
	app := refTestApp(t, path)
	const timeout = 12 * time.Second
	const older = `label:/^stash@\{1\}.*older entry$/`
	app.WaitFor(older, timeout)
	app.Press(older)
	app.WaitFor("id:stash-preview-dialog", timeout)
	app.Press("id:stash-preview-drop")
	app.WaitFor("id:stash-drop-dialog", timeout)
	if summary := app.WaitFor("id:stash-drop-files", timeout).Text; !strings.Contains(summary, "drop-file.txt") {
		t.Fatalf("the delete confirmation lost the file summary: %q", summary)
	}
	app.Press("id:stash-drop-dialog >> label:Cancel")
	app.WaitForGone("id:stash-drop-dialog", timeout)
	if got := git("", "stash", "list"); !strings.Contains(got, "older entry") || !strings.Contains(got, "latest entry") {
		t.Fatalf("cancel changed the stash list: %q", got)
	}
	app.Press("id:stash-preview-drop")
	app.WaitFor("id:stash-drop-dialog", timeout)
	app.Press("id:confirm-stash-drop")
	app.WaitForGone("id:stash-preview-dialog", timeout)
	app.WaitFor("text:Stash deleted", timeout)
	if got := git("", "stash", "list"); strings.Contains(got, "older entry") || !strings.Contains(got, "latest entry") {
		t.Fatalf("the wrong stash was deleted: %q", got)
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "base\n" {
		t.Fatalf("Delete touched the working copy: %q, %v", data, err)
	}
}

func TestNativeStashPopAppliesAndRemovesSelected(t *testing.T) {
	path, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Before pop")
	file := filepath.Join(path, "popped.txt")
	if err := os.WriteFile(file, []byte("popped content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("", "stash", "push", "-u", "-m", "pop this")
	app := refTestApp(t, path)
	const timeout = 12 * time.Second
	app.WaitFor("id:stash-row", timeout)
	app.Press("id:stash-row")
	app.WaitFor("id:stash-preview-dialog", timeout)
	app.Press("id:stash-preview-pop")
	app.WaitForGone("id:stash-preview-dialog", timeout)
	app.WaitFor("text:Stash applied and deleted", timeout)
	if data, err := os.ReadFile(file); err != nil || string(data) != "popped content\n" {
		t.Fatalf("pop did not restore the file: %q, %v", data, err)
	}
	if got := git("", "stash", "list"); got != "" {
		t.Fatalf("pop left the selected stash: %q", got)
	}
}
