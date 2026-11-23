package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

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

func TestNativeHistoryOpensCommitAndSwitchesFiles(t *testing.T) {
	if os.Getenv("GITWAVE_NATIVE_CORE") == "" {
		t.Skip("run make native-test to supply the Rust executable")
	}
	repo := filepath.Join(t.TempDir(), "sample-repo")
	if err := os.Mkdir(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
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
	git("init", "-b", "main")
	git("config", "user.name", "GitWave Test")
	git("config", "user.email", "test@example.invalid")
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
		app.PickDirectories = func(core.DirectoriesRequest) []core.Directory {
			return []core.Directory{{Name: "test repo", Path: repo}}
		}
		app.Press("label:Add local repository")
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
