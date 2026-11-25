package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

const legacyDatabaseCopyKey = "native.legacy-database-copy.v1"

// OpenWithLegacy creates the Go profile from a consistent snapshot on its
// first launch. A previously created Go profile retains its own data.
func OpenWithLegacy(ctx context.Context, path, previous string) (*Store, error) {
	if !filepath.IsAbs(path) || !filepath.IsAbs(previous) {
		return nil, errors.New("workspace database paths must be absolute")
	}
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) && filepath.Clean(path) != filepath.Clean(previous) {
		if err := backupLegacyDatabase(ctx, path, previous); err != nil {
			return nil, fmt.Errorf("restore previous workspace database: %w", err)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return Open(ctx, path)
}

// A live WAL can contain committed work that is absent from state.db.
func backupLegacyDatabase(ctx context.Context, path, previous string) error {
	if err := checkOrphanedDatabaseFiles(path); err != nil {
		return err
	}
	source, err := openLegacyReadOnly(previous)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer source.Close()
	if err := publishWorkspaceSnapshot(ctx, source, path, validateWorkspaceSnapshot); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return nil
}

func publishWorkspaceSnapshot(ctx context.Context, source *sql.DB, path string, prepare func(context.Context, string) error) error {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	private, err := os.MkdirTemp(parent, ".gitwave-upgrade-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(private)
	file, err := os.CreateTemp(private, "state-*.db")
	if err != nil {
		return err
	}
	stage := file.Name()
	if err := file.Close(); err != nil {
		return err
	}
	if err := copySQLiteSnapshot(ctx, source, stage); err != nil {
		return err
	}
	if err := prepare(ctx, stage); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := checkOrphanedDatabaseFiles(path); err != nil {
		return err
	}
	return publishNewFile(stage, path)
}

func checkOrphanedDatabaseFiles(path string) error {
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Lstat(path + suffix); !errors.Is(err, os.ErrNotExist) {
			if err != nil {
				return err
			}
			// Another process may have created the main file and then enabled WAL.
			if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return fmt.Errorf("workspace database is missing while its SQLite %s file is still present", suffix[1:])
		}
	}
	return nil
}

func openLegacyReadOnly(path string) (*sql.DB, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("the legacy workspace database path must be absolute")
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("the legacy workspace database must be a regular file")
	}
	return sql.Open("sqlite", databaseURI(path, "mode=ro&_pragma=query_only(ON)&_pragma=trusted_schema(OFF)&_pragma=busy_timeout(250)"))
}

func copySQLiteSnapshot(ctx context.Context, source *sql.DB, stage string) error {
	conn, err := source.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Raw(func(driverConn any) (err error) {
		backuper, ok := driverConn.(interface {
			NewBackup(string) (*sqlite.Backup, error)
		})
		if !ok {
			return errors.New("SQLite online backup is unavailable")
		}
		backup, err := backuper.NewBackup(databaseURI(stage, "mode=rw&_pragma=trusted_schema(OFF)"))
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, backup.Finish()) }()
		for ctx.Err() == nil {
			more, err := backup.Step(256)
			if err == nil {
				if !more {
					return ctx.Err()
				}
				continue
			}
			if !sqliteBusy(err) {
				return err
			}
			select {
			case <-ctx.Done():
			case <-time.After(20 * time.Millisecond):
			}
		}
		return ctx.Err()
	})
}

func sqliteBusy(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	code := sqliteErr.Code() & 0xff
	return code == sqlite3.SQLITE_BUSY || code == sqlite3.SQLITE_LOCKED
}

func validateWorkspaceSnapshot(ctx context.Context, path string) error {
	store, err := Open(ctx, path)
	if err != nil {
		return err
	}
	_, err = store.db.ExecContext(ctx, "INSERT OR IGNORE INTO app_settings VALUES (?, '1', ?)",
		legacyDatabaseCopyKey, time.Now().Unix())
	if err != nil {
		return errors.Join(err, store.Close())
	}
	return sealWorkspaceSnapshot(ctx, store.db, path)
}

func sealWorkspaceSnapshot(ctx context.Context, db *sql.DB, path string) error {
	var check, journal string
	err := db.QueryRowContext(ctx, "PRAGMA quick_check(1)").Scan(&check)
	if err == nil && check != "ok" {
		err = errors.New("the copied workspace database failed its integrity check")
	}
	if err == nil {
		// Publish one complete database file, independent of temporary sidecars.
		err = db.QueryRowContext(ctx, "PRAGMA journal_mode=DELETE").Scan(&journal)
		if err == nil && journal != "delete" {
			err = errors.New("the copied workspace database could not be checkpointed")
		}
	}
	if err = errors.Join(err, db.Close()); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	return errors.Join(file.Sync(), file.Close())
}
