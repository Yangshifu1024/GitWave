package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientProviderProtocolsAndScrubbing(t *testing.T) {
	for _, tc := range []struct {
		provider Provider
		path     string
		reply    string
	}{
		{OpenAI, "/gateway/v1/chat/completions", `{"choices":[{"message":{"content":"  Summary  "}}]}`},
		{Anthropic, "/gateway/v1/messages", `{"content":[{"type":"thinking","text":"ignored"},{"type":"text","text":"Sum"},{"type":"text","text":"mary"}]}`},
		{Ollama, "/gateway/api/chat", `{"message":{"content":"Summary"}}`},
	} {
		t.Run(string(tc.provider), func(t *testing.T) {
			const key = "fixture-value-96"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != tc.path || r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("request = %s %s %q", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
				}
				var input struct {
					Model       string    `json:"model"`
					System      string    `json:"system"`
					Messages    []message `json:"messages"`
					Temperature float64   `json:"temperature"`
					MaxTokens   int       `json:"max_tokens"`
					Stream      *bool     `json:"stream"`
				}
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				system := input.System
				if input.Model != tc.provider.DefaultModel() {
					t.Errorf("model = %q", input.Model)
				}
				if tc.provider == Anthropic {
					if r.Header.Get("x-api-key") != key || r.Header.Get("anthropic-version") != "2023-06-01" ||
						input.MaxTokens != 32768 || len(input.Messages) != 1 {
						t.Error("invalid Anthropic request")
					}
				} else if len(input.Messages) == 2 {
					system = input.Messages[0].Content
				} else {
					t.Error("expected system and user messages")
				}
				if tc.provider == OpenAI && (r.Header.Get("Authorization") != "Bearer "+key || input.Temperature != 0.2) {
					t.Error("invalid OpenAI request")
				}
				if tc.provider == Ollama && (r.Header.Get("Authorization") != "" || input.Stream == nil || *input.Stream) {
					t.Error("invalid Ollama request")
				}
				if !strings.Contains(system, "Japanese (日本語)") || strings.Contains(system, "private sample") {
					t.Errorf("unexpected system prompt: %q", system)
				}
				if len(input.Messages) > 0 {
					user := input.Messages[len(input.Messages)-1].Content
					if !strings.Contains(user, "keep context") || strings.Contains(user, "opaque-value") ||
						strings.Contains(user, "compatibility-key-value") || strings.Contains(user, "YWxpY2U6cGFzcw==") ||
						strings.Contains(user, "local-user:local-pass") ||
						(tc.provider != Ollama && strings.Contains(user, key)) {
						t.Errorf("unexpected user prompt: %q", user)
					}
				}
				_, _ = fmt.Fprint(w, tc.reply)
			}))
			defer server.Close()
			got, err := NewClient(nil).Generate(t.Context(), Configuration{Provider: tc.provider, BaseURL: server.URL + "/gateway/"}, func(p Provider) (string, error) {
				if p == Ollama {
					t.Error("Ollama tried to read a key")
				}
				return key, nil
			}, Prompt{
				System: "Be concise.\npassword=private sample", Language: "ja",
				User: "keep context\nToken : opaque-value\n" + key + "\nX-API-Key: compatibility-key-value\n" +
					"Authorization: Basic YWxpY2U6cGFzcw==\nhttp://local-user:local-pass@fixture.invalid",
			})
			if err != nil || got != (Outcome{Text: "Summary", ProviderUsed: tc.provider}) {
				t.Fatalf("generated %+v, %v", got, err)
			}
		})
	}
}

func TestFailoverRetriesNetworkErrorsAndStopsOnCredentials(t *testing.T) {
	var first, second atomic.Int32
	var status atomic.Int32
	status.Store(http.StatusTooManyRequests)
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		first.Add(1)
		w.WriteHeader(int(status.Load()))
		_, _ = fmt.Fprint(w, "fixture credential echoed by provider")
	}))
	defer primary.Close()
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		second.Add(1)
		var input struct{ Model string }
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Model != "fallback-model" {
			t.Errorf("fallback model = %q, %v", input.Model, err)
		}
		_, _ = fmt.Fprint(w, `{"message":{"content":"fallback"}}`)
	}))
	defer fallback.Close()
	config := Configuration{Provider: OpenAI, BaseURL: primary.URL, Failover: []ProviderConfig{
		{Provider: Anthropic}, {Provider: Ollama, BaseURL: fallback.URL, Model: "fallback-model"},
	}}
	key := func(p Provider) (string, error) {
		if p == OpenAI {
			return "fixture credential echoed by provider", nil
		}
		return "", nil
	}
	client := NewClient(nil)
	got, err := client.Generate(t.Context(), config, key, Prompt{})
	if err != nil || !got.UsedFallback || got.ProviderUsed != Ollama || got.Text != "fallback" || first.Load() != 2 || second.Load() != 1 {
		t.Fatalf("fallback = %+v, %v; attempts = %d/%d", got, err, first.Load(), second.Load())
	}
	status.Store(http.StatusUnauthorized)
	_, err = client.Generate(t.Context(), config, key, Prompt{})
	if err == nil || !strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "fixture credential") ||
		first.Load() != 3 || second.Load() != 1 {
		t.Fatalf("auth error = %v; attempts = %d/%d", err, first.Load(), second.Load())
	}
}

func TestOfflineOllamaIgnoresProxyKeysAndRedirects(t *testing.T) {
	var proxyHits, destinationHits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		destinationHits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = fmt.Fprint(w, `{"models":[{"name":"local:latest"},{}]}`)
		case "/api/chat":
			_, _ = fmt.Fprint(w, `{"message":{"content":"local"}}`)
		default:
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
		}
	}))
	defer server.Close()
	client := NewClient(func(*http.Request) (*url.URL, error) {
		proxyHits.Add(1)
		return url.Parse(target.URL)
	})
	local := strings.Replace(server.URL, "http://127.0.0.1:", "localhost:", 1)
	config := Configuration{Offline: true, Provider: OpenAI, Failover: []ProviderConfig{
		{Provider: Ollama, BaseURL: "http://192.0.2.1"}, {Provider: Anthropic}, {Provider: Ollama, BaseURL: local},
	}}
	got, err := client.Generate(t.Context(), config, func(p Provider) (string, error) {
		t.Errorf("offline request asked for %q key", p)
		return "", nil
	}, Prompt{})
	if err != nil || got.ProviderUsed != Ollama || got.Text != "local" {
		t.Fatalf("offline result = %+v, %v", got, err)
	}
	if models, err := client.ProbeOllama(t.Context(), local); err != nil || len(models) != 1 || models[0] != "local:latest" {
		t.Fatalf("probe = %v, %v", models, err)
	}
	if _, err := client.ProbeOllama(t.Context(), local+"/redirect"); err == nil ||
		proxyHits.Load() != 0 || destinationHits.Load() != 0 {
		t.Fatalf("redirect error = %v; proxy = %d, destination = %d", err, proxyHits.Load(), destinationHits.Load())
	}
	for _, invalid := range []string{
		"http://192.0.2.1", "http://localhost.example", "http://localhost@192.0.2.1",
		"http://localhost/#@", "http://localhost/?", "ftp://localhost", "http://127.0.0.1%2Fexample.test",
	} {
		if _, err := client.ProbeOllama(t.Context(), invalid); !errors.Is(err, ErrOffline) {
			t.Errorf("unexpected outcome for %q: %v", invalid, err)
		}
	}
}

func TestCloudRedirectDoesNotForwardAnthropicKey(t *testing.T) {
	var reached atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached.Store(true)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer destination.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	_, err := NewClient(nil).Generate(t.Context(), Configuration{Provider: Anthropic, BaseURL: origin.URL},
		func(Provider) (string, error) { return "fixture-key", nil }, Prompt{})
	if err == nil || reached.Load() {
		t.Fatalf("cross-origin redirect followed: %v, %v", reached.Load(), err)
	}
}

func TestCancelInFlightAIRequest(t *testing.T) {
	started, released := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input map[string]any
		_ = json.NewDecoder(r.Body).Decode(&input)
		close(started)
		select {
		case <-r.Context().Done():
		case <-released:
		}
	}))
	defer server.Close()
	defer close(released)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := NewClient(nil).Generate(ctx, Configuration{Provider: Ollama, BaseURL: server.URL}, nil, Prompt{})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled request returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request did not cancel")
	}
}

func TestSecretScrubbingPreservesOrdinaryContext(t *testing.T) {
	patch := "+-----begin OPENSSH private KEY-----\n+short payload\n@@ hunk @@\n+tail\n" +
		"diff --git a/code b/code\n+normal change\n+const tokenCount = 2\n" +
		"@@ other @@\n+AbCdEfGhIjKlMnOpQrStUvWxYz0123456789\n+tiny==\n+return result\n"
	scrubbed := Scrub(patch)
	for _, denied := range []string{"short payload", "tail", "PRIVATE KEY", "AbCd", "tiny", "OPENSSH"} {
		if strings.Contains(scrubbed, denied) {
			t.Errorf("left %q in the output", denied)
		}
	}
	for _, expected := range []string{"+normal change", "+const tokenCount = 2", "+return result", "diff --git"} {
		if !strings.Contains(scrubbed, expected) {
			t.Errorf("removed %q from the output", expected)
		}
	}
	if !ContainsPrivateKey([]byte("prefix private key-----\nshort")) || ContainsPrivateKey([]byte("ordinary filename")) {
		t.Fatal("unexpected full blob classification")
	}
}
