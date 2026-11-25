package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeWorkbenchBranchShowsTrackingAndDetachedHead(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Common base")
	git("", "switch", "-c", "upstream")
	git("", "commit", "--allow-empty", "-m", "Remote-only change")
	tip := git("", "rev-parse", "HEAD")
	git("", "switch", "main")
	git("", "commit", "--allow-empty", "-m", "Local-only change")
	git("", "remote", "add", "origin", ".")
	git("", "config", "remote.origin.fetch", "+refs/heads/upstream:refs/remotes/origin/main")
	git("", "update-ref", "refs/remotes/origin/main", tip)
	git("", "branch", "--set-upstream-to=origin/main", "main")

	app := refTestApp(t, repo)
	const branch = "id:workbench-branch"
	app.WaitFor(branch+" >> text:→ origin/main", refTestTimeout)
	app.Find(branch + " >> text:↑1")
	app.Find(branch + " >> text:↓1")
	app.Resize(1024, 768)
	app.Press("label:Toggle navigator")
	app.Press("id:workbench-changes-mode")
	app.WaitFor(branch+" >> text:↑1", refTestTimeout)
	app.Press("label:Focus diff")
	app.WaitForGone("id:commit-composer", refTestTimeout)
	app.Press("id:workbench-changes-mode")
	app.WaitFor("id:commit-composer", refTestTimeout)
	app.Press("label:Focus diff")
	app.WaitForGone("id:commit-composer", refTestTimeout)
	app.Press("id:titlebar-changes")
	app.WaitFor("id:commit-composer", refTestTimeout)
	name := app.Find(branch + " >> text:/^main$/")
	if name.Frame.Size.Width <= 0 {
		t.Fatal("the branch name was clipped after hiding the navigator")
	}

	git("", "switch", "--detach", tip)
	app.Key("mod+r")
	app.WaitFor(branch+" >> text:detached @ "+shortSHA(tip), refTestTimeout)
	for _, stale := range []string{"→ origin/main", "↑1", "↓1"} {
		if app.Exists(branch + " >> text:" + stale) {
			t.Fatalf("the detached checkout retained %q", stale)
		}
	}
	app.Press("id:workbench-history-mode")
	app.Find(branch + " >> text:detached @ " + shortSHA(tip))
}

func TestNativeBranchSidebarGroupsAndRevealsCheckout(t *testing.T) {
	repo, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Common branch base")
	base := git("", "rev-parse", "HEAD")
	git("", "switch", "-c", "temporary")
	git("", "commit", "--allow-empty", "-m", "Upstream-only commit")
	upstream := git("", "rev-parse", "HEAD")
	git("", "switch", "main")
	git("", "branch", "-D", "temporary")
	git("", "commit", "--allow-empty", "-m", "Local-only commit")
	git("", "branch", "feature/today", base)
	git("", "branch", "feature/tomorrow", base)
	git("", "remote", "add", "origin", "https://example.invalid/gitwave.git")
	git("", "update-ref", "refs/remotes/origin/main", upstream)
	git("", "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	git("", "branch", "--set-upstream-to=origin/main", "main")
	remote := "team/shared/" + strings.Repeat("long-name-", 12) + "repository"
	git("", "remote", "add", remote, "https://example.invalid/gitwave.git")
	git("", "update-ref", "refs/remotes/"+remote+"/queue/next", base)

	app := refTestApp(t, repo)
	const main = "id:branch-sidebar-list >> label:main"
	app.WaitFor(main, refTestTimeout)
	app.Find("id:branch-group >> text:LOCAL")
	app.WaitFor(main+" >> text:↑1", refTestTimeout)
	app.WaitFor(main+" >> text:↓1", refTestTimeout)
	app.Find(main + " >> text:origin/main")
	app.WaitFor("id:pull >> text:(1)", refTestTimeout)
	app.WaitFor("id:push >> text:(1)", refTestTimeout)
	app.Resize(1024, 768)
	app.Focus("id:resize-navigator")
	app.Key("left")
	app.Key("left")
	app.WaitForGone(main+" >> text:↑1", refTestTimeout)
	mainWidth := app.Find(main + " >> text:/^main$/").Frame.Size.Width
	if mainWidth <= 0 || !app.Exists(main+" >> text:HEAD") {
		t.Fatal("the narrow sidebar lost the current branch name or HEAD marker")
	}
	app.Find("id:pull >> text:(1)")
	app.Find("id:push >> text:(1)")
	app.Key("right")
	app.Key("right")
	app.Resize(1200, 800)
	app.WaitFor(main+" >> text:↑1", refTestTimeout)
	if app.Exists("id:branch-group >> text:ORIGIN") || app.Exists("label:feature/today") {
		t.Fatal("duplicated remote branches or a closed local folder are visible")
	}
	app.Focus("label:feature")
	app.Key("space")
	app.WaitFor("label:feature/today", refTestTimeout)
	app.Focus("label:feature/today")
	app.Key("space")
	app.WaitFor("id:commit-header >> text:Common branch base", refTestTimeout)
	if !app.Find("label:feature/today").Focused {
		t.Fatal("selecting a branch with the keyboard lost its focus")
	}
	if !app.Find("label:feature/today").Props.Selected || app.Find("label:feature/tomorrow").Props.Selected {
		t.Fatal("two branches at the same commit share a selection")
	}
	if branch := git("", "branch", "--show-current"); branch != "main" {
		t.Fatalf("single click changed the checked out branch to %q", branch)
	}
	app.Press("label:feature")
	app.WaitForGone("label:feature/today", refTestTimeout)
	group := "id:branch-sidebar-list >> label:" + remote
	groupView := app.Find(group)
	count := app.Find(group + " >> text:(1)").Frame
	if count.Origin.X+count.Size.Width > groupView.Frame.Size.Width {
		t.Fatalf("the long remote name hides its branch count: count %+v, group %+v", count, groupView.Frame)
	}
	app.Press(group)
	app.WaitFor("label:queue", refTestTimeout)
	app.Press("label:queue")
	app.WaitFor("label:"+remote+"/queue/next", refTestTimeout)
	app.Press(group)
	app.WaitForGone("label:"+remote+"/queue/next", refTestTimeout)

	app.Press("label:/^Common branch base,/ >> id:history-refs-more")
	app.WaitFor("id:history-refs-dialog", refTestTimeout)
	app.RightClick("id:history-refs-dialog >> label:Reference feature/today")
	app.ChoosePopup("Checkout")
	app.WaitFor("label:feature/today >> text:HEAD", refTestTimeout)
	if branch := git("", "branch", "--show-current"); branch != "feature/today" {
		t.Fatalf("checkout landed on %q", branch)
	}
	app.Screenshot("macos-branch-sidebar")
}

func TestBranchSidebarTrackingRefreshesAfterDeepening(t *testing.T) {
	source, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Common branch base")
	git("", "branch", "upstream")
	for range 4 {
		git("", "commit", "--allow-empty", "-m", "Local history")
	}
	git("", "switch", "upstream")
	for range 2 {
		git("", "commit", "--allow-empty", "-m", "Remote history")
	}
	git("", "switch", "main")
	checkout := filepath.Join(t.TempDir(), "checkout")
	git("", "clone", "--no-local", "--depth=1", "--no-single-branch", source, checkout)
	git("", "-C", checkout, "branch", "--set-upstream-to=origin/upstream", "main")
	app := refTestApp(t, checkout)
	app.WaitFor("label:main >> text:↑1", refTestTimeout)
	app.WaitFor("label:main >> text:↓1", refTestTimeout)
	// Deepening leaves both branch tips unchanged.
	git("", "-C", checkout, "fetch", "--unshallow")
	app.Key("mod+r")
	app.WaitFor("label:main >> text:↑4", refTestTimeout)
	app.WaitFor("label:main >> text:↓2", refTestTimeout)
}
