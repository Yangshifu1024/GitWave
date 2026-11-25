package main

import (
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

func (w *workbench) commitAIGenerateButton(chrome palette) core.Node {
	return core.Show{When: func() bool { return w.commitAIRequest.Get() != nil }, Then: func(*core.Scope) core.Node {
		label := w.tr("Cancel suggestion", "取消生成")
		button := toolbarAction(chrome, label, w.cancelCommitAI, chrome.label(label, chrome.text, 12))
		button.TestID = "commit-ai-cancel"
		return button
	}, Else: func(*core.Scope) core.Node {
		label := w.tr("Generate commit message with AI", "用 AI 生成提交信息")
		button := toolbarAction(chrome, label, w.generateCommitSuggestion,
			core.Icon{SVG: lucide.Sparkles(), Style: core.Style{FontSize: 14, Color: chrome.text}},
			chrome.label(w.tr("Generate", "生成"), chrome.text, 12))
		button.TestID = "commit-ai-generate"
		button.Tooltip = chrome.label(label, chrome.text, 12)
		button.Style.Disabled = &core.Style{Opacity: core.Alpha(.45)}
		button.Disabled = func() bool {
			state := w.changes.Get()
			return w.workspaceStore == nil || w.workspaceState.Get().Workspace.ID == "" ||
				w.currentChangesWrite().pending || len(state.staged) == 0 || len(state.conflicts) != 0
		}
		return button
	}}
}

func (w *workbench) commitAIStatus(chrome palette, pending func() bool) core.Node {
	return core.Show{When: func() bool {
		result := w.commitAIResult.Get()
		return w.commitAIRequest.Get() != nil || result.failure != "" || result.fallback != "" ||
			result.suggestion != "" || w.currentCommitDraft().hasBeforeSuggestion
	}, Then: func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Gap: 4, AlignItems: core.AlignStart}, Children: []core.Node{
			core.Show{When: func() bool { return w.commitAIRequest.Get() != nil }, Then: func(*core.Scope) core.Node {
				return core.Text{Content: w.tr("Generating a commit message…", "正在生成提交信息…"), Role: core.RoleStatus,
					Style: core.Style{FontSize: chrome.uiSize(11), Color: chrome.muted}}
			}},
			core.Show{When: func() bool { return w.commitAIResult.Get().failure != "" }, Then: func(*core.Scope) core.Node {
				return core.Text{TestID: "commit-ai-error", ContentFunc: func() string { return w.commitAIResult.Get().failure },
					Role: core.RoleAlert, Style: core.Style{FontSize: chrome.uiSize(11), Color: chrome.danger}}
			}},
			core.Show{When: func() bool { return w.commitAIResult.Get().fallback != "" }, Then: func(*core.Scope) core.Node {
				return core.Text{TestID: "commit-ai-fallback", Role: core.RoleStatus,
					ContentFunc: func() string { return w.commitAIFallbackText(w.commitAIResult.Get().fallback) },
					Style:       core.Style{FontSize: chrome.uiSize(11), Color: chrome.muted}}
			}},
			core.Show{When: func() bool { return w.commitAIResult.Get().suggestion != "" }, Then: func(*core.Scope) core.Node {
				label := w.tr("Use AI suggestion", "使用 AI 建议")
				button := toolbarAction(chrome, label, func() {
					result := w.commitAIResult.Get()
					if !pending() && result.suggestion != "" {
						w.applyCommitSuggestion(result.suggestion)
						result.suggestion = ""
						w.commitAIResult.Set(result)
					}
				}, chrome.label(label, chrome.text, 12))
				button.Disabled = pending
				return button
			}},
			core.Show{When: func() bool { return w.currentCommitDraft().hasBeforeSuggestion }, Then: func(*core.Scope) core.Node {
				label := w.tr("Undo AI suggestion", "撤销 AI 建议")
				button := toolbarAction(chrome, label, w.undoCommitSuggestion, chrome.label(label, chrome.text, 12))
				button.TestID, button.Disabled = "commit-ai-restore-draft", pending
				return button
			}},
		}}
	}}
}

func (w *workbench) commitAIConfigureDialog(chrome palette) core.Node {
	closeDialog := func() { w.commitAIResult.Set(commitAIResult{}) }
	return ui.Dialog{TestID: "commit-ai-configure-dialog", Open: func() bool { return w.commitAIResult.Get().setup },
		Title: w.tr("AI provider not configured", "尚未配置 AI 服务商"),
		Description: w.tr("Configure an AI provider for this Workspace to generate commit messages.",
			"为此工作区配置 AI 服务商后，即可生成提交信息。"),
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				closeDialog()
			}
		},
		Footer: []core.Node{
			action(chrome, w.tr("Not now", "稍后"), closeDialog),
			action(chrome, w.tr("Configure", "配置"), func() {
				closeDialog()
				w.openAISettings()
			}),
		}}
}
