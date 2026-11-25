package workspace

import (
	"context"
	"database/sql"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const transferVersion = 1

// Transfer is the legacy .gitwave-workspace.json format.
type Transfer struct {
	Version uint32   `json:"version"`
	Name    string   `json:"name"`
	Repos   []string `json:"repos"`
}

func ReadTransfer(path string) (Transfer, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Transfer{}, err
	}
	if !info.Mode().IsRegular() {
		return Transfer{}, fmt.Errorf("workspace transfer must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return Transfer{}, err
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil {
		return Transfer{}, err
	}
	if !info.Mode().IsRegular() {
		return Transfer{}, fmt.Errorf("workspace transfer must be a regular file")
	}
	var input struct {
		Version *uint32   `json:"version"`
		Name    *string   `json:"name"`
		Repos   []*string `json:"repos"`
	}
	if err := json.UnmarshalRead(file, &input); err != nil {
		return Transfer{}, fmt.Errorf("invalid workspace transfer: %w", err)
	}
	if input.Version == nil || input.Name == nil || input.Repos == nil {
		return Transfer{}, fmt.Errorf("workspace transfer requires version, name and repos")
	}
	if *input.Version != transferVersion {
		return Transfer{}, fmt.Errorf("unsupported workspace transfer version %d", *input.Version)
	}
	name := strings.TrimSpace(*input.Name)
	if name == "" {
		return Transfer{}, fmt.Errorf("workspace name is empty")
	}
	transfer := Transfer{Version: transferVersion, Name: name, Repos: make([]string, 0, len(input.Repos))}
	for _, path := range input.Repos {
		if path == nil {
			return Transfer{}, fmt.Errorf("workspace transfer repository paths must be strings")
		}
		transfer.Repos = append(transfer.Repos, *path)
	}
	return transfer, nil
}

// TransferTargetExists allows confirmation to be tied to a single path and
// file version. Export never replaces a destination without this evidence.
type TransferTargetExists struct {
	Path string
	info os.FileInfo
}

func (e *TransferTargetExists) Error() string {
	return fmt.Sprintf("workspace export destination already exists: %s", e.Path)
}

func (s *Store) Export(ctx context.Context, id, path string, replace *TransferTargetExists) error {
	transfer := Transfer{Version: transferVersion, Repos: []string{}}
	err := s.transaction(ctx, &sql.TxOptions{ReadOnly: true}, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, "SELECT name FROM workspaces WHERE id = ?", id).Scan(&transfer.Name); err != nil {
			return err
		}
		repos, err := listRepos(ctx, tx, id)
		if err != nil {
			return err
		}
		for _, repo := range repos {
			transfer.Repos = append(transfer.Repos, repo.Path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.databaseFile(path) {
		return fmt.Errorf("workspace export destination belongs to the active SQLite database: %s", path)
	}
	if _, err := checkTransferTarget(path, replace); err != nil {
		return err
	}
	return s.publishTransfer(ctx, path, transfer, replace)
}

func (s *Store) databaseFile(path string) bool {
	target := repoPathKey(filepath.Join(repoPathKey(filepath.Dir(path)), filepath.Base(path)))
	info, _ := os.Stat(path)
	resolved, _ := filepath.EvalSymlinks(s.path)
	for _, database := range []string{s.path, resolved} {
		if database == "" {
			continue
		}
		for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
			sidecar := database + suffix
			if repoPathKey(filepath.Join(repoPathKey(filepath.Dir(sidecar)), filepath.Base(sidecar))) == target {
				return true
			}
			if other, err := os.Stat(sidecar); err == nil && info != nil && os.SameFile(info, other) {
				return true
			}
		}
	}
	return false
}

func checkTransferTarget(path string, replace *TransferTargetExists) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("workspace export destination is not a regular file: %s", path)
	}
	if replace == nil || replace.Path != path || replace.info == nil || !os.SameFile(replace.info, info) ||
		replace.info.Size() != info.Size() || !replace.info.ModTime().Equal(info.ModTime()) {
		return false, &TransferTargetExists{Path: path, info: info}
	}
	return true, nil
}

func (s *Store) publishTransfer(ctx context.Context, path string, transfer Transfer, replace *TransferTargetExists) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".gitwave-workspace-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err := writeTransferFile(temp, transfer); err != nil {
		return err
	}
	// The preview's SQLite writer lock is shared by all its windows and
	// processes. Recheck confirmation under that lock before installing.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	exists, err := checkTransferTarget(path, replace)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if exists {
		return os.Rename(temp.Name(), path)
	}
	err = publishNewFile(temp.Name(), path)
	if err != nil {
		if _, conflict := checkTransferTarget(path, nil); conflict != nil {
			return conflict
		}
	}
	return err
}

func writeTransferFile(file *os.File, transfer Transfer) error {
	err := json.MarshalWrite(file, transfer, jsontext.WithIndent("  "))
	if err == nil {
		err = file.Sync()
	}
	return errors.Join(err, file.Close())
}
