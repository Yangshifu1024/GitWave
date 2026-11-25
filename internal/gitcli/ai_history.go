package gitcli

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var errAIRefsChanged = errors.New("the branch or base changed while preparing the PR description")

type CommitSubject struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
}

// RecentCommitSubjects bounds the metadata used to interpret palette requests.
func (r *Repository) RecentCommitSubjects(ctx context.Context, oid string) ([]CommitSubject, error) {
	if !validOID(oid) {
		return nil, errors.New("invalid Git commit ID")
	}
	raw, err := r.readLimited(ctx, "", 512<<10, "log", "-15", "--no-notes", "--no-show-signature", "--no-patch",
		"--no-decorate", "--encoding=UTF-8", "-z", "--format=%H%x00%s", oid, "--")
	if err != nil {
		return nil, err
	}
	fields := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
	if len(fields)%2 != 0 {
		return nil, errors.New("Git returned malformed commit subjects")
	}
	commits := make([]CommitSubject, 0, len(fields)/2)
	for i := 0; i < len(fields); i += 2 {
		if !validOID(fields[i]) {
			return nil, errors.New("Git returned an invalid commit ID")
		}
		subject := []rune(fields[i+1])
		commits = append(commits, CommitSubject{SHA: fields[i], Subject: string(subject[:min(len(subject), 512)])})
	}
	return commits, nil
}

// AIExplainCommitContext uses the selected commit and its first parent.
// A shallow boundary with a missing parent returns an error instead of
// treating the entire checked-in tree as newly added content.
func (r *Repository) AIExplainCommitContext(ctx context.Context, oid string) (string, error) {
	if !validOID(oid) {
		return "", errors.New("invalid Git commit ID")
	}
	parent, err := r.firstParent(ctx, Commit{SHA: oid})
	if err != nil {
		return "", err
	}
	message, err := r.readLimited(ctx, "", 24_000, "log", "-1", "--no-notes", "--no-show-signature", "--no-patch",
		"--no-decorate", "--encoding=UTF-8", "--format=%B", oid, "--")
	if errors.Is(err, errGitOutputLimit) {
		message = []byte("[commit message exceeds the preview limit]")
	} else if err != nil {
		return "", err
	}
	diff, err := r.aiPinnedDiff(ctx, parent, oid, "Diff")
	if err != nil {
		return "", err
	}
	return "Commit message:\n" + strings.TrimSpace(string(message)) + "\n\nChanged files:\n" + diff, nil
}

type AIPRContext struct {
	Context string
	Head    string
	HeadRef string
	Base    string
	baseRef string
	baseOID string
}

// AIPullRequestContext compares only committed objects between the tip and
// the merge base. The order of default references is the same as GitWave's
// Tauri workflow.
func (r *Repository) AIPullRequestContext(ctx context.Context, head, ref string) (AIPRContext, error) {
	if head == "" {
		return AIPRContext{}, errors.New("the repository has no commits yet")
	}
	if err := r.checkAIPRHead(ctx, head, ref); err != nil {
		return AIPRContext{}, err
	}
	baseRef, baseOID, err := r.aiDefaultPRBase(ctx)
	if err != nil {
		return AIPRContext{}, err
	}
	result := AIPRContext{Head: head, HeadRef: ref, baseRef: baseRef, baseOID: baseOID,
		Base: strings.TrimPrefix(strings.TrimPrefix(baseRef, "refs/remotes/"), "refs/heads/")}
	raw, err := r.readLimited(ctx, "", 256, "merge-base", result.baseOID, head)
	if gitExitCode(err, 1) {
		return AIPRContext{}, fmt.Errorf("no common ancestor with %s", result.Base)
	}
	if err != nil {
		return AIPRContext{}, err
	}
	mergeBase := strings.TrimSpace(string(raw))
	if !validOID(mergeBase) {
		return AIPRContext{}, errors.New("Git returned an invalid merge base")
	}
	if mergeBase == head {
		return AIPRContext{}, fmt.Errorf("no commits ahead of %s", result.Base)
	}
	branch := strings.TrimPrefix(ref, "refs/heads/")
	if branch == "" {
		branch = "HEAD (detached)"
	}
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "Branch: %s\nBase: %s\n\nCommits:\n", branch, result.Base)
	revisions := mergeBase + ".." + head
	args := []string{"log", "--no-notes", "--no-show-signature", "--no-patch", "--no-decorate", "--encoding=UTF-8"}
	if subjects, err := r.readLimited(ctx, "", 12_000, append(args, "-30", "--format=- %h %s", revisions, "--")...); err == nil {
		prompt.Write(subjects)
	}
	if full, err := r.readLimited(ctx, "", 12_000, append(args, "-5", "--format=---%n%B%n---", revisions, "--")...); err == nil {
		prompt.WriteString("\nFull commit messages of the branch (style reference):\n")
		prompt.Write(full)
	}
	diff, err := r.aiPinnedDiff(ctx, mergeBase, head, "Combined diff of the branch")
	if err != nil {
		return AIPRContext{}, err
	}
	prompt.WriteString("\nChanged files:\n")
	prompt.WriteString(diff)
	result.Context = prompt.String()
	return result, r.CheckAIPullRequestContext(ctx, result)
}

func (r *Repository) CheckAIPullRequestContext(ctx context.Context, result AIPRContext) error {
	if err := r.checkAIPRHead(ctx, result.Head, result.HeadRef); err != nil {
		return err
	}
	ref, oid, err := r.aiDefaultPRBase(ctx)
	if err != nil {
		return err
	}
	if ref != result.baseRef || oid != result.baseOID {
		return errAIRefsChanged
	}
	return nil
}

func (r *Repository) checkAIPRHead(ctx context.Context, head, ref string) error {
	err := r.checkAIHead(ctx, head, ref)
	if errors.Is(err, ErrAIStagedChanged) {
		return errAIRefsChanged
	}
	return err
}

func (r *Repository) aiDefaultPRBase(ctx context.Context) (string, string, error) {
	for _, candidate := range []string{"refs/remotes/origin/main", "refs/remotes/origin/master", "refs/heads/main", "refs/heads/master"} {
		base, err := r.optionalRef(ctx, "rev-parse", "--verify", "--quiet", candidate+"^{commit}")
		if err != nil {
			return "", "", err
		}
		if base != "" {
			return candidate, base, nil
		}
	}
	return "", "", errors.New("no base branch found among origin/main, origin/master, main and master")
}

func (r *Repository) aiPinnedDiff(ctx context.Context, parent, head, heading string) (string, error) {
	args := treeDiffArgs()
	revisions := []string{head}
	if parent != "" {
		revisions = []string{parent, head}
	}
	raw, err := r.readLimited(ctx, "", 4<<20, diffArguments(args, append(append([]string(nil), revisions...), "--"), "--raw", "-z", "--no-abbrev")...)
	if err != nil {
		return "", err
	}
	deltas, err := parseTreeDeltas(string(raw))
	if err != nil {
		return "", err
	}
	return writeAISafePatch(ctx, func(ctx context.Context, max int64, args ...string) ([]byte, error) {
		return r.readLimited(ctx, "", max, args...)
	}, deltas, args, revisions, heading)
}
