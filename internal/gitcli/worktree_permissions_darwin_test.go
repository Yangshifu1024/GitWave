package gitcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorktreeCreatePreservesExistingDirectoryACL(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	dest := filepath.Join(t.TempDir(), "private directory")
	if err := os.Mkdir(dest, 0o750); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.CommandContext(t.Context(), "/bin/chmod", "+a", "nobody deny list,search", dest).CombinedOutput(); err != nil {
		t.Skipf("the fixture directory does not support ACL updates: %v (%s)", err, output)
	}
	acl := func() string {
		t.Helper()
		// Print ACL UUIDs so a transient Directory Services lookup cannot change the listing.
		output, err := exec.CommandContext(t.Context(), "/bin/ls", "-lden", dest).Output()
		if err != nil {
			t.Fatal(err)
		}
		_, entries, _ := strings.Cut(string(output), "\n")
		return entries
	}
	original := acl()
	if !strings.Contains(original, "deny list,search") {
		t.Fatalf("test ACL was not installed: %q", original)
	}
	start := CheckoutTarget{Ref: "refs/heads/main", OID: gitValue(t, dir, "rev-parse", "HEAD")}
	if result, err := repo.AddWorktree(t.Context(), WorktreeAddRequest{Name: "linked", Destination: dest, Start: start}); err != nil || result.Warning != "" {
		t.Fatalf("add: %+v, %v", result, err)
	}
	if after := acl(); after != original {
		t.Fatalf("worktree ACL changed from %q to %q", original, after)
	}
}

func TestWorktreeCreateRejectsAnExternalACLChange(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	dest := filepath.Join(t.TempDir(), "original")
	if err := os.Mkdir(dest, 0o750); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	t.Setenv("GITWAVE_WORKTREE_REAL_GIT", repo.git)
	t.Setenv("GITWAVE_WORKTREE_ACL_DIR", dest)
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(`#!/bin/sh
for arg do
  case "$arg" in
    "worktree: add "*) /bin/chmod +a 'nobody deny list,search' "$GITWAVE_WORKTREE_ACL_DIR" || exit $?; break;;
  esac
done
exec "$GITWAVE_WORKTREE_REAL_GIT" "$@"
`), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	start := CheckoutTarget{Ref: "refs/heads/main", OID: gitValue(t, dir, "rev-parse", "HEAD")}
	if _, err := repo.AddWorktree(t.Context(), WorktreeAddRequest{Name: "linked", Destination: dest, Start: start}); err == nil ||
		!strings.Contains(err.Error(), "destination permissions or contents changed") {
		t.Fatalf("expected a stale directory permission error: %v", err)
	}
	if after, err := os.Stat(dest); err != nil || !os.SameFile(before, after) {
		t.Fatalf("the old directory was replaced: %v, %v", after, err)
	}
	if entries, err := os.ReadDir(dest); err != nil || len(entries) != 0 {
		t.Fatalf("the original directory was changed: %v, %v", entries, err)
	}
	if branches := gitValue(t, dir, "branch", "--list", "linked"); branches != "" {
		t.Fatalf("failed creation left an orphan branch: %q", branches)
	}
}
