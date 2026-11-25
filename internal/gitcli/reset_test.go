package gitcli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHardResetUpdatesBranchIndexAndReflogWithoutClearingIndependentFiles(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	base := gitValue(t, dir, "rev-parse", "HEAD")
	writeFixture(t, dir, "mixed.txt", "branch version\n")
	writeFixture(t, dir, "branch-file.txt", "branch file\n")
	fixtureGit(t, dir, "add", ".")
	fixtureGit(t, dir, "commit", "-m", "Advance branch")
	head := gitValue(t, dir, "rev-parse", "HEAD")
	writeFixture(t, dir, "mixed.txt", "staged edit\n")
	fixtureGit(t, dir, "add", "mixed.txt")
	writeFixture(t, dir, "mixed.txt", "worktree edit\n")
	writeFixture(t, dir, "independent.txt", "keep file\n")
	plan, err := repo.PrepareHardReset(t.Context(), CheckoutTarget{Ref: "refs/heads/main", OID: head}, base)
	if err != nil || plan.DirtyFiles != 1 {
		t.Fatalf("prepare: %+v, %v", plan, err)
	}
	result, err := plan.ApplyHardReset(t.Context())
	if err != nil || result.OID != base || result.Warning != "" {
		t.Fatalf("reset: %+v, %v", result, err)
	}
	if actual := readFixtureStatus(t, repo); actual.HeadOID != base || actual.HeadRef != "refs/heads/main" ||
		len(actual.Changes) != 1 || actual.Changes[0].Path != "independent.txt" {
		t.Fatalf("incorrect reset result: %+v", actual)
	}
	if got := gitValue(t, dir, "rev-parse", "ORIG_HEAD"); got != head {
		t.Fatalf("previous branch tip was not saved: %q", got)
	}
	for _, ref := range []string{"HEAD", "main"} {
		log, err := repo.Reflog(t.Context(), ref)
		if err != nil || len(log) == 0 || log[0].OldOID != head || log[0].NewOID != base || log[0].Action != "reset" {
			t.Fatalf("%s reset reflog: %+v, %v", ref, log, err)
		}
	}
	assertResetFile(t, dir, "independent.txt", "keep file\n")
	assertResetFile(t, dir, "mixed.txt", "original version\n")
	if _, err := os.Stat(filepath.Join(dir, "branch-file.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("branch file survived: %v", err)
	}
}

func TestHardResetRefusesStaleConfirmationAndLocalFileCollisions(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	writeFixture(t, dir, "collision.txt", "target branch file\n")
	fixtureGit(t, dir, "add", "collision.txt")
	fixtureGit(t, dir, "commit", "-m", "Tracked collision")
	target := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "reset", "--hard", "HEAD^")
	shown := CheckoutTarget{Ref: "refs/heads/main", OID: gitValue(t, dir, "rev-parse", "HEAD")}
	writeFixture(t, dir, "mixed.txt", "confirmed edit\n")
	plan, err := repo.PrepareHardReset(t.Context(), shown, target)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "mixed.txt", "new edit after confirmation\n")
	if _, err := plan.ApplyHardReset(t.Context()); err == nil || !strings.Contains(err.Error(), "changed since switching") {
		t.Fatalf("stale reset accepted: %v", err)
	}
	assertResetFile(t, dir, "mixed.txt", "new edit after confirmation\n")

	writeFixture(t, dir, "collision.txt", "local file\n")
	plan, err = repo.PrepareHardReset(t.Context(), shown, target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.ApplyHardReset(t.Context()); err == nil {
		t.Fatal("a reset over an independently created file was accepted")
	}
	if got := gitValue(t, dir, "rev-parse", "HEAD"); got != shown.OID {
		t.Fatalf("failed reset moved the branch: %q", got)
	}
	assertResetFile(t, dir, "collision.txt", "local file\n")
	assertResetFile(t, dir, "mixed.txt", "new edit after confirmation\n")
}

func TestHardResetAtSameHeadClearsOnlyTrackedChanges(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	head := gitValue(t, dir, "rev-parse", "HEAD")
	writeFixture(t, dir, "mixed.txt", "local change\n")
	writeFixture(t, dir, "loose.txt", "local untracked\n")
	plan, err := repo.PrepareHardReset(t.Context(), CheckoutTarget{Ref: "refs/heads/main", OID: head}, head)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.ApplyHardReset(t.Context()); err != nil {
		t.Fatal(err)
	}
	assertResetFile(t, dir, "mixed.txt", "original version\n")
	assertResetFile(t, dir, "loose.txt", "local untracked\n")
}

func TestHardResetToCurrentHeadRestoresChangesIfGitAborts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture wraps an executable with a Unix script")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("the fixture uses Python to interrupt the Git reference protocol")
	}
	dir := newFixture(t)
	repo := openFixture(t, dir)
	head := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "update-ref", "ORIG_HEAD", head)
	writeFixture(t, dir, "mixed.txt", "original staged edit\n")
	fixtureGit(t, dir, "add", "mixed.txt")
	writeFixture(t, dir, "mixed.txt", "original worktree edit\n")
	plan, err := repo.PrepareHardReset(t.Context(), CheckoutTarget{Ref: "refs/heads/main", OID: head}, head)
	if err != nil {
		t.Fatal(err)
	}
	previous := gitValue(t, dir, "reflog", "-1", "--format=%gs", "HEAD")
	t.Setenv("GITWAVE_RESET_REAL_GIT", repo.git)
	wrapper := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(wrapper, []byte(`#!/usr/bin/env python3
import os, subprocess, sys
args = [os.environ["GITWAVE_RESET_REAL_GIT"], *sys.argv[1:]]
if "update-ref" not in args or not any(arg.startswith("reset: moving to ") for arg in args):
    os.execv(args[0], args)
process = subprocess.Popen(args, stdin=subprocess.PIPE, stdout=subprocess.PIPE)
source = sys.stdin.buffer
payload = bytearray()
while not payload.endswith(b"prepare\0"):
    byte = source.read(1)
    if not byte:
        sys.exit(90)
    payload.extend(byte)
process.stdin.write(payload)
process.stdin.flush()
for _ in range(2):
    sys.stdout.buffer.write(process.stdout.readline())
    sys.stdout.buffer.flush()
if source.read(len(b"commit\0")) != b"commit\0":
    sys.exit(90)
process.stdin.write(b"abort\0")
process.stdin.close()
process.wait()
sys.exit(91)
`), 0o700); err != nil {
		t.Fatal(err)
	}
	repo.git = wrapper
	if _, err := plan.ApplyHardReset(t.Context()); err == nil {
		t.Fatal("aborted reset was reported as successful")
	}
	assertResetFile(t, dir, "mixed.txt", "original worktree edit\n")
	if staged := gitValue(t, dir, "show", ":mixed.txt"); staged != "original staged edit" {
		t.Fatalf("aborted reset changed the live index: %q", staged)
	}
	if entry := gitValue(t, dir, "reflog", "-1", "--format=%gs", "HEAD"); entry != previous {
		t.Fatalf("aborted reset added a reflog record: %q", entry)
	}
}

func assertResetFile(t *testing.T, dir, name, expected string) {
	t.Helper()
	actual, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil || string(actual) != expected {
		t.Fatalf("%s: %q, %v", name, actual, err)
	}
}
