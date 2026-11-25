package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestDiffRowsPairsUnequalChanges(t *testing.T) {
	hunk := diffHunk{
		OldStart: 1, OldLines: 2, NewStart: 1, NewLines: 3,
		Lines: []diffLine{
			{Kind: "removed", Content: "old"},
			{Kind: "added", Content: "new"},
			{Kind: "added", Content: "more"},
			{Kind: "context", Content: "same"},
		},
	}
	hunks := []diffHunk{hunk}
	split := diffRows(hunks, true)
	if len(split) != 4 || split[1].left.Content != "old" || split[1].right.Content != "new" ||
		split[2].left != nil || split[2].right.Content != "more" {
		t.Fatalf("split rows: %#v", split)
	}
	var order []string
	for _, row := range diffRows(hunks, false)[1:] {
		line := row.right
		if row.show == "left" {
			line = row.left
		}
		order = append(order, line.Content)
	}
	if !slices.Equal(order, []string{"old", "new", "more", "same"}) {
		t.Fatalf("unified order: %v", order)
	}
}

func TestChangedRangeKeepsUnicodeCharactersWhole(t *testing.T) {
	first, second := "文件🙂old世界", "文件🙂新版世界"
	from, to := changedRange(first, second)
	if got := first[from:to]; got != "old" {
		t.Fatalf("old range: %q", got)
	}
	from, to = changedRange(second, first)
	if got := second[from:to]; got != "新版" {
		t.Fatalf("new range: %q", got)
	}
}

func TestNativeDiffPreviewFitsNarrowInspectorAndStopsAtLimit(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Before the large file")
	var content strings.Builder
	for i := range 50_100 {
		fmt.Fprintf(&content, "line %05d\n", i)
	}
	writeRepositoryFixtureFile(t, repo, "preview.txt", content.String())
	git("", "add", "preview.txt")
	git("", "commit", "-m", "Large text preview")
	app := refTestApp(t, repo)
	app.Resize(880, 640)
	const commit = "label:/^Large text preview,/"
	app.WaitFor(commit, refTestTimeout)
	app.Press(commit)
	app.WaitFor("id:diff-line-list", refTestTimeout)
	for _, divider := range []string{"id:resize-navigator", "id:resize-history-files"} {
		app.Focus(divider)
		app.Key("end")
	}
	app.WaitFor("text:Preview truncated to keep scrolling responsive.", refTestTimeout)
	const expand = "label:Load a larger preview"
	const viewport = "id:diff-horizontal-scroll"
	panel := app.Find(viewport).Bounds()
	button := app.Find(expand).Bounds()
	if button.Origin.X < panel.Origin.X || button.Origin.X+button.Size.Width > panel.Origin.X+panel.Size.Width+1 {
		t.Fatalf("preview button is outside the inspector: button %v, viewport %v", button, panel)
	}
	app.Press(expand)
	app.WaitFor("text:Preview limit reached.", refTestTimeout)
	if app.Exists(expand) {
		t.Fatal("the exhausted preview still offers to load more")
	}
	app.Press("label:Switch to split view")
	app.WaitFor("label:Switch to unified view", refTestTimeout)
	app.Find("id:diff-line-list")
	if mounted := len(app.QueryMounted("id:diff-line")); mounted == 0 || mounted > 100 {
		t.Fatalf("the expanded preview did not keep its virtual window bounded: %d mounted lines", mounted)
	}
}

func TestNativeDiffKeepsLongLineEndsReachable(t *testing.T) {
	for _, sample := range []struct{ name, prefix string }{
		{"tabs", strings.Repeat("\t", 100)},
		{"flags", strings.Repeat("🇨🇳", 75)},
	} {
		t.Run(sample.name, func(t *testing.T) {
			lines := []diffLine{{Kind: "added", Content: sample.prefix + "RIGHT-END", New: 1}}
			if sample.name == "flags" {
				// More columns, but a narrower line with the macOS test font.
				lines = append(lines, diffLine{Kind: "added", Content: strings.Repeat("i", 175), New: 2})
			}
			additions := len(lines)
			for i := 1; i <= 80; i++ {
				lines = append(lines, diffLine{Kind: "context", Content: "unchanged", Old: i, New: i + additions})
			}
			file := fileDiff{Path: "long.txt", Additions: additions,
				Hunks: []diffHunk{{OldStart: 1, OldLines: 80, NewStart: 1, NewLines: len(lines), Lines: lines}}}
			var position diffViewport
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				w := newWorkbench(cx)
				chrome := colors(false)
				chrome.fonts.codeScale = 10.0 / 12
				return core.Dynamic(func(*core.Scope) core.Node {
					return w.fileDiffView(chrome, preview{Files: []fileDiff{file}},
						fileDiffOptions{collapsed: w.diffCollapsed, viewport: &position})
				})
			}, nsgotest.Size(320, 400), nsgotest.Environment(core.Environment{
				OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en", ReduceMotion: true,
			}))
			const viewport = "id:diff-horizontal-scroll"
			assertEnd := func() {
				t.Helper()
				app.WaitUntil(refTestTimeout, func() bool {
					scroll := app.Find(viewport)
					return scroll.Content.Width > scroll.Frame.Size.Width
				})
				app.Scroll(viewport, 20_000)
				scroll := app.Find(viewport)
				if remaining := scroll.Content.Width - scroll.Frame.Size.Width - scroll.Offset.X; remaining > 2 {
					app.Scroll(viewport, remaining)
				}
				app.WaitUntil(refTestTimeout, func() bool {
					scroll := app.Find(viewport)
					return scroll.Offset.X >= scroll.Content.Width-scroll.Frame.Size.Width-2
				})
				panel := app.Find(viewport).Bounds()
				rendered := app.Find("text:/RIGHT-END$/")
				end := rendered.Bounds().Origin.X + rendered.Bounds().Size.Width
				if !strings.HasSuffix(rendered.Text, "RIGHT-END") || end < panel.Origin.X+20 || end > panel.Origin.X+panel.Size.Width+1 {
					t.Fatalf("right edge of the line is outside the scrolled viewport: line %v, viewport %v", rendered.Bounds(), panel)
				}
			}
			assertEnd()
			app.Press("label:Switch to split view")
			app.WaitFor("label:Switch to unified view", refTestTimeout)
			assertEnd()
		})
	}
}

func TestDiffPreservesMeasuredWidthAcrossViews(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Before changes")
	content := strings.Repeat("中", 240) + "\n" + strings.Repeat("short\n", 298) + strings.Repeat("W", 400) + "LAST\n"
	writeRepositoryFixtureFile(t, repo, "long.txt", content)
	app := refTestApp(t, repo)
	const outer, inner = "id:diff-horizontal-scroll", "id:diff-line-list"
	app.WaitFor("label:/^Before changes,/", refTestTimeout)
	app.Press("id:navigate-changes")
	app.WaitFor("label:Unstaged · long.txt", refTestTimeout)
	app.Press("label:Unstaged · long.txt")
	app.WaitFor(outer, refTestTimeout)
	initial := app.Find(outer).Content.Width
	app.Scroll(inner, 20_000)
	app.WaitFor("text:/LAST$/", refTestTimeout)
	app.WaitUntil(refTestTimeout, func() bool { return app.Find(outer).Content.Width > initial+100 })
	app.Scroll(outer, 20_000)
	before := app.Find(outer).Offset.X
	app.ScrollTo(inner, core.Point{})
	app.WaitForGone("text:/LAST$/", refTestTimeout)
	app.Press("label:Collapse file diff")
	app.WaitForGone(outer, refTestTimeout)
	app.Press("label:Expand file diff")
	app.WaitUntil(refTestTimeout, func() bool { return app.Find(outer).Offset.X >= before-2 })
	app.Press("id:navigate-history")
	app.WaitForGone(outer, refTestTimeout)
	app.Press("id:navigate-changes")
	app.WaitFor(outer, refTestTimeout)
	app.WaitUntil(refTestTimeout, func() bool { return app.Find(outer).Offset.X >= before-2 })
}
