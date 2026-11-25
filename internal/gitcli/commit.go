package gitcli

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
)

type CommitResult struct {
	SHA     string
	Warning string
}

// Commit creates a commit only after an explicit user request, using the
// staged selection shown in Changes. An amend names the confirmed HEAD.
func (r *Repository) Commit(ctx context.Context, shown Status, message, amend string) (CommitResult, error) {
	message = strings.TrimSpace(message)
	if message == "" || strings.ContainsRune(message, 0) {
		return CommitResult{}, fmt.Errorf("enter a valid commit message")
	}
	lock, err := r.lockCheckoutIndex(ctx)
	if err != nil {
		return CommitResult{}, err
	}
	defer lock.close()
	current, err := r.Status(ctx)
	if err != nil {
		return CommitResult{}, err
	}
	if current.HeadOID != shown.HeadOID || current.HeadRef != shown.HeadRef || (amend != "" && current.Upstream != shown.Upstream) ||
		current.Commit != shown.Commit || !slices.Equal(stagedEntries(current), stagedEntries(shown)) {
		return CommitResult{}, fmt.Errorf("HEAD, staged changes or repository operation changed before committing")
	}
	if current.Commit.Continuation != "" && (amend == "" || !current.Commit.CanAmendInteractiveRebase()) {
		return CommitResult{}, fmt.Errorf("finish the %s with its Continue action first", current.Commit.Continuation)
	}
	for _, change := range current.Changes {
		if change.Kind == 'u' {
			return CommitResult{}, fmt.Errorf("resolve all index conflicts before committing")
		}
	}
	head := current.HeadOID
	if head == "(initial)" {
		head = ""
	}
	if current.HeadRef != "" && !strings.HasPrefix(current.HeadRef, "refs/heads/") {
		return CommitResult{}, fmt.Errorf("checkout a branch or commit before committing from this reference")
	}
	if amend != "" && (amend != head || current.HeadRef == "" || current.Commit.MergeHeads != "") {
		return CommitResult{}, fmt.Errorf("amend requires the confirmed branch HEAD with no merge in progress")
	}
	git, err := r.workingGit(ctx, head)
	if err != nil {
		return CommitResult{}, err
	}
	defer git.close()
	git.index = filepath.Join(git.dir, "index")
	if err := copyOriginalIndex(lock.root, git.index); err != nil {
		return CommitResult{}, err
	}
	if err := git.isolateWorkTree(); err != nil {
		return CommitResult{}, err
	}
	treeOutput, err := git.read(ctx, 4096, "write-tree")
	if err != nil {
		return CommitResult{}, err
	}
	tree := strings.TrimSpace(string(treeOutput))
	if !validOID(tree) {
		return CommitResult{}, fmt.Errorf("invalid Git tree for commit")
	}
	author, err := r.commitIdentity(ctx)
	if err != nil {
		return CommitResult{}, err
	}
	committer := author
	var parents []string
	if head != "" {
		previous, err := r.readCommitHeader(ctx, head)
		if err != nil {
			return CommitResult{}, err
		}
		if amend != "" {
			author, parents = previous.author, previous.parents
		} else {
			if current.Commit.MergeHeads == "" && tree == previous.tree {
				return CommitResult{}, fmt.Errorf("nothing to commit")
			}
			parents = append(parents, head)
		}
	} else if len(stagedEntries(current)) == 0 {
		return CommitResult{}, fmt.Errorf("nothing to commit")
	}
	for _, mergeHead := range strings.Fields(current.Commit.MergeHeads) {
		if len(parents) == 0 || !validOID(mergeHead) || slices.Contains(parents, mergeHead) {
			return CommitResult{}, fmt.Errorf("invalid parent in MERGE_HEAD")
		}
		parents = append(parents, mergeHead)
	}
	oid, err := r.createCommitObject(ctx, tree, parents, author, committer, message)
	if err != nil {
		return CommitResult{}, err
	}
	var interactive *interactiveAdvance
	if current.Commit.CanAmendInteractiveRebase() {
		expected := current.Commit.InteractiveRebase
		pause, err := r.loadInteractivePause(ctx, expected)
		if err != nil {
			return CommitResult{}, err
		}
		if pause.Head != head || pause.Branch != current.HeadRef {
			return CommitResult{}, fmt.Errorf("the paused interactive rebase belongs to a different checkout")
		}
		pause.Head = oid
		next, err := r.saveInteractivePause(ctx, pause)
		if err != nil {
			return CommitResult{}, err
		}
		interactive = &interactiveAdvance{before: expected, after: next}
	}
	subject, _, _ := strings.Cut(message, "\n")
	label := "commit: "
	if amend != "" {
		label = "commit (amend): "
	} else if current.Commit.MergeHeads != "" {
		label = "commit (merge): "
	} else if head == "" {
		label = "commit (initial): "
	}
	err = r.publishCommit(ctx, lock, git.dir, current, head, oid, label+strings.TrimRight(subject, "\r"), committer, interactive)
	if err != nil {
		return CommitResult{}, err
	}
	result := CommitResult{SHA: oid}
	if current.Commit.MergeHeads != "" {
		for _, name := range []string{"MERGE_HEAD", "MERGE_MODE", "MERGE_MSG"} {
			if err := lock.root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
				result.Warning = "The commit was created. Git could not clean up all merge state."
			}
		}
	}
	return result, nil
}

type stagedEntry struct{ path, previous, oid, mode string }

func stagedEntries(status Status) []stagedEntry {
	var entries []stagedEntry
	for _, change := range status.Changes {
		if change.Kind == 'u' || change.Index == '.' || change.Index == 0 {
			continue
		}
		entry := stagedEntry{path: change.Path, oid: change.IndexOID, mode: change.IndexMode}
		if change.Index == 'R' || change.Index == 'C' {
			entry.previous = change.PreviousPath
		}
		entries = append(entries, entry)
	}
	slices.SortFunc(entries, func(a, b stagedEntry) int { return strings.Compare(a.path, b.path) })
	return entries
}

type commitIdentity struct{ name, email, date string }

func (r *Repository) commitIdentity(ctx context.Context) (commitIdentity, error) {
	data, err := r.optionalRef(ctx, "config", "--null", "--get-regexp", `^user\.(name|email)$`)
	if err != nil {
		return commitIdentity{}, err
	}
	var identity commitIdentity
	for item := range strings.SplitSeq(data, "\x00") {
		key, value, _ := strings.Cut(item, "\n")
		switch key {
		case "user.name":
			identity.name = strings.TrimSpace(value)
		case "user.email":
			identity.email = strings.TrimSpace(value)
		}
	}
	if identity.name == "" || identity.email == "" {
		return commitIdentity{name: "GitWave", email: "noreply@gitwave.local"}, nil
	}
	return identity, nil
}

type commitHeader struct {
	tree    string
	author  commitIdentity
	parents []string
	message string
}

func (r *Repository) readCommitHeader(ctx context.Context, oid string) (commitHeader, error) {
	data, err := r.readLimited(ctx, "", 32<<20, "cat-file", "commit", oid)
	if err != nil {
		return commitHeader{}, err
	}
	lines, message, _ := strings.Cut(string(data), "\n\n")
	parsed := commitHeader{message: strings.TrimSuffix(message, "\n")}
	for line := range strings.SplitSeq(lines, "\n") {
		key, value, _ := strings.Cut(line, " ")
		switch key {
		case "tree":
			parsed.tree = value
		case "parent":
			if !validOID(value) {
				return commitHeader{}, fmt.Errorf("invalid commit parent")
			}
			parsed.parents = append(parsed.parents, value)
		case "author":
			end := strings.LastIndex(value, "> ")
			start := strings.LastIndex(value, " <")
			if start <= 0 || end <= start+2 {
				return commitHeader{}, fmt.Errorf("invalid original author signature")
			}
			parsed.author = commitIdentity{value[:start], value[start+2 : end], value[end+2:]}
		}
	}
	if !validOID(parsed.tree) || parsed.author.name == "" {
		return commitHeader{}, fmt.Errorf("invalid commit header")
	}
	return parsed, nil
}

func (r *Repository) createCommitObject(ctx context.Context, tree string, parents []string, author, committer commitIdentity, message string) (string, error) {
	args := []string{"-c", "i18n.commitencoding=UTF-8", "commit-tree", tree, "--no-gpg-sign", "-F", "-"}
	for _, parent := range parents {
		args = append(args, "-p", parent)
	}
	cmd := r.command(ctx, message+"\n", args...)
	cmd.Env = append(cmd.Env, "GIT_AUTHOR_NAME="+author.name, "GIT_AUTHOR_EMAIL="+author.email,
		"GIT_COMMITTER_NAME="+committer.name, "GIT_COMMITTER_EMAIL="+committer.email)
	if author.date != "" {
		cmd.Env = append(cmd.Env, "GIT_AUTHOR_DATE="+author.date)
	}
	data, err := readGitOutput(ctx, cmd, 4096, "commit-tree")
	if err != nil {
		return "", err
	}
	oid := strings.TrimSpace(string(data))
	if !validOID(oid) {
		return "", fmt.Errorf("Git returned an invalid commit ID")
	}
	return oid, nil
}
