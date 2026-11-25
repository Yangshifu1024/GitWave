package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitwave.work/internal/ai"
	"gitwave.work/internal/workspace"
)

func TestNativeCommandPaletteSearchAndSettings(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Needle first commit", "-m", "uniquely searchable body")
	oldest := git("", "rev-parse", "HEAD")
	for i := range 10 {
		git("", "commit", "--allow-empty", "-m", fmt.Sprintf("Needle commit %d", i+1))
	}
	git("", "commit", "--allow-empty", "-m", "Unrelated commit")
	app, _ := refreshApp(t, nil, repo)
	const timeout = 10 * time.Second
	app.WaitFor("label:/^Unrelated commit,/", timeout)
	app.Key("mod+k")
	app.WaitFor("id:palette-input", timeout)
	app.WaitFor("id:palette-row-fetch", timeout)
	app.Key("down")
	app.Key("enter")
	app.WaitFor("id:settings-dialog", timeout)
	app.Press("id:settings-dialog >> label:Close")
	app.WaitForGone("id:settings-dialog", timeout)
	app.Key("mod+k")
	app.WaitFor("id:palette-input", timeout)
	app.Type("id:palette-input", "Needle")
	app.WaitFor("id:palette-more", timeout)
	app.Scroll("id:palette-results", 24)
	offset := app.Find("id:palette-results").Offset.Y
	app.Press("id:palette-more")
	app.WaitForGone("id:palette-more", timeout)
	if after := app.Find("id:palette-results").Offset.Y; offset > 0 && after < offset-1 {
		t.Fatalf("pagination reset the scroll position from %v to %v", offset, after)
	}
	app.Press("id:palette-clear")
	app.WaitForGone("id:palette-clear", timeout)
	if input := app.Find("id:palette-input"); input.Text != "" || !input.Focused {
		t.Fatalf("clearing the search should empty and focus the input: %+v", input)
	}
	app.Type("id:palette-input", "uniquely searchable body")
	app.WaitFor("id:palette-row-"+oldest, timeout)
	app.Key("down")
	app.Key("enter")
	app.WaitForGone("id:command-palette", timeout)
	app.WaitFor("id:commit-header >> text:Needle first commit", timeout)
	app.WaitFor("label:/^Unrelated commit,/", timeout)
	app.Key("mod+k")
	app.WaitFor("id:palette-input", timeout)
	app.Key("mod+k")
	app.WaitForGone("id:command-palette", timeout)
	app.Key("mod+k")
	app.WaitFor("id:palette-input", timeout)
	git("", "commit", "--allow-empty", "-m", "New external commit")
	fresh := git("", "rev-parse", "HEAD")
	app.Type("id:palette-input", "new external commit")
	app.WaitFor("id:palette-row-"+fresh, timeout)
	app.Key("down")
	app.Key("enter")
	app.WaitForGone("id:command-palette", timeout)
	app.WaitFor("id:commit-header >> text:New external commit", timeout)
	app.WaitFor("label:/^New external commit,/", timeout)
}

func TestNativeCommandPaletteAIMutationsRequireConfirmation(t *testing.T) {
	path, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Rotate password temporarily")
	git("", "commit", "--allow-empty", "-m", "Palette base commit")
	base := git("", "rev-parse", "HEAD")
	git("", "branch", "existing")
	store, err := workspace.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ws, err := store.CreateWithRepos(t.Context(), "Palette workspace", []string{path})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Messages []struct{ Content string } }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) != 2 {
			t.Errorf("invalid provider request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		user := request.Messages[1].Content
		if !strings.Contains(user, "Palette base commit") || !strings.Contains(user, base) || !strings.Contains(user, "[REDACTED]") ||
			strings.Contains(user, path) || strings.Contains(user, "private worktree text") || strings.Contains(user, "Rotate password temporarily") {
			t.Error("provider request had unexpected repository data")
		}
		var intent string
		switch {
		case strings.HasSuffix(user, "create release branch"):
			intent = `{"action":"create_branch","params":{"name":"release"},"explanation":"Create a release branch."}`
		case strings.HasSuffix(user, "checkout existing branch"):
			intent = `{"action":"checkout_branch","params":{"name":"existing"}}`
		case strings.HasSuffix(user, "stash this work"):
			intent = `{"action":"stash_changes","params":{"message":"Palette stash"}}`
		case strings.HasSuffix(user, "add a version label"):
			intent = `{"action":"create_tag","params":{"name":"v1.0"}}`
		case strings.HasSuffix(user, "push the branch"):
			intent = `{"action":"push","params":{}}`
		case strings.HasSuffix(user, "create an invalid tag"):
			intent = `{"action":"create_tag","params":{"name":"v-invalid","sha":45}}`
		default:
			t.Error("unexpected AI prompt")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fmt.Fprintf(w, `{"message":{"content":%q}}`, intent)
	}))
	defer server.Close()
	if err := store.SaveAIConfiguration(t.Context(), ws.ID, ai.Configuration{Provider: ai.Ollama, BaseURL: server.URL, Offline: true}); err != nil {
		t.Fatal(err)
	}
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	app.WaitFor("label:/^Palette base commit,/", timeout)
	ask := func(query string) {
		t.Helper()
		if !app.Exists("id:command-palette") {
			app.Key("mod+k")
		}
		app.WaitFor("id:palette-input", timeout)
		app.Clear("id:palette-input")
		app.Type("id:palette-input", query)
		app.Press("id:palette-ask-ai")
	}
	ask("create release branch")
	app.WaitFor("id:palette-confirm", timeout)
	app.Press("id:palette-cancel-intent")
	app.WaitForGone("id:palette-intent", timeout)
	if git("", "branch", "--list", "release") != "" {
		t.Fatal("canceling the proposal created a branch")
	}
	ask("create release branch")
	app.WaitFor("id:palette-confirm", timeout)
	git("", "commit", "--allow-empty", "-m", "External commit after preview")
	app.Press("id:palette-confirm")
	app.WaitForGone("id:command-palette", timeout)
	app.WaitFor("text:Created branch release", timeout)
	if created := git("", "rev-parse", "refs/heads/release"); created != base {
		t.Fatalf("release branch = %s, want proposed %s", created, base)
	}
	app.WaitFor("label:/^External commit after preview,/", timeout)
	ask("add a version label")
	app.WaitFor("id:palette-confirm", timeout)
	if git("", "tag", "--list", "v1.0") != "" {
		t.Fatal("tag was created before confirmation")
	}
	app.Press("id:palette-confirm")
	app.WaitForGone("id:command-palette", timeout)
	app.WaitFor("text:/^Tagged .* as v1\\.0$/", timeout)
	if git("", "rev-parse", "refs/tags/v1.0") != git("", "rev-parse", "HEAD") {
		t.Fatal("tag did not use the proposed tip")
	}
	writeRepositoryFixtureFile(t, path, "local.txt", "private worktree text\n")
	ask("checkout existing branch")
	app.WaitFor("id:palette-confirm", timeout)
	app.Press("id:palette-confirm")
	app.WaitForGone("id:command-palette", timeout)
	app.WaitFor("text:/Working copy has 1 uncommitted file/", timeout)
	if git("", "branch", "--show-current") != "main" {
		t.Fatal("palette checked out a branch over a dirty worktree")
	}
	ask("stash this work")
	app.WaitFor("id:palette-confirm", timeout)
	if git("", "stash", "list") != "" {
		t.Fatal("stash was saved before confirmation")
	}
	app.Press("id:palette-confirm")
	app.WaitForGone("id:command-palette", timeout)
	app.WaitFor("text:Stash saved", timeout)
	if !strings.Contains(git("", "stash", "list"), "Palette stash") || git("", "status", "--porcelain") != "" {
		t.Fatal("confirmed stash did not save the untracked file")
	}
	ask("checkout existing branch")
	app.WaitFor("id:palette-confirm", timeout)
	git("", "commit", "--allow-empty", "-m", "External commit during checkout proposal")
	app.Press("id:palette-confirm")
	app.WaitFor("text:The checked-out branch or commit changed after the proposal.", timeout)
	if git("", "branch", "--show-current") != "main" {
		t.Fatal("an outdated checkout proposal changed the checked-out branch")
	}
	ask("checkout existing branch")
	app.WaitFor("id:palette-confirm", timeout)
	app.Press("id:palette-confirm")
	app.WaitFor("text:Checked out existing", timeout)
	if git("", "branch", "--show-current") != "existing" {
		t.Fatal("the up-to-date checkout proposal did not switch branches")
	}
	app.Press("id:custom-titlebar >> label:Changes")
	ask("stash this work")
	app.WaitFor("id:palette-confirm", timeout)
	git("", "switch", "-c", "external")
	writeRepositoryFixtureFile(t, path, "extra.txt", "another local change\n")
	app.Press("id:palette-confirm")
	app.WaitFor("text:/changed after the proposal. Ask again/", timeout)
	contents, err := os.ReadFile(filepath.Join(path, "extra.txt"))
	if app.Exists("id:palette-confirm") || app.Exists("id:stash-save-dialog") || git("", "branch", "--show-current") != "external" ||
		!strings.Contains(git("", "status", "--porcelain"), "?? extra.txt") ||
		git("", "stash", "list", "--format=%gs") != "On main: Palette stash" || err != nil || string(contents) != "another local change\n" {
		t.Fatal("a stale stash proposal moved files on a different branch")
	}
	ask("push the branch")
	app.WaitFor("id:palette-ai-error", timeout)
	if app.Exists("id:palette-confirm") {
		t.Fatal("a non-whitelisted proposal became executable")
	}
	ask("create an invalid tag")
	app.WaitFor("text:/must be a string/", timeout)
	if app.Exists("id:palette-confirm") || git("", "tag", "--list", "v-invalid") != "" {
		t.Fatal("an invalid base parameter was treated as HEAD")
	}
}
