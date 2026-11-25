package ai

import (
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

var errEndpoint = errors.New("AI endpoint must be an HTTP(S) URL without credentials, a query or a fragment")

func baseEndpoint(provider Provider, raw string, localOnly bool) (*url.URL, error) {
	base := strings.TrimSpace(raw)
	if base == "" {
		base = provider.DefaultBaseURL()
	}
	if provider == Ollama && !strings.Contains(base, "://") {
		base = "http://" + base
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" ||
		parsed.RawQuery != "" || parsed.ForceQuery || strings.Contains(base, "#") ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		if localOnly {
			return nil, ErrOffline
		}
		return nil, errEndpoint
	}
	host := parsed.Hostname()
	localhost := strings.EqualFold(host, "localhost")
	loopback := isLoopbackHost(host)
	if localOnly && !loopback {
		return nil, ErrOffline
	}
	if provider != Ollama && parsed.Scheme != "https" && !loopback {
		return nil, errors.New("cloud AI requires HTTPS outside loopback")
	}
	if localhost && (localOnly || parsed.Scheme == "http") {
		port := parsed.Port()
		parsed.Host = "127.0.0.1"
		if port != "" {
			parsed.Host = net.JoinHostPort(parsed.Host, port)
		}
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = strings.TrimRight(parsed.RawPath, "/")
	return parsed, nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.Zone() == "" && ip.IsLoopback()
}

func withPath(base *url.URL, suffix string) string {
	u := *base
	u.Path += suffix
	if u.RawPath != "" {
		u.RawPath += suffix
	}
	return u.String()
}

func chatEndpoint(provider Provider, raw string, offline bool) (string, error) {
	if provider.DefaultModel() == "" {
		return "", errors.New("unsupported AI provider")
	}
	base, err := baseEndpoint(provider, raw, offline)
	if err != nil {
		return "", err
	}
	if provider == Ollama {
		return withPath(base, "/api/chat"), nil
	}
	suffix := "/chat/completions"
	if provider == Anthropic {
		suffix = "/messages"
	}
	switch {
	case strings.HasSuffix(base.Path, suffix):
		return base.String(), nil
	case strings.HasSuffix(base.Path, "/v1"):
		return withPath(base, suffix), nil
	default:
		return withPath(base, "/v1"+suffix), nil
	}
}
