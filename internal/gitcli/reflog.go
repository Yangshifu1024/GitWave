package gitcli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type ReflogEntry struct {
	OldOID    string
	NewOID    string
	Message   string
	Action    string
	Committer string
	Time      int64
}

func (e ReflogEntry) RecoveryOID() string {
	if e.Action == "reset" || e.Action == "amend" {
		return e.OldOID
	}
	return e.NewOID
}

// Reflog reads the actual before and after OIDs. Git's formatted reflog
// includes only the new OID, and a retained record can refer to a previous
// state whose own record has expired.
func (r *Repository) Reflog(ctx context.Context, reference string) ([]ReflogEntry, error) {
	ref := strings.TrimSpace(reference)
	dir := r.GitDir
	if ref == "HEAD" {
		info, err := r.References(ctx)
		if err != nil || info.Head == "" {
			return nil, err
		}
	} else {
		if strings.HasPrefix(ref, "refs/") && !strings.HasPrefix(ref, "refs/heads/") {
			return nil, fmt.Errorf("select HEAD or a local branch reflog")
		}
		name := strings.TrimPrefix(ref, "refs/heads/")
		full, err := r.checkedRefName(ctx, "heads", name)
		if err != nil {
			return nil, err
		}
		if oid, err := r.exactRootRef(ctx, full); err != nil || oid == "" {
			return nil, err
		}
		ref, dir = full, r.CommonDir
	}
	format, err := r.readLimited(ctx, "", 4096, "rev-parse", "--show-ref-format")
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	switch strings.TrimSpace(string(format)) {
	case "files":
		log := filepath.FromSlash("logs/" + ref)
		data, _, err := readGitStorageFile(root, log, 64<<20)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		entries := parseFileReflog(ctx, string(data))
		return entries, ctx.Err()
	case "reftable":
		return readReftableReflog(ctx, root, ref)
	default:
		return nil, fmt.Errorf("unsupported Git reference format %q", strings.TrimSpace(string(format)))
	}
}

func parseFileReflog(ctx context.Context, data string) []ReflogEntry {
	var entries []ReflogEntry
	// An incomplete trailing record can occur while another Git process
	// appends to the log. It will be available after the next refresh.
	data = data[:strings.LastIndex(data, "\n")+1]
	for line := range strings.SplitSeq(data, "\n") {
		if ctx.Err() != nil {
			return nil
		}
		if line == "" {
			continue
		}
		meta, message, _ := strings.Cut(line, "\t")
		old, rest, _ := strings.Cut(meta, " ")
		next, identity, _ := strings.Cut(rest, " ")
		end := strings.LastIndex(identity, "> ")
		start := strings.LastIndex(identity, " <")
		if !validOID(old) || !validOID(next) || len(old) != len(next) || start < 0 || end <= start {
			continue
		}
		seconds, _, _ := strings.Cut(identity[end+2:], " ")
		when, err := strconv.ParseInt(seconds, 10, 64)
		if err == nil {
			entries = append(entries, newReflogEntry(old, next, message, identity[:start], when))
		}
	}
	slices.Reverse(entries)
	return entries
}

func newReflogEntry(old, next, message, committer string, when int64) ReflogEntry {
	message = strings.TrimSuffix(message, "\n")
	return ReflogEntry{
		OldOID: nonzeroReflogOID(old), NewOID: nonzeroReflogOID(next),
		Message: strings.ToValidUTF8(message, "�"), Committer: strings.ToValidUTF8(committer, "�"),
		Action: classifyReflog(message), Time: when,
	}
}

func nonzeroReflogOID(oid string) string {
	if strings.Trim(oid, "0") == "" {
		return ""
	}
	return oid
}

func classifyReflog(message string) string {
	text := strings.ToLower(strings.TrimSpace(message))
	for _, action := range [...]struct{ prefix, kind string }{
		{"commit (initial)", "initial_commit"}, {"commit (amend)", "amend"}, {"commit", "commit"},
		{"checkout", "checkout"}, {"reset", "reset"}, {"merge", "merge"}, {"rebase", "rebase"},
		{"pull", "pull"}, {"fetch", "fetch"}, {"push", "push"}, {"branch", "branch"},
		{"revert", "revert"}, {"cherry-pick", "cherry_pick"}, {"stash", "stash"}, {"clone", "clone"},
	} {
		if strings.HasPrefix(text, action.prefix) {
			return action.kind
		}
	}
	return "other"
}
