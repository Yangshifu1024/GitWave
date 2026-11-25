package workspace

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var ErrCloneDestinationRegistered = errors.New("the destination contains a repository registered in a workspace")

// CheckCloneDestination protects the paths used by every open workspace,
// including repositories nested under a proposed replacement.
func (s *Store) CheckCloneDestination(ctx context.Context, ws, destination string) error {
	info, _ := os.Stat(destination)
	key := repoPathKey(destination)
	return s.transaction(ctx, &sql.TxOptions{ReadOnly: true}, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT id FROM workspaces WHERE id = ?", ws).Scan(new(string)); err != nil {
			return err
		}
		rows, err := tx.QueryContext(ctx, "SELECT DISTINCT path FROM repos")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var path string
			if err := rows.Scan(&path); err != nil {
				return err
			}
			path = repoPathKey(path)
			if rel, err := filepath.Rel(key, path); err == nil && rel != ".." &&
				!strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return ErrCloneDestinationRegistered
			}
			for ancestor := path; info != nil; ancestor = filepath.Dir(ancestor) {
				if other, err := os.Stat(ancestor); err == nil && os.SameFile(info, other) {
					return ErrCloneDestinationRegistered
				}
				if filepath.Dir(ancestor) == ancestor {
					break
				}
			}
		}
		return rows.Err()
	})
}
