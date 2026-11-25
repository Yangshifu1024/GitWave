package main

import (
	"fmt"
	"slices"
	"strings"

	"gitwave.work/internal/ai"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

var aiProviderOptions = []ui.Option[ai.Provider]{
	{Value: ai.OpenAI, Label: "OpenAI"},
	{Value: ai.Anthropic, Label: "Anthropic"},
	{Value: ai.Ollama, Label: "Ollama"},
}

func (w *workbench) aiSettingsDialog(chrome palette) core.Node {
	return core.Show{When: func() bool { return w.aiSettings.Get() != nil }, Then: func(cx *core.Scope) core.Node {
		form := w.aiSettings.Get()
		title := fmt.Sprintf(w.tr("AI settings · %s", "AI 设置 · %s"), form.workspace.Name)
		disabled := func() bool { return form.busy.Get() || form.loading.Get() }
		modelProvider := core.NewMemo(cx, func() ai.Provider { return form.config.Get().Provider })
		providerOptions := ui.Select[ai.Provider]{TestID: "ai-provider", Label: w.tr("Provider", "服务商"),
			Value: modelProvider.Get, Options: aiProviderOptions, Disabled: disabled,
			OnChange: func(provider ai.Provider) {
				form.edit(func(cfg *ai.Configuration) { cfg.Provider, cfg.Model, cfg.BaseURL = provider, "", "" })
				form.key.Set("")
				w.readAIKeyStatus(form)
			}}
		model := w.aiTextField(chrome, "ai-model", w.tr("Model", "模型"), func() string { return modelProvider.Get().DefaultModel() },
			func() string { return form.config.Get().Model }, func(model string) { form.edit(func(c *ai.Configuration) { c.Model = model }) }, disabled)
		endpoint := w.aiTextField(chrome, "ai-base-url", w.tr("API base URL", "API 基础地址"), func() string { return modelProvider.Get().DefaultBaseURL() },
			func() string { return form.config.Get().BaseURL }, func(url string) { form.edit(func(c *ai.Configuration) { c.BaseURL = url }) }, disabled)
		saveLabel := w.tr("Save", "保存")
		save := toolbarAction(chrome, saveLabel, func() { w.saveAISettings(form) }, chrome.label(saveLabel, chrome.accent, 12))
		save.TestID, save.Disabled = "ai-settings-save", disabled
		closeLabel := w.tr("Close", "关闭")
		close := toolbarAction(chrome, closeLabel, func() { w.closeAISettings(form) }, chrome.label(closeLabel, chrome.text, 12))
		close.Disabled = form.busy.Get
		return ui.Dialog{TestID: "ai-settings-dialog", Title: title,
			Description: w.tr("Choose how this Workspace generates AI suggestions.", "配置此工作区如何生成 AI 建议。"),
			Open:        func() bool { return w.aiSettings.Get() == form },
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					w.closeAISettings(form)
				}
			},
			Style: core.Style{Width: core.Pt(618)}, Footer: []core.Node{close, save},
			Children: []core.Node{
				core.Show{When: form.loading.Get, Then: func(*core.Scope) core.Node {
					return chrome.label(w.tr("Loading…", "正在加载…"), chrome.muted, 12)
				},
					Else: func(*core.Scope) core.Node {
						return core.Scroll{Style: core.Style{Height: core.Pt(415), FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
							core.View{Style: core.Style{Gap: 12}, Children: []core.Node{
								core.Show{When: func() bool { return form.ruleSize.Get() > 0 }, Then: func(*core.Scope) core.Node {
									return settingsHint(chrome, fmt.Sprintf(w.tr("Repository rules from .gitwave/AI.md (%d characters).",
										"已找到 .gitwave/AI.md 仓库规则（%d 字符）。"), form.ruleSize.Get()))
								}},
								core.View{Style: core.Style{Gap: 5}, Children: []core.Node{chrome.label(providerOptions.Label, chrome.text, 12), providerOptions}},
								model, endpoint, w.aiCredentials(chrome, form, modelProvider.Get),
								ui.Checkbox{TestID: "ai-offline", Text: w.tr("Offline mode", "离线模式"), Disabled: disabled,
									Checked:  func() bool { return form.config.Get().Offline },
									OnChange: func(value bool) { form.edit(func(c *ai.Configuration) { c.Offline = value }) }},
								w.aiFallbackList(cx, chrome, form), w.aiPromptTemplates(chrome, form),
							}},
						}}
					}},
				core.Show{When: func() bool { return form.failure.Get() != "" }, Then: func(*core.Scope) core.Node {
					return core.Text{TestID: "ai-settings-error", ContentFunc: form.failure.Get, Role: core.RoleAlert,
						Style: core.Style{Color: chrome.danger, FontSize: chrome.uiSize(12)}}
				}},
				core.Show{When: func() bool { return form.status.Get() != "" }, Then: func(*core.Scope) core.Node {
					return core.Text{TestID: "ai-settings-status", ContentFunc: form.status.Get, Role: core.RoleStatus,
						Style: core.Style{Color: chrome.muted, FontSize: chrome.uiSize(12)}}
				}},
			}}
	}}
}

func (w *workbench) aiTextField(chrome palette, id, label string, placeholder func() string, value func() string,
	onChange func(string), disabled func() bool) core.Node {
	return core.View{Style: core.Style{Gap: 5}, Children: []core.Node{
		chrome.label(label, chrome.text, 12),
		ui.Input{TestID: id, Label: label, PlaceholderFunc: placeholder, Value: value, OnChange: onChange,
			Disabled: disabled, NoAutoCorrect: true, Style: core.Style{FontSize: chrome.uiSize(13)}},
	}}
}

func (w *workbench) aiCredentials(chrome palette, form *aiSettingsForm, provider func() ai.Provider) core.Node {
	return core.Show{When: func() bool { return provider() == ai.Ollama }, Then: func(*core.Scope) core.Node {
		label := w.tr("Detect Ollama", "检测 Ollama")
		detect := toolbarAction(chrome, label, func() { w.detectOllama(form) }, chrome.label(label, chrome.text, 12))
		detect.TestID = "ai-detect-ollama"
		detect.Disabled = func() bool { return form.probing.Get() || form.busy.Get() }
		return core.View{Style: core.Style{Gap: 4, AlignItems: core.AlignStart}, Children: []core.Node{
			detect, core.Show{When: form.probing.Get, Then: func(*core.Scope) core.Node {
				return settingsHint(chrome, w.tr("Checking local Ollama…", "正在检查本地 Ollama…"))
			}},
			core.Show{When: func() bool { return len(form.models.Get()) > 0 }, Then: func(*core.Scope) core.Node {
				return core.Text{TestID: "ai-ollama-models", ContentFunc: func() string { return strings.Join(form.models.Get(), ", ") },
					Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.muted}}
			}},
		}}
	}, Else: func(*core.Scope) core.Node {
		label := w.tr("API key", "API key")
		return core.View{Style: core.Style{Gap: 5}, Children: []core.Node{
			chrome.label(label, chrome.text, 12),
			ui.Input{TestID: "ai-api-key", Label: label, Masked: true, NoAutoCorrect: true,
				Value: form.key.Get, OnChange: form.key.Set, Disabled: form.busy.Get, PlaceholderFunc: func() string {
					if form.hasKey.Get() {
						return w.tr("Leave blank to keep the stored key", "留空以保留已保存的 key")
					}
					return label
				}},
			core.Show{When: func() bool { return form.keyFailure.Get() != "" }, Then: func(*core.Scope) core.Node {
				return core.Text{TestID: "ai-key-status-error", ContentFunc: form.keyFailure.Get, Role: core.RoleAlert,
					Style: core.Style{FontSize: chrome.uiSize(12), Color: chrome.danger}}
			}},
			core.Show{When: form.hasKey.Get, Then: func(*core.Scope) core.Node {
				label := w.tr("Clear stored key", "清除已保存的 key")
				button := toolbarAction(chrome, label, func() { w.clearAIKey(form) }, chrome.label(label, chrome.danger, 12))
				button.TestID, button.Disabled = "ai-clear-key", form.busy.Get
				button.Style.AlignSelf = core.AlignStart
				return button
			}},
		}}
	}}
}

func (w *workbench) aiFallbackList(cx *core.Scope, chrome palette, form *aiSettingsForm) core.Node {
	count := core.NewMemo(cx, func() int { return len(form.config.Get().Failover) })
	return core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
		chrome.label(w.tr("Fallback providers", "备用服务商"), chrome.text, 12),
		settingsHint(chrome, w.tr("Try these in order after a network error.", "网络出错后按顺序尝试。")),
		core.Dynamic(func(*core.Scope) core.Node {
			var rows []core.Node
			for i := range count.Get() {
				get := func() ai.ProviderConfig {
					entries := form.config.Get().Failover
					if i < len(entries) {
						return entries[i]
					}
					return ai.ProviderConfig{}
				}
				move := func(direction int) {
					form.edit(func(cfg *ai.Configuration) {
						j := i + direction
						if i >= 0 && i < len(cfg.Failover) && j >= 0 && j < len(cfg.Failover) {
							cfg.Failover = slices.Clone(cfg.Failover)
							cfg.Failover[i], cfg.Failover[j] = cfg.Failover[j], cfg.Failover[i]
						}
					})
				}
				up := iconAction(chrome, w.tr("Move fallback up", "上移备用服务商"), lucide.ArrowUp(), func() { move(-1) })
				down := iconAction(chrome, w.tr("Move fallback down", "下移备用服务商"), lucide.ArrowDown(), func() { move(1) })
				up.Disabled = func() bool { return form.busy.Get() || i == 0 }
				down.Disabled = func() bool { return form.busy.Get() || i+1 == count.Get() }
				remove := iconAction(chrome, w.tr("Remove fallback", "移除备用服务商"), lucide.X(), func() {
					form.edit(func(cfg *ai.Configuration) {
						if i < len(cfg.Failover) {
							cfg.Failover = slices.Delete(slices.Clone(cfg.Failover), i, i+1)
						}
					})
				})
				remove.Disabled = form.busy.Get
				rows = append(rows, core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
					core.View{Style: core.Style{Direction: core.Row, Gap: 3}, Children: []core.Node{
						ui.Select[ai.Provider]{TestID: fmt.Sprintf("ai-fallback-provider-%d", i), Options: aiProviderOptions,
							Label: fmt.Sprintf(w.tr("Fallback provider %d", "备用服务商 %d"), i+1), Disabled: form.busy.Get,
							Value: func() ai.Provider { return get().Provider }, OnChange: func(p ai.Provider) {
								form.editFallback(i, func(c *ai.ProviderConfig) { *c = ai.ProviderConfig{Provider: p} })
							}},
						up, down, remove,
					}},
					w.aiTextField(chrome, fmt.Sprintf("ai-fallback-model-%d", i), w.tr("Model", "模型"),
						func() string { return get().Provider.DefaultModel() }, func() string { return get().Model },
						func(model string) { form.editFallback(i, func(c *ai.ProviderConfig) { c.Model = model }) }, form.busy.Get),
					w.aiTextField(chrome, fmt.Sprintf("ai-fallback-url-%d", i), w.tr("API base URL", "API 基础地址"),
						func() string { return get().Provider.DefaultBaseURL() }, func() string { return get().BaseURL },
						func(url string) { form.editFallback(i, func(c *ai.ProviderConfig) { c.BaseURL = url }) }, form.busy.Get),
				}})
			}
			return core.View{Style: core.Style{Gap: 10}, Children: rows}
		}),
		action(chrome, w.tr("Add fallback", "添加备用服务商"), func() {
			form.edit(func(cfg *ai.Configuration) {
				cfg.Failover = append(slices.Clone(cfg.Failover), ai.ProviderConfig{Provider: ai.Ollama})
			})
		}),
	}}
}

func (w *workbench) aiPromptTemplates(chrome palette, form *aiSettingsForm) core.Node {
	return core.View{Style: core.Style{Gap: 6}, Children: []core.Node{
		action(chrome, w.tr("Prompt templates", "Prompt 模板"), func() { form.templates.Set(!form.templates.Get()) }),
		core.Show{When: form.templates.Get, Then: func(*core.Scope) core.Node {
			var inputs []core.Node
			for _, field := range []struct {
				id, label string
				value     func(*ai.Templates) *string
			}{
				{"commit", w.tr("Commit message", "提交信息"), func(t *ai.Templates) *string { return &t.Commit }},
				{"conflict", w.tr("Conflict explanation", "冲突解释"), func(t *ai.Templates) *string { return &t.Conflict }},
				{"pr", w.tr("PR description", "PR 描述"), func(t *ai.Templates) *string { return &t.PR }},
				{"reflog", w.tr("Reflog explanation", "Reflog 解释"), func(t *ai.Templates) *string { return &t.Reflog }},
				{"health", w.tr("Repository health summary", "仓库健康总结"), func(t *ai.Templates) *string { return &t.Health }},
			} {
				inputs = append(inputs, chrome.label(field.label, chrome.text, 12),
					ui.Input{TestID: "ai-template-" + field.id, Label: field.label, Multiline: true,
						Style:       core.Style{Height: core.Pt(70), FontSize: chrome.uiSize(12)},
						Placeholder: w.tr("Default prompt", "默认 Prompt"), Disabled: form.busy.Get,
						Value: func() string {
							config := form.config.Get()
							return *field.value(&config.Templates)
						}, OnChange: func(text string) { form.edit(func(cfg *ai.Configuration) { *field.value(&cfg.Templates) = text }) },
					})
			}
			return core.View{Style: core.Style{Gap: 6}, Children: inputs}
		}},
	}}
}
