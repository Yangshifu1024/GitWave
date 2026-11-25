package gitcli

import (
	"strings"
	"testing"
)

func TestAIExplainCommitReadsPinnedFirstParentObjects(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	private := "-----BEGIN PRIVATE KEY-----\nshort-fake-body\n-----END PRIVATE KEY-----\n"
	writeFixture(t, dir, "private.fixture", private)
	writeFixture(t, dir, "explain.txt", "Old explain content\n")
	fixtureGit(t, dir, "add", ".")
	fixtureGit(t, dir, "commit", "-m", "Before explanation")
	writeFixture(t, dir, "private.fixture", strings.ReplaceAll(private, "short-fake-body", "hidden-fake-body"))
	writeFixture(t, dir, "explain.txt", "Selected explain content\n")
	fixtureGit(t, dir, "add", ".")
	fixtureGit(t, dir, "commit", "-m", "Selected explanation commit")
	oid := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	writeFixture(t, dir, "explain.txt", "Later staged content\n")
	fixtureGit(t, dir, "add", "explain.txt")
	context, err := repo.AIExplainCommitContext(t.Context(), oid)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Selected explanation commit", "-Old explain content", "+Selected explain content"} {
		if !strings.Contains(context, expected) {
			t.Errorf("missing %q from context", expected)
		}
	}
	for _, denied := range []string{"hidden-fake-body", "short-fake-body", "private.fixture", "Later staged content"} {
		if strings.Contains(context, denied) {
			t.Errorf("unexpected %q in commit context", denied)
		}
	}
}

func TestAIPullRequestUsesDefaultRemoteBaseAndValidatesBothTips(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	common := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	writeFixture(t, dir, "upstream-only.txt", "Upstream only\n")
	fixtureGit(t, dir, "add", ".")
	fixtureGit(t, dir, "commit", "-m", "Only upstream changes")
	fixtureGit(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	fixtureGit(t, dir, "switch", "-c", "feature", common)
	writeFixture(t, dir, "feature.txt", "Committed feature\n")
	fixtureGit(t, dir, "add", "feature.txt")
	fixtureGit(t, dir, "commit", "-m", "Add the feature")
	head := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	writeFixture(t, dir, "feature.txt", "Staged only\n")
	fixtureGit(t, dir, "add", "feature.txt")
	context, err := repo.AIPullRequestContext(t.Context(), head, "refs/heads/feature")
	if err != nil {
		t.Fatal(err)
	}
	if context.Base != "origin/main" || !strings.Contains(context.Context, "Add the feature") ||
		!strings.Contains(context.Context, "+Committed feature") || strings.Contains(context.Context, "Upstream only") ||
		strings.Contains(context.Context, "Staged only") {
		t.Fatalf("unexpected PR context: %+v", context)
	}
	fixtureGit(t, dir, "update-ref", "refs/remotes/origin/main", common)
	if err := repo.CheckAIPullRequestContext(t.Context(), context); err != errAIRefsChanged {
		t.Fatalf("changed remote base did not invalidate the snapshot: %v", err)
	}
	fixtureGit(t, dir, "update-ref", "-d", "refs/remotes/origin/main")
	local, err := repo.AIPullRequestContext(t.Context(), head, "refs/heads/feature")
	if err != nil || local.Base != "main" {
		t.Fatalf("local base fallback = %q, %v", local.Base, err)
	}
	fixtureGit(t, dir, "update-ref", "refs/remotes/origin/main", "refs/heads/main")
	if err := repo.CheckAIPullRequestContext(t.Context(), local); err != errAIRefsChanged {
		t.Fatalf("a new higher-priority base at the same OID did not invalidate the snapshot: %v", err)
	}
	fixtureGit(t, dir, "switch", "-c", "renamed-source")
	if _, err := repo.AIPullRequestContext(t.Context(), head, "refs/heads/feature"); err != errAIRefsChanged {
		t.Fatalf("same-OID branch switch did not invalidate the snapshot: %v", err)
	}
}
