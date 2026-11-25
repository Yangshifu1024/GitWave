package workspace

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	ErrRepoPathInUse    = errors.New("repository path is already in this workspace")
	ErrRepoOrderChanged = errors.New("repository tabs changed before the new order could be saved")
)

// RemoveRepo deletes the reference and updates a saved selection in one
// transaction. The local worktree and its Git directory are untouched.
func (s *Store) RemoveRepo(ctx context.Context, ws string, repo Repo) error {
	return s.transaction(ctx, nil, func(tx *sql.Tx) error {
		if err := requireChange(tx.ExecContext(ctx, "DELETE FROM repos WHERE workspace_id = ? AND id = ? AND path = ?",
			ws, repo.ID, repo.Path)); err != nil {
			return err
		}
		return repairLastRepo(ctx, tx, ws)
	})
}

func repairLastRepo(ctx context.Context, tx *sql.Tx, ws string) error {
	_, err := tx.ExecContext(ctx, `WITH first_active AS (
		SELECT id FROM repos WHERE workspace_id = ? AND status = 'active' ORDER BY position, added_at, id LIMIT 1
	)
	UPDATE workspaces SET last_active_repo_id = (SELECT id FROM first_active), updated_at = ?
	WHERE id = ? AND NOT EXISTS (
		SELECT 1 FROM repos WHERE id = workspaces.last_active_repo_id AND workspace_id = workspaces.id AND status = 'active'
	) AND last_active_repo_id IS NOT (SELECT id FROM first_active)`, ws, time.Now().Unix(), ws)
	return err
}

// RelinkRepo changes a missing tab's path, keeping its ID, position, nickname
// and settings. The caller validates the new root with Git before writing.
func (s *Store) RelinkRepo(ctx context.Context, ws string, repo Repo, path string) error {
	return s.transaction(ctx, nil, func(tx *sql.Tx) error {
		repos, err := listRepos(ctx, tx, ws)
		if err != nil {
			return err
		}
		key := repoPathKey(path)
		for _, existing := range repos {
			if existing.ID != repo.ID && repoPathKey(existing.Path) == key {
				return ErrRepoPathInUse
			}
		}
		return requireChange(tx.ExecContext(ctx, `UPDATE repos SET path = ?, status = 'active', missing_at = NULL
			WHERE workspace_id = ? AND id = ? AND path = ? AND status = 'missing'`, path, ws, repo.ID, repo.Path))
	})
}

// ReorderRepos verifies the order the user saw at gesture start and applies
// one complete permutation. A concurrent tab edit cannot be overwritten.
func (s *Store) ReorderRepos(ctx context.Context, ws string, before, after []string) error {
	return s.transaction(ctx, nil, func(tx *sql.Tx) error {
		current, err := listRepos(ctx, tx, ws)
		if err != nil {
			return err
		}
		if len(current) == 0 || len(current) != len(before) || len(before) != len(after) {
			return ErrRepoOrderChanged
		}
		ids := make(map[string]bool, len(current))
		for i, repo := range current {
			if repo.ID != before[i] {
				return ErrRepoOrderChanged
			}
			ids[repo.ID] = true
		}
		for _, id := range after {
			if !ids[id] {
				return ErrRepoOrderChanged
			}
			delete(ids, id)
		}
		for pos, id := range after {
			if _, err := tx.ExecContext(ctx, "UPDATE repos SET position = ? WHERE workspace_id = ? AND id = ?", pos, ws, id); err != nil {
				return err
			}
		}
		return nil
	})
}
