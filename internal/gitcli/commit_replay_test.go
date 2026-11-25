package gitcli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCommitReplayCherryPickAndRevertCommitMetadata(t *testing.T) {
	dir, repo, upstream, local := mergeFixture(t, true, false)
	fixtureGit(t, dir, "checkout", "feature")
	fixtureGit(t, dir, "commit", "--amend", "--author=Original Author <author@example.invalid>",
		"--date=2020-03-04T05:06:07+08:00", "-m", "来源标题", "-m", "Original body")
	upstream.OID = gitValue(t, dir, "rev-parse", "HEAD")
	authorship := gitValue(t, dir, "show", "-s", "--format=%an|%ae|%aI", "HEAD")
	fixtureGit(t, dir, "checkout", "main")
	pick, err := repo.ReplayCommit(t.Context(), CherryPickCommit, upstream.OID, local)
	if err != nil {
		t.Fatal(err)
	}
	if got := gitValue(t, dir, "show", "-s", "--format=%P|%an|%ae|%aI", "HEAD"); got != local.OID+"|"+authorship {
		t.Fatalf("cherry-pick changed author or used the wrong parent: %s", got)
	}
	if got := gitValue(t, dir, "show", "-s", "--format=%B", "HEAD"); got != "来源标题\n\nOriginal body\n\n(cherry picked from commit "+upstream.OID+")" {
		t.Fatalf("wrong pick message: %q", got)
	}
	if got := gitValue(t, dir, "reflog", "-1", "--format=%gs"); got != "cherry-pick: 来源标题" {
		t.Fatalf("wrong pick reflog: %q", got)
	}
	local.OID = pick
	if _, err := repo.ReplayCommit(t.Context(), CherryPickCommit, upstream.OID, local); err == nil || !strings.Contains(err.Error(), "no changes") {
		t.Fatalf("duplicate pick: %v", err)
	}
	reverted, err := repo.ReplayCommit(t.Context(), RevertCommit, pick, local)
	if err != nil {
		t.Fatal(err)
	}
	if got := gitValue(t, dir, "show", "-s", "--format=%P|%an|%ae", "HEAD"); got != pick+"|GitWave Test|test@example.invalid" {
		t.Fatalf("revert used the wrong parent or author: %s", got)
	}
	if got := gitValue(t, dir, "show", "-s", "--format=%B", "HEAD"); got != "Revert \"来源标题\"\n\nThis reverts commit "+pick+"." {
		t.Fatalf("wrong revert message: %q", got)
	}
	if got := gitValue(t, dir, "reflog", "-1", "--format=%gs"); got != "revert: Revert \"来源标题\"" {
		t.Fatalf("wrong revert reflog: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "from-feature.txt")); !os.IsNotExist(err) {
		t.Fatalf("revert did not remove the picked file: %v", err)
	}
	if status := readFixtureStatus(t, repo); status.HeadOID != reverted || status.HeadRef != local.Ref ||
		status.Commit != (CommitState{}) || len(status.Changes) != 0 {
		t.Fatalf("working copy not updated with revert: %+v", status)
	}
	local.OID = reverted
	next, err := repo.ReplayCommit(t.Context(), RevertCommit, pick, local)
	if err != nil || next == reverted {
		t.Fatalf("an explicit repeated revert should keep the legacy empty commit: %s, %v", next, err)
	}
	if got := gitValue(t, dir, "diff-tree", "--no-commit-id", "--name-only", "-r", next); got != "" {
		t.Fatalf("repeated revert changed the tree: %s", got)
	}
}

func TestCommitReplayConflictKeepsLiveCheckout(t *testing.T) {
	for _, action := range []CommitReplayAction{CherryPickCommit, RevertCommit} {
		t.Run(string(action), func(t *testing.T) {
			dir, repo, upstream, local := mergeFixture(t, true, true)
			index := filepath.Join(repo.GitDir, "index")
			old, err := os.ReadFile(index)
			if err != nil {
				t.Fatal(err)
			}
			var conflicts *CommitReplayConflicts
			if _, err := repo.ReplayCommit(t.Context(), action, upstream.OID, local); !errors.As(err, &conflicts) ||
				conflicts.Action != action || !slices.Equal(conflicts.Paths, []string{"conflict.txt"}) {
				t.Fatalf("expected conflict paths, got %v", err)
			}
			if new, err := os.ReadFile(index); err != nil || !bytes.Equal(old, new) {
				t.Fatalf("conflict changed the live index: %v", err)
			}
			if got, err := os.ReadFile(filepath.Join(dir, "conflict.txt")); string(got) != "ours\n" || err != nil {
				t.Fatalf("conflict changed the working file: %q, %v", got, err)
			}
			if status := readFixtureStatus(t, repo); status.HeadOID != local.OID || status.HeadRef != local.Ref ||
				status.Commit != (CommitState{}) || len(status.Changes) != 0 {
				t.Fatalf("conflict left operation state: %+v", status)
			}
		})
	}
}

func TestCommitReplayRevertsRootOnDetachedHead(t *testing.T) {
	dir := newFixture(t)
	root := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "checkout", "--detach")
	repo := openFixture(t, dir)
	next, err := repo.ReplayCommit(t.Context(), RevertCommit, root, CheckoutTarget{OID: root})
	if err != nil {
		t.Fatal(err)
	}
	if gitValue(t, dir, "rev-parse", "refs/heads/main") != root || gitValue(t, dir, "rev-parse", "HEAD^") != root ||
		gitValue(t, dir, "ls-tree", "--name-only", next) != "" {
		t.Fatal("root revert did not create an empty tree on the detached checkout")
	}
	if status := readFixtureStatus(t, repo); status.HeadOID != next || status.HeadRef != "" || len(status.Changes) != 0 {
		t.Fatalf("unexpected root revert status: %+v", status)
	}
}

func TestCommitReplayRefusesStaleCheckoutAndProtectsIndependentChanges(t *testing.T) {
	dir, repo, upstream, local := mergeFixture(t, false, false)
	fixtureGit(t, dir, "checkout", "-b", "elsewhere")
	if _, err := repo.ReplayCommit(t.Context(), CherryPickCommit, upstream.OID, local); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("expected stale checked-out branch error: %v", err)
	}
	fixtureGit(t, dir, "checkout", "main")
	writeFixture(t, dir, "mixed.txt", "staged work\n")
	fixtureGit(t, dir, "add", "mixed.txt")
	if _, err := repo.ReplayCommit(t.Context(), CherryPickCommit, upstream.OID, local); err == nil || !strings.Contains(err.Error(), "not clean") {
		t.Fatalf("expected protected staged work: %v", err)
	}
	if got := gitValue(t, dir, "show", ":mixed.txt"); got != "staged work" {
		t.Fatalf("independent staged work lost: %q", got)
	}
	fixtureGit(t, dir, "reset", "--hard", local.OID)
	if err := os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, ".git/info/exclude", "from-feature.txt\n")
	writeFixture(t, dir, "from-feature.txt", "ignored independent work\n")
	if _, err := repo.ReplayCommit(t.Context(), CherryPickCommit, upstream.OID, local); err == nil {
		t.Fatal("pick accepted an overlapping ignored file")
	}
	if got, err := os.ReadFile(filepath.Join(dir, "from-feature.txt")); string(got) != "ignored independent work\n" || err != nil {
		t.Fatalf("ignored file changed: %q, %v", got, err)
	}
	if got := gitValue(t, dir, "rev-parse", "HEAD"); got != local.OID {
		t.Fatalf("rejected replay moved HEAD: %s", got)
	}
}

func TestCommitReplayRefusesToTreatAShallowBoundaryAsARoot(t *testing.T) {
	source := newFixture(t)
	writeFixture(t, source, "new.txt", "the only change in the boundary commit\n")
	fixtureGit(t, source, "add", "new.txt")
	fixtureGit(t, source, "commit", "-m", "Change after root")
	dir := filepath.Join(t.TempDir(), "shallow")
	fixtureGit(t, source, "clone", "--depth=1", "--no-local", source, dir)
	repo := openFixture(t, dir)
	tip := gitValue(t, dir, "rev-parse", "HEAD")
	local := CheckoutTarget{Ref: "refs/heads/main", OID: tip}
	for _, action := range []CommitReplayAction{CherryPickCommit, RevertCommit} {
		if _, err := repo.ReplayCommit(t.Context(), action, tip, local); err == nil || !strings.Contains(err.Error(), "shallow history boundary") {
			t.Fatalf("%s treated a shallow boundary as a root: %v", action, err)
		}
	}
	if got, err := os.ReadFile(filepath.Join(dir, "mixed.txt")); string(got) != "original version\n" || err != nil {
		t.Fatalf("pre-existing worktree file lost: %q, %v", got, err)
	}
	if got := gitValue(t, dir, "rev-parse", "HEAD"); got != tip {
		t.Fatalf("rejected replay moved the shallow branch: %s", got)
	}
}

func TestCommitReplayRestoresWorktreeAfterALateFailure(t *testing.T) {
	_, repo, upstream, local := mergeFixture(t, false, false)
	simulateLateCheckoutFailure(t, repo)
	if _, err := repo.ReplayCommit(t.Context(), CherryPickCommit, upstream.OID, local); err == nil ||
		!strings.Contains(err.Error(), "simulated late checkout failure") {
		t.Fatalf("late failure: %v", err)
	}
	if status := readFixtureStatus(t, repo); status.HeadOID != local.OID || status.HeadRef != local.Ref ||
		status.Commit != (CommitState{}) || len(status.Changes) != 0 {
		t.Fatalf("late failure left a changed checkout: %+v", status)
	}
}

func TestCommitReplayKeepsSubmoduleDirectoryWhenPickReplacesItsGitlink(t *testing.T) {
	for _, fixture := range []struct{ label, name, replacement string }{
		{label: "same path", name: "child", replacement: "child"},
		{label: "ancestor path", name: "outer/child", replacement: "outer"},
	} {
		t.Run(fixture.label, func(t *testing.T) {
			child := newFixture(t)
			dir := newFixture(t)
			fixtureGit(t, dir, "-c", "protocol.file.allow=always", "submodule", "add", "--", child, fixture.name)
			fixtureGit(t, dir, "commit", "-m", "Add the submodule")
			local := CheckoutTarget{Ref: "refs/heads/main", OID: gitValue(t, dir, "rev-parse", "HEAD")}
			fixtureGit(t, dir, "checkout", "-b", "replacement")
			fixtureGit(t, dir, "rm", "--force", "--", fixture.name)
			if err := os.Remove(filepath.Join(dir, fixture.replacement)); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			writeFixture(t, dir, fixture.replacement, "replacement file\n")
			fixtureGit(t, dir, "add", "--all")
			fixtureGit(t, dir, "commit", "-m", "Replace gitlink with a file")
			selected := gitValue(t, dir, "rev-parse", "HEAD")
			fixtureGit(t, dir, "checkout", "main")
			fixtureGit(t, dir, "-c", "protocol.file.allow=always", "submodule", "update", "--init", "--", fixture.name)
			checkout := filepath.Join(dir, filepath.FromSlash(fixture.name))
			writeFixture(t, checkout, "mixed.txt", "local submodule edit\n")
			writeFixture(t, checkout, "only-mine.txt", "untracked submodule file\n")
			repo := openFixture(t, dir)
			if _, err := repo.ReplayCommit(t.Context(), CherryPickCommit, selected, local); err == nil ||
				!strings.Contains(err.Error(), "nonempty submodule directory") {
				t.Fatalf("pick accepted replacing a submodule checkout: %v", err)
			}
			for name, expected := range map[string]string{
				"mixed.txt": "local submodule edit\n", "only-mine.txt": "untracked submodule file\n",
			} {
				if data, err := os.ReadFile(filepath.Join(checkout, name)); string(data) != expected || err != nil {
					t.Fatalf("submodule file %s changed: %q, %v", name, data, err)
				}
			}
			if got := gitValue(t, dir, "rev-parse", "HEAD"); got != local.OID {
				t.Fatalf("rejected pick moved HEAD: %s", got)
			}
		})
	}
}
