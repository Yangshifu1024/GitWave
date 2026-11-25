package main

import (
	"context"
	"strings"

	"gitwave.work/internal/ai"
	"gitwave.work/internal/proxy"
	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/core"
)

type workspaceAI struct {
	id       string
	store    *workspace.Store
	secrets  core.Secrets
	language string
	proxy    proxy.Settings
}

func (w *workbench) workspaceAI() workspaceAI {
	secrets := w.cx.Secrets()
	secrets.Service = aiSecretService
	return workspaceAI{
		id: w.workspaceState.Get().Workspace.ID, store: w.workspaceStore, secrets: secrets,
		language: w.desktopPrefs.Get().AILanguage, proxy: w.proxySettings.Get(),
	}
}

func (a workspaceAI) configured(ctx context.Context) (ai.Configuration, error) {
	if a.store == nil || a.id == "" {
		return ai.Configuration{}, ai.ErrNotConfigured
	}
	config, err := a.store.AIConfiguration(ctx, a.id)
	if err == nil && strings.TrimSpace(string(config.Provider)) == "" {
		err = ai.ErrNotConfigured
	}
	return config, err
}

func (a workspaceAI) generate(ctx context.Context, cfg ai.Configuration, prompt ai.Prompt) (ai.Outcome, error) {
	prompt.Language = a.language
	return aiClient.Generate(proxy.WithSettings(ctx, a.proxy), cfg, func(provider ai.Provider) (string, error) {
		value, _, err := readAISecret(ctx, a.secrets, aiSecretAccount(a.id, provider))
		return value, err
	}, prompt)
}
