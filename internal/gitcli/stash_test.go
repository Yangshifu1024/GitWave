package gitcli

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSaveStashPreservesIndexWorktreeAndUntracked(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	base := gitValue(t, dir, "rev-parse", "HEAD")
	original := gitValue(t, dir, "show", "HEAD:mixed.txt")
	writeFixture(t, dir, "mixed.txt", "staged\n")
	fixtureGit(t, dir, "add", "mixed.txt")
	writeFixture(t, dir, "mixed.txt", "in working tree\n")
	writeFixture(t, dir, "新 文件.txt", "never staged\n")
	if err := os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(dir, ".git"), "info/exclude", "private.log\n")
	writeFixture(t, dir, "private.log", "ignored\n")
	oid, err := repo.SaveStash(t.Context(), readFixtureStatus(t, repo), "two versions", true)
	if err != nil || oid == "" {
		t.Fatalf("save stash: %q, %v", oid, err)
	}
	entries, err := repo.Stashes(t.Context())
	if err != nil || len(entries) != 1 || entries[0].Index != 0 || entries[0].OID != oid || entries[0].Message != "On main: two versions" {
		t.Fatalf("stash list: %+v, %v", entries, err)
	}
	if result := readFixtureStatus(t, repo); result.HeadOID != base || len(result.Changes) != 0 {
		t.Fatalf("unexpected restored state: %+v", result)
	}
	if got := gitValue(t, dir, "show", ":mixed.txt"); got != original {
		t.Fatalf("index: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "新 文件.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("untracked file after save: %v", err)
	}
	if saved := gitValue(t, dir, "show", oid+"^2:mixed.txt"); saved != "staged" {
		t.Fatalf("stash index: %q", saved)
	}
	if saved := gitValue(t, dir, "show", oid+":mixed.txt"); saved != "in working tree" {
		t.Fatalf("stash working copy: %q", saved)
	}
	if ref := gitValue(t, dir, "for-each-ref", "refs/gitwave/checkout-recovery/"); ref != "" {
		t.Fatalf("unexpected recovery reference: %s", ref)
	}
	files, err := repo.StashDiff(t.Context(), oid, "", false)
	if err != nil || len(files.Files) != 2 || files.Files[0].Path != "mixed.txt" ||
		files.Files[0].Untracked || files.Files[1].Path != "新 文件.txt" || !files.Files[1].Untracked {
		t.Fatalf("stash files: %+v, %v", files, err)
	}
	diff, err := repo.StashDiff(t.Context(), oid, "新 文件.txt", false)
	if err != nil || len(diff.Files) != 1 || diff.Files[0].Additions != 1 || diff.Files[0].OldSHA != "" ||
		!diff.Files[0].Untracked || len(diff.Files[0].Hunks) != 1 {
		t.Fatalf("untracked diff: %+v, %v", diff, err)
	}
	fixtureGit(t, dir, "stash", "apply", "--index", oid)
	for file, expected := range map[string]string{
		"mixed.txt": "in working tree\n", "新 文件.txt": "never staged\n", "private.log": "ignored\n",
	} {
		if data, err := os.ReadFile(filepath.Join(dir, file)); err != nil || string(data) != expected {
			t.Fatalf("file %s: %q, %v", file, data, err)
		}
	}
}

func TestSaveStashTrackedOnlyKeepsNewFiles(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	writeFixture(t, dir, "outside.txt", "new\n")
	if _, err := repo.SaveStash(t.Context(), readFixtureStatus(t, repo), "", false); err == nil {
		t.Fatal("created a stash from excluded changes")
	}
	writeFixture(t, dir, "mixed.txt", "edited\n")
	oid, err := repo.SaveStash(t.Context(), readFixtureStatus(t, repo), "", false)
	if err != nil {
		t.Fatal(err)
	}
	status := readFixtureStatus(t, repo)
	if len(status.Changes) != 1 || status.Changes[0].Path != "outside.txt" || status.Changes[0].Kind != '?' {
		t.Fatalf("remaining changes: %+v", status.Changes)
	}
	header, err := repo.readCommitHeader(t.Context(), oid)
	if err != nil || len(header.parents) != 2 || header.message != "On main: WIP" {
		t.Fatalf("stash header: %+v, %v", header, err)
	}
}

func TestSaveStashProtectsUnselectedReplacementAndStaleHead(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	fixtureGit(t, dir, "rm", "mixed.txt")
	writeFixture(t, dir, "mixed.txt", "untracked replacement\n")
	shown := readFixtureStatus(t, repo)
	if oid, err := repo.SaveStash(t.Context(), shown, "", false); err == nil || oid != "" {
		t.Fatalf("saved over untracked replacement: %q, %v", oid, err)
	}
	fixtureGit(t, dir, "commit", "-m", "external deletion")
	if oid, err := repo.SaveStash(t.Context(), shown, "", true); !errors.Is(err, ErrStashTargetChanged) || oid != "" {
		t.Fatalf("stale head: %q, %v", oid, err)
	}
	p := filepath.Join(dir, "mixed.txt")
	if data, err := os.ReadFile(p); err != nil || string(data) != "untracked replacement\n" {
		t.Fatalf("replacement: %q, %v", data, err)
	}
	if entries, err := repo.Stashes(t.Context()); err != nil || len(entries) != 0 {
		t.Fatalf("stash list should be empty: %+v, %v", entries, err)
	}
}

func TestStashListsExistingGitEntriesInReflogOrder(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	for _, message := range []string{"CLI first", "CLI second"} {
		writeFixture(t, dir, "mixed.txt", message+"\n")
		fixtureGit(t, dir, "stash", "push", "-m", message)
	}
	entries, err := repo.Stashes(t.Context())
	if err != nil || len(entries) != 2 {
		t.Fatalf("existing stashes: %+v, %v", entries, err)
	}
	if entries[0].Index != 0 || !strings.Contains(entries[0].Message, "CLI second") ||
		entries[1].Index != 1 || !strings.Contains(entries[1].Message, "CLI first") {
		t.Fatalf("unexpected stash order: %+v", entries)
	}
}

func TestStashListsInBareRepository(t *testing.T) {
	dir := newFixture(t)
	writeFixture(t, dir, "mixed.txt", "entry\n")
	fixtureGit(t, dir, "stash", "push", "-m", "existing stash")
	fixtureGit(t, dir, "config", "core.bare", "true")
	repo := openFixture(t, filepath.Join(dir, ".git"))
	entries, err := repo.Stashes(t.Context())
	if err != nil || len(entries) != 1 || !strings.Contains(entries[0].Message, "existing stash") {
		t.Fatalf("bare stash list: %+v, %v", entries, err)
	}
}

func TestSaveStashKeepsLocalChangesWhenStashTipIsLocked(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	writeFixture(t, dir, "mixed.txt", "local edit\n")
	writeFixture(t, dir, "new.txt", "new file\n")
	refLock := filepath.Join(repo.CommonDir, "refs", "stash.lock")
	if err := os.WriteFile(refLock, []byte("external lock"), 0o600); err != nil {
		t.Fatal(err)
	}
	if oid, err := repo.SaveStash(t.Context(), readFixtureStatus(t, repo), "locked", true); err == nil || oid != "" {
		t.Fatalf("stash ref lock was ignored: %q, %v", oid, err)
	}
	for name, want := range map[string]string{"mixed.txt": "local edit\n", "new.txt": "new file\n"} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(data) != want {
			t.Fatalf("%s: %q, %v", name, data, err)
		}
	}
	if data, err := os.ReadFile(refLock); err != nil || string(data) != "external lock" {
		t.Fatalf("external lock changed: %q, %v", data, err)
	}
}

func TestSaveStashRetainsSnapshotAfterCleanupFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a shell wrapper")
	}
	dir := newFixture(t)
	repo := openFixture(t, dir)
	writeFixture(t, dir, "mixed.txt", "staged bytes\n")
	fixtureGit(t, dir, "add", "mixed.txt")
	writeFixture(t, dir, "mixed.txt", "worktree bytes\n")
	shown := readFixtureStatus(t, repo)
	t.Setenv("GITWAVE_STASH_REAL_GIT", repo.git)
	wrapper := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(wrapper, []byte(`#!/bin/sh
for arg do
  if [ "$arg" = --index-info ]; then
    echo "simulated stash cleanup failure" >&2
    exit 93
  fi
done
exec "$GITWAVE_STASH_REAL_GIT" "$@"
`), 0o700); err != nil {
		t.Fatal(err)
	}
	repo.git = wrapper
	oid, err := repo.SaveStash(t.Context(), shown, "recovery", false)
	if !validOID(oid) || err == nil || !strings.Contains(err.Error(), "could not be fully cleared") {
		t.Fatalf("expected recoverable failure: %q, %v", oid, err)
	}
	if got := gitValue(t, dir, "rev-parse", "refs/stash"); got != oid {
		t.Fatalf("snapshot missing from stash: %q, expected %s", got, oid)
	}
	if got := gitValue(t, dir, "show", oid+":mixed.txt"); got != "worktree bytes" {
		t.Fatalf("stash lost working bytes: %q", got)
	}
	if got := gitValue(t, dir, "show", oid+"^2:mixed.txt"); got != "staged bytes" {
		t.Fatalf("stash lost staged bytes: %q", got)
	}
	if got := gitValue(t, dir, "show", ":mixed.txt"); got != "staged bytes" {
		t.Fatalf("cleanup failure modified live index: %q", got)
	}
}

func TestSaveStashDuringInteractiveRebaseEdit(t *testing.T) {
	dir, repo, plan := interactiveFixture(t, "edit", "remaining")
	todos := plan.Todos()
	todos[0].Action = RebaseEdit
	if result, err := plan.Apply(t.Context(), todos); err != nil || !result.Paused {
		t.Fatalf("pause: %+v, %v", result, err)
	}
	writeFixture(t, dir, "edit.txt", "keep for later\n")
	shown := readFixtureStatus(t, repo)
	oid, err := repo.SaveStash(t.Context(), shown, "keep edit", true)
	if oid == "" || err != nil {
		t.Fatalf("save while paused: %q, %v", oid, err)
	}
	status := readFixtureStatus(t, repo)
	if status.HeadOID != shown.HeadOID || status.Commit != shown.Commit || len(status.Changes) != 0 {
		t.Fatalf("stash changed rebase pause: %+v", status)
	}
	if result, err := repo.ContinueInteractiveRebase(t.Context(), status.Commit.InteractiveRebase); err != nil || result.Paused {
		t.Fatalf("continue after stash: %+v, %v", result, err)
	}
	if got := gitValue(t, dir, "show", oid+":edit.txt"); got != "keep for later" {
		t.Fatalf("stash lost pause edits: %q", got)
	}
}

func TestStashUsesExactRefAndLeavesHomonymousBranchAlone(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	fixtureGit(t, dir, "branch", "refs/stash")
	branch := gitValue(t, dir, "show-ref", "--verify", "--hash", "refs/heads/refs/stash")
	if entries, err := repo.Stashes(t.Context()); err != nil || len(entries) != 0 {
		t.Fatalf("a branch was treated as a stash: %+v, %v", entries, err)
	}
	writeFixture(t, dir, "mixed.txt", "actual stash\n")
	oid, err := repo.SaveStash(t.Context(), readFixtureStatus(t, repo), "real stash", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := fixtureStashEntry(t, repo); got.OID != oid || got.Message != "On main: real stash" {
		t.Fatalf("stash was saved to the wrong ref: %+v", got)
	}
	if got := gitValue(t, dir, "show-ref", "--verify", "--hash", "refs/heads/refs/stash"); got != branch {
		t.Fatalf("the branch tip was changed: %q", got)
	}
}
