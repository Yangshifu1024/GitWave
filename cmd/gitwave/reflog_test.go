package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestRecoveryRemainsReachableInAnOverflowingSidebar(t *testing.T) {
	dir, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Recovery scroll fixture")
	for _, name := range []string{"alpha", "beta", "charlie", "delta", "echo", "foxtrot", "golf", "hotel", "india", "juliet", "kilo", "lima", "mike"} {
		git("", "branch", name)
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.open(dir)
		return w.view()
	}, nsgotest.Size(1024, 608), nsgotest.Environment(refreshEnvironment(core.LifecycleActive)))
	const timeout = 8 * time.Second
	const sidebar = "id:repository-sidebar"
	app.WaitFor("label:/^Recovery scroll fixture,/", timeout)
	app.Press("id:health-toggle")
	app.WaitFor("id:health-ai-summary", timeout)
	scrollToEnd := func() {
		app.WaitUntil(timeout, func() bool {
			view := app.Find(sidebar)
			return view.Content.Height > view.Frame.Size.Height
		})
		// The wheel can scroll the branch list while it passes under the
		// pointer. Continue until the outer viewport reaches its end.
		for range 3 {
			view := app.Find(sidebar)
			remaining := view.Content.Height - view.Frame.Size.Height - view.Offset.Y
			if remaining <= 1 {
				return
			}
			app.Scroll(sidebar, remaining)
		}
		app.WaitUntil(timeout, func() bool {
			view := app.Find(sidebar)
			return view.Offset.Y >= view.Content.Height-view.Frame.Size.Height-1
		})
	}
	scrollToEnd()
	app.Press("id:reflog-toggle")
	const entry = "label:/^Initial commit.*Recovery scroll fixture$/"
	app.WaitFor(entry, timeout)
	scrollToEnd()
	app.Press(entry)
	app.WaitFor("id:reflog-create-branch", timeout)
	scrollToEnd()
	app.Press("id:reflog-create-branch")
	app.WaitFor("id:ref-name", timeout)
}

func TestNativeReflogRecoversPreviousCommitAndResetsWithConfirmation(t *testing.T) {
	dir, git := historyRepository(t)
	file := filepath.Join(dir, "reset.txt")
	write := func(value string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("head content")
	git("", "add", "reset.txt")
	git("", "commit", "-m", "Prior message")
	previous := git("", "rev-parse", "HEAD")
	git("", "commit", "--amend", "-m", "Amended message")
	amended := git("", "rev-parse", "HEAD")
	write("local tracked change")
	app := refTestApp(t, dir)
	const timeout = 12 * time.Second
	app.WaitFor("id:reflog-toggle", timeout)
	app.Press("id:reflog-toggle")
	const item = "label:/^Amend.*Amended message$/"
	app.WaitFor(item, timeout)
	app.Press(item)
	app.Press("id:reflog-create-branch")
	app.WaitFor("id:ref-name", timeout)
	recovery := "recovery-" + shortSHA(previous)
	if name := app.Find("id:ref-name").Text; name != recovery {
		t.Fatalf("default branch name = %q, want %q", name, recovery)
	}
	app.Press("id:confirm-ref-action")
	app.WaitForGone("id:ref-dialog", timeout)
	app.WaitFor("text:Created recovery branch "+recovery, timeout)
	if got := git("", "rev-parse", "refs/heads/"+recovery); got != previous || git("", "rev-parse", "HEAD") != amended {
		t.Fatalf("recovery branch = %q, want previous %q with original HEAD %q", got, previous, amended)
	}

	app.Press("id:reflog-branch")
	app.WaitFor(item, timeout)
	app.Press(item)
	app.Press("id:reflog-reset")
	app.WaitFor("id:ref-dialog", timeout)
	app.Press("id:ref-dialog >> label:Cancel")
	app.WaitForGone("id:ref-dialog", timeout)
	if data, err := os.ReadFile(file); err != nil || string(data) != "local tracked change\n" {
		t.Fatalf("cancelling changed tracked data: %q, %v", data, err)
	}

	app.Press("id:reflog-reset")
	app.WaitFor("id:ref-dialog", timeout)
	app.WaitUntil(timeout, func() bool { return app.Enabled("id:confirm-ref-action") })
	app.Press("id:confirm-ref-action")
	app.WaitForGone("id:ref-dialog", timeout)
	app.WaitFor("text:Branch reset to "+shortSHA(previous), timeout)
	if head := git("", "rev-parse", "HEAD"); head != previous || git("", "rev-parse", "ORIG_HEAD") != amended {
		t.Fatalf("reset HEAD = %q, want %q and ORIG_HEAD %q", head, previous, amended)
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "head content\n" {
		t.Fatalf("reset did not restore the target file: %q, %v", data, err)
	}
}

func TestNativeHistoryResetUsesSelectedCommit(t *testing.T) {
	dir, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Reference point")
	base := git("", "rev-parse", "HEAD")
	git("", "commit", "--allow-empty", "-m", "Head before reset")
	app := refTestApp(t, dir)
	const timeout = 12 * time.Second
	row := "label:/^Reference point, .*" + shortSHA(base) + "$/"
	app.WaitFor(row, timeout)
	app.RightClick(row)
	app.ChoosePopup("Reset branch here…")
	app.WaitFor("id:ref-dialog", timeout)
	app.WaitUntil(timeout, func() bool { return app.Enabled("id:confirm-ref-action") })
	app.Press("id:confirm-ref-action")
	app.WaitForGone("id:ref-dialog", timeout)
	if got := git("", "rev-parse", "HEAD"); got != base {
		t.Fatalf("reset selected %s instead of %s", got, base)
	}
}
