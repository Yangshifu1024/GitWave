package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) {
	if gitcli.LocalLFSFilter() || gitcli.LFSPrePush() {
		return
	}
	if gitcli.CredentialHelper(os.Args[1:], os.Stdin, os.Stdout) {
		return
	}
	nsgotest.Main(m)
}

func TestGraphRoutesMergeAcrossPages(t *testing.T) {
	commits := []commit{
		{SHA: "merge", Parents: []string{"main", "branch"}},
		{SHA: "main", Parents: []string{"root"}},
		{SHA: "branch", Lane: 1, Parents: []string{"root"}},
		{SHA: "root"},
	}
	prefix := computeGraph(commits[:2])
	if !prefix[0].stub || len(prefix[0].outgoing) != 0 {
		t.Fatalf("first page: %#v", prefix)
	}
	rows := computeGraph(commits)
	if !slices.Equal(rows[0].outgoing, []int{1}) || !slices.Equal(rows[1].through, []int{1}) ||
		!slices.Equal(rows[2].incoming, []int{1}) || !slices.Equal(rows[3].incoming, []int{0, 1}) {
		t.Fatalf("merge edges: %#v", rows)
	}
}

func TestNativeHistoryKeepsDetailsWhenDiffFails(t *testing.T) {
	repo, git := historyRepository(t)
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("content\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("", "add", ".")
	git("", "commit", "-m", "Readable commit")
	tree := git("", "rev-parse", "HEAD^{tree}")
	if err := os.Rename(filepath.Join(repo, ".git", "objects", tree[:2], tree[2:]), filepath.Join(t.TempDir(), "tree")); err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.open(repo)
		return w.view()
	}, nsgotest.Size(1200, 800), nsgotest.Environment(core.Environment{
		OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en",
	}))
	const timeout = 5 * time.Second
	app.WaitFor("label:/Readable commit/", timeout)
	app.Press("label:/Readable commit/")
	app.WaitFor("text:/read changed files/", timeout)
	app.WaitFor("id:commit-header", timeout)
}

func TestNativeHistoryExpandsCompleteCommitDetails(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Earlier commit", "-m", "Earlier body.")
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("added line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("", "add", ".")
	body := strings.Repeat("Long commit paragraph with notes.\n", 30) + "Last line of the message."
	git("", "-c", "user.name="+strings.Repeat("LongAuthor", 18), "-c", "user.email=", "commit", "-m", "Latest commit", "-m", body)
	sha := git("", "rev-parse", "HEAD")
	git("", "remote", "add", "origin", "https://example.invalid/gitwave.git")
	git("", "update-ref", "refs/remotes/origin/main", sha)
	git("", "tag", "release-tip")
	app := refTestApp(t, repo)
	app.Resize(920, 640)

	const header = "id:commit-header"
	const disclosure = header + " >> id:commit-message-toggle"
	const message = header + " >> id:commit-message-body"
	const latest = "label:/^Latest commit,/"
	app.WaitFor(latest, refTestTimeout)
	app.Press(latest)
	app.WaitFor(disclosure, refTestTimeout)
	app.Find(header + " >> label:Reference main")
	app.Find(header + " >> label:Reference origin/main")
	app.Find(header + " >> label:Reference release-tip")
	if panel := app.Find(header); panel.Content.Width > panel.Frame.Size.Width+1 {
		t.Fatalf("author pushed the inspector past its viewport: content %v, viewport %v", panel.Content, panel.Frame.Size)
	}
	if app.Exists(message) {
		t.Fatal("the commit message was expanded by default")
	}
	app.Press(disclosure)
	expanded := app.Find(message)
	headerFrame := app.Find(header).Bounds()
	if !strings.HasSuffix(expanded.Text, "Last line of the message.") || expanded.Frame.Size.Height <= headerFrame.Size.Height {
		t.Fatalf("expected the full message in a smaller scrolling header, body frame %v, header frame %v", expanded.Frame, headerFrame)
	}
	app.Scroll(header, 1800)
	app.WaitUntil(refTestTimeout, func() bool {
		frame := app.Find(message).Bounds()
		return app.Find(header).Offset.Y > 0 && frame.Origin.Y+frame.Size.Height <= headerFrame.Origin.Y+headerFrame.Size.Height
	})
	if file := app.Find("id:changed-file").Bounds(); file.Size.Height == 0 || file.Origin.Y < headerFrame.Origin.Y+headerFrame.Size.Height {
		t.Fatalf("long message blocked the file list: %v", file)
	}
	app.Press("id:changed-file")
	app.WaitFor("text:added line", refTestTimeout)

	app.Press("label:/^Earlier commit,/")
	app.WaitFor(header+" >> text:Earlier body.", refTestTimeout)
	if app.Exists(header + " >> label:Reference release-tip") {
		t.Fatal("the selected commit still shows a tag from another commit")
	}
	app.Press("id:navigate-changes")
	app.WaitForGone(header, refTestTimeout)
	app.Press("id:navigate-history")
	app.WaitFor(header+" >> text:Earlier body.", refTestTimeout)
	app.Press(disclosure)
	app.WaitForGone(message, refTestTimeout)

	app.Focus(latest)
	app.Key("shift+f10")
	app.ChoosePopup("Copy Commit Info")
	author := git("", "show", "-s", "--format=%an", sha)
	if got := app.Clipboard; !strings.HasPrefix(got, sha+"\nAuthor: "+author+"\nDate: ") || !strings.HasSuffix(got, "\n\nLatest commit") {
		t.Fatalf("copied commit info = %q", got)
	}
}

func TestNativeHistoryOpensReferenceBadgesAndPreservesTheSelectedCommit(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Reference base")
	git("", "commit", "--allow-empty", "-m", "Reference tip")
	head := git("", "rev-parse", "HEAD")
	git("", "remote", "add", "team/shared", "https://example.invalid/gitwave.git")
	git("", "update-ref", "refs/remotes/team/shared/main", head)
	git("", "update-ref", "refs/remotes/team/shared/feature/other", head)
	tag := "release-name-for-reference-badges"
	git("", "tag", tag)

	app := refTestApp(t, repo)
	tip := "label:/^Reference tip,/"
	app.WaitFor(tip+" >> label:Reference "+tag, refTestTimeout)
	app.Find(tip + " >> text:+2")
	app.WaitFor("text:End of history · 2 commits", refTestTimeout)
	app.Press("label:/^Reference base,/")
	app.WaitFor("id:commit-header >> text:Reference base", refTestTimeout)
	app.Press(tip + " >> id:history-refs-more")
	dialog := "id:history-refs-dialog"
	app.WaitFor(dialog+" >> label:Reference main", refTestTimeout)
	app.Find(dialog + " >> label:Reference team/shared/feature/other")
	if app.Exists(dialog+" >> label:Reference team/shared/main") || app.Find(tip).Props.Selected {
		t.Fatal("folded remote ref was shown or opening the refs selected its commit")
	}
	app.Screenshot("history-reference-badges")
	app.RightClick(dialog + " >> label:Reference " + tag)
	app.ChoosePopup("Delete tag")
	app.WaitForGone(dialog, refTestTimeout)
	app.WaitFor("id:ref-dialog", refTestTimeout)
	app.Press("label:Cancel")
	app.WaitForGone("id:ref-dialog", refTestTimeout)
	if got := git("", "tag", "--list", tag); got != tag {
		t.Fatalf("cancel should retain the tag, got %q", got)
	}
	app.WaitFor("id:commit-header >> text:Reference tip", refTestTimeout)
	app.Screenshot("history-reference-row")
}

func TestNativeHistoryReferenceOverflowScrolls(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Many references")
	for i := range 28 {
		git("", "tag", fmt.Sprintf("ref-%02d", i))
	}
	app := refTestApp(t, repo)
	app.WaitFor("id:history-refs-more", refTestTimeout)
	app.Press("id:history-refs-more")
	list := "id:history-refs-list"
	app.WaitFor(list+" >> label:Reference ref-00", refTestTimeout)
	app.Scroll(list, 700)
	last := list + " >> label:Reference ref-27"
	app.WaitFor(last, refTestTimeout)
	app.RightClick(last)
	app.ChoosePopup("Delete tag")
	app.WaitFor("id:ref-dialog", refTestTimeout)
	app.Press("id:confirm-ref-action")
	app.WaitForGone("id:ref-dialog", refTestTimeout)
	if got := git("", "tag", "--list", "ref-27"); got != "" {
		t.Fatalf("expected the last tag to be deleted, got %q", got)
	}
}

func TestNativeHistoryOpensCommitAndSwitchesFiles(t *testing.T) {
	repo, runGit := historyRepository(t)
	git := func(args ...string) { runGit("", args...) }
	put := func(name, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(text+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	putImage := func(red uint8) {
		t.Helper()
		file, err := os.Create(filepath.Join(repo, "sample.png"))
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		picture := image.NewRGBA(image.Rect(0, 0, 16, 16))
		picture.SetRGBA(0, 0, color.RGBA{R: red, A: 255})
		if err := png.Encode(file, picture); err != nil {
			t.Fatal(err)
		}
	}
	git("config", "commit.gpgsign", "false")
	put("a.txt", "before")
	put("b.txt", "before")
	putImage(30)
	git("add", ".")
	git("commit", "-m", "Initial version")
	git("switch", "-c", "feature/docs")
	put("README.md", "# Example repository")
	git("add", ".")
	git("commit", "-m", "Add repository documentation")
	git("switch", "main")
	git("merge", "--no-ff", "feature/docs", "-m", "Merge feature/docs")
	put("a.txt", "first file")
	put("b.txt", "second file")
	putImage(90)
	git("add", ".")
	git("commit", "-m", "Change two files")

	onTarget := os.Getenv("NSGO_TEST_TARGET") != ""
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		if onTarget {
			w.open(repo)
		}
		return w.view()
	}, nsgotest.Size(1200, 800), nsgotest.Environment(core.Environment{
		OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en",
	}))
	const timeout = 5 * time.Second
	titlebar := app.Find("id:custom-titlebar")
	if titlebar.Frame.Size.Height != titleBarHeight || titlebar.Props.WindowDrag != core.WindowDragOn {
		t.Fatalf("title bar height or drag region diverged: %+v", titlebar.Frame)
	}
	if !onTarget {
		app.PickDirectories = func(nsgotest.DirectoryPick) []core.Directory {
			return []core.Directory{{Name: "test repo", Path: repo}}
		}
		app.Press("id:workspace-selector")
		app.ChoosePopup("Add local repository")
	}
	tabs := app.WaitFor("id:repository-tabs", timeout)
	if tabs.Frame.Origin.Y != titleBarHeight || tabs.Frame.Size.Height != 32 {
		t.Fatalf("repository tabs are off the title bar or the wrong height: %+v", tabs.Frame)
	}
	app.WaitFor("label:/Change two files/", timeout)
	app.Press("label:/Change two files/")
	app.WaitFor("text:first file", timeout)
	app.Press("label:b.txt")
	app.WaitFor("text:second file", timeout)
	divider := app.Find("id:resize-history-files").Frame
	if divider.Size.Width != 5 {
		t.Fatalf("History file divider width = %v", divider.Size.Width)
	}
	app.Focus("id:resize-history-files")
	app.Key("right")
	if after := app.Find("id:resize-history-files").Frame; after.Origin.X <= divider.Origin.X {
		t.Fatalf("keyboard resize did not move the divider: %+v → %+v", divider, after)
	}
	app.Key("home")
	if app.Exists("text:first file") {
		t.Fatal("switching files left the previous patch visible")
	}
	app.Press("Switch to split view")
	app.WaitFor("text:second file", timeout)
	app.WaitFor("text:before", timeout)
	app.Screenshot("native-history-split")
	app.Press("label:sample.png")
	app.WaitFor("role:image Old", timeout)
	app.WaitFor("role:image New", timeout)
}
