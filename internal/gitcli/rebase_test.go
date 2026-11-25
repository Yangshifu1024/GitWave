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

func TestBranchRebaseFastForwardAndAlreadyUpToDate(t *testing.T) {
	dir, repo, upstream, local := mergeFixture(t, false, false)
	fixtureGit(t, dir, "branch", "ORIG_HEAD", upstream.OID)
	result, err := repo.RebaseBranch(t.Context(), upstream, local)
	if err != nil || result.Kind != RebaseFastForward || result.OID != upstream.OID {
		t.Fatalf("fast-forward: %+v, %v", result, err)
	}
	if head, branch := gitValue(t, dir, "rev-parse", "HEAD"), gitValue(t, dir, "branch", "--show-current"); head != upstream.OID || branch != "main" {
		t.Fatalf("checkout = %s on %s", head, branch)
	}
	if original, err := repo.exactRootRef(t.Context(), "ORIG_HEAD"); original != local.OID || err != nil {
		t.Fatalf("ORIG_HEAD = %s, %v", original, err)
	}
	if branch := gitValue(t, dir, "show-ref", "--verify", "--hash", "refs/heads/ORIG_HEAD"); branch != upstream.OID {
		t.Fatalf("the branch named ORIG_HEAD moved: %s", branch)
	}
	if got := gitValue(t, dir, "reflog", "-1", "--format=%gs", "HEAD"); !strings.HasPrefix(got, "rebase: onto ") {
		t.Fatalf("reflog = %s", got)
	}
	local.OID = upstream.OID
	result, err = repo.RebaseBranch(t.Context(), upstream, local)
	if err != nil || result.Kind != RebaseUpToDate || result.OID != local.OID {
		t.Fatalf("already up to date: %+v, %v", result, err)
	}
	if original, err := repo.exactRootRef(t.Context(), "ORIG_HEAD"); original == local.OID || err != nil {
		t.Fatalf("no-op replaced the recovery point: %q, %v", original, err)
	}
}

func TestBranchRebaseReplaysOnlyTheSelectedBranch(t *testing.T) {
	dir, repo, upstream, local := mergeFixture(t, true, false)
	fixtureGit(t, dir, "branch", "keep", local.OID)
	fixtureGit(t, dir, "config", "rebase.updateRefs", "true")
	oldIdentity := gitValue(t, dir, "show", "-s", "--format=%aN|%aE|%aI|%B", local.OID)
	result, err := repo.RebaseBranch(t.Context(), upstream, local)
	if err != nil || result.Kind != RebaseClean || result.OID == local.OID || result.OID == upstream.OID {
		t.Fatalf("rebase: %+v, %v", result, err)
	}
	if head := gitValue(t, dir, "rev-parse", "HEAD"); head != result.OID {
		t.Fatalf("HEAD = %s", head)
	}
	if parent := gitValue(t, dir, "show", "-s", "--format=%P", "HEAD"); parent != upstream.OID {
		t.Fatalf("rewritten parent = %s", parent)
	}
	if other := gitValue(t, dir, "rev-parse", "refs/heads/keep"); other != local.OID {
		t.Fatalf("rebase moved another local branch: %s", other)
	}
	if got := gitValue(t, dir, "show", "-s", "--format=%aN|%aE|%aI|%B", "HEAD"); got != oldIdentity {
		t.Fatalf("rebase changed the commit's author, date or message: %q", got)
	}
	if status := gitValue(t, dir, "status", "--porcelain"); status != "" {
		t.Fatalf("index and worktree do not match rewritten HEAD: %s", status)
	}
}

func TestBranchRebaseConflictPreservesLiveCheckout(t *testing.T) {
	dir, repo, upstream, local := mergeFixture(t, true, true)
	fixtureGit(t, dir, "update-ref", "ORIG_HEAD", upstream.OID)
	index := filepath.Join(repo.GitDir, "index")
	before, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	result, err := repo.RebaseBranch(t.Context(), upstream, local)
	if err != nil || result.Kind != RebaseConflicts || !slices.Equal(result.Conflicts, []string{"conflict.txt"}) {
		t.Fatalf("conflict: %+v, %v", result, err)
	}
	if after, err := os.ReadFile(index); !bytes.Equal(before, after) || err != nil {
		t.Fatalf("rebase conflict changed the live index: %v", err)
	}
	if state := readFixtureStatus(t, repo); state.HeadOID != local.OID || state.HeadRef != local.Ref ||
		state.Commit != (CommitState{}) || len(state.Changes) != 0 {
		t.Fatalf("live checkout changed: %+v", state)
	}
	if original, err := repo.exactRootRef(t.Context(), "ORIG_HEAD"); original != upstream.OID || err != nil {
		t.Fatalf("conflict changed the existing recovery point: %q, %v", original, err)
	}
}

func TestBranchRebaseRejectsDirtyOrChangedCheckout(t *testing.T) {
	dir, repo, upstream, local := mergeFixture(t, false, false)
	writeFixture(t, dir, "mixed.txt", "working copy edit\n")
	var dirty *RebaseDirtyError
	if _, err := repo.RebaseBranch(t.Context(), upstream, local); !errors.As(err, &dirty) || dirty.Files != 1 {
		t.Fatalf("dirty checkout: %v", err)
	}
	fixtureGit(t, dir, "checkout", "--", "mixed.txt")
	fixtureGit(t, dir, "checkout", "-b", "also-main")
	if _, err := repo.RebaseBranch(t.Context(), upstream, local); err == nil || !strings.Contains(err.Error(), "checked-out branch changed") {
		t.Fatalf("rebase accepted a different branch at the same commit: %v", err)
	}
	fixtureGit(t, dir, "checkout", "main")
	fixtureGit(t, dir, "update-ref", upstream.Ref, local.OID)
	if _, err := repo.RebaseBranch(t.Context(), upstream, local); err == nil || !strings.Contains(err.Error(), "reference") {
		t.Fatalf("rebase accepted a different upstream commit: %v", err)
	}
	if head := gitValue(t, dir, "rev-parse", "HEAD"); head != local.OID {
		t.Fatalf("rejected rebase moved HEAD: %s", head)
	}
}

func TestBranchRebaseKeepsIgnoredFilesAndHiddenLocalEdits(t *testing.T) {
	for _, flag := range []string{"ignored", "assume-unchanged", "skip-worktree"} {
		t.Run(flag, func(t *testing.T) {
			dir, repo, upstream, local := mergeFixture(t, false, false)
			fixtureGit(t, dir, "checkout", "feature")
			writeFixture(t, dir, "mixed.txt", "upstream version\n")
			fixtureGit(t, dir, "commit", "-am", "Change a tracked file upstream")
			upstream.OID = gitValue(t, dir, "rev-parse", "HEAD")
			fixtureGit(t, dir, "checkout", "main")
			name := "mixed.txt"
			if flag == "ignored" {
				name = "from-feature.txt"
				if err := os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o755); err != nil {
					t.Fatal(err)
				}
				writeFixture(t, dir, ".git/info/exclude", name+"\n")
			} else {
				fixtureGit(t, dir, "update-index", "--"+flag, name)
			}
			writeFixture(t, dir, name, "independent edit must survive\n")
			if _, err := repo.RebaseBranch(t.Context(), upstream, local); err == nil {
				t.Fatal("rebase accepted an overlapping file")
			}
			if got, err := os.ReadFile(filepath.Join(dir, name)); string(got) != "independent edit must survive\n" || err != nil {
				t.Fatalf("overlapping files changed: %q, %v", got, err)
			}
			if head := gitValue(t, dir, "rev-parse", "HEAD"); head != local.OID {
				t.Fatalf("rejected rebase moved HEAD: %s", head)
			}
		})
	}
}

func TestBranchRebaseRestoresCheckoutAfterLateFailure(t *testing.T) {
	dir, repo, upstream, local := mergeFixture(t, false, false)
	fixtureGit(t, dir, "update-ref", "ORIG_HEAD", upstream.OID)
	simulateLateCheckoutFailure(t, repo)
	if _, err := repo.RebaseBranch(t.Context(), upstream, local); err == nil || !strings.Contains(err.Error(), "simulated late checkout failure") {
		t.Fatalf("late failure: %v", err)
	}
	status := readFixtureStatus(t, repo)
	if status.HeadOID != local.OID || status.HeadRef != local.Ref || len(status.Changes) != 0 {
		t.Fatalf("late failure left inconsistent checkout: %+v", status)
	}
	if original, err := repo.exactRootRef(t.Context(), "ORIG_HEAD"); original != upstream.OID || err != nil {
		t.Fatalf("late failure changed ORIG_HEAD: %q, %v", original, err)
	}
}

func TestBranchRebaseCanLandWhenAllLocalPatchesAlreadyExistUpstream(t *testing.T) {
	dir := newFixture(t)
	fixtureGit(t, dir, "checkout", "-b", "feature")
	writeFixture(t, dir, "common.txt", "equivalent patch\n")
	fixtureGit(t, dir, "add", "common.txt")
	fixtureGit(t, dir, "commit", "-m", "Upstream copy")
	upstream := CheckoutTarget{Ref: "refs/heads/feature", OID: gitValue(t, dir, "rev-parse", "HEAD")}
	fixtureGit(t, dir, "checkout", "main")
	writeFixture(t, dir, "common.txt", "equivalent patch\n")
	fixtureGit(t, dir, "add", "common.txt")
	fixtureGit(t, dir, "commit", "-m", "Local copy")
	local := CheckoutTarget{Ref: "refs/heads/main", OID: gitValue(t, dir, "rev-parse", "HEAD")}
	repo := openFixture(t, dir)
	result, err := repo.RebaseBranch(t.Context(), upstream, local)
	if err != nil || result.Kind != RebaseClean || result.OID != upstream.OID {
		t.Fatalf("branch did not land at upstream after dropping the duplicate patch: %+v, %v", result, err)
	}
	if head := gitValue(t, dir, "rev-parse", "HEAD"); head != upstream.OID {
		t.Fatalf("HEAD = %s", head)
	}
}

func TestBranchRebaseKeepsLocalPatchRevertedUpstream(t *testing.T) {
	dir := newFixture(t)
	fixtureGit(t, dir, "checkout", "-b", "feature")
	writeFixture(t, dir, "patch.txt", "wanted feature\n")
	fixtureGit(t, dir, "add", "patch.txt")
	fixtureGit(t, dir, "commit", "-m", "Upstream adds patch")
	fixtureGit(t, dir, "revert", "--no-edit", "HEAD")
	upstream := CheckoutTarget{Ref: "refs/heads/feature", OID: gitValue(t, dir, "rev-parse", "HEAD")}
	fixtureGit(t, dir, "checkout", "main")
	writeFixture(t, dir, "patch.txt", "wanted feature\n")
	fixtureGit(t, dir, "add", "patch.txt")
	fixtureGit(t, dir, "commit", "-m", "Local also adds patch")
	local := CheckoutTarget{Ref: "refs/heads/main", OID: gitValue(t, dir, "rev-parse", "HEAD")}
	result, err := openFixture(t, dir).RebaseBranch(t.Context(), upstream, local)
	if err != nil || result.Kind != RebaseClean || result.OID == upstream.OID {
		t.Fatalf("rebase dropped a patch that had been reverted upstream: %+v, %v", result, err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "patch.txt")); string(got) != "wanted feature\n" || err != nil {
		t.Fatalf("local patch did not survive: %q, %v", got, err)
	}
}

func TestBranchRebaseProtectsEditsMadeInMergeCommits(t *testing.T) {
	for _, extraEdit := range []bool{false, true} {
		dir, repo, upstream, _ := mergeFixture(t, false, false)
		fixtureGit(t, dir, "merge", "--no-ff", "--no-commit", "feature")
		if extraEdit {
			writeFixture(t, dir, "merge-only.txt", "only this merge introduces it\n")
			fixtureGit(t, dir, "add", "merge-only.txt")
		}
		fixtureGit(t, dir, "commit", "-m", "Local merge")
		fixtureGit(t, dir, "commit", "--allow-empty", "-m", "Ordinary local commit")
		local := CheckoutTarget{Ref: "refs/heads/main", OID: gitValue(t, dir, "rev-parse", "HEAD")}
		fixtureGit(t, dir, "checkout", "feature")
		fixtureGit(t, dir, "commit", "--allow-empty", "-m", "Advance upstream")
		upstream.OID = gitValue(t, dir, "rev-parse", "HEAD")
		fixtureGit(t, dir, "checkout", "main")
		result, err := repo.RebaseBranch(t.Context(), upstream, local)
		if !extraEdit {
			if err != nil || result.Kind != RebaseClean || result.OID == local.OID {
				t.Fatalf("a clean merge should permit linear replay: %+v, %v", result, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), "discard changes made in merge commit") {
			t.Fatalf("merge-only changes were not protected: %+v, %v", result, err)
		}
		if head := gitValue(t, dir, "rev-parse", "HEAD"); head != local.OID {
			t.Fatalf("rejected rebase changed the current branch: %s", head)
		}
		if got, err := os.ReadFile(filepath.Join(dir, "merge-only.txt")); string(got) != "only this merge introduces it\n" || err != nil {
			t.Fatalf("merge-only changes did not survive: %q, %v", got, err)
		}
	}
}
