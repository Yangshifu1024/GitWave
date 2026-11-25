package workspace

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	db          *sql.DB
	path        string
	profileLock *os.File
}

func PreviewPath() (string, error) {
	return workspacePath("GitWave Preview")
}

func LegacyPath() (string, error) {
	return workspacePath("GitWave")
}

func workspacePath(app string) (string, error) {
	base, err := os.UserConfigDir()
	if runtime.GOOS == "linux" {
		base = os.Getenv("XDG_DATA_HOME")
		if filepath.IsAbs(base) {
			err = nil
		} else {
			var home string
			home, err = os.UserHomeDir()
			base = filepath.Join(home, ".local", "share")
		}
	}
	if err != nil {
		return "", err
	}
	return filepath.Join(base, app, "state.db"), nil
}

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := openWorkspaceDB(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("open workspace database: %w", err)
	}
	return &Store{db: db, path: path}, nil
}

func openWorkspaceDB(ctx context.Context, path string) (*sql.DB, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("workspace database requires an absolute path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create workspace directory: %w", err)
	}
	if err := checkOrphanedDatabaseFiles(path); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		if err := file.Close(); err != nil {
			return nil, fmt.Errorf("create workspace database: %w", err)
		}
	} else if !os.IsExist(err) {
		return nil, fmt.Errorf("create workspace database: %w", err)
	}
	if err := protectDatabaseFiles(path); err != nil {
		return nil, fmt.Errorf("protect workspace database: %w", err)
	}

	db, err := sql.Open("sqlite", databaseURI(path, "_busy_timeout=5000&_foreign_keys=on&_txlock=immediate&_pragma=trusted_schema(OFF)"))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := configureWorkspaceWAL(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("open workspace database: %w", err)
	}
	return db, nil
}

func configureWorkspaceWAL(ctx context.Context, db *sql.DB) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		var journal string
		err := db.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&journal)
		if !sqliteBusy(err) {
			if err == nil && journal != "wal" {
				return fmt.Errorf("SQLite selected journal mode %q instead of WAL", journal)
			}
			return err
		}
		// SQLite can return BUSY without running its busy handler when two
		// first opens both need to change the database's journal mode.
		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), err)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func databaseURI(path, options string) string {
	sqlitePath := filepath.ToSlash(path)
	if !strings.HasPrefix(sqlitePath, "/") {
		sqlitePath = "/" + sqlitePath
	}
	uri := url.URL{Scheme: "file", Path: sqlitePath, RawQuery: options}
	return uri.String()
}

func (s *Store) Close() error {
	err := s.db.Close()
	if s.profileLock != nil {
		if closeErr := s.profileLock.Close(); !errors.Is(closeErr, os.ErrClosed) {
			err = errors.Join(err, closeErr)
		}
	}
	return err
}

func (s *Store) DataDirectory() string { return filepath.Dir(s.path) }

func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := migrateWorkspace(ctx, tx); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateWorkspace(ctx context.Context, tx *sql.Tx) error {
	files, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY NOT NULL)"); err != nil {
		return err
	}
	var newest int
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&newest); err != nil {
		return err
	}
	if newest > len(files) {
		return fmt.Errorf("workspace database schema %d is newer than supported schema %d", newest, len(files))
	}
	for i, file := range files {
		version := i + 1
		prefix := fmt.Sprintf("%04d-", version)
		if !strings.HasPrefix(file.Name(), prefix) {
			return fmt.Errorf("unexpected workspace migration %q", file.Name())
		}
		var applied bool
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM schema_version WHERE version = ?)", version).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		script, err := migrations.ReadFile("migrations/" + file.Name())
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(script)); err != nil {
			return fmt.Errorf("apply workspace migration %s: %w", file.Name(), err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_version (version) VALUES (?)", version); err != nil {
			return err
		}
	}
	return nil
}
