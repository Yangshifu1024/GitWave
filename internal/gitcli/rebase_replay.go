package gitcli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitwave.work/internal/process"
)

type PullRebaseConflict struct{ Paths []string }

func (e *PullRebaseConflict) Error() string {
	return fmt.Sprintf("pull --rebase hit conflicts; local commits were left untouched: %s", strings.Join(e.Paths, ", "))
}

func (r *Repository) rebasePull(ctx context.Context, local, upstream string) (string, error) {
	oid, conflicts, err := r.replayRebase(ctx, local, upstream)
	if len(conflicts) != 0 {
		return "", &PullRebaseConflict{Paths: conflicts}
	}
	return oid, err
}

// Perform the replay away from the live index, worktree and refs. Callers
// publish the resulting tip only after a complete replay.
func (r *Repository) replayRebase(ctx context.Context, local, upstream string) (string, []string, error) {
	base, err := r.optionalRef(ctx, "merge-base", local, upstream)
	if err != nil {
		return "", nil, err
	}
	if !validOID(base) {
		return "", nil, fmt.Errorf("the branches have no known common ancestor")
	}
	git, err := r.privateReplay(ctx, local)
	if err != nil {
		return "", nil, err
	}
	defer git.close()
	if err := git.checkLinearReplay(ctx, local, upstream); err != nil {
		return "", nil, err
	}
	if err := git.runCheckoutCommand(ctx, "", "read-tree", "--reset", "-u", local); err != nil {
		return "", nil, err
	}
	identity, err := r.commitIdentity(ctx)
	if err != nil {
		return "", nil, err
	}
	cmd := git.command(ctx, "", "rebase", "--no-update-refs", "--no-autostash", "--no-rebase-merges",
		"--no-autosquash", "--no-fork-point", "--no-gpg-sign", "--reapply-cherry-picks", "--empty=drop", upstream)
	cmd.Env = append(cmd.Env, "GIT_EDITOR=true", "GIT_SEQUENCE_EDITOR=true")
	withRefIdentity(cmd, identity)
	var stderr gitDiagnostic
	cmd.Stderr = &stderr
	if err := process.Run(cmd); err != nil {
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}
		paths, listErr := git.replayConflicts(ctx)
		if listErr != nil {
			return "", nil, errors.Join(err, listErr)
		}
		if len(paths) != 0 {
			return "", paths, nil
		}
		if diagnostic := strings.TrimSpace(stderr.buffer.String()); diagnostic != "" {
			return "", nil, fmt.Errorf("could not rebase in the private checkout: %s", diagnostic)
		}
		return "", nil, fmt.Errorf("could not rebase in the private checkout: %w", err)
	}
	if ctx.Err() != nil {
		return "", nil, ctx.Err()
	}
	if _, err := os.Stat(filepath.Join(git.dir, "rebase-merge")); !errors.Is(err, os.ErrNotExist) {
		return "", nil, fmt.Errorf("rebase stopped before finishing in the private checkout")
	}
	result, err := git.read(ctx, 1024, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", nil, err
	}
	oid := strings.TrimSuffix(string(result), "\n")
	if !validOID(oid) {
		return "", nil, fmt.Errorf("rebase did not produce a commit")
	}
	return oid, nil, nil
}

func (r *Repository) privateReplay(ctx context.Context, head string) (*workingGit, error) {
	git, err := r.workingGit(ctx, head)
	if err != nil {
		return nil, err
	}
	if err := git.isolateWorkTree(); err != nil {
		git.close()
		return nil, err
	}
	git.index = filepath.Join(git.dir, "index")
	git.settings = append(git.settings, "-c", "core.hooksPath="+git.dir, "-c", "rerere.enabled=false")
	shallow := filepath.Join(git.dir, "shallow")
	if err := copyGitMetadata(filepath.Join(r.CommonDir, "shallow"), shallow, 16<<20); err != nil {
		git.close()
		return nil, err
	}
	boundaries, err := os.ReadFile(shallow)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		git.close()
		return nil, err
	}
	if len(boundaries) != 0 {
		git.shallowBoundaries = make(map[string]bool)
		for _, oid := range strings.Fields(string(boundaries)) {
			git.shallowBoundaries[oid] = true
		}
	}
	return git, nil
}

func (g *workingGit) replayConflicts(ctx context.Context) ([]string, error) {
	conflicts, err := g.read(ctx, 4<<20, "ls-files", "--unmerged", "-z")
	if err != nil {
		return nil, err
	}
	var paths []string
	seen := make(map[string]bool)
	for record := range strings.SplitSeq(string(conflicts), "\x00") {
		_, path, ok := strings.Cut(record, "\t")
		if ok && !seen[path] {
			seen[path] = true
			paths = append(paths, path)
		}
	}
	return paths, nil
}

// Linear rebase skips merge commits. Verify that flattening will not
// silently discard changes made in a merge commit itself.
func (g *workingGit) checkLinearReplay(ctx context.Context, local, upstream string) error {
	output, err := g.read(ctx, 8<<20, "log", "--merges", "--format=%H %T %P", upstream+".."+local, "--")
	if err != nil {
		return err
	}
	for line := range strings.SplitSeq(strings.TrimSuffix(string(output), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 || !validOID(fields[0]) || !validOID(fields[1]) {
			return fmt.Errorf("invalid commit in local merge history")
		}
		commit, tree := fields[0], fields[1]
		if len(fields) != 4 {
			return fmt.Errorf("linear rebase cannot preserve merge %s with more than two parents", commit[:12])
		}
		if !validOID(fields[2]) || !validOID(fields[3]) {
			return fmt.Errorf("invalid parent in merge commit %s", commit[:12])
		}
		merged, err := g.run(ctx, fields[2]+" "+fields[3]+"\n", 32<<20, "merge-tree", "--write-tree", "--stdin", "-z")
		if err != nil {
			return fmt.Errorf("could not check changes in merge commit %s: %w", commit[:12], err)
		}
		var preview MergePlan
		if err := preview.readPreview(merged); err != nil {
			return err
		}
		if preview.Conflicted || preview.tree != tree {
			return fmt.Errorf("linear rebase would discard changes made in merge commit %s", commit[:12])
		}
	}
	return nil
}
