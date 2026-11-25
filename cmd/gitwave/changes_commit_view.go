package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

var conventionalPrefix = regexp.MustCompile(`(?i)^[a-z]+(\([^)]*\))?:[\s\p{Z}\x{FEFF}]*`)

var commitTypes = []string{"feat", "fix", "chore", "docs", "style", "refactor", "perf", "test", "build", "ci"}

func (w *workbench) commitComposer(cx *core.Scope, chrome palette) core.Node {
	input := new(core.Ref)
	pending := func() bool { return w.currentChangesWrite().pending || w.commitAIRequest.Get() != nil }
	submit := func() {
		if !pending() && input.HoldsFocus() {
			w.writeCommit()
		}
	}
	shortcut := core.KeyOptions{When: core.KeyAlways}
	cx.OnKeyWith("mod+enter", shortcut, submit)
	if cx.Env().OS() == core.OSMacOS {
		cx.OnKeyWith("ctrl+enter", shortcut, submit)
	}

	typeLabel := w.tr("Commit type (optional)", "Commit 类型（可选）")
	items := make([]base.SelectItem, len(commitTypes))
	for i, kind := range commitTypes {
		items[i] = base.SelectItem{Value: kind, Title: kind}
	}
	typeCaption := chrome.label(typeLabel, chrome.muted, 12)
	typeCaption.Style.FlexShrink, typeCaption.Style.MinWidth = 1, core.Pt(0)
	types := base.Select{
		TestID: "commit-type", Label: typeLabel, Disabled: pending,
		Items: func() []base.SelectItem { return items }, Value: func() *string { return nil },
		RootStyle: core.Style{FlexShrink: 1, MinWidth: core.Pt(0), MaxWidth: core.Pct(100)},
		Style: core.Style{Height: core.Pt(28), BorderWidth: 1, BorderColor: chrome.border, CornerRadius: 6,
			FlexShrink: 1, MinWidth: core.Pt(0), MaxWidth: core.Pct(100),
			Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, Padding: core.Edges{Left: 8, Right: 8}},
		PopupStyle: core.Style{Background: chrome.elevated, BorderWidth: 1, BorderColor: chrome.border,
			CornerRadius: 4, Padding: core.All(4), MaxHeight: core.Pt(220)},
		ItemStyle: core.Style{Padding: core.All(5), CornerRadius: 4,
			FocusVisible: &core.Style{Background: chrome.selected}, Hover: &core.Style{Background: chrome.hover}},
		Trigger: core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, FlexShrink: 1, MinWidth: core.Pt(0)},
			Children: []core.Node{typeCaption,
				core.Icon{SVG: lucide.ChevronDown(), Style: core.Style{FontSize: 13, Color: chrome.muted}}}},
		RenderItem: func(entry base.SelectEntry) core.Node { return chrome.label(entry.Value, chrome.text, 12) },
		OnChange: func(value *string) {
			if value == nil || pending() {
				return
			}
			prefix := *value + ": "
			w.editCommitMessage(prefix + conventionalPrefix.ReplaceAllString(w.currentCommitDraft().message, ""))
			cx.RequestAnimationFrame(func(time.Time) {
				input.Focus()
				input.SetSelection(len(prefix), len(prefix))
			})
		},
	}
	textbox := core.Input{
		TestID: "commit-message", Ref: input, Multiline: true, NoAutoCorrect: true,
		Label:       w.tr("Commit message", "提交信息"),
		Placeholder: w.tr("feat: your commit message here…", "feat: 在此输入提交信息…"),
		Value:       func() string { return w.currentCommitDraft().message },
		OnChange:    w.editCommitMessage, Disabled: pending,
		Style: core.Style{Height: core.Pt(96), BorderWidth: 1, BorderColor: chrome.border,
			CornerRadius: 8, Padding: core.Edges{Left: 12, Right: 12, Top: 8, Bottom: 8}, Background: chrome.elevated,
			Color: chrome.text, PlaceholderColor: chrome.textMuted, FontSize: chrome.uiSize(14)},
		StyleFunc: func() core.Style {
			if w.currentCommitDraft().amendHead != "" {
				return core.Style{BorderColor: chrome.warning}
			}
			return core.Style{}
		},
	}
	inAmend := func() bool { return w.currentCommitDraft().amendHead != "" }
	validAmend := func() bool { return w.changes.Get().amendMatches(w.currentCommitDraft()) }
	commitButton := func(label string) core.Node {
		caption := chrome.label(label, core.RGB(0xffffff), 12)
		caption.Style.FontWeight = 500
		button := toolbarAction(chrome, label, w.writeCommit, caption)
		button.TestID = "write-commit"
		button.Disabled = func() bool { return pending() || !w.changes.Get().canCommit(w.currentCommitDraft()) }
		button.Style.CornerRadius = 6
		button.Style.Background = chrome.accent
		button.Style.Hover = &core.Style{Background: chrome.accent, Opacity: core.Alpha(.88)}
		button.Style.Disabled = &core.Style{Opacity: core.Alpha(.4)}
		return button
	}
	amendLabel := w.tr("Amend", "修正")
	amend := toolbarAction(chrome, amendLabel, func() {
		if validAmend() {
			w.editCommitMessage(w.currentCommitDraft().original)
			cx.Post(input.Focus)
			return
		}
		w.openAmend()
	}, chrome.label(amendLabel, chrome.text, 12))
	amend.TestID = "open-amend"
	amend.Disabled = pending
	amend.Style.BorderWidth, amend.Style.BorderColor = 1, chrome.border
	restoreLabel := w.tr("Restore previous draft", "恢复此前草稿")
	restoreCaption := chrome.label(restoreLabel, chrome.text, 12)
	restoreCaption.Style.FlexShrink, restoreCaption.Style.MinWidth = 1, core.Pt(0)
	restore := toolbarAction(chrome, restoreLabel, func() {
		w.setCommitDraft(w.changesWriteKey(), commitDraft{message: w.currentCommitDraft().previous})
		cx.Post(input.Focus)
	}, restoreCaption)
	restore.Disabled = pending
	restore.Style.MaxWidth = core.Pct(100)
	restore.Tooltip = chrome.label(restoreLabel, chrome.text, 12)

	hint := func() string {
		draft := w.currentCommitDraft()
		if draft.amendHead != "" && !validAmend() {
			return w.tr("The confirmed HEAD changed. Confirm Amend again or restore your draft.",
				"已确认的 HEAD 发生变化。请重新确认修正，或恢复草稿。")
		}
		length := len(utf16.Encode([]rune(draft.message)))
		if length > 0 && length < 10 {
			return fmt.Sprintf(w.tr("%d characters", "%d 字符"), length)
		}
		line, _, _ := strings.Cut(draft.message, "\n")
		if first := len(utf16.Encode([]rune(line))); first > 72 {
			return fmt.Sprintf(w.tr("%d characters (first line over 72: %d)", "%d 字符（首行超过 72：%d）"), length, first)
		}
		return ""
	}
	return core.View{TestID: "commit-composer", Style: core.Style{
		Padding: core.All(8), Gap: 8, Borders: core.Edges{Top: 1}, BorderColor: chrome.border,
	}, Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{types}},
		textbox,
		w.commitAIStatus(chrome, pending),
		core.Show{When: func() bool { return hint() != "" }, Then: func(*core.Scope) core.Node {
			return core.Text{ContentFunc: hint, Style: core.Style{Color: chrome.muted, FontSize: chrome.uiSize(11), TextAlign: core.TextAlignEnd}}
		}},
		core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter, JustifyContent: core.JustifyEnd,
			Wrap: core.Wrap}, Children: []core.Node{
			core.Show{When: inAmend, Then: func(*core.Scope) core.Node { return restore }},
			core.Show{When: func() bool { return w.changes.Get().canAmend() }, Then: func(*core.Scope) core.Node { return amend }},
			w.commitAIGenerateButton(chrome),
			core.Show{When: inAmend,
				Then: func(*core.Scope) core.Node { return commitButton(w.tr("Amend commit", "修正提交")) },
				Else: func(*core.Scope) core.Node { return commitButton(w.tr("Commit", "提交")) }},
		}},
	}}
}

func (w *workbench) amendDialog(chrome palette) core.Node {
	cancel := func() { w.amendPrompt.Set(nil) }
	confirmLabel := w.tr("Amend", "修正")
	confirm := toolbarAction(chrome, confirmLabel, w.confirmAmend, chrome.label(confirmLabel, core.RGB(0xffffff), 12))
	confirm.TestID = "confirm-amend"
	confirm.Disabled = func() bool { return !w.canConfirmAmend(w.amendPrompt.Get()) }
	confirm.Style.Background = chrome.accent
	confirm.Style.Hover = &core.Style{Background: chrome.accent, Opacity: core.Alpha(.88)}
	confirm.Style.Disabled = &core.Style{Opacity: core.Alpha(.4)}
	return ui.Dialog{
		TestID: "amend-confirmation", Role: core.RoleAlertDialog, Title: w.tr("Amend the most recent commit?", "修正最近一次提交？"),
		Description: w.tr(
			"This rewrites history. The staged changes and edited message replace the previous commit. The original commit can be recovered from the reflog.",
			"这将改写历史。已暂存的变更和编辑后的提交信息将替换上一次提交。原提交仍可从 Reflog 恢复。"),
		Open: func() bool { return w.amendPrompt.Get() != nil },
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				cancel()
			}
		},
		Children: []core.Node{
			core.Show{When: func() bool {
				prompt := w.amendPrompt.Get()
				return prompt != nil && prompt.upstream != ""
			}, Then: func(*core.Scope) core.Node {
				return core.Text{Content: w.tr(
					"This branch has an upstream and the commit may have been pushed. Amending rewrites published history and the next push may be rejected.",
					"该分支已设置 upstream，提交可能已推送。修正将改写已发布的历史，下次 push 可能被拒绝。"),
					Style: core.Style{Color: chrome.text, BorderWidth: 1, BorderColor: chrome.warning, CornerRadius: 4,
						Padding: core.All(8), FontSize: chrome.uiSize(12)}}
			}},
			core.Dynamic(func(*core.Scope) core.Node {
				prompt := w.amendPrompt.Get()
				if prompt == nil {
					return core.View{Style: core.Style{Display: core.DisplayNone}}
				}
				switch {
				case prompt.loading:
					return chrome.label(w.tr("Loading commit message…", "正在读取提交信息…"), chrome.muted, 12)
				case prompt.err != "":
					return core.Text{Content: prompt.err, Role: core.RoleAlert, Style: core.Style{Color: chrome.danger, FontSize: chrome.uiSize(12)}}
				case !w.canConfirmAmend(prompt):
					return core.Text{Content: w.tr("The branch changed. Close this dialog and confirm again.",
						"分支已变化。请关闭此对话框并重新确认。"), Role: core.RoleAlert, Style: core.Style{Color: chrome.danger, FontSize: chrome.uiSize(12)}}
				default:
					return core.View{Style: core.Style{Display: core.DisplayNone}}
				}
			}),
		},
		Footer: []core.Node{action(chrome, w.tr("Cancel", "取消"), cancel), confirm},
	}
}
