package gitcli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommitTransactionLocksSymbolicHEAD(t *testing.T) {
	dir := newFixture(t)
	fixtureGit(t, dir, "branch", "alternate")
	fixtureGit(t, dir, "symbolic-ref", "refs/heads/alias", "refs/heads/main")
	fixtureGit(t, dir, "symbolic-ref", "HEAD", "refs/heads/alias")
	old := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	tree := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD^{tree}"))
	oid := strings.TrimSpace(fixtureGit(t, dir, "commit-tree", tree, "-p", old, "-m", "New commit"))
	repo := openFixture(t, dir)
	cmd := repo.command(t.Context(), "", "update-ref", "--stdin", "-z")
	published, _, txErr := runReferenceTransaction(cmd, "update HEAD\x00"+oid+"\x00"+old+"\x00", func() error {
		switchRef := repo.command(t.Context(), "", "symbolic-ref", "HEAD", "refs/heads/alternate")
		diagnostic, err := switchRef.CombinedOutput()
		if err == nil || !strings.Contains(string(diagnostic), "HEAD.lock") {
			return errors.New("a prepared commit did not protect the attached HEAD: " + string(diagnostic))
		}
		return nil
	})
	if !published || txErr != nil {
		t.Fatalf("reference transaction: %v", txErr)
	}
	if got := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD", "main", "alternate")); got != oid+"\n"+oid+"\n"+old {
		t.Fatalf("unexpected result: %s", got)
	}
}

func TestCommitCreatesUnbornHeadFromOnlyTheStagedSnapshot(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	fixtureGit(t, dir, "init", "--template=", "-b", "main")
	writeFixture(t, dir, "selected.txt", "staged contents\n")
	fixtureGit(t, dir, "add", "selected.txt")
	writeFixture(t, dir, "selected.txt", "unstaged contents\n")
	writeFixture(t, dir, "other.txt", "untracked contents\n")
	repo := openFixture(t, dir)
	before := readFixtureStatus(t, repo)
	result, err := repo.Commit(t.Context(), before, "  Initial review\n\n# Body text\n ", "")
	if err != nil || !validOID(result.SHA) || result.Warning != "" {
		t.Fatalf("initial commit: %+v, %v", result, err)
	}
	if got := fixtureGit(t, dir, "show", "HEAD:selected.txt"); got != "staged contents\n" {
		t.Fatalf("committed unstaged data: %q", got)
	}
	if got := strings.TrimSpace(fixtureGit(t, dir, "show", "-s", "--format=%an <%ae>%n%B", "HEAD")); got != "GitWave <noreply@gitwave.local>\nInitial review\n\n# Body text" {
		t.Fatalf("unexpected identity or message: %q", got)
	}
	status := readFixtureStatus(t, repo)
	if status.HeadOID != result.SHA || len(status.Changes) != 2 || len(stagedEntries(status)) != 0 {
		t.Fatalf("wrong status after commit: %+v", status)
	}
	if file, err := os.ReadFile(filepath.Join(dir, "selected.txt")); err != nil || string(file) != "unstaged contents\n" {
		t.Fatalf("working copy changed: %q, %v", file, err)
	}
}

func TestAmendPreservesAuthorParentsAndReflog(t *testing.T) {
	dir := newFixture(t)
	writeFixture(t, dir, "mixed.txt", "changed\n")
	fixtureGit(t, dir, "commit", "-am", "Previous message", "--author=Original Author <original@example.invalid>",
		"--date=2021-02-03T04:05:06+08:00")
	fixtureGit(t, dir, "config", "user.name", "New Committer")
	format := "--format=%an <%ae>%n%aI%n%P%n%T"
	before := fixtureGit(t, dir, "show", "-s", format, "HEAD")
	repo := openFixture(t, dir)
	status := readFixtureStatus(t, repo)
	result, err := repo.Commit(t.Context(), status, "Updated message", status.HeadOID)
	if err != nil || result.SHA == status.HeadOID || !validOID(result.SHA) {
		t.Fatalf("message-only amend: %+v, %v", result, err)
	}
	if after := fixtureGit(t, dir, "show", "-s", format, "HEAD"); after != before {
		t.Fatalf("amend changed author, parents or tree: %q to %q", before, after)
	}
	if got := fixtureGit(t, dir, "show", "-s", "--format=%cn%n%B", "HEAD"); strings.TrimSpace(got) != "New Committer\nUpdated message" {
		t.Fatalf("amended committer or message: %q", got)
	}
	if got := fixtureGit(t, dir, "reflog", "-2", "--format=%H %gs"); !strings.Contains(got, "commit (amend): Updated message") || !strings.Contains(got, status.HeadOID) {
		t.Fatalf("missing amend reflog or old commit: %q", got)
	}
}

func TestCommitRejectsExternalStagingAndChangedAmendTarget(t *testing.T) {
	dir := newFixture(t)
	writeFixture(t, dir, "mixed.txt", "first staging\n")
	fixtureGit(t, dir, "add", "mixed.txt")
	repo := openFixture(t, dir)
	shown := readFixtureStatus(t, repo)
	writeFixture(t, dir, "mixed.txt", "updated staging\n")
	fixtureGit(t, dir, "add", "mixed.txt")
	if _, err := repo.Commit(t.Context(), shown, "Stale commit", ""); err == nil {
		t.Fatal("a stale staged snapshot was committed")
	}
	if got := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD")); got != shown.HeadOID {
		t.Fatalf("HEAD changed on failure: %s", got)
	}
	current := readFixtureStatus(t, repo)
	fixtureGit(t, dir, "commit", "-m", "External commit")
	if _, err := repo.Commit(t.Context(), readFixtureStatus(t, repo), "Stale amend", current.HeadOID); err == nil {
		t.Fatal("amend was applied to a different HEAD")
	}
}

func TestCommitFinishesMergeWithUnchangedTree(t *testing.T) {
	dir := newFixture(t)
	fixtureGit(t, dir, "switch", "-c", "topic")
	writeFixture(t, dir, "topic.txt", "topic\n")
	fixtureGit(t, dir, "add", "topic.txt")
	fixtureGit(t, dir, "commit", "-m", "Topic")
	topic := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	fixtureGit(t, dir, "switch", "main")
	fixtureGit(t, dir, "merge", "--no-ff", "--no-commit", "-s", "ours", "topic")
	repo := openFixture(t, dir)
	status := readFixtureStatus(t, repo)
	if status.Commit.MergeHeads != topic || len(status.Changes) != 0 {
		t.Fatalf("expected an unchanged merge tree: %+v", status)
	}
	result, err := repo.Commit(t.Context(), status, "Merge topic", "")
	if err != nil || result.Warning != "" {
		t.Fatalf("finish merge: %+v, %v", result, err)
	}
	if got := strings.TrimSpace(fixtureGit(t, dir, "show", "-s", "--format=%P", "HEAD")); got != status.HeadOID+" "+topic {
		t.Fatalf("wrong merge parents: %q", got)
	}
	if _, err := os.Stat(filepath.Join(repo.GitDir, "MERGE_HEAD")); !os.IsNotExist(err) {
		t.Fatalf("merge state remained: %v", err)
	}
}

func TestCommitRetainsDetachedHeadAndDoesNotRunConfiguredCommands(t *testing.T) {
	dir := newFixture(t)
	fixtureGit(t, dir, "switch", "--detach")
	hooks := t.TempDir()
	for _, hook := range []string{"pre-commit", "reference-transaction"} {
		if err := os.WriteFile(filepath.Join(hooks, hook), []byte("#!/bin/sh\nexit 53\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	fixtureGit(t, dir, "config", "core.hooksPath", hooks)
	fixtureGit(t, dir, "config", "commit.gpgsign", "true")
	fixtureGit(t, dir, "config", "gpg.program", "gitwave-nonexistent-signing-command")
	writeFixture(t, dir, "mixed.txt", "detached edit\n")
	fixtureGit(t, dir, "add", "mixed.txt")
	repo := openFixture(t, dir)
	status := readFixtureStatus(t, repo)
	result, err := repo.Commit(t.Context(), status, "Detached commit", "")
	if err != nil {
		t.Fatal(err)
	}
	current := readFixtureStatus(t, repo)
	if current.HeadRef != "" || current.HeadOID != result.SHA ||
		strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "main")) != status.HeadOID {
		t.Fatalf("detached commit moved the branch: %+v", current)
	}
}
