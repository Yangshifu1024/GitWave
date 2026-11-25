package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"gitwave.work/internal/ai"
	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type aiAdvice struct {
	request *core.Signal[*aiAdviceRequest]
	result  *core.Signal[aiAdviceResult]
}

type aiAdviceRequest struct{ cancel context.CancelFunc }

type aiAdviceResult struct{ text, failure string }

func newAIAdvice(cx *core.Scope) aiAdvice {
	return aiAdvice{request: core.NewSignal[*aiAdviceRequest](cx, nil), result: core.NewSignal(cx, aiAdviceResult{})}
}

func (a aiAdvice) clear() {
	if request := a.request.Get(); request != nil {
		request.cancel()
		a.request.Set(nil)
	}
	a.result.Set(aiAdviceResult{})
}

func (w *workbench) runAIAdvice(flow aiAdvice, current func() bool,
	build func(context.Context, ai.Configuration) (ai.Prompt, error), verify func(context.Context) error) {
	client, backend := w.workspaceAI(), w.core
	if backend == nil || client.store == nil || client.id == "" || flow.request.Get() != nil || !current() {
		return
	}
	ctx, cancel := context.WithCancel(w.repoCtx)
	request := &aiAdviceRequest{cancel: cancel}
	flow.result.Set(aiAdviceResult{})
	flow.request.Set(request)
	go func() {
		defer cancel()
		output, err := func() (ai.Outcome, error) {
			cfg, err := client.configured(ctx)
			if err != nil {
				return ai.Outcome{}, err
			}
			prompt, err := build(ctx, cfg)
			if err != nil {
				return ai.Outcome{}, err
			}
			result, err := client.generate(ctx, cfg, prompt)
			if err == nil && verify != nil {
				err = verify(ctx)
			}
			return result, err
		}()
		w.cx.Post(func() {
			if w.appCtx.Err() != nil || w.core != backend || flow.request.Get() != request {
				return
			}
			flow.request.Set(nil)
			if !current() {
				return
			}
			if err == nil {
				flow.result.Set(aiAdviceResult{text: output.Text})
			} else if errors.Is(err, ai.ErrNotConfigured) {
				flow.result.Set(aiAdviceResult{failure: w.tr("Configure a provider in Workspace AI settings.", "请先在工作区 AI 设置中配置服务商。")})
			} else {
				flow.result.Set(aiAdviceResult{failure: err.Error()})
			}
		})
	}()
}

func (w *workbench) summarizeHealth(report *gitcli.HealthReport) {
	w.runAIAdvice(w.healthAI, func() bool { return w.healthOpen.Get() && w.health.Get().report == report },
		func(_ context.Context, cfg ai.Configuration) (ai.Prompt, error) {
			raw, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				return ai.Prompt{}, err
			}
			system := cfg.Templates.Health
			if system == "" {
				system = "You are a repository health assistant. You receive deterministic metrics about a Git repository " +
					"and summarize them for a developer. Advice only — you never execute anything."
			}
			return ai.Prompt{System: system, User: "Repo health metrics (JSON):\n" + string(raw) +
				"\n\nWrite a short health assessment: what looks fine, what needs attention, and the single most valuable next action. Plain text."}, nil
		}, nil)
}

func (w *workbench) explainReflogAI(entry gitcli.ReflogEntry) {
	child := w.core
	if child == nil {
		return
	}
	w.runAIAdvice(w.reflogAI, func() bool { return w.recoveryOpen.Get() && w.reflogSelected.Get() == entry },
		func(ctx context.Context, cfg ai.Configuration) (ai.Prompt, error) {
			system := cfg.Templates.Reflog
			if system == "" {
				system = "You are a git recovery assistant. A single reflog entry is provided. " +
					"In 2-4 sentences explain what happened, then give one concrete recovery recommendation. " +
					"Advice only — you never execute anything. Do not wrap in markdown fences."
			}
			user := fmt.Sprintf("Reflog entry\nAction: %s\nMessage: %s\n\nPrevious position: %s\n  (%s)\n\nNew position: %s\n  (%s)\n",
				entry.Action, entry.Message, entry.OldOID, child.repo.AICommitSubject(ctx, entry.OldOID),
				entry.NewOID, child.repo.AICommitSubject(ctx, entry.NewOID))
			return ai.Prompt{System: system, User: user}, nil
		}, nil)
}

func (w *workbench) explainConflictAI(file *gitcli.Conflict) {
	child := w.core
	if child == nil {
		return
	}
	w.runAIAdvice(w.conflictAI, func() bool {
		draft := w.conflictDraft.Get()
		return w.conflictPanel.Get() != nil && draft != nil && draft.file == file &&
			!w.currentChangesWrite().pending && file.MatchesStatus(w.changes.Get().status)
	}, func(ctx context.Context, cfg ai.Configuration) (ai.Prompt, error) {
		user, err := file.AIContext(ctx)
		if err != nil {
			return ai.Prompt{}, err
		}
		system := cfg.Templates.Conflict
		if system == "" {
			system = "You explain Git merge conflicts for a human developer. Describe what each side intends and " +
				"suggest a resolution approach. Do not output a full rewritten file unless asked. " +
				"Clearly state that this is advice and the user must apply changes."
		}
		return ai.Prompt{System: withAIRepoRules(system, child.repo.AIRules(ctx)), User: user}, nil
	}, file.CheckAIContext)
}

func (w *workbench) aiAdviceButton(chrome palette, flow aiAdvice, id, label string, run func()) core.Node {
	return core.Show{When: func() bool { return flow.request.Get() != nil }, Then: func(*core.Scope) core.Node {
		cancel := w.tr("Cancel AI request", "取消 AI 请求")
		button := toolbarAction(chrome, cancel, flow.clear, chrome.label(cancel, chrome.muted, 11))
		button.TestID = id + "-cancel"
		return button
	}, Else: func(*core.Scope) core.Node {
		button := toolbarAction(chrome, label, run, core.Icon{SVG: lucide.Sparkles(), Style: core.Style{FontSize: 12, Color: chrome.accent}},
			chrome.label(label, chrome.text, 11))
		button.TestID = id
		button.Disabled = func() bool { return w.workspaceStore == nil || w.workspaceState.Get().Workspace.ID == "" }
		return button
	}}
}

func aiAdviceStatus(chrome palette, flow aiAdvice, id string, inline bool) core.Node {
	return core.Show{When: func() bool { return flow.result.Get().failure != "" || (inline && flow.result.Get().text != "") },
		Then: func(*core.Scope) core.Node {
			return core.Text{TestID: id, Selectable: core.SelectableOn,
				ContentFunc: func() string {
					result := flow.result.Get()
					if result.failure != "" {
						return result.failure
					}
					return result.text
				},
				StyleFunc: func() core.Style {
					if flow.result.Get().failure != "" {
						return core.Style{Color: chrome.danger}
					}
					return core.Style{Color: chrome.text}
				},
				Style: core.Style{FontSize: chrome.uiSize(11), Padding: core.All(6)}, Role: core.RoleStatus}
		}}
}

func (w *workbench) aiAdviceDialog(chrome palette, id, title string, flow aiAdvice, open func() bool) core.Node {
	return ui.Dialog{TestID: id + "-dialog", Title: title, Open: open,
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				flow.clear()
			}
		},
		Children: []core.Node{core.Scroll{TestID: id + "-scroll", Style: core.Style{MaxHeight: core.Pt(360), FlexShrink: 1}, Children: []core.Node{
			aiAdviceStatus(chrome, flow, id+"-result", true),
		}}},
		Footer: []core.Node{action(chrome, w.tr("Close", "关闭"), flow.clear)},
	}
}

func (w *workbench) longReflogAdvice() bool {
	text := w.reflogAI.result.Get().text
	return strings.Count(text, "\n") > 3 || utf8.RuneCountInString(text) > 150
}
