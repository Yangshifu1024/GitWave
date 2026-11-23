package main

import (
	"cmp"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

const (
	historyRowHeight = 28
	laneGap          = 18
	graphPadding     = 8
	nodeRadius       = 3.2
)

type graphRow struct {
	through, incoming, outgoing []int
	stub                        bool
}

// The first parent stays in the child's lane and merges arrive in the
// parent's lane, matching the edge routing of the existing UI.
func computeGraph(commits []commit) []graphRow {
	index := make(map[string]int, len(commits))
	for i, c := range commits {
		index[c.SHA] = i
	}
	rows := make([]graphRow, len(commits))
	addLane := func(lanes []int, lane int) []int {
		if slices.Contains(lanes, lane) {
			return lanes
		}
		return append(lanes, lane)
	}
	for i, c := range commits {
		rows[i].stub = len(c.Parents) > 0
		for parentNumber, sha := range c.Parents {
			parent, loaded := index[sha]
			if !loaded || parent <= i {
				continue
			}
			lane := c.Lane
			if parentNumber > 0 {
				lane = commits[parent].Lane
				rows[i].outgoing = addLane(rows[i].outgoing, lane)
			}
			for j := i + 1; j < parent; j++ {
				rows[j].through = addLane(rows[j].through, lane)
			}
			rows[parent].incoming = addLane(rows[parent].incoming, lane)
		}
	}
	return rows
}

func laneColor(lane int) core.Color {
	colors := [...]uint32{0x4672a3, 0x795f91, 0x4d8176, 0x886f46, 0x986477, 0x58708d, 0x777c4c, 0x5777a4, 0x756a96}
	return core.RGB(colors[lane%len(colors)])
}

func drawGraph(d *core.Drawing, c commit, row graphRow, head bool, chrome palette) {
	laneX := func(lane int) float32 { return graphPadding + float32(lane)*laneGap }
	x, y := laneX(c.Lane), float32(historyRowHeight/2)
	color := laneColor(c.Lane)
	if head {
		color = chrome.accent
	}
	line := func(at, from, to float32, ink core.Color) {
		d.BeginPath().SetStrokeColor(ink).MoveTo(at, from).LineTo(at, to).Stroke()
	}
	d.SetLineWidth(1.5)
	for _, lane := range row.through {
		line(laneX(lane), 0, historyRowHeight, laneColor(lane))
	}
	for _, lane := range row.incoming {
		from := laneX(lane)
		if from == x {
			line(x, 0, y-nodeRadius, laneColor(lane))
		} else {
			d.BeginPath().SetStrokeColor(laneColor(lane)).MoveTo(from, 0).
				CubicTo(from, y/2, x, y/2, x, y-nodeRadius).Stroke()
		}
	}
	if row.stub {
		line(x, y+nodeRadius, historyRowHeight, color)
	}
	for _, lane := range row.outgoing {
		to := laneX(lane)
		d.BeginPath().SetStrokeColor(color).MoveTo(x, y+nodeRadius).
			CubicTo(x, historyRowHeight*.7, to, historyRowHeight*.7, to, historyRowHeight).Stroke()
	}
	radius := float32(nodeRadius)
	if head {
		radius++
	}
	d.BeginPath().Arc(x, y, radius, 0, 2*math.Pi, false).SetFillColor(color).Fill().
		SetStrokeColor(chrome.panel).Stroke()
	if head || len(c.Parents) > 1 {
		radius := float32(nodeRadius + 2)
		if head {
			radius++
		}
		d.BeginPath().Arc(x, y, radius, 0, 2*math.Pi, false).SetStrokeColor(color).Stroke()
	}
}

func (w *workbench) history(cx *core.Scope, chrome palette) core.Node {
	width := core.NewSignal(cx, core.Points(0))
	height := core.NewSignal(cx, core.Points(0))
	graphWidth := core.NewMemo(cx, func() float32 {
		lane := 0
		for _, c := range w.commits.Get() {
			lane = max(lane, c.Lane)
		}
		return float32(graphPadding*2 + laneGap*(lane+1))
	})
	list := core.List{
		Ref:           &w.historyView,
		EstimatedSize: historyRowHeight,
		Count:         func() int { return len(w.commits.Get()) },
		Key:           func(i int) any { return w.commits.Get()[i].SHA },
		Row: func(_ *core.Scope, row core.ListRow) core.Node {
			return w.historyRow(chrome, row, graphWidth.Get, width.Get)
		},
		OnLoadMore:        w.loadHistory,
		LoadMoreThreshold: 2,
		StyleFunc: func() core.Style {
			return core.Style{Width: core.Pt(max(width.Get(), core.Points(graphWidth.Get()+420))), Height: core.Pt(height.Get())}
		},
	}
	return core.View{
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
		Children: []core.Node{core.Show{
			When: func() bool { return len(w.commits.Get()) > 0 },
			Then: func(*core.Scope) core.Node {
				return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
					core.Show{
						When: func() bool { return w.historyError.Get() != "" },
						Then: func(*core.Scope) core.Node { return w.historyStatus(chrome) },
					},
					core.Scroll{
						Horizontal: true, HideIndicator: true,
						Style:    core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
						OnLayout: func(b core.Bounds) { width.Set(b.Size.Width); height.Set(b.Size.Height) },
						Children: []core.Node{list},
					},
				}}
			},
			Else: func(*core.Scope) core.Node { return w.historyStatus(chrome) },
		}},
	}
}

func (w *workbench) historyStatus(chrome palette) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		switch {
		case w.historyError.Get() != "":
			retry := func() {
				w.historyError.Set("")
				if w.core == nil {
					if path := w.path.Get(); path != "" {
						w.open(path)
					}
				} else {
					w.loadHistory()
				}
			}
			return notice(chrome, w.historyError.Get(), w.tr("Retry", "重试"), retry)
		case w.historyLoading.Get():
			return notice(chrome, w.tr("Loading history...", "正在加载历史…"), "", nil)
		case w.path.Get() == "":
			return notice(chrome, w.tr("Select a workspace", "选择工作区"), w.tr("Add local repository", "添加本地仓库"), w.pick)
		case w.filter != "":
			return notice(chrome, w.tr("No matching commits", "未找到匹配的提交"), w.tr("Clear search", "清除搜索"), func() {
				w.resetHistory("")
				w.loadHistory()
			})
		default:
			return notice(chrome, w.tr("No commits yet", "暂无提交"), "", nil)
		}
	})
}

func (w *workbench) historyRow(chrome palette, row core.ListRow, graphWidth func() float32, width func() core.Points) core.Node {
	at := func() commit {
		list := w.commits.Get()
		i := row.Index()
		if i < 0 || i >= len(list) {
			return commit{}
		}
		return list[i]
	}
	wide := func() bool { return width() >= 760 }
	column := func(value func() string, size core.Points, mono bool, shown func() bool) core.Node {
		style := core.Style{Width: core.Pt(size), FontSize: 11, Color: chrome.muted}
		if mono {
			style.FontFamily = "monospace"
		}
		return core.Text{ContentFunc: value, MaxLines: 1, Style: style, StyleFunc: func() core.Style {
			if shown != nil && !shown() {
				return core.Style{Display: core.DisplayNone}
			}
			return core.Style{}
		}}
	}
	return core.View{
		Role:      core.RoleButton,
		OnPress:   func(core.PressEvent) { w.selectCommit(at().SHA) },
		Selected:  func() bool { return w.selected.Get() == at().SHA },
		LabelFunc: func() string { return at().Message + ", " + at().Author + ", " + shortSHA(at().SHA) },
		TestID:    "commit-row",
		OnContextMenu: func(event core.ContextMenuEvent) {
			c := at()
			w.cx.PopupMenu([]core.MenuItem{
				{Label: w.tr("Copy SHA", "复制 SHA"), OnSelect: func() { w.cx.SetClipboard(c.SHA) }},
				{Label: w.tr("Copy commit message", "复制提交消息"), OnSelect: func() { w.cx.SetClipboard(c.Message) }},
			}, event.PagePosition, nil)
		},
		Style: core.Style{
			Direction: core.Row, Height: core.Pt(historyRowHeight), Gap: 8, AlignItems: core.AlignCenter,
			Borders: core.Edges{Left: 3}, BorderColor: core.Transparent,
			Overflow: core.Hidden, Hover: &core.Style{Background: chrome.hover},
			FocusVisible: &core.Style{OutlineColor: chrome.accent, OutlineWidth: 1, OutlineOffset: -1},
		},
		StyleFunc: func() core.Style {
			if w.selected.Get() == at().SHA {
				return core.Style{Background: chrome.selected, BorderColor: chrome.accent}
			}
			return core.Style{}
		},
		Children: []core.Node{
			core.Canvas{
				StyleFunc: func() core.Style {
					return core.Style{Width: core.Pt(core.Points(graphWidth())), Height: core.Pt(historyRowHeight)}
				},
				Draw: func(d *core.Drawing, _ core.Size) {
					c := at()
					art := w.art.Get()
					if i := row.Index(); i >= 0 && i < len(art) && c.SHA != "" {
						drawGraph(d, c, art[i], c.SHA == w.repo.Get().Head, chrome)
					}
				},
			},
			core.Text{
				ContentFunc: func() string { return at().Message }, MaxLines: 1,
				Style: core.Style{FontSize: 13, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Color: chrome.text},
			},
			core.Text{
				ContentFunc: func() string { return refLabels(at().Refs) }, MaxLines: 1,
				Style: core.Style{Width: core.Pct(20), FontSize: 11, Color: chrome.accent},
			},
			column(func() string { return shortSHA(at().SHA) }, 64, true, func() bool { return width() >= 512 }),
			column(func() string { return at().Author }, 112, false, wide),
			column(func() string { return w.relativeTime(at().Time) }, 64, false, wide),
		},
	}
}

func shortSHA(sha string) string { return sha[:min(len(sha), 7)] }

func refLabels(refs []commitRef) string {
	if len(refs) == 0 {
		return ""
	}
	priority := func(kind string) int {
		switch kind {
		case "tag":
			return 0
		case "local_branch":
			return 1
		default:
			return 2
		}
	}
	sorted := slices.Clone(refs)
	slices.SortStableFunc(sorted, func(a, b commitRef) int { return cmp.Compare(priority(a.Kind), priority(b.Kind)) })
	names := []string{sorted[0].Name}
	if len(sorted) > 1 {
		names = append(names, "+"+strconv.Itoa(len(sorted)-1))
	}
	return strings.Join(names, "  ")
}

func (w *workbench) relativeTime(stamp int64) string {
	if stamp <= 0 {
		return ""
	}
	when := time.Unix(stamp, 0)
	elapsed := time.Since(when)
	switch {
	case elapsed < time.Minute:
		return w.tr("now", "刚刚")
	case elapsed < time.Hour:
		return strconv.Itoa(int(elapsed/time.Minute)) + w.tr("m", " 分钟")
	case elapsed < 24*time.Hour:
		return strconv.Itoa(int(elapsed/time.Hour)) + w.tr("h", " 小时")
	case elapsed < 7*24*time.Hour:
		return strconv.Itoa(int(elapsed/(24*time.Hour))) + w.tr("d", " 天")
	default:
		return when.Format(w.tr("1/2/06", "2006/1/2"))
	}
}
