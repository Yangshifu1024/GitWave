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

func commitAIStore(t *testing.T) (*workspace.Store, workspace.Summary, string, func(string, ...string) string) {
	t.Helper()
	path, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Commit AI fixture")
	writeRepositoryFixtureFile(t, path, "staged.txt", "staged-content-fixture\n")
	git("", "add", "staged.txt")
	writeRepositoryFixtureFile(t, path, "staged.txt", "worktree-only-fixture\n")
	writeRepositoryFixtureFile(t, path, ".gitwave/AI.md", "Use short imperative messages.\n")
	store, err := workspace.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ws, err := store.CreateWithRepos(t.Context(), "Commit AI workspace", []string{path})
	if err != nil {
		t.Fatal(err)
	}
	return store, ws, path, git
}

func TestNativeCommitAISuggestsAnEditableDraft(t *testing.T) {
	store, ws, _, git := commitAIStore(t)
	head := git("", "rev-parse", "HEAD")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string
			Messages []struct{ Role, Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "commit-fixture" || len(body.Messages) != 2 {
			t.Errorf("generate request: model %q, messages %d, error %v", body.Model, len(body.Messages), err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		system, user := body.Messages[0].Content, body.Messages[1].Content
		if !strings.Contains(system, "Configured prompt.") || !strings.Contains(system, "Use short imperative messages.") ||
			!strings.Contains(system, "Japanese (日本語)") || !strings.Contains(user, "staged-content-fixture") ||
			strings.Contains(user, "worktree-only-fixture") {
			t.Error("request did not use the configured prompt, rules, language and staged context")
		}
		_, _ = fmt.Fprint(w, `{"message":{"content":"feat: add staged example"}}`)
	}))
	defer server.Close()
	if err := store.SaveAIConfiguration(t.Context(), ws.ID, ai.Configuration{
		Provider: ai.Ollama, Model: "commit-fixture", BaseURL: server.URL, Offline: true,
		Templates: ai.Templates{Commit: "Configured prompt."},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveDesktopPreference(t.Context(), workspace.AILanguagePreference, "ja"); err != nil {
		t.Fatal(err)
	}
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	app.WaitFor("label:/^Commit AI fixture,/", timeout)
	app.Press("id:navigate-changes")
	app.WaitFor("label:Staged · staged.txt", timeout)
	app.Type("id:commit-message", "my draft")
	app.Press("id:commit-ai-generate")
	app.WaitUntil(timeout, func() bool { return app.Find("id:commit-message").Text == "feat: add staged example" })
	app.Press("id:commit-ai-restore-draft")
	app.WaitUntil(timeout, func() bool { return app.Find("id:commit-message").Text == "my draft" })
	if got := git("", "rev-parse", "HEAD"); got != head {
		t.Fatalf("AI suggestion modified HEAD to %s", got)
	}
	if staged := git("", "show", ":staged.txt"); !strings.Contains(staged, "staged-content-fixture") {
		t.Fatal("AI suggestion modified the staged file")
	}
}

func TestNativeCommitAIOffersConfigurationForNewWorkspace(t *testing.T) {
	store, _, _, _ := commitAIStore(t)
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	app.WaitFor("label:/^Commit AI fixture,/", timeout)
	app.Press("id:navigate-changes")
	app.WaitFor("label:Staged · staged.txt", timeout)
	app.Press("id:commit-ai-generate")
	app.WaitFor("id:commit-ai-configure-dialog", timeout)
	app.Press("id:commit-ai-configure-dialog >> label:Configure")
	app.WaitFor("id:ai-settings-dialog", timeout)
	app.WaitFor("id:ai-provider", timeout)
}

func TestCommitAICancelsAndDiscardsSuggestionsWhenStagingChanges(t *testing.T) {
	if os.Getenv("NSGO_TEST_TARGET") != "" {
		t.Skip("The model coordinates the provider reply and Git mutations")
	}
	for _, action := range []string{"cancel", "restage"} {
		t.Run(action, func(t *testing.T) {
			store, ws, path, git := commitAIStore(t)
			started, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				var payload map[string]any
				_ = json.NewDecoder(req.Body).Decode(&payload)
				close(started)
				select {
				case <-req.Context().Done():
					close(canceled)
				case <-release:
					_, _ = fmt.Fprint(w, `{"message":{"content":"stale suggestion"}}`)
				}
			}))
			defer server.Close()
			// Release the server even if a test assertion fails before the response.
			released := false
			defer func() {
				if !released {
					close(release)
				}
			}()
			if err := store.SaveAIConfiguration(t.Context(), ws.ID, ai.Configuration{Provider: ai.Ollama, BaseURL: server.URL}); err != nil {
				t.Fatal(err)
			}
			app, _ := refreshApp(t, store, "")
			const timeout = 10 * time.Second
			app.WaitFor("label:/^Commit AI fixture,/", timeout)
			app.Press("id:navigate-changes")
			app.WaitFor("label:Staged · staged.txt", timeout)
			app.Type("id:commit-message", "original draft")
			app.Press("id:commit-ai-generate")
			app.WaitFor("id:commit-ai-cancel", timeout)
			select {
			case <-started:
			case <-time.After(timeout):
				t.Fatal("AI request never reached the server")
			}
			if action == "cancel" {
				app.Press("id:commit-ai-cancel")
				select {
				case <-canceled:
				case <-time.After(timeout):
					t.Fatal("provider did not observe cancellation")
				}
			} else {
				writeRepositoryFixtureFile(t, path, "staged.txt", "new staged content\n")
				git("", "add", "staged.txt")
				close(release)
				released = true
				app.WaitFor("id:commit-ai-error", timeout)
			}
			app.WaitFor("id:commit-ai-generate", timeout)
			if draft := app.Find("id:commit-message").Text; draft != "original draft" {
				t.Fatalf("original draft overwritten by %q", draft)
			}
		})
	}
}
