package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const stableProfileKey = "native.stable-profile.v1"

var ErrStableProfileInUse = errors.New("quit the other GitWave instance using this workspace profile")

// OpenStable opens the original GitWave state.db and preserves a standalone
// pre-NSGo copy before applying schema changes on the first stable launch.
func OpenStable(ctx context.Context, path string) (*Store, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("workspace database requires an absolute path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	lock, err := lockStableProfile(filepath.Join(filepath.Dir(path), ".gitwave-profile.lock"))
	if err != nil {
		return nil, err
	}
	db, err := openWorkspaceDB(ctx, path)
	if err != nil {
		lock.Close()
		return nil, err
	}
	if err := initializeStableProfile(ctx, db, path); err != nil {
		db.Close()
		lock.Close()
		return nil, fmt.Errorf("open the stable workspace database: %w", err)
	}
	return &Store{db: db, path: path, profileLock: lock}, nil
}

func initializeStableProfile(ctx context.Context, db *sql.DB, path string) error {
	// This reserves the original database's SQLite writer from snapshot
	// selection until the schema and initialization record are committed.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var hasSettings, hasData bool
	err = tx.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM sqlite_schema WHERE type = 'table' AND name = 'app_settings'),
		EXISTS(SELECT 1 FROM sqlite_schema WHERE type = 'table' AND name NOT GLOB 'sqlite_*')`).Scan(&hasSettings, &hasData)
	if err != nil {
		return err
	}
	var initialized bool
	if hasSettings {
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM app_settings WHERE key = ?)`, stableProfileKey).Scan(&initialized); err != nil {
			return err
		}
	}
	backup := ""
	if !initialized && hasData {
		backup, err = saveStableProfileBackup(ctx, path)
		if err != nil {
			return fmt.Errorf("back up the previous workspace database: %w", err)
		}
	}
	if err := migrateWorkspace(ctx, tx); err != nil {
		return err
	}
	if !initialized {
		if _, err := tx.ExecContext(ctx, `INSERT INTO app_settings VALUES (?, json_quote(?), ?)`, stableProfileKey, backup, time.Now().Unix()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func saveStableProfileBackup(ctx context.Context, original string) (string, error) {
	// SQLite's backup API requires a separate source connection while the
	// stable launch holds the reserved writer on the original database.
	source, err := openLegacyReadOnly(original)
	if err != nil {
		return "", err
	}
	defer source.Close()
	parent := filepath.Dir(original)
	dir, err := os.MkdirTemp(parent, "backup-before-nsgo-")
	if err != nil {
		return "", err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(dir)
		}
	}()
	if err := protectRecoveryDirectory(dir); err != nil {
		return "", err
	}
	backup := filepath.Join(dir, "state.db")
	if err := publishWorkspaceSnapshot(ctx, source, backup, func(ctx context.Context, path string) error {
		db, err := sql.Open("sqlite", databaseURI(path, "mode=rw&_pragma=trusted_schema(OFF)"))
		if err != nil {
			return err
		}
		db.SetMaxOpenConns(1)
		return sealWorkspaceSnapshot(ctx, db, path)
	}); err != nil {
		return "", err
	}
	for _, path := range []string{dir, parent} {
		if err := syncRecoveryDirectory(path); err != nil {
			return "", err
		}
	}
	keep = true
	return filepath.ToSlash(filepath.Join(filepath.Base(dir), filepath.Base(backup))), nil
}
