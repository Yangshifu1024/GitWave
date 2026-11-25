package main

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

type healthUI struct {
	health     *core.Signal[healthState]
	healthOpen *core.Signal[bool]
	stopHealth context.CancelFunc
	healthAI   aiAdvice
}

type healthState struct {
	report  *gitcli.HealthReport
	err     string
	loading bool
}

func newHealthUI(cx *core.Scope) healthUI {
	return healthUI{health: core.NewSignal(cx, healthState{}), healthOpen: core.NewSignal(cx, false), healthAI: newAIAdvice(cx)}
}

func (w *workbench) clearHealth() {
	w.healthAI.clear()
	if w.stopHealth != nil {
		w.stopHealth()
		w.stopHealth = nil
	}
	w.health.Set(healthState{})
	w.healthOpen.Set(false)
}

func (w *workbench) loadHealth(child *backend) {
	if child == nil {
		return
	}
	if w.stopHealth != nil {
		w.stopHealth()
	}
	ctx, cancel := context.WithCancel(w.repoCtx)
	w.stopHealth = cancel
	if w.health.Get().report == nil {
		w.health.Set(healthState{loading: true})
	}
	go func() {
		report, err := child.repo.Health(ctx)
		w.cx.Post(func() {
			defer cancel()
			if ctx.Err() != nil {
				return
			}
			w.stopHealth = nil
			if err != nil {
				w.healthAI.clear()
				w.health.Set(healthState{err: changesWriteError(err)})
				return
			}
			previous := w.health.Get()
			if previous.report != nil && reflect.DeepEqual(*previous.report, report) {
				return
			}
			w.healthAI.clear()
			w.health.Set(healthState{report: &report})
		})
	}()
}

func (w *workbench) healthSidebar(chrome palette) core.Node {
	return core.Show{When: func() bool { return w.health.Get() != (healthState{}) }, Then: func(*core.Scope) core.Node {
		return core.Show{When: func() bool { return w.health.Get().err != "" }, Then: func(*core.Scope) core.Node {
			return core.Text{TestID: "health-error", ContentFunc: func() string { return w.health.Get().err }, MaxLines: 2,
				Style: core.Style{Color: chrome.danger, FontSize: chrome.uiSize(11), Padding: core.All(8)}}
		}, Else: func(*core.Scope) core.Node {
			return core.View{Style: core.Style{Padding: core.Edges{Left: 4, Right: 4, Bottom: 4}}, Children: []core.Node{
				w.healthToggle(chrome),
				core.Show{When: w.healthOpen.Get, Then: func(*core.Scope) core.Node {
					return core.Dynamic(func(*core.Scope) core.Node {
						report := w.health.Get().report
						if report == nil {
							status, _ := w.healthStatus(chrome)
							return chrome.label(status, chrome.muted, 11)
						}
						return core.View{Children: append(w.healthMetrics(chrome, *report),
							w.aiAdviceButton(chrome, w.healthAI, "health-ai-summary", w.tr("AI health summary", "AI 健康总结"),
								func() { w.summarizeHealth(report) }), aiAdviceStatus(chrome, w.healthAI, "health-ai-error", false))}
					})
				}},
			}}
		}}
	}}
}

func (w *workbench) healthToggle(chrome palette) core.View {
	label := w.tr("Repo health", "仓库健康")
	value := healthValue(chrome, "", chrome.muted)
	value.TestID = "health-summary"
	value.ContentFunc = func() string { text, _ := w.healthStatus(chrome); return text }
	value.StyleFunc = func() core.Style { _, color := w.healthStatus(chrome); return core.Style{Color: color} }
	toggle := toolbarAction(chrome, label, func() {
		if w.healthOpen.Get() {
			w.healthAI.clear()
		}
		w.healthOpen.Set(!w.healthOpen.Get())
	}, core.Icon{SVG: lucide.Activity(), Style: core.Style{FontSize: 14, Color: chrome.text}},
		chrome.label(label, chrome.text, 12), value, core.Icon{SVGFunc: func() *core.SVG {
			if w.healthOpen.Get() {
				return lucide.ChevronUp()
			}
			return lucide.ChevronDown()
		}, Style: core.Style{FontSize: 12, Color: chrome.muted}})
	toggle.TestID, toggle.Expanded = "health-toggle", w.healthOpen.Get
	toggle.Style.Height = core.Pt(32)
	toggle.Tooltip = base.Tooltip{
		PopupStyleFunc: func() core.Style {
			report := w.health.Get().report
			if report == nil || len(report.ConflictResidue) == 0 {
				return core.Style{Display: core.DisplayNone}
			}
			return core.Style{}
		},
		Children: []core.Node{core.Text{Style: core.Style{FontSize: chrome.uiSize(11), Color: chrome.text}, ContentFunc: func() string {
			if report := w.health.Get().report; report != nil {
				return strings.Join(report.ConflictResidue, ", ")
			}
			return ""
		}}},
	}
	return toggle
}

func (w *workbench) healthStatus(chrome palette) (string, core.Color) {
	report := w.health.Get().report
	switch {
	case report == nil:
		return w.tr("Measuring…", "测量中…"), chrome.muted
	case len(report.ConflictResidue) != 0:
		return w.tr("Operation residue", "操作残留"), chrome.danger
	case report.SubmoduleWarning != "":
		return w.tr("Incomplete changes", "改动检查不完整"), chrome.warning
	case report.DirtyFiles != 0:
		return fmt.Sprintf(w.tr("%d changed", "%d 项变更"), report.DirtyFiles), chrome.muted
	default:
		return w.tr("Clean", "无变更"), chrome.muted
	}
}

func (w *workbench) healthMetrics(chrome palette, report gitcli.HealthReport) []core.Node {
	unpushed := w.tr("no upstream", "无上游")
	if report.Unpushed != nil {
		unpushed = w.tr("in sync", "已同步")
		if *report.Unpushed > 0 {
			unpushed = fmt.Sprintf(w.tr("%d commits", "%d 个提交"), *report.Unpushed)
		}
	}
	residue := w.tr("None", "无")
	if len(report.ConflictResidue) != 0 {
		residue = strings.Join(report.ConflictResidue, ", ")
	}
	stale := w.tr("none (30 days)", "无（30 天）")
	if len(report.StaleBranches) != 0 {
		stale = strings.Join(report.StaleBranches, ", ")
	}
	large := w.tr("none (>1 MB)", "无（>1 MB）")
	if report.LargeFilesLimited {
		large = w.tr("scan limit reached", "已达扫描上限")
	}
	if len(report.LargeFiles) != 0 {
		files := make([]string, 0, len(report.LargeFiles))
		for _, file := range report.LargeFiles {
			files = append(files, fmt.Sprintf("%s (%.1f MB)", file.Path, float64(file.SizeBytes)/(1<<20)))
		}
		large = strings.Join(files, ", ")
		if report.LargeFilesLimited {
			large += w.tr(" (scan limit reached)", "（已达扫描上限）")
		}
	}
	dirty := fmt.Sprint(report.DirtyFiles)
	if report.SubmoduleWarning != "" {
		dirty = fmt.Sprintf(w.tr("At least %d", "至少 %d"), report.DirtyFiles)
	}
	return []core.Node{
		healthMetric(chrome, "health-unpushed", w.tr("Unpushed", "未推送"), unpushed, report.Unpushed != nil && *report.Unpushed > 0),
		healthMetric(chrome, "health-residue", w.tr("Conflict residue", "冲突残留"), residue, len(report.ConflictResidue) != 0),
		healthMetric(chrome, "health-dirty", w.tr("Dirty files", "脏文件"), dirty, report.SubmoduleWarning != ""),
		healthMetric(chrome, "health-stale", w.tr("Stale branches", "过期分支"), stale, len(report.StaleBranches) != 0),
		healthMetric(chrome, "health-large", w.tr("Large files", "大文件"), large, len(report.LargeFiles) != 0),
		healthMetric(chrome, "health-refs", w.tr("Branches / tags", "分支 / 标签"), fmt.Sprintf("%d / %d", report.BranchCount, report.TagCount), false),
	}
}

func healthMetric(chrome palette, id, label, value string, warn bool) core.Node {
	labelColor, valueColor := chrome.muted, chrome.text
	if warn {
		labelColor, valueColor = chrome.warning, chrome.warning
	}
	text := healthValue(chrome, value, valueColor)
	text.TestID = id
	return core.View{Tooltip: chrome.label(value, chrome.text, 11),
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 8,
			Padding: core.Edges{Left: 8, Right: 8, Top: 4, Bottom: 4}},
		Children: []core.Node{chrome.label(label, labelColor, 11), text}}
}

func healthValue(chrome palette, value string, color core.Color) core.Text {
	text := chrome.label(value, color, 11)
	text.Style.FlexGrow, text.Style.FlexShrink = 1, 1
	text.Style.MinWidth, text.Style.TextAlign = core.Pt(0), core.TextAlignRight
	return text
}
