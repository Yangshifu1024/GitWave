package main

import (
	"context"
	"errors"
	"slices"
	"sort"

	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/core"
)

func (w *workbench) repositoryTabs(chrome palette) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		snapshot := func() workspace.State {
			state := w.workspaceState.Get()
			if w.workspaceStore == nil && w.path.Get() != "" {
				state.Repo = workspace.Repo{Path: w.path.Get()}
				state.Repos = []workspace.Repo{state.Repo}
			}
			return state
		}
		workspaceID := core.NewMemo(cx, func() string { return snapshot().Workspace.ID })
		hasRepos := func() bool { return len(snapshot().Repos) != 0 }
		return core.Show{
			When: func() bool { return workspaceID.Get() != "" || hasRepos() },
			Then: func(*core.Scope) core.Node {
				return core.View{
					TestID: "repository-tabs",
					Style: core.Style{Height: core.Pt(32), Background: chrome.bar,
						Borders: core.Edges{Bottom: 1}, BorderColor: chrome.border},
					Children: []core.Node{core.Show{
						When: hasRepos,
						Then: func(*core.Scope) core.Node {
							return core.Dynamic(func(cx *core.Scope) core.Node {
								return w.repoTabRow(cx, chrome, workspaceID.Get(), snapshot)
							})
						},
						Else: func(*core.Scope) core.Node {
							return core.View{Style: core.Style{Height: core.Pt(32), JustifyContent: core.JustifyCenter,
								Padding: core.Edges{Left: 12, Right: 12}},
								Children: []core.Node{chrome.label(
									w.tr("No repositories — add one from the Repository menu.",
										"暂无仓库 —— 可通过「仓库」菜单添加。"), chrome.muted, 12)}}
						},
					}},
				}
			},
		}
	})
}

func (w *workbench) repoTabRow(cx *core.Scope, chrome palette, ws string, snapshot func() workspace.State) core.Node {
	dragging := core.NewSignal(cx, "")
	focused := core.NewSignal(cx, "")
	preview := core.NewSignalFunc(cx, []workspace.Repo(nil), slices.Equal[[]workspace.Repo])
	selected := core.NewMemo(cx, func() string { return snapshot().Repo.ID })
	repos := core.NewMemoFunc(cx, func() []workspace.Repo { return snapshot().Repos }, slices.Equal[[]workspace.Repo])
	count := core.NewMemo(cx, func() int { return len(repos.Get()) })
	order := func() []workspace.Repo {
		if next := preview.Get(); len(next) != 0 {
			return next
		}
		return repos.Get()
	}
	refs := make(map[string]*core.Ref)
	var dragStart workspace.State
	pending := false
	cx.Effect(func() {
		if !w.workspaceBusy.Get() && pending {
			preview.Set(nil)
			pending = false
		}
		if dragging.Get() != "" && !slices.Equal(repos.Get(), dragStart.Repos) {
			preview.Set(nil)
			dragging.Set("")
		}
	})
	scroll := new(core.Ref)
	var offset core.Points
	wheel := func(e core.WheelEvent) {
		if dragging.Get() != "" {
			return
		}
		delta := e.Delta.X
		if delta == 0 {
			delta = e.Delta.Y
		}
		frame, frameOK := scroll.Frame()
		content, contentOK := scroll.ContentBounds()
		if delta == 0 || !frameOK || !contentOK {
			return
		}
		next := max(0, min(offset-delta, content.Size.Width-frame.Size.Width))
		if next != offset {
			offset = next
			scroll.ScrollTo(core.Point{X: offset}, false)
		}
	}
	tabs := core.Each(order, func(r workspace.Repo) string { return r.ID },
		func(cx *core.Scope, item func() workspace.Repo, index func() int) core.Node {
			id := item().ID
			ref := new(core.Ref)
			refs[id] = ref
			cx.Cleanup(func() {
				delete(refs, id)
				if focused.Get() == id {
					focused.Set("")
				}
			})
			drag := repoTabDrag{}
			isSelected := func() bool { return selected.Get() == id }
			cx.Effect(func() {
				if id != "" && isSelected() {
					// Removing another tab can move the selected tab out of view.
					count.Get()
					cx.Post(func() { ref.Reveal(core.RevealOptions{Horizontal: core.ScrollAlignNearest}) })
				}
			})
			focus := func(repo workspace.Repo) {
				focused.Set(repo.ID)
				cx.Post(func() {
					if target := refs[repo.ID]; target != nil && focused.Get() == repo.ID {
						target.Focus()
						target.Reveal(core.RevealOptions{Horizontal: core.ScrollAlignNearest})
					}
				})
			}
			navigate := func(key string) bool {
				tabs := order()
				to := index()
				if to < 0 || to >= len(tabs) {
					return false
				}
				switch key {
				case "left":
					to = (to + len(tabs) - 1) % len(tabs)
				case "right":
					to = (to + 1) % len(tabs)
				case "home":
					to = 0
				case "end":
					to = len(tabs) - 1
				default:
					return false
				}
				if !w.workspaceBlocked() && dragging.Get() == "" && snapshot().Workspace.ID == ws {
					repo := tabs[to]
					focus(repo)
					if !repo.Missing {
						w.selectWorkspaceRepo(ws, repo.ID)
					}
				}
				return true
			}
			onPan := func(e core.PanEvent) {
				if e.Phase == core.GestureBegan {
					if id == "" || w.workspaceBlocked() || w.workspaceModal.Get().kind != "" ||
						dragging.Get() != "" || len(repos.Get()) < 2 {
						return
					}
					dragStart = snapshot()
					if !drag.begin(dragStart.Repos, refs, index()) {
						return
					}
					dragging.Set(id)
				}
				if id == "" || dragging.Get() != id {
					return
				}
				if e.Phase == core.GestureCancelled {
					preview.Set(nil)
					dragging.Set("")
					return
				}
				target := drag.target(e.Translation.X)
				if target != drag.current {
					preview.Set(moveRepoTab(dragStart.Repos, drag.initial, target))
					drag.current = target
				}
				if e.Phase == core.GestureEnded {
					dragging.Set("")
					pending = target != drag.initial && w.reorderWorkspaceRepos(dragStart, order())
					if !pending {
						preview.Set(nil)
					}
				}
			}
			return core.Dynamic(func(*core.Scope) core.Node {
				repo := item()
				tab := w.repoTab(chrome, repo, ref, isSelected, func() bool { return id != "" && dragging.Get() == id })
				tab.OnWheel = wheel
				if id == "" {
					return tab
				}
				tab.Focusable = func() bool {
					if current := focused.Get(); current != "" {
						return id == current
					}
					return isSelected() || (selected.Get() == "" && index() == 0)
				}
				tab.OnFocus = func() { focused.Set(id) }
				tab.OnBlur = func() {
					if focused.Get() == id {
						focused.Set("")
					}
				}
				if !repo.Missing {
					tab.OnPress = func(core.PressEvent) {
						if !w.workspaceBlocked() && dragging.Get() == "" && snapshot().Workspace.ID == ws {
							focus(repo)
							w.selectWorkspaceRepo(ws, id)
						}
					}
				}
				tab.OnContextMenu = func(e core.ContextMenuEvent) { w.repoTabMenu(ws, id, e.PagePosition) }
				tab.OnKeyDown = func(e core.KeyEvent) bool {
					if (e.Key == "f10" && e.Modifiers.Shift) || e.Key == "contextmenu" {
						if bounds, ok := ref.Bounds(); ok {
							pos := bounds.Origin
							pos.Y += bounds.Size.Height
							w.repoTabMenu(ws, id, pos)
						}
						return true
					}
					return navigate(e.Key)
				}
				tab.OnPressDown = func(e core.PressEvent) { drag.pressX = e.Position.X }
				tab.OnPan = onPan
				tab.PanActivation = core.PanActivation{ActiveOffsetX: 6, FailOffsetY: 18}
				return tab
			})
		})
	return core.Scroll{TestID: "repository-tabs-scroll", Horizontal: true, HideIndicator: true,
		Ref: scroll, Busy: w.workspaceBusy.Get, Style: core.Style{FlexGrow: 1},
		OnScroll: func(e core.ScrollEvent) {
			if !e.Stale {
				offset = e.Offset.X
			}
		},
		Children: []core.Node{core.View{Role: core.RoleTabList,
			Style: core.Style{Direction: core.Row}, Children: []core.Node{tabs}}},
	}
}

type repoTabDrag struct {
	pressX, originX core.Points
	midpoints       []core.Points
	initial         int
	current         int
}

func (d *repoTabDrag) begin(repos []workspace.Repo, refs map[string]*core.Ref, index int) bool {
	d.initial, d.current = index, index
	d.midpoints = make([]core.Points, len(repos))
	for i, repo := range repos {
		ref := refs[repo.ID]
		if ref == nil {
			return false
		}
		frame, ok := ref.Frame()
		if !ok {
			return false
		}
		if i == index {
			d.originX = frame.Origin.X + d.pressX
		}
		d.midpoints[i] = frame.Origin.X + frame.Size.Width/2
	}
	return true
}

func (d *repoTabDrag) target(delta core.Points) int {
	x := d.originX + delta
	// Use the original tab centers so unequal widths cannot shift the
	// threshold back and forth as the preview moves beneath the pointer.
	if delta > 0 {
		return max(d.initial, sort.Search(len(d.midpoints), func(i int) bool { return d.midpoints[i] >= x })-1)
	}
	return min(d.initial, sort.Search(len(d.midpoints), func(i int) bool { return d.midpoints[i] >= x }))
}

func moveRepoTab(repos []workspace.Repo, from, to int) []workspace.Repo {
	ordered := slices.Clone(repos)
	if from < to {
		copy(ordered[from:to], repos[from+1:to+1])
	} else {
		copy(ordered[to+1:from+1], repos[to:from])
	}
	ordered[to] = repos[from]
	return ordered
}

func (w *workbench) repoTab(chrome palette, repo workspace.Repo, ref *core.Ref, selected, dragging func() bool) core.View {
	name := repoTabName(repo)
	style := core.Style{Height: core.Pt(32), Padding: core.Edges{Left: 16, Right: 16},
		Direction: core.Row, Gap: 6, AlignItems: core.AlignCenter, FlexShrink: 0,
		Hover:        &core.Style{Background: chrome.hover},
		FocusVisible: &core.Style{OutlineWidth: 1, OutlineColor: chrome.accent}}
	color, label, tooltip := chrome.text, name, repo.Path
	if repo.Missing {
		color = chrome.muted
		label += " (" + w.tr("missing", "缺失") + ")"
		tooltip = w.tr(name+" — missing, right-click to relink", name+" —— 路径缺失，右键可重新关联")
	}
	content := core.Text{Content: name, MaxLines: 1,
		Style: core.Style{FontSize: chrome.uiSize(12), FontWeight: 500, Color: color, MaxWidth: core.Pt(192)}}
	tab := core.View{
		Ref: ref, Role: core.RoleTab, Label: label, Selected: selected, Style: style,
		Tooltip: chrome.label(tooltip, chrome.text, 12), Children: []core.Node{content},
		StyleFunc: func() core.Style {
			var style core.Style
			if selected() {
				style.Borders, style.BorderColor = core.Edges{Bottom: 2}, chrome.accent
			}
			if dragging() {
				style.Opacity, style.Cursor = core.Alpha(0.5), core.CursorGrabbing
			}
			return style
		},
	}
	if repo.Missing {
		tab.Children = append(tab.Children, core.View{Style: core.Style{
			Width: core.Pt(6), Height: core.Pt(6), CornerRadius: 3, Background: chrome.warning}})
	}
	return tab
}

func (w *workbench) repoTabMenu(ws, id string, position core.Point) {
	if w.workspaceBlocked() || w.workspaceModal.Get().kind != "" {
		return
	}
	state := w.workspaceState.Get()
	i := slices.IndexFunc(state.Repos, func(repo workspace.Repo) bool { return repo.ID == id })
	if state.Workspace.ID != ws || i < 0 {
		return
	}
	repo := state.Repos[i]
	move := func(to int) {
		if to >= 0 && to < len(state.Repos) {
			w.reorderWorkspaceRepos(state, moveRepoTab(state.Repos, i, to))
		}
	}
	items := []core.MenuItem{
		{Label: w.tr("Move Left", "左移"), Disabled: i == 0, OnSelect: func() { move(i - 1) }},
		{Label: w.tr("Move Right", "右移"), Disabled: i == len(state.Repos)-1, OnSelect: func() { move(i + 1) }},
		{Separator: true},
	}
	if repo.Missing {
		items = append(items, core.MenuItem{Label: w.tr("Relink…", "重新关联…"), OnSelect: func() {
			w.showWorkspaceForm(workspaceDialog{kind: "relink", ws: state.Workspace, repo: repo}, repo.Path)
		}})
	}
	items = append(items, core.MenuItem{Label: w.tr("Remove", "移除"), OnSelect: func() {
		w.showWorkspaceForm(workspaceDialog{kind: "remove", ws: state.Workspace, repo: repo}, "")
	}})
	w.cx.PopupMenu(items, position, nil)
}

func (w *workbench) reorderWorkspaceRepos(state workspace.State, order []workspace.Repo) bool {
	if w.workspaceBlocked() || w.workspaceModal.Get().kind != "" || state.Workspace.ID == "" ||
		w.workspaceState.Get().Workspace.ID != state.Workspace.ID {
		return false
	}
	before, after := make([]string, len(state.Repos)), make([]string, len(order))
	for i, repo := range state.Repos {
		before[i] = repo.ID
	}
	for i, repo := range order {
		after[i] = repo.ID
	}
	w.runWorkspace(func(ctx context.Context) (workspaceResult, error) {
		err := w.workspaceStore.ReorderRepos(ctx, state.Workspace.ID, before, after)
		if errors.Is(err, workspace.ErrRepoOrderChanged) {
			return workspaceResult{notice: w.tr("Repository tabs changed. Try moving the tab again.", "仓库标签已更改，请重新移动标签。")}, nil
		}
		return workspaceResult{}, err
	})
	return true
}
