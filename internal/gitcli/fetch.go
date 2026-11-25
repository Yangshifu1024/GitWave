package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
)

type FetchOptions struct {
	Remote         string
	Expected       *RemoteInfo
	Challenge      *RemoteAuthError
	Credentials    *HTTPSCredential
	Vault          HTTPSCredentialVault
	NonInteractive bool
	Progress       func(string, TransferProgress)
}

type FetchResult struct {
	Completed []string
	Warnings  []string
}

type remoteSource struct{ endpoint, protocol string }

func (r *Repository) remoteEndpoint(ctx context.Context, name string) (remoteSource, error) {
	if _, err := r.checkedRefName(ctx, "remotes", name+"/probe"); err != nil {
		return remoteSource{}, err
	}
	out, err := r.readLimited(ctx, "", 64<<10, "remote", "get-url", "--", name)
	if err != nil {
		return remoteSource{}, fmt.Errorf("could not read the URL for remote %q: %w", name, err)
	}
	effective := strings.TrimSuffix(string(out), "\n")
	endpoint, protocol, err := parseTransportSource(effective, r.Path())
	if err != nil {
		return remoteSource{}, err
	}
	return remoteSource{endpoint: endpoint, protocol: protocol}, nil
}

// Fetch updates remote-tracking refs and Git-managed tags. It attempts each
// selected remote independently, stopping early for cancellation or auth.
func (r *Repository) Fetch(ctx context.Context, requested FetchOptions) (FetchResult, error) {
	var result FetchResult
	err := r.withRefEdit(ctx, func() error {
		items, err := r.Remotes(ctx)
		if err != nil {
			return err
		}
		slices.SortFunc(items, func(a, b RemoteInfo) int { return strings.Compare(a.Name, b.Name) })
		name := requested.Remote
		if requested.Challenge != nil {
			if name != "" && name != requested.Challenge.Remote {
				return fmt.Errorf("the authentication prompt belongs to another remote")
			}
			name = requested.Challenge.Remote
		} else if requested.Credentials != nil {
			return fmt.Errorf("retry authentication for a specific remote")
		}
		if name == "" && requested.Expected != nil {
			name = requested.Expected.Name
		}
		if name != "" {
			i := slices.IndexFunc(items, func(item RemoteInfo) bool { return item.Name == name })
			if i < 0 {
				return fmt.Errorf("remote %q is no longer configured", name)
			}
			if requested.Expected != nil {
				if requested.Expected.Name != name {
					return fmt.Errorf("the selected remote does not match this fetch")
				}
			}
			items = items[i : i+1]
		}
		var first error
		for _, item := range items {
			if err := ctx.Err(); err != nil {
				return err
			}
			name := item.Name
			attempt := requested
			if attempt.Expected == nil {
				attempt.Expected = &item
			}
			warning, err := retryWithHTTPSVault(ctx, r.git, r.Path(), attempt, func(transfer FetchOptions) (string, error) {
				return r.fetchRemote(ctx, name, transfer, nil)
			})
			if err == nil {
				result.Completed = append(result.Completed, name)
				if warning != "" {
					result.Warnings = append(result.Warnings, warning)
				}
				continue
			}
			if ctx.Err() != nil || errors.Is(err, ErrHTTPSAuthentication) {
				return err
			}
			if first == nil {
				first = fmt.Errorf("remote %q: %w", name, err)
			}
		}
		return first
	})
	return result, err
}

func (r *Repository) fetchRemote(ctx context.Context, name string, requested FetchOptions, selected *pullSource) (string, error) {
	expected := requested.Expected
	if expected == nil {
		item, err := r.remoteByName(ctx, name)
		if err != nil {
			return "", err
		}
		expected = &item
	}
	if expected.Name != name {
		return "", fmt.Errorf("the selected remote does not match this fetch")
	}
	endpoint, protocol, err := parseTransportSource(expected.endpoint, r.Path())
	if err != nil {
		return "", err
	}
	if challenge := requested.Challenge; challenge != nil {
		if challenge.operation != "fetch" || challenge.endpoint != endpoint ||
			challenge.media != "" && (selected == nil || challenge.media != selected.lfs.httpEndpoint()) {
			return "", fmt.Errorf("the remote URL changed since authentication was requested")
		}
	}
	refspecs, err := validateFetchRefspecs(name, expected.urls("fetch"))
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "gitwave-fetch-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	stage, err := r.prepareFetchStage(ctx, dir)
	if err != nil {
		return "", err
	}
	if err := stage.bindCredentialContext(ctx, endpoint); err != nil {
		return "", err
	}
	if err := stage.bindProxyContext(ctx, name, endpoint); err != nil {
		return "", err
	}
	if selected != nil {
		selected.target.Ref, err = pullTrackingReference(name, selected.branch, refspecs)
		if err != nil {
			return "", err
		}
		// Pull integrates the current branch through the checkout plan.
		refspecs = slices.DeleteFunc(refspecs, func(spec string) bool {
			return localFetchDestination(spec) == selected.local
		})
	}
	alias, err := stage.bindEndpoint(ctx, endpoint)
	if err != nil {
		return "", err
	}
	if requested.Progress != nil {
		requested.Progress(name, TransferProgress{})
	}
	if selected != nil && selected.lfs.enabled() {
		if err := selected.lfs.verify(ctx, r, name, remoteSource{endpoint: endpoint, protocol: protocol}); err != nil {
			return "", err
		}
	}
	reportProgress := func(p TransferProgress) {
		if requested.Progress != nil {
			requested.Progress(name, p)
		}
	}
	transport, err := prepareTransport(endpoint, protocol, requested.Credentials, reportProgress)
	if err != nil {
		return "", err
	}
	defer transport.close()
	if requested.NonInteractive {
		transport.options = append(transport.options, "-c", "credential.interactive=never", "-c", "core.askPass=")
		transport.env = append(transport.env, "GCM_INTERACTIVE=Never", "GIT_ASKPASS=", "SSH_ASKPASS=", "SSH_ASKPASS_REQUIRE=never")
	}
	if _, err := r.matchRemote(ctx, *expected); err != nil {
		return "", err
	}
	args := append([]string{}, transport.options...)
	args = append(args, "fetch", "--no-all", "--atomic", "--prune", "--no-prune-tags", "--no-recurse-submodules",
		"--no-auto-maintenance", "--no-write-commit-graph", "--progress", "--refmap=",
		"--upload-pack=git-upload-pack", "--", alias)
	// An explicit source and refmap keep Git from reloading a configured
	// destination refspec after it has been validated.
	args = append(args, refspecs...)
	const privatePullRef = "refs/gitwave/pull-source"
	mediaTransport := transport
	var verifyMedia func(context.Context) error
	if selected != nil {
		// The configured refspecs can map two different sources to the
		// same tracking ref. Fetch the selected source independently.
		args = append(args, "+"+selected.branch+":"+privatePullRef)
	}
	if err := transport.run(ctx, stage.command(ctx, "", args...), "fetch", stage.proxyURLs...); err != nil {
		if errors.Is(err, ErrHTTPSAuthentication) {
			return "", remoteAuthError(name, endpoint, "fetch")
		}
		return "", err
	}
	if selected != nil {
		selected.target.OID, err = stage.commitTip(ctx, privatePullRef)
		if err != nil {
			return "", err
		}
		if selected.target.Ref != selected.local {
			trackingTip, err := stage.commitTip(ctx, selected.target.Ref)
			if err != nil {
				return "", err
			}
			if !validOID(selected.target.OID) || trackingTip != selected.target.OID {
				return "", fmt.Errorf("the selected remote branch does not match its fetched tracking reference")
			}
		}
		if selected.lfs.enabled() && validOID(selected.target.OID) {
			mediaTransport, err = selected.lfs.mediaTransport(transport, selected.lfsAuth, reportProgress)
			if err != nil {
				return "", err
			}
			if mediaTransport != transport {
				defer mediaTransport.close()
			}
			if err := selected.lfs.download(ctx, stage.transportRepo, name, selected.target.OID,
				mediaTransport, expected, &selected.receipt); err != nil {
				if errors.Is(err, ErrHTTPSAuthentication) {
					return "", lfsAuthError(name, endpoint, mediaTransport.remote)
				}
				return "", err
			}
			if selected.receipt.directory != nil {
				verifyMedia = func(ctx context.Context) error {
					if err := selected.lfs.verify(ctx, r, name, selected.lfs.source); err != nil {
						return err
					}
					return selected.receipt.verify(ctx)
				}
			}
		}
	}
	warning, err := stage.publish(ctx, name, expected, refspecs, verifyMedia)
	if err != nil {
		return "", err
	}
	warning = joinCheckoutWarning(warning, transport.remember(ctx, r.git, dir, requested.Vault))
	if mediaTransport != transport {
		warning = joinCheckoutWarning(warning, mediaTransport.remember(ctx, r.git, dir, requested.Vault))
	}
	return warning, nil
}

func (r *Repository) fetchRefspecs(ctx context.Context, name string) ([]string, error) {
	item, err := r.remoteByName(ctx, name)
	if err != nil {
		return nil, err
	}
	return validateFetchRefspecs(name, item.urls("fetch"))
}

func (r *Repository) remoteByName(ctx context.Context, name string) (RemoteInfo, error) {
	items, err := r.readRemotes(ctx, name)
	if err != nil {
		return RemoteInfo{}, err
	}
	for _, item := range items {
		if item.Name == name {
			return item, nil
		}
	}
	return RemoteInfo{}, fmt.Errorf("remote %q is no longer configured", name)
}

func validateFetchRefspecs(name string, refspecs []string) ([]string, error) {
	if len(refspecs) == 0 {
		return []string{"HEAD"}, nil
	}
	for i, spec := range refspecs {
		_, dest, _ := strings.Cut(spec, ":")
		if dest == "" {
			continue
		}
		if !fetchDestination(dest) && localFetchDestination(spec) == "" {
			return nil, fmt.Errorf("fetch refspec for %q must target a tracking ref, tag or an explicit fast-forward local branch", name)
		}
		if strings.HasPrefix(dest, "refs/tags/") && !strings.HasPrefix(spec, "+") {
			// A conflicting shared tag can be received in the private
			// repository. Publication still preserves the existing tag.
			refspecs[i] = "+" + spec
		}
	}
	return refspecs, nil
}

func localFetchDestination(spec string) string {
	source, target, mapped := strings.Cut(spec, ":")
	if !mapped || !strings.HasPrefix(source, "refs/heads/") || !strings.HasPrefix(target, "refs/heads/") ||
		strings.ContainsRune(spec, '*') {
		return ""
	}
	return target
}

func fetchDestination(ref string) bool {
	return strings.HasPrefix(ref, "refs/remotes/") || strings.HasPrefix(ref, "refs/tags/")
}
