package gitcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

type CommitState struct {
	MergeHeads        string
	Continuation      string
	InteractiveRebase string
}

func (s CommitState) CanAmendInteractiveRebase() bool {
	return s.Continuation == interactiveRebaseOperation && validOID(s.InteractiveRebase)
}

func (r *Repository) readCommitState(ctx context.Context, root *os.Root) (CommitState, error) {
	heads, err := readOperationFile(root, "MERGE_HEAD")
	if err != nil {
		return CommitState{}, err
	}
	state := CommitState{MergeHeads: strings.TrimSpace(heads)}
	for _, item := range [...]struct{ path, operation string }{
		{"rebase-merge", "rebase"}, {"rebase-apply", "rebase"},
		{"CHERRY_PICK_HEAD", "cherry-pick"}, {"REVERT_HEAD", "revert"},
		{"sequencer", "sequencer"}, {"MERGE_AUTOSTASH", "merge with autostash"},
	} {
		if _, err := root.Lstat(item.path); err == nil {
			state.Continuation = item.operation
			break
		} else if !errors.Is(err, fs.ErrNotExist) {
			return CommitState{}, err
		}
	}
	if state.Continuation == "" && state.MergeHeads == "" {
		state.InteractiveRebase, err = r.interactiveRebaseRef(ctx)
		if err != nil {
			return CommitState{}, err
		}
		if state.InteractiveRebase != "" {
			state.Continuation = interactiveRebaseOperation
		} else if _, err := root.Lstat(legacyInteractiveRebaseFile); err == nil {
			raw, err := readOperationFile(root, legacyInteractiveRebaseFile)
			if err != nil {
				return CommitState{}, err
			}
			state.Continuation = interactiveRebaseOperation
			state.InteractiveRebase = legacyPauseVersion(raw)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return CommitState{}, err
		}
	}
	return state, nil
}

func readOperationFile(root *os.Root, name string) (string, error) {
	file, err := root.OpenFile(name, os.O_RDONLY|workFileOpenFlags, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	const limit = 5 << 20
	if !info.Mode().IsRegular() || info.Size() > limit {
		return "", fmt.Errorf("invalid Git operation file %s", name)
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if len(data) > limit {
		return "", fmt.Errorf("Git operation file %s exceeds the read limit", name)
	}
	if strings.TrimSpace(string(data)) == "" && err == nil {
		return "", fmt.Errorf("Git operation file %s is empty", name)
	}
	return string(data), err
}

func (r *Repository) commitState(ctx context.Context) (CommitState, error) {
	root, err := os.OpenRoot(r.GitDir)
	if err != nil {
		return CommitState{}, err
	}
	defer root.Close()
	return r.readCommitState(ctx, root)
}
