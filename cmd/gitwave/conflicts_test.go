package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeConflictEditorRetainsDraftAcrossFilesAndExternalResolution(t *testing.T) {
	path, git := historyRepository(t)
	write := func(name, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(path, name), []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeBoth := func(value string) {
		write("a.txt", value+"\none\ntwo\nthree\nfour\nfive\nsix\n"+value)
		write("b.txt", value)
		git("", "add", "--all")
		git("", "commit", "-m", value)
	}
	writeBoth("Base version")
	git("", "switch", "-c", "topic")
	writeBoth("Topic version")
	git("", "switch", "main")
	writeBoth("Main version")
	app := refTestApp(t, path)
	const timeout = 10 * time.Second
	app.WaitFor("label:topic", timeout)
	app.RightClick("label:topic")
	app.ChoosePopup("Merge into current")
	app.WaitFor("text:/May conflict in 2 file/", timeout)
	app.Press("id:confirm-ref-action")
	app.WaitFor("id:open-conflicts", timeout)
	app.Press("id:open-conflicts")
	app.Press("label:a.txt")
	app.WaitFor("id:conflict-editor", timeout)
	app.Press("id:conflict-next")
	if app.Find("id:conflict-previous").Props.Disabled {
		t.Fatal("next conflict did not reach the second region")
	}
	app.Press("id:conflict-ours")
	app.Type("label:Resolved file content", " draft-a")
	waitForDraft := func() {
		t.Helper()
		// The editor draws document rows and gives keyboard focus to a
		// separate input. The displayed rows aren't accessibility labels.
		app.WaitUntil(timeout, func() bool { return strings.Contains(app.Dump(), "Main version draft-a") })
	}
	waitForDraft()
	app.Press("label:b.txt")
	app.WaitFor("id:conflict-editor", timeout)
	app.Press("id:conflict-theirs")
	write("a.txt", "resolved on disk")
	git("", "add", "a.txt")
	app.FastForward(3 * time.Second)
	app.WaitUntil(timeout, func() bool { return strings.Contains(app.Dump(), `"Merge conflicts (1)"`) })
	app.Press("label:a.txt")
	app.WaitFor("label:Copy unsaved edits", timeout)
	waitForDraft()
	if !app.Find("id:save-conflict").Props.Disabled {
		t.Fatal("app could save an externally resolved file")
	}
	write("b.txt", "second resolved on disk")
	git("", "add", "b.txt")
	app.FastForward(3 * time.Second)
	app.WaitUntil(timeout, func() bool { return strings.Contains(app.Dump(), `"Merge conflicts (0)"`) })
	app.Press("label:b.txt")
	app.WaitFor("label:Copy unsaved edits", timeout)
	app.Press("label:a.txt")
	waitForDraft()
	app.Press("id:close-conflicts")
	app.WaitFor("id:discard-conflict-edits", timeout)
	app.Press("label:Discard edits")
	app.WaitForGone("id:conflicts-dialog", timeout)
	if got := git("", "show", ":a.txt"); got != "resolved on disk" {
		t.Fatalf("editor replaced the external resolution: %q", got)
	}
}
