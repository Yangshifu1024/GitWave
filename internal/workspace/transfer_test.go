package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestReadWorkspaceTransferAcceptsLegacyJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.gitwave-workspace.json")
	const legacy = `{
  "version": 1,
  "name": "旧版示例 & Tools",
  "repos": [
    "/projects/with spaces",
    "C:\\Code\\GitWave",
    "/projects/line\nbreak"
  ]
}`
	must(t, os.WriteFile(path, []byte(legacy), 0o600))
	transfer, err := ReadTransfer(path)
	must(t, err)
	expected := []string{"/projects/with spaces", `C:\Code\GitWave`, "/projects/line\nbreak"}
	if transfer.Version != 1 || transfer.Name != "旧版示例 & Tools" || !slices.Equal(transfer.Repos, expected) {
		t.Fatalf("decoded legacy transfer: %+v", transfer)
	}
}

func TestReadWorkspaceTransferRejectsInvalidDocuments(t *testing.T) {
	for name, input := range map[string]string{
		"malformed":     `{`,
		"version":       `{"version":2,"name":"Test","repos":[]}`,
		"duplicates":    `{"version":1,"version":2,"name":"Test","repos":[]}`,
		"missing field": `{"version":1,"name":"Test"}`,
		"null name":     `{"version":1,"name":null,"repos":[]}`,
		"null repo":     `{"version":1,"name":"Test","repos":[null]}`,
		"blank name":    `{"version":1,"name":" ","repos":[]}`,
		"trailing data": `{"version":1,"name":"Test","repos":[]} {}`,
		"invalid UTF-8": "{\"version\":1,\"name\":\"\xff\",\"repos\":[]}",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "transfer.json")
			must(t, os.WriteFile(path, []byte(input), 0o600))
			if _, err := ReadTransfer(path); err == nil {
				t.Fatalf("accepted invalid transfer %q", input)
			}
		})
	}
}

func TestWorkspaceExportKeepsOrderAndOmitsPrivateSettings(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	store, err := Open(ctx, filepath.Join(dir, "state.db"))
	must(t, err)
	defer store.Close()
	ws, err := store.CreateWithRepos(ctx, "Original & 中文", []string{"/repos/first", "/repos/second"})
	must(t, err)
	state, err := store.Load(ctx)
	must(t, err)
	first, second := state.Repos[0], state.Repos[1]
	before := []string{first.ID, second.ID}
	must(t, store.ReorderRepos(ctx, ws.ID, before, []string{second.ID, first.ID}))
	second.Missing = true
	must(t, store.UpdatePresence(ctx, ws.ID, []Repo{second}))
	_, err = store.db.Exec(`UPDATE workspaces SET settings_json = '{"api_key":"private value","prompt_templates":{}}' WHERE id = ?`, ws.ID)
	must(t, err)
	_, err = store.db.Exec(`UPDATE repos SET settings_override_json = '{"token":"private value"}', nickname = 'Private nickname' WHERE id = ?`, first.ID)
	must(t, err)
	must(t, store.Create(ctx, "Other active workspace"))
	path := filepath.Join(dir, "saved.gitwave-workspace.json")
	must(t, store.Export(ctx, ws.ID, path, nil))
	raw, err := os.ReadFile(path)
	must(t, err)
	var fields map[string]json.RawMessage
	must(t, json.Unmarshal(raw, &fields))
	if len(fields) != 3 || fields["version"] == nil || fields["name"] == nil || fields["repos"] == nil {
		t.Fatalf("export contains unexpected fields: %s", raw)
	}
	result, err := ReadTransfer(path)
	must(t, err)
	if result.Name != ws.Name || !slices.Equal(result.Repos, []string{second.Path, first.Path}) {
		t.Fatalf("exported transfer: %+v", result)
	}
	state, err = store.Load(ctx)
	must(t, err)
	if state.Workspace.Name != "Other active workspace" {
		t.Fatalf("export switched workspace: %+v", state)
	}
}

func TestWorkspaceExportRequiresConfirmationForCurrentFile(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	store, err := Open(ctx, filepath.Join(dir, "state.db"))
	must(t, err)
	defer store.Close()
	ws, err := store.CreateWithRepos(ctx, "Export", nil)
	must(t, err)
	path := filepath.Join(dir, "transfer.json")
	must(t, os.WriteFile(path, []byte("existing"), 0o600))
	var target *TransferTargetExists
	if err := store.Export(ctx, ws.ID, path, nil); !errors.As(err, &target) {
		t.Fatalf("export did not ask to replace: %v", err)
	}
	const updated = "modified during confirmation"
	must(t, os.WriteFile(path, []byte(updated), 0o600))
	var retry *TransferTargetExists
	if err := store.Export(ctx, ws.ID, path, target); !errors.As(err, &retry) {
		t.Fatalf("export did not recheck destination: %v", err)
	}
	content, err := os.ReadFile(path)
	must(t, err)
	if string(content) != updated {
		t.Fatalf("export replaced changed file: %q", content)
	}
	must(t, store.Export(ctx, ws.ID, path, retry))
	transfer, err := ReadTransfer(path)
	must(t, err)
	if transfer.Name != ws.Name || transfer.Repos == nil || len(transfer.Repos) != 0 {
		t.Fatalf("invalid empty workspace export: %+v", transfer)
	}
}

func TestConcurrentWorkspaceExportsDoNotClobberEachOther(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")
	store, err := Open(ctx, dbPath)
	must(t, err)
	defer store.Close()
	a, err := store.CreateWithRepos(ctx, "First", nil)
	must(t, err)
	b, err := store.CreateWithRepos(ctx, "Second", nil)
	must(t, err)
	other, err := Open(ctx, dbPath)
	must(t, err)
	defer other.Close()
	for _, existing := range []bool{false, true} {
		name := "new file"
		if existing {
			name = "confirmed replacement"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "transfer.json")
			var target *TransferTargetExists
			if existing {
				must(t, os.WriteFile(path, []byte("existing file"), 0o600))
				if err := store.Export(ctx, a.ID, path, nil); !errors.As(err, &target) {
					t.Fatalf("needed replacement confirmation: %v", err)
				}
			}
			type result struct {
				name string
				err  error
			}
			results := make(chan result, 2)
			go func() { results <- result{a.Name, store.Export(ctx, a.ID, path, target)} }()
			go func() { results <- result{b.Name, other.Export(ctx, b.ID, path, target)} }()
			winner := ""
			for range 2 {
				res := <-results
				if res.err == nil && winner == "" {
					winner = res.name
					continue
				}
				var occupied *TransferTargetExists
				if !errors.As(res.err, &occupied) {
					t.Fatalf("expected exactly one export and one conflict, got %q then %+v", winner, res)
				}
			}
			transfer, err := ReadTransfer(path)
			must(t, err)
			if winner == "" || transfer.Name != winner {
				t.Fatalf("winner %q differs from file: %+v", winner, transfer)
			}
		})
	}
}

func TestWorkspaceExportProtectsDatabasePaths(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(ctx, path)
	must(t, err)
	defer store.Close()
	ws, err := store.CreateWithRepos(ctx, "Workspace", nil)
	must(t, err)
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		err := store.Export(ctx, ws.ID, path+suffix, nil)
		if err == nil || !strings.Contains(err.Error(), "active SQLite database") {
			t.Fatalf("did not protect %q: %v", path+suffix, err)
		}
	}
	state, err := store.Load(ctx)
	must(t, err)
	if state.Workspace.ID != ws.ID {
		t.Fatalf("workspace state was affected: %+v", state)
	}
}

func TestCreateWorkspaceWithReposRollsBackIfAnyWriteFails(t *testing.T) {
	ctx := t.Context()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	must(t, err)
	defer store.Close()
	must(t, store.Create(ctx, "Current"))
	_, err = store.db.Exec(`CREATE TRIGGER reject_repo BEFORE INSERT ON repos WHEN NEW.path = '/bad'
		BEGIN SELECT RAISE(ABORT, 'test simulated write failure'); END`)
	must(t, err)
	if _, err := store.CreateWithRepos(ctx, "Partial import", []string{"/good", "/bad"}); err == nil {
		t.Fatal("expected import transaction to fail")
	}
	state, err := store.Load(ctx)
	must(t, err)
	var count int
	must(t, store.db.QueryRow("SELECT COUNT(*) FROM repos").Scan(&count))
	if state.Workspace.Name != "Current" || len(state.Workspaces) != 1 || count != 0 {
		t.Fatalf("partially committed import: %+v, repos=%d", state, count)
	}
}
