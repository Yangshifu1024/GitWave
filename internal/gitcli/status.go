package gitcli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

type Status struct {
	HeadOID          string
	HeadRef          string
	Branch           string
	Upstream         string
	Ahead            int
	Behind           int
	Changes          []Change
	Commit           CommitState
	SubmoduleWarning string
}

type Change struct {
	Kind         byte
	Index        byte
	WorkTree     byte
	Submodule    string
	Path         string
	PreviousPath string
	HeadOID      string
	IndexOID     string
	WorkTreeOID  string
	HeadMode     string
	IndexMode    string
	WorkTreeMode string
	Conflict     [3]IndexStage
}

type IndexStage struct{ Mode, OID string }

// SubmoduleFilesOnly reports an unstaged change that the parent cannot stage.
func (c Change) SubmoduleFilesOnly() bool {
	return c.hasSubmoduleFileChanges() && c.Submodule[1] == '.'
}

func (c Change) hasSubmoduleFileChanges() bool {
	return len(c.Submodule) == 4 && c.Submodule[0] == 'S' &&
		(c.Submodule[2] == 'M' || c.Submodule[3] == 'U')
}

func (r *Repository) Status(ctx context.Context) (Status, error) {
	ref, err := r.statusReference(ctx)
	if err != nil {
		return Status{}, err
	}
	status, err := r.workingStatus(ctx, ref.OID, "--untracked-files=all", "--renames", "--branch")
	if err != nil {
		return Status{}, err
	}
	status.Commit, err = r.commitState(ctx)
	if err != nil {
		return Status{}, err
	}
	if ref.Name != "" {
		status.Branch = strings.TrimPrefix(ref.Name, "refs/heads/")
	}
	status.HeadRef = ref.Name
	status.Upstream = strings.TrimPrefix(strings.TrimPrefix(ref.Upstream, "refs/heads/"), "refs/remotes/")
	status.Ahead, status.Behind = ref.Ahead, ref.Behind
	return status, nil
}

// The Changes view refreshes often and needs only the checked-out reference.
// Read its upstream and tracking counts with the same Git ref lookup.
func (r *Repository) statusReference(ctx context.Context) (Reference, error) {
	headRef, err := r.optionalRef(ctx, "symbolic-ref", "-q", "HEAD")
	if err != nil {
		return Reference{}, err
	}
	if !strings.HasPrefix(headRef, "refs/heads/") {
		oid, err := r.optionalRef(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
		return Reference{Name: headRef, OID: oid}, err
	}
	ref, err := r.localStatusReference(ctx, headRef, "%(upstream:track,nobracket)")
	if err == nil || ctx.Err() != nil {
		return ref, err
	}
	// Custom fetch namespaces used to yield zero tracking counts. Preserve
	// local status when Git cannot traverse history in such an upstream.
	metadata, metadataErr := r.localStatusReference(ctx, headRef, "")
	if metadataErr != nil || metadata.Upstream == "" {
		return Reference{}, err
	}
	namespace, _ := strings.CutPrefix(metadata.Upstream, "refs/")
	namespace, _, _ = strings.Cut(namespace, "/")
	switch namespace {
	case "heads", "remotes", "tags":
		return Reference{}, err
	default:
		return metadata, nil
	}
}

func (r *Repository) localStatusReference(ctx context.Context, headRef, trackAtom string) (Reference, error) {
	format := "--format=%(refname)%00%(objectname)%00%(upstream)%00" + trackAtom
	// A literal ref pattern also matches descendants. Exclude them before
	// Git computes tracking data, even when HEAD itself is unborn.
	output, err := r.readLimited(ctx, "", 128<<10, "for-each-ref", "--count=1", "--exclude="+headRef+"/", format, "--", headRef)
	if err != nil {
		return Reference{}, err
	}
	ref := Reference{Name: headRef}
	if len(output) == 0 {
		return ref, nil
	}
	fields := strings.Split(strings.TrimSuffix(string(output), "\n"), "\x00")
	if len(fields) != 4 || !validOID(fields[1]) {
		return Reference{}, fmt.Errorf("invalid Git checkout reference")
	}
	if fields[0] == headRef {
		ref.OID, ref.Upstream = fields[1], fields[2]
		ref.readTracking(fields[3])
	}
	return ref, nil
}

// WorkingChangeCount reads the working tree and index without counting branch divergence.
func (r *Repository) WorkingChangeCount(ctx context.Context) (int, error) {
	if r.WorkTree == "" {
		return 0, nil
	}
	head, err := r.optionalRef(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil {
		return 0, err
	}
	status, err := r.workingStatus(ctx, head, "--untracked-files=all", "--renames")
	if err == nil && status.SubmoduleWarning != "" {
		err = errors.New(status.SubmoduleWarning)
	}
	return len(status.Changes), err
}

func (r *Repository) workingStatus(ctx context.Context, head string, options ...string) (Status, error) {
	git, err := r.workingGit(ctx, head)
	if err != nil {
		return Status{}, err
	}
	defer git.close()
	if err := git.configureLocalLFS(false, false); err != nil {
		return Status{}, err
	}
	args := append([]string{"status", "--porcelain=v2", "-z", "--ignore-submodules=dirty"}, options...)
	output, err := git.read(ctx, 32<<20, args...)
	if err != nil {
		return Status{}, err
	}
	status, err := parseStatus(string(output), slices.Contains(options, "--branch"))
	if err != nil {
		return Status{}, err
	}
	if err := r.addSubmoduleFileChanges(ctx, &status); err != nil {
		if ctx.Err() != nil {
			return status, ctx.Err()
		}
		status.SubmoduleWarning = err.Error()
	}
	return status, nil
}

func parseStatus(output string, requireBranch bool) (Status, error) {
	var status Status
	for output != "" {
		record, rest, ok := strings.Cut(output, "\x00")
		if !ok || len(record) < 3 || record[1] != ' ' {
			return Status{}, fmt.Errorf("invalid Git porcelain v2 record")
		}
		output = rest
		if record[0] == '#' {
			if err := status.readHeader(record[2:]); err != nil {
				return Status{}, err
			}
			continue
		}
		change, err := parseChange(record)
		if err != nil {
			return Status{}, err
		}
		if change.Kind == '2' {
			change.PreviousPath, output, ok = strings.Cut(output, "\x00")
			if !ok || change.PreviousPath == "" {
				return Status{}, fmt.Errorf("missing original Git path in rename or copy")
			}
		}
		status.Changes = append(status.Changes, change)
	}
	if requireBranch && (status.HeadOID == "" || status.Branch == "") {
		return Status{}, fmt.Errorf("missing Git porcelain v2 branch headers")
	}
	return status, nil
}

func (s *Status) readHeader(header string) error {
	key, value, _ := strings.Cut(header, " ")
	switch key {
	case "branch.oid":
		s.HeadOID = value
	case "branch.head":
		s.Branch = value
	case "branch.upstream":
		s.Upstream = value
	case "branch.ab":
		ahead, behind, _ := strings.Cut(value, " ")
		if !strings.HasPrefix(ahead, "+") || !strings.HasPrefix(behind, "-") {
			return fmt.Errorf("invalid Git ahead/behind counters")
		}
		var err error
		s.Ahead, err = strconv.Atoi(ahead[1:])
		if err != nil || s.Ahead < 0 {
			return fmt.Errorf("invalid Git ahead counter")
		}
		s.Behind, err = strconv.Atoi(behind[1:])
		if err != nil || s.Behind < 0 {
			return fmt.Errorf("invalid Git behind counter")
		}
	}
	return nil
}

func parseChange(record string) (Change, error) {
	kind := record[0]
	var count int
	switch kind {
	case '1':
		count = 9
	case '2':
		count = 10
	case 'u':
		count = 11
	case '?':
		return Change{Kind: kind, Path: record[2:]}, nil
	default:
		return Change{}, fmt.Errorf("unsupported Git porcelain v2 change type")
	}
	fields := strings.SplitN(record, " ", count)
	if len(fields) != count || len(fields[1]) != 2 || len(fields[2]) != 4 || fields[count-1] == "" {
		return Change{}, fmt.Errorf("invalid Git porcelain v2 change")
	}
	change := Change{Kind: kind, Index: fields[1][0], WorkTree: fields[1][1],
		Submodule: fields[2], Path: fields[count-1]}
	if kind == '1' || kind == '2' {
		change.HeadOID = fields[6]
		change.IndexOID = fields[7]
		change.HeadMode = fields[3]
		change.IndexMode = fields[4]
		change.WorkTreeMode = fields[5]
	} else if kind == 'u' {
		change.WorkTreeMode = fields[6]
		for i := range change.Conflict {
			change.Conflict[i] = IndexStage{Mode: fields[3+i], OID: fields[7+i]}
		}
	}
	return change, nil
}
