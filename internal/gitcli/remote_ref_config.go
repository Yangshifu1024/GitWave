package gitcli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
)

func (r *Repository) renameRemoteWithURLs(ctx context.Context, expected RemoteInfo, name, fetchURL, pushURL string) error {
	return r.editRemoteConfig(ctx, func(stage *remoteConfigEdit) error {
		remote, err := stage.localRemote(expected, true)
		if err != nil {
			return err
		}
		names, err := r.RemoteNames(ctx)
		if err != nil {
			return err
		}
		if err := availableRemoteName(names, remote.Name, name); err != nil {
			return err
		}
		if _, err := r.readLimited(ctx, "", 64<<10, "config", "--file", stage.path, "--rename-section",
			"remote."+remote.Name, "remote."+name); err != nil {
			return err
		}
		keys, err := r.remoteTrackingConfig(ctx, remote.Name, false)
		if err != nil {
			return err
		}
		for _, key := range keys {
			if _, err := r.readLimited(ctx, "", 64<<10, "config", "--file", stage.path, "--replace-all", key, name); err != nil {
				return err
			}
		}
		from, to := "refs/remotes/"+remote.Name+"/", "refs/remotes/"+name+"/"
		specs := remote.urls("fetch")
		moveRefs := false
		for i, spec := range specs {
			src, dest, mapped := strings.Cut(spec, ":")
			if rest, owned := strings.CutPrefix(dest, from); mapped && owned && !strings.HasPrefix(src, "^") {
				specs[i] = src + ":" + to + rest
				moveRefs = true
			}
		}
		if moveRefs {
			if err := stage.values(name, "fetch", specs); err != nil {
				return err
			}
			if err := stage.planRemoteRename(remote, from, to); err != nil {
				return err
			}
		}
		remote.Name = name
		return stage.setURLs(remote, fetchURL, pushURL)
	})
}

func (s *remoteConfigEdit) planRemoteRename(remote RemoteInfo, from, to string) error {
	format, err := s.repo.optionalRef(s.ctx, "rev-parse", "--show-ref-format")
	if err != nil {
		return err
	}
	if format == "reftable" {
		s.reftableRename = &remoteRenameRequest{remote: remote, from: from, to: to}
		return nil
	}
	items, err := s.repo.Remotes(s.ctx)
	if err != nil {
		return err
	}
	refs, err := s.repo.remoteTrackingRefs(s.ctx)
	if err != nil {
		return err
	}
	if err := s.appendUnresolvedRemoteRefs(from, to, &refs); err != nil {
		return err
	}
	return s.planRemoteRefMoves(remote, from, to, items, refs)
}

type remoteRenameRequest struct {
	remote   RemoteInfo
	from, to string
}

func (s *remoteConfigEdit) planRemoteRefMoves(remote RemoteInfo, from, to string, items []RemoteInfo, refs []remoteTrackingRef) error {
	deleting := make(map[string]bool)
	for _, ref := range refs {
		if tail, own := strings.CutPrefix(ref.name, from); own {
			if !remoteTracksRef(remote, ref.name) && ref.name != from+"HEAD" {
				continue
			}
			next := ref
			next.name = to + tail
			if dest, owned := strings.CutPrefix(ref.symbolic, from); owned &&
				(remoteTracksRef(remote, ref.symbolic) || ref.symbolic == from+"HEAD") {
				next.symbolic = to + dest
			}
			s.moves = append(s.moves, remoteRefMove{from: ref, to: next})
			if !slices.ContainsFunc(items, func(item RemoteInfo) bool {
				return item.Name != remote.Name && remoteTracksRef(item, ref.name)
			}) {
				deleting[ref.name] = true
			}
		} else if strings.HasPrefix(ref.name, to) {
			return fmt.Errorf("remote tracking references already exist in %q", to)
		}
	}
	protectTrackingAliases(refs, deleting)
	for _, move := range s.moves {
		if deleting[move.from.name] {
			s.deletes = append(s.deletes, move.from)
		}
	}
	return nil
}

func remoteTracksRef(remote RemoteInfo, ref string) bool {
	specs := remote.urls("fetch")
	found := false
	for _, spec := range specs {
		src, dest, ok := strings.Cut(strings.TrimPrefix(spec, "+"), ":")
		if !ok || strings.HasPrefix(src, "^") {
			continue
		}
		middle, matches := matchRefspecPattern(dest, ref)
		if !matches {
			continue
		}
		source := strings.Replace(src, "*", middle, 1)
		excluded := slices.ContainsFunc(specs, func(spec string) bool {
			pattern, negative := strings.CutPrefix(spec, "^")
			_, matches := matchRefspecPattern(pattern, source)
			return negative && matches
		})
		if excluded {
			return false
		}
		found = true
	}
	return found
}

func matchRefspecPattern(pattern, ref string) (string, bool) {
	before, after, wildcard := strings.Cut(pattern, "*")
	if !wildcard {
		return "", pattern == ref
	}
	middle, ok := strings.CutPrefix(ref, before)
	if !ok {
		return "", false
	}
	return strings.CutSuffix(middle, after)
}

func (r *Repository) remoteRefState(ctx context.Context) ([]byte, error) {
	return r.readLimited(ctx, "", 32<<20, "for-each-ref", "--sort=refname",
		"--format=%(refname)%00%(objectname)%00%(symref)")
}

const remoteConfigLimit = 16 << 20

func (r *Repository) restoreRemoteConfig(before, published []byte) error {
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
	current, mode, err := readGitStorageFile(common, "config", remoteConfigLimit)
	if err != nil || bytes.Equal(current, before) {
		return err
	}
	if !bytes.Equal(current, published) {
		return fmt.Errorf("the Git config changed during the operation; reload the remote list before retrying")
	}
	if err := lock.prepare(before, mode); err != nil {
		return err
	}
	return lock.publish()
}
