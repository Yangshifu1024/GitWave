package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeAbortMergeKeepsIndependentChanges(t *testing.T) {
	path, git := historyRepository(t)
	write := func(name, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(path, name), []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("conflict.txt", "base")
	git("", "add", "conflict.txt")
	git("", "commit", "-m", "Common ancestor")
	git("", "switch", "-c", "topic")
	write("conflict.txt", "topic")
	git("", "commit", "-am", "Conflict from topic")
	tip := git("", "rev-parse", "HEAD")
	git("", "switch", "main")
	write("conflict.txt", "main")
	git("", "commit", "-am", "Collision on main")
	base := git("", "rev-parse", "HEAD")
	app := refTestApp(t, path)
	const timeout = 10 * time.Second
	app.WaitFor("label:topic", timeout)
	app.RightClick("label:topic")
	app.ChoosePopup("Merge into current")
	app.WaitFor("text:/May conflict in 1 file/", timeout)
	app.Press("id:confirm-ref-action")
	app.WaitFor("id:abort-merge", timeout)
	app.Press("id:abort-merge")
	app.WaitFor("id:merge-abort-dialog", timeout)
	app.Press("id:cancel-merge-abort")
	app.WaitForGone("id:merge-abort-dialog", timeout)
	if head := git("", "rev-parse", "MERGE_HEAD"); head != tip {
		t.Fatal("cancel changed MERGE_HEAD")
	}

	write("local.txt", "independent staged")
	git("", "add", "local.txt")
	write("local.txt", "independent unstaged")
	app.Press("id:open-conflicts")
	app.WaitFor("label:conflict.txt", timeout)
	app.Press("label:conflict.txt")
	app.WaitFor("id:conflict-editor", timeout)
	app.Press("id:conflict-ours")
	app.Type("label:Resolved file content", " draft")
	app.Press("id:abort-merge-conflicts")
	app.WaitFor("id:merge-abort-dialog", timeout)
	app.Press("id:cancel-merge-abort")
	app.WaitForGone("id:merge-abort-dialog", timeout)
	app.WaitUntil(timeout, func() bool { return strings.Contains(app.Dump(), "main draft") })

	app.Press("id:abort-merge-conflicts")
	app.WaitFor("id:merge-abort-dialog", timeout)
	app.WaitUntil(timeout, func() bool { return !app.Find("id:confirm-merge-abort").Props.Disabled })
	app.Press("id:confirm-merge-abort")
	app.WaitForGone("id:merge-abort-dialog", timeout)
	app.WaitForGone("id:conflicts-dialog", timeout)
	app.WaitForGone("id:merge-banner", timeout)
	if head := git("", "rev-parse", "HEAD"); head != base {
		t.Fatal("abort changed HEAD")
	}
	if staged := git("", "show", ":local.txt"); staged != "independent staged" {
		t.Fatalf("independent staged change was lost: %q", staged)
	}
	for name, expected := range map[string]string{"local.txt": "independent unstaged\n", "conflict.txt": "main\n"} {
		if got, err := os.ReadFile(filepath.Join(path, name)); string(got) != expected || err != nil {
			t.Errorf("%s = %q, want %q: %v", name, got, expected, err)
		}
	}
}

func TestNativeMergePreviewAndNoFastForwardFromHistory(t *testing.T) {
	path, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Target before merge")
	base := git("", "rev-parse", "HEAD")
	git("", "switch", "-c", "topic")
	if err := os.WriteFile(filepath.Join(path, "topic.txt"), []byte("new file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("", "add", "topic.txt")
	git("", "commit", "-m", "Tip of topic")
	tip := git("", "rev-parse", "HEAD")
	git("", "switch", "main")
	app := refTestApp(t, path)
	const timeout = 8 * time.Second
	badge := "label:/^Tip of topic,/ >> id:history-refs"
	app.WaitFor(badge, timeout)
	app.RightClick(badge)
	app.ChoosePopup("Merge into current")
	app.WaitFor("text:Merge can be done without conflicts", timeout)
	app.Press("label:Cancel")
	app.WaitForGone("id:ref-dialog", timeout)
	if got := git("", "rev-parse", "HEAD"); got != base {
		t.Fatal("cancel moved HEAD")
	}
	app.RightClick("label:topic")
	app.ChoosePopup("Merge into current")
	app.WaitFor("text:Merge can be done without conflicts", timeout)
	app.Press("id:merge-mode")
	app.Press("text:No fast-forward — always create a merge commit")
	app.Press("id:confirm-ref-action")
	app.WaitForGone("id:ref-dialog", timeout)
	app.WaitFor("label:/^Merge branch .*topic/", timeout)
	if got := git("", "show", "-s", "--format=%P", "HEAD"); got != base+" "+tip {
		t.Fatalf("no-fast-forward option produced the wrong parents: %q", got)
	}
}

func TestNativeMergeConflictPausesAndFinishesFromChanges(t *testing.T) {
	path, git := historyRepository(t)
	file := filepath.Join(path, "conflict.txt")
	write := func(value string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		git("", "add", "conflict.txt")
	}
	write("base")
	git("", "commit", "-m", "Common ancestor")
	git("", "switch", "-c", "topic")
	write("topic")
	git("", "commit", "-m", "Conflict from topic")
	tip := git("", "rev-parse", "HEAD")
	git("", "switch", "main")
	write("main")
	git("", "commit", "-m", "Collision on main")
	base := git("", "rev-parse", "HEAD")
	app := refTestApp(t, path)
	const timeout = 8 * time.Second
	app.WaitFor("label:topic", timeout)
	app.RightClick("label:topic")
	app.ChoosePopup("Merge into current")
	app.WaitFor("text:/May conflict in 1 file/", timeout)
	app.Press("id:confirm-ref-action")
	app.WaitFor("text:/Merge in progress .* 1 conflicted file/", timeout)
	if head := git("", "rev-parse", "HEAD"); head != base {
		t.Fatal("conflicted merge moved HEAD")
	}
	app.Press("id:open-conflicts")
	app.WaitFor("id:conflicts-dialog", timeout)
	app.Press("label:conflict.txt")
	app.WaitFor("id:conflict-editor", timeout)
	app.WaitFor("id:conflict-count", timeout)
	app.Press("id:save-conflict")
	app.WaitFor("id:confirm-conflict-markers", timeout)
	app.Press("label:Keep editing")
	app.WaitForGone("id:confirm-conflict-markers", timeout)
	app.Press("id:conflict-theirs")
	app.Press("id:close-conflicts")
	app.WaitFor("id:discard-conflict-edits", timeout)
	app.Press("label:Keep editing")
	app.WaitForGone("id:discard-conflict-edits", timeout)
	app.Press("id:save-conflict")
	app.WaitForGone("id:conflicts-dialog", timeout)
	app.WaitFor("text:/Merge in progress .* all conflicts resolved/", timeout)
	if got, err := os.ReadFile(file); err != nil || string(got) != "topic\n" {
		t.Fatalf("choosing theirs did not install the selected text: %q, %v", got, err)
	}
	app.Press("id:custom-titlebar >> label:Changes")
	app.WaitFor("id:commit-message", timeout)
	app.Type("id:commit-message", "Complete the merge")
	app.Press("id:write-commit")
	app.WaitForGone("id:merge-banner", timeout)
	if got := git("", "show", "-s", "--format=%P", "HEAD"); got != base+" "+tip {
		t.Fatalf("resolved merge produced the wrong parents: %q", got)
	}
}
