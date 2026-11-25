package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitwave.work/internal/ai"

	"github.com/nsgo-dev/nsgo/core"
)

func TestNativeAISettingsDetectsAndPersistsOllama(t *testing.T) {
	store := settingsStore(t)
	state, err := store.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	writeRepositoryFixtureFile(t, state.Repo.Path, ".gitwave/AI.md", "Prefer short messages.\n")
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet || req.URL.Path != "/api/tags" {
			t.Errorf("probe = %s %s", req.Method, req.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = fmt.Fprint(w, `{"models":[{"name":"local-fixture"}]}`)
	}))
	defer ollama.Close()
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	app.WaitFor("label:/^Settings test repository,/", timeout)
	app.ChooseMenu("Workspace", "AI")
	app.WaitFor("id:ai-provider", timeout)
	app.WaitFor("text:/Repository rules from/", timeout)
	app.Press("id:ai-provider")
	app.WaitFor("id:ai-provider-2", timeout)
	app.Press("id:ai-provider-2")
	app.WaitFor("id:ai-detect-ollama", timeout)
	app.Type("id:ai-base-url", ollama.URL)
	app.Press("id:ai-detect-ollama")
	app.WaitFor("id:ai-ollama-models", timeout)
	if got := app.Find("id:ai-ollama-models").Text; !strings.Contains(got, "local-fixture") {
		t.Fatalf("detected models = %q", got)
	}
	app.Type("id:ai-model", "local-fixture")
	app.Press("id:ai-offline")
	app.Press("id:ai-settings-save")
	app.WaitFor("text:AI settings saved.", timeout)
	config, err := store.AIConfiguration(t.Context(), state.Workspace.ID)
	if err != nil || config.Provider != ai.Ollama || !config.Offline || config.Model != "local-fixture" || config.BaseURL != ollama.URL {
		t.Fatalf("saved AI = %+v, %v", config, err)
	}
	app.Press("id:ai-settings-dialog >> label:Close")
	app.WaitForGone("id:ai-settings-dialog", timeout)
	app.ChooseMenu("Workspace", "AI")
	app.WaitFor("id:ai-detect-ollama", timeout)
	if !app.Find("id:ai-offline").Props.Checked || app.Find("id:ai-model").Text != "local-fixture" {
		t.Fatal("saved Ollama settings did not reload")
	}
}

func TestNativeAISettingsStoresAndClearsKeyInSecretStore(t *testing.T) {
	store := settingsStore(t)
	state, err := store.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	app.WaitFor("label:/^Settings test repository,/", timeout)
	app.ChooseMenu("Workspace", "AI")
	app.WaitFor("id:ai-api-key", timeout)
	if !app.Find("id:ai-api-key").Input.Masked {
		t.Fatal("provider key input is visible")
	}
	const key = "fixture-key"
	app.Type("id:ai-api-key", key)
	app.Press("id:ai-settings-save")
	app.WaitFor("id:ai-clear-key", timeout)
	item := core.SecretItem{Service: aiSecretService, Account: aiSecretAccount(state.Workspace.ID, ai.OpenAI)}
	secret, found, err := app.Secrets.ReadSecret(item)
	if err != nil || !found || secret != key {
		t.Fatalf("expected fixture key in secure store: found = %v, err = %v", found, err)
	}
	app.Press("id:ai-clear-key")
	app.WaitFor("text:Stored API key removed.", timeout)
	if _, found, err := app.Secrets.ReadSecret(item); err != nil || found {
		t.Fatalf("expected stored key to be removed: found = %v, err = %v", found, err)
	}
}
