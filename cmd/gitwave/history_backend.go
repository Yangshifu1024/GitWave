package main

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"gitwave.work/internal/gitcli"
)

const (
	historyPageSize  = 300
	historyScanLimit = 2_000
	historyCommitCap = 100_000
)

type historySnapshot struct {
	id        uint64
	filter    string
	refs      gitcli.References
	oids      []string
	truncated bool
	labels    map[string][]commitRef
	commits   []historyEntry
	lanes     []laneReservation
}

type historyEntry struct {
	row    commit
	search string
}

type laneReservation struct{ sha, branch string }

func visibleBranch(ref gitcli.Reference) (name, kind string) {
	if name, ok := strings.CutPrefix(ref.Name, "refs/heads/"); ok {
		return name, "local"
	}
	if name, ok := strings.CutPrefix(ref.Name, "refs/remotes/"); ok && name != "HEAD" && !strings.HasSuffix(name, "/HEAD") {
		return name, "remote"
	}
	return "", ""
}

func (b *backend) loadRepository(ctx context.Context) (repository, error) {
	refs, err := b.repo.References(ctx)
	if err != nil {
		return repository{}, err
	}
	return b.repositoryFromRefs(ctx, refs)
}

func (b *backend) repositoryFromRefs(ctx context.Context, refs gitcli.References) (repository, error) {
	remotes, err := b.repo.RemoteNames(ctx)
	if err != nil {
		return repository{}, err
	}
	info := repository{Head: refs.Head, Shallow: refs.Shallow, Remotes: remotes}
	if refs.Head != "" {
		info.Branch = "HEAD"
		if refs.HeadRef != "" {
			info.Branch = strings.TrimPrefix(refs.HeadRef, "refs/heads/")
		}
	}
	for _, ref := range refs.Refs {
		if name, ok := strings.CutPrefix(ref.Name, "refs/tags/"); ok {
			tip, err := b.repo.ReferenceCommit(ctx, ref)
			if err != nil {
				return repository{}, err
			}
			info.Tags = append(info.Tags, tagRef{Name: name, Tip: tip, Object: ref.OID, Annotated: ref.Type == "tag"})
			continue
		}
		name, kind := visibleBranch(ref)
		if kind == "" {
			continue
		}
		upstream := strings.TrimPrefix(strings.TrimPrefix(ref.Upstream, "refs/heads/"), "refs/remotes/")
		tip := ref.OID
		if ref.Symbolic != "" {
			tip = ""
		}
		info.Branches = append(info.Branches, branch{Name: name, Kind: kind, Current: ref.Name == refs.HeadRef, Tip: tip,
			Upstream: upstream, UpstreamRef: ref.Upstream, Time: ref.CommitTime})
	}
	if refs.Head == "" && refs.HeadRef != "" {
		name := strings.TrimPrefix(refs.HeadRef, "refs/heads/")
		index := slices.IndexFunc(info.Branches, func(branch branch) bool { return branch.Kind == "remote" })
		if index == -1 {
			index = len(info.Branches)
		}
		info.Branches = slices.Insert(info.Branches, index, branch{Name: name, Kind: "local", Current: true})
	}
	return info, nil
}

func (b *backend) loadCommit(ctx context.Context, oid string) (commitDetails, error) {
	commits, err := b.repo.Commits(ctx, []string{oid})
	if err != nil {
		return commitDetails{}, err
	}
	details := commits[0]
	details.Message = strings.TrimSpace(details.Message)
	return details, nil
}

func (b *backend) historyContains(oid string) bool {
	b.historyMu.Lock()
	defer b.historyMu.Unlock()
	return b.snapshot != nil && slices.Contains(b.snapshot.oids, oid)
}

func (b *backend) paletteHistorySnapshot(ctx context.Context, refs gitcli.References, filter string) (*historySnapshot, error) {
	b.historyMu.Lock()
	snapshot := b.snapshot
	if snapshot != nil && sameReferences(snapshot.refs, refs) {
		// The graph owns its rows and lane allocation. Its input order and labels are immutable.
		search := &historySnapshot{id: 1, filter: filter, refs: refs,
			oids: snapshot.oids, labels: snapshot.labels, truncated: snapshot.truncated}
		b.historyMu.Unlock()
		return search, nil
	}
	b.historyMu.Unlock()
	isolated := backend{repo: b.repo}
	return isolated.newHistorySnapshot(ctx, refs, filter)
}

func (b *backend) fetchHistory(ctx context.Context, cursor, filter string) (historyPage, bool, error) {
	b.historyMu.Lock()
	defer b.historyMu.Unlock()
	refs, err := b.repo.References(ctx)
	if err != nil {
		return historyPage{}, cursor == "", err
	}
	filter = strings.ToLower(strings.TrimSpace(filter))
	s := b.snapshot
	if cursor != "" && s != nil && s.filter == filter && sameReferences(s.refs, refs) {
		start, ok := s.offset(cursor)
		if ok {
			page, err := s.page(ctx, b.repo, start)
			return page, false, err
		}
	}

	s, err = b.newHistorySnapshot(ctx, refs, filter)
	if err != nil {
		return historyPage{}, true, err
	}
	b.snapshot = s
	page, err := s.page(ctx, b.repo, 0)
	if err != nil || cursor == "" || !page.More {
		return page, true, err
	}
	// If only the refs changed, the first page can still have the same rows.
	// Continue the scroll request so the virtual list gets a new boundary.
	start, _ := s.offset(page.Cursor)
	next, err := s.page(ctx, b.repo, start)
	if err != nil {
		return page, true, err
	}
	next.Commits = append(page.Commits, next.Commits...)
	return next, true, nil
}

// refreshHistory prepares the replacement before publishing a new cursor
// generation. It preserves the old loaded depth and leaves a usable snapshot
// in place when a refresh fails or is canceled.
func (b *backend) refreshHistory(ctx context.Context, filter string, loaded int, force bool) (repository, *historyPage, error) {
	b.historyMu.Lock()
	defer b.historyMu.Unlock()
	refs, err := b.repo.References(ctx)
	if err != nil {
		return repository{}, nil, err
	}
	info, err := b.repositoryFromRefs(ctx, refs)
	if err != nil {
		return repository{}, nil, err
	}
	filter = strings.ToLower(strings.TrimSpace(filter))
	if previous := b.snapshot; !force && previous != nil && previous.filter == filter && sameReferences(previous.refs, refs) {
		return info, nil, nil
	}
	snapshot, err := b.newHistorySnapshot(ctx, refs, filter)
	if err != nil {
		return repository{}, nil, err
	}
	var result historyPage
	start := 0
	for {
		page, err := snapshot.page(ctx, b.repo, start)
		if err != nil {
			return repository{}, nil, err
		}
		page.Commits = append(result.Commits, page.Commits...)
		result = page
		if !page.More || len(result.Commits) >= max(loaded, 1) {
			break
		}
		start, _ = snapshot.offset(page.Cursor)
	}
	if err := ctx.Err(); err != nil {
		return repository{}, nil, err
	}
	b.snapshot = snapshot
	return info, &result, nil
}

func sameReferences(a, b gitcli.References) bool {
	// Packing refs can change the starting order of a new walk. Existing
	// cursors keep their order until something changes the logical refs.
	return a.Head == b.Head && a.HeadRef == b.HeadRef && a.Shallow == b.Shallow &&
		slices.EqualFunc(a.Refs, b.Refs, func(a, b gitcli.Reference) bool {
			a.Loose, b.Loose = false, false
			return a == b
		})
}

func (b *backend) newHistorySnapshot(ctx context.Context, refs gitcli.References, filter string) (*historySnapshot, error) {
	labels := make(map[string][]commitRef)
	for _, ref := range refs.Refs {
		if strings.HasPrefix(ref.Name, "refs/heads/") || strings.HasPrefix(ref.Name, "refs/remotes/") {
			if ref.Type == "commit" && ref.Symbolic == "" {
				if name, kind := visibleBranch(ref); kind != "" {
					labels[ref.OID] = append(labels[ref.OID], commitRef{Name: name, Kind: kind + "_branch"})
				}
			}
			continue
		}
		if name, ok := strings.CutPrefix(ref.Name, "refs/tags/"); ok {
			oid, err := b.repo.ReferenceCommit(ctx, ref)
			if err != nil {
				return nil, err
			}
			if oid != "" {
				labels[oid] = append(labels[oid], commitRef{Name: name, Kind: "tag"})
			}
		}
	}
	tips, err := b.repo.HistoryTips(ctx, refs)
	if err != nil {
		return nil, err
	}
	oids, err := b.repo.Walk(ctx, tips, historyCommitCap+1)
	if err != nil {
		return nil, err
	}
	b.historyID++
	return &historySnapshot{id: b.historyID, filter: filter, refs: refs, oids: oids[:min(len(oids), historyCommitCap)],
		truncated: len(oids) > historyCommitCap, labels: labels}, nil
}

func (s *historySnapshot) offset(cursor string) (int, bool) {
	id, position, found := strings.Cut(cursor, ":")
	start, err := strconv.Atoi(position)
	return start, found && id == strconv.FormatUint(s.id, 10) && err == nil && start >= 0 && start < len(s.oids)
}

func (s *historySnapshot) page(ctx context.Context, repo *gitcli.Repository, start int) (historyPage, error) {
	return s.pageWithLimit(ctx, repo, start, historyPageSize)
}

func (s *historySnapshot) pageWithLimit(ctx context.Context, repo *gitcli.Repository, start, limit int) (historyPage, error) {
	page := historyPage{Size: len(s.oids), Truncated: s.truncated}
	position, end := start, min(start+historyScanLimit, len(s.oids))
	for position < end && len(page.Commits) < limit {
		if position == len(s.commits) {
			commits, err := repo.Commits(ctx, s.oids[position:min(position+historyPageSize, end)])
			if err != nil {
				return historyPage{}, err
			}
			for _, c := range commits {
				s.commits = append(s.commits, s.historyEntry(c))
			}
		}
		entry := s.commits[position]
		if s.filter == "" || strings.Contains(entry.search, s.filter) {
			page.Commits = append(page.Commits, entry.row)
		}
		position++
	}
	if position < len(s.oids) {
		page.More = true
		page.Cursor = fmt.Sprintf("%d:%d", s.id, position)
	}
	return page, nil
}

func (s *historySnapshot) historyEntry(c gitcli.Commit) historyEntry {
	message, _, _ := strings.Cut(c.Message, "\n")
	refs := s.labels[c.SHA]
	row := commit{SHA: c.SHA, Parents: c.Parents, Author: c.Author, Email: c.Email, Time: c.Time,
		Message: strings.TrimSuffix(message, "\r"), Refs: refs, Lane: s.assignLane(c.SHA, c.Parents, refs)}
	entry := historyEntry{row: row}
	if s.filter != "" {
		entry.search = strings.ToLower(strings.Join([]string{c.SHA, c.Author, c.Email, c.Message}, "\n"))
	}
	return entry
}

func (s *historySnapshot) assignLane(sha string, parents []string, refs []commitRef) int {
	lane := -1
	for i, slot := range s.lanes {
		if slot.sha != sha {
			continue
		}
		if lane == -1 {
			lane = i
		}
		if slices.ContainsFunc(refs, func(ref commitRef) bool { return ref.Kind != "tag" && ref.Name == slot.branch }) {
			lane = i
			break
		}
	}
	if lane == -1 {
		lane = s.freeLane()
	}
	lineage := primaryBranch(refs)
	if lineage == "" {
		lineage = s.lanes[lane].branch
	}
	for i := range s.lanes {
		if s.lanes[i].sha == sha {
			s.lanes[i] = laneReservation{}
		}
	}
	for i, parent := range parents {
		if i == 0 {
			s.lanes[lane] = laneReservation{parent, lineage}
		} else if !slices.ContainsFunc(s.lanes, func(slot laneReservation) bool { return slot.sha == parent }) {
			slot := s.freeLane()
			s.lanes[slot] = laneReservation{parent, primaryBranch(s.labels[parent])}
		}
	}
	return lane
}

func (s *historySnapshot) freeLane() int {
	if i := slices.Index(s.lanes, laneReservation{}); i != -1 {
		return i
	}
	s.lanes = append(s.lanes, laneReservation{})
	return len(s.lanes) - 1
}

func primaryBranch(refs []commitRef) string {
	for _, ref := range refs {
		if ref.Kind != "tag" {
			return ref.Name
		}
	}
	return ""
}
