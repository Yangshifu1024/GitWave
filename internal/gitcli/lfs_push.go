package gitcli

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gitwave.work/internal/gittool"
)

const lfsPrePushArg = "--gitwave-lfs-pre-push"
const lfsPrePushConfig = "GITWAVE_LFS_PRE_PUSH"
const lfsCredentialURL = "GITWAVE_LFS_CREDENTIAL_URL"
const lfsCredentialKey = "GITWAVE_LFS_CREDENTIAL_KEY"

var errLFSPushAuthentication = errors.New("authentication required at the selected media endpoint")

var lfsDiagnosticHTTPURL = regexp.MustCompile(`https?://[^[:space:]<>'"]+`)

type lfsPushSpec struct {
	Git, Stage, GitObjects string
	Cache, Endpoint        string
	Name, Protocol, LFS    string
	MediaURL               string
	IndependentMedia       bool
	MediaOptions           []string
	ConfigError            string
	TargetIdentity         string
	TargetCache            string
	GitTargetIdentity      string
	CacheBinding           lfsCacheBinding
}

func (d pushDestination) lfsHookEnvironment(ctx context.Context) ([]string, error) {
	r := d.stage.repo
	_, local, err := r.workingGitSettings(ctx)
	if err != nil {
		return nil, err
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	spec := lfsPushSpec{
		Git: r.git, Stage: d.stage.dir, GitObjects: d.stage.objects, Cache: local.storage,
		Name: d.name, Endpoint: d.transport.remote, Protocol: d.lfsProtocol, MediaURL: d.lfsURL,
	}
	if d.media != nil {
		spec.IndependentMedia = true
		spec.MediaOptions = d.media.options
	}
	spec.ConfigError = d.lfsConfigError
	if spec.Protocol == "file" {
		spec.TargetIdentity, spec.TargetCache, spec.CacheBinding = d.local.mediaIdentity, d.local.mediaPath, d.local.cache
		if spec.ConfigError == "" {
			spec.ConfigError = d.local.mediaError
		}
	} else {
		spec.LFS, _ = r.lfsExecutable(ctx)
	}
	spec.GitTargetIdentity = d.local.gitIdentity
	config, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	// This is the only enabled hook in the private Git directory.
	hook := "#!/bin/sh\nexec \"$GITWAVE_LFS_EXECUTABLE\" " + lfsPrePushArg + " \"$1\" \"$2\"\n"
	if err := os.WriteFile(filepath.Join(d.stage.dir, "pre-push"), []byte(hook), 0o700); err != nil {
		return nil, err
	}
	env := []string{lfsFilterExecutable + "=" + filepath.ToSlash(executable), lfsPrePushConfig + "=" + string(config)}
	if d.media != nil && d.media.broker != nil {
		env = append(env, lfsCredentialURL+"="+d.media.broker.address, lfsCredentialKey+"="+d.media.broker.key)
	}
	return env, nil
}

// LFSPrePush handles GitWave's private pre-push hook before the GUI starts.
func LFSPrePush() bool {
	if len(os.Args) < 2 || os.Args[1] != lfsPrePushArg {
		return false
	}
	if err := runLFSPrePush(context.Background(), os.Args[2:], os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, "error: Git LFS:", err)
		os.Exit(1)
	}
	return true
}

func runLFSPrePush(ctx context.Context, args []string, stdin io.Reader) error {
	var spec lfsPushSpec
	if err := json.Unmarshal([]byte(os.Getenv(lfsPrePushConfig)), &spec); err != nil {
		return fmt.Errorf("invalid private push configuration")
	}
	if len(args) != 2 || args[0] != spec.Name || args[1] != spec.Endpoint {
		return fmt.Errorf("Git's push target changed after it was selected")
	}
	if spec.GitTargetIdentity != "" {
		_, identity, err := identifyLocalPushRepository(ctx, spec.Endpoint)
		if err != nil || identity != spec.GitTargetIdentity {
			return fmt.Errorf("the selected local Git destination changed before upload")
		}
	}
	data, err := readLFSPushInput(stdin)
	if err != nil {
		return err
	}
	updates, err := readLFSPushUpdates(data)
	if err != nil {
		return err
	}
	objects, err := spec.pushObjects(ctx, updates)
	if err != nil || len(objects) == 0 {
		return err
	}
	if spec.ConfigError != "" {
		return errors.New(spec.ConfigError)
	}
	if spec.Protocol == "file" {
		return spec.pushLocalMedia(ctx, objects)
	}
	return spec.pushNetworkMedia(ctx, data, objects)
}

// A local Git remote has its own LFS storage configuration. Keep the source
// cache settings out of the destination and verify every object copied.
func (s lfsPushSpec) pushLocalMedia(ctx context.Context, objects []lfsPointer) error {
	identity, cachePath, err := s.localTarget(ctx)
	if err != nil {
		return err
	}
	if identity != s.TargetIdentity || cachePath != s.TargetCache {
		return fmt.Errorf("the selected local LFS destination changed before upload")
	}
	destination, err := s.CacheBinding.open(cachePath, true)
	if err != nil {
		return err
	}
	defer destination.Close()
	for _, pointer := range objects {
		if err := s.copyLocalMedia(ctx, destination, pointer); err != nil {
			return err
		}
	}
	current, currentCache, err := s.localTarget(ctx)
	if err != nil || current != identity || currentCache != cachePath || !s.CacheBinding.stillAt(cachePath) {
		return fmt.Errorf("the selected local LFS destination changed during upload")
	}
	opened, err := destination.Stat(".")
	if err != nil {
		return err
	}
	actual, err := os.Stat(cachePath)
	if err != nil || !sameFileIdentity(opened, actual) {
		return fmt.Errorf("the destination LFS cache was replaced during upload")
	}
	return nil
}

func (s lfsPushSpec) localTarget(ctx context.Context) (string, string, error) {
	endpoint := s.MediaURL
	if endpoint == "" {
		endpoint = s.Endpoint
	}
	target, identity, err := identifyLocalPushRepository(ctx, endpoint)
	if err != nil {
		return "", "", err
	}
	_, targetLFS, err := target.workingGitSettings(ctx)
	if err != nil {
		return "", "", err
	}
	return identity, targetLFS.storage, nil
}

func defaultLFSHTTPURL(endpoint string) string {
	address, err := url.Parse(endpoint)
	if err != nil || address.Scheme != "https" && address.Scheme != "http" {
		return ""
	}
	path := strings.TrimRight(address.EscapedPath(), "/")
	if !strings.HasSuffix(path, ".git") {
		path += ".git"
	}
	path += "/info/lfs"
	address.Path, _ = url.PathUnescape(path)
	address.RawPath = path
	return address.String()
}

func (s lfsPushSpec) copyLocalMedia(ctx context.Context, destination *os.Root, pointer lfsPointer) error {
	existing, err := openLFSObjectIn(destination, pointer.oid)
	if err == nil {
		defer existing.Close()
		return copyLFSObject(ctx, io.Discard, existing, pointer)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	media, err := openLFSObject(s.Cache, pointer.oid)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("LFS object %s is missing from the local cache and destination", pointer.oid)
	}
	if err != nil {
		return err
	}
	defer media.Close()
	return storeVerifiedLFSMedia(ctx, destination, media, pointer)
}

func (s lfsPushSpec) pushNetworkMedia(ctx context.Context, updates []byte, objects []lfsPointer) error {
	if s.LFS == "" {
		return fmt.Errorf("install Git LFS to upload the selected LFS objects")
	}
	cache, err := s.snapshotNetworkMedia(ctx, objects)
	if err != nil {
		return err
	}
	defer os.RemoveAll(cache)
	type setting struct{ key, value string }
	settings := []setting{
		{"lfs.storage", cache}, {"lfs.allowincompletepush", "false"}, {"lfs.standalonetransferagent", ""},
		{"lfs.basictransfersonly", "true"}, {"lfs.transfer.enablehrefrewrite", "false"},
	}
	if s.MediaURL != "" {
		alias := "gitwave-lfs:" + rand.Text()
		settings = append(settings, setting{"url." + s.MediaURL + ".pushInsteadOf", alias}, setting{"lfs.pushurl", alias})
	}
	for _, setting := range settings {
		config := s.objectCommand(ctx, "config", "--local", "--replace-all", setting.key, setting.value)
		if _, err := readGitOutput(ctx, config, 8192, "configure isolated LFS transport"); err != nil {
			return err
		}
	}
	cmd := exec.CommandContext(ctx, s.LFS, "pre-push", s.Name, s.Endpoint)
	cmd.Dir = s.Stage
	// Retain Git's scoped credential helper and transport options from the
	// parent push. The stage contains no live repository configuration or HEAD.
	cmd.Env = append(gittool.Environment(s.mediaEnvironment(), s.Git, s.LFS), "GIT_DIR="+s.Stage, "GIT_OBJECT_DIRECTORY="+s.GitObjects,
		"GIT_LFS_SKIP_PUSH=0")
	if s.Protocol == "ssh" {
		config := gitCommand(ctx, s.Git, s.Stage, []string{"--git-dir=" + s.Stage}, "", "config", "--null", "--get", "core.sshCommand")
		if err := setLFSDefaultSSH(ctx, cmd, config, os.Getenv("GIT_SSH")); err != nil {
			return err
		}
	}
	cmd.Stdin = strings.NewReader(string(updates))
	var diagnostic gitDiagnostic
	cmd.Stdout, cmd.Stderr = &diagnostic, &diagnostic
	if err := cmd.Run(); err != nil {
		if message := strings.TrimSpace(diagnostic.buffer.String()); message != "" {
			if s.Protocol == "https" && s.MediaURL != "" && isHTTPSAuthenticationError(message) {
				return errLFSPushAuthentication
			}
			message = redactProxyDiagnostic(message, cmd.Env, nil)
			return fmt.Errorf("upload objects: %s", message)
		}
		return fmt.Errorf("upload objects: %w", err)
	}
	return nil
}

func (s lfsPushSpec) mediaEnvironment() []string {
	env := os.Environ()
	if !s.IndependentMedia {
		return env
	}
	// The nested Git credential lookup may use its own helper and broker.
	// The primary helper and its capability must stay with the primary push.
	env = slices.DeleteFunc(env, func(entry string) bool {
		name, _, _ := strings.Cut(entry, "=")
		name = strings.ToUpper(name)
		return name == "GIT_CONFIG_PARAMETERS" || name == "GIT_CONFIG_COUNT" ||
			strings.HasPrefix(name, "GIT_CONFIG_KEY_") || strings.HasPrefix(name, "GIT_CONFIG_VALUE_") ||
			name == credentialBrokerURL || name == credentialBrokerKey || name == lfsCredentialURL || name == lfsCredentialKey
	})
	if len(s.MediaOptions) == 0 {
		return env
	}
	env = append(env, credentialBrokerURL+"="+os.Getenv(lfsCredentialURL), credentialBrokerKey+"="+os.Getenv(lfsCredentialKey),
		"GIT_CONFIG_COUNT="+strconv.Itoa(len(s.MediaOptions)/2))
	for i := 1; i < len(s.MediaOptions); i += 2 {
		key, value, _ := strings.Cut(s.MediaOptions[i], "=")
		index := strconv.Itoa(i / 2)
		env = append(env, "GIT_CONFIG_KEY_"+index+"="+key, "GIT_CONFIG_VALUE_"+index+"="+value)
	}
	return env
}

func (s lfsPushSpec) snapshotNetworkMedia(ctx context.Context, objects []lfsPointer) (_ string, err error) {
	path, err := os.MkdirTemp(s.Stage, "lfs-upload-")
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(path)
		}
	}()
	cache, err := os.OpenRoot(path)
	if err != nil {
		return "", err
	}
	defer cache.Close()
	for _, pointer := range objects {
		media, err := openLFSObject(s.Cache, pointer.oid)
		if errors.Is(err, fs.ErrNotExist) {
			continue // The server may already have it. Let the LFS client check.
		}
		if err != nil {
			return "", err
		}
		err = storeVerifiedLFSMedia(ctx, cache, media, pointer)
		media.Close()
		if err != nil {
			return "", err
		}
	}
	return path, nil
}

func storeVerifiedLFSMedia(ctx context.Context, destination *os.Root, media *os.File, pointer lfsPointer) error {
	info, err := media.Stat()
	if err != nil {
		return err
	}
	if info.Size() != pointer.size {
		return fmt.Errorf("%s: %w (size)", pointer.oid, errLFSObjectMismatch)
	}
	return storeLFSObject(ctx, destination, media, pointer.oid)
}

func redactLFSURLs(message string) string {
	return lfsDiagnosticHTTPURL.ReplaceAllStringFunc(message, func(address string) string {
		// Server replies may include credentials in signed URLs.
		parsed, err := url.Parse(address)
		if err != nil {
			return "[invalid URL]"
		}
		parsed.User, parsed.RawQuery, parsed.Fragment, parsed.RawFragment = nil, "", "", ""
		parsed.ForceQuery = false
		return parsed.String()
	})
}
