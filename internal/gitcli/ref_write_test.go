package gitcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalBranchLifecyclePreservesCheckoutAndTracking(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	tip := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	fixtureGit(t, dir, "remote", "add", "origin", "https://example.invalid/gitwave.git")
	fixtureGit(t, dir, "update-ref", "refs/remotes/origin/main", tip)
	assertRef := func(want string, args ...string) {
		t.Helper()
		if got := strings.TrimSpace(fixtureGit(t, dir, args...)); got != want {
			t.Fatalf("git %v: got %q, want %q", args, got, want)
		}
	}
	if err := repo.CreateBranch(t.Context(), "feature/local", tip); err != nil {
		t.Fatal(err)
	}
	assertRef("main", "branch", "--show-current")
	assertRef(tip, "rev-parse", "refs/heads/feature/local")
	if err := repo.SetBranchUpstream(t.Context(), "main", tip, "", "refs/remotes/origin/main"); err != nil {
		t.Fatal(err)
	}
	if err := repo.RenameBranch(t.Context(), "main", "trunk", tip); err != nil {
		t.Fatal(err)
	}
	assertRef("trunk", "branch", "--show-current")
	assertRef("refs/remotes/origin/main", "rev-parse", "--symbolic-full-name", "@{upstream}")
	if err := repo.SetBranchUpstream(t.Context(), "trunk", tip, "refs/remotes/origin/main", ""); err != nil {
		t.Fatal(err)
	}
	refs, err := repo.References(t.Context())
	if err != nil || findRef(refs, "refs/heads/trunk").Upstream != "" {
		t.Fatalf("clearing upstream failed: %+v, %v", refs, err)
	}
	if err := repo.DeleteBranch(t.Context(), "trunk", tip); err == nil {
		t.Fatal("deleted the current branch")
	}
	if err := repo.DeleteBranch(t.Context(), "feature/local", tip); err != nil {
		t.Fatal(err)
	}
	assertRef("trunk", "branch", "--format=%(refname:short)")
}

func TestBranchEditsRejectStaleAndOccupiedReferences(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	tip := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	fixtureGit(t, dir, "branch", "feature")
	linked := filepath.Join(t.TempDir(), "linked checkout")
	fixtureGit(t, dir, "worktree", "add", linked, "feature")
	if err := repo.RenameBranch(t.Context(), "feature", "elsewhere", tip); err == nil || !strings.Contains(err.Error(), "checked out in a worktree") {
		t.Fatalf("renamed an occupied branch: %v", err)
	}
	if err := repo.DeleteBranch(t.Context(), "feature", tip); err == nil {
		t.Fatal("deleted an occupied branch")
	}
	if got := strings.TrimSpace(fixtureGit(t, linked, "branch", "--show-current")); got != "feature" {
		t.Fatalf("linked checkout moved: %q", got)
	}
	fixtureGit(t, dir, "commit", "--allow-empty", "-m", "External commit")
	if err := repo.RenameBranch(t.Context(), "main", "stale", tip); err == nil || !strings.Contains(err.Error(), "moved or changed") {
		t.Fatalf("renamed a branch newer than the view: %v", err)
	}
	if err := repo.CreateBranch(t.Context(), "@{-1}", tip); err == nil || !strings.Contains(err.Error(), "invalid Git reference name") {
		t.Fatalf("accepted Git shorthand as a literal branch name: %v", err)
	}
}

func TestBranchDeletionProtectsLinkedCheckoutsAndPausedOperations(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	tip := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	fixtureGit(t, dir, "branch", "feature")
	path := filepath.Join(t.TempDir(), "other checkout")
	fixtureGit(t, dir, "worktree", "add", "--detach", path, tip)
	linked := openFixture(t, path)
	unlock, err := repo.lockBranchWorktreeIndices(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := fixtureCommand(t, path, "switch", "feature").CombinedOutput()
	unlock()
	if err == nil || !strings.Contains(string(out), "index.lock") {
		t.Fatalf("linked checkout switched while indices were locked: %s (%v)", out, err)
	}
	if err := os.Mkdir(filepath.Join(linked.GitDir, "rebase-merge"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, linked.GitDir, "rebase-merge/head-name", "refs/heads/feature\n")
	if err := repo.DeleteBranch(t.Context(), "feature", tip); err == nil || !strings.Contains(err.Error(), "paused Git operation") {
		t.Fatalf("deleted branch used by a paused rebase: %v", err)
	}
	if err := os.Remove(filepath.Join(linked.GitDir, "rebase-merge", "head-name")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(linked.GitDir, "rebase-merge")); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteBranch(t.Context(), "feature", tip); err != nil {
		t.Fatal(err)
	}
}

func TestBranchEditsRejectCaseAliasCheckedOutInLinkedWorktree(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	tip := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	fixtureGit(t, dir, "branch", "feature")
	if _, err := os.Stat(filepath.Join(repo.CommonDir, "refs", "heads", "FEATURE")); os.IsNotExist(err) {
		t.Skip("this filesystem distinguishes ref names by case")
	} else if err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(t.TempDir(), "aliased checkout")
	fixtureGit(t, dir, "worktree", "add", linked, "FEATURE")
	if err := repo.RenameBranch(t.Context(), "feature", "elsewhere", tip); err == nil {
		t.Fatal("renamed a ref checked out through a filesystem alias")
	}
	if err := repo.DeleteBranch(t.Context(), "feature", tip); err == nil {
		t.Fatal("deleted a ref checked out through a filesystem alias")
	}
	if got := strings.TrimSpace(fixtureGit(t, linked, "rev-parse", "HEAD")); got != tip {
		t.Fatalf("linked checkout lost its tip: %q", got)
	}
}

func TestBranchRenameFailsBeforeMovingHEADWhenConfigurationIsLocked(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	tip := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	writeFixture(t, repo.CommonDir, "config.lock", "")
	if err := repo.RenameBranch(t.Context(), "main", "trunk", tip); err == nil || !strings.Contains(err.Error(), "configuration is locked") {
		t.Fatalf("rename accepted a locked config: %v", err)
	}
	if got := strings.TrimSpace(fixtureGit(t, dir, "symbolic-ref", "HEAD")); got != "refs/heads/main" {
		t.Fatalf("failed rename changed HEAD to %q", got)
	}
}

func TestTagEditsRequireTheObservedObjectAndDoNotRunHooks(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	base := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	fixtureGit(t, dir, "commit", "--allow-empty", "-m", "Second version")
	next := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	hooks := t.TempDir()
	if err := os.WriteFile(filepath.Join(hooks, "reference-transaction"), []byte("#!/bin/sh\nexit 34\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "config", "core.hooksPath", hooks)
	fixtureGit(t, dir, "config", "tag.gpgSign", "true")
	ctx := t.Context()
	if err := repo.CreateTag(ctx, "release/v1", base, "", ""); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(fixtureGit(t, dir, "cat-file", "-t", "refs/tags/release/v1")); got != "commit" {
		t.Fatalf("lightweight tag stored %q", got)
	}
	if err := repo.CreateTag(ctx, "release/v1", next, "", ""); err == nil {
		t.Fatal("tag creation silently replaced an existing tag")
	}
	note := "Version one\n\nRelease notes"
	if err := repo.CreateTag(ctx, "release/v1", next, note, base); err != nil {
		t.Fatal(err)
	}
	refs, err := repo.References(ctx)
	tag := findRef(refs, "refs/tags/release/v1")
	if err != nil || tag.Type != "tag" || tag.PeeledOID != next || tag.OID == next || tag.OID == base {
		t.Fatalf("wrong annotated tag: %+v, %v", tag, err)
	}
	if got, err := repo.TagAnnotation(ctx, tag.OID); got != note || err != nil {
		t.Fatalf("tag annotation: %q, %v", got, err)
	}
	if err := repo.DeleteTag(ctx, "release/v1", base); err == nil {
		t.Fatal("stale deletion removed the new tag")
	}
	if err := repo.DeleteTag(ctx, "release/v1", tag.OID); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(fixtureGit(t, dir, "tag", "--list")); got != "" {
		t.Fatalf("tag still exists: %q", got)
	}
	if got := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD")); got != next {
		t.Fatalf("editing tags moved HEAD: %q", got)
	}
}

func TestDirectReferenceTransactionRejectsSymbolicRefWithMatchingOID(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	tip := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	fixtureGit(t, dir, "tag", "base")
	fixtureGit(t, dir, "symbolic-ref", "refs/tags/alias", "refs/tags/base")
	if err := repo.editDirectRef(t.Context(), "refs/tags/alias", tip, "", "test symbolic guard", nil); err == nil {
		t.Fatal("deleted a different reference type at the same object")
	}
	if got := strings.TrimSpace(fixtureGit(t, dir, "symbolic-ref", "refs/tags/alias")); got != "refs/tags/base" {
		t.Fatalf("symbolic ref moved: %q", got)
	}
}
