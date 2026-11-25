package main

import (
	"fmt"
	"strings"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

func (w *workbench) aiDraftDialog(chrome palette) core.Node {
	return core.Show{When: func() bool { return w.aiDraft.Get() != nil }, Then: func(*core.Scope) core.Node {
		form := w.aiDraft.Get()
		isPR := form.target.kind == pullRequestDraft
		title := fmt.Sprintf(w.tr("Explain commit · %s", "解释提交 · %s"), form.target.subject)
		if form.target.subject == "" {
			title = fmt.Sprintf(w.tr("Explain commit · %s", "解释提交 · %s"), shortSHA(form.target.sha))
		}
		if runes := []rune(title); len(runes) > 80 {
			title = string(runes[:79]) + "…"
		}
		description := w.tr("AI explanation of the selected commit.", "查看所选提交的 AI 解释。")
		if isPR {
			title = w.tr("AI PR description", "AI PR 描述")
			description = w.tr("Draft a title and Markdown description for the current branch.", "为当前分支生成标题和 Markdown 描述草稿。")
		}
		regenerateLabel := w.tr("Regenerate", "重新生成")
		regenerate := toolbarAction(chrome, regenerateLabel, func() { w.generateAIDraft(form, true) }, chrome.label(regenerateLabel, chrome.text, 12))
		regenerate.TestID, regenerate.Disabled = "ai-draft-regenerate", form.busy
		copyText := func(id, label string, text func() string) core.Node {
			button := toolbarAction(chrome, label, func() { w.copyAIDraft(form, text()) }, chrome.label(label, chrome.text, 12))
			button.TestID = id
			button.Disabled = func() bool { return form.busy() || form.failure.Get() != "" || strings.TrimSpace(text()) == "" }
			return button
		}
		buttons := []core.Node{action(chrome, w.tr("Close", "关闭"), w.closeAIDraft), regenerate}
		if isPR {
			buttons = append(buttons,
				copyText("ai-pr-copy-title", w.tr("Copy title", "复制标题"), form.title.Get),
				copyText("ai-pr-copy-body", w.tr("Copy description", "复制描述"), form.body.Get),
				copyText("ai-pr-copy-all", w.tr("Copy all", "复制全部"), func() string {
					return strings.TrimSpace(form.title.Get() + "\n\n" + form.body.Get())
				}),
			)
		} else {
			buttons = append(buttons, copyText("ai-explain-copy", w.tr("Copy", "复制"), form.body.Get))
		}
		var content core.Node
		if isPR {
			content = w.aiPRFields(chrome, form)
		} else {
			content = core.Show{When: func() bool { return form.body.Get() != "" }, Then: func(*core.Scope) core.Node {
				return core.Scroll{TestID: "ai-explain-scroll", Style: core.Style{MaxHeight: core.Pt(360), FlexShrink: 1}, Children: []core.Node{
					core.Text{TestID: "ai-explain-text", ContentFunc: form.body.Get, Selectable: core.SelectableOn,
						Style: core.Style{FontSize: chrome.uiSize(13), Color: chrome.text, Padding: core.All(10)}},
				}}
			}}
		}
		return ui.Dialog{TestID: "ai-draft-dialog", Title: title, Description: description, Style: core.Style{Width: core.Pt(670)},
			Open: func() bool { return w.aiDraft.Get() == form },
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open && w.aiDraft.Get() == form {
					w.closeAIDraft()
				}
			},
			Footer: buttons,
			Children: []core.Node{
				content,
				core.Show{When: form.busy, Then: func(*core.Scope) core.Node {
					return core.Text{ContentFunc: form.activity.Get, Role: core.RoleStatus, Style: core.Style{Color: chrome.muted, FontSize: chrome.uiSize(12)}}
				}},
				core.Show{When: func() bool { return form.failure.Get() != "" }, Then: func(*core.Scope) core.Node {
					return core.Text{TestID: "ai-draft-error", ContentFunc: form.failure.Get, Role: core.RoleAlert,
						Style: core.Style{Color: chrome.danger, FontSize: chrome.uiSize(12)}}
				}},
				core.Show{When: func() bool { return form.status.Get() != "" }, Then: func(*core.Scope) core.Node {
					return core.Text{TestID: "ai-draft-provider", ContentFunc: form.status.Get, Role: core.RoleStatus,
						Style: core.Style{Color: chrome.muted, FontSize: chrome.uiSize(11)}}
				}},
			}}
	}}
}

func (w *workbench) aiPRFields(chrome palette, form *aiDraftForm) core.Node {
	title := w.tr("Title", "标题")
	body := w.tr("Description (Markdown)", "描述（Markdown）")
	return core.View{Style: core.Style{Gap: 5}, Children: []core.Node{
		chrome.label(title, chrome.text, 12),
		ui.Input{TestID: "ai-pr-title", Label: title, Value: form.title.Get, OnChange: form.title.Set, Disabled: form.busy,
			Style: core.Style{FontSize: chrome.uiSize(13)}},
		chrome.label(body, chrome.text, 12),
		ui.Input{TestID: "ai-pr-body", Label: body, Value: form.body.Get, OnChange: form.body.Set, Disabled: form.busy,
			Multiline: true, NoAutoCorrect: true,
			Style: core.Style{Height: core.Pt(290), FontFamily: chrome.fonts.code, FontSize: chrome.codeSize(12)}},
	}}
}

func (w *workbench) commitExplainButton(chrome palette, sha, subject string) core.Node {
	return core.Show{When: func() bool { return w.workspaceStore != nil && w.workspaceState.Get().Workspace.ID != "" },
		Then: func(*core.Scope) core.Node {
			label := w.tr("Explain with AI", "用 AI 解释")
			button := toolbarAction(chrome, label, func() { w.openCommitExplanation(sha, subject) },
				core.Icon{SVG: lucide.Sparkles(), Style: core.Style{FontSize: 13, Color: chrome.accent}},
				chrome.label(w.tr("Explain", "解释"), chrome.text, 12))
			button.TestID = "commit-ai-explain"
			return button
		}}
}
