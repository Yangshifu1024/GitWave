package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNativeCommitReplayFromHistoryAndMoreActions(t *testing.T) {
	path, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Main before replay")
	base := git("", "rev-parse", "HEAD")
	git("", "switch", "-c", "topic")
	file := filepath.Join(path, "from-topic.txt")
	if err := os.WriteFile(file, []byte("from topic\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("", "add", "--", "from-topic.txt")
	git("", "commit", "-m", "Topic to pick")
	topic := git("", "rev-parse", "HEAD")
	git("", "switch", "main")

	app := refTestApp(t, path)
	const timeout = 12 * time.Second
	topicRow := "label:/^Topic to pick, .*" + shortSHA(topic) + "$/"
	app.WaitFor(topicRow, timeout)
	app.RightClick(topicRow)
	app.ChoosePopup("Cherry-pick Commit…")
	app.WaitFor("text:Cherry-pick "+shortSHA(topic)+"?", timeout)
	app.Press("label:Cancel")
	app.WaitForGone("id:ref-dialog", timeout)
	if head := git("", "rev-parse", "HEAD"); head != base {
		t.Fatalf("cancel changed HEAD: %s", head)
	}

	app.Press(topicRow)
	app.WaitFor("id:commit-more-actions", timeout)
	app.Press("id:commit-more-actions")
	app.ChoosePopup("Cherry-pick")
	app.WaitFor("id:ref-dialog", timeout)
	app.Press("id:confirm-ref-action")
	app.WaitFor("text:Cherry-picked "+shortSHA(topic), timeout)
	picked := git("", "rev-parse", "HEAD")
	if picked == base || picked == topic {
		t.Fatalf("expected a new cherry-picked commit, got %s", picked)
	}
	if got, err := os.ReadFile(file); string(got) != "from topic\n" || err != nil {
		t.Fatalf("cherry-pick did not update the worktree: %q, %v", got, err)
	}

	pickedRow := "label:/^Topic to pick, .*" + shortSHA(picked) + "$/"
	app.WaitFor(pickedRow, timeout)
	app.RightClick(pickedRow)
	app.ChoosePopup("Revert Commit…")
	app.WaitFor("text:Revert "+shortSHA(picked)+"?", timeout)
	app.Press("id:confirm-ref-action")
	app.WaitFor("text:Reverted "+shortSHA(picked), timeout)
	if got := git("", "rev-parse", "HEAD^"); got != picked {
		t.Fatalf("revert did not create a commit on the current branch: %s", got)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("revert left the picked file: %v", err)
	}
}

func TestNativeCommitReplayShowsConflictAndDoesNotChangeCheckout(t *testing.T) {
	path, git := historyRepository(t)
	file := filepath.Join(path, "collision.txt")
	write := func(value string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(value+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("original")
	git("", "add", "collision.txt")
	git("", "commit", "-m", "Base")
	git("", "switch", "-c", "topic")
	write("upstream")
	git("", "commit", "-am", "Conflicting pick")
	git("", "switch", "main")
	write("local")
	git("", "commit", "-am", "Local edit")
	base := git("", "rev-parse", "HEAD")

	app := refTestApp(t, path)
	const timeout = 12 * time.Second
	app.WaitFor("label:/^Conflicting pick,/", timeout)
	app.RightClick("label:/^Conflicting pick,/")
	app.ChoosePopup("Cherry-pick Commit…")
	app.WaitFor("id:ref-dialog", timeout)
	app.Press("id:confirm-ref-action")
	app.WaitFor("text:Cherry-pick hit conflicts in collision.txt", timeout)
	if got := git("", "rev-parse", "HEAD"); got != base {
		t.Fatalf("conflicted pick moved HEAD: %s", got)
	}
	if got, err := os.ReadFile(file); string(got) != "local\n" || err != nil {
		t.Fatalf("conflicted pick changed the working file: %q, %v", got, err)
	}
	if status := git("", "status", "--porcelain"); status != "" {
		t.Fatalf("conflicted pick left working state: %s", status)
	}
}
