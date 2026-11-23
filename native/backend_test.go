package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Changing only the refs expires a Rust snapshot without changing the
// first 300 rows. The page requested while scrolling must still arrive.
func TestHistoryContinuesAfterRefChanges(t *testing.T) {
	if os.Getenv("GITWAVE_NATIVE_CORE") == "" {
		t.Skip("run make native-test to supply the Rust executable")
	}
	const total = 305
	repo := t.TempDir()
	run := func(input string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Stdin = strings.NewReader(input)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	run("", "init", "-b", "main")
	var stream strings.Builder
	for i := range total {
		fmt.Fprintf(&stream, "commit refs/heads/main\ncommitter GitWave <test@example.invalid> %d +0000\ndata 7\nexample\n\n",
			1700000000+i)
	}
	run(stream.String(), "fast-import", "--quiet")
	ctx := t.Context()
	core, err := startBackend(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	defer core.close()
	req := request{Method: "history"}
	first, replace, err := core.fetchHistory(ctx, req)
	if err != nil || !replace || len(first.Commits) != 300 || !first.More || first.Cursor == "" {
		t.Fatalf("first page: %d commits, cursor %q, error %v", len(first.Commits), first.Cursor, err)
	}
	run("", "branch", "additional-ref", "HEAD")
	req.Cursor = first.Cursor
	fresh, replace, err := core.fetchHistory(ctx, req)
	if err != nil || !replace || len(fresh.Commits) != total || fresh.More {
		t.Fatalf("resumed page: %d commits, replace %v, more %v, error %v", len(fresh.Commits), replace, fresh.More, err)
	}
	for i, c := range first.Commits {
		if fresh.Commits[i].SHA != c.SHA {
			t.Fatalf("row %d changed SHA when only its refs changed", i)
		}
	}
}
