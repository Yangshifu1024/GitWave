package main

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

const (
	historyRowHeight = 28
	historyRowPad    = 8
	historyRowBorder = 3
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
		stroke := color.Faded(.5)
		d.SetLineWidth(1)
		if head {
			radius++
			stroke = color
			d.SetLineWidth(1.5)
		}
		d.BeginPath().Arc(x, y, radius, 0, 2*math.Pi, false).SetStrokeColor(stroke).Stroke()
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
		return float32(graphPadding*2 + laneGap*lane)
	})
	list := core.List{
		TestID:        "history-commit-list",
		Ref:           &w.historyView,
		EstimatedSize: historyRowHeight,
		ContentOffset: core.Point{Y: w.historyOffset.Y},
		Count:         func() int { return len(w.commits.Get()) },
		Key:           func(i int) any { return w.commits.Get()[i].SHA },
		Row: func(cx *core.Scope, row core.ListRow) core.Node {
			return w.historyRow(cx, chrome, row, graphWidth.Get, width.Get)
		},
		Footer:            w.historyFooter(chrome),
		OnLoadMore:        w.loadHistory,
		LoadMoreThreshold: 2,
		OnLayout: func(core.Bounds) {
			if w.locate != "" {
				cx.Post(w.finishLocate)
			}
		},
		OnScroll: func(e core.ScrollEvent) {
			if !e.Stale {
				w.historyOffset.Y = max(0, e.Offset.Y)
			}
		},
		StyleFunc: func() core.Style {
			span := core.Pt(max(width.Get(), core.Points(graphWidth.Get()+420)))
			return core.Style{Width: span, MinWidth: span, Height: core.Pt(height.Get())}
		},
	}
	return core.View{
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
		Children: []core.Node{core.Show{
			When: func() bool { return len(w.commits.Get()) > 0 },
			Then: func(graphScope *core.Scope) core.Node {
				restoringX := w.historyOffset.X != 0
				return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
					core.Show{
						When: func() bool { return w.historyError.Get() != "" },
						Then: func(*core.Scope) core.Node { return w.historyStatus(chrome) },
					},
					core.Scroll{
						TestID: "history-graph-scroll", Ref: &w.historyGraphView, Horizontal: true, HideIndicator: true,
						Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
						OnLayout: func(b core.Bounds) {
							width.Set(b.Size.Width)
							height.Set(b.Size.Height)
							if w.locate != "" {
								graphScope.Post(w.finishLocate)
							}
							if b.Size.Width > 0 && restoringX {
								// Restore after the wider list's content size is committed.
								graphScope.Post(func() {
									if restoringX {
										restoringX = false
										w.historyGraphView.ScrollTo(core.Point{X: w.historyOffset.X}, false)
									}
								})
							}
						},
						OnScroll: func(e core.ScrollEvent) {
							if !e.Stale && !restoringX {
								w.historyOffset.X = max(0, e.Offset.X)
							}
						},
						Children: []core.Node{list},
					},
				}}
			},
			Else: func(*core.Scope) core.Node { return w.historyStatus(chrome) },
		}},
	}
}

func (w *workbench) revealHistoryLane(lane int) {
	frame, ok := w.historyGraphView.Frame()
	if !ok || frame.Size.Width <= 0 {
		return
	}
	center := core.Points(historyRowPad + historyRowBorder + graphPadding + lane*laneGap)
	margin := core.Points(graphPadding + nodeRadius + 4)
	left := w.historyOffset.X
	if center-margin < left {
		left = max(0, center-margin)
	} else if center+margin > left+frame.Size.Width {
		left = center + margin - frame.Size.Width
	}
	if left != w.historyOffset.X {
		w.historyOffset.X = left
		w.historyGraphView.ScrollTo(core.Point{X: left}, false)
	}
}

func (w *workbench) historyFooter(chrome palette) core.Node {
	return core.Show{
		When: func() bool { return w.historyLoading.Get() || !w.more.Get() },
		Then: func(*core.Scope) core.Node {
			return core.Text{TestID: "history-footer", ContentFunc: func() string {
				if w.historyLoading.Get() {
					return w.tr("Loading older commits…", "正在加载更早的提交…")
				}
				label := w.tr("End of history · %d commits", "已到历史末尾 · 共 %d 个提交")
				if w.snapshotLimit.Get() != 0 {
					label = w.tr("End of loaded range · %d commits", "已到加载范围末尾 · 共 %d 个提交")
				}
				return fmt.Sprintf(label, len(w.commits.Get()))
			}, Style: core.Style{FontSize: chrome.uiSize(10), Color: chrome.textMuted, TextAlign: core.TextAlignCenter,
				Padding: core.Edges{Top: 8, Bottom: 8}}}
		},
	}
}

func (w *workbench) historyStatus(chrome palette) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		switch {
		case w.historyError.Get() != "":
			retry := func() {
				if w.core == nil {
					w.queueRefresh(refreshManual)
				} else {
					w.refreshRepository(true)
				}
			}
			return notice(chrome, w.historyError.Get(), w.tr("Retry", "重试"), retry)
		case w.historyLoading.Get():
			return notice(chrome, w.tr("Loading history...", "正在加载历史…"), "", nil)
		case w.workspaceFailed:
			return notice(chrome, w.workspaceError.Get(), "", nil)
		case w.path.Get() == "" && w.workspaceBusy.Get():
			return notice(chrome, w.tr("Loading workspaces…", "正在加载工作区…"), "", nil)
		case w.workspaceReload.Get():
			return notice(chrome, w.tr("Could not load workspaces", "无法加载工作区"), w.tr("Retry", "重试"), func() {
				w.queueRefresh(refreshManual)
			})
		case w.path.Get() == "" && w.workspaceState.Get().Workspace.ID != "":
			return workbenchEmptyState(chrome, nil, w.tr("No repository selected", "未选择仓库"),
				w.tr("To view history and diffs, select a repository tab or add a repository from the Repository menu.",
					"选择仓库标签，或从「仓库」菜单添加仓库，即可查看历史和 diff。"))
		case w.path.Get() == "":
			return workbenchEmptyState(chrome, nil, w.tr("Select a workspace", "选择工作区"),
				w.tr("Choose or create a workspace from the Workspaces menu to open repositories.", "从「工作区」菜单选择或创建工作区，即可打开仓库。"))
		default:
			return notice(chrome, w.tr("No commits yet", "暂无提交"), "", nil)
		}
	})
}

func (w *workbench) historyRow(cx *core.Scope, chrome palette, row core.ListRow, graphWidth func() float32, width func() core.Points) core.Node {
	at := func() commit {
		list := w.commits.Get()
		i := row.Index()
		if i < 0 || i >= len(list) {
			return commit{}
		}
		return list[i]
	}
	wide := func() bool { return width() >= 672 }
	isHead := func() bool { return at().SHA != "" && at().SHA == w.repo.Get().Head }
	selected := func() bool { return w.selected.Get() == at().SHA }
	selectRow := func(core.PressEvent) {
		w.locate = ""
		w.selectCommit(at().SHA)
	}
	column := func(value func() string, size core.Points, mono bool, align core.TextAlign, shown func() bool) core.Node {
		style := core.Style{Width: core.Pt(size), FontSize: chrome.uiSize(11), Color: chrome.textMuted, TextAlign: align}
		if mono {
			style.FontFamily, style.FontSize = chrome.fonts.code, chrome.codeSize(11)
		}
		return core.Text{ContentFunc: value, MaxLines: 1, Style: style, StyleFunc: func() core.Style {
			if shown != nil && !shown() {
				return core.Style{Display: core.DisplayNone}
			}
			return core.Style{}
		}}
	}
	commitMenu := func(c commit, at core.Point) {
		selectRow(core.PressEvent{})
		copyText := func(text string) {
			w.cx.SetClipboard(text)
			w.refNotice.Set(referenceNotice{text: w.tr("Copied to clipboard", "已复制到剪贴板")})
		}
		items := []core.MenuItem{
			{Label: w.tr("Copy SHA", "复制 SHA"), OnSelect: func() { copyText(c.SHA) }},
			{Label: w.tr("Copy Commit Info", "复制提交信息"), OnSelect: func() {
				locale := strings.ToLower(w.systemLocale)
				layout := "1/2/2006, 3:04:05 PM"
				switch {
				case strings.HasPrefix(locale, "zh"):
					layout = "2006/1/2 15:04:05"
				case strings.HasPrefix(locale, "en-gb"):
					layout = "02/01/2006, 15:04:05"
				}
				date := time.Unix(c.Time, 0).Format(layout)
				copyText(fmt.Sprintf("%s\nAuthor: %s\nDate: %s\n\n%s", c.SHA, c.Author, date, c.Message))
			}},
			{Label: w.tr("Explain with AI", "用 AI 解释"), Disabled: w.workspaceStore == nil || w.workspaceState.Get().Workspace.ID == "",
				OnSelect: w.scopedRefAction(func() { w.openCommitExplanation(c.SHA, c.Message) })},
			{Separator: true},
			{Label: w.tr("Checkout Commit", "检出提交"), Disabled: c.SHA == "" || c.SHA == w.repo.Get().Head || w.currentChangesWrite().pending,
				OnSelect: w.scopedRefAction(func() { w.checkoutCommit(c.SHA) })},
			{Label: w.tr("New Branch…", "新建分支…"), Disabled: c.SHA == "" || w.currentChangesWrite().pending,
				OnSelect: w.scopedRefAction(func() {
					w.openRefForm(refDialogState{kind: createBranchForm, base: c.SHA, baseLabel: shortSHA(c.SHA)})
				})},
			{Label: w.tr("Tag…", "标签…"), Disabled: c.SHA == "" || w.currentChangesWrite().pending,
				OnSelect: w.scopedRefAction(func() { w.showTagManager(c.SHA) })},
		}
		w.cx.PopupMenu(append(items, w.commitReplayItems(c.SHA, c.Message, true)...), at, nil)
	}
	rowRef := new(core.Ref)
	return core.View{
		Role:          core.RoleButton,
		Ref:           rowRef,
		OnPress:       selectRow,
		Selected:      selected,
		LabelFunc:     func() string { return at().Message + ", " + at().Author + ", " + shortSHA(at().SHA) },
		TestID:        "commit-row",
		OnContextMenu: func(event core.ContextMenuEvent) { commitMenu(at(), event.PagePosition) },
		OnKeyDown: func(e core.KeyEvent) bool {
			return refMenuKey(rowRef, e, func(point core.Point) { commitMenu(at(), point) })
		},
		Style: core.Style{
			Direction: core.Row, Height: core.Pt(historyRowHeight), Gap: 8, AlignItems: core.AlignCenter,
			Padding: core.Edges{Left: historyRowPad, Right: historyRowPad},
			Borders: core.Edges{Left: historyRowBorder}, BorderColor: core.Transparent,
			Overflow: core.Hidden, Hover: &core.Style{Background: chrome.hover},
			FocusVisible: &core.Style{OutlineColor: chrome.accent, OutlineWidth: 1, OutlineOffset: -1},
		},
		StyleFunc: func() core.Style {
			if selected() {
				return core.Style{Background: chrome.selected, BorderColor: chrome.accent,
					Hover: &core.Style{Background: chrome.selected}}
			}
			if isHead() {
				return core.Style{BorderColor: chrome.accent}
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
				Style: core.Style{FontSize: chrome.uiSize(12), FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Color: chrome.text},
				StyleFunc: func() core.Style {
					if isHead() || selected() {
						return core.Style{FontWeight: 500}
					}
					return core.Style{}
				},
			},
			w.historyReferenceCell(cx, chrome, at, isHead, func() core.Points {
				graph := core.Points(graphWidth())
				return (max(width(), graph+420) - graph - 27) * .2
			}),
			column(func() string { return shortSHA(at().SHA) }, 64, true, core.TextAlignStart, func() bool { return width() >= 512 }),
			column(func() string { return at().Author }, 112, false, core.TextAlignStart, wide),
			column(func() string { return w.relativeTime(at().Time) }, 64, false, core.TextAlignEnd, wide),
		},
	}
}

func shortSHA(sha string) string { return sha[:min(len(sha), 7)] }

func (w *workbench) relativeTime(stamp int64) string {
	if stamp <= 0 {
		return ""
	}
	when := time.Unix(stamp, 0)
	elapsed := time.Since(when)
	switch {
	case elapsed < time.Minute:
		return w.tr("just now", "刚刚")
	case elapsed < time.Hour:
		return strconv.Itoa(int(elapsed/time.Minute)) + w.tr("m ago", " 分钟前")
	case elapsed < 24*time.Hour:
		return strconv.Itoa(int(elapsed/time.Hour)) + w.tr("h ago", " 小时前")
	case elapsed < 7*24*time.Hour:
		return strconv.Itoa(int(elapsed/(24*time.Hour))) + w.tr("d ago", " 天前")
	default:
		return when.Format(w.tr("1/2/2006", "2006/1/2"))
	}
}
