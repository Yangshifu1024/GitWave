package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type MergePlan struct {
	repo           *Repository
	source, target CheckoutTarget
	tree           string
	indexInfo      string
	Conflicts      []string
	DirtyFiles     int
	UpToDate       bool
	FastForward    bool
	Conflicted     bool
}

type MergeResult struct {
	OID         string
	Conflicted  bool
	FastForward bool
}

// Looking up the exact root reference avoids resolving a local branch with
// the same name as an operation's recovery reference.
func (r *Repository) exactRootRef(ctx context.Context, name string) (string, error) {
	symbolic, err := r.optionalRef(ctx, "symbolic-ref", "--no-recurse", "-q", name)
	if err != nil {
		return "", err
	}
	if symbolic != "" {
		return "", fmt.Errorf("expected a direct Git reference for %s", name)
	}
	out, err := r.readLimited(ctx, "", 4096, "show-ref", "--verify", "--hash", name)
	if err == nil {
		oid := strings.TrimSuffix(string(out), "\n")
		if !validOID(oid) {
			return "", fmt.Errorf("invalid Git reference %s", name)
		}
		return oid, nil
	}
	_, checkErr := r.readLimited(ctx, "", 4096, "show-ref", "--exists", name)
	var exit *exec.ExitError
	if errors.As(checkErr, &exit) && exit.ExitCode() == 2 {
		return "", nil
	}
	return "", err
}

func deleteMergeTree(oid string) string {
	return "option no-deref\x00delete AUTO_MERGE\x00" + oid + "\x00"
}

func (r *Repository) PrepareMerge(ctx context.Context, source, target CheckoutTarget) (*MergePlan, error) {
	plan := &MergePlan{repo: r, source: source, target: target}
	if err := plan.verify(ctx); err != nil {
		return nil, err
	}
	var err error
	plan.UpToDate, err = r.isAncestor(ctx, source.OID, target.OID)
	if err != nil || plan.UpToDate {
		return plan, err
	}
	status, err := r.Status(ctx)
	if err != nil {
		return nil, err
	}
	plan.DirtyFiles = len(status.Changes)
	plan.FastForward, err = r.isAncestor(ctx, target.OID, source.OID)
	if err != nil || plan.FastForward || plan.DirtyFiles != 0 {
		return plan, err
	}
	if err := plan.previewTree(ctx); err != nil {
		return nil, err
	}
	return plan, nil
}

func (p *MergePlan) verify(ctx context.Context) error {
	r := p.repo
	if r.WorkTree == "" || !validOID(p.source.OID) || !validOID(p.target.OID) || !strings.HasPrefix(p.source.Ref, "refs/heads/") ||
		(p.target.Ref != "" && !strings.HasPrefix(p.target.Ref, "refs/heads/")) {
		return fmt.Errorf("select a local branch and a checked-out commit to merge into")
	}
	if err := r.checkCheckoutOperation(ctx); err != nil {
		return err
	}
	if !r.headMatches(ctx, p.target.Ref, p.target.OID) {
		return fmt.Errorf("the target checkout changed since the merge was requested")
	}
	refs, err := r.References(ctx)
	if err != nil {
		return err
	}
	return checkRefTip(refs, p.source.Ref, p.source.OID)
}

func (p *MergePlan) previewTree(ctx context.Context) error {
	git, err := p.repo.workingGit(ctx, p.target.OID)
	if err != nil {
		return err
	}
	defer git.close()
	if err := copyGitMetadata(filepath.Join(p.repo.CommonDir, "shallow"), filepath.Join(git.dir, "shallow"), 16<<20); err != nil {
		return err
	}
	if _, err := git.read(ctx, 4096, "update-ref", p.source.Ref, p.source.OID); err != nil {
		return err
	}
	output, err := git.run(ctx, "HEAD "+p.source.Ref+"\n", 32<<20, "merge-tree", "--write-tree", "--stdin", "-z")
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			if diagnostic := strings.TrimSpace(string(exit.Stderr)); diagnostic != "" {
				return fmt.Errorf("could not preview the merge: %s", diagnostic)
			}
		}
		return err
	}
	return p.readPreview(output)
}

func (p *MergePlan) readPreview(output []byte) error {
	status, records, _ := strings.Cut(string(output), "\x00")
	tree, records, ok := strings.Cut(records, "\x00")
	if !ok || !validOID(tree) || (status != "0" && status != "1") {
		return fmt.Errorf("invalid Git merge preview")
	}
	p.tree = tree
	if status == "1" {
		return nil
	}
	p.Conflicted = true
	var index strings.Builder
	seen := make(map[string]bool)
	for record := range strings.SplitSeq(records, "\x00") {
		if record == "" {
			break
		}
		entry, path, found := strings.Cut(record, "\t")
		fields := strings.Fields(entry)
		if !found || !validIndexPath(path) || len(fields) != 3 || !validOID(fields[1]) ||
			(fields[2] != "1" && fields[2] != "2" && fields[2] != "3") {
			return fmt.Errorf("invalid Git merge preview conflict")
		}
		switch fields[0] {
		case "100644", "100755", "120000", "160000":
		default:
			return fmt.Errorf("invalid Git merge preview conflict mode")
		}
		if !seen[path] {
			seen[path] = true
			p.Conflicts = append(p.Conflicts, path)
			fmt.Fprintf(&index, "0 %s\t%s\x00", strings.Repeat("0", len(tree)), path)
		}
		index.WriteString(record)
		index.WriteByte(0)
	}
	p.indexInfo = index.String()
	return nil
}

func (p *MergePlan) Apply(ctx context.Context, noFastForward bool) (MergeResult, error) {
	if err := p.verify(ctx); err != nil {
		return MergeResult{}, err
	}
	if p.UpToDate {
		return MergeResult{OID: p.target.OID}, nil
	}
	if p.DirtyFiles != 0 {
		return MergeResult{}, fmt.Errorf("commit or stash changes before merging")
	}
	r := p.repo
	message := fmt.Sprintf("Merge branch \"%s\"", strings.TrimPrefix(p.source.Ref, "refs/heads/"))
	publication := &mergeCheckout{message: message, tree: p.tree, indexInfo: p.indexInfo, paths: p.Conflicts, paused: p.Conflicted}
	result := MergeResult{OID: p.source.OID, FastForward: p.FastForward && !noFastForward, Conflicted: p.Conflicted}
	if !result.FastForward {
		tree := p.tree
		if p.FastForward {
			header, err := r.readCommitHeader(ctx, p.source.OID)
			if err != nil {
				return MergeResult{}, err
			}
			tree = header.tree
		}
		identity, err := r.commitIdentity(ctx)
		if err != nil {
			return MergeResult{}, err
		}
		// For conflicts this commit only carries the conflict-marker tree
		// through checkout. No branch or reflog ever points to it.
		result.OID, err = r.createCommitObject(ctx, tree, []string{p.target.OID, p.source.OID}, identity, identity, message)
		if err != nil {
			return MergeResult{}, err
		}
	}
	checkout, err := r.prepareCheckoutPlan(ctx, p.source, result.OID, publication, nil, false)
	if err != nil {
		return MergeResult{}, err
	}
	if checkout.source.head != p.target.OID || checkout.source.attached != p.target.Ref {
		return MergeResult{}, fmt.Errorf("the target checkout changed before merging")
	}
	if _, err := checkout.Apply(ctx); err != nil {
		return MergeResult{}, err
	}
	if result.Conflicted {
		result.OID = p.target.OID
	}
	return result, nil
}
