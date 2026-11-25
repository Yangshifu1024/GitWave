package workspace

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFirstLaunchCopiesCommittedLegacyWALAndItsOwnPreferenceSnapshot(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	previous, target := filepath.Join(dir, "old %#.db"), filepath.Join(dir, "native", "state.db")
	legacy, err := Open(ctx, previous)
	must(t, err)
	defer legacy.Close()
	_, err = legacy.db.ExecContext(ctx, "PRAGMA wal_autocheckpoint=0; PRAGMA wal_checkpoint(TRUNCATE)")
	must(t, err)
	ws, err := legacy.CreateWithRepos(ctx, "WAL workspace", []string{t.TempDir(), t.TempDir()})
	must(t, err)
	state, err := legacy.LoadWorkspace(ctx, ws.ID)
	must(t, err)
	repo := state.Repos[1]
	_, err = legacy.db.ExecContext(ctx, "DELETE FROM app_settings WHERE key = ?", selectionKey)
	must(t, err)
	_, err = legacy.db.ExecContext(ctx, "UPDATE repos SET nickname = 'Preserved' WHERE id = ?", repo.ID)
	must(t, err)
	_, err = legacy.db.ExecContext(ctx, "UPDATE workspaces SET settings_json = ? WHERE id = ?",
		`{"prompt_templates":{},"future":{"enabled":true}}`, ws.ID)
	must(t, err)
	values := map[string]string{"gitwave.language": "zh-CN", "gitwave.lastActiveWorkspaceId": ws.ID, "gitwave.lastActiveRepoId": repo.ID}
	saveWebViewSnapshot(t, ctx, legacy, values)

	// Confirm a plain file copy would miss the commits in the live WAL.
	withoutWAL, err := sql.Open("sqlite", databaseURI(previous, "mode=ro&immutable=1"))
	must(t, err)
	var count int
	must(t, withoutWAL.QueryRowContext(ctx, "SELECT count(*) FROM workspaces").Scan(&count))
	must(t, withoutWAL.Close())
	if count != 0 {
		t.Fatalf("the fixture should store the workspace only in WAL, found %d in the main file", count)
	}

	store, err := OpenWithLegacy(ctx, target, previous)
	must(t, err)
	defer store.Close()
	values["gitwave.language"] = "en"
	must(t, legacy.Rename(ctx, ws.ID, "Changed after snapshot"))
	saveWebViewSnapshot(t, ctx, legacy, values)
	must(t, store.ImportLegacyPreferences(ctx, previous))
	restored, err := store.Load(ctx)
	must(t, err)
	if restored.Workspace.Name != ws.Name || restored.Workspace.ID != ws.ID || restored.Repo.ID != repo.ID || restored.Repo.Nickname != "Preserved" {
		t.Fatalf("restored WAL state = %+v", restored)
	}
	prefs, err := store.DesktopPreferences(ctx)
	must(t, err)
	if prefs.UILanguage != "zh-CN" {
		t.Fatalf("copied profile used a newer source preference: %+v", prefs)
	}
	var future bool
	must(t, store.db.QueryRowContext(ctx, "SELECT json_extract(settings_json, '$.future.enabled') FROM workspaces WHERE id=?",
		ws.ID).Scan(&future))
	if !future {
		t.Fatal("a legacy workspace setting was lost during the copy")
	}
	if err := legacy.db.QueryRowContext(ctx, "SELECT count(*) FROM app_settings WHERE key IN (?, ?)",
		legacyDatabaseCopyKey, legacyImportKey).Scan(&count); err != nil || count != 0 {
		t.Fatalf("legacy database was mutated by the copy or preference import: %d, %v", count, err)
	}
	second, err := OpenWithLegacy(ctx, target, previous)
	must(t, err)
	defer second.Close()
	again, err := second.Load(ctx)
	must(t, err)
	if again.Workspace != restored.Workspace || again.Repo != restored.Repo {
		t.Fatalf("reopening imported source changes: %+v", again)
	}
}

func TestUnsupportedLegacyBackupNeverPublishesANewProfile(t *testing.T) {
	ctx := t.Context()
	previous := filepath.Join(t.TempDir(), "previous.db")
	legacy, err := Open(ctx, previous)
	must(t, err)
	defer legacy.Close()
	_, err = legacy.db.ExecContext(ctx, "INSERT INTO schema_version VALUES (99)")
	must(t, err)
	target := filepath.Join(t.TempDir(), "state.db")
	if store, err := OpenWithLegacy(ctx, target, previous); err == nil {
		store.Close()
		t.Fatal("unsupported schema was published")
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed copy left a destination: %v", err)
	}
	_, err = legacy.db.ExecContext(ctx, "DELETE FROM schema_version WHERE version=99")
	must(t, err)
	store, err := OpenWithLegacy(ctx, target, previous)
	must(t, err)
	defer store.Close()
	if err := store.ImportLegacyPreferences(ctx, previous); !errors.Is(err, ErrLegacyPreferencesMissing) {
		t.Fatalf("copied profile with no WebView snapshot = %v", err)
	}
}

func TestFirstLaunchPreservesOrphanedSQLiteFiles(t *testing.T) {
	ctx := t.Context()
	previous := filepath.Join(t.TempDir(), "previous.db")
	legacy, err := Open(ctx, previous)
	must(t, err)
	defer legacy.Close()
	target := filepath.Join(t.TempDir(), "state.db")
	const orphan = "unresolved WAL contents"
	must(t, os.WriteFile(target+"-wal", []byte(orphan), 0o600))
	if store, err := OpenWithLegacy(ctx, target, previous); err == nil {
		store.Close()
		t.Fatal("published a copy over another database's WAL")
	}
	if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed copy left a destination: %v", err)
	}
	stored, err := os.ReadFile(target + "-wal")
	must(t, err)
	if string(stored) != orphan {
		t.Fatalf("the orphan WAL changed: %q", stored)
	}
}
