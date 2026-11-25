package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/base/textview"
	"github.com/nsgo-dev/nsgo/base/textview/highlight"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

type diffRow struct {
	header      string
	left, right *diffLine
	show        string
}

const (
	diffLineNumberWidth = 39
	diffSignWidth       = 15
)

// Keep replacement lines paired when switching views, even when one
// side has more lines. The unified view shows deletions before additions.
func diffRows(hunks []diffHunk, split bool) []diffRow {
	var rows []diffRow
	for _, hunk := range hunks {
		rows = append(rows, diffRow{header: fmt.Sprintf("@@ -%d,%d +%d,%d @@",
			hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines)})
		for i := 0; i < len(hunk.Lines); {
			line := &hunk.Lines[i]
			if line.Kind == "context" {
				rows = append(rows, diffRow{left: line, right: line})
				i++
				continue
			}
			var removed, added []*diffLine
			for i < len(hunk.Lines) && hunk.Lines[i].Kind != "context" {
				line = &hunk.Lines[i]
				if line.Kind == "removed" {
					removed = append(removed, line)
				} else {
					added = append(added, line)
				}
				i++
			}
			at := func(lines []*diffLine, i int) *diffLine {
				if i < len(lines) {
					return lines[i]
				}
				return nil
			}
			if split {
				for j := range max(len(removed), len(added)) {
					rows = append(rows, diffRow{left: at(removed, j), right: at(added, j)})
				}
				continue
			}
			for j, left := range removed {
				rows = append(rows, diffRow{left: left, right: at(added, j), show: "left"})
			}
			for j, right := range added {
				rows = append(rows, diffRow{left: at(removed, j), right: right, show: "right"})
			}
		}
	}
	return rows
}

func changedRange(line, other string) (int, int) {
	start, end, otherEnd := 0, len(line), len(other)
	for start < end && start < otherEnd {
		a, size := utf8.DecodeRuneInString(line[start:])
		b, _ := utf8.DecodeRuneInString(other[start:])
		if a != b {
			break
		}
		start += size
	}
	for end > start && otherEnd > start {
		a, size := utf8.DecodeLastRuneInString(line[:end])
		b, otherSize := utf8.DecodeLastRuneInString(other[:otherEnd])
		if a != b {
			break
		}
		end -= size
		otherEnd -= otherSize
	}
	return start, end
}

func (w *workbench) diffPanel(chrome palette) core.Node {
	return core.Dynamic(func(cx *core.Scope) core.Node {
		path := w.file.Get()
		if blame := w.blame.Get(); path != "" && blame.path == path {
			return w.blamePanel(cx, chrome, blame)
		}
		switch {
		case path == "":
			return notice(chrome, w.tr("No changed files", "没有变更文件"), "", nil)
		case w.diffLoading.Get():
			return notice(chrome, w.tr("Loading diff…", "正在加载 diff…"), "", nil)
		case w.diffError.Get() != "":
			return notice(chrome, w.diffError.Get(), w.tr("Retry", "重试"), func() {
				w.selectFile(path, w.expanded.Get())
			})
		}
		return w.fileDiffView(chrome, w.diff.Get(), fileDiffOptions{
			collapsed: w.diffCollapsed, expanded: w.expanded.Get(),
			viewport: &w.historyDiffViewport,
			expand:   func() { w.selectFile(path, true) }, blame: w.showBlame,
		})
	})
}

type fileDiffOptions struct {
	chip       string
	title      string
	collapsed  *core.Signal[bool]
	expanded   bool
	editorHint bool
	expand     func()
	blame      func(fileDiff)
	viewport   *diffViewport
}

type diffViewport struct {
	offset    core.Point
	firstHunk *diffHunk
	font      core.Font
	textWidth core.Points
}

func (w *workbench) fileDiffView(chrome palette, result preview, opts fileDiffOptions) core.Node {
	if len(result.Files) == 0 {
		return notice(chrome, w.tr("No diff available", "暂无 diff"), "", nil)
	}
	file := result.Files[0]
	split := w.splitDiff.Get()
	children := []core.Node{w.diffToolbar(chrome, file, result.TooLarge, split, opts)}
	if result.Truncated {
		message := w.tr("Preview truncated to keep scrolling responsive.", "预览已截断，以保持滚动流畅。")
		if result.TooLarge {
			message = w.tr("This file exceeds the preview size limit.", "该文件超过预览大小限制。")
		}
		line := []core.Node{core.Text{Content: message, Role: core.RoleStatus,
			Style: core.Style{FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(220), MinWidth: core.Pt(0),
				FontSize: chrome.uiSize(12), Color: chrome.muted}}}
		if !opts.expanded {
			line = append(line, action(chrome, w.tr("Load a larger preview", "加载更多内容"), opts.expand))
		} else {
			limit := w.tr("Preview limit reached.", "已达到预览上限。")
			if opts.editorHint {
				limit += w.tr(" Open the file in your editor to inspect the full content.", "请在编辑器中打开文件查看完整内容。")
			}
			line = append(line, core.Text{Content: limit,
				Style: core.Style{FlexShrink: 1, MinWidth: core.Pt(0), FontSize: chrome.uiSize(12), Color: chrome.muted}})
		}
		children = append(children, core.View{TestID: "diff-preview-notice", Style: core.Style{
			Direction: core.Row, Wrap: core.Wrap, AlignItems: core.AlignCenter, Gap: 6,
			Padding: core.All(8), Background: chrome.hunk,
		}, Children: line})
	}
	children = append(children, core.Show{
		When: func() bool { return !opts.collapsed.Get() },
		Then: func(cx *core.Scope) core.Node {
			if isImagePath(file.Path) {
				return w.imageDiff(cx, chrome, file)
			}
			if len(file.Hunks) == 0 {
				return notice(chrome, w.tr("No text preview (binary, metadata-only, or preview limit).",
					"无文本预览（可能为二进制、仅元数据变化或已达预览上限）。"), "", nil)
			}
			return w.diffText(cx, chrome, file, split, opts.viewport)
		},
	})
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), MinHeight: core.Pt(0)}, Children: children}
}

func (w *workbench) diffToolbar(chrome palette, file fileDiff, tooLarge, split bool, opts fileDiffOptions) core.Node {
	label := w.tr("Switch to split view", "切换到分栏视图")
	modeIcon := lucide.Columns2()
	if split {
		label = w.tr("Switch to unified view", "切换到统一视图")
		modeIcon = lucide.Square()
	}
	title := opts.title
	if title == "" {
		title = file.Path
	}
	items := []core.Node{core.Text{Content: title, MaxLines: 1,
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), FontSize: chrome.uiSize(12), Color: chrome.muted}}}
	if opts.chip != "" {
		items = append(items, chrome.label(opts.chip, chrome.accent, 10))
	}
	if !tooLarge {
		items = append(items, chrome.label(fmt.Sprintf("+%d", file.Additions), core.RGB(0x2e8c4c), 13),
			chrome.label(fmt.Sprintf("-%d", file.Deletions), core.RGB(0xc64c55), 13))
	}
	meta := core.View{Style: core.Style{
		Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6,
		FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(120), MinWidth: core.Pt(0),
	}, Children: items}
	var controls []core.Node
	if opts.blame != nil {
		controls = append(controls, action(chrome, "Blame", func() { opts.blame(file) }))
	}
	controls = append(controls, core.Dynamic(func(*core.Scope) core.Node {
		name := w.tr("Collapse file diff", "折叠文件 diff")
		icon := lucide.FoldVertical()
		if opts.collapsed.Get() {
			name = w.tr("Expand file diff", "展开文件 diff")
			icon = lucide.UnfoldVertical()
		}
		return iconAction(chrome, name, icon, func() { opts.collapsed.Set(!opts.collapsed.Get()) })
	}), iconAction(chrome, label, modeIcon, func() { w.splitDiff.Set(!split) }))
	return core.View{TestID: "file-diff-toolbar", Style: core.Style{
		Direction: core.Row, Wrap: core.Wrap, AlignItems: core.AlignCenter, JustifyContent: core.JustifyEnd,
		MinHeight: core.Pt(36), Padding: core.Edges{Top: 3, Bottom: 3, Left: 8, Right: 8}, Gap: 6,
		Background: chrome.elevated, Borders: core.Edges{Bottom: 1}, BorderColor: chrome.border,
	}, Children: []core.Node{meta, core.View{Style: core.Style{
		Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4,
	}, Children: controls}}}
}

func (w *workbench) diffText(cx *core.Scope, chrome palette, file fileDiff, split bool, viewport *diffViewport) core.Node {
	rows := diffRows(file.Hunks, split)
	width := core.NewSignal(cx, core.Points(0))
	height := core.NewSignal(cx, core.Points(0))
	// Shape the longest tabbed and untabbed candidates up front. Visible
	// rows can widen the viewport if a fallback glyph needs more room.
	type sample struct {
		text    string
		columns int
	}
	var plain, tabbed sample
	for _, hunk := range file.Hunks {
		for _, line := range hunk.Lines {
			candidate := &plain
			if strings.ContainsRune(line.Content, '\t') {
				candidate = &tabbed
			}
			if columns := codeColumns(line.Content); columns > candidate.columns {
				*candidate = sample{line.Content, columns}
			}
		}
	}
	font := core.Font{Family: chrome.fonts.code, Size: chrome.codeSize(12)}
	initial := core.Points(max(cx.MeasureText(plain.text, font).Width, cx.MeasureText(tabbed.text, font).Width))
	firstHunk := &file.Hunks[0]
	if viewport != nil {
		if viewport.firstHunk == firstHunk && viewport.font == font {
			initial = max(initial, viewport.textWidth)
		}
		viewport.firstHunk, viewport.font, viewport.textWidth = firstHunk, font, initial
	}
	textWidth := core.NewSignal(cx, initial)
	reportWidth := func(bounds core.Bounds) {
		if bounds.Size.Width > textWidth.Get() {
			textWidth.Set(bounds.Size.Width)
			if viewport != nil && viewport.firstHunk == firstHunk && viewport.font == font {
				viewport.textWidth = max(viewport.textWidth, bounds.Size.Width)
			}
		}
	}
	lexer := highlight.New(cx)
	language := diffLanguage(file.Path)
	if len(rows) > 500 {
		language = ""
	}
	rowHeight := chrome.codeSize(20)
	list := core.List{
		TestID: "diff-line-list",
		Count:  func() int { return len(rows) }, EstimatedSize: rowHeight,
		StyleFunc: func() core.Style {
			contentWidth := textWidth.Get() + chrome.codeSize(8)
			minWidth := chrome.codeSize(2*diffLineNumberWidth+diffSignWidth) + contentWidth
			if split {
				minWidth = 2 * (chrome.codeSize(diffLineNumberWidth+diffSignWidth) + contentWidth + 1)
			}
			span := core.Pt(max(width.Get(), minWidth))
			return core.Style{Width: span, MinWidth: span, Height: core.Pt(height.Get())}
		},
		Row: func(_ *core.Scope, row core.ListRow) core.Node {
			return core.Dynamic(func(*core.Scope) core.Node {
				current := rows[row.Index()]
				if current.header != "" {
					return core.View{Style: core.Style{Height: core.Pt(rowHeight), Background: chrome.hunk,
						JustifyContent: core.JustifyCenter, Padding: core.Edges{Left: 8}},
						Children: []core.Node{core.Text{Content: current.header, Selectable: core.SelectableOn,
							Style: core.Style{FontSize: chrome.codeSize(11), FontFamily: chrome.fonts.code, Color: chrome.muted}}},
					}
				}
				if split {
					return core.View{Style: core.Style{Direction: core.Row, Height: core.Pt(rowHeight)}, Children: []core.Node{
						diffCell(chrome, current.left, current.right, true, false, language, lexer, reportWidth),
						diffCell(chrome, current.right, current.left, true, true, language, lexer, reportWidth),
					}}
				}
				if current.show == "left" {
					return diffCell(chrome, current.left, current.right, false, false, language, lexer, reportWidth)
				}
				return diffCell(chrome, current.right, current.left, false, true, language, lexer, reportWidth)
			})
		},
	}
	var horizontal core.Ref
	var restoreX core.Points
	outer := core.Scroll{
		TestID: "diff-horizontal-scroll", Ref: &horizontal, Horizontal: true,
		OnLayout: func(b core.Bounds) {
			width.Set(b.Size.Width)
			height.Set(b.Size.Height)
			if b.Size.Width > 0 && restoreX > 0 {
				target := core.Point{X: restoreX}
				restoreX = 0
				// The horizontal content size is committed after this layout callback.
				cx.Post(func() { horizontal.ScrollTo(target, false) })
			}
		},
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
	}
	if viewport != nil {
		restoreX = viewport.offset.X
		outer.OnScroll = func(e core.ScrollEvent) {
			if !e.Stale {
				viewport.offset.X = e.Offset.X
			}
		}
		list.ContentOffset = core.Point{Y: viewport.offset.Y}
		list.OnScroll = func(e core.ScrollEvent) {
			if !e.Stale {
				viewport.offset.Y = e.Offset.Y
			}
		}
	}
	outer.Children = []core.Node{list}
	return outer
}

func diffCell(chrome palette, line, other *diffLine, split, right bool, language string, highlight textview.Highlighter,
	reportWidth func(core.Bounds)) core.Node {
	style := core.Style{Direction: core.Row, Height: core.Pt(chrome.codeSize(20)), AlignItems: core.AlignCenter, Background: chrome.panel}
	if split {
		style.FlexBasis, style.FlexGrow, style.MinWidth = core.Pt(0), 1, core.Pt(0)
		if right {
			style.Borders, style.BorderColor = core.Edges{Left: 1}, chrome.border
		}
	}
	if line == nil {
		style.Background = chrome.elevated
		return core.View{Style: style}
	}
	var ink core.Color
	sign := " "
	switch line.Kind {
	case "added":
		style.Background, ink, sign = chrome.add, chrome.addWord, "+"
	case "removed":
		style.Background, ink, sign = chrome.remove, chrome.removeWord, "-"
	}
	lineNo := func(n int) core.Node {
		text := ""
		if n > 0 {
			text = strconv.Itoa(n)
		}
		return core.Text{Content: text, Selectable: core.SelectableOff, MaxLines: 1,
			Style: core.Style{Width: core.Pt(chrome.codeSize(diffLineNumberWidth)), TextAlign: core.TextAlignRight, Padding: core.Edges{Right: 6},
				FontSize: chrome.codeSize(11), FontFamily: chrome.fonts.code, Color: chrome.muted}}
	}
	var children []core.Node
	if !split || !right {
		children = append(children, lineNo(line.Old))
	}
	if !split || right {
		children = append(children, lineNo(line.New))
	}
	children = append(children, core.Text{Content: sign, Selectable: core.SelectableOff,
		Style: core.Style{Width: core.Pt(chrome.codeSize(diffSignWidth)), FontSize: chrome.codeSize(11), FontFamily: chrome.fonts.code, Color: chrome.muted}})
	start, end := 0, 0
	if ink != 0 && other != nil && other.Kind != line.Kind {
		start, end = changedRange(line.Content, other.Content)
	}
	children = append(children, core.Text{
		SpansFunc: func() []core.Span {
			shades := highlight(textview.CodeBlock{Language: language, Code: line.Content})
			return diffSpans(line.Content, shades, start, end, ink)
		},
		MaxLines: 1, Selectable: core.SelectableOn, TestID: "diff-line", OnLayout: reportWidth,
		Style: core.Style{FontFamily: chrome.fonts.code, FontSize: chrome.codeSize(12), Color: chrome.text,
			WhiteSpace: core.WhiteSpaceNoWrap, FlexShrink: 0},
	})
	return core.View{Style: style, Children: children}
}

func diffSpans(line string, highlights []textview.Highlight, from, to int, ink core.Color) []core.Span {
	var spans []core.Span
	write := func(start, end int, style core.Style) {
		for start < end {
			stop := end
			segmentStyle := style
			if to > from {
				switch {
				case start < from:
					stop = min(stop, from)
				case start < to:
					stop = min(stop, to)
					segmentStyle.Background = ink
				}
			}
			spans = append(spans, core.Span{Content: line[start:stop], Style: segmentStyle})
			start = stop
		}
	}
	pos := 0
	for _, shade := range highlights {
		start, end := max(pos, min(shade.Start, len(line))), min(shade.End, len(line))
		if end <= start {
			continue
		}
		write(pos, start, core.Style{})
		write(start, end, shade.Style)
		pos = end
	}
	write(pos, len(line), core.Style{})
	return spans
}

func diffLanguage(path string) string {
	_, suffix, _ := strings.Cut(path[strings.LastIndex(path, "/")+1:], ".")
	if last := strings.LastIndex(suffix, "."); last >= 0 {
		suffix = suffix[last+1:]
	}
	switch suffix {
	case "js", "jsx", "mjs", "cjs":
		return "javascript"
	case "ts", "tsx":
		return "typescript"
	case "rs":
		return "rust"
	case "go", "css", "html", "json", "yaml", "toml":
		return suffix
	case "yml":
		return "yaml"
	case "py":
		return "python"
	case "sh", "zsh", "bash":
		return "bash"
	case "md", "mdx":
		return "markdown"
	}
	return ""
}
