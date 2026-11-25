package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitwave.work/internal/ai"
	"gitwave.work/internal/workspace"
)

func TestNativeAICommitExplanationAndPullRequestDraft(t *testing.T) {
	path, git := historyRepository(t)
	writeRepositoryFixtureFile(t, path, "feature.txt", "Old committed content\n")
	git("", "add", "feature.txt")
	git("", "commit", "-m", "Base for native AI draft")
	git("", "switch", "-c", "topic")
	writeRepositoryFixtureFile(t, path, "feature.txt", "Selected committed content\n")
	git("", "add", "feature.txt")
	git("", "commit", "-m", "Topic commit for explanation")
	head := git("", "rev-parse", "HEAD")
	writeRepositoryFixtureFile(t, path, "feature.txt", "Local staged content\n")
	git("", "add", "feature.txt")
	writeRepositoryFixtureFile(t, path, ".gitwave/AI.md", "Describe the behavior change.\n")
	store, err := workspace.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	ws, err := store.CreateWithRepos(t.Context(), "AI history workspace", []string{path})
	if err != nil {
		t.Fatal(err)
	}
	var explanations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Messages []struct{ Content string } }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) != 2 {
			t.Errorf("invalid provider request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		system, user := request.Messages[0].Content, request.Messages[1].Content
		if !strings.Contains(system, "Describe the behavior change.") || !strings.Contains(user, "Topic commit for explanation") ||
			!strings.Contains(user, "+Selected committed content") || strings.Contains(user, "Local staged content") {
			t.Error("request did not use immutable commit data and repository rules")
		}
		switch {
		case strings.HasPrefix(system, "You explain Git commits"):
			_, _ = fmt.Fprintf(w, `{"message":{"content":"Commit explanation %d."}}`, explanations.Add(1))
		case strings.HasPrefix(system, "Use the PR template."):
			if !strings.Contains(user, "Branch: topic\nBase: main") {
				t.Error("PR request did not use the current branch and default base")
			}
			if strings.Contains(user, "+Latest topic content") {
				_, _ = fmt.Fprint(w, `{"message":{"content":"Refresh the topic draft\n\n## Summary\nDescribe the additional change."}}`)
			} else {
				_, _ = fmt.Fprint(w, `{"message":{"content":"Add the topic change\n\n## Summary\nDescribe the update."}}`)
			}
		default:
			t.Error("unexpected AI prompt")
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	if err := store.SaveAIConfiguration(t.Context(), ws.ID, ai.Configuration{
		Provider: ai.Ollama, BaseURL: server.URL, Offline: true, Templates: ai.Templates{PR: "Use the PR template."},
	}); err != nil {
		t.Fatal(err)
	}
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	app.WaitFor("label:/^Topic commit for explanation,/", timeout)
	app.Press("label:/^Topic commit for explanation,/")
	app.WaitFor("id:commit-ai-explain", timeout)
	app.Press("id:commit-ai-explain")
	app.WaitFor("text:Commit explanation 1.", timeout)
	app.Press("id:ai-draft-regenerate")
	app.WaitFor("text:Commit explanation 2.", timeout)
	if os.Getenv("NSGO_TEST_TARGET") == "" {
		app.Press("id:ai-explain-copy")
		if app.Clipboard != "Commit explanation 2." {
			t.Fatalf("clipboard = %q", app.Clipboard)
		}
	}
	app.Press("id:ai-draft-dialog >> label:Close")
	app.WaitForGone("id:ai-draft-dialog", timeout)
	app.ChooseMenu("Branch", "PR")
	app.WaitFor("id:ai-pr-title", timeout)
	app.WaitUntil(timeout, func() bool { return app.Find("id:ai-pr-title").Text == "Add the topic change" })
	app.Clear("id:ai-pr-title")
	app.Type("id:ai-pr-title", "Edited title")
	if git("", "rev-parse", "HEAD") != head || !strings.Contains(git("", "show", ":feature.txt"), "Local staged content") {
		t.Fatal("the AI dialogs changed HEAD or the staged content")
	}
	if os.Getenv("NSGO_TEST_TARGET") == "" {
		app.Press("id:ai-pr-copy-all")
		app.WaitUntil(timeout, func() bool { return strings.HasPrefix(app.Clipboard, "Edited title\n\n## Summary") })
		copied := app.Clipboard
		app.Clear("id:ai-pr-title")
		app.Type("id:ai-pr-title", "Second edited title")
		writeRepositoryFixtureFile(t, path, "latest.txt", "Latest topic content\n")
		git("", "add", "latest.txt")
		git("", "commit", "--only", "-m", "Latest topic commit", "--", "latest.txt")
		app.Press("id:ai-pr-copy-all")
		app.WaitFor("id:ai-draft-error", timeout)
		if app.Clipboard != copied || app.Find("id:ai-pr-title").Text != "Second edited title" {
			t.Fatal("the stale draft altered the clipboard or lost the edited title")
		}
		app.Press("id:ai-draft-regenerate")
		app.WaitUntil(timeout, func() bool { return app.Find("id:ai-pr-title").Text == "Refresh the topic draft" })
		git("", "switch", "-c", "other")
		app.Press("id:ai-draft-regenerate")
		app.WaitFor("id:ai-draft-error", timeout)
		if !app.Find("id:ai-pr-copy-all").Props.Disabled {
			t.Fatal("a stale draft remained copyable after switching branches")
		}
	}
}
