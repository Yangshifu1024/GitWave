package gitcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeAbortKeepsIndependentChanges(t *testing.T) {
	for _, external := range []bool{false, true} {
		name := "native merge"
		if external {
			name = "external merge"
		}
		t.Run(name, func(t *testing.T) {
			dir, repo, source, target := mergeFixture(t, true, true)
			if external {
				fixtureConflict(t, dir, "merge", "feature")
			} else {
				merge, err := repo.PrepareMerge(t.Context(), source, target)
				if err != nil {
					t.Fatal(err)
				}
				if result, err := merge.Apply(t.Context(), false); err != nil || !result.Conflicted {
					t.Fatalf("expected native conflict: %+v, %v", result, err)
				}
			}
			writeFixture(t, dir, "mixed.txt", "independent staged\n")
			fixtureGit(t, dir, "add", "mixed.txt")
			writeFixture(t, dir, "mixed.txt", "independent unstaged\n")
			writeFixture(t, dir, "conflict.txt", "unsaved resolution\n")
			writeFixture(t, dir, "notes.txt", "independent untracked\n")
			writeAbortExclude(t, dir, "ignored.txt")
			writeFixture(t, dir, "ignored.txt", "independent ignored\n")
			plan, err := repo.PrepareMergeAbort(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if warning, err := plan.Apply(t.Context()); err != nil || warning != nil {
				t.Fatalf("abort: %v, warning: %v", err, warning)
			}
			if head := gitValue(t, dir, "rev-parse", "HEAD"); head != target.OID {
				t.Fatalf("abort moved HEAD: %s", head)
			}
			if original := gitValue(t, dir, "rev-parse", "ORIG_HEAD"); original != target.OID {
				t.Fatalf("abort changed ORIG_HEAD: %s", original)
			}
			if mergeTree, err := repo.exactRootRef(t.Context(), "AUTO_MERGE"); mergeTree != "" || err != nil {
				t.Fatalf("AUTO_MERGE remains: %q, %v", mergeTree, err)
			}
			if current := readFixtureStatus(t, repo); current.Commit != (CommitState{}) {
				t.Fatalf("operation state remains: %+v", current.Commit)
			}
			if got := gitValue(t, dir, "ls-files", "--unmerged"); got != "" {
				t.Fatalf("unmerged index remains: %s", got)
			}
			if got := gitValue(t, dir, "show", ":mixed.txt"); got != "independent staged" {
				t.Fatalf("independent staged content changed: %q", got)
			}
			for name, expected := range map[string]string{
				"conflict.txt": "ours\n", "mixed.txt": "independent unstaged\n",
				"notes.txt": "independent untracked\n", "ignored.txt": "independent ignored\n",
			} {
				if got, err := os.ReadFile(filepath.Join(dir, name)); string(got) != expected || err != nil {
					t.Errorf("%s = %q, want %q: %v", name, got, expected, err)
				}
			}
		})
	}
}

func TestMergeAbortRejectsExpiredWorkingFileOrIndex(t *testing.T) {
	for _, stage := range []bool{false, true} {
		dir, repo, _, _ := mergeFixture(t, true, true)
		fixtureConflict(t, dir, "merge", "feature")
		plan, err := repo.PrepareMergeAbort(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if stage {
			writeFixture(t, dir, "mixed.txt", "new staged work\n")
			fixtureGit(t, dir, "add", "mixed.txt")
		} else {
			writeFixture(t, dir, "conflict.txt", "new conflict edits\n")
		}
		status := gitValue(t, dir, "status", "--porcelain=v2")
		mergeHead := gitValue(t, dir, "rev-parse", "MERGE_HEAD")
		if _, err := plan.Apply(t.Context()); err == nil || !strings.Contains(err.Error(), "changed") {
			t.Fatalf("stale confirmation: %v", err)
		}
		if got := gitValue(t, dir, "status", "--porcelain=v2"); got != status {
			t.Fatalf("stale confirmation changed state: %s", got)
		}
		if got := gitValue(t, dir, "rev-parse", "MERGE_HEAD"); got != mergeHead {
			t.Fatalf("stale confirmation changed MERGE_HEAD: %s", got)
		}
	}
}

func TestMergeAbortKeepsStagedPathWhenExternalStrategyHasNoResultTree(t *testing.T) {
	dir, repo, _, _ := mergeFixture(t, true, false)
	fixtureGit(t, dir, "merge", "--no-commit", "--no-ff", "-s", "ours", "feature")
	writeFixture(t, dir, "from-feature.txt", "my own version\n")
	fixtureGit(t, dir, "add", "from-feature.txt")
	if _, err := repo.PrepareMergeAbort(t.Context()); err == nil || !strings.Contains(err.Error(), "no recorded result tree") {
		t.Fatalf("expected the abort to preserve unattributed staged work: %v", err)
	}
	if data := gitValue(t, dir, "show", ":from-feature.txt"); data != "my own version" {
		t.Fatalf("independent staged work changed: %q", data)
	}
	if current := readFixtureStatus(t, repo); current.Commit.MergeHeads == "" {
		t.Fatal("ambiguous abort cleared the merge")
	}
}

func TestMergeAbortProtectsEditsInAutomaticallyMergedPaths(t *testing.T) {
	for _, option := range []string{"staged", "assume-unchanged", "skip-worktree"} {
		t.Run(option, func(t *testing.T) {
			dir, repo, _, _ := mergeFixture(t, true, true)
			fixtureGit(t, dir, "checkout", "feature")
			writeFixture(t, dir, "incoming.txt", "from feature\n")
			fixtureGit(t, dir, "add", "incoming.txt")
			fixtureGit(t, dir, "commit", "-m", "Add an automatically merged file")
			fixtureGit(t, dir, "checkout", "main")
			fixtureConflict(t, dir, "merge", "feature")
			writeFixture(t, dir, "incoming.txt", "from feature\nindependent edits\n")
			if option == "staged" {
				fixtureGit(t, dir, "add", "incoming.txt")
			} else {
				fixtureGit(t, dir, "update-index", "--"+option, "incoming.txt")
			}
			if _, err := repo.PrepareMergeAbort(t.Context()); err == nil || !strings.Contains(err.Error(), "incoming.txt") {
				t.Fatalf("expected protection for overlapping work: %v", err)
			}
			if got, err := os.ReadFile(filepath.Join(dir, "incoming.txt")); string(got) != "from feature\nindependent edits\n" || err != nil {
				t.Fatalf("overlapping edits changed: %q, %v", got, err)
			}
		})
	}
}

func TestMergeAbortRejectsAnInvalidResultTree(t *testing.T) {
	dir, repo, _, target := mergeFixture(t, true, true)
	fixtureConflict(t, dir, "merge", "feature")
	fixtureGit(t, dir, "update-ref", "AUTO_MERGE", target.OID)
	if _, err := repo.PrepareMergeAbort(t.Context()); err == nil || !strings.Contains(err.Error(), "does not name a Git tree") {
		t.Fatalf("expected invalid merge reference to be rejected: %v", err)
	}
}

func TestMergeAbortKeepsOriginalModifiedDuringPublication(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "conflict.txt", "conflicted\n")
	sourceDir := t.TempDir()
	writeFixture(t, sourceDir, "restored", "pre-merge\n")
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	source, err := os.OpenRoot(sourceDir)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	group, err := inspectAbortGroup(t.Context(), root, "conflict.txt", []string{"conflict.txt"})
	if err != nil {
		t.Fatal(err)
	}
	swap, err := group.prepare(t.Context(), root, source, map[string]string{"conflict.txt": "restored"}, map[string]string{"conflict.txt": "100644"}, false)
	if err != nil {
		t.Fatal(err)
	}
	defer swap.close()
	if err := swap.apply(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	if err := swap.temp.WriteFile("original", []byte("later edits\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := swap.finish(t.Context()); err == nil || !strings.Contains(err.Error(), swap.tempName) {
		t.Fatalf("expected a recovery path: %v", err)
	}
	if got, err := root.ReadFile(filepath.Join(swap.tempName, "original")); string(got) != "later edits\n" || err != nil {
		t.Fatalf("concurrent edit not retained: %q, %v", got, err)
	}
	if got, err := root.ReadFile("conflict.txt"); string(got) != "pre-merge\n" || err != nil {
		t.Fatalf("pre-merge worktree file not installed: %q, %v", got, err)
	}
}

func TestMergeAbortRestoresFileReplacedByDirectory(t *testing.T) {
	dir, repo, _, _ := mergeFixture(t, true, true)
	writeFixture(t, dir, "layout", "main file\n")
	fixtureGit(t, dir, "add", "layout")
	fixtureGit(t, dir, "commit", "-m", "Use a file on main")
	fixtureGit(t, dir, "checkout", "feature")
	if err := os.Mkdir(filepath.Join(dir, "layout"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "layout/child.txt", "incoming child\n")
	fixtureGit(t, dir, "add", "layout/child.txt")
	fixtureGit(t, dir, "commit", "-m", "Use a directory on feature")
	fixtureGit(t, dir, "checkout", "main")
	fixtureConflict(t, dir, "merge", "feature")
	// Resolve the path shape in favor of the incoming directory.
	conflict := "layout~HEAD"
	fixtureGit(t, dir, "rm", "--force", "--", conflict)
	ignored := "layout/ignored.txt"
	writeAbortExclude(t, dir, ignored)
	writeFixture(t, dir, ignored, "keep\n")
	if _, err := repo.PrepareMergeAbort(t.Context()); err == nil || !strings.Contains(err.Error(), ignored) {
		t.Fatalf("expected independent file protection: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, ignored)); err != nil {
		t.Fatal(err)
	}
	plan, err := repo.PrepareMergeAbort(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if warning, err := plan.Apply(t.Context()); err != nil || warning != nil {
		t.Fatalf("abort: %v, warning: %v", err, warning)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "layout")); string(got) != "main file\n" || err != nil {
		t.Fatalf("file/directory transition not restored: %q, %v", got, err)
	}
	if got := gitValue(t, dir, "status", "--porcelain"); got != "" {
		t.Fatalf("remaining changes after abort: %s", got)
	}
}

func writeAbortExclude(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git", "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, ".git/info/exclude", name+"\n")
}
