package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

type RemoteInfo struct {
	Name     string
	FetchURL string
	PushURL  string
	config   []configValue
	endpoint string
	legacy   []legacyRemoteFile
	imports  []configValue
}

func (r RemoteInfo) SameSelection(other RemoteInfo) bool {
	return r.Name == other.Name && r.FetchURL == other.FetchURL && r.PushURL == other.PushURL && r.endpoint == other.endpoint &&
		slices.Equal(r.config, other.config) && slices.EqualFunc(r.legacy, other.legacy, legacyRemoteFile.same)
}

func (r *Repository) RemoteNames(ctx context.Context) ([]string, error) {
	names, _, err := r.remoteInventory(ctx)
	return names, err
}

func (r *Repository) remoteInventory(ctx context.Context) ([]string, legacyRemotes, error) {
	out, err := r.readLimited(ctx, "", 1<<20, "remote")
	if err != nil {
		return nil, nil, err
	}
	legacy, err := r.readLegacyRemotes(ctx)
	if err != nil {
		return nil, nil, err
	}
	var names []string
	if len(out) != 0 {
		names = strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	}
	for name := range legacy {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names, legacy, nil
}

func (r *Repository) Remotes(ctx context.Context) ([]RemoteInfo, error) {
	return r.readRemotes(ctx, "")
}

func (r *Repository) readRemotes(ctx context.Context, only string) ([]RemoteInfo, error) {
	names, legacy, err := r.remoteInventory(ctx)
	if err != nil || len(names) == 0 {
		return nil, err
	}
	config, err := r.readConfigValues(ctx, "^remote[.].*[.][^.]+$")
	if err != nil {
		return nil, err
	}
	values := make(map[string][]configValue, len(names))
	ordered := make([]string, 0, len(names))
	for _, entry := range config {
		end := strings.LastIndexByte(entry.key, '.')
		if end <= len("remote.") || !strings.HasPrefix(entry.key, "remote.") {
			return nil, fmt.Errorf("invalid Git remote configuration")
		}
		name := entry.key[len("remote."):end]
		if len(values[name]) == 0 && slices.Contains(names, name) {
			ordered = append(ordered, name)
		}
		entry.key = entry.key[end+1:]
		values[name] = append(values[name], entry)
	}
	for _, name := range names {
		if len(values[name]) == 0 {
			ordered = append(ordered, name)
		}
	}
	items := make([]RemoteInfo, 0, len(names))
	for _, name := range ordered {
		if only != "" && name != only {
			continue
		}
		source := legacy[name]
		item := RemoteInfo{Name: name, config: values[name], legacy: source.files}
		if len(item.urls("url")) == 0 && len(item.urls("vcs")) == 0 {
			item.imports = source.config
			item.config = append(item.config, item.imports...)
		}
		fetch, push := item.urls("url"), item.urls("pushurl")
		if len(fetch) != 0 {
			item.FetchURL = fetch[0]
		}
		if len(push) != 0 {
			item.PushURL = push[0]
		}
		effective, err := r.readLimited(ctx, "", 64<<10, "remote", "get-url", "--", name)
		if err == nil {
			item.endpoint = strings.TrimSuffix(string(effective), "\n")
		} else if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if len(item.config) == 0 {
			item.FetchURL = item.endpoint
		}
		items = append(items, item)
	}
	return items, nil
}

func (r RemoteInfo) urls(key string) []string {
	var values []string
	for _, entry := range r.config {
		if entry.key == key {
			if entry.value == "" && (key == "url" || key == "pushurl") {
				values = values[:0]
			} else {
				values = append(values, entry.value)
			}
		}
	}
	return values
}

func (r *Repository) matchRemote(ctx context.Context, expected RemoteInfo) (RemoteInfo, error) {
	current, err := r.readRemotes(ctx, expected.Name)
	if err != nil {
		return RemoteInfo{}, err
	}
	i := slices.IndexFunc(current, func(item RemoteInfo) bool { return item.SameSelection(expected) })
	if i < 0 {
		return RemoteInfo{}, fmt.Errorf("the remote configuration changed since it was shown")
	}
	return current[i], nil
}

func (s *remoteConfigEdit) localRemote(expected RemoteInfo, retire bool) (RemoteInfo, error) {
	r, ctx := s.repo, s.ctx
	remote, err := r.matchRemote(ctx, expected)
	if err != nil {
		return RemoteInfo{}, err
	}
	if len(remote.config) == 0 {
		return RemoteInfo{}, fmt.Errorf("remote %q does not have an editable configuration", remote.Name)
	}
	for _, entry := range remote.config[:len(remote.config)-len(remote.imports)] {
		if !r.isPrimaryConfig(entry.origin) {
			return RemoteInfo{}, fmt.Errorf("remote %q is configured outside this repository's config file", remote.Name)
		}
	}
	if len(remote.imports) != 0 {
		if remote.endpoint == "" {
			return RemoteInfo{}, fmt.Errorf("Git could not read legacy remote %q for migration", remote.Name)
		}
		for _, key := range []string{"url", "fetch", "push"} {
			if err := s.values(remote.Name, key, remote.urls(key)); err != nil {
				return RemoteInfo{}, err
			}
		}
	}
	if retire || len(remote.imports) != 0 {
		s.legacy = remote.legacy
	}
	return remote, nil
}

func (r *Repository) isPrimaryConfig(source string) bool {
	origin, file := strings.CutPrefix(source, "file:")
	if !file {
		return false
	}
	if !filepath.IsAbs(origin) {
		origin = filepath.Join(r.Path(), origin)
	}
	origin = filepath.Clean(origin)
	expected := filepath.Join(r.CommonDir, "config")
	return origin == expected || runtime.GOOS == "windows" && strings.EqualFold(origin, expected)
}

func (r *Repository) AddRemote(ctx context.Context, name, fetchURL string) error {
	return r.withRefEdit(ctx, func() error {
		if err := r.checkRemoteInput(ctx, name, fetchURL, ""); err != nil {
			return err
		}
		return r.editRemoteConfig(ctx, func(stage *remoteConfigEdit) error {
			names, err := r.RemoteNames(ctx)
			if err != nil {
				return err
			}
			if err := availableRemoteName(names, "", name); err != nil {
				return err
			}
			if _, err := r.readLimited(ctx, "", 64<<10, "remote", "get-url", "--", name); err == nil {
				return fmt.Errorf("remote %q already exists", name)
			} else if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := stage.values(name, "url", []string{fetchURL}); err != nil {
				return err
			}
			return stage.values(name, "fetch", []string{"+refs/heads/*:refs/remotes/" + name + "/*"})
		})
	})
}

func (r *Repository) EditRemote(ctx context.Context, expected RemoteInfo, name, fetchURL, pushURL string) error {
	return r.withRefEdit(ctx, func() error {
		if err := r.checkRemoteInput(ctx, name, fetchURL, pushURL); err != nil {
			return err
		}
		if name == expected.Name {
			return r.editRemoteURLs(ctx, expected, fetchURL, pushURL)
		}
		return r.renameRemoteWithURLs(ctx, expected, name, fetchURL, pushURL)
	})
}

func (r *Repository) RemoveRemote(ctx context.Context, expected RemoteInfo) error {
	return r.withRefEdit(ctx, func() error {
		return r.removeConfiguredRemote(ctx, expected)
	})
}

func (r *Repository) checkRemoteInput(ctx context.Context, name, fetchURL, pushURL string) error {
	if _, err := r.checkedRefName(ctx, "remotes", name+"/probe"); err != nil || name == "" || name == "." {
		return fmt.Errorf("enter a valid remote name")
	}
	if fetchURL == "" || strings.ContainsAny(fetchURL, "\x00\r\n") || strings.ContainsAny(pushURL, "\x00\r\n") {
		return fmt.Errorf("enter a nonempty fetch URL and use only one line per URL")
	}
	return nil
}

func availableRemoteName(names []string, previous, name string) error {
	for _, existing := range names {
		if existing == previous {
			continue
		}
		if existing == name || strings.HasPrefix(existing, name+"/") || strings.HasPrefix(name, existing+"/") {
			return fmt.Errorf("remote name %q conflicts with %q", name, existing)
		}
	}
	return nil
}

func (r *Repository) editRemoteURLs(ctx context.Context, expected RemoteInfo, fetchURL, pushURL string) error {
	return r.editRemoteConfig(ctx, func(stage *remoteConfigEdit) error {
		remote, err := stage.localRemote(expected, false)
		if err != nil {
			return err
		}
		return stage.setURLs(remote, fetchURL, pushURL)
	})
}

func (s remoteConfigEdit) setURLs(remote RemoteInfo, fetchURL, pushURL string) error {
	if fetchURL != remote.FetchURL {
		if err := s.values(remote.Name, "url", replacePrimaryURL(remote.urls("url"), fetchURL)); err != nil {
			return err
		}
	}
	if pushURL != remote.PushURL {
		return s.values(remote.Name, "pushurl", replacePrimaryURL(remote.urls("pushurl"), pushURL))
	}
	return nil
}

func replacePrimaryURL(urls []string, value string) []string {
	if value == "" {
		return nil
	}
	if len(urls) == 0 {
		return []string{value}
	}
	urls[0] = value
	return urls
}

type remoteConfigEdit struct {
	ctx            context.Context
	repo           *Repository
	path           string
	deletes        []remoteTrackingRef
	moves          []remoteRefMove
	reftableRename *remoteRenameRequest
	legacy         []legacyRemoteFile
}

func (s remoteConfigEdit) values(name, key string, values []string) error {
	key = "remote." + name + "." + key
	if err := s.unset(key); err != nil {
		return err
	}
	for _, value := range values {
		if _, err := s.repo.readLimited(s.ctx, "", 64<<10, "config", "--file", s.path, "--add", key, value); err != nil {
			return fmt.Errorf("could not write Git config key %q: %w", key, err)
		}
	}
	return nil
}

func (s remoteConfigEdit) unset(key string) error {
	_, err := s.repo.readLimited(s.ctx, "", 64<<10, "config", "--file", s.path, "--unset-all", key)
	if err != nil && !gitExitCode(err, 5) {
		return fmt.Errorf("could not update Git config key %q: %w", key, err)
	}
	return nil
}

func (r *Repository) editRemoteConfig(ctx context.Context, edit func(*remoteConfigEdit) error) error {
	common, err := os.OpenRoot(r.CommonDir)
	if err != nil {
		return err
	}
	defer common.Close()
	lock, err := lockGitStorageFile(common, "config")
	if err != nil {
		return err
	}
	defer lock.close()
	original, mode, err := readGitStorageFile(common, "config", remoteConfigLimit)
	if err != nil {
		return err
	}
	// Includes and config.worktree are outside the primary config lock.
	const configPattern = "^remote[.]|^branch[.].*[.](remote|pushremote|merge)$|^url[.].*[.](insteadof|pushinsteadof)$"
	effective, err := r.readConfigValues(ctx, configPattern)
	if err != nil {
		return err
	}
	legacy, err := r.readLegacyRemotes(ctx)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "gitwave-remote-config-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		return err
	}
	stage := &remoteConfigEdit{ctx: ctx, repo: r, path: path}
	if err := edit(stage); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	next, _, err := readGitStorageFile(root, "config", remoteConfigLimit)
	if err != nil || bytes.Equal(original, next) && len(stage.legacy) == 0 {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := lock.prepare(next, mode); err != nil {
		return err
	}
	retirement := legacyRemoteRetirement{root: common}
	publish := func() error {
		now, currentMode, err := readGitStorageFile(common, "config", remoteConfigLimit)
		if err != nil || !bytes.Equal(now, original) || currentMode != mode {
			return fmt.Errorf("the Git config changed during the remote edit")
		}
		current, err := r.readConfigValues(ctx, configPattern)
		if err != nil {
			return err
		}
		if !slices.Equal(current, effective) {
			return fmt.Errorf("the effective Git config changed during the remote edit")
		}
		currentLegacy, err := r.readLegacyRemotes(ctx)
		if err != nil {
			return err
		}
		if !legacy.same(currentLegacy) {
			return fmt.Errorf("a Git legacy remote changed during the remote edit")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := retirement.park(stage.legacy); err != nil {
			return err
		}
		return lock.publish()
	}
	if stage.reftableRename != nil {
		err = stage.publishReftableRename(common, original, next, publish)
	} else if len(stage.deletes) == 0 && len(stage.moves) == 0 {
		err = publish()
	} else {
		err = stage.publishTrackingChanges(original, next, publish)
	}
	return retirement.finish(err)
}

func gitExitCode(err error, code int) bool {
	var exit *exec.ExitError
	return errors.As(err, &exit) && exit.ExitCode() == code
}
