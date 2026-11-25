package gitcli

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// HTTPSCredentialVault is the application's private fallback for its
// remembered HTTPS credentials. Git's configured helpers run first.
type HTTPSCredentialVault interface {
	Lookup(ctx context.Context, endpoint, username string) (HTTPSCredential, bool, error)
	Store(ctx context.Context, endpoint string, credential HTTPSCredential) error
}

var ErrSavedHTTPSCredential = errors.New("the saved HTTPS credential for this account is unavailable")

var credentialConfigQuote = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\t", `\t`)

// Copy only account selection and path matching. A private transport can
// use the configured system helper without executing helpers from local config.
func (r *Repository) credentialContextFile(ctx context.Context, dir string, endpoints ...string) (string, error) {
	var config strings.Builder
	for _, endpoint := range endpoints {
		if !strings.HasPrefix(endpoint, "https://") {
			continue
		}
		values, err := r.readLimited(ctx, "", 64<<10, "config", "--null", "--get-urlmatch", "credential", endpoint)
		if err != nil && !gitExitCode(err, 1) {
			return "", fmt.Errorf("could not read the HTTPS credential settings: %w", err)
		}
		var settings strings.Builder
		for record := range strings.SplitSeq(string(values), "\x00") {
			key, value, assigned := strings.Cut(record, "\n")
			switch key {
			case "credential.username":
				if !assigned {
					continue
				}
			case "credential.usehttppath":
				if !assigned {
					value = "true"
				}
			default:
				continue
			}
			if strings.ContainsAny(value, "\x00\r\n") {
				continue
			}
			fmt.Fprintf(&settings, "\t%s = \"%s\"\n", strings.TrimPrefix(key, "credential."), credentialConfigQuote.Replace(value))
		}
		if settings.Len() > 0 {
			fmt.Fprintf(&config, "[credential \"%s\"]\n%s", credentialConfigQuote.Replace(endpoint), settings.String())
		}
	}
	if config.Len() == 0 {
		return "", nil
	}
	path := filepath.Join(dir, "credential-context")
	return path, os.WriteFile(path, []byte(config.String()), 0o600)
}

func (s *transportRepo) bindCredentialContext(ctx context.Context, endpoints ...string) error {
	file, err := s.repo.credentialContextFile(ctx, s.dir, endpoints...)
	if err != nil || file == "" {
		return err
	}
	return s.addConfig(ctx, "include.path", file)
}

func (e *RemoteAuthError) credentialEndpoint() string {
	if e.media != "" {
		return e.media
	}
	return e.endpoint
}

// ResolveHTTPSCredential uses the named account in the selected challenge
// when the user submits the authentication form with an empty token.
func ResolveHTTPSCredential(ctx context.Context, vault HTTPSCredentialVault, challenge *RemoteAuthError, input *HTTPSCredential) (*HTTPSCredential, error) {
	if input == nil || input.Password != "" {
		return input, nil
	}
	if vault == nil || challenge == nil || input.Username == "" {
		return nil, ErrSavedHTTPSCredential
	}
	credential, found, err := vault.Lookup(ctx, challenge.credentialEndpoint(), input.Username)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil || !found || credential.Username != input.Username {
		return nil, ErrSavedHTTPSCredential
	}
	credential.Remember = input.Remember
	return &credential, nil
}

func retryWithHTTPSVault[T any](ctx context.Context, git, dir string, options FetchOptions, run func(FetchOptions) (T, error)) (T, error) {
	if options.Vault == nil || options.NonInteractive {
		return run(options)
	}
	seen := make(map[RemoteAuthError]struct{})
	if options.Challenge != nil && options.Credentials != nil {
		seen[*options.Challenge] = struct{}{}
	}
	for {
		result, err := run(options)
		var challenge *RemoteAuthError
		if ctx.Err() != nil || !errors.As(err, &challenge) {
			return result, err
		}
		if _, tried := seen[*challenge]; tried {
			return result, err
		}
		seen[*challenge] = struct{}{}
		endpoint := challenge.credentialEndpoint()
		address, parseErr := url.Parse(endpoint)
		if parseErr != nil || address.Scheme != "https" || address.Host == "" || address.User != nil {
			return result, err
		}
		username, _ := readGit(ctx, git, dir, nil, "", 8<<10, "config", "--get-urlmatch", "credential.username", endpoint)
		credential, found, lookupErr := options.Vault.Lookup(ctx, endpoint, strings.TrimSuffix(string(username), "\n"))
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if lookupErr != nil || !found {
			return result, err
		}
		options.Challenge, options.Credentials = challenge, &credential
	}
}
