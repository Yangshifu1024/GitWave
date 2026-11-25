package gitcli

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func writeLegacyRemote(t *testing.T, repo *Repository, group, name, contents string) string {
	t.Helper()
	dir := filepath.Join(repo.CommonDir, group)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLegacyRemoteMigratesFromLinkedWorktree(t *testing.T) {
	for _, format := range []string{"files", "reftable"} {
		t.Run(format, func(t *testing.T) {
			source := newFixture(t)
			dir := newFixture(t, "--ref-format="+format)
			linked := filepath.Join(t.TempDir(), "linked")
			fixtureGit(t, dir, "worktree", "add", "-b", "linked", linked)
			repo := openFixture(t, linked)
			oldFile := writeLegacyRemote(t, repo, "remotes", "archived", "URL: "+source+"\n"+
				"Push: HEAD:refs/heads/review\nPull: +refs/heads/*:refs/remotes/archived/*\n"+
				"Pull: ^refs/heads/private\n")
			fixtureGit(t, source, "branch", "private")
			entry := requireRemote(t, repo, "archived")
			if entry.FetchURL != source || !slices.Equal(entry.urls("push"), []string{"HEAD:refs/heads/review"}) {
				t.Fatalf("legacy remote metadata mismatch: %+v", entry)
			}
			if err := repo.AddRemote(t.Context(), "archived", source); err == nil {
				t.Fatal("accepted a name already owned by a legacy remote")
			}
			if _, err := repo.Fetch(t.Context(), FetchOptions{Remote: entry.Name, Expected: &entry}); err != nil {
				t.Fatal(err)
			}
			tip := gitValue(t, source, "rev-parse", "HEAD")
			if got := gitValue(t, linked, "rev-parse", "refs/remotes/archived/main"); got != tip {
				t.Fatalf("legacy fetch selected the wrong tip: %q", got)
			}
			if got := gitValue(t, linked, "for-each-ref", "--format=%(refname)", "refs/remotes/archived/private"); got != "" {
				t.Fatalf("legacy negative refspec was ignored: %q", got)
			}

			if err := repo.EditRemote(t.Context(), entry, "current", source, "/push.git"); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Lstat(oldFile); !os.IsNotExist(err) {
				t.Fatalf("legacy config remained after rename: %v", err)
			}
			entry = requireRemote(t, repo, "current")
			if want := []string{"+refs/heads/*:refs/remotes/current/*", "^refs/heads/private"}; !slices.Equal(entry.urls("fetch"), want) {
				t.Fatalf("migrated refspecs: %v, want %v", entry.urls("fetch"), want)
			}
			if entry.PushURL != "/push.git" || len(entry.urls("push")) != 1 {
				t.Fatalf("push metadata changed after migration: %+v", entry)
			}
			if got := gitValue(t, dir, "reflog", "show", "--format=%gs", "refs/remotes/current/main"); !strings.Contains(got, "fetch: archived") {
				t.Fatalf("legacy tracking reflog was not moved: %q", got)
			}
			if err := repo.RemoveRemote(t.Context(), entry); err != nil {
				t.Fatal(err)
			}
			if names, err := repo.RemoteNames(t.Context()); err != nil || len(names) != 0 {
				t.Fatalf("remote remained after removal: %v, %v", names, err)
			}
			fixtureGit(t, dir, "fsck", "--no-dangling")
		})
	}
}

func TestLegacyBranchesRemoteFetchAndPullProtectLocalCheckout(t *testing.T) {
	source := newFixture(t)
	client := t.TempDir()
	fixtureGit(t, client, "clone", "--quiet", "--", source, client)
	fixtureGit(t, client, "remote", "remove", "origin")
	fixtureGit(t, client, "config", "init.defaultBranch", "main")
	repo := openFixture(t, client)
	path := writeLegacyRemote(t, repo, "branches", "topic", source+"\n")
	remote := requireRemote(t, repo, "topic")
	if !slices.Equal(remote.urls("fetch"), []string{"refs/heads/main:refs/heads/topic"}) {
		t.Fatalf("incorrect default legacy branch mapping: %v", remote.urls("fetch"))
	}
	choices, err := repo.PullChoices(t.Context(), "main")
	if err != nil || len(choices.Remotes) != 1 || choices.Remotes[0].Name != "topic" ||
		!slices.Equal(choices.Remotes[0].Branches, []string{"main"}) {
		t.Fatalf("the explicit remote branch was not offered before the first fetch: %+v, %v", choices, err)
	}
	if _, err := repo.Fetch(t.Context(), FetchOptions{Remote: "topic"}); err != nil {
		t.Fatal(err)
	}
	base := gitValue(t, client, "rev-parse", "refs/heads/topic")
	linked := filepath.Join(t.TempDir(), "linked")
	fixtureGit(t, client, "worktree", "add", linked, "topic")
	fixtureGit(t, source, "commit", "--allow-empty", "-m", "Advance legacy branch target")
	newTip := gitValue(t, source, "rev-parse", "HEAD")
	if _, err := repo.Fetch(t.Context(), FetchOptions{Remote: "topic"}); err == nil ||
		!strings.Contains(err.Error(), "checked-out reference") {
		t.Fatalf("fetch into an attached branch was not blocked: %v", err)
	}
	if got := gitValue(t, linked, "rev-parse", "HEAD"); got != base {
		t.Fatalf("the linked worktree advanced without checkout: %q", got)
	}
	linkedRepo := openFixture(t, linked)
	plan, err := linkedRepo.PreparePull(t.Context(), PullOptions{
		Remote: "topic", Branch: "main", Local: CheckoutTarget{Ref: "refs/heads/topic", OID: base},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := plan.Apply(t.Context(), FetchOptions{}); err != nil || !result.Fetched || result.OID != newTip {
		t.Fatalf("pull could not integrate the legacy branch: %+v, %v", result, err)
	}

	if err := repo.RemoveRemote(t.Context(), requireRemote(t, repo, "topic")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("legacy file remained after deletion: %v", err)
	}
	if got := gitValue(t, linked, "rev-parse", "HEAD"); got != newTip {
		t.Fatalf("removing a legacy remote removed its local branch: %q", got)
	}
}

func TestLegacyRemoteRemoveRejectsAReplacedFile(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	path := writeLegacyRemote(t, repo, "remotes", "old", "URL: /old.git\n")
	remote := requireRemote(t, repo, "old")
	if err := os.WriteFile(path, []byte("URL: /new.git\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := repo.RemoveRemote(t.Context(), remote); err == nil {
		t.Fatal("removed a changed legacy remote")
	}
	if got := requireRemote(t, repo, "old").FetchURL; got != "/new.git" {
		t.Fatalf("the replacement remote was changed: %q", got)
	}
}

func TestLegacyRemoteMigrationKeepsMalformedRefspecOutOfPrimaryConfig(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	fixtureGit(t, dir, "remote", "add", "working", "/working.git")
	contents := "URL: /broken.git\nPull: refs/heads/main:refs/remotes/broken/*\n"
	path := writeLegacyRemote(t, repo, "remotes", "broken", contents)
	broken := requireRemote(t, repo, "broken")
	if err := repo.EditRemote(t.Context(), broken, "broken", "/new.git", ""); err == nil {
		t.Fatal("imported an invalid Git refspec")
	}
	if got := gitValue(t, dir, "remote"); got != "working" {
		t.Fatalf("changed valid remote configuration: %q", got)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != contents {
		t.Fatalf("changed the invalid source file: %q, %v", data, err)
	}
}

func TestLegacyRemoteKeepsRefspecsWhenURLComesFromBranches(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	remotesPath := writeLegacyRemote(t, repo, "remotes", "archive",
		"URL: /ignored.git\nURL:\nPull: +refs/heads/extra:refs/remotes/archive/extra\nPush: HEAD:refs/heads/extra\n")
	branchesPath := writeLegacyRemote(t, repo, "branches", "archive", "/origin.git#stable\n")
	fixtureGit(t, dir, "config", "--add", "remote.archive.url", "/also-ignored.git")
	fixtureGit(t, dir, "config", "--add", "remote.archive.url", "")
	remote := requireRemote(t, repo, "archive")
	url, err := fixtureCommand(t, dir, "remote", "get-url", "archive").Output()
	if err != nil || remote.FetchURL != "/origin.git" || remote.endpoint != strings.TrimSuffix(string(url), "\n") {
		t.Fatalf("incorrect URL fallback: %q (Git returned %q, %v)", remote.FetchURL, url, err)
	}
	if err := repo.EditRemote(t.Context(), remote, "archive", "/next.git", ""); err != nil {
		t.Fatal(err)
	}
	want := []string{"+refs/heads/extra:refs/remotes/archive/extra", "refs/heads/stable:refs/heads/archive"}
	if updated := requireRemote(t, repo, "archive"); !slices.Equal(updated.urls("fetch"), want) {
		t.Fatalf("migration lost a refspec: %v", updated.urls("fetch"))
	}
	for _, path := range []string{remotesPath, branchesPath} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("legacy file remained after migration: %s, %v", path, err)
		}
	}
}

func TestLegacyFetchLocksLinkedWorktreeBeforePublishingLocalBranch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a POSIX Git wrapper")
	}
	source := newFixture(t)
	client := t.TempDir()
	fixtureGit(t, client, "clone", "--quiet", "--", source, client)
	fixtureGit(t, client, "remote", "remove", "origin")
	repo := openFixture(t, client)
	writeLegacyRemote(t, repo, "branches", "topic", source+"#main\n")
	fixtureGit(t, client, "branch", "topic")
	linked := filepath.Join(t.TempDir(), "linked")
	fixtureGit(t, client, "worktree", "add", "-b", "other", linked)
	fixtureGit(t, source, "commit", "--allow-empty", "-m", "Advance topic")
	latest := gitValue(t, source, "rev-parse", "HEAD")
	output := filepath.Join(t.TempDir(), "attempt.log")
	wrapper := filepath.Join(t.TempDir(), "git")
	t.Setenv("GITWAVE_LEGACY_REAL_GIT", repo.git)
	t.Setenv("GITWAVE_LEGACY_LINKED", linked)
	t.Setenv("GITWAVE_LEGACY_SWITCH_LOG", output)
	if err := os.WriteFile(wrapper, []byte(`#!/bin/sh
for arg do
    if [ "$arg" = "fetch: topic" ]; then
        "$GITWAVE_LEGACY_REAL_GIT" -C "$GITWAVE_LEGACY_LINKED" switch topic >"$GITWAVE_LEGACY_SWITCH_LOG" 2>&1
        break
    fi
done
exec "$GITWAVE_LEGACY_REAL_GIT" "$@"
`), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(wrapper)+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := repo.Fetch(t.Context(), FetchOptions{Remote: "topic"}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(output); err != nil || !strings.Contains(string(got), "index.lock") {
		t.Fatalf("the concurrent switch was not blocked by the index lock: %s, %v", got, err)
	}
	if got := gitValue(t, linked, "symbolic-ref", "HEAD"); got != "refs/heads/other" {
		t.Fatalf("the linked checkout changed branches: %s", got)
	}
	if got := gitValue(t, client, "rev-parse", "refs/heads/topic"); got != latest {
		t.Fatalf("the inactive branch was not advanced: %s", got)
	}
}
