package gitcli

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Preserve the confirmed repository's HTTP proxy settings in the isolated
// transport. Copy scalar settings only, without including the live config.
func (r *Repository) proxyContextFile(ctx context.Context, dir, remote string, endpoints ...string) (string, []string, error) {
	var config strings.Builder
	var proxies []string
	for i, endpoint := range endpoints {
		if (!strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://")) ||
			slices.Contains(endpoints[:i], endpoint) {
			continue
		}
		var section strings.Builder
		for _, field := range []string{"proxy", "proxyAuthMethod"} {
			var value string
			var found bool
			var err error
			if i == 0 && remote != "" {
				value, found, err = r.readProxyConfig(ctx, "--get", "remote."+remote+"."+field)
			}
			if err == nil && !found {
				value, found, err = r.readProxyConfig(ctx, "--get-urlmatch", "http."+field, endpoint)
			}
			if err != nil {
				return "", nil, err
			}
			if found {
				fmt.Fprintf(&section, "\t%s = \"%s\"\n", field, credentialConfigQuote.Replace(value))
				if field == "proxy" && value != "" {
					proxies = append(proxies, value)
				}
			}
		}
		if section.Len() > 0 {
			fmt.Fprintf(&config, "[http \"%s\"]\n%s", credentialConfigQuote.Replace(endpoint), section.String())
		}
	}
	if config.Len() == 0 {
		return "", nil, nil
	}
	path := filepath.Join(dir, "proxy-context")
	return path, proxies, os.WriteFile(path, []byte(config.String()), 0o600)
}

func (r *Repository) readProxyConfig(ctx context.Context, keys ...string) (string, bool, error) {
	args := append([]string{"config", "--null"}, keys...)
	raw, err := r.readLimited(ctx, "", 64<<10, args...)
	if gitExitCode(err, 1) {
		return "", false, nil
	}
	if ctx.Err() != nil {
		return "", false, ctx.Err()
	}
	value := strings.TrimSuffix(string(raw), "\x00")
	if err != nil || strings.ContainsAny(value, "\x00\r\n") {
		return "", false, errors.New("could not read the repository's HTTP proxy settings")
	}
	return value, true, nil
}

func (s *transportRepo) bindProxyContext(ctx context.Context, remote string, endpoints ...string) error {
	file, proxies, err := s.repo.proxyContextFile(ctx, s.dir, remote, endpoints...)
	if err != nil || file == "" {
		return err
	}
	s.proxyURLs = proxies
	return s.addConfig(ctx, "include.path", file)
}

func redactProxyDiagnostic(message string, environment, configured []string) string {
	proxies := slices.Clone(configured)
	for _, entry := range environment {
		key, value, _ := strings.Cut(entry, "=")
		switch strings.ToLower(key) {
		case "http_proxy", "https_proxy", "all_proxy":
			if value != "" && !slices.Contains(proxies, value) {
				proxies = append(proxies, value)
			}
		}
	}
	for _, address := range proxies {
		if !strings.Contains(address, "://") {
			address = "http://" + address
		}
		parsed, err := url.Parse(address)
		if err != nil || parsed.User == nil {
			continue
		}
		password, _ := parsed.User.Password()
		if password == "" {
			if parsed.User.Username() != "" {
				// A Git credential helper may supply a password we cannot redact.
				return "network request failed using an authenticated proxy"
			}
			continue
		}
		// A proxy can reflect decoded credentials or the Basic header in a response.
		basic := base64.StdEncoding.EncodeToString([]byte(parsed.User.Username() + ":" + password))
		_, escapedPassword, _ := strings.Cut(parsed.User.String(), ":")
		for _, secret := range []string{basic, parsed.User.String(), escapedPassword, password} {
			message = strings.ReplaceAll(message, secret, "[redacted]")
		}
	}
	return redactLFSURLs(message)
}
