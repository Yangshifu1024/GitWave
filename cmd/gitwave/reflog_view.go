package main

import (
	"fmt"
	"time"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

func (w *workbench) recoverySidebar(chrome palette) core.Node {
	return core.Show{When: func() bool { return w.path.Get() != "" }, Then: func(*core.Scope) core.Node {
		label := w.tr("Recovery", "恢复")
		toggle := sidebarHeading(chrome, "reflog-toggle", label, w.recoveryOpen.Get, func() {
			if w.recoveryOpen.Get() {
				w.reflogAI.clear()
			}
			w.recoveryOpen.Set(!w.recoveryOpen.Get())
			w.loadReflog()
		})
		return core.View{Children: []core.Node{
			toggle,
			core.Show{When: w.recoveryOpen.Get, Then: func(*core.Scope) core.Node {
				return core.View{Style: core.Style{Padding: core.Edges{Left: 4, Right: 4, Bottom: 4}},
					Children: []core.Node{w.reflogTabs(chrome), w.reflogRows(chrome), w.reflogActions(chrome)}}
			}},
		}}
	}}
}

func (w *workbench) reflogTabs(chrome palette) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		branch := w.reflogLocalBranch()
		if branch.Name == "" {
			return core.View{}
		}
		tab := func(label, id string, local bool) core.Node {
			button := toolbarAction(chrome, label, func() {
				if w.recoveryBranch.Get() != local {
					w.recoveryBranch.Set(local)
					w.loadReflog()
				}
			}, chrome.label(label, chrome.text, 11))
			button.TestID = id
			button.Style.Height = core.Pt(24)
			button.Selected = func() bool { return local == w.recoveryBranch.Get() }
			button.Style.Selected = &core.Style{Background: chrome.selected}
			return button
		}
		return core.View{Style: core.Style{Direction: core.Row, Gap: 2, Padding: core.Edges{Left: 4, Bottom: 3}},
			Children: []core.Node{tab("HEAD", "reflog-head", false), tab(branch.Name, "reflog-branch", true)}}
	})
}

func (w *workbench) reflogRows(chrome palette) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		log := w.reflog.Get()
		if log.loading {
			return sidebarSectionHint(chrome, w.tr("Loading…", "正在加载…"))
		}
		if log.err != "" {
			return core.View{Children: []core.Node{core.Text{Content: log.err, Role: core.RoleAlert,
				Style: core.Style{FontSize: chrome.uiSize(11), Color: chrome.danger}},
				action(chrome, w.tr("Retry", "重试"), w.loadReflog)}}
		}
		if len(log.entries) == 0 {
			return sidebarSectionHint(chrome, w.tr("No reflog entries", "暂无 reflog 记录"))
		}
		const rowHeight = 26
		return core.List{TestID: "reflog-list", EstimatedSize: rowHeight,
			Style: core.Style{Height: core.Pt(core.Points(min(182, rowHeight*len(log.entries)))), FlexShrink: 1},
			Count: func() int { return len(log.entries) },
			Key:   func(i int) any { return i },
			Row: func(_ *core.Scope, row core.ListRow) core.Node {
				return core.Dynamic(func(*core.Scope) core.Node {
					i := row.Index()
					if i < 0 || i >= len(log.entries) {
						return core.View{}
					}
					return w.reflogRow(chrome, log.entries[i])
				})
			}}
	})
}

func (w *workbench) reflogRow(chrome palette, e gitcli.ReflogEntry) core.Node {
	actionLabel := w.reflogActionLabel(e.Action)
	title := chrome.label(actionLabel, chrome.text, 11)
	title.Selectable = core.SelectableOff
	message := core.Text{Content: e.Message, MaxLines: 1, Selectable: core.SelectableOff,
		Style: core.Style{Color: chrome.muted, FontSize: chrome.uiSize(11), FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}}
	when := time.Unix(e.Time, 0).In(time.Local).Format(w.tr("Jan 2, 15:04", "1月2日 15:04"))
	date := chrome.label(when, chrome.muted, 10)
	date.Selectable = core.SelectableOff
	row := toolbarAction(chrome, actionLabel+" · "+e.Message, func() {
		w.reflogAI.clear()
		selected := e
		if w.reflogSelected.Get() == e {
			selected = gitcli.ReflogEntry{}
		}
		w.reflogSelected.Set(selected)
	}, core.Icon{SVG: lucide.Clock(), Style: core.Style{Color: chrome.muted, FontSize: 12}}, title, message, date)
	row.TestID = "reflog-row"
	row.Style.Height = core.Pt(26)
	row.Style.Gap = 4
	row.Style.JustifyContent = core.JustifyStart
	row.Selected = func() bool { return w.reflogSelected.Get() == e }
	row.Style.Selected = &core.Style{Background: chrome.selected}
	row.Tooltip = chrome.label(e.Message, chrome.text, 12)
	return row
}

func (w *workbench) reflogActions(chrome palette) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		e := w.reflogSelected.Get()
		if e == (gitcli.ReflogEntry{}) {
			return core.View{}
		}
		oid := e.RecoveryOID()
		create := w.tr("Recovery branch", "恢复分支")
		branch := toolbarAction(chrome, create, func() {
			w.openRefForm(refDialogState{kind: recoveryBranchForm, base: oid})
		}, core.Icon{SVG: lucide.Undo2(), Style: core.Style{FontSize: 12, Color: chrome.muted}}, chrome.label(create, chrome.text, 11))
		branch.TestID = "reflog-create-branch"
		branch.Disabled = func() bool { return oid == "" || !w.canSync() }
		reset := w.tr("Reset branch here", "重置分支到此处")
		if e.Action == "reset" || e.Action == "amend" {
			reset = w.tr("Reset back here", "重置回此处")
		}
		resetButton := toolbarAction(chrome, reset, func() { w.requestHardReset(oid) },
			core.Icon{SVG: lucide.RotateCcw(), Style: core.Style{FontSize: 12, Color: chrome.muted}}, chrome.label(reset, chrome.text, 11))
		resetButton.TestID = "reflog-reset"
		resetButton.Disabled = func() bool { return oid == "" || w.reflogLocalBranch().Tip == "" || !w.canSync() }
		desc := core.Text{Content: fmt.Sprintf("%s · %s", w.reflogActionLabel(e.Action), e.Message), MaxLines: 3,
			Style: core.Style{FontSize: chrome.uiSize(11), Color: chrome.text}}
		children := []core.Node{desc}
		if e.Action == "reset" && oid != "" {
			children = append(children, core.Text{
				Content: w.tr("Create a recovery branch to keep the commit from before the reset.",
					"创建恢复分支，以保留重置前的提交。"),
				MaxLines: 3, Style: core.Style{FontSize: chrome.uiSize(11), Color: chrome.warning},
			})
		}
		explain := w.aiAdviceButton(chrome, w.reflogAI, "reflog-ai-explain",
			w.tr("Explain with AI", "用 AI 解释"), func() { w.explainReflogAI(e) })
		advice := core.Show{When: func() bool { return w.reflogAI.result.Get() != (aiAdviceResult{}) && !w.longReflogAdvice() }, Then: func(*core.Scope) core.Node {
			return aiAdviceStatus(chrome, w.reflogAI, "reflog-ai-inline", true)
		}}
		return core.View{Style: core.Style{Padding: core.All(6), Gap: 4, BorderWidth: 1, BorderColor: chrome.border, CornerRadius: 4},
			Children: append(children, core.View{Style: core.Style{Direction: core.Row, Gap: 2, Wrap: core.Wrap},
				Children: []core.Node{branch, resetButton, explain}}, advice)}
	})
}

func (w *workbench) reflogActionLabel(kind string) string {
	switch kind {
	case "initial_commit":
		return w.tr("Initial commit", "初始提交")
	case "commit":
		return w.tr("Commit", "提交")
	case "amend":
		return w.tr("Amend", "修正")
	case "checkout":
		return w.tr("Checkout", "检出")
	case "reset":
		return w.tr("Reset", "重置")
	case "merge":
		return w.tr("Merge", "合并")
	case "rebase":
		return w.tr("Rebase", "变基")
	case "pull":
		return w.tr("Pull", "拉取")
	case "fetch":
		return w.tr("Fetch", "获取")
	case "push":
		return w.tr("Push", "推送")
	case "branch":
		return w.tr("Branch", "分支")
	case "revert":
		return w.tr("Revert", "撤销")
	case "cherry_pick":
		return w.tr("Cherry-pick", "拣选")
	case "stash":
		return w.tr("Stash", "贮藏")
	case "clone":
		return w.tr("Clone", "克隆")
	default:
		return w.tr("Other", "其他")
	}
}
