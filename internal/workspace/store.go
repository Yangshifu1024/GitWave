package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
)

const selectionKey = "native.last-active-workspace"

type Summary struct{ ID, Name, LastRepoID string }

type Repo struct {
	ID, Path, Nickname string
	Missing            bool
}

type State struct {
	Workspaces []Summary
	Workspace  Summary
	Repos      []Repo
	Repo       Repo
}

func (s *Store) Load(ctx context.Context) (State, error) {
	return s.load(ctx, nil)
}

// LoadWorkspace reads a window's current workspace independently from the
// last workspace persisted for application startup.
func (s *Store) LoadWorkspace(ctx context.Context, id string) (State, error) {
	return s.load(ctx, &id)
}

func (s *Store) load(ctx context.Context, workspaceID *string) (State, error) {
	var state State
	err := s.transaction(ctx, &sql.TxOptions{ReadOnly: true}, func(tx *sql.Tx) error {
		var active string
		if workspaceID != nil {
			active = *workspaceID
		} else {
			var value string
			err := tx.QueryRowContext(ctx, "SELECT value_json FROM app_settings WHERE key = ?", selectionKey).Scan(&value)
			switch {
			case errors.Is(err, sql.ErrNoRows):
			case err != nil:
				return err
			default:
				if err := json.Unmarshal([]byte(value), &active); err != nil {
					return fmt.Errorf("read last active workspace: %w", err)
				}
			}
		}
		workspaces, err := listWorkspaces(ctx, tx)
		if err != nil {
			return err
		}
		state.Workspaces = workspaces
		for _, ws := range workspaces {
			if ws.ID == active {
				state.Workspace = ws
				break
			}
		}
		if state.Workspace.ID == "" {
			return nil
		}
		state.Repos, err = listRepos(ctx, tx, active)
		if err != nil {
			return err
		}
		for _, repo := range state.Repos {
			if repo.Missing {
				continue
			}
			if repo.ID == state.Workspace.LastRepoID {
				state.Repo = repo
				break
			}
			if state.Repo.ID == "" {
				state.Repo = repo
			}
		}
		return nil
	})
	if err != nil {
		return State{}, err
	}
	return state, nil
}

func listWorkspaces(ctx context.Context, tx *sql.Tx) ([]Summary, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, name, COALESCE(last_active_repo_id, '')
		FROM workspaces ORDER BY updated_at DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var workspaces []Summary
	for rows.Next() {
		var ws Summary
		if err := rows.Scan(&ws.ID, &ws.Name, &ws.LastRepoID); err != nil {
			return nil, err
		}
		workspaces = append(workspaces, ws)
	}
	return workspaces, rows.Err()
}

func listRepos(ctx context.Context, tx *sql.Tx, workspace string) ([]Repo, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, path, COALESCE(nickname, ''), status = 'missing'
		FROM repos WHERE workspace_id = ? ORDER BY position, added_at, id`, workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var repos []Repo
	for rows.Next() {
		var repo Repo
		if err := rows.Scan(&repo.ID, &repo.Path, &repo.Nickname, &repo.Missing); err != nil {
			return nil, err
		}
		repos = append(repos, repo)
	}
	return repos, rows.Err()
}

func (s *Store) Create(ctx context.Context, name string) error {
	_, err := s.CreateWithRepos(ctx, name, nil)
	return err
}

// CreateWithRepos creates and selects a workspace and its validated repos
// together. The first repo in the imported order is selected.
func (s *Store) CreateWithRepos(ctx context.Context, name string, paths []string) (Summary, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Summary{}, fmt.Errorf("workspace name is empty")
	}
	id, err := uuid.NewRandom()
	if err != nil {
		return Summary{}, err
	}
	ws := Summary{ID: id.String(), Name: name}
	err = s.transaction(ctx, nil, func(tx *sql.Tx) error {
		now := time.Now().Unix()
		// prompt_templates is required by older GitWave versions.
		if _, err := tx.ExecContext(ctx, `INSERT INTO workspaces (id, name, settings_json, created_at, updated_at)
			VALUES (?, ?, '{"prompt_templates":{}}', ?, ?)`, ws.ID, ws.Name, now, now); err != nil {
			return err
		}
		for pos, path := range paths {
			repoID, err := insertRepo(ctx, tx, ws.ID, path, now, int64(pos))
			if err != nil {
				return err
			}
			if pos == 0 {
				ws.LastRepoID = repoID
			}
		}
		if ws.LastRepoID != "" {
			if err := setLastRepo(ctx, tx, ws.ID, ws.LastRepoID, now); err != nil {
				return err
			}
		}
		return rememberWorkspace(ctx, tx, ws.ID)
	})
	if err != nil {
		return Summary{}, err
	}
	return ws, nil
}

func (s *Store) Rename(ctx context.Context, id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("workspace name is empty")
	}
	return requireChange(s.db.ExecContext(ctx, "UPDATE workspaces SET name = ?, updated_at = ? WHERE id = ?",
		name, time.Now().Unix(), id))
}

func (s *Store) Delete(ctx context.Context, id string) error {
	return s.transaction(ctx, nil, func(tx *sql.Tx) error {
		if err := requireChange(tx.ExecContext(ctx, "DELETE FROM workspaces WHERE id = ?", id)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM app_settings WHERE key = ? AND value_json = json_quote(?)", selectionKey, id)
		return err
	})
}

func (s *Store) SelectWorkspace(ctx context.Context, id string) error {
	return s.transaction(ctx, nil, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT id FROM workspaces WHERE id = ?", id).Scan(new(string)); err != nil {
			return err
		}
		return rememberWorkspace(ctx, tx, id)
	})
}

func (s *Store) SelectRepo(ctx context.Context, ws, repo string) error {
	return s.transaction(ctx, nil, func(tx *sql.Tx) error {
		if err := requireChange(tx.ExecContext(ctx, `UPDATE workspaces SET last_active_repo_id = ?, updated_at = ?
			WHERE id = ? AND EXISTS (SELECT 1 FROM repos WHERE id = ? AND workspace_id = ? AND status = 'active')`,
			repo, time.Now().Unix(), ws, repo, ws)); err != nil {
			return err
		}
		return rememberWorkspace(ctx, tx, ws)
	})
}

// AddRepos accepts paths already validated against Git. The last newly
// inserted repo becomes active in the same transaction as the tab list.
func (s *Store) AddRepos(ctx context.Context, ws string, paths []string) (int, string, error) {
	return s.addRepos(ctx, ws, paths, false)
}

// SelectOrAddRepo accepts a validated checkout and selects its existing tab
// or registers it as the active tab in the same transaction.
func (s *Store) SelectOrAddRepo(ctx context.Context, ws, path string) (string, error) {
	_, id, err := s.addRepos(ctx, ws, []string{path}, true)
	return id, err
}

func (s *Store) addRepos(ctx context.Context, ws string, paths []string, selectExisting bool) (int, string, error) {
	var added int
	var last string
	err := s.transaction(ctx, nil, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT id FROM workspaces WHERE id = ?", ws).Scan(new(string)); err != nil {
			return err
		}
		repos, err := listRepos(ctx, tx, ws)
		if err != nil {
			return err
		}
		seen := make(map[string]Repo, len(repos)+len(paths))
		for _, repo := range repos {
			seen[repoPathKey(repo.Path)] = repo
		}
		var pos int64
		if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(position), -1) + 1 FROM repos WHERE workspace_id = ?", ws).Scan(&pos); err != nil {
			return err
		}
		now := time.Now().Unix()
		for _, path := range paths {
			key := repoPathKey(path)
			if existing, ok := seen[key]; ok {
				if selectExisting {
					last = existing.ID
					if existing.Missing {
						if _, err := tx.ExecContext(ctx, `UPDATE repos SET status = 'active', missing_at = NULL
							WHERE workspace_id = ? AND id = ?`, ws, last); err != nil {
							return err
						}
					}
				}
				continue
			}
			last, err = insertRepo(ctx, tx, ws, path, now, pos)
			if err != nil {
				return err
			}
			seen[key] = Repo{ID: last}
			added++
			pos++
		}
		if last == "" {
			return nil
		}
		if err := setLastRepo(ctx, tx, ws, last, now); err != nil {
			return err
		}
		return rememberWorkspace(ctx, tx, ws)
	})
	if err != nil {
		return 0, "", err
	}
	return added, last, nil
}

func insertRepo(ctx context.Context, tx *sql.Tx, ws, path string, now, pos int64) (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO repos (id, workspace_id, path, added_at, position) VALUES (?, ?, ?, ?, ?)",
		id.String(), ws, path, now, pos)
	return id.String(), err
}

func setLastRepo(ctx context.Context, tx *sql.Tx, ws, repo string, now int64) error {
	_, err := tx.ExecContext(ctx, "UPDATE workspaces SET last_active_repo_id = ?, updated_at = ? WHERE id = ?", repo, now, ws)
	return err
}

func repoPathKey(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return filepath.Clean(path)
}

// UpdatePresence compares both the path and old status before publishing a
// probe, then persists the fallback selection when the old repo is unavailable.
func (s *Store) UpdatePresence(ctx context.Context, ws string, changed []Repo) error {
	return s.transaction(ctx, nil, func(tx *sql.Tx) error {
		now := time.Now().Unix()
		for _, repo := range changed {
			before, after := "missing", "active"
			var missingAt any
			if repo.Missing {
				before, after, missingAt = "active", "missing", now
			}
			if _, err := tx.ExecContext(ctx, `UPDATE repos SET status = ?, missing_at = ?
				WHERE workspace_id = ? AND id = ? AND path = ? AND status = ?`,
				after, missingAt, ws, repo.ID, repo.Path, before); err != nil {
				return err
			}
		}
		return repairLastRepo(ctx, tx, ws)
	})
}

func rememberWorkspace(ctx context.Context, tx *sql.Tx, id string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO app_settings (key, value_json, updated_at)
		VALUES (?, json_quote(?), ?) ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`,
		selectionKey, id, time.Now().Unix())
	return err
}

func requireChange(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return sql.ErrNoRows
	}
	return err
}

func (s *Store) transaction(ctx context.Context, opts *sql.TxOptions, use func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, opts)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := use(tx); err != nil {
		return err
	}
	return tx.Commit()
}
