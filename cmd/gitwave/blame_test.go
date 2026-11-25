package main

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestNativeBlameFromHistoricalDiffAndDeletedFile(t *testing.T) {
	repo, run := historyRepository(t)
	put := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	put("a.txt", "first owner line\nold line\n")
	put("b.txt", "first b\n")
	put("deleted.txt", "removed file contents\n")
	run("", "add", ".")
	run("", "commit", "-m", "Original owners")
	root := run("", "rev-parse", "HEAD")
	put("a.txt", "first owner line\nhistorical text\n")
	put("b.txt", "historical b text\n")
	run("", "commit", "-am", "Historical change")
	put("a.txt", "only in current HEAD\n")
	run("", "rm", "deleted.txt")
	run("", "commit", "-am", "Current HEAD change")

	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.open(repo)
		return w.view()
	}, nsgotest.Size(1200, 800), nsgotest.Environment(core.Environment{
		OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en", ReduceMotion: true,
	}))
	const timeout = 5 * time.Second
	app.WaitFor("label:/Historical change/", timeout)
	app.Press("label:/Historical change/")
	app.WaitFor("label:Blame", timeout)
	app.Press("label:Blame")
	app.WaitFor("id:blame-lines", timeout)
	app.WaitFor("text:/^historical text$/", timeout)
	app.WaitForGone("text:/only in current HEAD/", timeout)
	app.Press("label:Back to diff")
	app.WaitForGone("id:blame-panel", timeout)
	app.WaitFor("label:Blame", timeout)
	app.Press("label:Blame")
	app.WaitFor("id:blame-lines", timeout)
	app.Press("label:b.txt")
	app.WaitForGone("id:blame-panel", timeout)
	app.WaitFor("text:/^historical b text$/", timeout)
	app.Press("label:/Current HEAD change/")
	app.WaitFor("label:deleted.txt", timeout)
	app.Press("label:deleted.txt")
	app.WaitFor("text:/^removed file contents$/", timeout)
	app.Press("label:Blame")
	app.WaitFor("id:blame-lines", timeout)
	app.WaitFor("text:/^removed file contents$/", timeout)
	app.Press("label:View commit " + shortSHA(root))
	app.WaitForGone("id:blame-panel", timeout)
	app.WaitFor("id:commit-header >> text:"+shortSHA(root), timeout)
}

func TestNativeBlameScrollsLongLines(t *testing.T) {
	const long = "wide line "
	sha := strings.Repeat("a", 40)
	lines := []gitcli.BlameLine{{LineNo: 1, SHA: sha, Content: long + strings.Repeat("汉", 140)}}
	for n := 2; n <= 80; n++ {
		lines = append(lines, gitcli.BlameLine{LineNo: n, Content: "blamed line " + strconv.Itoa(n)})
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		return w.blameLines(cx, colors(false), blameState{lines: lines})
	}, nsgotest.Size(650, 260), nsgotest.Environment(core.Environment{
		OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en", ReduceMotion: true,
	}))
	const timeout = 5 * time.Second
	app.WaitFor("text:/^wide line /", timeout)
	scroll := app.Find("id:blame-scroll")
	app.Scroll("id:blame-scroll", (scroll.Content.Width-scroll.Frame.Size.Width)/2)
	app.WaitUntil(timeout, func() bool {
		offset := app.Find("id:blame-scroll").Offset.X
		if nsgotest.Target() != "" {
			// Platform animations live on the native surface. Its step model
			// receives scroll offsets, but not the animated view's translation.
			return offset > 0
		}
		gutter := app.Find("label:View commit " + shortSHA(sha)).Parent
		return offset > 0 && gutter.Animated.TranslateX == offset
	})
	app.Scroll("id:blame-lines", 600)
	app.WaitUntil(timeout, func() bool { return app.Find("id:blame-lines").Offset.Y > 0 })
}

func TestBlamePreservesLineOwnersAcrossAFileRename(t *testing.T) {
	useFixtureCommitIdentity(t)
	t.Setenv("GIT_AUTHOR_DATE", "@1700000000 +0000")
	t.Setenv("GIT_COMMITTER_DATE", "@1700000000 +0000")
	repo, git := historyRepository(t)
	original, renamed := filepath.Join(repo, "origin.txt"), filepath.Join(repo, "renamed.txt")
	content := "line for the first revision\r\n\r\nold final line"
	if err := os.WriteFile(original, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	git("", "add", ".")
	git("", "commit", "-m", "First")
	first := git("", "rev-parse", "HEAD")
	git("", "mv", "origin.txt", "renamed.txt")
	git("", "commit", "-m", "Rename without edits")
	if err := os.WriteFile(renamed, []byte("line for the first revision\r\n\r\nnew final line"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_AUTHOR_DATE", "@1700000009 +0000")
	t.Setenv("GIT_COMMITTER_DATE", "@1700000010 +0000")
	t.Setenv("GIT_AUTHOR_NAME", "New Author")
	git("", "commit", "-am", "Last edit")
	head := git("", "rev-parse", "HEAD")
	b, err := startBackend(t.Context(), repo)
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.repo.Blame(t.Context(), gitcli.Commit{SHA: head}, "renamed.txt", false)
	if err != nil {
		t.Fatal(err)
	}
	want := []gitcli.BlameLine{
		{LineNo: 1, SHA: first, Author: "GitWave", AuthorEmail: "test@example.invalid", Time: 1700000000, Content: "line for the first revision"},
		{LineNo: 2, SHA: first, Author: "GitWave", AuthorEmail: "test@example.invalid", Time: 1700000000},
		{LineNo: 3, SHA: head, Author: "New Author", AuthorEmail: "test@example.invalid", Time: 1700000010, Content: "new final line"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("blame = %+v, want %+v", got, want)
	}
}
