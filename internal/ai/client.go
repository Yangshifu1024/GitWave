package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	requestTimeout   = 60 * time.Second
	maxResponseBytes = 4 << 20
)

type Client struct {
	online, offline *http.Client
}

func NewClient(proxy func(*http.Request) (*url.URL, error)) *Client {
	if proxy == nil {
		proxy = http.ProxyFromEnvironment
	}
	onlineProxy := func(request *http.Request) (*url.URL, error) {
		if isLoopbackHost(request.URL.Hostname()) {
			return nil, nil
		}
		return proxy(request)
	}
	transport := func(proxy func(*http.Request) (*url.URL, error)) *http.Transport {
		return &http.Transport{
			Proxy:                 proxy,
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			ForceAttemptHTTP2:     true,
			TLSHandshakeTimeout:   10 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			ResponseHeaderTimeout: requestTimeout,
		}
	}
	return &Client{
		online: &http.Client{Transport: transport(onlineProxy), Timeout: requestTimeout,
			CheckRedirect: sameOriginRedirect},
		offline: &http.Client{Transport: transport(nil), Timeout: requestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

func sameOriginRedirect(next *http.Request, previous []*http.Request) error {
	initial := previous[0]
	if len(previous) >= 10 || next.URL.Scheme != initial.URL.Scheme ||
		!strings.EqualFold(next.URL.Host, initial.URL.Host) || next.URL.User != nil || next.Method != initial.Method {
		return http.ErrUseLastResponse
	}
	return nil
}

type Prompt struct {
	System, User, Language string
}

type Outcome struct {
	Text         string
	ProviderUsed Provider
	UsedFallback bool
}

type responseError struct {
	provider  Provider
	message   string
	transient bool
}

func (e *responseError) Error() string {
	return string(e.provider) + ": " + e.message
}

func (c *Client) Generate(ctx context.Context, config Configuration, keys KeyLookup, prompt Prompt) (Outcome, error) {
	chain, err := config.resolve(ctx, keys)
	if err != nil {
		return Outcome{}, err
	}
	system := prompt.System + replyDirective(prompt.Language)
	user := prompt.User
	// A configured key may not match any vendor pattern. Remove it from
	// both prompts before any attempt, including when the chain fails over.
	for _, entry := range chain {
		if entry.key != "" {
			system = strings.ReplaceAll(system, entry.key, redaction)
			user = strings.ReplaceAll(user, entry.key, redaction)
		}
	}
	messages := []message{{Role: "system", Content: Scrub(system)}, {Role: "user", Content: Scrub(user)}}
	client := c.online
	if config.Offline {
		client = c.offline
	}
	var failures []error
	for i, entry := range chain {
		if entry.err != nil {
			return Outcome{}, entry.err
		}
		var last error
		for range 2 {
			text, err := chat(ctx, client, entry, messages)
			if err == nil {
				for _, configured := range chain {
					if configured.key != "" {
						text = strings.ReplaceAll(text, configured.key, redaction)
					}
				}
				return Outcome{Text: text, ProviderUsed: entry.provider, UsedFallback: i > 0}, nil
			}
			var failure *responseError
			if !errors.As(err, &failure) || !failure.transient {
				return Outcome{}, err
			}
			last = err
		}
		failures = append(failures, last)
	}
	if len(failures) == 1 {
		return Outcome{}, failures[0]
	}
	return Outcome{}, fmt.Errorf("all AI providers failed: %w", errors.Join(failures...))
}

func replyDirective(language string) string {
	switch language {
	case "en":
		return "\n\nAlways respond in English."
	case "zh":
		return "\n\nAlways respond in Chinese (简体中文)."
	case "ja":
		return "\n\nAlways respond in Japanese (日本語)."
	case "ko":
		return "\n\nAlways respond in Korean (한국어)."
	default:
		return ""
	}
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type completion struct {
	Choices []struct {
		Message message `json:"message"`
	} `json:"choices"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Message    message `json:"message"`
	StopReason string  `json:"stop_reason"`
}

func chat(ctx context.Context, client *http.Client, entry attempt, messages []message) (string, error) {
	payload := map[string]any{"model": entry.model, "messages": messages}
	switch entry.provider {
	case OpenAI:
		payload["temperature"] = 0.2
	case Anthropic:
		payload["system"] = messages[0].Content
		payload["messages"] = messages[1:]
		payload["max_tokens"] = 32768
	case Ollama:
		payload["stream"] = false
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, entry.endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", &responseError{entry.provider, "invalid chat request", false}
	}
	request.Header.Set("Content-Type", "application/json")
	switch entry.provider {
	case OpenAI:
		request.Header.Set("Authorization", "Bearer "+entry.key)
	case Anthropic:
		request.Header.Set("x-api-key", entry.key)
		request.Header.Set("anthropic-version", "2023-06-01")
	}
	var reply completion
	if err := exchange(client, entry.provider, request, &reply); err != nil {
		return "", err
	}
	var result string
	switch entry.provider {
	case OpenAI:
		if len(reply.Choices) > 0 {
			result = reply.Choices[0].Message.Content
		}
	case Anthropic:
		var text strings.Builder
		for _, block := range reply.Content {
			if block.Type == "" || block.Type == "text" {
				text.WriteString(block.Text)
			}
		}
		result = text.String()
		if strings.TrimSpace(result) == "" && reply.StopReason == "max_tokens" {
			return "", &responseError{entry.provider, "the model reached its token limit before producing text", false}
		}
	case Ollama:
		result = reply.Message.Content
	}
	if result = strings.TrimSpace(result); result == "" {
		return "", &responseError{entry.provider, "response contained no text", false}
	}
	return result, nil
}

// ProbeOllama queries the local model list without proxies or redirects.
func (c *Client) ProbeOllama(ctx context.Context, raw string) ([]string, error) {
	base, err := baseEndpoint(Ollama, raw, true)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, withPath(base, "/api/tags"), nil)
	if err != nil {
		return nil, ErrOffline
	}
	var reply struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := exchange(c.offline, Ollama, request, &reply); err != nil {
		return nil, err
	}
	models := make([]string, 0, len(reply.Models))
	for _, model := range reply.Models {
		if model.Name != "" {
			models = append(models, model.Name)
		}
	}
	return models, nil
}

func exchange(client *http.Client, provider Provider, request *http.Request, reply any) error {
	resp, err := client.Do(request)
	if err != nil {
		if err := request.Context().Err(); err != nil {
			return err
		}
		// Transport diagnostics can contain a configured URL or proxy
		// credentials. Keep them out of display errors and logs.
		return &responseError{provider, "request failed", true}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Provider error bodies can echo headers and prompt content.
		status := resp.StatusCode
		transient := status >= 400 && status != http.StatusBadRequest && status != http.StatusUnprocessableEntity &&
			status != http.StatusUnauthorized && status != http.StatusForbidden
		return &responseError{provider, fmt.Sprintf("HTTP %d %s", status, http.StatusText(status)), transient}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		if err := request.Context().Err(); err != nil {
			return err
		}
		return &responseError{provider, "could not read the response", true}
	}
	if len(raw) > maxResponseBytes {
		return &responseError{provider, "response exceeds the size limit", false}
	}
	if err := json.Unmarshal(raw, reply); err != nil {
		return &responseError{provider, "invalid JSON response", false}
	}
	return nil
}
