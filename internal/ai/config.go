package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Provider string

const (
	OpenAI    Provider = "openai"
	Anthropic Provider = "anthropic"
	Ollama    Provider = "ollama"
)

var (
	ErrNotConfigured = errors.New("AI provider not configured for this Workspace")
	ErrOffline       = errors.New("offline mode requires an HTTP(S) Ollama endpoint on loopback")
	ErrMissingKey    = errors.New("API key not configured")
)

type ProviderConfig struct {
	Provider Provider `json:"provider"`
	Model    string   `json:"model"`
	BaseURL  string   `json:"base_url"`
}

type Configuration struct {
	Provider  Provider         `json:"ai_provider"`
	Model     string           `json:"ai_model"`
	BaseURL   string           `json:"ai_base_url"`
	Offline   bool             `json:"ai_offline"`
	Failover  []ProviderConfig `json:"ai_failover"`
	Templates Templates        `json:"prompt_templates"`
}

type Templates struct {
	Commit   string `json:"commit"`
	Conflict string `json:"conflict"`
	PR       string `json:"pr"`
	Reflog   string `json:"reflog"`
	Health   string `json:"health"`
}

// Normalize validates all edited endpoints before persisting a Workspace.
// An offline configuration can retain cloud choices for a later session.
func (c Configuration) Normalize() (Configuration, error) {
	normalize := func(entry ProviderConfig) (ProviderConfig, error) {
		entry.Provider = Provider(strings.ToLower(strings.TrimSpace(string(entry.Provider))))
		entry.Model = strings.TrimSpace(entry.Model)
		entry.BaseURL = strings.TrimSpace(entry.BaseURL)
		if _, err := chatEndpoint(entry.Provider, entry.BaseURL, false); err != nil {
			return ProviderConfig{}, err
		}
		return entry, nil
	}
	if c.Provider != "" {
		entry, err := normalize(ProviderConfig{Provider: c.Provider, Model: c.Model, BaseURL: c.BaseURL})
		if err != nil {
			return Configuration{}, err
		}
		c.Provider, c.Model, c.BaseURL = entry.Provider, entry.Model, entry.BaseURL
	} else {
		c.Model, c.BaseURL = "", ""
	}
	fallbacks := make([]ProviderConfig, len(c.Failover))
	for i, entry := range c.Failover {
		var err error
		fallbacks[i], err = normalize(entry)
		if err != nil {
			return Configuration{}, fmt.Errorf("AI fallback %d: %w", i+1, err)
		}
	}
	c.Failover = fallbacks
	c.Templates = Templates{
		Commit: strings.TrimSpace(c.Templates.Commit), Conflict: strings.TrimSpace(c.Templates.Conflict),
		PR: strings.TrimSpace(c.Templates.PR), Reflog: strings.TrimSpace(c.Templates.Reflog), Health: strings.TrimSpace(c.Templates.Health),
	}
	return c, nil
}

func (p Provider) DefaultModel() string {
	switch p {
	case OpenAI:
		return "gpt-4o-mini"
	case Anthropic:
		return "claude-3-5-haiku-latest"
	case Ollama:
		return "llama3.2"
	default:
		return ""
	}
}

func (p Provider) DefaultBaseURL() string {
	switch p {
	case OpenAI:
		return "https://api.openai.com"
	case Anthropic:
		return "https://api.anthropic.com"
	case Ollama:
		return "http://127.0.0.1:11434"
	default:
		return ""
	}
}

type KeyLookup func(Provider) (string, error)

type attempt struct {
	provider Provider
	model    string
	endpoint string
	key      string
	err      error
}

func (c Configuration) resolve(ctx context.Context, keys KeyLookup) ([]attempt, error) {
	if strings.TrimSpace(string(c.Provider)) == "" {
		return nil, ErrNotConfigured
	}
	primary := ProviderConfig{Provider: c.Provider, Model: c.Model, BaseURL: c.BaseURL}
	configured := append([]ProviderConfig{primary}, c.Failover...)
	chain := make([]attempt, 0, len(configured))
	for i, entry := range configured {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		provider := Provider(strings.ToLower(strings.TrimSpace(string(entry.Provider))))
		if c.Offline && provider != Ollama {
			continue
		}
		if i > 0 && provider == "" {
			continue
		}
		model := strings.TrimSpace(entry.Model)
		if model == "" {
			model = provider.DefaultModel()
		}
		endpoint, err := chatEndpoint(provider, entry.BaseURL, c.Offline)
		if err != nil {
			if c.Offline {
				continue
			}
			// Validate each online endpoint when its attempt is reached.
			// An invalid optional endpoint must not mask a working primary.
			if i == 0 {
				return nil, err
			}
			chain = append(chain, attempt{provider: provider, err: err})
			continue
		}
		key := ""
		if provider != Ollama {
			if keys != nil {
				key, err = keys(provider)
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
			}
			if err != nil || strings.TrimSpace(key) == "" {
				if i != 0 {
					continue
				}
				if err != nil {
					return nil, err
				}
				return nil, fmt.Errorf("%s: %w", provider, ErrMissingKey)
			}
		}
		chain = append(chain, attempt{provider: provider, model: model, endpoint: endpoint, key: key})
	}
	if len(chain) == 0 {
		return nil, ErrOffline
	}
	return chain, nil
}
