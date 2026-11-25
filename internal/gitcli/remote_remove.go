package gitcli

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

type remoteTrackingRef struct {
	name     string
	oid      string
	symbolic string
}

func (r *Repository) remoteTrackingConfig(ctx context.Context, remote string, remove bool) ([]string, error) {
	const pattern = "^branch[.].*[.](remote|pushremote|merge)$|^remote[.]pushdefault$"
	entries, err := r.readConfigValues(ctx, pattern)
	if err != nil {
		return nil, err
	}
	grouped := make(map[string][]configValue)
	for _, entry := range entries {
		grouped[entry.key] = append(grouped[entry.key], entry)
	}
	var keys []string
	for key, values := range grouped {
		if strings.HasSuffix(key, ".merge") || values[len(values)-1].value != remote {
			continue
		}
		keys = append(keys, key)
		if remove && strings.HasSuffix(key, ".remote") {
			keys = append(keys, strings.TrimSuffix(key, ".remote")+".merge")
		}
	}
	slices.Sort(keys)
	for _, key := range keys {
		for _, entry := range grouped[key] {
			if !r.isPrimaryConfig(entry.origin) {
				return nil, fmt.Errorf("%q is configured outside this repository's config file", key)
			}
		}
	}
	return keys, nil
}

func (r *Repository) removeConfiguredRemote(ctx context.Context, expected RemoteInfo) error {
	return r.editRemoteConfig(ctx, func(stage *remoteConfigEdit) error {
		remote, err := stage.localRemote(expected, true)
		if err != nil {
			return err
		}
		items, err := r.Remotes(ctx)
		if err != nil {
			return err
		}
		refs, err := r.remoteTrackingRefs(ctx)
		if err != nil {
			return err
		}
		deleting := make(map[string]bool)
		for _, ref := range refs {
			if strings.HasPrefix(ref.name, "refs/remotes/") && remoteTracksRef(remote, ref.name) &&
				!slices.ContainsFunc(items, func(other RemoteInfo) bool {
					return other.Name != remote.Name && remoteTracksRef(other, ref.name)
				}) {
				deleting[ref.name] = true
			}
		}
		protectTrackingAliases(refs, deleting)
		for _, ref := range refs {
			if deleting[ref.name] {
				stage.deletes = append(stage.deletes, ref)
			}
		}
		keys, err := r.remoteTrackingConfig(ctx, remote.Name, true)
		if err != nil {
			return err
		}
		for _, key := range keys {
			if err := stage.unset(key); err != nil {
				return err
			}
		}
		_, err = r.readLimited(ctx, "", 64<<10, "config", "--file", stage.path, "--remove-section", "remote."+remote.Name)
		return err
	})
}

func (r *Repository) remoteTrackingRefs(ctx context.Context) ([]remoteTrackingRef, error) {
	data, err := r.remoteRefState(ctx)
	if err != nil {
		return nil, err
	}
	var refs []remoteTrackingRef
	for line := range strings.SplitSeq(strings.TrimSuffix(string(data), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\x00")
		if len(fields) != 3 || !strings.HasPrefix(fields[0], "refs/") || !validOID(fields[1]) {
			return nil, fmt.Errorf("invalid remote-tracking reference data")
		}
		if fields[2] == "" && !strings.HasPrefix(fields[0], "refs/remotes/") {
			continue
		}
		ref := remoteTrackingRef{name: fields[0], oid: fields[1]}
		if fields[2] != "" {
			ref.symbolic, err = r.optionalRef(ctx, "symbolic-ref", "--no-recurse", "-q", ref.name)
			if err != nil || ref.symbolic == "" {
				return nil, fmt.Errorf("remote-tracking reference %q changed while it was read", ref.name)
			}
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

// Follow aliases outside the deletion set so their targets survive.
func protectTrackingAliases(refs []remoteTrackingRef, deleting map[string]bool) {
	symbolic := make(map[string]string)
	for _, ref := range refs {
		if ref.symbolic != "" {
			symbolic[ref.name] = ref.symbolic
		}
	}
	for ref, target := range symbolic {
		if deleting[ref] {
			continue
		}
		for deleting[target] {
			delete(deleting, target)
			target = symbolic[target]
		}
	}
}
