package gitcli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func interactiveFixture(t *testing.T, names ...string) (string, *Repository, *InteractiveRebasePlan) {
	t.Helper()
	dir, repo, upstream, _ := mergeFixture(t, false, false)
	for _, name := range names {
		writeFixture(t, dir, name+".txt", name+"\n")
		fixtureGit(t, dir, "add", "--", name+".txt")
		fixtureGit(t, dir, "commit", "-m", name)
	}
	local := CheckoutTarget{Ref: "refs/heads/main", OID: gitValue(t, dir, "rev-parse", "HEAD")}
	plan, err := repo.PrepareInteractiveRebase(t.Context(), upstream, local)
	if err != nil {
		t.Fatal(err)
	}
	if got := plan.Todos(); len(got) != len(names) {
		t.Fatalf("got %d todos, want %d: %+v", len(got), len(names), got)
	}
	return dir, repo, plan
}

func TestInteractiveRebaseReordersAndAppliesTodoActions(t *testing.T) {
	dir, _, plan := interactiveFixture(t, "one", "two", "three", "four", "five")
	todos := plan.Todos()
	todos[0], todos[1] = todos[1], todos[0]
	todos[0].Action, todos[0].Message = RebaseReword, "rewritten"
	todos[1].Action = RebaseSquash
	todos[2].Action = RebaseFixup
	todos[3].Action = RebaseDrop
	result, err := plan.Apply(t.Context(), todos)
	if err != nil || result.Paused || len(result.Conflicts) != 0 {
		t.Fatalf("apply todos = %+v, %v", result, err)
	}
	if parent := gitValue(t, dir, "rev-parse", "HEAD~2"); parent != plan.upstream.OID {
		t.Fatalf("wrong rebase base: %s", parent)
	}
	if got := gitValue(t, dir, "show", "-s", "--format=%B", "HEAD~1"); got != "rewritten\n\none" {
		t.Fatalf("wrong combined commit message: %q", got)
	}
	if got := gitValue(t, dir, "show", "-s", "--format=%s", "HEAD"); got != "five" {
		t.Fatalf("last pick = %s", got)
	}
	for _, name := range []string{"one", "two", "three", "five"} {
		if data, err := os.ReadFile(filepath.Join(dir, name+".txt")); string(data) != name+"\n" || err != nil {
			t.Fatalf("%s = %q, %v", name, data, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "four.txt")); !os.IsNotExist(err) {
		t.Fatalf("dropped commit left its file: %v", err)
	}
	if got := gitValue(t, dir, "status", "--porcelain"); got != "" {
		t.Fatalf("dirty rebase result: %s", got)
	}
}

func TestInteractiveRebaseResumesEditAfterRepositoryReopen(t *testing.T) {
	dir, repo, plan := interactiveFixture(t, "edit me", "fix this", "last")
	todos := plan.Todos()
	todos[0].Action, todos[1].Action, todos[2].Action = RebaseEdit, RebaseFixup, RebaseEdit
	first, err := plan.Apply(t.Context(), todos)
	if err != nil || !first.Paused {
		t.Fatalf("first pause = %+v, %v", first, err)
	}
	repo = openFixture(t, dir)
	shown := readFixtureStatus(t, repo)
	pause := shown.Commit.InteractiveRebase
	if !validOID(pause) || shown.HeadOID != first.OID || shown.Commit.Continuation != interactiveRebaseOperation {
		t.Fatalf("reopened pause = %+v", shown)
	}
	if _, err := repo.PrepareCheckout(t.Context(), plan.upstream); err == nil {
		t.Fatal("checkout accepted during an edit pause")
	}
	writeFixture(t, dir, "edit me.txt", "amended content\n")
	if _, err := repo.ContinueInteractiveRebase(t.Context(), pause); err == nil || !strings.Contains(err.Error(), "stage or stash") {
		t.Fatalf("continue accepted unstaged edits: %v", err)
	}
	shown = readFixtureStatus(t, repo)
	if err := repo.SetStaged(t.Context(), shown, shown.Changes, true); err != nil {
		t.Fatal(err)
	}
	second, err := repo.ContinueInteractiveRebase(t.Context(), pause)
	if err != nil || !second.Paused || second.OID == first.OID {
		t.Fatalf("second pause = %+v, %v", second, err)
	}
	if _, err := repo.ContinueInteractiveRebase(t.Context(), pause); err == nil {
		t.Fatal("continued an outdated edit pause")
	}
	shown = readFixtureStatus(t, repo)
	last, err := repo.ContinueInteractiveRebase(t.Context(), shown.Commit.InteractiveRebase)
	if err != nil || last.Paused || last.OID != second.OID {
		t.Fatalf("last continue = %+v, %v", last, err)
	}
	if got := gitValue(t, dir, "show", "HEAD~1:edit me.txt"); got != "amended content" {
		t.Fatalf("edit pause did not fold staged content into the paused commit: %q", got)
	}
	if got := gitValue(t, dir, "show", "HEAD~1:fix this.txt"); got != "fix this" {
		t.Fatalf("fixup after pause did not amend the paused commit: %q", got)
	}
	if state := readFixtureStatus(t, repo); state.Commit != (CommitState{}) || len(state.Changes) != 0 {
		t.Fatalf("finished sequence left state: %+v", state)
	}
	if original, err := repo.exactRootRef(t.Context(), "ORIG_HEAD"); original != plan.local.OID || err != nil {
		t.Fatalf("edit sequence replaced original recovery point: %q, %v", original, err)
	}
}

func TestInteractiveRebaseAbortAndConflictsRestoreOriginalCheckout(t *testing.T) {
	dir, repo, upstream, local := mergeFixture(t, true, true)
	plan, err := repo.PrepareInteractiveRebase(t.Context(), upstream, local)
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.Apply(t.Context(), plan.Todos())
	if err != nil || result.Paused || !slices.Equal(result.Conflicts, []string{"conflict.txt"}) {
		t.Fatalf("conflicted replay = %+v, %v", result, err)
	}
	if status := readFixtureStatus(t, repo); status.HeadOID != local.OID || status.Commit != (CommitState{}) || len(status.Changes) != 0 {
		t.Fatalf("conflict touched the real checkout: %+v", status)
	}
	dir, repo, plan = interactiveFixture(t, "pause", "remaining")
	todos := plan.Todos()
	todos[0].Action = RebaseEdit
	if result, err := plan.Apply(t.Context(), todos); err != nil || !result.Paused {
		t.Fatalf("pause = %+v, %v", result, err)
	}
	shown := readFixtureStatus(t, repo)
	writeFixture(t, dir, "pause.txt", "independent edits\n")
	if err := repo.AbortInteractiveRebase(t.Context(), shown.Commit.InteractiveRebase); err == nil {
		t.Fatal("abort accepted changed edit-pause files")
	}
	if got, err := os.ReadFile(filepath.Join(dir, "pause.txt")); string(got) != "independent edits\n" || err != nil {
		t.Fatalf("abort changed edit-pause files: %q, %v", got, err)
	}
	fixtureGit(t, dir, "checkout", "--", "pause.txt")
	if err := repo.AbortInteractiveRebase(t.Context(), shown.Commit.InteractiveRebase); err != nil {
		t.Fatal(err)
	}
	if state := readFixtureStatus(t, repo); state.HeadOID != plan.local.OID || state.HeadRef != plan.local.Ref ||
		state.Commit != (CommitState{}) || len(state.Changes) != 0 {
		t.Fatalf("abort did not restore original checkout: %+v", state)
	}
}

func TestInteractiveRebaseAmendUpdatesPauseAndBranchTogether(t *testing.T) {
	dir, repo, plan := interactiveFixture(t, "edit", "remaining")
	todos := plan.Todos()
	todos[0].Action = RebaseEdit
	if first, err := plan.Apply(t.Context(), todos); err != nil || !first.Paused {
		t.Fatalf("pause = %+v, %v", first, err)
	}
	current := readFixtureStatus(t, repo)
	oldPause := current.Commit.InteractiveRebase
	amended, err := repo.Commit(t.Context(), current, "amended message", current.HeadOID)
	if err != nil {
		t.Fatal(err)
	}
	current = readFixtureStatus(t, repo)
	if current.Commit.InteractiveRebase == oldPause || current.HeadOID != amended.SHA {
		t.Fatalf("amend did not update the pause together with HEAD: %+v", current)
	}
	if result, err := repo.ContinueInteractiveRebase(t.Context(), current.Commit.InteractiveRebase); err != nil || result.Paused {
		t.Fatalf("continue after amend = %+v, %v", result, err)
	}
	if got := gitValue(t, dir, "show", "-s", "--format=%s", "HEAD~1"); got != "amended message" {
		t.Fatalf("continue lost amended commit message: %q", got)
	}
}

func TestInteractiveRebaseContinuesLegacyPauseAfterUpgrade(t *testing.T) {
	dir, repo := legacyInteractiveFixture(t)
	repo = openFixture(t, dir)
	current := readFixtureStatus(t, repo)
	if !strings.HasPrefix(current.Commit.InteractiveRebase, legacyPausePrefix) {
		t.Fatalf("did not detect the legacy pause: %+v", current.Commit)
	}
	if _, err := os.Stat(filepath.Join(repo.GitDir, legacyInteractiveRebaseFile)); err != nil {
		t.Fatalf("reading status modified the old pause: %v", err)
	}
	if result, err := repo.ContinueInteractiveRebase(t.Context(), current.Commit.InteractiveRebase); err != nil || result.Paused {
		t.Fatalf("continue legacy pause = %+v, %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(repo.GitDir, legacyInteractiveRebaseFile)); !os.IsNotExist(err) {
		t.Fatalf("old pause file remained after migration: %v", err)
	}
	if current := readFixtureStatus(t, repo); current.Commit != (CommitState{}) || len(current.Changes) != 0 {
		t.Fatalf("migration left an operation or dirty checkout: %+v", current)
	}
}

func TestInteractiveRebaseDoesNotAdoptLegacyPauseOnAnotherBranch(t *testing.T) {
	dir, repo := legacyInteractiveFixture(t)
	head := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "checkout", "-b", "other")
	shown := readFixtureStatus(t, repo)
	pause := shown.Commit.InteractiveRebase
	if err := repo.AbortInteractiveRebase(t.Context(), pause); err == nil || !strings.Contains(err.Error(), "original checkout") {
		t.Fatalf("abort used an unowned legacy pause: %v", err)
	}
	if _, err := repo.ContinueInteractiveRebase(t.Context(), pause); err == nil {
		t.Fatal("continue adopted a legacy pause from a different branch")
	}
	if current := readFixtureStatus(t, repo); current.HeadOID != head || current.HeadRef != "refs/heads/other" ||
		current.Commit.InteractiveRebase != pause {
		t.Fatalf("rejected legacy pause changed the checkout: %+v", current)
	}
	fixtureGit(t, dir, "checkout", "main")
	if result, err := repo.ContinueInteractiveRebase(t.Context(), pause); err != nil || result.Paused {
		t.Fatalf("the original branch's pause was not recoverable: %+v, %v", result, err)
	}
	if got := gitValue(t, dir, "rev-parse", "refs/heads/other"); got != head {
		t.Fatalf("continuing the original branch rewrote the separate branch: %s", got)
	}
}

func TestInteractiveRebaseDoesNotRollBackNewCommitsAfterLegacyPause(t *testing.T) {
	dir, repo := legacyInteractiveFixture(t)
	cmd := fixtureCommand(t, dir, "commit", "--allow-empty", "-m", "Independent commit after the pause")
	cmd.Env = append(cmd.Env, "GIT_COMMITTER_DATE="+time.Now().Add(time.Minute).Format(time.RFC3339))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create independent commit: %s, %v", output, err)
	}
	shown := readFixtureStatus(t, repo)
	if err := repo.AbortInteractiveRebase(t.Context(), shown.Commit.InteractiveRebase); err == nil ||
		!strings.Contains(err.Error(), "original checkout") {
		t.Fatalf("abort accepted a legacy pause after additional commits: %v", err)
	}
	if head := gitValue(t, dir, "rev-parse", "HEAD"); head != shown.HeadOID {
		t.Fatalf("abort moved the independent commit: %s", head)
	}
}

func TestDiscardLegacyPauseWithoutOriginalKeepsDirtyCheckout(t *testing.T) {
	dir, repo := legacyInteractiveFixture(t)
	writeFixture(t, repo.GitDir, legacyInteractiveRebaseFile, `{"upstream":"feature","remaining":[]}`)
	writeFixture(t, dir, "edit.txt", "unstaged work\n")
	shown := readFixtureStatus(t, repo)
	if err := repo.AbortInteractiveRebase(t.Context(), shown.Commit.InteractiveRebase); err != nil {
		t.Fatal(err)
	}
	if current := readFixtureStatus(t, repo); current.HeadOID != shown.HeadOID || current.HeadRef != shown.HeadRef ||
		current.Commit != (CommitState{}) || !slices.Equal(current.Changes, shown.Changes) {
		t.Fatalf("discarding the old pause changed the checkout: %+v", current)
	}
}

func legacyInteractiveFixture(t *testing.T) (string, *Repository) {
	t.Helper()
	dir, repo, plan := interactiveFixture(t, "edit", "remaining")
	todos := plan.Todos()
	// libgit2 recorded these reset and commit entries in both reflogs.
	fixtureGit(t, dir, "reset", "--hard", plan.upstream.OID)
	fixtureGit(t, dir, "cherry-pick", "--no-commit", todos[0].OID)
	fixtureGit(t, dir, "commit", "-m", "edit")
	data, err := json.Marshal(map[string]any{"upstream": "feature", "remaining": todos[1:], "original_head": plan.local.OID})
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, repo.GitDir, legacyInteractiveRebaseFile, string(data))
	return dir, repo
}

func TestInteractiveRebaseKeepsHiddenUnstagedBytes(t *testing.T) {
	for _, flag := range []string{"assume-unchanged", "skip-worktree"} {
		t.Run(flag, func(t *testing.T) {
			dir, repo, plan := interactiveFixture(t, "edit", "remaining")
			todos := plan.Todos()
			todos[0].Action = RebaseEdit
			if result, err := plan.Apply(t.Context(), todos); err != nil || !result.Paused {
				t.Fatalf("pause = %+v, %v", result, err)
			}
			writeFixture(t, dir, "edit.txt", "staged bytes\n")
			fixtureGit(t, dir, "add", "edit.txt")
			fixtureGit(t, dir, "update-index", "--"+flag, "edit.txt")
			writeFixture(t, dir, "edit.txt", "hidden unstaged bytes\n")
			shown := readFixtureStatus(t, repo)
			if _, err := repo.ContinueInteractiveRebase(t.Context(), shown.Commit.InteractiveRebase); err == nil ||
				!strings.Contains(err.Error(), "cannot verify unstaged changes") {
				t.Fatalf("continue accepted hidden edits: %v", err)
			}
			if got, err := os.ReadFile(filepath.Join(dir, "edit.txt")); string(got) != "hidden unstaged bytes\n" || err != nil {
				t.Fatalf("hidden worktree edits changed: %q, %v", got, err)
			}
			if current := readFixtureStatus(t, repo); current.HeadOID != shown.HeadOID || current.Commit != shown.Commit {
				t.Fatalf("rejected continue changed the pause: %+v", current)
			}
		})
	}
}

func TestInteractiveRebasePauseIsScopedToLinkedWorktree(t *testing.T) {
	for _, format := range []string{"files", "reftable"} {
		t.Run(format, func(t *testing.T) {
			dir := newFixture(t, "--ref-format="+format)
			main := openFixture(t, dir)
			original := gitValue(t, dir, "rev-parse", "HEAD")
			linked := filepath.Join(t.TempDir(), "linked")
			fixtureGit(t, dir, "worktree", "add", "-b", "topic", linked)
			writeFixture(t, linked, "local.txt", "linked worktree commit\n")
			fixtureGit(t, linked, "add", "local.txt")
			fixtureGit(t, linked, "commit", "-m", "Linked commit")
			local := CheckoutTarget{Ref: "refs/heads/topic", OID: gitValue(t, linked, "rev-parse", "HEAD")}
			upstream := CheckoutTarget{Ref: "refs/heads/main", OID: original}
			repo := openFixture(t, linked)
			plan, err := repo.PrepareInteractiveRebase(t.Context(), upstream, local)
			if err != nil {
				t.Fatal(err)
			}
			todos := plan.Todos()
			todos[0].Action = RebaseEdit
			result, err := plan.Apply(t.Context(), todos)
			if err != nil || !result.Paused {
				t.Fatalf("pause in linked worktree = %+v, %v", result, err)
			}
			if status := readFixtureStatus(t, main); status.HeadOID != original || status.Commit != (CommitState{}) {
				t.Fatalf("linked worktree changes leaked into main: %+v", status)
			}
			state := readFixtureStatus(t, openFixture(t, linked))
			if err := repo.RenameBranch(t.Context(), "topic", "renamed", state.HeadOID); err == nil || !strings.Contains(err.Error(), "paused interactive rebase") {
				t.Fatalf("rename of paused branch should be blocked: %v", err)
			}
			fixtureGit(t, linked, "checkout", "--detach")
			if err := main.RenameBranch(t.Context(), "topic", "renamed", state.HeadOID); err == nil || !strings.Contains(err.Error(), "paused interactive rebase") {
				t.Fatalf("rename of paused branch in another worktree should be blocked: %v", err)
			}
			fixtureGit(t, linked, "checkout", "topic")
			if err := repo.AbortInteractiveRebase(t.Context(), state.Commit.InteractiveRebase); err != nil {
				t.Fatal(err)
			}
			if status := readFixtureStatus(t, repo); status.HeadOID != local.OID || status.Commit != (CommitState{}) {
				t.Fatalf("abort in linked worktree: %+v", status)
			}
		})
	}
}
