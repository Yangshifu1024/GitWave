package gitcli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkingStatusSeparatesSubmoduleFilesFromItsGitlink(t *testing.T) {
	parent, source := newFixture(t), newFixture(t)
	writeFixture(t, source, ".gitignore", "ignored.txt\n")
	fixtureGit(t, source, "add", ".gitignore")
	fixtureGit(t, source, "commit", "-m", "Ignore a local file")
	fixtureSubmodule(t, parent, source, "dep", "deps/sub repo")
	fixtureGit(t, parent, "commit", "-m", "Add submodule")
	child := filepath.Join(parent, "deps", "sub repo")
	repo := openFixture(t, parent)
	writeFixture(t, child, "ignored.txt", "ignored contents\n")
	if status := readFixtureStatus(t, repo); len(status.Changes) != 0 {
		t.Fatalf("an ignored submodule file made the parent dirty: %+v", status.Changes)
	}
	check := func(index, workTree byte, flags string) (Status, Change) {
		t.Helper()
		status := readFixtureStatus(t, repo)
		if len(status.Changes) != 1 {
			t.Fatalf("parent changes = %+v, want one submodule", status.Changes)
		}
		change := status.Changes[0]
		if change.Path != "deps/sub repo" || change.Index != index || change.WorkTree != workTree || change.Submodule != flags {
			t.Fatalf("unexpected gitlink change: %+v", change)
		}
		return status, change
	}
	writeFixture(t, child, "note.txt", "local note\n")
	for _, flag := range []string{"skip-worktree", "assume-unchanged"} {
		fixtureGit(t, parent, "update-index", "--"+flag, "--", "deps/sub repo")
		if status := readFixtureStatus(t, repo); len(status.Changes) != 0 {
			t.Fatalf("%s gitlink unexpectedly appears in Changes: %+v", flag, status.Changes)
		}
		fixtureGit(t, parent, "update-index", "--no-"+flag, "--", "deps/sub repo")
	}
	status, change := check('.', 'M', "S..U")
	if !change.SubmoduleFilesOnly() || change.CanDiscard() {
		t.Fatalf("invalid write options for internal-only change: %+v", change)
	}
	if n, err := repo.WorkingChangeCount(t.Context()); err != nil || n != 1 {
		t.Fatalf("quit change count = %d, %v", n, err)
	}
	if err := repo.SetStaged(t.Context(), status, []Change{change}, true); err == nil {
		t.Fatal("the parent staged an internal-only change")
	}
	preview, err := repo.WorkingDiff(t.Context(), change, false, false)
	if err != nil || len(preview.Files) != 1 || len(preview.Files[0].Hunks) != 1 {
		t.Fatalf("submodule patch = %+v, %v", preview, err)
	}
	lines := preview.Files[0].Hunks[0].Lines
	if len(lines) != 2 || lines[0].Content != "Subproject commit "+change.IndexOID || lines[1].Content != "Subproject commit "+change.IndexOID+"-dirty" {
		t.Fatalf("unexpected submodule patch: %+v", lines)
	}
	fixtureGit(t, child, "add", "--", "note.txt")
	check('.', 'M', "S.M.")
	fixtureGit(t, child, "-c", "user.name=GitWave Test", "-c", "user.email=test@example.invalid", "commit", "-m", "Child change")
	tip := gitValue(t, child, "rev-parse", "HEAD")
	writeFixture(t, child, "note.txt", "further local edit\n")
	status, change = check('.', 'M', "SCM.")
	if change.SubmoduleFilesOnly() || change.WorkTreeOID != tip {
		t.Fatalf("changed pointer is not available for staging: %+v", change)
	}
	if err := repo.SetStaged(t.Context(), status, []Change{change}, true); err != nil {
		t.Fatal(err)
	}
	status, change = check('M', 'M', "S.M.")
	if change.IndexOID != tip || !change.SubmoduleFilesOnly() {
		t.Fatalf("staging the pointer lost the internal modification: %+v", change)
	}
	if err := repo.SetStaged(t.Context(), status, []Change{change}, false); err != nil {
		t.Fatal(err)
	}
	check('.', 'M', "SCM.")
	if contents, err := os.ReadFile(filepath.Join(child, "note.txt")); err != nil || string(contents) != "further local edit\n" {
		t.Fatalf("the parent changed the submodule file: %q, %v", contents, err)
	}
}

func TestWorkingStatusPropagatesNestedSubmoduleChanges(t *testing.T) {
	parent, middle, leaf := newFixture(t), newFixture(t), newFixture(t)
	fixtureSubmodule(t, middle, leaf, "leaf", "leaf")
	fixtureGit(t, middle, "commit", "-m", "Add leaf")
	fixtureSubmodule(t, parent, middle, "middle", "middle")
	fixtureGit(t, parent, "commit", "-m", "Add middle")
	fixtureGit(t, parent, "-c", "protocol.file.allow=always", "submodule", "update", "--init", "--recursive")
	nested := filepath.Join(parent, "middle", "leaf")
	repo := openFixture(t, parent)
	check := func(flags string) {
		t.Helper()
		status := readFixtureStatus(t, repo)
		if len(status.Changes) != 1 || status.Changes[0].Path != "middle" || status.Changes[0].Submodule != flags {
			t.Fatalf("nested changes = %+v, want %s", status.Changes, flags)
		}
	}
	writeFixture(t, nested, "cow.txt", "untracked\n")
	check("S..U")
	fixtureGit(t, nested, "add", "--", "cow.txt")
	check("S.M.")
	fixtureGit(t, nested, "-c", "user.name=GitWave Test", "-c", "user.email=test@example.invalid", "commit", "-m", "Leaf change")
	writeFixture(t, nested, "new.txt", "another untracked file\n")
	check("S.MU")
}

func TestWorkingStatusKeepsParentChangesWhenSubmoduleIndexIsUnreadable(t *testing.T) {
	parent, source := newFixture(t), newFixture(t)
	fixtureSubmodule(t, parent, source, "dep", "child")
	fixtureGit(t, parent, "commit", "-m", "Add child")
	child := openFixture(t, filepath.Join(parent, "child"))
	writeFixture(t, child.GitDir, "index", "invalid Git index\n")
	writeFixture(t, parent, "note.txt", "local parent file\n")
	repo := openFixture(t, parent)
	status := readFixtureStatus(t, repo)
	if len(status.Changes) != 1 || status.Changes[0].Path != "note.txt" || status.SubmoduleWarning == "" {
		t.Fatalf("expected the parent change and a submodule warning: %+v", status)
	}
	if count, err := repo.WorkingChangeCount(t.Context()); count != 1 || err == nil {
		t.Fatalf("partial quit check = %d, %v", count, err)
	}
	if report, err := repo.Health(t.Context()); err != nil || report.DirtyFiles != 1 || report.SubmoduleWarning == "" {
		t.Fatalf("partial health report = %+v, %v", report, err)
	}
	if err := repo.SetStaged(t.Context(), status, status.Changes, true); err != nil {
		t.Fatal(err)
	}
	if got := gitValue(t, parent, "diff", "--cached", "--name-only"); got != "note.txt" {
		t.Fatalf("staged paths = %q", got)
	}
}
