package workspace

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStableProfileBacksUpCommittedWALAndRetainsLiveChangesForRollback(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "state.db")
	legacy, err := Open(ctx, path)
	must(t, err)
	defer legacy.Close()
	_, err = legacy.db.ExecContext(ctx, "PRAGMA wal_autocheckpoint=0; PRAGMA wal_checkpoint(TRUNCATE)")
	must(t, err)
	ws, err := legacy.CreateWithRepos(ctx, "Old workspace", []string{t.TempDir(), t.TempDir()})
	must(t, err)
	state, err := legacy.LoadWorkspace(ctx, ws.ID)
	must(t, err)
	selected := state.Repos[1].ID
	_, err = legacy.db.ExecContext(ctx, "DELETE FROM app_settings WHERE key = ?", selectionKey)
	must(t, err)
	_, err = legacy.db.ExecContext(ctx, `UPDATE workspaces SET settings_json = '{"future":{"keep":true}}' WHERE id = ?`, ws.ID)
	must(t, err)
	saveWebViewSnapshot(t, ctx, legacy, map[string]string{
		"gitwave.language": "zh-CN", "gitwave.lastActiveWorkspaceId": ws.ID, "gitwave.lastActiveRepoId": selected,
	})
	mainOnly, err := sql.Open("sqlite", databaseURI(path, "mode=ro&immutable=1"))
	must(t, err)
	var count int
	must(t, mainOnly.QueryRowContext(ctx, "SELECT count(*) FROM workspaces").Scan(&count))
	must(t, mainOnly.Close())
	if count != 0 {
		t.Fatalf("test fixture contains %d checkpointed workspaces", count)
	}

	type openedProfile struct {
		store *Store
		err   error
	}
	ready := make(chan struct{})
	results := make(chan openedProfile, 2)
	for range 2 {
		go func() {
			<-ready
			db, err := openWorkspaceDB(ctx, path)
			if err != nil {
				results <- openedProfile{nil, err}
				return
			}
			results <- openedProfile{&Store{db: db, path: path}, initializeStableProfile(ctx, db, path)}
		}()
	}
	close(ready)
	profiles := []openedProfile{<-results, <-results}
	for _, profile := range profiles {
		if profile.store != nil {
			defer profile.store.Close()
		}
	}
	for _, profile := range profiles {
		must(t, profile.err)
		must(t, profile.store.ImportLegacyPreferences(ctx, path))
	}
	live := profiles[0].store
	active, err := live.Load(ctx)
	must(t, err)
	prefs, err := live.DesktopPreferences(ctx)
	must(t, err)
	if active.Workspace.ID != ws.ID || active.Repo.ID != selected || prefs.UILanguage != "zh-CN" {
		t.Fatalf("stable workbench did not restore the legacy selection and preferences: %+v %+v", active, prefs)
	}
	var relative string
	must(t, live.db.QueryRowContext(ctx, `SELECT json_extract(value_json, '$') FROM app_settings WHERE key = ?`, stableProfileKey).Scan(&relative))
	backups, err := filepath.Glob(filepath.Join(filepath.Dir(path), "backup-before-nsgo-*"))
	must(t, err)
	if len(backups) != 1 || filepath.Join(filepath.Dir(path), filepath.FromSlash(relative)) != filepath.Join(backups[0], "state.db") {
		t.Fatalf("expected one retained backup for both opens: %v, %q", backups, relative)
	}
	backup, err := openLegacyReadOnly(filepath.Join(backups[0], "state.db"))
	must(t, err)
	defer backup.Close()
	var label, journal string
	var future bool
	must(t, backup.QueryRowContext(ctx, `SELECT name, json_extract(settings_json, '$.future.keep') FROM workspaces WHERE id = ?`, ws.ID).Scan(&label, &future))
	must(t, backup.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal))
	must(t, backup.QueryRowContext(ctx, "SELECT count(*) FROM app_settings WHERE key IN (?, ?)", stableProfileKey, legacyImportKey).Scan(&count))
	if label != "Old workspace" || !future || journal != "delete" || count != 0 {
		t.Fatalf("rollback backup was changed by initialization: %q %t %q %d", label, future, journal, count)
	}
	must(t, live.Rename(ctx, ws.ID, "Edited in NSGo"))
	// An older version reopening the live file keeps edits made in NSGo.
	downgrade, err := legacy.LoadWorkspace(ctx, ws.ID)
	must(t, err)
	if downgrade.Workspace.Name != "Edited in NSGo" || downgrade.Workspace.LastRepoID != selected {
		t.Fatalf("downgrade did not see current compatible data: %+v", downgrade)
	}
}

func TestStableProfileIsReservedUntilDesktopCloses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	first, err := OpenStable(t.Context(), path)
	must(t, err)
	defer first.Close()
	second, err := OpenStable(t.Context(), path)
	if second != nil {
		second.Close()
	}
	if !errors.Is(err, ErrStableProfileInUse) {
		t.Fatalf("a second desktop reserved the live profile: %v", err)
	}
	must(t, first.Close())
	reopened, err := OpenStable(t.Context(), path)
	must(t, err)
	defer reopened.Close()
}

func TestFreshWorkspaceCanInitializeWALFromConcurrentConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	ready := make(chan struct{})
	done := make(chan error, 6)
	for range 6 {
		go func() {
			<-ready
			store, err := Open(t.Context(), path)
			if err == nil {
				err = store.Close()
			}
			done <- err
		}()
	}
	close(ready)
	for range 6 {
		if err := <-done; err != nil {
			t.Error(err)
		}
	}
}

func TestStableProfileWithNoLegacyDatabaseNeedsNoBackupOrPreferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := OpenStable(t.Context(), path)
	must(t, err)
	defer store.Close()
	must(t, store.ImportLegacyPreferences(t.Context(), path))
	if enabled, err := store.AutoUpdate(t.Context()); err != nil || !enabled {
		t.Fatalf("fresh profile auto-update = %t, %v", enabled, err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	must(t, err)
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "backup-before-nsgo-") {
			t.Fatalf("a new database generated a legacy backup: %s", entry.Name())
		}
	}
}

func TestStableProfileRestartsMigrationAfterACommitFailure(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := openWorkspaceDB(ctx, path)
	must(t, err)
	defer db.Close()
	_, err = db.ExecContext(ctx, `CREATE TABLE schema_version (version INTEGER PRIMARY KEY NOT NULL)`)
	must(t, err)
	for i, name := range []string{"0001-workspaces-repos.sql", "0002-repos-status.sql", "0003-repos-position.sql"} {
		script, err := migrations.ReadFile("migrations/" + name)
		must(t, err)
		_, err = db.ExecContext(ctx, string(script))
		must(t, err)
		_, err = db.ExecContext(ctx, "INSERT INTO schema_version VALUES (?)", i+1)
		must(t, err)
	}
	_, err = db.ExecContext(ctx, `CREATE TRIGGER block_upgrade BEFORE INSERT ON schema_version
		WHEN new.version = 4 BEGIN SELECT RAISE(ABORT, 'fixture prevented migration'); END`)
	must(t, err)
	failed, err := OpenStable(ctx, path)
	if failed != nil || err == nil || !strings.Contains(err.Error(), "fixture prevented migration") {
		t.Fatalf("failed migration produced a usable store: %v", err)
	}
	var migrated bool
	must(t, db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_schema WHERE name = 'app_settings')`).Scan(&migrated))
	if migrated {
		t.Fatal("the failed migration partially changed the live schema")
	}
	_, err = db.ExecContext(ctx, "DROP TRIGGER block_upgrade")
	must(t, err)
	store, err := OpenStable(ctx, path)
	must(t, err)
	defer store.Close()
	if err := store.ImportLegacyPreferences(ctx, path); !errors.Is(err, ErrLegacyPreferencesMissing) {
		t.Fatalf("existing legacy database missing a snapshot produced %v", err)
	}
	if enabled, err := store.AutoUpdate(ctx); err != nil || enabled {
		t.Fatalf("missing legacy preferences enabled auto-update: %t, %v", enabled, err)
	}
	must(t, store.SaveAutoUpdate(ctx, true))
	if enabled, err := store.AutoUpdate(ctx); err != nil || !enabled {
		t.Fatalf("explicit preference was ignored: %t, %v", enabled, err)
	}
	var relative string
	must(t, store.db.QueryRowContext(ctx, `SELECT json_extract(value_json, '$') FROM app_settings WHERE key = ?`, stableProfileKey).Scan(&relative))
	backup, err := openLegacyReadOnly(filepath.Join(filepath.Dir(path), filepath.FromSlash(relative)))
	must(t, err)
	defer backup.Close()
	var latest int
	must(t, backup.QueryRowContext(ctx, `SELECT max(version) FROM schema_version`).Scan(&latest))
	if latest != 3 {
		t.Fatalf("rollback backup schema = %d, want 3", latest)
	}
}
