package gitcli

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestDropStashSelectedEntryAndNewestAndLast(t *testing.T) {
	for _, format := range []string{"files", "reftable"} {
		t.Run(format, func(t *testing.T) {
			options := []string{"--ref-format=" + format}
			if format == "reftable" {
				options = append(options, "--object-format=sha256")
			}
			dir := newFixture(t, options...)
			repo := openFixture(t, dir)
			base := gitValue(t, dir, "rev-parse", "HEAD")
			fixtureGit(t, dir, "branch", "keep-ref")
			for _, name := range []string{"first", "middle", "latest"} {
				writeFixture(t, dir, "mixed.txt", name+"\n")
				fixtureGit(t, dir, "stash", "push", "-m", name)
			}
			before := fixtureStashList(t, repo)
			if err := repo.DropStash(t.Context(), before[1]); err != nil {
				t.Fatal(err)
			}
			assertStashOrder(t, repo, before[0], before[2])
			if err := repo.DropStash(t.Context(), fixtureStashEntry(t, repo)); err != nil {
				t.Fatal(err)
			}
			assertStashOrder(t, repo, before[2])
			if err := repo.DropStash(t.Context(), fixtureStashEntry(t, repo)); err != nil {
				t.Fatal(err)
			}
			assertStashOrder(t, repo)
			if out, err := repo.exactRootRef(t.Context(), stashRef); err != nil || out != "" {
				t.Fatalf("the last stash reference survived: %s, %v", out, err)
			}
			if head := gitValue(t, dir, "rev-parse", "keep-ref"); head != base {
				t.Fatalf("another branch changed: %q", head)
			}
			fixtureGit(t, dir, "fsck", "--no-dangling")
			writeFixture(t, dir, "mixed.txt", "stash after deletion\n")
			fixtureGit(t, dir, "stash", "push", "-m", "after")
			if list := fixtureStashList(t, repo); len(list) != 1 || !strings.HasSuffix(list[0].Message, ": after") {
				t.Fatalf("old entries were resurrected: %+v", list)
			}
		})
	}
}

func TestDropStashRemovesPackedTip(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	writeFixture(t, dir, "mixed.txt", "packed stash\n")
	fixtureGit(t, dir, "stash", "push", "-m", "packed")
	writeFixture(t, dir, "mixed.txt", "packed newest stash\n")
	fixtureGit(t, dir, "stash", "push", "-m", "packed newest")
	fixtureGit(t, dir, "tag", "-a", "-m", "tag", "keep-tag")
	fixtureGit(t, dir, "pack-refs", "--all")
	if _, err := os.Stat(filepath.Join(repo.CommonDir, "refs", "stash")); !os.IsNotExist(err) {
		t.Fatalf("expected a packed-only fixture: %v", err)
	}
	entries := fixtureStashList(t, repo)
	if err := repo.DropStash(t.Context(), entries[0]); err != nil {
		t.Fatal(err)
	}
	assertStashOrder(t, repo, entries[1])
	if err := repo.DropStash(t.Context(), fixtureStashEntry(t, repo)); err != nil {
		t.Fatal(err)
	}
	assertStashOrder(t, repo)
	if out, err := repo.exactRootRef(t.Context(), stashRef); err != nil || out != "" {
		t.Fatalf("packed stash survived: %s, %v", out, err)
	}
	if got := gitValue(t, dir, "tag", "--list"); got != "keep-tag" {
		t.Fatalf("another packed ref changed: %s", got)
	}
	fixtureGit(t, dir, "fsck", "--no-dangling")
}

func TestDropStashRejectsStaleOrdinalAndReplacedIdenticalCommit(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	writeFixture(t, dir, "mixed.txt", "first\n")
	fixtureGit(t, dir, "stash", "push", "-m", "first")
	original := fixtureStashEntry(t, repo)
	writeFixture(t, dir, "mixed.txt", "second\n")
	fixtureGit(t, dir, "stash", "push", "-m", "second")
	if err := repo.DropStash(t.Context(), original); err == nil || !strings.Contains(err.Error(), "stash list changed") {
		t.Fatalf("dropped a stale ordinal: %v", err)
	}
	fixtureGit(t, dir, "stash", "drop", "stash@{0}")
	fixtureGit(t, dir, "stash", "drop", "stash@{0}")
	replace := fixtureCommand(t, dir, "stash", "store", "-m", original.Message, original.OID)
	replace.Env = append(replace.Env, "GIT_COMMITTER_DATE=2000-01-02T00:00:00+00:00")
	if out, err := replace.CombinedOutput(); err != nil {
		t.Fatalf("replace reflog entry: %s, %v", out, err)
	}
	if err := repo.DropStash(t.Context(), original); err == nil || !strings.Contains(err.Error(), "stash list changed") {
		t.Fatalf("deleted the replacement reflog record: %v", err)
	}
	if entry := fixtureStashEntry(t, repo); entry.OID != original.OID || entry.Message != original.Message || entry.SameSelection(original) {
		t.Fatalf("incorrect fixture or replacement lost: old=%+v current=%+v", original, entry)
	}
}

func TestDropStashRespectsGitBackendLocks(t *testing.T) {
	for _, tc := range []struct{ format, lock string }{
		{"files", "refs/stash.lock"},
		{"files", "packed-refs.lock"},
		{"reftable", "reftable/tables.list.lock"},
	} {
		t.Run(tc.lock, func(t *testing.T) {
			dir := newFixture(t, "--ref-format="+tc.format)
			repo := openFixture(t, dir)
			writeFixture(t, dir, "mixed.txt", "locked stash\n")
			fixtureGit(t, dir, "stash", "push", "-m", "locked")
			entry := fixtureStashEntry(t, repo)
			lockPath := filepath.Join(repo.CommonDir, filepath.FromSlash(tc.lock))
			if err := os.WriteFile(lockPath, []byte("external Git lock"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := repo.DropStash(t.Context(), entry); err == nil {
				t.Fatal("ignored an external Git lock")
			}
			if data, err := os.ReadFile(lockPath); err != nil || string(data) != "external Git lock" {
				t.Fatalf("external lock was changed: %q, %v", data, err)
			}
			assertStashOrder(t, repo, entry)
		})
	}
}

func TestDropStashInReftableLinkedWorktree(t *testing.T) {
	dir := newFixture(t, "--ref-format=reftable")
	linked := filepath.Join(t.TempDir(), "linked")
	fixtureGit(t, dir, "worktree", "add", "-b", "other-branch", linked)
	writeFixture(t, dir, "mixed.txt", "main stash\n")
	fixtureGit(t, dir, "stash", "push", "-m", "main stash")
	writeFixture(t, linked, "mixed.txt", "linked stash\n")
	fixtureGit(t, linked, "stash", "push", "-m", "linked stash")
	mainRepo, linkedRepo := openFixture(t, dir), openFixture(t, linked)
	entries := fixtureStashList(t, mainRepo)
	if err := linkedRepo.DropStash(t.Context(), entries[1]); err != nil {
		t.Fatal(err)
	}
	assertStashOrder(t, linkedRepo, entries[0])
	assertStashOrder(t, mainRepo, entries[0])
	if got := gitValue(t, linked, "branch", "--show-current"); got != "other-branch" {
		t.Fatalf("linked HEAD changed: %q", got)
	}
}

func fixtureStashList(t *testing.T, repo *Repository) []StashEntry {
	t.Helper()
	entries, err := repo.Stashes(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func assertStashOrder(t *testing.T, repo *Repository, entries ...StashEntry) {
	t.Helper()
	expected := slices.Clone(entries)
	for i := range expected {
		expected[i].Index = i
	}
	if got := fixtureStashList(t, repo); !slices.EqualFunc(got, expected, func(a, b StashEntry) bool { return a.Index == b.Index && a.sameRecord(b) }) {
		t.Fatalf("actual stash list: %+v, expected %+v", got, expected)
	}
}

func TestPopStashRetainsConflictAndDropsOnlyAfterCleanApply(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	writeFixture(t, dir, "conflict.txt", "stashed version\n")
	fixtureGit(t, dir, "stash", "push", "-m", "conflict")
	writeFixture(t, dir, "mixed.txt", "stash to pop\n")
	fixtureGit(t, dir, "stash", "push", "-m", "clean")
	writeFixture(t, dir, "conflict.txt", "updated branch\n")
	fixtureGit(t, dir, "commit", "-am", "advance")
	entries := fixtureStashList(t, repo)
	result, err := repo.PopStash(t.Context(), entries[1], readFixtureStatus(t, repo))
	if err != nil || result.Dropped || !slices.Equal(result.Conflicts, []string{"conflict.txt"}) {
		t.Fatalf("pop with conflicts: %+v, %v", result, err)
	}
	assertStashOrder(t, repo, entries...)
	fixtureGit(t, dir, "reset", "--hard", "HEAD")
	result, err = repo.PopStash(t.Context(), entries[0], readFixtureStatus(t, repo))
	if err != nil || !result.Dropped || result.Warning != nil || len(result.Conflicts) != 0 {
		t.Fatalf("clean pop: %+v, %v", result, err)
	}
	assertStashOrder(t, repo, entries[1])
	if data, err := os.ReadFile(filepath.Join(dir, "mixed.txt")); err != nil || string(data) != "stash to pop\n" {
		t.Fatalf("stash was not applied: %q, %v", data, err)
	}
}

func TestPopStashReportsSuccessOfApplyBeforeDropLockFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a shell wrapper")
	}
	dir := newFixture(t)
	repo := openFixture(t, dir)
	writeFixture(t, dir, "mixed.txt", "stash to pop\n")
	fixtureGit(t, dir, "stash", "push", "-m", "recovery")
	entry := fixtureStashEntry(t, repo)
	shown := readFixtureStatus(t, repo)
	wrapper := filepath.Join(t.TempDir(), "git")
	t.Setenv("GITWAVE_STASH_REAL_GIT", repo.git)
	t.Setenv("GITWAVE_STASH_BLOCK_DROP", filepath.Join(repo.CommonDir, "refs", "stash.lock"))
	if err := os.WriteFile(wrapper, []byte(`#!/bin/sh
for arg do
  if [ "$arg" = --show-ref-format ]; then
    printf '%s' external > "$GITWAVE_STASH_BLOCK_DROP" || exit "$?"
    break
  fi
done
exec "$GITWAVE_STASH_REAL_GIT" "$@"
`), 0o700); err != nil {
		t.Fatal(err)
	}
	repo.git = wrapper
	result, err := repo.PopStash(t.Context(), entry, shown)
	if err != nil || result.Dropped || result.Warning == nil || !strings.Contains(result.Warning.Error(), "applied but could not be removed") {
		t.Fatalf("partial pop result: %+v, %v", result, err)
	}
	assertStashOrder(t, repo, entry)
	if data, err := os.ReadFile(filepath.Join(dir, "mixed.txt")); err != nil || string(data) != "stash to pop\n" {
		t.Fatalf("worktree after the applied pop: %q, %v", data, err)
	}
}

func TestDropStashRejectsIdenticalReplacementInBothRefFormats(t *testing.T) {
	for _, format := range []string{"files", "reftable"} {
		t.Run(format, func(t *testing.T) {
			dir := newFixture(t, "--ref-format="+format)
			repo := openFixture(t, dir)
			writeFixture(t, dir, "mixed.txt", "stored twice\n")
			fixtureGit(t, dir, "stash", "push", "-m", "snapshot")
			oid := fixtureStashEntry(t, repo).OID
			fixtureGit(t, dir, "stash", "drop", "stash@{0}")
			store := func() {
				cmd := fixtureCommand(t, dir, "stash", "store", "-m", "identical record", oid)
				cmd.Env = append(cmd.Env, "GIT_COMMITTER_DATE=2000-01-02T00:00:00+00:00")
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("store stash: %s, %v", out, err)
				}
			}
			store()
			old := fixtureStashEntry(t, repo)
			fixtureGit(t, dir, "stash", "drop", "stash@{0}")
			store()
			replacement := fixtureStashEntry(t, repo)
			if !old.sameRecord(replacement) || old.SameSelection(replacement) {
				t.Fatalf("ref storage identity did not distinguish the replacement: %+v, %+v", old, replacement)
			}
			if err := repo.DropStash(t.Context(), old); err == nil || !strings.Contains(err.Error(), "stash list changed") {
				t.Fatalf("deleted the identical replacement: %v", err)
			}
			assertStashOrder(t, repo, replacement)
		})
	}
}

func TestDropStashRejectsDamagedReflogOrdinal(t *testing.T) {
	dir := newFixture(t, "--ref-format=files")
	repo := openFixture(t, dir)
	writeFixture(t, dir, "mixed.txt", "preserved stash\n")
	fixtureGit(t, dir, "stash", "push", "-m", "before a malformed log entry")
	old := fixtureStashEntry(t, repo)
	log := filepath.Join(repo.CommonDir, "logs", "refs", "stash")
	file, err := os.OpenFile(log, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(old.OID + " " + old.OID + " missing actor and timestamp\n"); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	shown := fixtureStashEntry(t, repo)
	if err := repo.DropStash(t.Context(), shown); err == nil || !strings.Contains(err.Error(), "unreadable records") {
		t.Fatalf("malformed reflog was edited using a false ordinal: %v", err)
	}
	assertStashOrder(t, repo, shown)
}

func TestPopStashOnDetachedReftableLinkedWorktree(t *testing.T) {
	dir := newFixture(t, "--ref-format=reftable", "--object-format=sha256")
	linked := filepath.Join(t.TempDir(), "linked")
	fixtureGit(t, dir, "worktree", "add", "--detach", linked)
	main, repo := openFixture(t, dir), openFixture(t, linked)
	writeFixture(t, linked, "mixed.txt", "popped in linked worktree\n")
	fixtureGit(t, linked, "stash", "push", "-m", "linked fixture")
	entry := fixtureStashEntry(t, main)
	result, err := repo.PopStash(t.Context(), entry, readFixtureStatus(t, repo))
	if err != nil || result.Warning != nil || !result.Dropped {
		t.Fatalf("linked reftable pop: %+v, %v", result, err)
	}
	assertStashOrder(t, main)
	if b, err := os.ReadFile(filepath.Join(linked, "mixed.txt")); err != nil || string(b) != "popped in linked worktree\n" {
		t.Fatalf("linked worktree contents: %q, %v", b, err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "mixed.txt")); err != nil || string(b) != "original version\n" {
		t.Fatalf("the other worktree was modified: %q, %v", b, err)
	}
}

func TestDropStashKeepsSharedRefPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture verifies POSIX group mode bits")
	}
	for _, format := range []string{"files", "reftable"} {
		t.Run(format, func(t *testing.T) {
			dir := newFixture(t, "--ref-format="+format, "--shared=group")
			repo := openFixture(t, dir)
			for _, value := range []string{"earlier", "latest"} {
				writeFixture(t, dir, "mixed.txt", value+"\n")
				fixtureGit(t, dir, "stash", "push", "-m", value)
			}
			if err := repo.DropStash(t.Context(), fixtureStashEntry(t, repo)); err != nil {
				t.Fatal(err)
			}
			assertMode := func(path string) {
				t.Helper()
				info, err := os.Stat(filepath.Join(repo.CommonDir, filepath.FromSlash(path)))
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm()&0o060 != 0o060 {
					t.Fatalf("Drop restricted Git group permissions on %s to %o", path, info.Mode().Perm())
				}
			}
			if format == "files" {
				assertMode("refs/stash")
				assertMode("logs/refs/stash")
				return
			}
			data, err := os.ReadFile(filepath.Join(repo.CommonDir, "reftable", "tables.list"))
			if err != nil {
				t.Fatal(err)
			}
			names, err := parseReftableList(data)
			if err != nil || len(names) == 0 {
				t.Fatalf("invalid published stack: %v, %v", names, err)
			}
			assertMode("reftable/tables.list")
			for _, name := range names {
				assertMode("reftable/" + name)
			}
		})
	}
}
