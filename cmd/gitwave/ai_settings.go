package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"gitwave.work/internal/ai"
	"gitwave.work/internal/proxy"
	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/core"
)

const aiSecretService = "gitwave.ai"

var aiClient = ai.NewClient(proxy.ForRequest)

type aiSettingsForm struct {
	workspace workspace.Summary
	store     *workspace.Store
	secrets   core.Secrets
	ctx       context.Context
	cancel    context.CancelFunc

	config     *core.Signal[ai.Configuration]
	key        *core.Signal[string]
	hasKey     *core.Signal[bool]
	keyFailure *core.Signal[string]
	models     *core.Signal[[]string]
	ruleSize   *core.Signal[int]
	busy       *core.Signal[bool]
	loading    *core.Signal[bool]
	probing    *core.Signal[bool]
	templates  *core.Signal[bool]
	status     *core.Signal[string]
	failure    *core.Signal[string]
	credential uint64
	probe      uint64
	stopProbe  context.CancelFunc
}

func (w *workbench) openAISettings() {
	ws, store := w.workspaceState.Get().Workspace, w.workspaceStore
	if ws.ID == "" || store == nil || w.refreshBlocked() {
		return
	}
	w.cancelCommitAI()
	w.healthAI.clear()
	w.reflogAI.clear()
	cx := w.cx
	ctx, cancel := context.WithCancel(w.appCtx)
	secrets := cx.Secrets()
	secrets.Service = aiSecretService
	form := &aiSettingsForm{
		workspace: ws, store: store, secrets: secrets, ctx: ctx, cancel: cancel,
		config: core.NewSignalFunc(cx, ai.Configuration{}, nil), key: core.NewSignal(cx, ""),
		hasKey: core.NewSignal(cx, false), keyFailure: core.NewSignal(cx, ""), models: core.NewSignalFunc(cx, []string(nil), nil),
		ruleSize: core.NewSignal(cx, 0),
		busy:     core.NewSignal(cx, false), loading: core.NewSignal(cx, true),
		probing: core.NewSignal(cx, false), templates: core.NewSignal(cx, false),
		status: core.NewSignal(cx, ""), failure: core.NewSignal(cx, ""),
	}
	w.aiSettings.Set(form)
	repo := w.core
	go func() {
		config, err := store.AIConfiguration(ctx, ws.ID)
		ruleSize := 0
		if err == nil && repo != nil {
			ruleSize = utf8.RuneCountInString(repo.repo.AIRules(ctx))
		}
		w.cx.Post(func() {
			if !w.currentAISettings(form) {
				return
			}
			if err != nil {
				form.failure.Set(err.Error())
				return
			}
			if config.Provider.DefaultModel() == "" {
				config.Provider = ai.OpenAI
			}
			form.config.Set(config)
			form.ruleSize.Set(ruleSize)
			form.loading.Set(false)
			w.readAIKeyStatus(form)
		})
	}()
}

func (w *workbench) currentAISettings(form *aiSettingsForm) bool {
	return form.ctx.Err() == nil && w.aiSettings.Get() == form && w.workspaceState.Get().Workspace.ID == form.workspace.ID
}

func (w *workbench) closeAISettings(form *aiSettingsForm) {
	if w.aiSettings.Get() == form && !form.busy.Get() {
		form.cancel()
		form.key.Set("")
		w.aiSettings.Set(nil)
	}
}

func aiSecretAccount(workspaceID string, provider ai.Provider) string {
	return workspaceID + ":" + string(provider)
}

func (w *workbench) readAIKeyStatus(form *aiSettingsForm) {
	form.hasKey.Set(false)
	form.keyFailure.Set("")
	form.credential++
	id, provider := form.credential, form.config.Get().Provider
	if provider == ai.Ollama {
		return
	}
	go func() {
		_, found, err := readAISecret(form.ctx, form.secrets, aiSecretAccount(form.workspace.ID, provider))
		w.cx.Post(func() {
			if w.currentAISettings(form) && form.credential == id {
				form.hasKey.Set(found)
				if errors.Is(err, errLegacyAIKeyAmbiguous) {
					form.keyFailure.Set(w.tr("Several matching keys were found. Resolve the duplicate entries in your system keyring.",
						"发现多条匹配的密钥。请在系统密钥库中处理重复条目。"))
				} else if err != nil {
					form.keyFailure.Set(w.tr("The system keyring could not verify the stored key.", "系统密钥库无法验证已保存的密钥。"))
				}
			}
		})
	}()
}

func (w *workbench) saveAISettings(form *aiSettingsForm) {
	if !w.currentAISettings(form) || form.busy.Get() || form.loading.Get() {
		return
	}
	config, err := form.config.Get().Normalize()
	if err != nil {
		form.failure.Set(err.Error())
		return
	}
	key := strings.TrimSpace(form.key.Get())
	form.busy.Set(true)
	form.failure.Set("")
	form.status.Set("")
	go func() {
		err := form.store.SaveAIConfiguration(form.ctx, form.workspace.ID, config)
		if err == nil && key != "" && config.Provider != ai.Ollama {
			if err = form.secrets.Set(aiSecretAccount(form.workspace.ID, config.Provider), key); err != nil {
				err = fmt.Errorf("AI settings were saved, but the system could not store the API key")
			}
		}
		w.cx.Post(func() {
			if !w.currentAISettings(form) {
				return
			}
			form.busy.Set(false)
			if err != nil {
				form.failure.Set(err.Error())
				return
			}
			form.config.Set(config)
			form.key.Set("")
			form.status.Set(w.tr("AI settings saved.", "AI 设置已保存。"))
			w.readAIKeyStatus(form)
		})
	}()
}

func (w *workbench) clearAIKey(form *aiSettingsForm) {
	if !w.currentAISettings(form) || form.busy.Get() {
		return
	}
	provider := form.config.Get().Provider
	form.busy.Set(true)
	form.failure.Set("")
	form.status.Set("")
	go func() {
		err := deleteAISecret(form.ctx, form.secrets, aiSecretAccount(form.workspace.ID, provider))
		w.cx.Post(func() {
			if !w.currentAISettings(form) {
				return
			}
			form.busy.Set(false)
			if err != nil {
				form.failure.Set(w.tr("The system could not remove the stored API key.", "系统未能清除已保存的 API key。"))
				return
			}
			form.key.Set("")
			form.status.Set(w.tr("Stored API key removed.", "已清除保存的 API key。"))
			w.readAIKeyStatus(form)
		})
	}()
}

func (w *workbench) detectOllama(form *aiSettingsForm) {
	if !w.currentAISettings(form) || form.probing.Get() {
		return
	}
	raw := form.config.Get().BaseURL
	ctx, cancel := context.WithCancel(form.ctx)
	form.stopProbe = cancel
	form.probe++
	id := form.probe
	form.probing.Set(true)
	form.status.Set("")
	form.failure.Set("")
	form.models.Set(nil)
	go func() {
		defer cancel()
		models, err := aiClient.ProbeOllama(ctx, raw)
		w.cx.Post(func() {
			if !w.currentAISettings(form) || form.probe != id {
				return
			}
			form.stopProbe = nil
			form.probing.Set(false)
			if err != nil {
				form.failure.Set(err.Error())
				return
			}
			form.models.Set(models)
			if len(models) == 0 {
				form.status.Set(w.tr("Ollama has no installed models.", "Ollama 尚未安装模型。"))
			} else {
				form.status.Set(fmt.Sprintf(w.tr("Found %d Ollama models.", "检测到 %d 个 Ollama 模型。"), len(models)))
			}
		})
	}()
}

func (f *aiSettingsForm) edit(edit func(*ai.Configuration)) {
	if f.busy.Get() {
		return
	}
	config := f.config.Get()
	provider, baseURL := config.Provider, config.BaseURL
	edit(&config)
	if config.Provider != provider || config.BaseURL != baseURL {
		if f.stopProbe != nil {
			f.stopProbe()
			f.stopProbe = nil
		}
		f.probe++
		f.probing.Set(false)
		f.models.Set(nil)
	}
	f.config.Set(config)
	f.status.Set("")
}

func (f *aiSettingsForm) editFallback(i int, edit func(*ai.ProviderConfig)) {
	f.edit(func(config *ai.Configuration) {
		if i < 0 || i >= len(config.Failover) {
			return
		}
		config.Failover = slices.Clone(config.Failover)
		edit(&config.Failover[i])
	})
}
