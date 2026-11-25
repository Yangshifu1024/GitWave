package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitwave.work/internal/ai"
)

func TestNativeAIAdviceUsesHealthAndSelectedReflogContext(t *testing.T) {
	store, workspace, _, git := commitAIStore(t)
	head := git("", "rev-parse", "HEAD")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []struct{ Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Messages) != 2 {
			t.Errorf("unexpected AI request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		system, user := input.Messages[0].Content, input.Messages[1].Content
		switch {
		case strings.HasPrefix(system, "Use the metrics only."):
			if !strings.Contains(user, `"branch_count"`) || !strings.Contains(user, `"dirty_files"`) ||
				strings.Contains(user, "staged-content-fixture") {
				t.Error("health prompt did not contain only the measured context")
			}
			_, _ = fmt.Fprint(w, `{"message":{"content":"Review the local health measurements."}}`)
		case strings.HasPrefix(system, "Explain the selected entry."):
			if !strings.Contains(user, strings.TrimSpace(head)) || !strings.Contains(user, "Commit AI fixture") ||
				strings.Contains(user, "staged-content-fixture") {
				t.Error("reflog prompt did not use the selected entry and commit subject")
			}
			text := "The selected entry created a commit.\n" + strings.Repeat("Review this recovery recommendation.\n", 40)
			_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]string{"content": text}})
		default:
			t.Error("prompt override was not used")
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	if err := store.SaveAIConfiguration(t.Context(), workspace.ID, ai.Configuration{
		Provider: ai.Ollama, BaseURL: server.URL, Offline: true,
		Templates: ai.Templates{Health: "Use the metrics only.", Reflog: "Explain the selected entry."},
	}); err != nil {
		t.Fatal(err)
	}
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	app.WaitFor("label:/^Commit AI fixture,/", timeout)
	app.Press("id:health-toggle")
	app.WaitFor("id:health-ai-summary", timeout)
	app.Press("id:health-ai-summary")
	app.WaitFor("id:health-ai-dialog", timeout)
	app.WaitFor("text:Review the local health measurements.", timeout)
	app.Press("id:health-ai-dialog >> label:Close")
	app.WaitForGone("id:health-ai-dialog", timeout)
	app.Press("id:health-toggle")
	app.Press("id:reflog-toggle")
	const initial = "label:/^Initial commit.*Commit AI fixture$/"
	app.WaitFor(initial, timeout)
	app.Press(initial)
	app.WaitFor("id:reflog-ai-explain", timeout)
	app.Press("id:reflog-ai-explain")
	app.WaitFor("id:reflog-ai-dialog", timeout)
	app.WaitFor("text:/^The selected entry created a commit/", timeout)
	const scroll = "id:reflog-ai-scroll"
	app.WaitUntil(timeout, func() bool {
		view := app.Find(scroll)
		return view.Bounds().Origin.Y+view.Frame.Size.Height <= 800 && view.Content.Height > view.Frame.Size.Height
	})
	app.Scroll(scroll, 100)
	app.WaitUntil(timeout, func() bool { return app.Find(scroll).Offset.Y >= 50 })
	if got := git("", "rev-parse", "HEAD"); got != head {
		t.Fatalf("AI advice changed HEAD to %s", got)
	}
}
