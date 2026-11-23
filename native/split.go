package main

import (
	"math"
	"strconv"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

type splitSpec struct {
	id                    string
	axis                  base.Axis
	initial, lower, upper float32
}

func split(cx *core.Scope, chrome palette, spec splitSpec, label string, visible func() bool, first, second core.Node) core.Node {
	const divider = 5
	storage, key := cx.Storage(), "gitwave.split."+spec.id
	clamp := func(value float32) float32 { return max(spec.lower, min(value, spec.upper)) }
	initial := spec.initial
	cx.Untrack(func() {
		if text, ok := storage.GetItem(key); ok {
			if value, err := strconv.ParseFloat(text, 32); err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) {
				initial = clamp(float32(value))
			}
		}
	})
	ratio := core.NewSignal(cx, initial)
	span := core.NewSignal(cx, core.Points(0))
	set := func(value float32, persist bool) {
		next := clamp(value)
		ratio.Set(next)
		if persist {
			_ = storage.SetItem(key, strconv.FormatFloat(float64(next), 'f', -1, 32))
		}
	}
	layer := func() core.Style {
		length := core.Pct(ratio.Get())
		if size := span.Get(); size > divider {
			length = core.Pt((size - divider) * core.Points(ratio.Get()/100))
		}
		if spec.axis == base.Vertical {
			return core.Style{Height: length}
		}
		return core.Style{Width: length}
	}
	direction, cursor := core.Row, core.CursorColResize
	previous, next := "left", "right"
	separatorSize := core.Style{Width: core.Pt(divider)}
	axisLength := func(size core.Size) core.Points { return size.Width }
	delta := func(point core.Point) core.Points { return point.X }
	if spec.axis == base.Vertical {
		direction, cursor = core.Column, core.CursorRowResize
		previous, next = "up", "down"
		separatorSize = core.Style{Height: core.Pt(divider)}
		axisLength = func(size core.Size) core.Points { return size.Height }
		delta = func(point core.Point) core.Points { return point.Y }
	}
	separatorSize.Background, separatorSize.BorderColor, separatorSize.Cursor = chrome.bar, chrome.border, cursor
	separatorSize.BorderWidth = 1
	separatorSize.Hover = &core.Style{Background: chrome.accent}
	separatorSize.FocusVisible = &core.Style{Background: chrome.accent}
	start := initial
	return core.View{
		TestID: "split-" + spec.id,
		Style: core.Style{Direction: direction, FlexGrow: 1, FlexShrink: 1,
			MinHeight: core.Pt(0), MinWidth: core.Pt(0), Overflow: core.Hidden},
		OnLayout: func(bounds core.Bounds) { span.Set(axisLength(bounds.Size)) },
		Children: []core.Node{
			core.View{Style: core.Style{Overflow: core.Hidden}, StyleFunc: func() core.Style {
				if !visible() {
					return core.Style{Display: core.DisplayNone}
				}
				return layer()
			}, Children: []core.Node{first}},
			core.View{
				TestID: "resize-" + spec.id, Role: core.RoleAdjustable, Label: label,
				Focusable: visible, Style: separatorSize,
				StyleFunc: func() core.Style {
					if !visible() {
						return core.Style{Display: core.DisplayNone}
					}
					return core.Style{}
				},
				AccessibilityValue: func() core.AccessibilityValue {
					return core.AccessibilityValue{Min: spec.lower, Max: spec.upper, Now: ratio.Get()}
				},
				AccessibilityActions: []core.AccessibilityAction{{Name: core.ActionIncrement}, {Name: core.ActionDecrement}},
				OnAccessibilityAction: func(name string) {
					if name == core.ActionIncrement {
						set(ratio.Get()+2, true)
					} else if name == core.ActionDecrement {
						set(ratio.Get()-2, true)
					}
				},
				OnPress: func(e core.PressEvent) {
					if e.Clicks == 2 {
						set(spec.initial, true)
					}
				},
				OnKeyDown: func(e core.KeyEvent) bool {
					step := float32(2)
					if e.Modifiers.Shift {
						step = 10
					}
					switch e.Key {
					case previous:
						set(ratio.Get()-step, true)
					case next:
						set(ratio.Get()+step, true)
					case "home":
						set(spec.initial, true)
					case "end":
						set(spec.upper, true)
					default:
						return false
					}
					return true
				},
				OnPan: func(e core.PanEvent) {
					if e.Phase == core.GestureBegan {
						start = ratio.Get()
					}
					if size := span.Get(); size > divider {
						set(start+100*float32(delta(e.Translation)/(size-divider)),
							e.Phase == core.GestureEnded || e.Phase == core.GestureCancelled)
					}
				},
			},
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0),
				MinHeight: core.Pt(0), Overflow: core.Hidden}, Children: []core.Node{second}},
		},
	}
}
