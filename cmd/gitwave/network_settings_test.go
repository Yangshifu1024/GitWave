package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gitwave.work/internal/ai"

	"gitwave.work/internal/proxy"
)

func TestNativeNetworkSettingsPersistAndValidateProxy(t *testing.T) {
	store := settingsStore(t)
	saved := proxy.Settings{Mode: proxy.Manual, ManualURL: "http://127.0.0.1:9000"}
	if err := store.SaveProxySettings(t.Context(), saved); err != nil {
		t.Fatal(err)
	}
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	const address = "id:settings-proxy-address"
	app.WaitFor("label:/^Settings test repository,/", timeout)
	app.Key("mod+,")
	app.WaitFor("id:settings-network", timeout)
	app.Press("id:settings-network")
	app.WaitFor(address, timeout)
	if !app.Find("id:settings-proxy-manual").Props.Checked {
		t.Fatal("the saved manual proxy mode was not selected")
	}
	// GTK's test host blurs the field when it synthesizes a second focus click.
	// Reject an edit to the saved value in one typing step.
	app.Type(address, ":bad\n")
	app.WaitFor("id:settings-proxy-error", timeout)
	if got, err := store.ProxySettings(t.Context()); err != nil || got != saved {
		t.Fatalf("the invalid address changed saved settings: %+v, %v", got, err)
	}
	app.Clear(address)
	app.Key("enter")
	app.WaitUntil(timeout, func() bool {
		prefs, err := store.ProxySettings(t.Context())
		return err == nil && prefs.Mode == proxy.Manual && prefs.ManualURL == ""
	})
	app.Type(address, "127.0.0.1:7890/")
	app.Press("id:settings-proxy-off")
	app.WaitUntil(timeout, func() bool {
		prefs, err := store.ProxySettings(t.Context())
		return err == nil && prefs.Mode == proxy.Off && prefs.ManualURL == "http://127.0.0.1:7890"
	})
	app.WaitForGone(address, timeout)
	app.Press("id:settings-proxy-manual")
	app.WaitFor(address, timeout)
	app.WaitUntil(timeout, func() bool { return app.Find(address).Text == "http://127.0.0.1:7890" })
}

func TestNativeNetworkSettingsKeepsTheLastChoiceDuringAWrite(t *testing.T) {
	store := settingsStore(t)
	if err := store.SaveProxySettings(t.Context(), proxy.Settings{Mode: proxy.Manual, ManualURL: "http://manual.invalid:8090"}); err != nil {
		t.Fatal(err)
	}
	blocker, err := sql.Open("sqlite", filepath.Join(store.DataDirectory(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Close()
	blocker.SetMaxOpenConns(1)
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	app.WaitFor("label:/^Settings test repository,/", timeout)
	app.Key("mod+,")
	app.WaitFor("id:settings-network", timeout)
	app.Press("id:settings-network")
	app.WaitFor("id:settings-proxy-address", timeout)
	if _, err := blocker.ExecContext(t.Context(), "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	defer blocker.Exec("ROLLBACK")
	app.Press("id:settings-proxy-off")
	app.Press("id:settings-proxy-manual")
	if _, err := blocker.ExecContext(t.Context(), "ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	app.WaitFor("id:settings-proxy-saved", timeout)
	app.WaitUntil(timeout, func() bool {
		saved, err := store.ProxySettings(t.Context())
		return err == nil && saved.Mode == proxy.Manual && app.Find("id:settings-proxy-manual").Props.Checked
	})
}

func TestWorkspaceAIUsesManualProxyAndKeepsOfflineDirect(t *testing.T) {
	t.Setenv("no_proxy", "*")
	const host = "ai-proxy-fixture.invalid"
	var proxyHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host || r.URL.Scheme != "http" || r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		proxyHits.Add(1)
		_, _ = fmt.Fprint(w, `{"message":{"content":"Via proxy"}}`)
	}))
	defer server.Close()
	a := workspaceAI{proxy: proxy.Settings{Mode: proxy.Manual, ManualURL: server.URL}}
	config := ai.Configuration{Provider: ai.Ollama, BaseURL: "http://" + host}
	if outcome, err := a.generate(t.Context(), config, ai.Prompt{User: "fixture"}); err != nil || outcome.Text != "Via proxy" {
		t.Fatalf("AI request through manual proxy = %+v, %v", outcome, err)
	}
	direct := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"message":{"content":"Direct"}}`)
	}))
	defer direct.Close()
	config.BaseURL, config.Offline = direct.URL, true
	if outcome, err := a.generate(t.Context(), config, ai.Prompt{User: "fixture"}); err != nil || outcome.Text != "Direct" || proxyHits.Load() != 1 {
		t.Fatalf("offline AI request = %+v, %v, proxy requests = %d", outcome, err, proxyHits.Load())
	}
}
