package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

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
		for _, width := range []core.Points{1024, 1030, 1050, 1060, 1080, 1120, 1152, 1200, 1235, 1280, 1375, 1440} {
			t.Run(fmt.Sprintf("%s-%g", test.name, width), func(t *testing.T) {
				app := nsgotest.Start(t, func(cx *core.Scope) core.Node { return newWorkbench(cx).view() },
					nsgotest.Size(width, 768), nsgotest.Environment(core.Environment{
						OS: test.os, Idiom: core.IdiomDesktop, Locale: "en", TitleBar: test.insets,
					}))
				rightEdge := func(view *nsgotest.View) core.Points {
					edge := view.Frame.Size.Width
					for ; view != nil; view = view.Parent {
						edge += view.Frame.Origin.X
					}
					return edge
				}
				push := app.FindMounted("label:Push")
				if edge := rightEdge(push); edge > width-test.insets.Right {
					t.Fatalf("Push overlaps the reserved window controls: right edge %v", edge)
				}
				if test.os == core.OSLinux {
					closeButton := app.FindMounted("label:Close")
					if edge := rightEdge(closeButton); edge > width {
						t.Fatalf("Linux close button is clipped: right edge %v", edge)
					}
				}
				drag := app.FindMounted("id:titlebar-drag")
				if drag.Frame.Size.Width < 8 || drag.Props.WindowDrag != core.WindowDragOn {
					t.Fatalf("the title bar has no usable drag region: %+v", drag.Frame)
				}
			})
		}
	}
}
