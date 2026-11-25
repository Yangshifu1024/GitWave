package gitcli

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Replay explicit commits in a private index and worktree. Publishing an
// edit pause or a completed sequence is a separate checkout transaction.
func (r *Repository) replayInteractive(ctx context.Context, base string, todos []RebaseTodo) (InteractiveRebaseResult, []RebaseTodo, error) {
	result := InteractiveRebaseResult{OID: base}
	if len(todos) == 0 {
		return result, nil, nil
	}
	git, err := r.privateReplay(ctx, base)
	if err != nil {
		return InteractiveRebaseResult{}, nil, err
	}
	defer git.close()
	if err := git.runCheckoutCommand(ctx, "", "read-tree", "--reset", "-u", base); err != nil {
		return InteractiveRebaseResult{}, nil, err
	}
	identity, err := r.commitIdentity(ctx)
	if err != nil {
		return InteractiveRebaseResult{}, nil, err
	}
	for i, todo := range todos {
		header, err := r.readCommitHeader(ctx, todo.OID)
		if err != nil {
			return InteractiveRebaseResult{}, nil, err
		}
		if len(header.parents) > 1 {
			return InteractiveRebaseResult{}, nil, fmt.Errorf("cannot replay merge commit %s without a mainline parent", todo.OID[:12])
		}
		paths, err := git.applyCommitChange(ctx, CherryPickCommit, todo.OID, len(header.parents) != 0)
		if err != nil {
			return InteractiveRebaseResult{}, nil, err
		}
		if len(paths) != 0 {
			return InteractiveRebaseResult{Conflicts: paths}, nil, nil
		}
		tree, err := git.writeIndexTree(ctx)
		if err != nil {
			return InteractiveRebaseResult{}, nil, err
		}
		author, parents, message := header.author, []string{result.OID}, header.message
		switch todo.Action {
		case RebaseReword:
			if strings.TrimSpace(todo.Message) != "" {
				message = todo.Message
			}
		case RebaseSquash, RebaseFixup:
			previous, err := r.readCommitHeader(ctx, result.OID)
			if err != nil {
				return InteractiveRebaseResult{}, nil, err
			}
			author, parents = previous.author, previous.parents
			message = previous.message
			if todo.Action == RebaseSquash {
				message = todo.Message
				if strings.TrimSpace(message) == "" {
					message = strings.TrimRight(previous.message, "\n") + "\n\n" + strings.TrimRight(header.message, "\n")
				}
			}
		}
		result.OID, err = r.createCommitObject(ctx, tree, parents, author, identity, message)
		if err != nil {
			return InteractiveRebaseResult{}, nil, err
		}
		if _, err := git.read(ctx, 4096, "update-ref", "--no-deref", "HEAD", result.OID); err != nil {
			return InteractiveRebaseResult{}, nil, err
		}
		if todo.Action == RebaseEdit {
			result.Paused = true
			return result, todos[i+1:], nil
		}
	}
	return result, nil, nil
}

func (g *workingGit) writeIndexTree(ctx context.Context) (string, error) {
	output, err := g.read(ctx, 4096, "write-tree")
	if err != nil {
		return "", err
	}
	oid := strings.TrimSpace(string(output))
	if !validOID(oid) {
		return "", fmt.Errorf("invalid tree produced from the index")
	}
	return oid, nil
}

func (g *workingGit) applyCommitChange(ctx context.Context, action CommitReplayAction, oid string, hasParent bool) ([]string, error) {
	if hasParent && g.shallowBoundaries[oid] {
		return nil, fmt.Errorf("%s needs the parent of %s beyond the shallow history boundary", action, oid[:12])
	}
	err := g.runCheckoutCommand(ctx, "", string(action), "--no-commit", "--no-gpg-sign", oid)
	if err == nil {
		return nil, nil
	}
	paths, listErr := g.replayConflicts(ctx)
	if len(paths) != 0 && listErr == nil {
		return paths, nil
	}
	return nil, errors.Join(err, listErr)
}
