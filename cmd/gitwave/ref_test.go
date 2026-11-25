package main

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

const refTestTimeout = 5 * time.Second

func refTestApp(t *testing.T, repo string) *nsgotest.App {
	t.Helper()
	return nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.open(repo)
		return w.view()
	}, nsgotest.Size(1200, 800), nsgotest.Environment(core.Environment{
		OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en", ReduceMotion: true,
	}))
}

func TestNativeBranchCreateTrackingRenameAndDelete(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Base for branch")
	tip := git("", "rev-parse", "HEAD")
	git("", "remote", "add", "origin", "https://example.invalid/gitwave.git")
	git("", "update-ref", "refs/remotes/origin/main", tip)
	app := refTestApp(t, repo)
	app.WaitFor("label:/^Base for branch,/", refTestTimeout)
	app.RightClick("label:/^Base for branch,/")
	app.ChoosePopup("New Branch…")
	app.WaitFor("id:ref-dialog", refTestTimeout)
	app.Type("id:ref-name", "feature/topic")
	app.Press("id:confirm-ref-action")
	app.WaitForGone("id:ref-dialog", refTestTimeout)
	app.WaitFor("id:branch-folder >> text:feature", refTestTimeout)
	app.Press("id:branch-folder >> text:feature")
	app.WaitFor("label:feature/topic", refTestTimeout)
	if got := git("", "branch", "--show-current"); got != "main" {
		t.Fatalf("creating a branch changed checkout to %q", got)
	}

	app.Press("label:feature/topic")
	app.WaitUntil(refTestTimeout, func() bool { return app.Find("label:feature/topic").Props.Selected })
	app.Focus("label:feature/topic")
	app.Key("shift+f10")
	app.ChoosePopup("Tracking…")
	app.WaitFor("id:branch-upstream", refTestTimeout)
	app.Press("id:branch-upstream")
	app.WaitFor("id:branch-upstream-1", refTestTimeout)
	app.Press("id:branch-upstream-1")
	app.Press("id:confirm-ref-action")
	app.WaitForGone("id:ref-dialog", refTestTimeout)
	app.Press("label:feature/topic")
	app.WaitFor("label:feature/topic >> text:origin/main", refTestTimeout)

	app.RightClick("label:feature/topic")
	app.ChoosePopup("Rename…")
	app.WaitFor("id:ref-name", refTestTimeout)
	app.Clear("id:ref-name")
	app.Type("id:ref-name", "feature/renamed")
	app.Press("id:confirm-ref-action")
	app.WaitFor("label:feature/renamed", refTestTimeout)
	app.WaitForGone("label:feature/topic", refTestTimeout)
	if got := git("", "config", "branch.feature/renamed.merge"); got != "refs/heads/main" {
		t.Fatalf("rename lost tracking configuration: %q", got)
	}

	app.RightClick("label:feature/renamed")
	app.ChoosePopup("Delete Branch…")
	app.WaitFor("id:ref-dialog", refTestTimeout)
	app.WaitForGone("text:Reading tracking references…", refTestTimeout)
	app.Press("label:Cancel")
	app.WaitForGone("id:ref-dialog", refTestTimeout)
	app.WaitFor("label:feature/renamed", refTestTimeout)
	app.RightClick("label:feature/renamed")
	app.ChoosePopup("Delete Branch…")
	app.WaitFor("id:ref-dialog", refTestTimeout)
	app.WaitForGone("text:Reading tracking references…", refTestTimeout)
	app.Press("id:confirm-ref-action")
	app.WaitForGone("label:feature/renamed", refTestTimeout)
	if got := git("", "branch", "--format=%(refname:short)"); got != "main" {
		t.Fatalf("unexpected branches after confirmed deletion: %q", got)
	}
}

func TestNativeTagManagerConfirmsReplacementAndDeletion(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "First tag target")
	first := git("", "rev-parse", "HEAD")
	git("", "commit", "--allow-empty", "-m", "Second tag target")
	second := git("", "rev-parse", "HEAD")
	app := refTestApp(t, repo)
	app.WaitFor("label:/^First tag target,/", refTestTimeout)
	app.RightClick("label:/^First tag target,/")
	app.ChoosePopup("Tag…")
	app.WaitFor("id:ref-name", refTestTimeout)
	app.Type("id:ref-name", "v1.0")
	app.Type("id:tag-annotation", "Release notes")
	app.Press("id:confirm-ref-action")
	app.WaitForGone("id:ref-dialog", refTestTimeout)
	app.WaitUntil(refTestTimeout, func() bool { return app.Find("id:tags-toggle").Props.Role == core.RoleButton })
	if app.Exists("id:tags-list") {
		t.Fatal("the first tag should leave the Tags section collapsed")
	}
	if got := git("", "rev-parse", "refs/tags/v1.0^{commit}"); got != first {
		t.Fatalf("tag points to %q instead of the selected commit", got)
	}
	if got := git("", "for-each-ref", "--format=%(contents:subject)", "refs/tags/v1.0"); got != "Release notes" {
		t.Fatalf("tag annotation: %q", got)
	}

	app.Press("label:/^Second tag target,/")
	app.WaitFor("id:commit-more-actions", refTestTimeout)
	app.Press("id:commit-more-actions")
	app.ChoosePopup("Tag")
	app.WaitFor("id:ref-name", refTestTimeout)
	app.Type("id:ref-name", "v1.0")
	app.Press("id:confirm-ref-action")
	app.WaitFor("label:Replace tag", refTestTimeout)
	if got := git("", "rev-parse", "refs/tags/v1.0^{commit}"); got != first {
		t.Fatalf("unconfirmed replacement moved the tag to %q", got)
	}
	app.Press("label:Cancel")
	app.WaitFor("id:ref-name", refTestTimeout)
	if got := app.Find("id:ref-name").Text; got != "v1.0" {
		t.Fatalf("cancelling the replacement lost the tag name: %q", got)
	}
	app.Press("id:confirm-ref-action")
	app.WaitFor("label:Replace tag", refTestTimeout)
	app.Press("id:confirm-ref-action")
	app.WaitForGone("id:ref-dialog", refTestTimeout)
	if got := git("", "rev-parse", "refs/tags/v1.0"); got != second {
		t.Fatalf("confirmed replacement did not create a lightweight tag at the second commit: %q", got)
	}
	app.Press("id:tags-toggle")
	app.WaitFor("id:tag-row", refTestTimeout)
	app.RightClick("id:tag-row")
	app.ChoosePopup("Delete tag")
	app.WaitFor("id:ref-dialog", refTestTimeout)
	app.Press("id:confirm-ref-action")
	app.WaitForGone("id:tag-row", refTestTimeout)
	if got := git("", "tag", "--list"); got != "" {
		t.Fatalf("tag still exists after confirmed deletion: %q", got)
	}
	if got := git("", "rev-parse", "HEAD"); got != second {
		t.Fatalf("tag edits moved HEAD: %q", got)
	}
}
