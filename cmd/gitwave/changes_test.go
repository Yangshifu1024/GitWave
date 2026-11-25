package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestStageAndUnstageMixedWorkingChanges(t *testing.T) {
	repo, git := historyRepository(t)
	writeRepositoryFixtureFile(t, repo, "mixed.txt", "old\n")
	writeRepositoryFixtureFile(t, repo, "removed.txt", "old content\n")
	git("", "add", ".")
	git("", "commit", "-m", "Base")
	writeRepositoryFixtureFile(t, repo, "mixed.txt", "staged\n")
	git("", "add", "mixed.txt")
	writeRepositoryFixtureFile(t, repo, "mixed.txt", "worktree\n")
	writeRepositoryFixtureFile(t, repo, "new.txt", "new file\n")
	if err := os.Remove(filepath.Join(repo, "removed.txt")); err != nil {
		t.Fatal(err)
	}

	b, err := startBackend(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	indexEntry := func(path, content string) string {
		return "100644 " + git(content, "hash-object", "--stdin") + " 0\t" + path + "\x00"
	}
	for _, step := range []struct {
		stage bool
		paths []string
		index string
	}{
		{true, []string{"mixed.txt", "new.txt", "removed.txt"}, indexEntry("mixed.txt", "worktree\n") + indexEntry("new.txt", "new file\n")},
		{false, []string{"mixed.txt", "new.txt"}, indexEntry("mixed.txt", "old\n")},
	} {
		status, err := b.repo.Status(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		var changes []gitcli.Change
		for _, change := range status.Changes {
			if slices.Contains(step.paths, change.Path) {
				changes = append(changes, change)
			}
		}
		if err := b.repo.SetStaged(t.Context(), status, changes, step.stage); err != nil {
			t.Fatal(err)
		}
		if got := git("", "ls-files", "--stage", "-z"); got != step.index {
			t.Fatalf("stage=%v: Git index = %q, want %q", step.stage, got, step.index)
		}
	}
}

func TestCommitAndAmendPreserveUnstagedChanges(t *testing.T) {
	useFixtureCommitIdentity(t)
	t.Setenv("GIT_AUTHOR_DATE", "2024-01-01T12:00:00+08:00")
	t.Setenv("GIT_COMMITTER_DATE", "2024-01-01T12:00:00+08:00")
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Base")
	parent := git("", "rev-parse", "HEAD")
	writeRepositoryFixtureFile(t, repo, "tracked.txt", "staged\n")
	git("", "add", "tracked.txt")
	tree := git("", "write-tree")
	index := git("", "ls-files", "--stage")
	writeRepositoryFixtureFile(t, repo, "tracked.txt", "working\n")
	statusBefore := git("", "diff", "--name-status")
	b, err := startBackend(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		message string
		amend   bool
	}{
		{message: "feat: staged change\n\nKeep a separate worktree edit"},
		{message: "fix: updated subject\n\nReword the body", amend: true},
	} {
		status, err := b.repo.Status(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		target := ""
		if test.amend {
			target = status.HeadOID
		}
		result, err := b.repo.Commit(t.Context(), status, test.message, target)
		if err != nil || result.SHA != git("", "rev-parse", "HEAD") {
			t.Fatalf("commit: %+v, %v", result, err)
		}
		want := strings.Join([]string{tree, parent, "GitWave", "test@example.invalid", test.message}, "\n")
		if got := git("", "show", "-s", "--format=%T%n%P%n%an%n%ae%n%B", "HEAD"); got != want {
			t.Fatalf("amend=%v: Git commit = %q, want %q", test.amend, got, want)
		}
		if git("", "ls-files", "--stage") != index || git("", "diff", "--name-status") != statusBefore {
			t.Fatalf("amend=%v changed the index or worktree status", test.amend)
		}
		if got, err := os.ReadFile(filepath.Join(repo, "tracked.txt")); err != nil || string(got) != "working\n" {
			t.Fatalf("worktree contents = %q, %v", got, err)
		}
	}
}

func TestDiscardSelectedChangesPreservesIndexAndUnselectedFiles(t *testing.T) {
	repo, git := historyRepository(t)
	writeRepositoryFixtureFile(t, repo, "mixed.txt", "base\n")
	writeRepositoryFixtureFile(t, repo, "deleted.txt", "restore\n")
	git("", "add", ".")
	git("", "commit", "-m", "Base")
	writeRepositoryFixtureFile(t, repo, "mixed.txt", "staged\n")
	git("", "add", "mixed.txt")
	index := git("", "ls-files", "--stage")
	writeRepositoryFixtureFile(t, repo, "mixed.txt", "working\n")
	writeRepositoryFixtureFile(t, repo, "new.txt", "discard\n")
	writeRepositoryFixtureFile(t, repo, "keep.txt", "keep\n")
	if err := os.Remove(filepath.Join(repo, "deleted.txt")); err != nil {
		t.Fatal(err)
	}

	b, err := startBackend(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	shown, err := b.repo.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	requested := []string{"mixed.txt", "new.txt", "deleted.txt"}
	var selected []gitcli.Change
	for _, c := range shown.Changes {
		if slices.Contains(requested, c.Path) {
			selected = append(selected, c)
		}
	}
	plan, err := b.repo.PrepareDiscard(t.Context(), shown, selected)
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}

	for name, want := range map[string]string{"mixed.txt": "staged\n", "deleted.txt": "restore\n", "keep.txt": "keep\n"} {
		got, err := os.ReadFile(filepath.Join(repo, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s = %q, %v, want %q", name, got, err, want)
		}
	}
	if _, err := os.Lstat(filepath.Join(repo, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("selected untracked file remains: %v", err)
	}
	if got := git("", "status", "--porcelain"); got != "M  mixed.txt\n?? keep.txt" {
		t.Fatalf("Git status = %q", got)
	}
	if git("", "ls-files", "--stage") != index {
		t.Fatal("discard changed the staged snapshot")
	}
}

func crlfTailHunk(before, after string) diffHunk {
	return diffHunk{OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 3, Lines: []diffLine{
		{Kind: "context", Content: "first", Old: 1, New: 1},
		{Kind: "context", Old: 2, New: 2},
		{Kind: "removed", Content: before, Old: 3},
		{Kind: "added", Content: after, New: 3},
	}}
}

func TestWorkingDiffKeepsLineNumbersAcrossIndexAndWorktree(t *testing.T) {
	repo, git := historyRepository(t)
	writeRepositoryFixtureFile(t, repo, "mixed.txt", "first\r\n\r\nbase tail")
	writeRepositoryFixtureFile(t, repo, "deleted.txt", "removed\n")
	git("", "add", ".")
	git("", "commit", "-m", "Base")
	writeRepositoryFixtureFile(t, repo, "mixed.txt", "first\r\n\r\nstaged tail")
	git("", "add", "mixed.txt")
	writeRepositoryFixtureFile(t, repo, "mixed.txt", "first\r\n\r\nworking tail")
	writeRepositoryFixtureFile(t, repo, "untracked.txt", "first\r\n\r\nnew tail")
	if err := os.Remove(filepath.Join(repo, "deleted.txt")); err != nil {
		t.Fatal(err)
	}

	b, err := startBackend(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	status, err := b.repo.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	grouped := groupChanges(status)
	for _, test := range []struct {
		name string
		key  changeKey
		add  int
		del  int
		hunk diffHunk
	}{
		{"staged", changeKey{"mixed.txt", true}, 1, 1, crlfTailHunk("base tail", "staged tail")},
		{"unstaged", changeKey{path: "mixed.txt"}, 1, 1, crlfTailHunk("staged tail", "working tail")},
		{"deleted", changeKey{path: "deleted.txt"}, 0, 1, diffHunk{OldStart: 1, OldLines: 1, Lines: []diffLine{
			{Kind: "removed", Content: "removed", Old: 1},
		}}},
		{"untracked", changeKey{path: "untracked.txt"}, 3, 0, diffHunk{NewStart: 1, NewLines: 3, Lines: []diffLine{
			{Kind: "added", Content: "first", New: 1}, {Kind: "added", New: 2}, {Kind: "added", Content: "new tail", New: 3},
		}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			item := grouped.find(test.key)
			if item.key != test.key {
				t.Fatalf("missing change: %+v", test.key)
			}
			got, err := b.repo.WorkingDiff(t.Context(), item.change, item.key.staged, false)
			if err != nil || got.Truncated || got.TooLarge || len(got.Files) != 1 {
				t.Fatalf("unexpected working diff: %+v, %v", got, err)
			}
			file := got.Files[0]
			if file.Path != test.key.path || file.Additions != test.add || file.Deletions != test.del ||
				!reflect.DeepEqual(file.Hunks, []diffHunk{test.hunk}) {
				t.Fatalf("unexpected file diff: %+v", file)
			}
		})
	}
}

func TestNativeChangesLargeListKeepsControlsReachable(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Start working copy")
	const count = 495
	for i := range count {
		path := filepath.Join(repo, fmt.Sprintf("change-%03d.txt", i))
		content := "Working copy content\n"
		if i == count-1 {
			content += strings.Repeat("Additional preview content\n", 6_000)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app := refTestApp(t, repo)
	const timeout = 8 * time.Second
	const list = "id:changes-unstaged-list"
	app.WaitFor("id:history-commit-list", timeout)
	app.Press("id:navigate-changes")
	app.WaitFor("text:Unstaged (495)", timeout)
	assertLayout := func() {
		t.Helper()
		paneHeight := app.Find("id:split-changes-files").Frame.Size.Height
		for _, id := range []string{list, "id:changes-staged-list"} {
			frame := app.Find(id).Frame
			if frame.Origin.Y < 31 || frame.Size.Height <= 0 || frame.Size.Height >= paneHeight {
				t.Fatalf("%s has unbounded content or its header is hidden: %+v, pane height %v", id, frame, paneHeight)
			}
		}
		composer := app.Find("id:commit-composer").Frame
		if composer.Origin.Y < 0 || composer.Origin.Y+composer.Size.Height > paneHeight+1 {
			t.Fatalf("composer is outside the changes pane: %+v, pane height %v", composer, paneHeight)
		}
		if mounted := len(app.QueryMounted(list + " >> role:option")); mounted == 0 || mounted > 64 {
			t.Fatalf("expected a bounded window into %d changes, mounted %d rows", count, mounted)
		}
		app.WaitUntil(timeout, func() bool {
			heading := app.Find("id:changes-unstaged-heading").Bounds()
			more := app.Find("id:unstaged-more-actions").Bounds()
			endInset := heading.Origin.X + heading.Size.Width - more.Origin.X - more.Size.Width
			return endInset >= 0 && endInset <= 12
		})
		message := app.Find("id:commit-message").Bounds()
		generate := app.Find("id:commit-ai-generate").Bounds()
		if generate.Origin.Y < message.Origin.Y+message.Size.Height {
			t.Fatalf("Generate is not below the commit message: message %+v, button %+v", message, generate)
		}
	}
	assertLayout()
	app.Resize(1024, 768)
	app.WaitFor("id:titlebar-changes >> text:(495)", timeout)
	app.WaitFor("id:push >> text:Push", timeout)
	assertLayout()
	app.Focus("label:Unstaged · change-000.txt")
	app.Key("end")
	last := "label:Unstaged · change-494.txt"
	app.WaitFor(last, timeout)
	assertLayout()
	app.Press("id:unstaged-more-actions")
	app.ChoosePopup("Discard")
	app.WaitFor("id:discard-confirmation", timeout)
	app.Press("label:Cancel")
	app.WaitForGone("id:discard-confirmation", timeout)
	app.Press(last)
	app.WaitFor("id:diff-line-list >> text:Working copy content", timeout)
	app.WaitFor("text:Preview truncated to keep scrolling responsive.", timeout)
	app.Type("id:commit-message", "Keep the message editor reachable")
	if got := app.Find("id:commit-message").Text; got != "Keep the message editor reachable" {
		t.Fatalf("the commit message editor did not receive input: %q", got)
	}
}

func TestNativeChangesPreviewsBothSidesAndFollowsRefresh(t *testing.T) {
	repo, git := historyRepository(t)
	put := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, path), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	put("both.txt", "base line\n")
	git("", "add", ".")
	git("", "commit", "-m", "Base version")
	put("both.txt", "stage line\n")
	git("", "add", ".")
	put("both.txt", "working line\n")
	put("new.txt", "untracked line\n")
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.open(repo)
		return w.view()
	}, nsgotest.Size(1200, 800), nsgotest.Environment(core.Environment{
		OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en", ReduceMotion: true,
	}))
	const timeout = 5 * time.Second
	app.WaitFor("label:/Base version/", timeout)
	app.Press("id:navigate-changes")
	app.WaitFor("text:Unstaged (2)", timeout)
	app.WaitFor("text:Staged (1)", timeout)
	app.WaitFor("text:/^No file selected/", timeout)
	app.Press("label:Unstaged · both.txt")
	app.WaitFor("text:/^working line$/", timeout)
	app.Press("label:Staged · both.txt")
	app.WaitFor("text:/^base line$/", timeout)
	app.WaitForGone("text:/^working line$/", timeout)
	app.Press("id:navigate-history")
	app.WaitFor("label:/Base version/", timeout)
	app.Press("id:navigate-changes")
	app.WaitFor("text:/^base line$/", timeout)
	git("", "restore", "--staged", "--", "both.txt")
	app.Key("mod+r")
	app.WaitFor("text:Staged (0)", timeout)
	app.WaitFor("text:/^working line$/", timeout)
	app.Press("label:Unstaged · new.txt")
	app.WaitFor("text:/^untracked line$/", timeout)
}

func TestNativeStageAndUnstageFromFileRowsAndSections(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Base version")
	for _, name := range []string{"a.txt", "b.txt", "c.txt", "d.txt"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte("contents of "+name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.open(repo)
		return w.view()
	}, nsgotest.Size(1200, 800), nsgotest.Environment(core.Environment{
		OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en", ReduceMotion: true,
	}))
	const timeout = 5 * time.Second
	app.Press("id:navigate-changes")
	app.WaitFor("text:Unstaged (4)", timeout)
	app.Press("label:Unstaged · a.txt")
	app.WaitFor("text:/^contents of a.txt$/", timeout)
	app.Press("label:Stage d.txt")
	app.WaitFor("text:Staged (1)", timeout)
	app.WaitFor("text:/^contents of a.txt$/", timeout)
	app.PressWith("label:Unstaged · b.txt", core.Modifiers{Shift: true})
	app.WaitFor("label:Stage", timeout)
	app.Press("label:Stage")
	app.WaitFor("text:Staged (3)", timeout)
	app.WaitFor("text:Unstaged (1)", timeout)
	if got := git("", "diff", "--cached", "--name-only"); got != "a.txt\nb.txt\nd.txt" {
		t.Fatalf("batch selection staged %q", got)
	}
	app.Press("label:Unstage a.txt")
	app.WaitFor("text:Staged (2)", timeout)
	app.Press("label:Stage All")
	app.WaitFor("text:Staged (4)", timeout)
	app.Press("label:Unstage All")
	app.WaitFor("text:Unstaged (4)", timeout)
	app.WaitFor("text:Staged (0)", timeout)
	if got := git("", "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("Unstage All left changes staged: %q", got)
	}
	app.Press("label:Unstaged · a.txt")
	app.Key("down")
	app.WaitFor("text:/^contents of b.txt$/", timeout)
	app.Key("mod+a")
	app.Press("label:Stage")
	app.WaitFor("text:Staged (4)", timeout)
}

func TestNativeChangesShowsDirtySubmoduleAndStagesOnlyParentFiles(t *testing.T) {
	repo, git := historyRepository(t)
	source, gitSource := historyRepository(t)
	gitSource("", "commit", "--allow-empty", "-m", "Submodule base")
	git("", "-c", "protocol.file.allow=always", "submodule", "add", "--", source, "child")
	git("", "commit", "-m", "Add child")
	writeRepositoryFixtureFile(t, filepath.Join(repo, "child"), "note.txt", "Local submodule file\n")
	writeRepositoryFixtureFile(t, repo, "parent.txt", "Local parent file\n")
	app, _ := refreshApp(t, nil, repo)
	const timeout = 10 * time.Second
	app.Press("id:navigate-changes")
	app.WaitFor("text:Unstaged (2)", timeout)
	app.Press("label:Unstaged · child")
	app.WaitFor("text:/Subproject commit [0-9a-f]+-dirty/", timeout)
	if !app.Find("label:Stage child").Props.Disabled || !app.Find("label:Stage").Props.Disabled {
		t.Fatal("the parent offers to stage a file inside its submodule")
	}
	app.Press("label:Stage All")
	app.WaitFor("text:Staged (1)", timeout)
	app.WaitFor("text:Unstaged (1)", timeout)
	app.Find("label:Unstaged · child")
	if !app.Find("label:Stage All").Props.Disabled || git("", "diff", "--cached", "--name-only") != "parent.txt" {
		t.Fatal("Stage All did not leave the submodule's internal change unstaged")
	}
}

func TestNativeFileMoveStagesAndUnstagesBothPaths(t *testing.T) {
	repo, git := historyRepository(t)
	writeRepositoryFixtureFile(t, repo, "old.txt", "Move this file\n")
	git("", "add", "old.txt")
	git("", "commit", "-m", "Original file")
	if err := os.Rename(filepath.Join(repo, "old.txt"), filepath.Join(repo, "new.txt")); err != nil {
		t.Fatal(err)
	}
	app := refTestApp(t, repo)
	app.WaitFor("id:history-commit-list", refTestTimeout)
	app.Press("id:navigate-changes")
	app.WaitFor("text:Unstaged (1)", refTestTimeout)
	app.Press("label:Unstaged · old.txt → new.txt")
	app.WaitFor("text:No text preview", refTestTimeout)
	app.Press("label:Stage new.txt")
	app.WaitFor("label:Staged · new.txt", refTestTimeout)
	if got := git("", "diff", "--cached", "--name-status", "--find-renames"); got != "R100\told.txt\tnew.txt" {
		t.Fatalf("staging both sides of the move left this index diff: %q", got)
	}
	app.Press("label:Unstage new.txt")
	app.WaitFor("label:Unstaged · old.txt → new.txt", refTestTimeout)
	if got := git("", "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("unstaging the rename left these paths staged: %q", got)
	}
	if got, err := os.ReadFile(filepath.Join(repo, "new.txt")); err != nil || string(got) != "Move this file\n" {
		t.Fatalf("working file changed: %q, %v", got, err)
	}
}

func TestNativeEditedMoveStaysSelectedDuringLargeRefresh(t *testing.T) {
	repo, git := historyRepository(t)
	const source, destination = "original.txt", "zz moved [file].txt"
	var original strings.Builder
	for i := range 40 {
		fmt.Fprintf(&original, "Working file line number %d\n", i)
	}
	writeRepositoryFixtureFile(t, repo, source, original.String())
	git("", "add", "--", source)
	git("", "commit", "-m", "Original file")
	if err := os.Remove(filepath.Join(repo, source)); err != nil {
		t.Fatal(err)
	}
	modified := strings.Replace(original.String(), "Working file line number 12", "Edited after moving", 1)
	writeRepositoryFixtureFile(t, repo, destination, modified)
	for i := range 494 {
		writeRepositoryFixtureFile(t, repo, fmt.Sprintf("change-%03d.txt", i), "Unrelated untracked file\n")
	}
	app := refTestApp(t, repo)
	const timeout = 12 * time.Second
	const row = "label:Unstaged · original.txt → zz moved [file].txt"
	const changedLine = "id:diff-line-list >> text:Edited after moving"
	app.WaitFor("id:history-commit-list", timeout)
	app.Press("id:navigate-changes")
	app.WaitFor("text:Unstaged (495)", timeout)
	app.Focus("label:Unstaged · change-000.txt")
	app.Key("end")
	app.WaitFor(row, timeout)
	app.WaitFor(changedLine, timeout)
	writeRepositoryFixtureFile(t, repo, "a-external.txt", "Unrelated external file\n")
	app.Key("mod+r")
	app.WaitFor("text:Unstaged (496)", timeout)
	app.WaitFor(row, timeout)
	app.WaitFor(changedLine, timeout)
	app.Press("label:Stage zz moved [file].txt")
	app.WaitFor("label:Staged · zz moved [file].txt", timeout)
	if got := git("", "diff", "--cached", "--name-status", "--find-renames"); !strings.HasPrefix(got, "R") || !strings.HasSuffix(got, "\t"+source+"\t"+destination) || strings.Contains(got, "\n") {
		t.Fatalf("staging the edited move affected unexpected paths: %q", got)
	}
}

func TestNativeStagedTextMoveKeepsSeparateWorktreeChanges(t *testing.T) {
	repo, git := historyRepository(t)
	const source, destination = "old/shared.txt", "new/shared.txt"
	for _, folder := range []string{"old", "new"} {
		if err := os.Mkdir(filepath.Join(repo, folder), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	var original, edited strings.Builder
	for n := range 12 {
		line := fmt.Sprintf("Distinct shared text line %d", n)
		fmt.Fprintln(&original, "\t"+line)
		fmt.Fprintln(&edited, "    "+line)
	}
	edited.WriteString("Staged version only\n")
	writeRepositoryFixtureFile(t, repo, source, original.String())
	git("", "add", "--", source)
	git("", "commit", "-m", "Original text")
	if err := os.Remove(filepath.Join(repo, source)); err != nil {
		t.Fatal(err)
	}
	writeRepositoryFixtureFile(t, repo, destination, edited.String())

	app := refTestApp(t, repo)
	const timeout = 10 * time.Second
	const pair = source + " → " + destination
	app.WaitFor("id:history-commit-list", timeout)
	app.Press("id:navigate-changes")
	app.WaitFor("label:Unstaged · "+pair, timeout)
	app.Press("label:Stage " + destination)
	app.WaitFor("text:Staged (1)", timeout)
	app.WaitFor("label:Staged · "+pair, timeout)

	writeRepositoryFixtureFile(t, repo, source, "Recreated source\n")
	writeRepositoryFixtureFile(t, repo, destination, "Worktree edit only\n")
	app.Key("mod+r")
	app.WaitFor("label:Unstaged · "+source, timeout)
	app.WaitFor("label:Unstaged · "+destination, timeout)
	app.Press("label:Staged · " + pair)
	app.WaitFor("id:diff-line-list >> text:Staged version only", timeout)
	app.WaitForGone("id:diff-line-list >> text:Worktree edit only", timeout)
	app.Press("label:Unstaged · " + destination)
	app.WaitFor("id:diff-line-list >> text:Worktree edit only", timeout)
	if err := os.Remove(filepath.Join(repo, destination)); err != nil {
		t.Fatal(err)
	}
	app.Key("mod+r")
	app.WaitForGone("id:diff-line-list >> text:Worktree edit only", timeout)
	app.Press("label:Unstage " + destination)
	app.WaitFor("text:Staged (0)", timeout)
	if got := git("", "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("a staged path remained after unstaging the paired row: %q", got)
	}
	if data, err := os.ReadFile(filepath.Join(repo, source)); err != nil || string(data) != "Recreated source\n" {
		t.Fatalf("unstage changed the recreated source: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(repo, destination)); !os.IsNotExist(err) {
		t.Fatalf("unstage restored the removed destination: %v", err)
	}
}

func TestNativeCommitComposerFitsLargeTextInNarrowFilesPane(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Original message")
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		_ = cx.Storage().SetItem("gitwave.split.navigator", "32")
		_ = cx.Storage().SetItem("gitwave.split.changes-files", "28")
		w := newWorkbench(cx)
		preferences := w.desktopPrefs.Get()
		preferences.Fonts.UISize = "20"
		w.desktopPrefs.Set(preferences)
		w.open(repo)
		return w.view()
	}, nsgotest.Size(1024, 768), nsgotest.Environment(core.Environment{
		OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en", ReduceMotion: true,
	}))
	app.WaitFor("id:history-commit-list", refTestTimeout)
	app.Press("id:navigate-changes")
	app.WaitFor("id:open-amend", refTestTimeout)
	app.Press("id:open-amend")
	app.WaitFor("id:confirm-amend", refTestTimeout)
	app.WaitUntil(refTestTimeout, func() bool { return app.Enabled("id:confirm-amend") })
	app.Press("id:confirm-amend")
	const restore = "label:Restore previous draft"
	app.WaitFor(restore, refTestTimeout)
	composer := app.Find("id:commit-composer").Bounds()
	for _, selector := range []string{"id:commit-type", "id:commit-message", restore,
		"id:open-amend", "id:commit-ai-generate", "id:write-commit"} {
		control := app.Find(selector).Bounds()
		if control.Size.Width <= 0 || control.Origin.X < composer.Origin.X ||
			control.Origin.X+control.Size.Width > composer.Origin.X+composer.Size.Width+1 {
			t.Errorf("%s falls outside composer: control %+v, composer %+v", selector, control, composer)
		}
	}
}

func TestNativeCommitAndAmendFromChanges(t *testing.T) {
	repo, git := historyRepository(t)
	put := func(content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	put("base\n")
	git("", "add", "tracked.txt")
	git("", "commit", "-m", "Base version")
	base := git("", "rev-parse", "HEAD")
	put("staged\n")
	git("", "add", "tracked.txt")
	put("working\n")
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.open(repo)
		return w.view()
	}, nsgotest.Size(1200, 800), nsgotest.Environment(core.Environment{
		OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en", ReduceMotion: true,
	}))
	const timeout = 5 * time.Second
	const message = "id:commit-message"
	waitMessage := func(want string) {
		t.Helper()
		app.WaitUntil(timeout, func() bool { return app.Find(message).Text == want })
	}
	openAmend := func() {
		t.Helper()
		app.Press("id:open-amend")
		app.WaitFor("id:amend-confirmation", timeout)
		app.WaitUntil(timeout, func() bool { return app.Enabled("id:confirm-amend") })
	}
	app.Press("id:navigate-changes")
	app.WaitFor(message, timeout)
	if app.Enabled("id:write-commit") {
		t.Fatal("empty commit message enabled submission")
	}
	app.Type(message, "feat(core): staged content")
	app.Press("id:commit-type")
	app.WaitFor("text:fix", timeout)
	app.Press("text:fix")
	waitMessage("fix: staged content")
	app.Press("id:navigate-history")
	app.Press("id:navigate-changes")
	waitMessage("fix: staged content")
	app.Press("id:write-commit")
	app.WaitFor("id:commit-success", timeout)
	waitMessage("")
	app.WaitFor("text:Unstaged (1)", timeout)
	app.WaitFor("text:Staged (0)", timeout)
	if got := git("", "show", "HEAD:tracked.txt"); got != "staged" {
		t.Fatalf("commit included working tree content: %q", got)
	}
	first := git("", "rev-parse", "HEAD")
	app.Press("label:View in History")
	app.WaitFor("id:commit-header", timeout)
	app.WaitFor("label:/fix: staged content/", timeout)
	app.Press("id:navigate-changes")
	app.WaitFor(message, timeout)
	app.Type(message, "work in progress")
	openAmend()
	app.Press("label:Cancel")
	app.WaitForGone("id:amend-confirmation", timeout)
	waitMessage("work in progress")
	openAmend()
	app.Press("id:confirm-amend")
	app.WaitForGone("id:amend-confirmation", timeout)
	waitMessage("fix: staged content")
	git("", "switch", "-c", "another-branch")
	app.Key("mod+r")
	app.WaitFor("text:/^The confirmed HEAD changed/", timeout)
	if app.Enabled("id:write-commit") {
		t.Fatal("an amend confirmation carried to a different branch with the same tip")
	}
	app.Press("label:Restore previous draft")
	waitMessage("work in progress")
	openAmend()
	app.Press("id:confirm-amend")
	app.WaitForGone("id:amend-confirmation", timeout)
	app.Clear(message)
	app.Type(message, "fix: corrected subject")
	app.Key("enter")
	app.Type(message, "Details")
	waitMessage("fix: corrected subject\nDetails")
	app.Key("mod+enter")
	app.WaitUntil(timeout, func() bool {
		return app.Exists("id:commit-success") && app.Find("id:commit-success").Text != "Committed "+shortSHA(first)
	})
	waitMessage("")
	if got := git("", "log", "-1", "--format=%B"); got != "fix: corrected subject\nDetails" {
		t.Fatalf("amend message: %q", got)
	}
	if got := git("", "rev-parse", "HEAD^"); got != base {
		t.Fatalf("amend created a new child of %s: parent is %s", first, got)
	}
	if got := git("", "rev-parse", "main"); got != first {
		t.Fatalf("amending the confirmed branch moved main to %s", got)
	}
}

func TestNativeDiscardFromFileMenuAndUnstagedActions(t *testing.T) {
	repo, git := historyRepository(t)
	put := func(path, data string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, path), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	put("mixed.txt", "base\n")
	git("", "add", "mixed.txt")
	git("", "commit", "-m", "Base version")
	put("mixed.txt", "staged\n")
	git("", "add", "mixed.txt")
	put("mixed.txt", "working\n")
	put("a.txt", "first untracked\n")
	put("z.txt", "second untracked\n")
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.open(repo)
		return w.view()
	}, nsgotest.Size(1200, 800), nsgotest.Environment(core.Environment{
		OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en", ReduceMotion: true,
	}))
	const timeout = 5 * time.Second
	const dialog, confirm = "id:discard-confirmation", "id:confirm-discard"
	waitConfirmation := func() {
		t.Helper()
		app.WaitFor(dialog, timeout)
		app.WaitUntil(timeout, func() bool { return app.Enabled(confirm) })
	}
	app.Press("id:navigate-changes")
	app.WaitFor("text:Unstaged (3)", timeout)
	app.RightClick("label:Unstaged · a.txt")
	app.ChoosePopup("Discard changes…")
	waitConfirmation()
	app.WaitFor("text:/permanently deletes this file/", timeout)
	app.Press("label:Cancel")
	app.WaitForGone(dialog, timeout)
	if got, err := os.ReadFile(filepath.Join(repo, "a.txt")); err != nil || string(got) != "first untracked\n" {
		t.Fatalf("cancel changed the selected file: %q, %v", got, err)
	}

	app.Focus("label:Unstaged · a.txt")
	app.Key("shift+f10")
	app.ChoosePopup("Discard changes…")
	waitConfirmation()
	put("a.txt", "new external edit\n")
	app.Press(confirm)
	app.WaitFor("id:changes-write-error", timeout)
	if got, err := os.ReadFile(filepath.Join(repo, "a.txt")); err != nil || string(got) != "new external edit\n" {
		t.Fatalf("stale discard lost an external edit: %q, %v", got, err)
	}

	app.Press("label:Unstaged · a.txt")
	app.PressWith("label:Unstaged · mixed.txt", core.Modifiers{Shift: true})
	app.Press("id:unstaged-more-actions")
	app.ChoosePopup("Discard")
	waitConfirmation()
	app.WaitFor("text:Discard changes in 2 files?", timeout)
	app.Press(confirm)
	app.WaitFor("text:Unstaged (1)", timeout)
	app.WaitFor("text:Staged (1)", timeout)
	if got, err := os.ReadFile(filepath.Join(repo, "mixed.txt")); err != nil || string(got) != "staged\n" {
		t.Fatalf("discard lost the staged version: %q, %v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(repo, "a.txt")); !os.IsNotExist(err) {
		t.Fatalf("selected untracked file remains: %v", err)
	}

	app.Press("id:unstaged-more-actions")
	app.ChoosePopup("Discard All")
	waitConfirmation()
	app.Press(confirm)
	app.WaitFor("text:Unstaged (0)", timeout)
	app.WaitFor("text:Staged (1)", timeout)
	if _, err := os.Lstat(filepath.Join(repo, "z.txt")); !os.IsNotExist(err) {
		t.Fatalf("untracked file remains after Discard All: %v", err)
	}
}
