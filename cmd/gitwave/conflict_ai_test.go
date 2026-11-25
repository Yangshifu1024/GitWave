package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitwave.work/internal/ai"
	"gitwave.work/internal/workspace"
)

func TestNativeConflictAIExplainsIndexStagesWithoutEditing(t *testing.T) {
	path, git := historyRepository(t)
	commitVersion := func(value string) {
		writeRepositoryFixtureFile(t, path, "conflict.txt", value+"\n")
		git("", "add", "conflict.txt")
		git("", "commit", "-m", value)
	}
	writeRepositoryFixtureFile(t, path, ".gitwave/AI.md", "Compare the intent of the two sides.\n")
	git("", "add", ".gitwave/AI.md")
	commitVersion("Shared conflict base")
	git("", "switch", "-c", "topic")
	commitVersion("Topic conflict version")
	git("", "switch", "main")
	commitVersion("Main conflict version")
	head := git("", "rev-parse", "HEAD")
	store, err := workspace.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ws, err := store.CreateWithRepos(t.Context(), "Conflict AI workspace", []string{path})
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
		system, user := request.Messages[0].Content, request.Messages[1].Content
		if !strings.Contains(system, "Custom conflict prompt.") || !strings.Contains(system, "Compare the intent") ||
			!strings.Contains(user, "=== BASE ===\nShared conflict base") ||
			!strings.Contains(user, "=== OURS ===\nMain conflict version") ||
			!strings.Contains(user, "=== THEIRS ===\nTopic conflict version") || strings.Contains(user, "private editor note") {
			t.Error("request did not use only the configured rule and three conflict stages")
		}
		_, _ = fmt.Fprint(w, `{"message":{"content":"Keep the main version and review the topic change."}}`)
	}))
	defer server.Close()
	if err := store.SaveAIConfiguration(t.Context(), ws.ID, ai.Configuration{
		Provider: ai.Ollama, BaseURL: server.URL, Offline: true, Templates: ai.Templates{Conflict: "Custom conflict prompt."},
	}); err != nil {
		t.Fatal(err)
	}
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	app.WaitFor("label:topic", timeout)
	app.RightClick("label:topic")
	app.ChoosePopup("Merge into current")
	app.WaitFor("text:/May conflict in 1 file/", timeout)
	app.Press("id:confirm-ref-action")
	app.WaitFor("id:open-conflicts", timeout)
	app.Press("id:open-conflicts")
	app.WaitFor("label:conflict.txt", timeout)
	app.Press("label:conflict.txt")
	app.WaitFor("id:conflict-editor", timeout)
	app.Press("id:conflict-ours")
	app.Type("label:Resolved file content", " private editor note")
	app.Press("id:conflict-ai-explain")
	app.WaitFor("text:Keep the main version and review the topic change.", timeout)
	if git("", "rev-parse", "HEAD") != head || git("", "ls-files", "--unmerged", "conflict.txt") == "" ||
		!strings.Contains(app.Dump(), "private editor note") {
		t.Fatal("conflict explanation modified HEAD, the index or the editor draft")
	}
}
