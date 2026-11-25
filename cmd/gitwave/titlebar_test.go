package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestNativeApplicationMenuRestoresWindowActions(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		newWorkbench(cx).configureTitleBar()
		return core.Text{ContentFunc: func() string {
			return fmt.Sprintf("fullscreen=%t minimized=%t", cx.Env().FullScreen(), cx.Env().Minimized())
		}}
	}, nsgotest.Environment(refreshEnvironment(core.LifecycleActive)))
	const timeout = 10 * time.Second
	app.WaitUntil(timeout, func() bool { return len(app.Menu) == 6 })
	labels := func(items []nsgotest.MenuItem) []string {
		out := make([]string, 0, len(items))
		for _, item := range items {
			out = append(out, item.Label)
		}
		return out
	}
	if got := labels(app.Menu); !slices.Equal(got, []string{"GitWave", "Edit", "Workspace", "Repository", "Branch", "Window"}) {
		t.Fatalf("macOS app menus = %q", got)
	}
	for i, want := range [][]string{
		{"New", "Rename", "AI", "Export", "Import", "Delete"},
		{"Init", "Clone", "Add", "Fetch", "LFS", "Hooks", "New worktree"},
		{"New", "Pull", "Push", "PR"},
	} {
		if menu := app.Menu[i+2]; !slices.Equal(labels(menu.Items), want) {
			t.Fatalf("%s menu = %q, want %q", menu.Label, labels(menu.Items), want)
		}
	}
	application := app.Menu[0].Items
	if len(application) != 10 || application[0].Label != "About GitWave" || application[1].Label != "Check for Updates…" ||
		!application[2].Separator || application[3].Label != "Settings…" || !application[4].Separator ||
		application[5].Role != core.MenuHide || application[6].Role != core.MenuHideOthers || application[7].Role != core.MenuShowAll ||
		!application[8].Separator || application[9].Role != core.MenuQuit {
		t.Fatalf("GitWave menu = %+v", application)
	}
	window := app.Menu[5]
	if window.Role != core.MenuWindowMenu || len(window.Items) != 5 ||
		window.Items[0].Role != core.MenuMinimize ||
		window.Items[1].Role != core.MenuZoom || window.Items[2].Role != core.MenuToggleFullScreen ||
		!window.Items[3].Separator || window.Items[4].Role != core.MenuBringAllToFront {
		t.Fatalf("Window menu = %+v", window)
	}
	if os.Getenv("NSGO_TEST_TARGET") != "" {
		// AppKit's fullscreen role requires a key window, which the native
		// runner can lack when it injects keys into a background window.
		return
	}
	app.WaitFor("text:fullscreen=false minimized=false", timeout)
	app.Key("ctrl+meta+f")
	app.WaitFor("text:fullscreen=true minimized=false", timeout)
	app.Key("ctrl+meta+f")
	app.WaitFor("text:fullscreen=false minimized=false", timeout)
	app.Key("meta+m")
	app.WaitFor("text:fullscreen=false minimized=true", timeout)
}

func TestTitleBarFitsDesktopSizes(t *testing.T) {
	if os.Getenv("NSGO_TEST_TARGET") != "" {
		t.Skip("the native harness retains the host operating system")
	}
	for _, test := range []struct {
		name   string
		os     core.OS
		insets core.Edges
	}{
		{"macOS", core.OSMacOS, core.Edges{Left: 82}},
		{"Windows", core.OSWindows, core.Edges{Right: 112}},
		{"Linux", core.OSLinux, core.Edges{}},
	} {
		for _, appearance := range []struct{ locale, size, push string }{
			{"en", "16", "Push"},
			{"en", "20", "Push"},
			{"zh-CN", "20", "推送"},
		} {
			for _, width := range []core.Points{1024, 1030, 1050, 1060, 1080, 1120, 1152, 1200, 1235, 1280, 1375, 1440} {
				t.Run(fmt.Sprintf("%s-%s-%s-%g", test.name, appearance.locale, appearance.size, width), func(t *testing.T) {
					app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
						w := newWorkbench(cx)
						w.workspaceState.Set(workspace.State{Workspace: workspace.Summary{
							ID: "long-name", Name: strings.Repeat("GitWave workspace with a long name ", 6),
						}})
						prefs := w.desktopPrefs.Get()
						prefs.Fonts.UISize = appearance.size
						w.desktopPrefs.Set(prefs)
						return w.view()
					},
						nsgotest.Size(width, 768), nsgotest.Environment(core.Environment{
							OS: test.os, Idiom: core.IdiomDesktop, Locale: appearance.locale, TitleBar: test.insets,
						}))
					leftEdge := func(view *nsgotest.View) core.Points {
						var edge core.Points
						for ; view != nil; view = view.Parent {
							edge += view.Frame.Origin.X
						}
						return edge
					}
					var edge core.Points
					for _, id := range []string{"workspace-selector", "reveal-repository", "open-terminal", "open-editor",
						"titlebar-drag", "titlebar-changes", "stash-save", "fetch", "pull", "push"} {
						control := app.FindMounted("id:" + id)
						left := leftEdge(control)
						if left < edge || control.Frame.Size.Width <= 0 {
							t.Fatalf("%s overlaps or has no width: left %v, previous edge %v, frame %+v", id, left, edge, control.Frame)
						}
						edge = left + control.Frame.Size.Width
					}
					if edge > width-test.insets.Right {
						t.Fatalf("Push overlaps the reserved window controls: right edge %v", edge)
					}
					if test.os == core.OSLinux {
						closeLabel := "Close"
						if appearance.locale == "zh-CN" {
							closeLabel = "关闭"
						}
						closeButton := app.FindMounted("label:" + closeLabel)
						if edge := leftEdge(closeButton) + closeButton.Frame.Size.Width; edge > width {
							t.Fatalf("Linux close button is clipped: right edge %v", edge)
						}
					}
					if test.os == core.OSMacOS {
						app.FindMounted("id:push >> text:" + appearance.push)
					}
					drag := app.FindMounted("id:titlebar-drag")
					if drag.Frame.Size.Width < 8 || drag.Props.WindowDrag != core.WindowDragOn {
						t.Fatalf("the title bar has no usable drag region: %+v", drag.Frame)
					}
				})
			}
		}
	}
}
