package gitcli

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gitwave.work/internal/process"
	"gitwave.work/internal/proxy"
	"gitwave.work/internal/sshclient"
)

var ErrHTTPSAuthentication = errors.New("HTTPS authentication failed")

type RemoteAuthError struct {
	Remote, Host string
	endpoint     string
	operation    string
	media        string
}

func (e *RemoteAuthError) Error() string {
	return fmt.Sprintf("HTTPS authentication failed for remote %q", e.Remote)
}

func (e *RemoteAuthError) Unwrap() error { return ErrHTTPSAuthentication }

func (e *RemoteAuthError) LFSLocation() string {
	return redactLFSURLs(e.media)
}

func (e *RemoteAuthError) Location() string {
	return redactLFSURLs(e.credentialEndpoint())
}

func (e *RemoteAuthError) SameChallenge(other *RemoteAuthError) bool {
	return other != nil && e.Remote == other.Remote && e.endpoint == other.endpoint &&
		e.operation == other.operation && e.media == other.media
}

func remoteAuthError(name, endpoint, operation string) *RemoteAuthError {
	address, _ := url.Parse(endpoint)
	return &RemoteAuthError{Remote: name, Host: address.Host, endpoint: endpoint, operation: operation}
}

func lfsAuthError(name, endpoint, media string) *RemoteAuthError {
	challenge := remoteAuthError(name, media, "fetch")
	challenge.endpoint, challenge.media = endpoint, media
	return challenge
}

type TransferProgress struct {
	Phase     string
	Percent   int
	Completed int
	Total     int
}

const TransferLFSDownload = "Downloading Git LFS objects"

type transportAttempt struct {
	remote, protocol string
	defaultSSH       string
	options, env     []string
	auth             *HTTPSCredential
	broker           *credentialBroker
	output           transferOutput
}

func prepareTransport(remote, protocol string, auth *HTTPSCredential, progress func(TransferProgress), credentialURLs ...string) (*transportAttempt, error) {
	attempt := &transportAttempt{remote: remote, protocol: protocol, auth: auth,
		env: []string{"GIT_ALLOW_PROTOCOL=" + protocol}, output: transferOutput{report: progress}}
	if protocol == "ssh" {
		attempt.defaultSSH = sshclient.PreferredGitSSH()
	}
	if auth == nil {
		return attempt, nil
	}
	broker, err := newCredentialBroker(remote, *auth, credentialURLs...)
	if err != nil {
		return nil, err
	}
	options, env, err := broker.commandConfig()
	if err != nil {
		_ = broker.server.Close()
		return nil, err
	}
	attempt.broker = broker
	attempt.options = options
	attempt.env = append(attempt.env, env...)
	return attempt, nil
}

func (t *transportAttempt) close() {
	if t.broker != nil {
		_ = t.broker.server.Close()
	}
}

func (t *transportAttempt) run(ctx context.Context, cmd *exec.Cmd, action string, proxyURLs ...string) error {
	t.output = transferOutput{report: t.output.report}
	cmd.Env = append(cmd.Env, t.env...)
	if t.protocol == "http" || t.protocol == "https" || t.protocol == "ssh" {
		cmd.Env = proxy.CommandEnvironment(ctx, cmd.Env)
	}
	if t.defaultSSH != "" {
		cmd.Env = append(cmd.Env, "GIT_SSH="+t.defaultSSH)
	}
	cmd.Stderr = &t.output
	err := process.Run(cmd)
	t.output.flush()
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	diagnostic := strings.TrimSpace(t.output.diagnostic.buffer.String())
	if strings.Contains(diagnostic, "error: Git LFS: "+errLFSPushAuthentication.Error()) {
		return errLFSPushAuthentication
	}
	if t.protocol == "https" && isHTTPSAuthenticationError(diagnostic) {
		return ErrHTTPSAuthentication
	}
	if t.auth != nil {
		diagnostic = strings.ReplaceAll(diagnostic, t.auth.Password, "[redacted]")
	}
	if diagnostic != "" {
		return fmt.Errorf("git %s: %s", action, redactProxyDiagnostic(diagnostic, cmd.Env, proxyURLs))
	}
	return fmt.Errorf("git %s: %w", action, err)
}

type transportRepo struct {
	repo         *Repository
	dir, objects string
	proxyURLs    []string
}

func (r *Repository) prepareTransportRepo(ctx context.Context, dir, shallow string) (*transportRepo, error) {
	objects, err := r.readLimited(ctx, "", 64<<10, "rev-parse", "--path-format=absolute", "--git-path", "objects")
	if err != nil {
		return nil, err
	}
	format, err := r.readLimited(ctx, "", 64, "rev-parse", "--show-object-format")
	if err != nil {
		return nil, err
	}
	stage := &transportRepo{repo: r, dir: filepath.Join(dir, "git"), objects: strings.TrimSuffix(string(objects), "\n")}
	if !filepath.IsAbs(stage.objects) {
		return nil, fmt.Errorf("Git did not return an absolute object directory")
	}
	kind := strings.TrimSpace(string(format))
	if kind != "sha1" && kind != "sha256" {
		return nil, fmt.Errorf("unsupported Git object format")
	}
	options := []string{"-c", "init.defaultRefFormat=files"}
	if _, err := readGit(ctx, r.git, dir, options, "", 64<<10,
		"init", "--quiet", "--bare", "--template=", "--object-format="+kind, "--", stage.dir); err != nil {
		return nil, err
	}
	if shallow != "" {
		if err := os.WriteFile(filepath.Join(stage.dir, "shallow"), []byte(shallow), 0o600); err != nil {
			return nil, err
		}
	}
	return stage, nil
}

func (s *transportRepo) command(ctx context.Context, input string, args ...string) *exec.Cmd {
	options := []string{"--git-dir=" + s.dir, "-c", "core.hooksPath=" + s.dir}
	cmd := gitCommand(ctx, s.repo.git, s.repo.Path(), options, input, args...)
	cmd.Env = append(cmd.Env, "GIT_OBJECT_DIRECTORY="+s.objects)
	return cmd
}

func (s *transportRepo) bindEndpoint(ctx context.Context, endpoint string) (string, error) {
	// A unique alias resolves once to the validated endpoint, even when
	// the endpoint itself also matches a user's insteadOf rule.
	alias := "gitwave-transport:" + rand.Text()
	if err := s.addConfig(ctx, "url."+endpoint+".insteadOf", alias); err != nil {
		return "", err
	}
	return alias, nil
}

func (s *transportRepo) addConfig(ctx context.Context, key, value string) error {
	cmd := s.command(ctx, "", "config", "--local", "--add", key, value)
	_, err := readGitOutput(ctx, cmd, 64<<10, "configure isolated Git transport")
	return err
}

func (t *transportAttempt) remember(ctx context.Context, git, dir string, vault HTTPSCredentialVault) string {
	if t.broker == nil || !t.auth.Remember {
		return ""
	}
	var used []string
	t.broker.used.Range(func(endpoint, _ any) bool {
		used = append(used, endpoint.(string))
		return true
	})
	slices.Sort(used)
	var warnings []string
	for _, endpoint := range used {
		helperErr := rememberHTTPSCredential(ctx, git, dir, endpoint, *t.auth)
		if vault == nil {
			if helperErr != nil {
				warnings = append(warnings, helperErr.Error())
			}
			continue
		}
		if err := vault.Store(ctx, endpoint, *t.auth); err != nil && helperErr != nil {
			warnings = append(warnings, "could not save the HTTPS credentials in the Git helper or system keyring")
		}
	}
	return strings.Join(warnings, "\n")
}

func isHTTPSAuthenticationError(diagnostic string) bool {
	return strings.Contains(diagnostic, "Authentication failed") ||
		strings.Contains(diagnostic, "could not read Username") ||
		strings.Contains(diagnostic, "could not read Password") ||
		strings.Contains(diagnostic, "unable to get username from user") ||
		strings.Contains(diagnostic, "unable to get password from user") ||
		strings.Contains(diagnostic, "returned error: 401") || strings.Contains(diagnostic, "batch response: Authentication required")
}

var transferProgressPattern = regexp.MustCompile("^(Receiving objects|Resolving deltas|Updating files|Checking out files|Writing objects):\\s+(\\d+)%\\s+\\((\\d+)/(\\d+)\\)")

type transferOutput struct {
	report     func(TransferProgress)
	line       []byte
	diagnostic gitDiagnostic
	collision  bool
}

func (o *transferOutput) Write(data []byte) (int, error) {
	for _, b := range data {
		if b == '\r' || b == '\n' {
			o.flush()
		} else if len(o.line) < 4096 {
			o.line = append(o.line, b)
		}
	}
	return len(data), nil
}

func (o *transferOutput) flush() {
	line := strings.TrimSpace(string(o.line))
	o.line = o.line[:0]
	if strings.HasPrefix(line, "warning: the following paths have collided") {
		o.collision = true
	}
	if strings.HasPrefix(line, "error:") || strings.HasPrefix(line, "fatal:") {
		_, _ = o.diagnostic.Write([]byte(line + "\n"))
	}
	if o.report == nil {
		return
	}
	fields := transferProgressPattern.FindStringSubmatch(line)
	if len(fields) != 5 {
		return
	}
	percent, _ := strconv.Atoi(fields[2])
	completed, _ := strconv.Atoi(fields[3])
	total, _ := strconv.Atoi(fields[4])
	o.report(TransferProgress{Phase: fields[1], Percent: percent, Completed: completed, Total: total})
}
