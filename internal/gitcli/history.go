package gitcli

import (
	"cmp"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type Reference struct {
	Name, OID, Type, PeeledOID, PeeledType, Symbolic, Upstream string
	Loose                                                      bool
	CommitTime                                                 int64
	Ahead, Behind                                              int
}

type References struct {
	Head, HeadRef, Shallow string
	Refs                   []Reference
	IgnoreCase             bool
}

func (r *Repository) References(ctx context.Context) (References, error) {
	return r.references(ctx, false)
}

func (r *Repository) ReferencesWithTracking(ctx context.Context) (References, error) {
	return r.references(ctx, true)
}

func (r *Repository) references(ctx context.Context, tracking bool) (References, error) {
	headRef, err := r.optionalRef(ctx, "symbolic-ref", "-q", "HEAD")
	if err != nil {
		return References{}, err
	}
	format := "--format=%(refname)%00%(objectname)%00%(objecttype)%00%(*objectname)%00%(*objecttype)%00%(symref)%00%(upstream)%00%(committerdate:unix)"
	columns := 8
	if tracking {
		format += "%00%(upstream:track,nobracket)"
		columns++
	}
	output, err := r.read(ctx, "", "for-each-ref", "--sort=refname", format,
		"refs/heads", "refs/remotes", "refs/tags")
	if err != nil {
		return References{}, err
	}
	refs := References{HeadRef: headRef}
	for line := range strings.SplitSeq(strings.TrimSuffix(string(output), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\x00")
		if len(fields) != columns || !validOID(fields[1]) {
			return References{}, fmt.Errorf("invalid Git reference data")
		}
		ref := Reference{Name: fields[0], OID: fields[1], Type: fields[2], PeeledOID: fields[3],
			PeeledType: fields[4], Symbolic: fields[5], Upstream: fields[6]}
		ref.CommitTime, _ = strconv.ParseInt(fields[7], 10, 64)
		if tracking {
			ref.readTracking(fields[8])
		}
		if ref.isHistoryBranch() {
			info, err := os.Stat(filepath.Join(r.CommonDir, filepath.FromSlash(ref.Name)))
			ref.Loose = err == nil && info.Mode().IsRegular()
		}
		refs.Refs = append(refs.Refs, ref)
		if fields[0] == headRef {
			refs.Head = fields[1]
		}
	}
	if !strings.HasPrefix(headRef, "refs/heads/") {
		refs.Head, err = r.optionalRef(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
		if err != nil {
			return References{}, err
		}
	}
	// Deepening a shallow clone can change History without moving any ref.
	shallow, err := os.ReadFile(filepath.Join(r.CommonDir, "shallow"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return References{}, err
	}
	refs.Shallow = string(shallow)
	if slices.ContainsFunc(refs.Refs, func(ref Reference) bool { return ref.Loose }) {
		ignoreCase, err := r.optionalRef(ctx, "config", "--type=bool", "--get", "core.ignorecase")
		if err != nil {
			return References{}, err
		}
		refs.IgnoreCase = ignoreCase == "true"
	}
	return refs, nil
}

func (r *Reference) readTracking(value string) {
	for part := range strings.SplitSeq(value, ", ") {
		kind, count, _ := strings.Cut(part, " ")
		switch kind {
		case "ahead":
			r.Ahead, _ = strconv.Atoi(count)
		case "behind":
			r.Behind, _ = strconv.Atoi(count)
		}
	}
}

func (r Reference) isHistoryBranch() bool {
	return (r.Type == "commit" || r.Type == "tag") && r.Symbolic == "" &&
		(strings.HasPrefix(r.Name, "refs/heads/") || strings.HasPrefix(r.Name, "refs/remotes/"))
}

// HistoryTips follows the old ref iterator's push order. Keep presentation refs
// name-sorted. History walks loose branches, packed branches and non-branch HEAD.
func (r *Repository) HistoryTips(ctx context.Context, refs References) ([]string, error) {
	branches := make([]Reference, 0, len(refs.Refs))
	for _, ref := range refs.Refs {
		if ref.isHistoryBranch() {
			branches = append(branches, ref)
		}
	}
	slices.SortStableFunc(branches, func(a, b Reference) int {
		if a.Loose != b.Loose {
			if a.Loose {
				return -1
			}
			return 1
		}
		if a.Loose && refs.IgnoreCase {
			return compareRefNamesFoldASCII(a.Name, b.Name)
		}
		return cmp.Compare(a.Name, b.Name)
	})
	tips := make([]string, 0, len(branches)+1)
	for _, branch := range branches {
		oid, err := r.ReferenceCommit(ctx, branch)
		if err != nil {
			return nil, err
		}
		if oid != "" {
			tips = append(tips, oid)
		}
	}
	if refs.Head != "" && !strings.HasPrefix(refs.HeadRef, "refs/heads/") {
		tips = append(tips, refs.Head)
	}
	return tips, nil
}

func (r *Repository) ReferenceCommit(ctx context.Context, ref Reference) (string, error) {
	switch {
	case ref.Type == "commit":
		return ref.OID, nil
	case ref.PeeledType == "commit":
		return ref.PeeledOID, nil
	case ref.PeeledType == "tag":
		if !validOID(ref.OID) {
			return "", fmt.Errorf("invalid Git object ID")
		}
		return r.optionalRef(ctx, "rev-parse", "--verify", "--quiet", ref.OID+"^{commit}")
	default:
		return "", nil
	}
}

func compareRefNamesFoldASCII(a, b string) int {
	lower := func(c byte) byte {
		if c >= 'A' && c <= 'Z' {
			return c + ('a' - 'A')
		}
		return c
	}
	for i := 0; i < min(len(a), len(b)); i++ {
		if order := cmp.Compare(lower(a[i]), lower(b[i])); order != 0 {
			return order
		}
	}
	return cmp.Compare(len(a), len(b))
}

func (r *Repository) optionalRef(ctx context.Context, args ...string) (string, error) {
	output, err := r.read(ctx, "", args...)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return "", nil
	}
	return strings.TrimSuffix(string(output), "\n"), err
}

type Commit struct {
	SHA, Author, Email, Message string
	Time                        int64
	Parents                     []string
}

// HasCommit reports whether the full object ID can be resolved to a commit.
func (r *Repository) HasCommit(ctx context.Context, oid string) (bool, error) {
	if !validOID(oid) {
		return false, fmt.Errorf("invalid Git commit ID")
	}
	output, err := r.readLimited(ctx, "", 128, "rev-parse", "--verify", "--quiet", "--end-of-options", oid+"^{commit}")
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 && len(exit.Stderr) == 0 {
		return false, nil
	}
	return strings.TrimSpace(string(output)) == oid, err
}

func (r *Repository) Commits(ctx context.Context, oids []string) ([]Commit, error) {
	const columns = 6
	fields, err := r.readCommitFields(ctx, oids, 0, "%H%x00%P%x00%an%x00%ae%x00%ct%x00%B", columns)
	if err != nil {
		return nil, err
	}
	commits := make([]Commit, len(oids))
	for i, oid := range oids {
		row := fields[i*columns : (i+1)*columns]
		stamp, err := strconv.ParseInt(row[4], 10, 64)
		if row[0] != oid || err != nil {
			return nil, fmt.Errorf("invalid Git commit data")
		}
		commits[i] = Commit{SHA: oid, Parents: strings.Fields(row[1]), Author: row[2], Email: row[3], Time: stamp, Message: row[5]}
	}
	return commits, nil
}

func (r *Repository) readCommitFields(ctx context.Context, oids []string, limit int64, format string, columns int) ([]string, error) {
	if len(oids) == 0 {
		return nil, nil
	}
	for _, oid := range oids {
		if !validOID(oid) {
			return nil, fmt.Errorf("invalid Git commit ID")
		}
	}
	output, err := r.readLimited(ctx, strings.Join(oids, "\n")+"\n", limit, "log", "--no-walk=unsorted", "--stdin",
		"--no-show-signature", "--no-notes", "--no-decorate", "--no-patch", "--color=never", "--encoding=UTF-8", "-z",
		"--format="+format, "--")
	if err != nil {
		return nil, err
	}
	fields := strings.Split(string(output), "\x00")
	if len(fields) != len(oids)*columns+1 || fields[len(fields)-1] != "" {
		return nil, fmt.Errorf("incomplete Git commit data")
	}
	return fields, nil
}

func validOID(oid string) bool {
	return validOIDBytes([]byte(oid))
}

func validOIDBytes(oid []byte) bool {
	if len(oid) != 40 && len(oid) != 64 {
		return false
	}
	var decoded [32]byte
	_, err := hex.Decode(decoded[:], oid)
	return err == nil
}
