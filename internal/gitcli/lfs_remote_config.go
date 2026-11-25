package gitcli

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
)

type lfsRemoteTarget struct {
	url, protocol, diagnostic string
}

func (r *Repository) lfsPushTarget(ctx context.Context, remote string) lfsRemoteTarget {
	url, protocol, err := r.lfsUploadURL(ctx, remote)
	if err != nil {
		return lfsRemoteTarget{diagnostic: err.Error()}
	}
	return lfsRemoteTarget{url: url, protocol: protocol}
}

func (r *Repository) lfsUploadURL(ctx context.Context, remote string) (string, string, error) {
	keys := []string{"lfs.pushurl", "lfs.url", "remote." + remote + ".lfspushurl", "remote." + remote + ".lfsurl"}
	urls, err := r.lfsConfigValues(ctx, keys, keys[2])
	if err != nil {
		return "", "", err
	}
	target := r.lfsConfiguredEndpoint(ctx, keys, urls, true)
	if target.diagnostic != "" {
		return "", "", fmt.Errorf("%s", target.diagnostic)
	}
	return target.url, target.protocol, nil
}

func (r *Repository) lfsConfigValues(ctx context.Context, keys []string, gitOnly ...string) (map[string]string, error) {
	return r.lfsConfigValuesAt(ctx, lfsConfigSource{}, keys, gitOnly...)
}

func (r *Repository) lfsConfigValuesAt(ctx context.Context, source lfsConfigSource, keys []string, gitOnly ...string) (map[string]string, error) {
	quoted := make([]string, 0, len(keys))
	for _, key := range keys {
		quoted = append(quoted, regexp.QuoteMeta(key))
	}
	pattern := "^(" + strings.Join(quoted, "|") + ")$"
	project, found, err := r.lfsTreeConfig(ctx, source.target, source.targetPath, pattern)
	if err == nil && !found {
		if source.cleanSource {
			project, _, err = r.lfsTreeConfig(ctx, source.original, source.restoredPath, pattern)
		} else {
			indexPath := ""
			if source.useIndex {
				indexPath = source.restoredPath
			}
			project, err = r.lfsProjectConfig(ctx, pattern, indexPath)
		}
	}
	if err != nil {
		return nil, err
	}
	user, err := r.readLimited(ctx, "", 64<<10, "config", "--null", "--get-regexp", pattern)
	if err != nil && !gitExitCode(err, 1) {
		return nil, err
	}
	urls := make(map[string]string, len(keys))
	for i, data := range []string{string(project), string(user)} {
		for record := range strings.SplitSeq(strings.TrimSuffix(data, "\x00"), "\x00") {
			key, value, ok := strings.Cut(record, "\n")
			if ok && (i != 0 || !slices.Contains(gitOnly, key)) {
				urls[key] = value
			}
		}
	}
	return urls, nil
}

func (r *Repository) lfsTreeConfig(ctx context.Context, tree, path, pattern string) ([]byte, bool, error) {
	if tree == "" {
		return nil, false, nil
	}
	if !validOID(tree) {
		return nil, false, fmt.Errorf("invalid checkout tree for .lfsconfig")
	}
	entry, err := r.readLimited(ctx, "", 4096, "ls-tree", "-z", tree, "--", path)
	if err != nil || len(entry) == 0 {
		return nil, false, err
	}
	header, name, _ := strings.Cut(string(entry), "\t")
	fields := strings.Fields(header)
	if name != path+"\x00" || len(fields) != 3 || (fields[0] != "100644" && fields[0] != "100755") ||
		fields[1] != "blob" || !validOID(fields[2]) {
		return nil, false, fmt.Errorf("the checkout's .lfsconfig is not a regular file")
	}
	data, err := r.readLFSProjectBlob(ctx, fields[2], pattern)
	return data, true, err
}

func (r *Repository) readLFSProjectBlob(ctx context.Context, oid, pattern string) ([]byte, error) {
	data, err := r.readLimited(ctx, "", 64<<10, "cat-file", "blob", oid)
	if err != nil {
		return nil, err
	}
	return r.readLFSProjectValues(ctx, data, pattern)
}

func (r *Repository) readLFSProjectValues(ctx context.Context, data []byte, pattern string) ([]byte, error) {
	values, err := r.readLimited(ctx, string(data), 64<<10, "config", "--no-includes", "--file", "-", "--null", "--get-regexp", pattern)
	if gitExitCode(err, 1) {
		return nil, nil
	}
	return values, err
}

func (r *Repository) lfsConfiguredEndpoint(ctx context.Context, keys []string, urls map[string]string, push bool) lfsRemoteTarget {
	for _, key := range keys {
		if address := urls[key]; address != "" {
			address, err := r.rewriteLFSURL(ctx, address, push)
			if err != nil {
				return lfsRemoteTarget{diagnostic: err.Error()}
			}
			endpoint, scheme, err := parseTransportSource(address, r.Path())
			if err != nil {
				return lfsRemoteTarget{diagnostic: fmt.Sprintf("invalid Git LFS endpoint in %s: %v", key, err)}
			}
			if scheme != "file" && scheme != "https" && scheme != "http" {
				return lfsRemoteTarget{diagnostic: "unsupported protocol for the configured Git LFS endpoint"}
			}
			return lfsRemoteTarget{url: endpoint, protocol: scheme}
		}
	}
	return lfsRemoteTarget{}
}

func (r *Repository) rewriteLFSURL(ctx context.Context, address string, upload bool) (string, error) {
	data, err := r.readLimited(ctx, "", 4<<20, "config", "--null", "--get-regexp", `^url\..+\.(insteadof|pushinsteadof)$`)
	if err != nil && !gitExitCode(err, 1) {
		return "", err
	}
	// Git LFS gives push rewrites priority over the general rewrite table.
	type rewrite struct{ prefix, target string }
	var push, general rewrite
	for record := range strings.SplitSeq(strings.TrimSuffix(string(data), "\x00"), "\x00") {
		key, prefix, ok := strings.Cut(record, "\n")
		if !ok || prefix == "" || !strings.HasPrefix(address, prefix) {
			continue
		}
		key = strings.TrimPrefix(key, "url.")
		if target, scoped := strings.CutSuffix(key, ".pushinsteadof"); scoped {
			if upload && len(prefix) >= len(push.prefix) {
				push = rewrite{prefix, target}
			}
		} else if len(prefix) >= len(general.prefix) {
			general = rewrite{prefix, strings.TrimSuffix(key, ".insteadof")}
		}
	}
	if push.prefix != "" {
		return push.target + strings.TrimPrefix(address, push.prefix), nil
	}
	return general.target + strings.TrimPrefix(address, general.prefix), nil
}

func (r *Repository) lfsProjectConfig(ctx context.Context, pattern, restoredIndexPath string) ([]byte, error) {
	if r.WorkTree != "" && restoredIndexPath == "" {
		root, err := os.OpenRoot(r.WorkTree)
		if err != nil {
			return nil, err
		}
		file, err := readGitStorageSnapshot(root, ".lfsconfig", 64<<10)
		root.Close()
		if err != nil {
			return nil, err
		}
		if file.mode != 0 {
			return r.readLFSProjectValues(ctx, file.data, pattern)
		}
	}
	lookups := []string{"HEAD:.lfsconfig"}
	indexPath := ".lfsconfig"
	if restoredIndexPath != "" {
		indexPath = restoredIndexPath
	}
	if r.WorkTree != "" {
		lookups = append([]string{":" + indexPath}, lookups...)
	}
	for _, name := range lookups {
		oid, err := r.optionalRef(ctx, "rev-parse", "--verify", "--quiet", name)
		if err != nil {
			return nil, err
		}
		if oid == "" {
			continue
		}
		args := []string{"ls-tree", "-z", "HEAD", "--", ".lfsconfig"}
		objectField := 2
		wantPath := ".lfsconfig"
		if name == ":"+indexPath {
			args = []string{"ls-files", "--stage", "-z", "--", indexPath}
			objectField = 1
			wantPath = indexPath
		}
		entry, err := r.readLimited(ctx, "", 4096, args...)
		if err != nil {
			return nil, err
		}
		header, path, valid := strings.Cut(string(entry), "\t")
		fields := strings.Fields(header)
		if !valid || path != wantPath+"\x00" || len(fields) != 3 ||
			(fields[0] != "100644" && fields[0] != "100755") || fields[objectField] != oid {
			return nil, fmt.Errorf("the tracked .lfsconfig is not a stable regular file")
		}
		return r.readLFSProjectBlob(ctx, oid, pattern)
	}
	return nil, nil
}

type localLFSTransport struct {
	gitIdentity, gitError    string
	mediaIdentity, mediaPath string
	mediaError               string
	cache                    lfsCacheBinding
}

func inspectLocalLFSTransport(ctx context.Context, source remoteSource, lfs lfsRemoteTarget) localLFSTransport {
	var result localLFSTransport
	var gitTarget *Repository
	if source.protocol == "file" {
		target, identity, err := identifyLocalPushRepository(ctx, source.endpoint)
		if err != nil {
			result.gitError = err.Error()
		} else {
			gitTarget = target
			result.gitIdentity = identity
		}
	}
	var target *Repository
	switch {
	case lfs.protocol == "file":
		repo, identity, err := identifyLocalPushRepository(ctx, lfs.url)
		if err != nil {
			result.mediaError = err.Error()
			return result
		}
		target, result.mediaIdentity = repo, identity
	case lfs.url == "" && source.protocol == "file":
		if gitTarget == nil {
			result.mediaError = result.gitError
			return result
		}
		target, result.mediaIdentity = gitTarget, result.gitIdentity
	}
	if target != nil {
		_, config, err := target.workingGitSettings(ctx)
		if err != nil {
			result.mediaError = err.Error()
		} else {
			result.mediaPath = config.storage
			result.cache, err = bindLFSCache(result.mediaPath)
			if err != nil {
				result.mediaError = err.Error()
			}
		}
	}
	return result
}

func (shown localLFSTransport) matches(current localLFSTransport) bool {
	current.cache = shown.cache
	if current != shown {
		return false
	}
	return shown.mediaPath == "" || shown.mediaError != "" || shown.cache.stillAt(shown.mediaPath)
}

func identifyLocalPushRepository(ctx context.Context, endpoint string) (*Repository, string, error) {
	target, err := openLocalPushRepository(ctx, endpoint)
	if err != nil {
		return nil, "", err
	}
	checkoutID, err := lfsDirectoryIdentity(target.GitDir)
	if err != nil {
		return nil, "", err
	}
	if target.CommonDir == target.GitDir {
		return target, checkoutID, nil
	}
	commonID, err := lfsDirectoryIdentity(target.CommonDir)
	if err != nil {
		return nil, "", err
	}
	return target, checkoutID + "/" + commonID, nil
}
