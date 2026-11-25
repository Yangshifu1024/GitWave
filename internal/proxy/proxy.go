package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/net/http/httpproxy"
)

type Mode string

const (
	System Mode = "system"
	Manual Mode = "manual"
	Off    Mode = "off"
)

// Settings uses the same format as the released desktop app's proxy entry.
type Settings struct {
	Mode      Mode   `json:"mode"`
	ManualURL string `json:"manual_url,omitempty"`
}

func (s Settings) Normalize() (Settings, error) {
	s.ManualURL = strings.TrimSpace(s.ManualURL)
	switch s.Mode {
	case "", System:
		s.Mode = System
	case Off:
	case Manual:
		if s.ManualURL != "" {
			s.ManualURL = proxyURL(s.ManualURL)
			if s.ManualURL == "" {
				return Settings{}, errors.New("enter a valid HTTP or HTTPS proxy address")
			}
		}
	default:
		return Settings{}, errors.New("invalid proxy mode")
	}
	return s, nil
}

func proxyURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	address, err := url.Parse(raw)
	if err != nil || address.Hostname() == "" || (address.Scheme != "http" && address.Scheme != "https") {
		return ""
	}
	if port := address.Port(); port != "" {
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return ""
		}
	}
	return strings.TrimRight(address.String(), "/")
}

type systemProxy struct {
	http, https string
	bypass      []string
}

type policy struct {
	resolve   func(*url.URL) (*url.URL, error)
	variables []string
}

type policyKey struct{}

// WithSettings binds one route policy to a network operation. OS discovery
// waits until the first request and happens once even if the operation retries.
func WithSettings(ctx context.Context, settings Settings) context.Context {
	return context.WithValue(ctx, policyKey{}, sync.OnceValue(func() policy {
		var system systemProxy
		if settings.Mode == "" || settings.Mode == System {
			system = detectSystem(ctx)
		}
		return resolve(settings, system, os.Getenv)
	}))
}

func route(ctx context.Context) (policy, bool) {
	get, ok := ctx.Value(policyKey{}).(func() policy)
	if !ok {
		return policy{}, false
	}
	return get(), true
}

func ForRequest(request *http.Request) (*url.URL, error) {
	if p, ok := route(request.Context()); ok {
		return p.resolve(request.URL)
	}
	return http.ProxyFromEnvironment(request)
}

// CommandEnvironment adds proxy settings only to a network subprocess.
func CommandEnvironment(ctx context.Context, env []string) []string {
	p, ok := route(ctx)
	if !ok || len(p.variables) == 0 {
		return env
	}
	return append(slices.DeleteFunc(slices.Clone(env), func(entry string) bool {
		name, _, _ := strings.Cut(entry, "=")
		switch strings.ToLower(name) {
		case "http_proxy", "https_proxy", "no_proxy":
			return true
		}
		return false
	}), p.variables...)
}

func resolve(settings Settings, system systemProxy, getenv func(string) string) policy {
	variable := func(name string) string {
		if value := getenv(strings.ToLower(name)); value != "" {
			return value
		}
		return getenv(name)
	}
	httpProxy, httpsProxy := variable("HTTP_PROXY"), variable("HTTPS_PROXY")
	noProxy, allProxy := variable("NO_PROXY"), variable("ALL_PROXY")
	injected := false
	if settings.Mode == Manual && settings.ManualURL != "" {
		httpProxy, httpsProxy = settings.ManualURL, settings.ManualURL
		noProxy = ""
		injected = true
	} else if settings.Mode == "" || settings.Mode == System {
		if httpProxy == "" && allProxy == "" && system.http != "" {
			httpProxy, injected = system.http, true
		}
		if httpsProxy == "" && allProxy == "" && system.https != "" {
			httpsProxy, injected = system.https, true
		}
	}
	var p policy
	if injected {
		bypass := append([]string{"localhost", "127.0.0.1", "127.0.0.0/8", "::1"}, strings.Split(noProxy, ",")...)
		if settings.Mode != Manual {
			bypass = append(bypass, system.bypass...)
		}
		noProxy = strings.Join(distinctEntries(bypass), ",")
		for _, entry := range []struct{ name, value string }{
			{"HTTP_PROXY", httpProxy}, {"HTTPS_PROXY", httpsProxy}, {"NO_PROXY", noProxy},
		} {
			// Git uses libcurl and Git LFS uses Go. Provide both cases on Unix.
			p.variables = append(p.variables, entry.name+"="+entry.value, strings.ToLower(entry.name)+"="+entry.value)
		}
	}
	if httpProxy == "" {
		httpProxy = allProxy
	}
	if httpsProxy == "" {
		httpsProxy = allProxy
	}
	config := httpproxy.Config{HTTPProxy: httpProxy, HTTPSProxy: httpsProxy, NoProxy: noProxy}
	p.resolve = config.ProxyFunc()
	return p
}

func bypassEntries(raw string) []string {
	entries := strings.FieldsFunc(raw, func(r rune) bool { return r == ';' || r == ',' })
	for i, entry := range entries {
		entry = strings.TrimPrefix(strings.TrimSpace(entry), "*.")
		// macOS may report shortened IPv4 prefixes such as 169.254/16.
		if address, bits, ok := strings.Cut(entry, "/"); ok && !strings.Contains(address, ":") {
			if missing := 3 - strings.Count(address, "."); missing > 0 {
				if prefix, err := netip.ParsePrefix(address + strings.Repeat(".0", missing) + "/" + bits); err == nil {
					entry = prefix.Masked().String()
				}
			}
		}
		entries[i] = entry
	}
	return distinctEntries(entries)
}

func distinctEntries(entries []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		key := strings.ToLower(entry)
		if entry != "" && key != "<local>" && !seen[key] {
			seen[key] = true
			result = append(result, entry)
		}
	}
	return result
}
