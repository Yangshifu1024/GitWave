package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
)

const remoteSecretService = "gitwave.remote"

type remoteCredentials struct {
	secrets core.Secrets
}

func (w *workbench) remoteCredentials() remoteCredentials {
	secrets := w.cx.Secrets()
	secrets.Service = remoteSecretService
	return remoteCredentials{secrets: secrets}
}

func (s remoteCredentials) Lookup(ctx context.Context, endpoint, username string) (gitcli.HTTPSCredential, bool, error) {
	if err := ctx.Err(); err != nil {
		return gitcli.HTTPSCredential{}, false, err
	}
	path, host, err := remoteCredentialScope(endpoint)
	if err != nil {
		return gitcli.HTTPSCredential{}, false, err
	}
	selected := username
	if selected == "" {
		selected, _, err = s.secrets.Get(remoteCredentialKey("selected", path))
		if err != nil {
			return gitcli.HTTPSCredential{}, false, err
		}
	}
	if selected != "" {
		value, found, err := s.secrets.Get(remoteCredentialKey("account", path+"\n"+selected))
		if err != nil || found {
			return decodeRemoteCredential(value, selected, found, err)
		}
	}
	if host == "" {
		return gitcli.HTTPSCredential{}, false, nil
	}
	if selected != "" {
		value, found, err := readKeyringSecret(ctx, s.secrets, "https/"+host+"\x1f"+selected)
		if err != nil || found {
			auth, found, err := decodeRemoteCredential(value, selected, found, err)
			auth.Remember = found
			return auth, found, err
		}
	}
	value, found, err := readKeyringSecret(ctx, s.secrets, "https/"+host)
	auth, found, err := decodeRemoteCredential(value, selected, found, err)
	auth.Remember = found
	return auth, found, err
}

func (s remoteCredentials) Store(ctx context.Context, endpoint string, auth gitcli.HTTPSCredential) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path, _, err := remoteCredentialScope(endpoint)
	if err != nil {
		return err
	}
	if auth.Username == "" || auth.Password == "" || strings.ContainsAny(auth.Username+auth.Password, "\x00\r\n") {
		return errors.New("invalid HTTPS credential")
	}
	key, record := remoteCredentialKey("account", path+"\n"+auth.Username), auth.Username+"\n"+auth.Password
	old, found, err := s.secrets.Get(key)
	if err != nil {
		return err
	}
	if !found || old != record {
		if err := s.secrets.Set(key, record); err != nil {
			return err
		}
	}
	key = remoteCredentialKey("selected", path)
	old, found, err = s.secrets.Get(key)
	if err != nil || found && old == auth.Username {
		return err
	}
	return s.secrets.Set(key, auth.Username)
}

func remoteCredentialScope(endpoint string) (string, string, error) {
	address, err := url.Parse(endpoint)
	if err != nil || address.Scheme != "https" || address.Hostname() == "" || address.User != nil ||
		address.RawQuery != "" || address.ForceQuery || address.Fragment != "" {
		return "", "", errors.New("invalid HTTPS credential endpoint")
	}
	authority := strings.ToLower(address.Host)
	path := "https://" + authority + address.EscapedPath()
	legacyHost := authority
	if strings.HasPrefix(legacyHost, "[") {
		// The legacy unbracketed format cannot distinguish an IPv6 suffix from a port.
		legacyHost = ""
	}
	return path, strings.TrimRight(legacyHost, "."), nil
}

func remoteCredentialKey(kind, scope string) string {
	return fmt.Sprintf("https-v2/%s/%x", kind, sha256.Sum256([]byte(scope)))
}

func decodeRemoteCredential(value, username string, found bool, err error) (gitcli.HTTPSCredential, bool, error) {
	if err != nil || !found {
		return gitcli.HTTPSCredential{}, false, err
	}
	user, password, ok := strings.Cut(value, "\n")
	if !ok || user == "" || password == "" || strings.ContainsAny(user+password, "\x00\r\n") {
		return gitcli.HTTPSCredential{}, false, errors.New("invalid password record in the system keyring")
	}
	if username != "" && username != user {
		return gitcli.HTTPSCredential{}, false, nil
	}
	return gitcli.HTTPSCredential{Username: user, Password: password}, true, nil
}
