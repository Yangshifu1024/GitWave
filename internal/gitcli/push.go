package gitcli

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

type PushRemote struct {
	Name    string
	sources []remoteSource
	local   []localLFSTransport
	lfs     lfsRemoteTarget
	err     error
}

func (r PushRemote) DestinationCount() int { return len(r.sources) }
func (r PushRemote) LFSUploadURL() string  { return r.lfs.url }

type PushOptions struct {
	Remote         PushRemote
	Branch         CheckoutTarget
	Tags           []CheckoutTarget
	RequireCurrent bool
	Force          bool
}

type PushResult struct {
	Attempted   bool
	Branches    int
	SkippedTags []string
	Warnings    []string
}

type PushLease struct {
	Destination string
	Ref         string
	Before      string
	After       string
}

type PushReview struct {
	Changes []PushLease
}

type PushPlan struct {
	mu          sync.Mutex
	repo        *Repository
	options     PushOptions
	shallow     string
	tracking    CheckoutTarget
	review      *PushReview
	tips        []map[string]string
	credentials map[string]pushAuthentication
	next        int
	branchDone  bool
	tagsDone    map[string]bool
	result      PushResult
}

type pushAuthentication struct{ git, media *HTTPSCredential }

func (r *Repository) PushRemotes(ctx context.Context) ([]PushRemote, error) {
	names, err := r.RemoteNames(ctx)
	if err != nil {
		return nil, err
	}
	remotes := make([]PushRemote, 0, len(names))
	for _, name := range names {
		sources, err := r.pushEndpoints(ctx, name)
		lfs := r.lfsPushTarget(ctx, name)
		local := make([]localLFSTransport, 0, len(sources))
		for _, source := range sources {
			local = append(local, inspectLocalLFSTransport(ctx, source, lfs))
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		remotes = append(remotes, PushRemote{Name: name, sources: sources, local: local, lfs: lfs, err: err})
	}
	return remotes, nil
}

func (r *Repository) pushEndpoints(ctx context.Context, name string) ([]remoteSource, error) {
	if _, err := r.checkedRefName(ctx, "remotes", name+"/probe"); err != nil {
		return nil, err
	}
	before, limit, err := r.pushURLConfig(ctx, name)
	if err != nil {
		return nil, err
	}
	out, err := r.readLimited(ctx, "", 1<<20, "remote", "get-url", "--push", "--all", "--", name)
	if err != nil {
		return nil, fmt.Errorf("could not read the push URL for remote %q: %w", name, err)
	}
	after, _, err := r.pushURLConfig(ctx, name)
	if err != nil {
		return nil, err
	}
	if before != after {
		return nil, fmt.Errorf("the remote URL configuration changed while selecting push destinations")
	}
	addresses := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if limit != 0 && len(addresses) > limit {
		return nil, fmt.Errorf("Git reported more push destinations than were configured")
	}
	var sources []remoteSource
	for _, address := range addresses {
		endpoint, protocol, err := parseTransportSource(address, r.Path())
		if err != nil {
			return nil, err
		}
		source := remoteSource{endpoint: endpoint, protocol: protocol}
		if !slices.Contains(sources, source) {
			sources = append(sources, source)
		}
	}
	return sources, nil
}

func (r *Repository) pushURLConfig(ctx context.Context, name string) (string, int, error) {
	pattern := "^remote[.]" + regexp.QuoteMeta(name) + "[.](url|pushurl)$|^url[.].*[.](insteadof|pushinsteadof)$"
	raw, err := r.readLimited(ctx, "", 4<<20, "config", "--null", "--get-regexp", pattern)
	if err != nil && !gitExitCode(err, 1) {
		return "", 0, err
	}
	var fetch, push int
	for record := range strings.SplitSeq(strings.TrimSuffix(string(raw), "\x00"), "\x00") {
		if record == "" {
			continue
		}
		key, value, ok := strings.Cut(record, "\n")
		if !ok || strings.ContainsAny(key, "\r") || strings.ContainsAny(value, "\r\n") {
			return "", 0, fmt.Errorf("a configured push URL or rewrite contains a line break")
		}
		switch key {
		case "remote." + name + ".pushurl":
			if value == "" {
				push = 0
			} else {
				push++
			}
		case "remote." + name + ".url":
			if value == "" {
				fetch = 0
			} else {
				fetch++
			}
		}
	}
	if push != 0 {
		return string(raw), push, nil
	}
	if fetch == 0 {
		legacy, err := r.readLegacyRemotes(ctx)
		if err != nil {
			return "", 0, err
		}
		for _, entry := range legacy[name].config {
			if entry.key == "url" {
				raw = append(raw, entry.value...)
				raw = append(raw, 0)
				fetch++
			}
		}
	}
	return string(raw), fetch, nil
}

func (r *Repository) PreparePush(ctx context.Context, options PushOptions) (*PushPlan, error) {
	if options.Remote.err != nil {
		return nil, options.Remote.err
	}
	if len(options.Remote.sources) == 0 {
		return nil, fmt.Errorf("select a remote with a configured push URL")
	}
	options.Tags = slices.Clone(options.Tags)
	options.Remote.sources = slices.Clone(options.Remote.sources)
	options.Remote.local = slices.Clone(options.Remote.local)
	plan := &PushPlan{
		repo: r, options: options, tips: make([]map[string]string, len(options.Remote.sources)),
		credentials: make(map[string]pushAuthentication), tagsDone: make(map[string]bool),
	}
	refs, err := plan.verify(ctx)
	if err != nil {
		return nil, err
	}
	plan.shallow = refs.Shallow
	// A distinct push URL may point at a repository unrelated to the
	// fetch URL. Only update local tracking when the destinations match.
	if len(options.Remote.sources) == 1 {
		if fetch, err := r.remoteEndpoint(ctx, options.Remote.Name); err == nil && fetch == options.Remote.sources[0] {
			if specs, err := r.fetchRefspecs(ctx, options.Remote.Name); err == nil {
				if name, err := pullTrackingReference(options.Remote.Name, options.Branch.Ref, specs); err == nil &&
					strings.HasPrefix(name, "refs/remotes/") &&
					uniqueTrackingSource(options.Branch.Ref, name, specs) {
					ref := findRef(refs, name)
					if ref.Symbolic == "" {
						plan.tracking = CheckoutTarget{Ref: name, OID: ref.OID}
					}
				}
			}
		}
	}
	return plan, nil
}

func (p *PushPlan) verify(ctx context.Context) (References, error) {
	r := p.repo
	opts := p.options
	sources, err := r.pushEndpoints(ctx, opts.Remote.Name)
	if err != nil {
		return References{}, err
	}
	if !slices.Equal(sources, opts.Remote.sources) {
		return References{}, fmt.Errorf("the push destination changed since it was selected")
	}
	if current := r.lfsPushTarget(ctx, opts.Remote.Name); current != opts.Remote.lfs {
		return References{}, fmt.Errorf("the Git LFS upload configuration changed since the push destination was selected")
	}
	if len(opts.Remote.local) != len(sources) {
		return References{}, fmt.Errorf("select the push destination again")
	}
	for i, source := range sources {
		if i >= p.next && !opts.Remote.local[i].matches(inspectLocalLFSTransport(ctx, source, opts.Remote.lfs)) {
			return References{}, fmt.Errorf("a selected local Git or LFS destination changed before pushing")
		}
	}
	refs, err := r.References(ctx)
	if err != nil {
		return References{}, err
	}
	if !strings.HasPrefix(opts.Branch.Ref, "refs/heads/") || !validOID(opts.Branch.OID) {
		return References{}, fmt.Errorf("select a local branch with a commit to push")
	}
	if err := checkRefTip(refs, opts.Branch.Ref, opts.Branch.OID); err != nil {
		return References{}, err
	}
	if opts.RequireCurrent && (refs.HeadRef != opts.Branch.Ref || refs.Head != opts.Branch.OID) {
		return References{}, fmt.Errorf("the checked-out branch changed before pushing")
	}
	seen := make(map[string]bool)
	for _, tag := range opts.Tags {
		if !strings.HasPrefix(tag.Ref, "refs/tags/") || !validOID(tag.OID) || seen[tag.Ref] {
			return References{}, fmt.Errorf("invalid tag selected for push")
		}
		seen[tag.Ref] = true
		if err := checkRefTip(refs, tag.Ref, tag.OID); err != nil {
			return References{}, err
		}
		tip, err := r.ReferenceCommit(ctx, findRef(refs, tag.Ref))
		if err != nil {
			return References{}, err
		}
		if tip != opts.Branch.OID {
			return References{}, fmt.Errorf("only tags on the selected branch tip can be pushed")
		}
	}
	return refs, nil
}

// Review obtains the current object IDs before a force push is approved.
// A later Apply uses this exact snapshot as explicit Git leases.
func (p *PushPlan) Review(ctx context.Context, transfer FetchOptions) (*PushReview, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return retryWithHTTPSVault(ctx, p.repo.git, p.repo.Path(), transfer, func(attempt FetchOptions) (*PushReview, error) {
		return p.reviewRemoteTips(ctx, attempt)
	})
}

func (p *PushPlan) reviewRemoteTips(ctx context.Context, transfer FetchOptions) (*PushReview, error) {
	if !p.options.Force {
		return nil, fmt.Errorf("force push review requires force push to be selected")
	}
	err := p.repo.withRefEdit(ctx, func() error {
		if _, err := p.verify(ctx); err != nil {
			return err
		}
		if err := p.acceptCredentials(transfer); err != nil {
			return err
		}
		for i, source := range p.options.Remote.sources {
			if p.tips[i] != nil {
				continue
			}
			err := p.withDestination(ctx, i, source, transfer, func(d pushDestination) error {
				before, err := d.readTips(ctx, p.targets())
				if err != nil {
					return err
				}
				p.tips[i] = before
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	review := &PushReview{}
	for i, source := range p.options.Remote.sources {
		for _, ref := range p.targets() {
			review.Changes = append(review.Changes, PushLease{
				Destination: source.endpoint, Ref: ref.Ref, Before: p.tips[i][ref.Ref], After: ref.OID,
			})
		}
	}
	p.review = review
	return review, nil
}

func (p *PushPlan) Apply(ctx context.Context, transfer FetchOptions, approved *PushReview) (PushResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return retryWithHTTPSVault(ctx, p.repo.git, p.repo.Path(), transfer, func(attempt FetchOptions) (PushResult, error) {
		return p.apply(ctx, attempt, approved)
	})
}

func (p *PushPlan) apply(ctx context.Context, transfer FetchOptions, approved *PushReview) (PushResult, error) {
	err := p.repo.withRefEdit(ctx, func() error {
		if p.options.Force && (approved == nil || approved != p.review) {
			return fmt.Errorf("review and confirm the remote references before forcing a push")
		}
		if _, err := p.verify(ctx); err != nil {
			return err
		}
		if err := p.acceptCredentials(transfer); err != nil {
			return err
		}
		for p.next < len(p.options.Remote.sources) {
			i := p.next
			source := p.options.Remote.sources[i]
			err := p.withDestination(ctx, i, source, transfer, func(d pushDestination) error {
				if !p.branchDone {
					p.result.Attempted = true
					report, err := d.push(ctx, []CheckoutTarget{p.options.Branch}, p.tips[i])
					if len(report.accepted) != 0 {
						p.result.Branches++
						p.branchDone = true
					}
					if err != nil {
						return err
					}
				}
				tags := make([]CheckoutTarget, 0, len(p.options.Tags))
				for _, tag := range p.options.Tags {
					if !p.tagsDone[tag.Ref] {
						tags = append(tags, tag)
					}
				}
				if len(tags) != 0 {
					report, err := d.push(ctx, tags, p.tips[i])
					for _, ref := range report.accepted {
						p.tagsDone[ref] = true
					}
					for _, ref := range report.skipped {
						p.tagsDone[ref] = true
						name := strings.TrimPrefix(ref, "refs/tags/")
						if !slices.Contains(p.result.SkippedTags, name) {
							p.result.SkippedTags = append(p.result.SkippedTags, name)
						}
					}
					if err != nil {
						return err
					}
				}
				if warning := d.transport.remember(ctx, p.repo.git, d.scratch, transfer.Vault); warning != "" {
					p.result.Warnings = append(p.result.Warnings, warning)
				}
				if d.media != nil {
					if warning := d.media.remember(ctx, p.repo.git, d.scratch, transfer.Vault); warning != "" {
						p.result.Warnings = append(p.result.Warnings, warning)
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
			p.next++
			p.branchDone = false
			clear(p.tagsDone)
			delete(p.credentials, source.endpoint)
		}
		return nil
	})
	if p.result.Branches == len(p.options.Remote.sources) && p.tracking.Ref != "" {
		warning := p.updateTracking(ctx)
		if warning != "" {
			p.result.Warnings = append(p.result.Warnings, warning)
		}
		p.tracking = CheckoutTarget{}
	}
	result := p.result
	result.SkippedTags = slices.Clone(result.SkippedTags)
	result.Warnings = slices.Clone(result.Warnings)
	return result, err
}

func (p *PushPlan) targets() []CheckoutTarget {
	return append([]CheckoutTarget{p.options.Branch}, p.options.Tags...)
}

func (p *PushPlan) acceptCredentials(options FetchOptions) error {
	if options.Remote != "" && options.Remote != p.options.Remote.Name {
		return fmt.Errorf("the requested remote does not match the push plan")
	}
	if options.Credentials == nil {
		return nil
	}
	challenge := options.Challenge
	if challenge == nil || challenge.operation != "push" || challenge.Remote != p.options.Remote.Name {
		return fmt.Errorf("the authentication prompt belongs to another operation")
	}
	for _, source := range p.options.Remote.sources {
		if source.endpoint == challenge.endpoint {
			auth := p.credentials[source.endpoint]
			if challenge.media == "" {
				auth.git = options.Credentials
			} else {
				media, protocol := pushMediaEndpoint(source, p.options.Remote.lfs)
				if challenge.media != media || protocol != "https" {
					return fmt.Errorf("the Git LFS destination changed since authentication was requested")
				}
				auth.media = options.Credentials
			}
			p.credentials[source.endpoint] = auth
			return nil
		}
	}
	return fmt.Errorf("the push destination changed since authentication was requested")
}

func (p *PushPlan) withDestination(ctx context.Context, index int, source remoteSource, options FetchOptions,
	run func(pushDestination) error) error {
	return p.repo.withPushDestination(ctx, source, p.shallow, p.options.Remote.Name, "push",
		p.options.Remote.lfs, p.options.Remote.local[index],
		p.credentials[source.endpoint], options, run)
}

func (r *Repository) withPushDestination(ctx context.Context, source remoteSource, shallow, remote, operation string,
	lfs lfsRemoteTarget, local localLFSTransport, auth pushAuthentication, options FetchOptions, run func(pushDestination) error) error {
	scratch, err := os.MkdirTemp("", "gitwave-push-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratch)
	stage, err := r.prepareTransportRepo(ctx, scratch, shallow)
	if err != nil {
		return err
	}
	alias, err := stage.bindEndpoint(ctx, source.endpoint)
	if err != nil {
		return err
	}
	name := "gitwave-push-" + rand.Text()
	for _, key := range []string{"remote." + name + ".url", "remote." + name + ".pushurl"} {
		if err := stage.addConfig(ctx, key, alias); err != nil {
			return err
		}
	}
	cmd := stage.command(ctx, "", "remote", "get-url", "--push", "--all", "--", name)
	resolved, err := readGitOutput(ctx, cmd, 64<<10, "verify push endpoint")
	if err != nil {
		return err
	}
	if string(resolved) != source.endpoint+"\n" {
		return fmt.Errorf("the isolated push URL does not match the selected destination")
	}
	if source.protocol == "http" || source.protocol == "https" {
		// A redirect could transfer the refs to a different repository.
		// Override URL-specific user config as well as the common default.
		if err := stage.addConfig(ctx, "http."+source.endpoint+".followRedirects", "false"); err != nil {
			return err
		}
	}
	if options.Progress != nil {
		options.Progress(remote, TransferProgress{})
	}
	var credentialURLs []string
	var media *transportAttempt
	lfsURL, lfsProtocol := "", ""
	report := func(progress TransferProgress) {
		if options.Progress != nil {
			options.Progress(remote, progress)
		}
	}
	if operation == "push" {
		lfsURL, lfsProtocol = pushMediaEndpoint(source, lfs)
		if (lfsProtocol == "https" || lfsProtocol == "http") && (auth.media != nil || lfs.url != "") {
			media, err = prepareTransport(lfsURL, lfsProtocol, auth.media, report)
			if err != nil {
				return err
			}
			defer media.close()
		} else if lfsURL != "" {
			credentialURLs = append(credentialURLs, lfsURL)
		}
	}
	if err := stage.bindCredentialContext(ctx, source.endpoint, lfsURL); err != nil {
		return err
	}
	if err := stage.bindProxyContext(ctx, remote, source.endpoint, lfsURL); err != nil {
		return err
	}
	transport, err := prepareTransport(source.endpoint, source.protocol, auth.git, report, credentialURLs...)
	if err != nil {
		return err
	}
	defer transport.close()
	transport.options = append(transport.options, "-c", "http.followRedirects=false")
	err = run(pushDestination{stage: stage, transport: transport, media: media, name: name, scratch: scratch,
		lfsURL: lfsURL, lfsProtocol: lfsProtocol, lfsConfigError: lfs.diagnostic, local: local})
	if errors.Is(err, errLFSPushAuthentication) && lfsProtocol == "https" {
		challenge := remoteAuthError(remote, lfsURL, operation)
		challenge.endpoint, challenge.media = source.endpoint, lfsURL
		return challenge
	}
	if errors.Is(err, ErrHTTPSAuthentication) {
		return remoteAuthError(remote, source.endpoint, operation)
	}
	return err
}

func pushMediaEndpoint(source remoteSource, target lfsRemoteTarget) (string, string) {
	if target.url != "" {
		return target.url, target.protocol
	}
	return defaultLFSHTTPURL(source.endpoint), source.protocol
}

func (p *PushPlan) updateTracking(ctx context.Context) string {
	r, tracking := p.repo, p.tracking
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	err := r.withRefEdit(ctx, func() error {
		fetch, err := r.remoteEndpoint(ctx, p.options.Remote.Name)
		if err != nil || fetch != p.options.Remote.sources[0] {
			return fmt.Errorf("the fetch URL changed")
		}
		specs, err := r.fetchRefspecs(ctx, p.options.Remote.Name)
		if err != nil {
			return err
		}
		name, err := pullTrackingReference(p.options.Remote.Name, p.options.Branch.Ref, specs)
		if err != nil || name != tracking.Ref || !uniqueTrackingSource(p.options.Branch.Ref, name, specs) {
			return fmt.Errorf("the tracking reference changed")
		}
		expected := tracking.OID
		if expected == "" {
			expected = strings.Repeat("0", len(p.options.Branch.OID))
		}
		if tracking.OID == p.options.Branch.OID {
			return nil
		}
		protect := map[string]bool{tracking.Ref: true}
		format, err := r.optionalRef(ctx, "rev-parse", "--show-ref-format")
		if err != nil {
			return err
		}
		if err := r.protectFetchRefPaths(ctx, protect, format != "reftable"); err != nil {
			return err
		}
		return r.editDirectRef(ctx, tracking.Ref, expected, p.options.Branch.OID, "push: update tracking", func() error {
			if err := r.protectFetchRefPaths(ctx, protect, format != "reftable"); err != nil {
				return err
			}
			return r.protectFetchHeads(ctx, protect)
		})
	})
	if err != nil {
		return fmt.Sprintf("Push succeeded, but the local tracking reference could not be updated: %v", err)
	}
	return ""
}

func uniqueTrackingSource(source, destination string, specs []string) bool {
	found := false
	for _, spec := range specs {
		from, to, ok := strings.Cut(strings.TrimPrefix(spec, "+"), ":")
		if !ok {
			continue
		}
		value, matches := matchFetchSource(to, destination)
		if !matches {
			continue
		}
		candidate := strings.Replace(from, "*", value, 1)
		if fetchSourceExcluded(specs, candidate) {
			continue
		}
		if candidate != source {
			return false
		}
		found = true
	}
	return found
}

func fetchSourceExcluded(specs []string, source string) bool {
	for _, rule := range specs {
		if negative, ok := strings.CutPrefix(rule, "^"); ok {
			if _, matches := matchFetchSource(negative, source); matches {
				return true
			}
		}
	}
	return false
}
