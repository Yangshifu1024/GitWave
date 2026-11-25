package gitcli

import (
	"errors"
	"strings"
	"testing"
)

func TestAICommitContextUsesOnlyInspectableStagedBlobs(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	key := "-----BEGIN PRIVATE KEY-----\n" + strings.Repeat("harmless short fixture line\n", 10) + "short-body-before\n-----END PRIVATE KEY-----\n"
	writeFixture(t, dir, "fixture.pem", key)
	writeFixture(t, dir, "app.txt", "const value = 1;\n")
	fixtureGit(t, dir, "add", ".")
	fixtureGit(t, dir, "commit", "-m", "Initial AI fixture")
	head := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	if _, err := repo.AICommitContext(t.Context(), readFixtureStatus(t, repo)); !errors.Is(err, ErrNoAIStagedChanges) {
		t.Fatalf("clean staged context = %v", err)
	}
	writeFixture(t, dir, "fixture.pem", strings.ReplaceAll(key, "short-body-before", "short-body-after"))
	writeFixture(t, dir, "app.txt", "const value = 7;\nAuthorization: Basic sample-token\n")
	fixtureGit(t, dir, "add", ".")
	writeFixture(t, dir, "app.txt", "worktree-only-content\n")
	writeFixture(t, dir, "untracked.txt", "untracked-only-content\n")
	shown := readFixtureStatus(t, repo)
	snapshot, err := repo.AICommitContext(t.Context(), shown)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Initial AI fixture", "+const value = 7;", "[REDACTED]", "1 file(s) omitted"} {
		if !strings.Contains(snapshot.Context, expected) {
			t.Errorf("missing %q in context", expected)
		}
	}
	for _, denied := range []string{"sample-token", "fixture.pem", "short-body-after", "worktree-only-content", "untracked-only-content"} {
		if strings.Contains(snapshot.Context, denied) {
			t.Errorf("unexpected %q in context", denied)
		}
	}
	if err := repo.CheckAICommitContext(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "add", "app.txt")
	if _, err := repo.AICommitContext(t.Context(), shown); !errors.Is(err, ErrAIStagedChanged) {
		t.Fatalf("pre-request modified index check = %v", err)
	}
	if err := repo.CheckAICommitContext(t.Context(), snapshot); !errors.Is(err, ErrAIStagedChanged) {
		t.Fatalf("modified index check = %v", err)
	}
	if actual := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD")); actual != head {
		t.Fatalf("generation changed HEAD to %s", actual)
	}
}

func TestAICommitContextSupportsUnbornBranch(t *testing.T) {
	dir := t.TempDir()
	fixtureGit(t, dir, "init", "--template=", "-b", "main")
	repo := openFixture(t, dir)
	writeFixture(t, dir, "first.txt", "first staged content\n")
	fixtureGit(t, dir, "add", "first.txt")
	snapshot, err := repo.AICommitContext(t.Context(), readFixtureStatus(t, repo))
	if err != nil || !strings.Contains(snapshot.Context, "+first staged content") {
		t.Fatalf("unborn context = %+v, %v", snapshot, err)
	}
	if err := repo.CheckAICommitContext(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
}

func TestAICommitContextChecksStagedRenameAndDeletion(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	fixtureGit(t, dir, "mv", "old name.txt", "new name.txt")
	fixtureGit(t, dir, "rm", "mixed.txt")
	snapshot, err := repo.AICommitContext(t.Context(), readFixtureStatus(t, repo))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"old name.txt", "new name.txt", "mixed.txt"} {
		if !strings.Contains(snapshot.Context, name) {
			t.Errorf("staged context does not mention %q", name)
		}
	}
}
