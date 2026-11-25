package gitcli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func requireRemote(t *testing.T, repo *Repository, name string) RemoteInfo {
	t.Helper()
	items, err := repo.Remotes(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Name == name {
			return item
		}
	}
	t.Fatalf("remote %q missing", name)
	return RemoteInfo{}
}

func TestRemoteConfigurationAddEditRenameAndRemove(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	base := gitValue(t, dir, "rev-parse", "HEAD")
	ctx := t.Context()
	if err := repo.AddRemote(ctx, "origin", "/fetch.git"); err != nil {
		t.Fatal(err)
	}
	if got := gitValue(t, dir, "config", "--get", "remote.origin.fetch"); got != "+refs/heads/*:refs/remotes/origin/*" {
		t.Fatalf("incorrect default refspec: %q", got)
	}
	fixtureGit(t, dir, "config", "--add", "remote.origin.url", "/secondary.git")
	fixtureGit(t, dir, "config", "remote.origin.prune", "true")
	fixtureGit(t, dir, "update-ref", "--create-reflog", "-m", "tracked before rename", "refs/remotes/origin/main", base)
	fixtureGit(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	fixtureGit(t, dir, "branch", "--set-upstream-to=origin/main", "main")
	fixtureGit(t, dir, "config", "branch.main.pushRemote", "origin")
	fixtureGit(t, dir, "config", "remote.pushDefault", "origin")
	item := requireRemote(t, repo, "origin")
	if err := repo.EditRemote(ctx, item, "origin", "/next-fetch.git", "/push.git"); err != nil {
		t.Fatal(err)
	}
	item = requireRemote(t, repo, "origin")
	if !slices.Equal(item.urls("url"), []string{"/next-fetch.git", "/secondary.git"}) || item.PushURL != "/push.git" {
		t.Fatalf("incorrect URLs: %+v", item)
	}
	if err := repo.EditRemote(ctx, item, "publish", "/next-fetch.git", ""); err != nil {
		t.Fatal(err)
	}
	item = requireRemote(t, repo, "publish")
	if item.PushURL != "" || gitValue(t, dir, "remote", "get-url", "--push", "publish") != item.FetchURL {
		t.Fatalf("push URL did not fall back to fetch: %+v", item)
	}
	configChecks := map[string]string{"branch.main.remote": "publish", "branch.main.pushRemote": "publish",
		"remote.pushDefault": "publish", "remote.publish.prune": "true"}
	for key, want := range configChecks {
		if got := gitValue(t, dir, "config", "--get", key); got != want {
			t.Fatalf("%s is %q, want %q", key, got, want)
		}
	}
	if got := gitValue(t, dir, "symbolic-ref", "refs/remotes/publish/HEAD"); got != "refs/remotes/publish/main" {
		t.Fatalf("incorrect remote HEAD: %q", got)
	}
	if got := gitValue(t, dir, "reflog", "show", "--format=%gs", "refs/remotes/publish/main"); !strings.Contains(got, "tracked before rename") {
		t.Fatalf("tracking reflog did not follow the renamed remote: %q", got)
	}
	if err := repo.RemoveRemote(ctx, item); err != nil {
		t.Fatal(err)
	}
	if names, err := repo.RemoteNames(ctx); err != nil || len(names) != 0 {
		t.Fatalf("remote was not removed: %+v, %v", names, err)
	}
	if refs := gitValue(t, dir, "for-each-ref", "--format=%(refname)", "refs/remotes"); refs != "" {
		t.Fatalf("unexpected tracking references: %q", refs)
	}
	if local := gitValue(t, dir, "rev-parse", "refs/heads/main"); local != base {
		t.Fatalf("local branch moved: %s", local)
	}
}

func TestRemoteConfigurationRejectsStaleSelectionAndConflictingName(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	ctx := t.Context()
	fixtureGit(t, dir, "remote", "add", "origin", "/old.git")
	fixtureGit(t, dir, "remote", "add", "upstream", "/upstream.git")
	origin := requireRemote(t, repo, "origin")
	if err := repo.EditRemote(ctx, origin, "upstream", "/should-not-be-written.git", ""); err == nil {
		t.Fatal("a conflicting name was accepted")
	}
	if got := requireRemote(t, repo, "origin").FetchURL; got != "/old.git" {
		t.Fatalf("the failed rename applied its URL: %q", got)
	}
	fixtureGit(t, dir, "config", "remote.origin.prune", "true")
	if err := repo.RemoveRemote(ctx, origin); err == nil || !strings.Contains(err.Error(), "changed since it was shown") {
		t.Fatalf("stale remove: %v", err)
	}
	if err := repo.EditRemote(ctx, origin, "origin", "/bad.git", ""); err == nil || !strings.Contains(err.Error(), "changed since it was shown") {
		t.Fatalf("stale edit: %v", err)
	}
	if _, err := repo.Fetch(ctx, FetchOptions{Remote: "origin", Expected: &origin}); err == nil ||
		!strings.Contains(err.Error(), "changed since it was shown") {
		t.Fatalf("stale fetch: %v", err)
	}
}

func TestRemoteConfigurationRenameRestoresConfigWhenTrackingRefIsLocked(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	base := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "remote", "add", "origin", "/old.git")
	fixtureGit(t, dir, "update-ref", "refs/remotes/origin/main", base)
	remote := requireRemote(t, repo, "origin")
	locked := filepath.Join(repo.CommonDir, "refs", "remotes", "origin", "main.lock")
	if err := os.WriteFile(locked, []byte("another Git process owns this lock"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo.CommonDir, "config")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.EditRemote(t.Context(), remote, "renamed", "/new.git", ""); err == nil {
		t.Fatal("rename unexpectedly finished")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("failed rename changed the config: %v", err)
	}
	if got := gitValue(t, dir, "rev-parse", "refs/remotes/origin/main"); got != base {
		t.Fatalf("failed rename changed the tracking tip: %s", got)
	}
	if data, err := os.ReadFile(locked); err != nil || string(data) != "another Git process owns this lock" {
		t.Fatalf("the other process lost its lock: %q, %v", data, err)
	}
}

func TestRemoteConfigurationProtectsConfigLockAndExistingPermissions(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	config := filepath.Join(repo.CommonDir, "config")
	if err := os.Chmod(config, 0o640); err != nil {
		t.Fatal(err)
	}
	lock := config + ".lock"
	if err := os.WriteFile(lock, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddRemote(t.Context(), "origin", "/remote.git"); err == nil {
		t.Fatal("adding a remote ignored the config lock")
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddRemote(t.Context(), "origin", "/remote.git"); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(config); err != nil || info.Mode().Perm() != 0o640 {
			t.Fatalf("config permissions changed: %v, %v", info, err)
		}
	}
}

func TestRemoteConfigurationRemoveKeepsSharedTrackingInSHA256Reftable(t *testing.T) {
	dir := newFixture(t, "--ref-format=reftable", "--object-format=sha256")
	repo := openFixture(t, dir)
	base := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "remote", "add", "upstream", "/upstream.git")
	fixtureGit(t, dir, "remote", "add", "origin", "/origin.git")
	fixtureGit(t, dir, "update-ref", "refs/remotes/origin/main", base)
	fixtureGit(t, dir, "update-ref", "refs/remotes/origin/exclusive", base)
	fixtureGit(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	fixtureGit(t, dir, "branch", "--set-upstream-to=origin/main", "main")
	fixtureGit(t, dir, "config", "--add", "remote.upstream.fetch", "+refs/heads/main:refs/remotes/origin/main")
	if items, err := repo.Remotes(t.Context()); err != nil || len(items) != 2 || items[0].Name != "upstream" {
		t.Fatalf("remote display order: %+v, %v", items, err)
	}
	origin := requireRemote(t, repo, "origin")
	if err := repo.RemoveRemote(t.Context(), origin); err != nil {
		t.Fatal(err)
	}
	if tip := gitValue(t, dir, "rev-parse", "refs/remotes/origin/main"); tip != base {
		t.Fatalf("changed a tracking reference shared with upstream: %s", tip)
	}
	if refs := gitValue(t, dir, "for-each-ref", "--format=%(refname)", "refs/remotes"); refs != "refs/remotes/origin/main" {
		t.Fatalf("exclusive references were not removed: %q", refs)
	}
	if tip := gitValue(t, dir, "rev-parse", "refs/heads/main"); tip != base {
		t.Fatalf("changed local main: %s", tip)
	}
	fixtureGit(t, dir, "fsck", "--no-dangling")
}

func TestRemoteConfigurationRemoveRetainsTrackingWhenRefIsLocked(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	base := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "remote", "add", "origin", "/origin.git")
	fixtureGit(t, dir, "update-ref", "refs/remotes/origin/main", base)
	fixtureGit(t, dir, "branch", "--set-upstream-to=origin/main", "main")
	fixtureGit(t, dir, "config", "remote.pushDefault", "origin")
	origin := requireRemote(t, repo, "origin")
	lock := filepath.Join(repo.CommonDir, "refs", "remotes", "origin", "main.lock")
	if err := os.WriteFile(lock, []byte("external"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := repo.RemoveRemote(t.Context(), origin); err == nil {
		t.Fatal("remove unexpectedly finished")
	}
	if got := requireRemote(t, repo, "origin"); !got.SameSelection(origin) {
		t.Fatalf("failed remove changed the remote: %+v", got)
	}
	if got := gitValue(t, dir, "config", "--get", "branch.main.remote"); got != "origin" {
		t.Fatalf("failed remove changed branch tracking: %q", got)
	}
	if got := gitValue(t, dir, "config", "--get", "remote.pushDefault"); got != "origin" {
		t.Fatalf("failed remove changed the push default: %q", got)
	}
	if data, err := os.ReadFile(lock); err != nil || string(data) != "external" {
		t.Fatalf("failed remove changed the external lock: %q, %v", data, err)
	}
}

func TestRemoteConfigurationRemovePreservesExcludedRefsAndLocalAliases(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	base := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "remote", "add", "origin", "/origin.git")
	fixtureGit(t, dir, "config", "--add", "remote.origin.fetch", "^refs/heads/excluded")
	for _, name := range []string{"excluded", "keep", "delete"} {
		fixtureGit(t, dir, "update-ref", "refs/remotes/origin/"+name, base)
	}
	fixtureGit(t, dir, "symbolic-ref", "refs/heads/saved", "refs/remotes/origin/keep")
	if err := repo.RemoveRemote(t.Context(), requireRemote(t, repo, "origin")); err != nil {
		t.Fatal(err)
	}
	if found := gitValue(t, dir, "for-each-ref", "--format=%(refname)", "refs/remotes"); found !=
		"refs/remotes/origin/excluded\nrefs/remotes/origin/keep" {
		t.Fatalf("incorrect refs remain: %q", found)
	}
	if tip := gitValue(t, dir, "rev-parse", "refs/heads/saved"); tip != base {
		t.Fatalf("a local symbolic branch lost its tip: %s", tip)
	}
}

func TestRemoteConfigurationRemoveProtectsIndirectCheckout(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	base := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "remote", "add", "origin", "/origin.git")
	fixtureGit(t, dir, "remote", "add", "other", "/other.git")
	fixtureGit(t, dir, "config", "remote.other.fetch", "+refs/heads/main:refs/remotes/origin/main")
	fixtureGit(t, dir, "update-ref", "refs/remotes/origin/main", base)
	fixtureGit(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	fixtureGit(t, dir, "symbolic-ref", "HEAD", "refs/remotes/origin/HEAD")
	if err := repo.RemoveRemote(t.Context(), requireRemote(t, repo, "origin")); err == nil ||
		!strings.Contains(err.Error(), "checked-out reference") {
		t.Fatalf("remove accepted a reference in the checkout's chain: %v", err)
	}
	if head := gitValue(t, dir, "rev-parse", "HEAD"); head != base {
		t.Fatalf("HEAD changed: %s", head)
	}
}

func TestRemoteConfigurationSelectedFetchRejectsURLRewriteChange(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	fixtureGit(t, dir, "remote", "add", "origin", "example:project")
	fixtureGit(t, dir, "config", "url./original/.insteadOf", "example:")
	shown := requireRemote(t, repo, "origin")
	if shown.endpoint != "/original/project" {
		t.Fatalf("incorrect rewritten endpoint: %q", shown.endpoint)
	}
	fixtureGit(t, dir, "config", "--remove-section", "url./original/")
	fixtureGit(t, dir, "config", "url./replacement/.insteadOf", "example:")
	if _, err := repo.Fetch(t.Context(), FetchOptions{Remote: shown.Name, Expected: &shown}); err == nil ||
		!strings.Contains(err.Error(), "changed since it was shown") {
		t.Fatalf("stale rewritten Fetch endpoint accepted: %v", err)
	}
}

func TestRemoteConfigurationRenameKeepsExternalSymbolicTargetsAndSharedRefs(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	base := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "remote", "add", "origin", "/origin.git")
	fixtureGit(t, dir, "remote", "add", "other", "/other.git")
	fixtureGit(t, dir, "config", "--add", "remote.other.fetch", "+refs/heads/*:refs/remotes/origin/*")
	fixtureGit(t, dir, "update-ref", "refs/remotes/origin/main", base)
	fixtureGit(t, dir, "symbolic-ref", "refs/remotes/origin/shortcut", "refs/heads/nonexistent")
	origin := requireRemote(t, repo, "origin")
	if err := repo.EditRemote(t.Context(), origin, "renamed", origin.FetchURL, ""); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"origin", "renamed"} {
		if tip := gitValue(t, dir, "rev-parse", "refs/remotes/"+name+"/main"); tip != base {
			t.Fatalf("incorrect tracking reference for %s: %s", name, tip)
		}
		if tip := gitValue(t, dir, "symbolic-ref", "--no-recurse", "refs/remotes/"+name+"/shortcut"); tip != "refs/heads/nonexistent" {
			t.Fatalf("incorrect symbolic target for %s: %s", name, tip)
		}
	}
}

func TestRemoteConfigurationRenameMovesReftableHistoryFromLinkedCheckout(t *testing.T) {
	for _, format := range []string{"sha1", "sha256"} {
		t.Run(format, func(t *testing.T) {
			dir := newFixture(t, "--ref-format=reftable", "--object-format="+format)
			base := gitValue(t, dir, "rev-parse", "HEAD")
			fixtureGit(t, dir, "remote", "add", "origin", "/origin.git")
			fixtureGit(t, dir, "remote", "add", "alternate", "/alternate.git")
			fixtureGit(t, dir, "config", "--add", "remote.alternate.fetch", "+refs/heads/shared:refs/remotes/origin/shared")
			fixtureGit(t, dir, "update-ref", "--create-reflog", "-m", "initial tracking history", "refs/remotes/origin/main", base)
			fixtureGit(t, dir, "update-ref", "--create-reflog", "-m", "shared history", "refs/remotes/origin/shared", base)
			fixtureGit(t, dir, "symbolic-ref", "refs/remotes/origin/shortcut", "refs/heads/nonexistent")
			fixtureGit(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
			fixtureGit(t, dir, "commit", "--allow-empty", "-m", "New GitWave commit")
			newer := gitValue(t, dir, "rev-parse", "HEAD")
			fixtureGit(t, dir, "-c", "user.name=Alternate Author", "-c", "user.email=author@example.invalid",
				"update-ref", "--create-reflog", "-m", "newer tracking history", "refs/remotes/origin/main", newer)
			linked := filepath.Join(t.TempDir(), "linked")
			fixtureGit(t, dir, "worktree", "add", "-b", "linked", linked)
			repo := openFixture(t, linked)
			const formatLine = "--format=%H %gn %ge %gs"
			previous := gitValue(t, dir, "reflog", "show", formatLine, "refs/remotes/origin/main")
			shared := gitValue(t, dir, "reflog", "show", formatLine, "refs/remotes/origin/shared")
			mainHistory := gitValue(t, dir, "reflog", "show", formatLine, "HEAD")

			origin := requireRemote(t, repo, "origin")
			if err := repo.EditRemote(t.Context(), origin, "renamed", "/new.git", "/push.git"); err != nil {
				t.Fatal(err)
			}
			newRemote := requireRemote(t, repo, "renamed")
			if newRemote.FetchURL != "/new.git" || newRemote.PushURL != "/push.git" {
				t.Fatalf("renamed URLs: %+v", newRemote)
			}
			moved := gitValue(t, dir, "reflog", "show", formatLine, "refs/remotes/renamed/main")
			if !strings.HasSuffix(moved, previous) {
				t.Fatalf("tracking reflog was not preserved:\nbefore: %s\nafter: %s", previous, moved)
			}
			if got := gitValue(t, dir, "reflog", "show", formatLine, "refs/remotes/origin/shared"); got != shared {
				t.Fatalf("shared source reflog changed: %s", got)
			}
			if got := gitValue(t, dir, "reflog", "show", formatLine, "refs/remotes/renamed/shared"); !strings.HasSuffix(got, shared) {
				t.Fatalf("shared tracking reflog was not copied: %s", got)
			}
			if got := gitValue(t, dir, "reflog", "show", formatLine, "HEAD"); got != mainHistory {
				t.Fatalf("main HEAD reflog changed: %s", got)
			}
			if got := gitValue(t, linked, "symbolic-ref", "HEAD"); got != "refs/heads/linked" {
				t.Fatalf("linked checkout changed: %s", got)
			}
			if got := gitValue(t, dir, "symbolic-ref", "--no-recurse", "refs/remotes/renamed/shortcut"); got != "refs/heads/nonexistent" {
				t.Fatalf("dangling alias changed: %s", got)
			}
			if got := gitValue(t, dir, "symbolic-ref", "--no-recurse", "refs/remotes/renamed/HEAD"); got != "refs/remotes/renamed/main" {
				t.Fatalf("remote HEAD changed: %s", got)
			}
			fixtureGit(t, dir, "update-ref", "refs/heads/nonexistent", base)
			fixtureGit(t, dir, "fsck", "--no-dangling")
		})
	}
}

func TestRemoteConfigurationRenameReadsIndexedReftableAndSplitsLogCopies(t *testing.T) {
	dir := newFixture(t, "--ref-format=reftable")
	repo := openFixture(t, dir)
	head := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "remote", "add", "origin", "/origin.git")
	fixtureGit(t, dir, "config", "reftable.blockSize", "256")
	var refs strings.Builder
	for i := range 320 {
		fmt.Fprintf(&refs, "create refs/tags/unchanged-%04d %s\n", i, head)
	}
	cmd := fixtureCommand(t, dir, "update-ref", "--stdin")
	cmd.Stdin = strings.NewReader(refs.String())
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create indexed reftable: %s, %v", output, err)
	}
	fixtureGit(t, dir, "commit", "--allow-empty", "-m", "Another commit")
	tips := []string{head, gitValue(t, dir, "rev-parse", "HEAD")}
	const count = 20
	for i := range count {
		message := fmt.Sprintf("op %d %s", i, strings.Repeat("r", 14<<10))
		cmd := fixtureCommand(t, dir, "-c", "reftable.blockSize=32768", "update-ref", "--create-reflog",
			"-m", message, "refs/remotes/origin/main", tips[i%len(tips)])
		cmd.Env = append(cmd.Env, "GIT_TEST_REFTABLE_AUTOCOMPACTION=0")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("write long reflog entry: %.1000s, %v", output, err)
		}
	}
	old := gitValue(t, dir, "reflog", "show", "--format=%gs", "refs/remotes/origin/main")

	if err := repo.EditRemote(t.Context(), requireRemote(t, repo, "origin"), "renamed", "/origin.git", ""); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			t.Fatalf("%v\n%s", err, exit.Stderr)
		}
		t.Fatal(err)
	}
	updated := gitValue(t, dir, "reflog", "show", "--format=%gs", "refs/remotes/renamed/main")
	if strings.Count(old, "\n") != count-1 || !strings.HasSuffix(updated, old) {
		t.Fatalf("the moved history differs, before %d bytes and after %d bytes", len(old), len(updated))
	}
	if got := gitValue(t, dir, "rev-parse", "refs/tags/unchanged-0319"); got != head {
		t.Fatalf("unrelated tag changed: %q", got)
	}
	fixtureGit(t, dir, "fsck", "--no-dangling")
}

func TestRemoteConfigurationRenameRestoresConfigAfterReftablePublishFailure(t *testing.T) {
	dir := newFixture(t, "--ref-format=reftable")
	repo := openFixture(t, dir)
	head := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "remote", "add", "origin", "/origin.git")
	fixtureGit(t, dir, "update-ref", "--create-reflog", "-m", "saved reflog", "refs/remotes/origin/main", head)
	origin := requireRemote(t, repo, "origin")
	common, err := os.OpenRoot(repo.CommonDir)
	if err != nil {
		t.Fatal(err)
	}
	defer common.Close()
	manifest, err := common.ReadFile(reftableListPath)
	if err != nil {
		t.Fatal(err)
	}
	before, mode, err := readGitStorageFile(common, "config", remoteConfigLimit)
	if err != nil {
		t.Fatal(err)
	}
	after := bytes.ReplaceAll(before, []byte("origin"), []byte("renamed"))
	configLock, err := lockGitStorageFile(common, "config")
	if err != nil {
		t.Fatal(err)
	}
	defer configLock.close()
	if err := configLock.prepare(after, mode); err != nil {
		t.Fatal(err)
	}
	stage := remoteConfigEdit{
		repo: repo, ctx: t.Context(),
		reftableRename: &remoteRenameRequest{origin, "refs/remotes/origin/", "refs/remotes/renamed/"},
	}
	err = stage.publishReftableRename(common, before, after, func() error {
		if err := configLock.publish(); err != nil {
			t.Fatal(err)
		}
		if err := common.Remove(reftableListPath + ".lock"); err != nil {
			t.Fatal(err)
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "lock") {
		t.Fatalf("expected a manifest lock failure, got %v", err)
	}
	if actual, err := common.ReadFile("config"); err != nil || !bytes.Equal(actual, before) {
		t.Fatalf("the original config was not restored: %v", err)
	}
	if actual, err := common.ReadFile(reftableListPath); err != nil || !bytes.Equal(actual, manifest) {
		t.Fatalf("the original manifest changed: %v", err)
	}
	tables, err := os.ReadDir(filepath.Join(repo.CommonDir, "reftable"))
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		if table.Name() != "tables.list" && !bytes.Contains(manifest, []byte(table.Name()+"\n")) {
			t.Errorf("unpublished table %q remained", table.Name())
		}
	}
	if got := gitValue(t, dir, "reflog", "show", "--format=%gs", "refs/remotes/origin/main"); got != "saved reflog" {
		t.Fatalf("source reflog changed: %q", got)
	}
	fixtureGit(t, dir, "fsck", "--no-dangling")
}

func TestRemoteConfigurationRenamePreservesRefClaimedThroughInclude(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a POSIX Git wrapper")
	}
	dir := newFixture(t, "--ref-format=reftable")
	repo := openFixture(t, dir)
	fixtureGit(t, dir, "remote", "add", "origin", "/origin.git")
	fixtureGit(t, dir, "remote", "add", "alternate", "/alternate.git")
	fixtureGit(t, dir, "update-ref", "--create-reflog", "-m", "source history", "refs/remotes/origin/main", "HEAD")
	include := filepath.Join(t.TempDir(), "remotes")
	if err := os.WriteFile(include, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "config", "include.path", include)
	origin := requireRemote(t, repo, "origin")

	wrapper := filepath.Join(t.TempDir(), "git")
	t.Setenv("GITWAVE_REMOTE_TEST_GIT", repo.git)
	t.Setenv("GITWAVE_REMOTE_TEST_INCLUDE", include)
	if err := os.WriteFile(wrapper, []byte(`#!/bin/sh
shadow=
for arg do
    case "$arg" in
        --git-dir=*gitwave-remote-reftable-*) shadow=yes ;;
        update-ref)
            if [ "$shadow" = yes ]; then
                "$GITWAVE_REMOTE_TEST_GIT" config --file "$GITWAVE_REMOTE_TEST_INCLUDE" --add remote.alternate.fetch '+refs/heads/main:refs/remotes/origin/main' || exit 1
            fi
            break ;;
    esac
done
exec "$GITWAVE_REMOTE_TEST_GIT" "$@"
`), 0o700); err != nil {
		t.Fatal(err)
	}
	repo.git = wrapper
	err := repo.EditRemote(t.Context(), origin, "renamed", "/origin.git", "")
	if err == nil || !strings.Contains(err.Error(), "effective Git config changed") {
		t.Fatalf("expected a stale remote configuration error, got %v", err)
	}
	if got := gitValue(t, dir, "config", "--file", include, "--get", "remote.alternate.fetch"); got != "+refs/heads/main:refs/remotes/origin/main" {
		t.Fatalf("external configuration was not preserved: %s", got)
	}
	if got := gitValue(t, dir, "reflog", "show", "--format=%gs", "refs/remotes/origin/main"); got != "source history" {
		t.Fatalf("source reflog changed: %s", got)
	}
	if got := gitValue(t, dir, "for-each-ref", "--format=%(refname)", "refs/remotes/renamed"); got != "" {
		t.Fatalf("renamed refs were published: %s", got)
	}
	if got := gitValue(t, dir, "remote", "get-url", "origin"); got != "/origin.git" {
		t.Fatalf("origin was changed: %s", got)
	}
}

func TestRemoteConfigurationRenameRestoresConfigAndLogWhenRefCommitAborts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a Git process wrapper")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("the Git process wrapper requires python3")
	}
	t.Run("modern", func(t *testing.T) { testRemoteRenameRefCommitFailure(t, false) })
	t.Run("legacy", func(t *testing.T) { testRemoteRenameRefCommitFailure(t, true) })
}

func testRemoteRenameRefCommitFailure(t *testing.T, legacy bool) {
	t.Helper()
	dir := newFixture(t)
	repo := openFixture(t, dir)
	base := gitValue(t, dir, "rev-parse", "HEAD")
	if legacy {
		writeLegacyRemote(t, repo, "remotes", "origin", "URL: /origin.git\nPull: +refs/heads/*:refs/remotes/origin/*\n")
	} else {
		fixtureGit(t, dir, "remote", "add", "origin", "/origin.git")
	}
	fixtureGit(t, dir, "update-ref", "--create-reflog", "-m", "previous history", "refs/remotes/origin/main", base)
	origin := requireRemote(t, repo, "origin")
	wrapper := filepath.Join(t.TempDir(), "git")
	t.Setenv("GITWAVE_REMOTE_TEST_GIT", repo.git)
	if err := os.WriteFile(wrapper, []byte(`#!/usr/bin/env python3
import os, subprocess, sys
args = [os.environ["GITWAVE_REMOTE_TEST_GIT"], *sys.argv[1:]]
if "update-ref" not in args or "remote: update tracking references" not in args:
    os.execv(args[0], args)
process = subprocess.Popen(args, stdin=subprocess.PIPE, stdout=subprocess.PIPE)
source = sys.stdin.buffer
payload = bytearray()
while not payload.endswith(b"prepare\0"):
    byte = source.read(1)
    if not byte:
        sys.exit(90)
    payload.extend(byte)
process.stdin.write(payload)
process.stdin.flush()
for _ in range(2):
    sys.stdout.buffer.write(process.stdout.readline())
    sys.stdout.buffer.flush()
if source.read(len(b"commit\0")) != b"commit\0":
    sys.exit(90)
process.stdin.write(b"abort\0")
process.stdin.close()
process.wait()
print("injected remote ref commit failure", file=sys.stderr)
sys.exit(91)
`), 0o700); err != nil {
		t.Fatal(err)
	}
	repo.git = wrapper
	if err := repo.EditRemote(t.Context(), origin, "renamed", "/new.git", "/push.git"); err == nil ||
		!strings.Contains(err.Error(), "injected remote ref commit failure") {
		t.Fatalf("expected an injected ref commit failure: %v", err)
	}
	if got := requireRemote(t, repo, "origin"); !got.SameSelection(origin) {
		t.Fatalf("the failed rename changed remote config: %+v", got)
	}
	if tips := gitValue(t, dir, "for-each-ref", "--format=%(refname)", "refs/remotes"); tips != "refs/remotes/origin/main" {
		t.Fatalf("the failed rename changed remote tracking refs: %q", tips)
	}
	if got := gitValue(t, dir, "reflog", "show", "--format=%gs", "refs/remotes/origin/main"); got != "previous history" {
		t.Fatalf("the failed rename changed previous history: %q", got)
	}
	log := filepath.Join(repo.CommonDir, "logs", "refs", "remotes", "renamed", "main")
	if _, err := os.Lstat(log); !os.IsNotExist(err) {
		t.Fatalf("the failed rename left a provisional reflog: %v", err)
	}
}
