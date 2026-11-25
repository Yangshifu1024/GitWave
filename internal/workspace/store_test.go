package workspace

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestMigratesLegacyDatabaseWithoutChangingWorkspaceData(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "legacy %#.db")
	slashPath := filepath.ToSlash(path)
	if !strings.HasPrefix(slashPath, "/") {
		slashPath = "/" + slashPath
	}
	uri := url.URL{Scheme: "file", Path: slashPath}
	legacy, err := sql.Open("sqlite", uri.String())
	must(t, err)
	files, err := migrations.ReadDir("migrations")
	must(t, err)
	for _, file := range files[:2] {
		script, err := migrations.ReadFile("migrations/" + file.Name())
		must(t, err)
		_, err = legacy.Exec(string(script))
		must(t, err)
	}
	settings := `{"prompt_templates":{},"future":{"keep":true}}`
	override := `{"future_override":true}`
	_, err = legacy.Exec(`CREATE TABLE schema_version (version INTEGER PRIMARY KEY NOT NULL);
		INSERT INTO schema_version VALUES (1), (2);
		INSERT INTO workspaces VALUES ('old-workspace', 'Original', 'placeholder', 'repo-z', 1, 2);
		INSERT INTO repos (id, workspace_id, path, nickname, settings_override_json, added_at, status)
		VALUES ('repo-z', 'old-workspace', '/existing/z', 'Z', NULL, 3, 'missing'),
		       ('repo-a', 'old-workspace', '/existing/a', 'A', NULL, 3, 'active');`)
	must(t, err)
	_, err = legacy.Exec("UPDATE workspaces SET settings_json = ?", settings)
	must(t, err)
	_, err = legacy.Exec("UPDATE repos SET settings_override_json = ? WHERE id = 'repo-a'", override)
	must(t, err)
	must(t, legacy.Close())

	store, err := Open(ctx, path)
	must(t, err)
	defer store.Close()
	must(t, store.SelectWorkspace(ctx, "old-workspace"))
	state, err := store.Load(ctx)
	must(t, err)
	if state.Workspace.LastRepoID != "repo-z" || state.Repo.ID != "repo-a" ||
		len(state.Repos) != 2 || state.Repos[0].Nickname != "A" || !state.Repos[1].Missing {
		t.Fatalf("restored workspace: %+v", state)
	}
	must(t, store.Rename(ctx, "old-workspace", "Renamed"))
	var gotSettings, gotOverride string
	err = store.db.QueryRow(`SELECT w.settings_json, r.settings_override_json FROM workspaces w
		JOIN repos r ON r.workspace_id = w.id WHERE r.id = 'repo-a'`).Scan(&gotSettings, &gotOverride)
	must(t, err)
	if gotSettings != settings || gotOverride != override {
		t.Fatalf("settings changed to %q and %q", gotSettings, gotOverride)
	}
	var version int
	must(t, store.db.QueryRow("SELECT MAX(version) FROM schema_version").Scan(&version))
	if version != 4 {
		t.Fatalf("schema version = %d", version)
	}
}

func TestWorkspaceSwitchAndRestartRestoreRepo(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(ctx, path)
	must(t, err)
	defer func() {
		if store != nil {
			store.Close()
		}
	}()
	must(t, store.Create(ctx, " First "))
	first, err := store.Load(ctx)
	must(t, err)
	if first.Workspace.Name != "First" || first.Repo.ID != "" {
		t.Fatalf("new workspace: %+v", first)
	}
	added, _, err := store.AddRepos(ctx, first.Workspace.ID, []string{"/repo/a", "/repo/a", "/repo/b"})
	must(t, err)
	if added != 2 {
		t.Fatalf("added %d repos", added)
	}
	first, err = store.Load(ctx)
	must(t, err)
	if len(first.Repos) != 2 || first.Repos[0].Path != "/repo/a" || first.Repo.Path != "/repo/b" {
		t.Fatalf("after add: %+v", first)
	}
	must(t, store.Create(ctx, "Second"))
	second, err := store.Load(ctx)
	must(t, err)
	if !slices.ContainsFunc(second.Workspaces, func(ws Summary) bool { return ws.ID == first.Workspace.ID }) {
		t.Fatalf("workspace list: %+v", second.Workspaces)
	}
	if err := store.SelectRepo(ctx, second.Workspace.ID, first.Repo.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("selecting another workspace's repo: %v", err)
	}
	must(t, store.SelectWorkspace(ctx, first.Workspace.ID))
	must(t, store.Close())
	store, err = Open(ctx, path)
	must(t, err)
	got, err := store.Load(ctx)
	must(t, err)
	if got.Workspace != first.Workspace || got.Repo != first.Repo || !slices.Equal(got.Repos, first.Repos) {
		t.Fatalf("after reopening: %+v, want %+v", got, first)
	}
	missing := first.Repo
	missing.Missing = true
	must(t, store.UpdatePresence(ctx, first.Workspace.ID, []Repo{missing}))
	got, err = store.Load(ctx)
	must(t, err)
	if got.Repo.ID != first.Repos[0].ID || !got.Repos[1].Missing {
		t.Fatalf("missing repo fallback: %+v", got)
	}
	must(t, store.Delete(ctx, first.Workspace.ID))
	got, err = store.Load(ctx)
	must(t, err)
	var remaining int
	must(t, store.db.QueryRow("SELECT COUNT(*) FROM repos").Scan(&remaining))
	if len(got.Workspaces) != 1 || got.Workspace.ID != "" || remaining != 0 {
		t.Fatalf("after deleting active workspace: %+v, repo count %d", got, remaining)
	}
}

func TestAddKeepsLegacySymlinkPathWithoutAddingDuplicate(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	actual, alias := filepath.Join(dir, "actual"), filepath.Join(dir, "alias")
	must(t, os.Mkdir(actual, 0o700))
	if err := os.Symlink(actual, alias); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	store, err := Open(ctx, filepath.Join(dir, "state.db"))
	must(t, err)
	defer store.Close()
	must(t, store.Create(ctx, "Aliases"))
	initial, err := store.Load(ctx)
	must(t, err)
	_, err = store.db.Exec(`INSERT INTO repos (id, workspace_id, path, added_at, position)
		VALUES ('legacy-repo', ?, ?, 0, 0)`, initial.Workspace.ID, alias)
	must(t, err)
	added, _, err := store.AddRepos(ctx, initial.Workspace.ID, []string{actual})
	must(t, err)
	state, err := store.Load(ctx)
	must(t, err)
	if added != 0 || len(state.Repos) != 1 || state.Repo.Path != alias || state.Repo.ID != "legacy-repo" {
		t.Fatalf("added %d repos, got %+v", added, state)
	}
}

func TestLoadReadsWALSnapshotWhileAnotherConnectionWrites(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	writer, err := Open(ctx, path)
	must(t, err)
	defer writer.Close()
	reader, err := Open(ctx, path)
	must(t, err)
	defer reader.Close()
	must(t, writer.Create(ctx, "Before"))
	tx, err := writer.db.BeginTx(ctx, nil)
	must(t, err)
	defer tx.Rollback()
	_, err = tx.Exec("UPDATE workspaces SET name = 'After'")
	must(t, err)
	state, err := reader.Load(ctx)
	must(t, err)
	if state.Workspace.Name != "Before" {
		t.Fatalf("read uncommitted workspace data: %+v", state)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
