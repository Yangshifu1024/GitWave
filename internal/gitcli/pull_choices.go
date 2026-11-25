package gitcli

import (
	"context"
	"slices"
	"strings"
)

type PullChoices struct {
	Remotes                        []PullRemote
	UpstreamRemote, UpstreamBranch string
}

type PullRemote struct {
	Name     string
	Branches []string
}

// PullChoices resolves source branch names from explicit mappings and
// local destinations already available for wildcard mappings.
func (r *Repository) PullChoices(ctx context.Context, local string) (PullChoices, error) {
	if _, err := r.checkedRefName(ctx, "heads", local); err != nil {
		return PullChoices{}, err
	}
	items, err := r.Remotes(ctx)
	if err != nil {
		return PullChoices{}, err
	}
	slices.SortFunc(items, func(a, b RemoteInfo) int { return strings.Compare(a.Name, b.Name) })
	remote, err := r.optionalRef(ctx, "config", "--get", "branch."+local+".remote")
	if err != nil {
		return PullChoices{}, err
	}
	upstream, err := r.optionalRef(ctx, "config", "--get", "branch."+local+".merge")
	if err != nil {
		return PullChoices{}, err
	}
	refs, err := r.References(ctx)
	if err != nil {
		return PullChoices{}, err
	}
	choices := PullChoices{Remotes: make([]PullRemote, 0, len(items))}
	if branch, ok := strings.CutPrefix(upstream, "refs/heads/"); ok &&
		slices.ContainsFunc(items, func(item RemoteInfo) bool { return item.Name == remote }) {
		choices.UpstreamRemote, choices.UpstreamBranch = remote, branch
	}
	for _, item := range items {
		name := item.Name
		entry := PullRemote{Name: name}
		refspecs, err := validateFetchRefspecs(name, item.urls("fetch"))
		if err == nil {
			entry.Branches = pullSourceNames(name, refs.Refs, refspecs)
		}
		if ctx.Err() != nil {
			return PullChoices{}, ctx.Err()
		}
		choices.Remotes = append(choices.Remotes, entry)
	}
	return choices, nil
}

func pullSourceNames(remote string, refs []Reference, refspecs []string) []string {
	branches := make(map[string]bool)
	add := func(source, dest string) {
		if chosen, err := pullTrackingReference(remote, source, refspecs); err == nil && chosen == dest {
			branches[strings.TrimPrefix(source, "refs/heads/")] = true
		}
	}
	for _, spec := range refspecs {
		source, dest, _ := strings.Cut(strings.TrimPrefix(spec, "+"), ":")
		if !strings.HasPrefix(source, "refs/heads/") ||
			!strings.HasPrefix(dest, "refs/remotes/") && localFetchDestination(spec) == "" {
			continue
		}
		if !strings.ContainsRune(source, '*') && !strings.ContainsRune(dest, '*') {
			add(source, dest)
			continue
		}
		for _, ref := range refs {
			if ref.Symbolic != "" {
				continue
			}
			if middle, ok := matchFetchSource(dest, ref.Name); ok {
				add(strings.Replace(source, "*", middle, 1), ref.Name)
			}
		}
	}
	names := make([]string, 0, len(branches))
	for name := range branches {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
