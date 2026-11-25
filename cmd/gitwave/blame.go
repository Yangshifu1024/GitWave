package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
)

const (
	blameRowHeight       = 20
	blameNarrowGutter    = 192
	blameWideGutter      = 320
	blameLineNumberWidth = 48
)

type blameState struct {
	path    string
	deleted bool
	lines   []gitcli.BlameLine
	loading bool
	err     error
}

func (w *workbench) showBlame(file fileDiff) {
	w.loadBlame(file.Path, file.NewSHA == "")
}

func (w *workbench) loadBlame(path string, deleted bool) {
	if w.core == nil || path == "" {
		return
	}
	source := path
	commit := w.details.Get()
	if w.changesMode.Get() {
		item := w.changeFile.Get()
		if path != item.key.path {
			return
		}
		head := w.changes.Get().status.HeadOID
		if head == "" || head == "(initial)" || item.kind() == '?' || item.change.Index == 'A' {
			w.closeBlame()
			w.blame.Set(blameState{path: path})
			return
		}
		commit = gitcli.Commit{SHA: head}
		deleted = false
		if (item.change.Index == 'R' || item.kind() == 'R') && item.change.PreviousPath != "" {
			source = item.change.PreviousPath
		}
	} else if path != w.file.Get() || commit.SHA == "" {
		return
	}
	if w.stopBlame != nil {
		w.stopBlame()
	}
	ctx, stop := context.WithCancel(w.repoCtx)
	w.stopBlame = stop
	w.blame.Set(blameState{path: path, deleted: deleted, loading: true})
	repo := w.core.repo
	go func() {
		lines, err := repo.Blame(ctx, commit, source, deleted)
		w.cx.Post(func() {
			defer stop()
			if ctx.Err() != nil {
				return
			}
			w.blame.Set(blameState{path: path, deleted: deleted, lines: lines, err: err})
		})
	}()
}

func (w *workbench) closeBlame() {
	if w.stopBlame != nil {
		w.stopBlame()
		w.stopBlame = nil
	}
	if w.blame.Get().path != "" {
		w.blame.Set(blameState{})
	}
}

func (w *workbench) blamePanel(cx *core.Scope, chrome palette, state blameState) core.Node {
	var body core.Node
	switch {
	case state.loading:
		body = notice(chrome, w.tr("Loading blame…", "正在加载 blame…"), "", nil)
	case errors.Is(state.err, gitcli.ErrBlameTooLarge):
		body = notice(chrome, w.tr("File exceeds the blame preview limit", "文件超出 blame 预览上限"), "", nil)
	case state.err != nil:
		body = notice(chrome, state.err.Error(), w.tr("Retry", "重试"), func() {
			w.loadBlame(state.path, state.deleted)
		})
	case len(state.lines) == 0:
		body = notice(chrome, w.tr("No blame information available", "暂无 blame 信息"), "", nil)
	default:
		body = w.blameLines(cx, chrome, state)
	}
	return core.View{TestID: "blame-panel", Style: core.Style{FlexGrow: 1, MinHeight: core.Pt(0)},
		Children: []core.Node{
			bar(chrome, 36, chrome.elevated,
				action(chrome, w.tr("Back to diff", "返回 diff"), w.closeBlame),
				core.Text{Content: state.path, MaxLines: 1,
					Style: core.Style{Color: chrome.muted, FontSize: chrome.uiSize(12), FlexShrink: 1, MinWidth: core.Pt(0)}}),
			body,
		}}
}

func (w *workbench) blameLines(cx *core.Scope, chrome palette, state blameState) core.Node {
	size := core.NewSignal(cx, core.Size{})
	wide := core.NewMemo(cx, func() bool { return size.Get().Width >= 512 })
	offset := core.NewAnimatedValue(cx, 0)
	maxColumns := 0
	for _, line := range state.lines {
		maxColumns = max(maxColumns, codeColumns(line.Content))
	}
	rowHeight := chrome.codeSize(blameRowHeight)
	lineNumberWidth := chrome.codeSize(blameLineNumberWidth)
	cellWidth := chrome.codeCellWidth(cx)
	list := core.List{
		TestID: "blame-lines", Count: func() int { return len(state.lines) }, EstimatedSize: rowHeight,
		StyleFunc: func() core.Style {
			gutter := blameNarrowGutter
			if wide.Get() {
				gutter = blameWideGutter
			}
			content := size.Get()
			width := core.Points(gutter+24) + lineNumberWidth + core.Points(maxColumns)*cellWidth
			return core.Style{MinWidth: core.Pt(max(content.Width, width)), Height: core.Pt(content.Height)}
		},
		Row: func(_ *core.Scope, row core.ListRow) core.Node {
			return core.Dynamic(func(*core.Scope) core.Node {
				line := state.lines[row.Index()]
				return core.View{
					Style: core.Style{Direction: core.Row, Height: core.Pt(rowHeight),
						Hover: &core.Style{Background: chrome.hover}},
					Children: []core.Node{
						w.blameGutter(chrome, line, wide.Get, offset),
						core.Text{Content: line.Content, MaxLines: 1, Selectable: core.SelectableOn, TestID: "blame-line-content",
							Style: core.Style{FontSize: chrome.codeSize(12), FontFamily: chrome.fonts.code, Color: chrome.text,
								Height: core.Pt(rowHeight), Padding: core.Edges{Left: 12, Right: 12}, FlexShrink: 0}},
					},
				}
			})
		},
	}
	return core.Scroll{TestID: "blame-scroll", Horizontal: true, OffsetValues: core.AnimatedXY{X: offset},
		OnLayout: func(b core.Bounds) { size.Set(b.Size) },
		Style:    core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
		Children: []core.Node{list}}
}

func (w *workbench) blameGutter(chrome palette, line gitcli.BlameLine, wide func() bool, offset core.AnimatedNode) core.Node {
	date := time.Unix(line.Time, 0).Format(w.tr("Jan 2, 2006", "2006年1月2日"))
	label := fmt.Sprintf(w.tr("View commit %s", "查看提交 %s"), shortSHA(line.SHA))
	return core.View{
		Style: core.Style{Direction: core.Row, FlexShrink: 0, Height: core.Pt(chrome.codeSize(blameRowHeight))},
		StyleFunc: func() core.Style {
			if wide() {
				return core.Style{ZIndex: 1, Animated: &core.AnimatedStyle{TranslateX: offset}}
			}
			return core.Style{}
		},
		Children: []core.Node{
			core.View{
				Role: core.RoleButton, Label: label,
				OnPress: func(core.PressEvent) {
					w.closeBlame()
					w.locateCommit(line.SHA)
					w.selectCommit(line.SHA)
				},
				Tooltip: core.Text{Content: fmt.Sprintf("%s\n%s <%s>\n%s", line.SHA, line.Author, line.AuthorEmail, date),
					Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.text}},
				Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
					Padding: core.Edges{Left: 8, Right: 8}, Width: core.Pt(blameNarrowGutter), FlexShrink: 0, Overflow: core.Hidden,
					Background: chrome.panel, Borders: core.Edges{Right: 1}, BorderColor: chrome.border,
					FocusVisible: &core.Style{OutlineWidth: 1, OutlineColor: chrome.accent, OutlineOffset: -1}},
				StyleFunc: func() core.Style {
					if wide() {
						return core.Style{Width: core.Pt(blameWideGutter)}
					}
					return core.Style{}
				},
				Children: []core.Node{
					core.Text{Content: shortSHA(line.SHA), MaxLines: 1,
						Style: core.Style{FontSize: chrome.codeSize(12), FontFamily: chrome.fonts.code, Color: chrome.accent, FlexShrink: 0}},
					core.Text{Content: line.Author, MaxLines: 1,
						Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.muted, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}},
					core.Show{When: wide, Then: func(*core.Scope) core.Node {
						return chrome.label(date, chrome.muted, 10)
					}},
				},
			},
			core.Text{Content: strconv.Itoa(line.LineNo), MaxLines: 1, Style: core.Style{
				Width: core.Pt(chrome.codeSize(blameLineNumberWidth)), FlexShrink: 0, FontSize: chrome.codeSize(12), FontFamily: chrome.fonts.code,
				Color: chrome.muted, TextAlign: core.TextAlignRight, Padding: core.Edges{Right: 8},
				Background: chrome.elevated, Borders: core.Edges{Right: 1}, BorderColor: chrome.border,
			}},
		},
	}
}
