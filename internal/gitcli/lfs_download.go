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
	"slices"
	"strings"
	"time"

	"gitwave.work/internal/gittool"
	"gitwave.work/internal/process"
	"gitwave.work/internal/proxy"
)

type lfsDownloadPlan struct {
	executable       string
	include, exclude string
	source           remoteSource
	media            lfsRemoteTarget
	remoteFiles      localLFSTransport
	config           localLFSConfig
	storage          string
	cache            lfsCacheBinding
	allowExtensions  bool
}

type lfsDownloadReceipt struct {
	config    localLFSConfig
	storage   string
	directory os.FileInfo
	files     map[lfsPointer]lfsMediaStamp
	paths     map[string]lfsPointer
	filtered  bool
}

func (receipt *lfsDownloadReceipt) verify(ctx context.Context) error {
	if receipt == nil || receipt.directory == nil {
		return nil
	}
	cache, err := os.OpenRoot(receipt.storage)
	if err != nil {
		return fmt.Errorf("the local LFS cache is no longer available: %w", err)
	}
	defer cache.Close()
	info, err := cache.Stat(".")
	if err != nil || !sameFileIdentity(info, receipt.directory) {
		return fmt.Errorf("the local LFS cache was replaced before publication")
	}
	for object, before := range receipt.files {
		file, err := openLFSObjectIn(cache, object.oid)
		if err != nil {
			return fmt.Errorf("the downloaded LFS object %s is no longer available: %w", object.oid, err)
		}
		current, err := stampLFSMedia(file)
		if err == nil && !before.matches(current) {
			err = fmt.Errorf("the downloaded LFS object %s changed before publication", object.oid)
		}
		if err == nil && !current.hasChangeTime {
			err = copyLFSObject(ctx, io.Discard, file, object)
		}
		file.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) prepareLFSDownload(ctx context.Context, remote string) (lfsDownloadPlan, error) {
	return r.prepareLFSDownloadAt(ctx, remote, "", false)
}

func (r *Repository) prepareLFSDownloadAt(ctx context.Context, remote, storage string, allowExtensions bool) (lfsDownloadPlan, error) {
	_, config, err := r.workingGitSettings(ctx)
	if err != nil || config.standardDriver() == "" || config.unsupportedFormat ||
		config.skipSmudge || config.extensions && !allowExtensions {
		return lfsDownloadPlan{}, err
	}
	driver := config.standardDriver()
	var executable string
	if filepath.IsAbs(driver) {
		executable, err = exec.LookPath(driver)
	} else {
		executable, err = r.lfsExecutable(ctx)
	}
	if err != nil {
		if ctx.Err() != nil {
			return lfsDownloadPlan{}, ctx.Err()
		}
		return lfsDownloadPlan{}, nil
	}
	source, err := r.remoteEndpoint(ctx, remote)
	if err != nil {
		return lfsDownloadPlan{}, err
	}
	keys := []string{"lfs.url", "remote." + remote + ".lfsurl", "lfs.fetchinclude", "lfs.fetchexclude"}
	values, err := r.lfsConfigValues(ctx, keys)
	if err != nil {
		return lfsDownloadPlan{}, err
	}
	media := r.lfsConfiguredEndpoint(ctx, keys[:2], values, false)
	if storage == "" {
		storage = config.storage
	}
	cache, err := bindLFSCache(storage)
	if err != nil {
		return lfsDownloadPlan{}, err
	}
	return lfsDownloadPlan{
		executable: executable, include: values[keys[2]], exclude: values[keys[3]],
		source: source, media: media, remoteFiles: inspectLocalLFSTransport(ctx, source, media),
		config: config, storage: storage, cache: cache, allowExtensions: allowExtensions,
	}, nil
}

func (p lfsDownloadPlan) enabled() bool { return p.executable != "" }

func (p lfsDownloadPlan) verify(ctx context.Context, repo *Repository, name string, source remoteSource) error {
	fresh, err := OpenRoot(ctx, repo.Path())
	if err != nil {
		return err
	}
	if !repo.SameCheckout(fresh) {
		return fmt.Errorf("the checkout changed during LFS download")
	}
	current, err := fresh.prepareLFSDownloadAt(ctx, name, p.storage, p.allowExtensions)
	if err != nil {
		return err
	}
	if source != p.source || !p.remoteFiles.matches(current.remoteFiles) || !p.cache.stillAt(p.storage) {
		return fmt.Errorf("the Git LFS source or cache changed after selection")
	}
	current.cache, current.remoteFiles.cache = p.cache, p.remoteFiles.cache
	if current != p {
		return fmt.Errorf("the Git LFS download configuration changed after selection")
	}
	return nil
}

func (p lfsDownloadPlan) httpEndpoint() string {
	if p.media.protocol == "https" || p.media.protocol == "http" {
		return p.media.url
	}
	if p.media.url == "" {
		return defaultLFSHTTPURL(p.source.endpoint)
	}
	return ""
}

func (p lfsDownloadPlan) mediaTransport(git *transportAttempt, auth *HTTPSCredential,
	progress func(TransferProgress)) (*transportAttempt, error) {
	endpoint := p.httpEndpoint()
	if endpoint == "" {
		return git, nil
	}
	if auth == nil && p.media.url == "" && sameHTTPSOrigin(endpoint, git.remote) {
		auth = git.auth
	}
	scheme, _, _ := strings.Cut(endpoint, ":")
	return prepareTransport(endpoint, scheme, auth, progress)
}

func (p lfsDownloadPlan) download(ctx context.Context, stage *transportRepo, name, tip string,
	transport *transportAttempt, expected *RemoteInfo, receipt *lfsDownloadReceipt) error {
	private, err := os.MkdirTemp(filepath.Dir(stage.dir), "lfs-download-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(private)
	if err := stage.addConfig(ctx, "lfs.storage", private); err != nil {
		return err
	}
	objects, paths, err := p.scan(ctx, stage, tip)
	receipt.paths = paths
	receipt.filtered = paths != nil && (p.include != "" || p.exclude != "")
	if err != nil || len(objects) == 0 {
		return err
	}
	missing, err := p.missingMedia(objects)
	if err != nil {
		return err
	}
	var staged *os.Root
	if len(missing) != 0 {
		if p.media.diagnostic != "" {
			return errors.New(p.media.diagnostic)
		}
		if report := transport.output.report; report != nil {
			report(TransferProgress{Phase: TransferLFSDownload})
		}
		staged, err = os.OpenRoot(private)
		if err != nil {
			return err
		}
		defer staged.Close()
		if p.media.protocol == "file" || p.media.url == "" && p.source.protocol == "file" {
			err = p.downloadLocal(ctx, staged, missing)
		} else {
			err = p.downloadNetwork(ctx, stage.repo, private, missing, transport)
		}
		if err != nil {
			return err
		}
	}
	if err := p.verify(ctx, stage.repo, name, p.source); err != nil {
		return err
	}
	if expected != nil {
		if _, err := stage.repo.matchRemote(ctx, *expected); err != nil {
			return err
		}
	}
	cache, err := p.cache.open(p.storage, len(missing) != 0)
	if err != nil {
		return err
	}
	defer cache.Close()
	if staged != nil {
		if err := copyLFSMedia(ctx, cache, staged, missing); err != nil {
			return err
		}
	}
	receipt.config = p.config
	receipt.storage = p.storage
	receipt.directory, err = cache.Stat(".")
	if err != nil {
		return err
	}
	receipt.files = make(map[lfsPointer]lfsMediaStamp, len(objects))
	for _, object := range objects {
		file, err := openLFSObjectIn(cache, object.oid)
		if err != nil {
			return err
		}
		before, err := stampLFSMedia(file)
		if err == nil {
			err = copyLFSObject(ctx, io.Discard, file, object)
		}
		after, stampErr := stampLFSMedia(file)
		file.Close()
		if err != nil {
			return err
		}
		if stampErr != nil || !before.matches(after) {
			return fmt.Errorf("LFS object %s changed during download verification", object.oid)
		}
		receipt.files[object] = after
	}
	return receipt.verify(ctx)
}

func (p lfsDownloadPlan) scan(ctx context.Context, stage *transportRepo, tip string) ([]lfsPointer, map[string]lfsPointer, error) {
	cmd := p.command(ctx, stage, isolatedLFSConfig(), "ls-files", "--json", "--include="+p.include, "--exclude="+p.exclude, tip)
	raw, err := readGitOutput(ctx, cmd, 64<<20, "find LFS objects in the selected commit")
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && strings.Contains(string(exit.Stderr), "unknown flag: --json") {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	var result struct {
		Files []struct {
			Name string `json:"name"`
			OID  string `json:"oid"`
			Size int64  `json:"size"`
		} `json:"files"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, nil, fmt.Errorf("read selected Git LFS objects: %w", err)
	}
	allowed := make(map[string]lfsPointer, len(result.Files))
	if len(result.Files) == 0 {
		return nil, allowed, nil
	}
	paths := make([]string, 0, len(result.Files))
	for _, file := range result.Files {
		paths = append(paths, file.Name)
	}
	working, err := stage.repo.workingGit(ctx, tip)
	if err != nil {
		return nil, nil, err
	}
	defer working.close()
	if err := working.freezeWorktreeAttributes(ctx); err != nil {
		return nil, nil, err
	}
	filters, err := working.filterValues(ctx, paths, "--source="+tip)
	if err != nil {
		return nil, nil, err
	}
	seen := make(map[string]int64, len(result.Files))
	var objects []lfsPointer
	for i, file := range result.Files {
		if filters[i] != "lfs" {
			continue
		}
		if !validLFSOID(file.OID) || file.Size <= 0 {
			return nil, nil, fmt.Errorf("invalid Git LFS object in the selected commit")
		}
		pointer := lfsPointer{oid: file.OID, size: file.Size}
		allowed[file.Name] = pointer
		if size, ok := seen[file.OID]; ok {
			if size != file.Size {
				return nil, nil, fmt.Errorf("inconsistent sizes for Git LFS object %s", file.OID)
			}
			continue
		}
		seen[file.OID] = file.Size
		objects = append(objects, pointer)
	}
	return objects, allowed, nil
}

func (p lfsDownloadPlan) checkoutPaths(ctx context.Context, repo *Repository, tip string) (map[string]lfsPointer, error) {
	private, err := os.MkdirTemp("", "gitwave-lfs-paths-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(private)
	stage, err := repo.prepareTransportRepo(ctx, private, "")
	if err != nil {
		return nil, err
	}
	_, paths, err := p.scan(ctx, stage, tip)
	return paths, err
}

func (p lfsDownloadPlan) missingMedia(objects []lfsPointer) ([]lfsPointer, error) {
	cache, err := p.cache.open(p.storage, false)
	if errors.Is(err, fs.ErrNotExist) {
		return objects, nil
	}
	if err != nil {
		return nil, err
	}
	defer cache.Close()
	var missing []lfsPointer
	for _, pointer := range objects {
		file, err := openLFSObjectIn(cache, pointer.oid)
		if errors.Is(err, fs.ErrNotExist) {
			missing = append(missing, pointer)
			continue
		}
		if err != nil {
			return nil, err
		}
		file.Close()
	}
	return missing, nil
}

func (p lfsDownloadPlan) downloadLocal(ctx context.Context, root *os.Root, objects []lfsPointer) error {
	if p.remoteFiles.mediaError != "" {
		return errors.New(p.remoteFiles.mediaError)
	}
	source, err := p.remoteFiles.cache.open(p.remoteFiles.mediaPath, false)
	if err != nil {
		return fmt.Errorf("open the remote LFS cache: %w", err)
	}
	defer source.Close()
	return copyLFSMedia(ctx, root, source, objects)
}

func copyLFSMedia(ctx context.Context, destination, source *os.Root, objects []lfsPointer) error {
	for _, pointer := range objects {
		file, err := openLFSObjectIn(source, pointer.oid)
		if err != nil {
			return fmt.Errorf("open Git LFS object %s: %w", pointer.oid, err)
		}
		err = storeVerifiedLFSMedia(ctx, destination, file, pointer)
		file.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func (p lfsDownloadPlan) downloadNetwork(ctx context.Context, repo *Repository, private string, objects []lfsPointer,
	transport *transportAttempt) error {
	stage, err := repo.prepareTransportRepo(ctx, private, "")
	if err != nil {
		return err
	}
	stage.objects = filepath.Join(stage.dir, "objects")
	const ref = "refs/heads/gitwave-download"
	// Git LFS fetch scans every pointer-shaped blob in a ref. Give it only
	// the missing objects that passed the selected paths and Git attributes.
	var input strings.Builder
	input.WriteString("commit " + ref + "\ncommitter GitWave <gitwave@localhost> 1 +0000\ndata 0\n")
	for _, object := range objects {
		var pointer strings.Builder
		if err := object.writeTo(&pointer); err != nil {
			return err
		}
		fmt.Fprintf(&input, "M 100644 inline %s\ndata %d\n%s\n", object.oid, pointer.Len(), pointer.String())
	}
	cmd := stage.command(ctx, input.String(), "fast-import", "--quiet")
	if _, err := readGitOutput(ctx, cmd, 64<<10, "prepare exact Git LFS download"); err != nil {
		return err
	}
	media := p.httpEndpoint()
	if err := stage.bindCredentialContext(ctx, media); err != nil {
		return err
	}
	if err := stage.bindProxyContext(ctx, "", media); err != nil {
		return err
	}
	remoteURL := p.source.endpoint
	if media != "" {
		// Git LFS prefers the named remote when asking Git for credentials.
		// This private remote performs only media requests.
		remoteURL = media
	}
	alias := "gitwave-lfs:" + rand.Text()
	settings := [][2]string{
		{"lfs.storage", private},
		{"remote.gitwave-lfs.url", alias}, {"url." + remoteURL + ".insteadOf", alias},
		{"lfs.fetchrecentalways", "false"},
		{"lfs.basictransfersonly", "true"}, {"lfs.standalonetransferagent", ""},
		{"lfs.transfer.enablehrefrewrite", "false"},
	}
	if media != "" {
		settings = append(settings, [2]string{"lfs.url", alias},
			[2]string{"lfs." + media + ".standalonetransferagent", ""})
	}
	for i := 1; i < len(transport.options); i += 2 {
		key, value, _ := strings.Cut(transport.options[i], "=")
		settings = append(settings, [2]string{key, value})
	}
	for _, setting := range settings {
		if err := stage.addConfig(ctx, setting[0], setting[1]); err != nil {
			return err
		}
	}
	cmd = p.command(ctx, stage, proxy.CommandEnvironment(ctx, gitEnvironment()), "fetch", "--include=", "--exclude=", "gitwave-lfs", ref)
	cmd.Env = append(cmd.Env, transport.env...)
	config := stage.command(ctx, "", "config", "--null", "--get", "core.sshCommand")
	if err := setLFSDefaultSSH(ctx, cmd, config, transport.defaultSSH); err != nil {
		return err
	}
	var diagnostic gitDiagnostic
	cmd.Stdout, cmd.Stderr = &diagnostic, &diagnostic
	if err := process.Run(cmd); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		message := strings.TrimSpace(diagnostic.buffer.String())
		if isHTTPSAuthenticationError(message) && media != "" && transport.protocol == "https" {
			return ErrHTTPSAuthentication
		}
		if transport.auth != nil {
			message = strings.ReplaceAll(message, transport.auth.Password, "[redacted]")
		}
		if message != "" {
			return fmt.Errorf("download Git LFS objects: %s", redactProxyDiagnostic(message, cmd.Env, stage.proxyURLs))
		}
		return fmt.Errorf("download Git LFS objects: %w", err)
	}
	return nil
}

func setLFSDefaultSSH(ctx context.Context, cmd, config *exec.Cmd, executable string) error {
	if executable == "" {
		return nil
	}
	// Git LFS gives GIT_SSH priority over core.sshCommand, unlike Git itself.
	_, err := readGitOutput(ctx, config, 64<<10, "read SSH command for Git LFS")
	cmd.Env = slices.DeleteFunc(cmd.Env, func(entry string) bool { return strings.HasPrefix(entry, "GIT_SSH=") })
	if gitExitCode(err, 1) {
		cmd.Env = append(cmd.Env, "GIT_SSH="+executable)
		return nil
	}
	return err
}

func (p lfsDownloadPlan) command(ctx context.Context, stage *transportRepo, env []string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, p.executable, args...)
	process.Hide(cmd)
	cmd.WaitDelay = 2 * time.Second
	cmd.Dir = stage.dir
	cmd.Env = append(gittool.Environment(env, stage.repo.git, p.executable),
		"GIT_DIR="+stage.dir, "GIT_OBJECT_DIRECTORY="+stage.objects)
	return cmd
}

func sameHTTPSOrigin(first, second string) bool {
	a, err := url.Parse(first)
	b, secondErr := url.Parse(second)
	return err == nil && secondErr == nil && a.Scheme == "https" && b.Scheme == a.Scheme &&
		a.Host != "" && strings.EqualFold(a.Host, b.Host)
}
