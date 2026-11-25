package main

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

type branchKey struct{ kind, name string }
type branchFolderKey struct{ remote, prefix string }
type branchRowIdentity struct {
	branch branchKey
	folder branchFolderKey
	exists bool
}

type branchTrackingStatus struct {
	tip, upstream string
	ahead, behind int
}

func (w *workbench) watchBranchTracking() {
	var stop context.CancelFunc
	w.cx.Cleanup(func() {
		if stop != nil {
			stop()
		}
	})
	w.cx.Effect(func() {
		if stop != nil {
			stop()
			stop = nil
		}
		info, child := w.repo.Get(), w.core
		w.branchTracking.Set(nil)
		if child == nil || !slices.ContainsFunc(info.Branches, func(b branch) bool { return b.UpstreamRef != "" }) {
			return
		}
		ctx, cancel := context.WithCancel(w.repoCtx)
		stop = cancel
		go func() {
			refs, err := child.repo.ReferencesWithTracking(ctx)
			if err != nil {
				return
			}
			statuses := make(map[string]branchTrackingStatus)
			for _, ref := range refs.Refs {
				if name, ok := strings.CutPrefix(ref.Name, "refs/heads/"); ok && ref.Upstream != "" {
					statuses[name] = branchTrackingStatus{ref.OID, ref.Upstream, ref.Ahead, ref.Behind}
				}
			}
			w.cx.Post(func() {
				if ctx.Err() == nil && w.core == child {
					w.branchTracking.Set(statuses)
				}
			})
		}()
	})
}

type sidebarBranch struct {
	branch
	label string
}

func (b sidebarBranch) key() branchKey { return branchKey{b.Kind, b.Name} }

type branchGroup struct {
	remote   string
	roots    []sidebarBranch
	folders  map[string][]sidebarBranch
	prefixes []string
	count    int
}

func branchGroups(info repository) []branchGroup {
	local := make(map[string]bool)
	for _, b := range info.Branches {
		if b.Kind == "local" {
			local[b.Name] = true
		}
	}
	groups := []branchGroup{{folders: make(map[string][]sidebarBranch)}}
	indices := make(map[string]int)
	for _, b := range info.Branches {
		index, label := 0, b.Name
		if b.Kind == "remote" {
			remote, short := splitRemoteBranch(b.Name, info.Remotes)
			if short == "HEAD" || local[short] {
				continue
			}
			label = short
			index = indices[remote]
			if index == 0 {
				index = len(groups)
				indices[remote] = index
				groups = append(groups, branchGroup{remote: remote, folders: make(map[string][]sidebarBranch)})
			}
		}
		group := &groups[index]
		group.count++
		if prefix, tail, nested := strings.Cut(label, "/"); nested {
			if len(group.folders[prefix]) == 0 {
				group.prefixes = append(group.prefixes, prefix)
			}
			group.folders[prefix] = append(group.folders[prefix], sidebarBranch{b, tail})
		} else {
			group.roots = append(group.roots, sidebarBranch{b, label})
		}
	}
	for i := range groups {
		slices.Sort(groups[i].prefixes)
	}
	return groups
}

func splitRemoteBranch(name string, remotes []string) (string, string) {
	remote := ""
	for _, configured := range remotes {
		if len(configured) > len(remote) && strings.HasPrefix(name, configured+"/") {
			remote = configured
		}
	}
	if remote != "" {
		return remote, strings.TrimPrefix(name, remote+"/")
	}
	if remote, short, ok := strings.Cut(name, "/"); ok {
		return remote, short
	}
	return name, name
}

type branchSidebarRow struct {
	folder branchFolderKey
	leaf   sidebarBranch
	count  int
	closed bool
	active bool
}

func (r branchSidebarRow) height() core.Points {
	if r.leaf.Name == "" {
		if r.folder.remote != "" && r.folder.prefix == "" {
			return 39
		}
		return 31
	}
	if r.active {
		return 40
	}
	return 32
}

func branchSidebarRows(groups []branchGroup, collapsed map[branchFolderKey]bool, selected branchKey) []branchSidebarRow {
	var current branchKey
	found := false
	visit := func(list []sidebarBranch) {
		for _, b := range list {
			found = found || b.key() == selected
			if b.Current {
				current = b.key()
			}
		}
	}
	for _, group := range groups {
		visit(group.roots)
		for _, folder := range group.folders {
			visit(folder)
		}
	}
	if !found {
		selected = current
	}
	closed := func(key branchFolderKey, fallback bool) bool {
		if value, set := collapsed[key]; set {
			return value
		}
		return fallback
	}
	var rows []branchSidebarRow
	addLeaves := func(folder branchFolderKey, branches []sidebarBranch) {
		for _, b := range branches {
			rows = append(rows, branchSidebarRow{folder: folder, leaf: b, active: b.key() == selected})
		}
	}
	for _, group := range groups {
		if group.count == 0 {
			continue
		}
		key := branchFolderKey{remote: group.remote}
		groupClosed := closed(key, group.remote != "")
		rows = append(rows, branchSidebarRow{folder: key, count: group.count, closed: groupClosed})
		if groupClosed {
			continue
		}
		addLeaves(key, group.roots)
		for _, prefix := range group.prefixes {
			key := branchFolderKey{remote: group.remote, prefix: prefix}
			branches := group.folders[prefix]
			containsActive := slices.ContainsFunc(branches, func(b sidebarBranch) bool { return b.Current || b.key() == selected })
			folderClosed := closed(key, !containsActive)
			rows = append(rows, branchSidebarRow{folder: key, count: len(branches), closed: folderClosed})
			if !folderClosed {
				addLeaves(key, branches)
			}
		}
	}
	return rows
}

func (w *workbench) revealSidebarBranch(info repository) {
	for _, b := range info.Branches {
		if b.Current {
			w.branchSelection.Set(branchKey{b.Kind, b.Name})
			collapsed := maps.Clone(w.branchCollapsed.Get())
			collapsed[branchFolderKey{}] = false
			if prefix, _, nested := strings.Cut(b.Name, "/"); nested {
				collapsed[branchFolderKey{prefix: prefix}] = false
			}
			w.branchCollapsed.Set(collapsed)
			w.branchSectionOpen.Set(true)
			return
		}
	}
}

func (w *workbench) branchSidebar(chrome palette) core.Node {
	label := w.tr("Branches", "分支")
	header := sidebarHeading(chrome, "branches-toggle", label, w.branchSectionOpen.Get,
		func() { w.branchSectionOpen.Set(!w.branchSectionOpen.Get()) })
	body := core.Show{When: w.branchSectionOpen.Get, Then: func(cx *core.Scope) core.Node {
		groups := core.NewMemoFunc(cx, func() []branchGroup { return branchGroups(w.repo.Get()) }, nil)
		rows := core.NewMemoFunc(cx, func() []branchSidebarRow {
			return branchSidebarRows(groups.Get(), w.branchCollapsed.Get(), w.branchSelection.Get())
		}, nil)
		return core.View{Children: []core.Node{w.interactiveRebaseControls(chrome),
			core.Show{When: func() bool { return len(rows.Get()) > 0 }, Then: func(cx *core.Scope) core.Node {
				var padding core.Edges
				if cx.Env().OS() == core.OSMacOS {
					padding.Right = 13
				}
				showTracking := core.NewSignal(cx, false)
				return core.List{TestID: "branch-sidebar-list", EstimatedSize: 28,
					Style: core.Style{Padding: padding},
					OnLayout: func(bounds core.Bounds) {
						showTracking.Set(bounds.Size.Width-padding.Right >= chrome.uiSize(164))
					},
					StyleFunc: func() core.Style {
						var height core.Points
						for _, row := range rows.Get() {
							height += row.height()
							if height >= 252 {
								height = 252
								break
							}
						}
						return core.Style{Height: core.Pt(height), MinHeight: core.Pt(height)}
					},
					Count: func() int { return len(rows.Get()) },
					Key: func(i int) any {
						row := rows.Get()[i]
						if row.leaf.Name != "" {
							return row.leaf.key()
						}
						return row.folder
					},
					Row: func(cx *core.Scope, item core.ListRow) core.Node {
						row := core.NewMemo(cx, func() branchSidebarRow {
							visible, i := rows.Get(), item.Index()
							if i < 0 || i >= len(visible) {
								return branchSidebarRow{}
							}
							return visible[i]
						})
						identity := core.NewMemo(cx, func() branchRowIdentity {
							current := row.Get()
							return branchRowIdentity{current.leaf.key(), current.folder, current.count != 0 || current.leaf.Name != ""}
						})
						return core.Dynamic(func(*core.Scope) core.Node {
							id := identity.Get()
							if !id.exists {
								return core.View{}
							}
							if id.branch.name != "" {
								return w.sidebarBranchRow(chrome, id, row.Get, showTracking.Get)
							}
							return w.branchFolderRow(chrome, id.folder, row.Get)
						})
					}}
			}, Else: func(*core.Scope) core.Node {
				return core.Dynamic(func(*core.Scope) core.Node {
					switch {
					case w.path.Get() == "":
						return sidebarSectionHint(chrome, w.tr("Select a repository to view branches", "选择仓库以查看分支"))
					case w.historyLoading.Get():
						return core.View{Style: core.Style{Height: core.Pt(32)}}
					default:
						return sidebarSectionHint(chrome, w.tr("No branches found", "未找到分支"))
					}
				})
			}}}}
	}}
	return core.View{Children: []core.Node{header, body}}
}

func (w *workbench) branchFolderRow(chrome palette, key branchFolderKey, row func() branchSidebarRow) core.Node {
	name := key.remote
	left, size, weight := core.Points(12), core.Points(12), core.FontWeight(600)
	if key.prefix != "" {
		name, left, size, weight = key.prefix, 24, 10, 500
	} else if name == "" {
		name = w.tr("Local", "本地")
	}
	display := name
	if key.prefix == "" {
		display = strings.ToUpper(name)
	}
	label := chrome.label(display, chrome.muted, 11)
	label.Style.FontWeight = weight
	label.Style.FlexShrink, label.Style.MinWidth = 1, core.Pt(0)
	if key.prefix == "" {
		label.Style.Color, label.Style.LetterSpacingEm = chrome.textMuted, .05
	}
	count := core.Text{ContentFunc: func() string { return "(" + strconv.Itoa(row().count) + ")" },
		Style: core.Style{Color: chrome.textMuted, FontSize: chrome.uiSize(11)}, MaxLines: 1}
	button := toolbarAction(chrome, name, func() {
		next := maps.Clone(w.branchCollapsed.Get())
		next[key] = !row().closed
		w.branchCollapsed.Set(next)
	}, core.Dynamic(func(*core.Scope) core.Node {
		icon := lucide.ChevronDown()
		if row().closed {
			icon = lucide.ChevronRight()
		}
		return core.Icon{SVG: icon, Style: core.Style{Color: chrome.muted, FontSize: size}}
	}), label, count)
	button.TestID = "branch-group"
	if key.prefix != "" {
		button.TestID = "branch-folder"
	}
	button.ShowsExpanded, button.Expanded = true, func() bool { return !row().closed }
	button.Style.Padding.Left = left
	if key.remote != "" && key.prefix == "" {
		button.Style.Padding.Top = 8
	}
	button.StyleFunc = func() core.Style { return core.Style{Height: core.Pt(row().height())} }
	button.Style.JustifyContent, button.Style.CornerRadius = core.JustifyStart, 0
	return button
}

func (w *workbench) sidebarBranchRow(chrome palette, id branchRowIdentity, row func() branchSidebarRow, showTracking func() bool) core.Node {
	left := core.Points(16)
	if id.folder.prefix != "" {
		left = 32
	}
	ref := new(core.Ref)
	menu := func(at core.Point) { w.branchRowMenu(row().leaf.branch, at) }
	return core.View{
		Role: core.RoleButton, Label: id.branch.name, Ref: ref, TestID: "branch-row",
		Tooltip: core.Text{Content: id.branch.name, Style: core.Style{
			Color: chrome.text, FontSize: chrome.uiSize(12), MaxWidth: core.Pt(420)}},
		Selected: func() bool { return row().active },
		OnPress: func(e core.PressEvent) {
			if w.currentChangesWrite().pending {
				return
			}
			b := row().leaf
			// AppKit counts clicks by screen position, even if a row moved after the first press.
			if e.Clicks >= 2 {
				sameBranch := e.Clicks == 2 && w.branchPress == b.key()
				w.branchPress = branchKey{}
				if sameBranch {
					w.checkoutBranch(b.branch)
				}
			} else {
				w.branchPress = b.key()
				w.branchSelection.Set(b.key())
				w.locateCommit(b.Tip)
				if b.Tip != "" {
					w.selectCommit(b.Tip)
				}
			}
		},
		OnContextMenu: func(e core.ContextMenuEvent) { menu(e.PagePosition) },
		OnKeyDown:     func(e core.KeyEvent) bool { return refMenuKey(ref, e, menu) },
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
			Padding: core.Edges{Left: left, Right: 12}, Borders: core.Edges{Left: 3}, BorderColor: core.Transparent,
			Hover:        &core.Style{Background: chrome.elevated},
			FocusVisible: &core.Style{OutlineWidth: 1, OutlineColor: chrome.accent, OutlineOffset: -1}},
		StyleFunc: func() core.Style {
			row := row()
			style := core.Style{Height: core.Pt(row.height())}
			if row.active {
				style.Background, style.BorderColor = chrome.accent.Faded(.1), chrome.accent
				style.Hover = &core.Style{Background: chrome.accent.Faded(.15)}
			}
			return style
		},
		Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			return w.sidebarBranchContent(chrome, row(), showTracking())
		})},
	}
}

func (w *workbench) sidebarBranchContent(chrome palette, row branchSidebarRow, showTracking bool) core.Node {
	b := row.leaf
	label := chrome.label(b.label, chrome.muted, 13)
	if b.Current || row.active {
		label.Style.Color, label.Style.FontWeight = chrome.text, 500
	}
	content := []core.Node{label}
	if row.active {
		style := core.Style{Color: chrome.textMuted, FontSize: chrome.uiSize(11), FlexShrink: 1, MinWidth: core.Pt(0)}
		if b.Upstream != "" {
			content = append(content, core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4},
				Children: []core.Node{
					core.Icon{SVG: lucide.ArrowRight(), Style: core.Style{FontSize: 10, Color: chrome.textMuted}},
					core.Text{Content: b.Upstream, MaxLines: 1, Style: style},
				}})
		} else if b.Tip != "" {
			spans := []core.Span{{Content: shortSHA(b.Tip), Style: core.Style{FontFamily: chrome.fonts.code}}}
			if b.Time > 0 {
				spans = append(spans, core.Span{Content: " · " + w.sidebarBranchTime(b.Time)})
			}
			content = append(content, core.Text{Spans: spans, MaxLines: 1, Style: style})
		}
	}
	iconColor := chrome.muted
	if b.Kind == "remote" {
		iconColor = chrome.textMuted
	}
	children := []core.Node{
		core.Icon{SVG: lucide.GitBranch(), Style: core.Style{FontSize: 14, Color: iconColor}},
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: content},
	}
	if tracking, ok := w.branchTracking.Get()[b.Name]; showTracking && b.Kind == "local" && ok && tracking.tip == b.Tip && tracking.upstream == b.UpstreamRef {
		if tracking.ahead > 0 {
			children = append(children, chrome.label("↑"+strconv.Itoa(tracking.ahead), chrome.accent, 10))
		}
		if tracking.behind > 0 {
			children = append(children, chrome.label("↓"+strconv.Itoa(tracking.behind), chrome.warning, 10))
		}
	}
	if b.Current {
		head := chrome.label("HEAD", chrome.accent, 10)
		head.Style.FontWeight = 500
		children = append(children, head)
	}
	return core.Group(children)
}

func (w *workbench) sidebarBranchTime(stamp int64) string {
	when := time.Unix(stamp, 0)
	elapsed := time.Since(when)
	switch {
	case elapsed < time.Minute:
		return w.tr("just now", "刚刚")
	case elapsed < time.Hour:
		return fmt.Sprintf(w.tr("%dm ago", "%d 分钟前"), int(elapsed/time.Minute))
	case elapsed < 24*time.Hour:
		return fmt.Sprintf(w.tr("%dh ago", "%d 小时前"), int(elapsed/time.Hour))
	case elapsed < 7*24*time.Hour:
		return fmt.Sprintf(w.tr("%dd ago", "%d 天前"), int(elapsed/(24*time.Hour)))
	default:
		return when.Format(w.tr("1/2/2006", "2006/1/2"))
	}
}
