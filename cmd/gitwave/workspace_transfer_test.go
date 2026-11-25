package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"gitwave.work/internal/workspace"
)

func TestNativeWorkspaceTransfer(t *testing.T) {
	a, gitA := historyRepository(t)
	b, gitB := historyRepository(t)
	gitA("", "commit", "--allow-empty", "-m", "First imported repository")
	gitB("", "commit", "--allow-empty", "-m", "Second imported repository")
	bare := filepath.Join(t.TempDir(), "mirror.git")
	gitB("", "clone", "--bare", b, bare)
	a, err := filepath.EvalSymlinks(a)
	if err != nil {
		t.Fatal(err)
	}
	b, err = filepath.EvalSymlinks(bare)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "state.db")
	store, err := workspace.Open(t.Context(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if err := store.Create(t.Context(), "Original workspace"); err != nil {
		t.Fatal(err)
	}
	brokenPath, transferPath := filepath.Join(dir, "broken.json"), filepath.Join(dir, "legacy.gitwave-workspace.json")
	if err := os.WriteFile(brokenPath, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	archive := struct {
		Version uint32   `json:"version"`
		Name    string   `json:"name"`
		Repos   []string `json:"repos"`
		APIKey  string   `json:"api_key"`
	}{1, "Imported workspace", []string{a, filepath.Join(dir, "missing"), a, b}, "never imported"}
	data, err := json.MarshalIndent(archive, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(transferPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	app := workspaceApp(t, store)
	const timeout = 5 * time.Second
	const input = "id:workspace-transfer-path"
	const dialog = "id:workspace-transfer-dialog"
	app.WaitFor("text:Original workspace", timeout)
	app.ChooseMenu("Workspace", "Import")
	app.WaitFor(input, timeout)
	setPath := func(path string) {
		app.Clear(input)
		app.Type(input, path)
		app.WaitUntil(timeout, func() bool { return app.Find(input).Text == path })
	}
	setPath(brokenPath)
	app.Press("label:Import")
	app.WaitFor("text:/invalid workspace transfer/", timeout)
	setPath(transferPath)
	app.Press("label:Import")
	app.WaitForGone(dialog, timeout)
	app.WaitFor("label:/First imported repository/", timeout)
	app.WaitFor("text:/Skipped 2 unavailable or duplicate paths/", timeout)
	app.Find("text:/^Imported workspace$/")
	state, err := store.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Workspaces) != 2 || state.Repo.Path != a || len(state.Repos) != 2 || state.Repos[1].Path != b {
		t.Fatalf("unexpected import selection or ordering: %+v", state)
	}
	app.Press("label:mirror.git")
	app.WaitFor("label:/Second imported repository/", timeout)

	// The export only replaces the existing file after confirmation.
	exportPath := filepath.Join(dir, "round-trip.json")
	if err := os.WriteFile(exportPath, []byte("keep until confirmed"), 0o600); err != nil {
		t.Fatal(err)
	}
	app.ChooseMenu("Workspace", "Export")
	app.WaitFor(input, timeout)
	if suggested := app.Find(input).Text; suggested != "Imported workspace.gitwave-workspace.json" {
		t.Fatalf("unexpected suggested filename %q", suggested)
	}
	setPath(exportPath)
	app.Press("label:Export")
	app.WaitFor("label:Replace", timeout)
	if raw, err := os.ReadFile(exportPath); err != nil || string(raw) != "keep until confirmed" {
		t.Fatalf("existing export changed before confirmation: %q, %v", raw, err)
	}
	app.Press("label:Cancel")
	app.WaitForGone(dialog, timeout)
	app.WaitForGone("text:/Replace the existing file/", timeout)
	app.ChooseMenu("Workspace", "Export")
	app.WaitFor(input, timeout)
	setPath(exportPath)
	app.Press("label:Export")
	app.WaitFor("label:Replace", timeout)
	app.Press("label:Replace")
	app.WaitForGone(dialog, timeout)
	app.WaitFor("text:/Workspace exported to/", timeout)
	exported, err := workspace.ReadTransfer(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	if exported.Name != archive.Name || !slices.Equal(exported.Repos, []string{a, b}) {
		t.Fatalf("export did not preserve imported workspace data: %+v", exported)
	}
	raw, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || len(fields) != 3 || fields["api_key"] != nil {
		t.Fatalf("private import metadata appeared in export: %s (%v)", raw, err)
	}
}
