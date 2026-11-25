package gitcli

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestBlameUsesExactRevisionAndIgnoresTextConversions(t *testing.T) {
	dir := newFixture(t)
	commit := func(author, authorDate, committerDate, message string) {
		t.Helper()
		fixtureGit(t, dir, "add", ".")
		cmd := fixtureCommand(t, dir, "-c", "user.name="+author, "commit", "--date="+authorDate, "-m", message)
		cmd.Env = append(cmd.Env, "GIT_COMMITTER_DATE="+committerDate)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("commit blame fixture: %v\n%s", err, output)
		}
	}
	const path = "[blâme].txt"
	writeFixture(t, dir, ".gitattributes", "*.txt -text diff=blame-fixture\n")
	writeFixture(t, dir, path, " first line\r\n\r\nold final line\r")
	commit("First Author", "@1700000000 +0000", "@1700000001 +0000", "Original file")
	old := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	writeFixture(t, dir, path, " first line\r\n\r\nnew final line")
	commit("Second Author", "@1700000002 +0000", "@1700000003 +0000", "Changed last line")
	tip := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	writeFixture(t, dir, ".mailmap", "Other Author <other@example.invalid> <test@example.invalid>\n")
	writeFixture(t, dir, path, "uncommitted text\n")
	writeFixture(t, dir, "ignore-file", tip+"\n")
	fixtureGit(t, dir, "config", "diff.blame-fixture.textconv", "gitwave-nonexistent-textconv")
	fixtureGit(t, dir, "config", "blame.ignoreRevsFile", "ignore-file")
	repo := openFixture(t, dir)
	got, err := repo.Blame(t.Context(), Commit{SHA: tip}, path, false)
	want := []BlameLine{
		{LineNo: 1, SHA: old, Author: "First Author", AuthorEmail: "test@example.invalid", Time: 1700000001, Content: " first line"},
		{LineNo: 2, SHA: old, Author: "First Author", AuthorEmail: "test@example.invalid", Time: 1700000001},
		{LineNo: 3, SHA: tip, Author: "Second Author", AuthorEmail: "test@example.invalid", Time: 1700000003, Content: "new final line"},
	}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("tip blame: %+v, %v", got, err)
	}
	fixtureGit(t, dir, "config", "blame.ignoreRevsFile", "missing-ignore-file")
	previous, err := repo.Blame(t.Context(), Commit{SHA: old}, path, false)
	if err != nil || len(previous) != 3 || previous[2].Content != "old final line\r" || previous[2].SHA != old {
		t.Fatalf("historical blame: %+v, %v", previous, err)
	}
	if _, err := repo.Blame(t.Context(), Commit{SHA: tip}, "../file", false); err == nil {
		t.Fatal("outside path accepted")
	}
}

func TestBlameDeletionAcrossShallowBoundary(t *testing.T) {
	dir := newFixture(t)
	fixtureGit(t, dir, "branch", "base")
	parent := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	fixtureGit(t, dir, "rm", "mixed.txt")
	fixtureGit(t, dir, "commit", "-m", "Delete file")
	tip := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	shallow := t.TempDir()
	fixtureGit(t, dir, "clone", "--depth=1", "--no-single-branch", "--no-local", dir, shallow)
	repo := openFixture(t, shallow)
	commits, err := repo.Commits(t.Context(), []string{tip})
	if err != nil || len(commits) != 1 || len(commits[0].Parents) != 0 {
		t.Fatalf("expected a shallow boundary: %+v, %v", commits, err)
	}
	lines, err := repo.Blame(t.Context(), commits[0], "mixed.txt", true)
	if err != nil || len(lines) != 1 || lines[0].SHA != parent || lines[0].Content != "original version" {
		t.Fatalf("deleted file blame: %+v, %v", lines, err)
	}
}

func TestBlameBoundsLargeBlobsAndLineCounts(t *testing.T) {
	dir := newFixture(t)
	writeFixture(t, dir, "many-lines", strings.Repeat("\n", maxBlameLines+1))
	writeFixture(t, dir, "large-blob", strings.Repeat("a", expandedBytes+1))
	fixtureGit(t, dir, "add", ".")
	fixtureGit(t, dir, "commit", "-m", "Large files")
	tip := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	repo := openFixture(t, dir)
	for _, path := range []string{"many-lines", "large-blob"} {
		if _, err := repo.Blame(t.Context(), Commit{SHA: tip}, path, false); !errors.Is(err, ErrBlameTooLarge) {
			t.Fatalf("blame %s: want limit error, got %v", path, err)
		}
	}
	if _, err := repo.readLimited(t.Context(), "", 1024, "cat-file", "blob", tip+":many-lines"); !errors.Is(err, errGitOutputLimit) {
		t.Fatalf("bounded Git output: %v", err)
	}
}

func TestBlameAssignsGroupsByResultLine(t *testing.T) {
	old, tip := strings.Repeat("a", 40), strings.Repeat("b", 40)
	block := "filename \"file\\nname\"\n"
	output := tip + " 2 2 1\nauthor a\n" + block + old + " 1 1 1\n" + block + old + " 8 3 1\n" + block
	lines := blameContent("old\r\nnew\n\n", 3)
	oids, err := assignBlame(t.Context(), output, lines)
	if err != nil || !slices.Equal(oids, []string{tip, old}) || lines[0].SHA != old ||
		lines[1].SHA != tip || lines[2].SHA != old || lines[2].Content != "" {
		t.Fatalf("unexpected incremental blame: %+v, %v", lines, err)
	}
	for _, malformed := range []string{
		old + " 1 1 1\n",
		old + " 1 4 1\n" + block,
		old + " 1 1 3\n" + block + tip + " 1 2 1\n" + block,
		old + " 1 1 2\n" + block,
	} {
		if _, err := assignBlame(t.Context(), malformed, make([]BlameLine, 3)); err == nil {
			t.Fatalf("accepted invalid blame: %q", malformed)
		}
	}
}
