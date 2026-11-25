package gitcli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func mergeFixture(t *testing.T, diverged, conflict bool, initOptions ...string) (string, *Repository, CheckoutTarget, CheckoutTarget) {
	t.Helper()
	dir := newFixture(t, initOptions...)
	fixtureGit(t, dir, "checkout", "-b", "feature")
	if conflict {
		writeFixture(t, dir, "conflict.txt", "theirs\n")
	} else {
		writeFixture(t, dir, "from-feature.txt", "theirs\n")
	}
	fixtureGit(t, dir, "add", "--all")
	fixtureGit(t, dir, "commit", "-m", "Feature commit")
	source := CheckoutTarget{Ref: "refs/heads/feature", OID: gitValue(t, dir, "rev-parse", "HEAD")}
	fixtureGit(t, dir, "checkout", "main")
	if diverged {
		writeFixture(t, dir, "conflict.txt", "ours\n")
		fixtureGit(t, dir, "add", "--all")
		fixtureGit(t, dir, "commit", "-m", "Main commit")
	}
	target := CheckoutTarget{Ref: "refs/heads/main", OID: gitValue(t, dir, "rev-parse", "HEAD")}
	return dir, openFixture(t, dir), source, target
}

func TestMergePublishesFastForwardOrTwoParentCommit(t *testing.T) {
	for _, scenario := range []struct {
		label      string
		diverged   bool
		noFF       bool
		fastResult bool
	}{
		{label: "fast forward", fastResult: true},
		{label: "no fast forward", noFF: true},
		{label: "three-way", diverged: true},
	} {
		t.Run(scenario.label, func(t *testing.T) {
			dir, repo, source, target := mergeFixture(t, scenario.diverged, false)
			plan, err := repo.PrepareMerge(t.Context(), source, target)
			if err != nil || plan.UpToDate || plan.FastForward == scenario.diverged || plan.DirtyFiles != 0 || len(plan.Conflicts) != 0 {
				t.Fatalf("unexpected merge preview: %+v, %v", plan, err)
			}
			if got := gitValue(t, dir, "rev-parse", "HEAD"); got != target.OID {
				t.Fatal("preview moved HEAD")
			}
			fixtureGit(t, dir, "update-ref", "ORIG_HEAD", source.OID)
			result, err := plan.Apply(t.Context(), scenario.noFF)
			if err != nil || result.Conflicted || result.FastForward != scenario.fastResult {
				t.Fatalf("unexpected merge result: %+v, %v", result, err)
			}
			if got, branch := gitValue(t, dir, "rev-parse", "HEAD"), gitValue(t, dir, "branch", "--show-current"); got != result.OID || branch != "main" {
				t.Fatalf("unexpected checkout: %s on %s", got, branch)
			}
			if got := gitValue(t, dir, "rev-parse", "ORIG_HEAD"); got != target.OID {
				t.Fatalf("ORIG_HEAD points to %s, expected the pre-merge tip %s", got, target.OID)
			}
			if scenario.fastResult {
				if result.OID != source.OID {
					t.Fatal("fast forward did not reach source commit")
				}
			} else if parents := gitValue(t, dir, "show", "-s", "--format=%P", "HEAD"); parents != target.OID+" "+source.OID {
				t.Fatalf("incorrect merge parents: %s", parents)
			}
			if contents, err := os.ReadFile(filepath.Join(dir, "from-feature.txt")); err != nil || string(contents) != "theirs\n" {
				t.Fatalf("source changes were lost: %q, %v", contents, err)
			}
			if got := gitValue(t, dir, "status", "--porcelain"); got != "" {
				t.Fatalf("wrong index or worktree after merge: %s", got)
			}
		})
	}
}

func TestMergeConflictPublishesGitStateWithoutMovingHEAD(t *testing.T) {
	dir, repo, source, target := mergeFixture(t, true, true)
	plan, err := repo.PrepareMerge(t.Context(), source, target)
	if err != nil || !slices.Equal(plan.Conflicts, []string{"conflict.txt"}) {
		t.Fatalf("wrong preview: %+v, %v", plan, err)
	}
	if got := gitValue(t, dir, "status", "--porcelain"); got != "" {
		t.Fatalf("preview had side effects: %s", got)
	}
	result, err := plan.Apply(t.Context(), false)
	if err != nil || !result.Conflicted || result.OID != target.OID {
		t.Fatalf("merge did not pause correctly: %+v, %v", result, err)
	}
	if got := gitValue(t, dir, "rev-parse", "HEAD"); got != target.OID {
		t.Fatalf("merge advanced HEAD despite conflicts: %s", got)
	}
	if got := gitValue(t, dir, "rev-parse", "MERGE_HEAD"); got != source.OID {
		t.Fatalf("wrong MERGE_HEAD: %s", got)
	}
	if got := gitValue(t, dir, "status", "--porcelain"); got != "UU conflict.txt" {
		t.Fatalf("conflict stages were not installed: %s", got)
	}
	contents, err := os.ReadFile(filepath.Join(dir, "conflict.txt"))
	if err != nil || !strings.Contains(string(contents), "<<<<<<<") || !strings.Contains(string(contents), "ours") || !strings.Contains(string(contents), "theirs") {
		t.Fatalf("conflict markers were not checked out: %q, %v", contents, err)
	}
	fixtureGit(t, dir, "merge", "--abort")
	if got := gitValue(t, dir, "status", "--porcelain"); got != "" {
		t.Fatalf("Git did not recover the pre-merge checkout: %s", got)
	}
}

func TestMergeCommitRemovesResultTreeInReftableRepository(t *testing.T) {
	dir, repo, source, target := mergeFixture(t, true, true, "--ref-format=reftable")
	merge, err := repo.PrepareMerge(t.Context(), source, target)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := merge.Apply(t.Context(), false); err != nil || !result.Conflicted {
		t.Fatalf("expected a reftable merge conflict: %+v, %v", result, err)
	}
	if tree, err := repo.exactRootRef(t.Context(), "AUTO_MERGE"); tree != merge.tree || err != nil {
		t.Fatalf("wrong AUTO_MERGE: %q, %v", tree, err)
	}
	writeFixture(t, dir, "conflict.txt", "resolved\n")
	fixtureGit(t, dir, "add", "conflict.txt")
	status := readFixtureStatus(t, repo)
	if result, err := repo.Commit(t.Context(), status, "Finish merge", ""); err != nil || result.Warning != "" {
		t.Fatalf("finish merge: %+v, %v", result, err)
	}
	if tree, err := repo.exactRootRef(t.Context(), "AUTO_MERGE"); tree != "" || err != nil {
		t.Fatalf("AUTO_MERGE remains: %q, %v", tree, err)
	}
}

func TestMergeRejectsStaleSourceAndProtectsIgnoredFile(t *testing.T) {
	dir, repo, source, target := mergeFixture(t, false, false)
	plan, err := repo.PrepareMerge(t.Context(), source, target)
	if err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "update-ref", source.Ref, target.OID)
	if _, err := plan.Apply(t.Context(), false); err == nil {
		t.Fatal("accepted a moved source ref")
	}
	fixtureGit(t, dir, "update-ref", source.Ref, source.OID)
	put := filepath.Join(dir, "from-feature.txt")
	if err := os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, ".git/info/exclude", "from-feature.txt\n")
	writeFixture(t, dir, "from-feature.txt", "keep this file\n")
	if _, err := plan.Apply(t.Context(), false); err == nil {
		t.Fatal("accepted merge over an ignored file")
	}
	if contents, err := os.ReadFile(put); err != nil || string(contents) != "keep this file\n" {
		t.Fatalf("ignored file changed: %q, %v", contents, err)
	}
	if got := gitValue(t, dir, "rev-parse", "HEAD"); got != target.OID {
		t.Fatalf("HEAD moved on rejected merge: %s", got)
	}
}

func TestMergeConflictPublicationFailureRestoresCheckout(t *testing.T) {
	dir, repo, source, target := mergeFixture(t, true, true)
	index := filepath.Join(dir, ".git", "index")
	before, err := os.ReadFile(index)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := repo.PrepareMerge(t.Context(), source, target)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, ".git/MERGE_MSG", "existing merge message\n")
	if _, err := plan.Apply(t.Context(), false); err == nil || !strings.Contains(err.Error(), "MERGE_MSG") {
		t.Fatalf("conflicting merge metadata did not stop publication: %v", err)
	}
	if actual, err := os.ReadFile(index); err != nil || !bytes.Equal(actual, before) {
		t.Fatalf("rollback changed the live index: %v", err)
	}
	if actual, err := os.ReadFile(filepath.Join(dir, "conflict.txt")); err != nil || string(actual) != "ours\n" {
		t.Fatalf("rollback changed the worktree: %q, %v", actual, err)
	}
	if actual, err := os.ReadFile(filepath.Join(dir, ".git", "MERGE_MSG")); err != nil || string(actual) != "existing merge message\n" {
		t.Fatalf("rollback changed a preexisting merge message: %q, %v", actual, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, ".git", "MERGE_HEAD")); !os.IsNotExist(err) {
		t.Fatalf("failed merge left MERGE_HEAD: %v", err)
	}
}

func TestMergeStagesAConflictResolvedByDeletingTheFile(t *testing.T) {
	dir, repo, source, target := mergeFixture(t, true, true)
	plan, err := repo.PrepareMerge(t.Context(), source, target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "conflict.txt")); err != nil {
		t.Fatal(err)
	}
	state := readFixtureStatus(t, repo)
	if len(state.Changes) != 1 || state.Changes[0].Kind != 'u' || state.Changes[0].WorkTreeMode != "000000" {
		t.Fatalf("missing conflict deletion: %+v", state.Changes)
	}
	if err := repo.SetStaged(t.Context(), state, state.Changes, true); err != nil {
		t.Fatal(err)
	}
	if got := gitValue(t, dir, "status", "--porcelain"); got != "D  conflict.txt" {
		t.Fatalf("unexpected index after conflict resolution: %s", got)
	}
}

func TestMergeMaterializesOnlyConflictsOutsideSparseSelection(t *testing.T) {
	dir := newFixture(t)
	for _, directory := range []string{"included", "excluded"} {
		if err := os.Mkdir(filepath.Join(dir, directory), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFixture(t, dir, directory+"/file.txt", "base\n")
	}
	writeFixture(t, dir, "excluded/clean.txt", "also excluded\n")
	fixtureGit(t, dir, "add", "--all")
	fixtureGit(t, dir, "commit", "-m", "Add sparse paths")
	fixtureGit(t, dir, "checkout", "-b", "feature")
	writeFixture(t, dir, "excluded/file.txt", "theirs\n")
	writeFixture(t, dir, "excluded/clean.txt", "theirs only\n")
	fixtureGit(t, dir, "commit", "-am", "Edit excluded paths on feature")
	source := CheckoutTarget{Ref: "refs/heads/feature", OID: gitValue(t, dir, "rev-parse", "HEAD")}
	fixtureGit(t, dir, "checkout", "main")
	writeFixture(t, dir, "excluded/file.txt", "ours\n")
	fixtureGit(t, dir, "commit", "-am", "Edit the excluded path on main")
	target := CheckoutTarget{Ref: "refs/heads/main", OID: gitValue(t, dir, "rev-parse", "HEAD")}
	fixtureGit(t, dir, "sparse-checkout", "set", "--cone", "included")
	repo := openFixture(t, dir)
	plan, err := repo.PrepareMerge(t.Context(), source, target)
	if err != nil || !slices.Equal(plan.Conflicts, []string{"excluded/file.txt"}) {
		t.Fatalf("wrong sparse preview: %+v, %v", plan, err)
	}
	result, err := plan.Apply(t.Context(), false)
	if err != nil || !result.Conflicted {
		t.Fatalf("sparse merge did not pause: %+v, %v", result, err)
	}
	if output, err := os.ReadFile(filepath.Join(dir, "excluded", "file.txt")); err != nil || !strings.Contains(string(output), "<<<<<<<") {
		t.Fatalf("sparse conflict markers missing: %q, %v", output, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "excluded", "clean.txt")); !os.IsNotExist(err) {
		t.Fatalf("sparse merge materialized an unrelated excluded file: %v", err)
	}
	if got := gitValue(t, dir, "rev-parse", "ORIG_HEAD"); got != target.OID {
		t.Fatalf("sparse merge did not record origin: %s", got)
	}
	writeFixture(t, dir, "excluded/file.txt", "resolved sparse conflict\n")
	status := readFixtureStatus(t, repo)
	var conflicts []Change
	for _, change := range status.Changes {
		if change.Kind == 'u' {
			conflicts = append(conflicts, change)
		}
	}
	if len(conflicts) != 1 || conflicts[0].WorkTreeMode != "100644" {
		t.Fatalf("missing materialized conflict: %+v", conflicts)
	}
	if err := repo.SetStaged(t.Context(), status, conflicts, true); err != nil {
		t.Fatal(err)
	}
	abort, err := repo.PrepareMergeAbort(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if warning, err := abort.Apply(t.Context()); err != nil || warning != nil {
		t.Fatalf("sparse abort: %v, warning: %v", err, warning)
	}
	if got := gitValue(t, dir, "ls-files", "-t", "--", "excluded/file.txt"); got != "S excluded/file.txt" {
		t.Fatalf("sparse abort did not restore the skip-worktree bit: %s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "excluded", "file.txt")); !os.IsNotExist(err) {
		t.Fatalf("sparse abort left the excluded file materialized: %v", err)
	}
	if got := gitValue(t, dir, "status", "--porcelain"); got != "" {
		t.Fatalf("sparse abort did not recover the checkout: %s", got)
	}
}

func TestMergeIntoDetachedHEADWithoutMovingABranchNamedORIG_HEAD(t *testing.T) {
	dir, repo, source, target := mergeFixture(t, false, false)
	fixtureGit(t, dir, "branch", "ORIG_HEAD", source.OID)
	fixtureGit(t, dir, "checkout", "--detach", target.OID)
	target.Ref = ""
	plan, err := repo.PrepareMerge(t.Context(), source, target)
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.Apply(t.Context(), false)
	if err != nil || !result.FastForward || result.OID != source.OID {
		t.Fatalf("wrong detached merge: %+v, %v", result, err)
	}
	if got := gitValue(t, dir, "branch", "--show-current"); got != "" {
		t.Fatalf("detached merge unexpectedly attached to %s", got)
	}
	if got := gitValue(t, dir, "show-ref", "--verify", "--hash", "ORIG_HEAD"); got != target.OID {
		t.Fatalf("wrong recovery pointer: %s", got)
	}
	if got := gitValue(t, dir, "rev-parse", "refs/heads/ORIG_HEAD"); got != source.OID {
		t.Fatalf("merge changed the branch named ORIG_HEAD: %s", got)
	}
}
