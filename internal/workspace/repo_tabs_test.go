package workspace

import (
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"testing"
)

func TestRepoTabMutationsRejectStaleAndForeignTabs(t *testing.T) {
	ctx := t.Context()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	must(t, err)
	defer store.Close()
	must(t, store.Create(ctx, "First"))
	state, err := store.Load(ctx)
	must(t, err)
	ws := state.Workspace.ID
	_, _, err = store.AddRepos(ctx, ws, []string{"/git/a", "/git/b", "/git/moved"})
	must(t, err)
	state, err = store.Load(ctx)
	must(t, err)
	a, b, moved := state.Repos[0], state.Repos[1], state.Repos[2]
	moved.Missing = true
	must(t, store.UpdatePresence(ctx, ws, []Repo{moved}))
	const settings = `{"future_field":true}`
	_, err = store.db.Exec("UPDATE repos SET settings_override_json = ?, nickname = 'My repo' WHERE id = ?", settings, moved.ID)
	must(t, err)
	must(t, store.Create(ctx, "Second"))
	second, err := store.Load(ctx)
	must(t, err)
	if err := store.RemoveRepo(ctx, second.Workspace.ID, a); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("removing a foreign tab: %v", err)
	}
	if err := store.RelinkRepo(ctx, second.Workspace.ID, moved, "/git/new"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("relinking a foreign tab: %v", err)
	}
	must(t, store.SelectWorkspace(ctx, ws))
	initial := []string{a.ID, b.ID, moved.ID}
	order := []string{moved.ID, b.ID, a.ID}
	if err := store.ReorderRepos(ctx, ws, initial, []string{moved.ID, b.ID, b.ID}); !errors.Is(err, ErrRepoOrderChanged) {
		t.Fatalf("reordering with duplicate ID: %v", err)
	}
	must(t, store.ReorderRepos(ctx, ws, initial, order))
	if err := store.ReorderRepos(ctx, ws, initial, []string{b.ID, moved.ID, a.ID}); !errors.Is(err, ErrRepoOrderChanged) {
		t.Fatalf("reordering against stale snapshot: %v", err)
	}
	if err := store.RelinkRepo(ctx, ws, moved, b.Path); !errors.Is(err, ErrRepoPathInUse) {
		t.Fatalf("relinking to duplicate repo: %v", err)
	}
	must(t, store.RelinkRepo(ctx, ws, moved, "/git/new"))
	if err := store.RemoveRepo(ctx, ws, moved); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("removing relinked tab with stale path: %v", err)
	}
	state, err = store.Load(ctx)
	must(t, err)
	var ids []string
	for _, repo := range state.Repos {
		ids = append(ids, repo.ID)
	}
	if !slices.Equal(ids, order) || state.Repos[0].Missing || state.Repos[0].Path != "/git/new" ||
		state.Repos[0].Nickname != "My repo" || state.Repo.ID != a.ID {
		t.Fatalf("repo details or active repo changed during relink: %+v", state)
	}
	var storedSettings string
	var missingAt sql.NullInt64
	must(t, store.db.QueryRow("SELECT settings_override_json, missing_at FROM repos WHERE id = ?", moved.ID).Scan(&storedSettings, &missingAt))
	if storedSettings != settings || missingAt.Valid {
		t.Fatalf("relink metadata: %q, missing at %+v", storedSettings, missingAt)
	}
	must(t, store.RemoveRepo(ctx, ws, a))
	state, err = store.Load(ctx)
	must(t, err)
	if state.Repo.ID != moved.ID || state.Workspace.LastRepoID != moved.ID || len(state.Repos) != 2 {
		t.Fatalf("selection was not repaired after removal: %+v", state)
	}
}
