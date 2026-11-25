package gitcli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func checkoutFixture(t *testing.T) (string, *Repository, string, string) {
	t.Helper()
	dir := newFixture(t)
	base := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "switch", "-c", "topic")
	writeFixture(t, dir, "mixed.txt", "topic version\n")
	writeFixture(t, dir, ".gitattributes", "mixed.txt text eol=crlf\n")
	fixtureGit(t, dir, "add", ".")
	fixtureGit(t, dir, "commit", "-m", "Topic version")
	topic := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "switch", "main")
	return dir, openFixture(t, dir), base, topic
}

func TestCheckoutSwitchesBranchesAndDetachesAtAnnotatedTag(t *testing.T) {
	dir, repo, base, topic := checkoutFixture(t)
	fixtureGit(t, dir, "tag", "-a", "-m", "Release notes", "release", topic)
	tag := gitValue(t, dir, "rev-parse", "refs/tags/release")
	hooks := t.TempDir()
	for _, name := range []string{"post-checkout", "reference-transaction"} {
		if err := os.WriteFile(filepath.Join(hooks, name), []byte("#!/bin/sh\nexit 59\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	fixtureGit(t, dir, "config", "core.hooksPath", hooks)
	apply := func(target CheckoutTarget, expectedHEAD, expectedSymbolic string) {
		t.Helper()
		plan, err := repo.PrepareCheckout(t.Context(), target)
		if err != nil || plan.DirtyFiles != 0 || plan.Current {
			t.Fatalf("prepare: %+v, %v", plan, err)
		}
		if result, err := plan.Apply(t.Context()); err != nil || result.OID != expectedHEAD {
			t.Fatalf("checkout: %+v, %v", result, err)
		}
		status := readFixtureStatus(t, repo)
		if status.HeadOID != expectedHEAD || status.HeadRef != expectedSymbolic || len(status.Changes) != 0 {
			t.Fatalf("incorrect checkout status: %+v", status)
		}
	}
	apply(CheckoutTarget{Ref: "refs/heads/topic", OID: topic}, topic, "refs/heads/topic")
	data, err := os.ReadFile(filepath.Join(dir, "mixed.txt"))
	if err != nil || string(data) != "topic version\r\n" {
		t.Fatalf("checkout did not apply target attributes: %q, %v", data, err)
	}
	apply(CheckoutTarget{Ref: "refs/tags/release", OID: tag}, topic, "")
	apply(CheckoutTarget{Ref: "refs/heads/main", OID: base}, base, "refs/heads/main")
	if got := gitValue(t, dir, "reflog", "-1", "--format=%gs"); !strings.HasPrefix(got, "checkout: moving from ") {
		t.Fatalf("checkout missing from HEAD reflog: %q", got)
	}
}

func TestCheckoutRemoteCreatesTrackingAndReusesLocalBranch(t *testing.T) {
	dir, repo, base, topic := checkoutFixture(t)
	fixtureGit(t, dir, "remote", "add", "origin", "https://example.invalid/gitwave")
	fixtureGit(t, dir, "update-ref", "refs/remotes/origin/feature/remote", topic)
	target := CheckoutTarget{Ref: "refs/remotes/origin/feature/remote", OID: topic}
	plan, err := repo.PrepareCheckout(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.Apply(t.Context())
	if err != nil || !result.Created || result.Warning != "" || result.Name != "feature/remote" {
		t.Fatalf("remote switch: %+v, %v", result, err)
	}
	if got := gitValue(t, dir, "rev-parse", "--symbolic-full-name", "@{upstream}"); got != target.Ref {
		t.Fatalf("new branch tracks %q", got)
	}
	fixtureGit(t, dir, "commit", "--allow-empty", "-m", "Local work ahead of upstream")
	local := gitValue(t, dir, "rev-parse", "HEAD")
	main, err := repo.PrepareCheckout(t.Context(), CheckoutTarget{Ref: "refs/heads/main", OID: base})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := main.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	again, err := repo.PrepareCheckout(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	result, err = again.Apply(t.Context())
	if err != nil || result.Created || result.Warning != "" || result.OID != local {
		t.Fatalf("remote checkout overwrote the existing local branch: %+v, %v", result, err)
	}
}

func TestCheckoutPreservesNewEditsAndRejectsMovedRefs(t *testing.T) {
	dir, repo, base, topic := checkoutFixture(t)
	target := CheckoutTarget{Ref: "refs/heads/topic", OID: topic}
	plan, err := repo.PrepareCheckout(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "new.txt", "new edit")
	if _, err := plan.Apply(t.Context()); err == nil || !strings.Contains(err.Error(), "commit or stash") {
		t.Fatalf("checkout accepted edits added after preparation: %v", err)
	}
	if gitValue(t, dir, "rev-parse", "HEAD") != base {
		t.Fatal("failed checkout moved HEAD")
	}
	if data, err := os.ReadFile(filepath.Join(dir, "new.txt")); err != nil || string(data) != "new edit" {
		t.Fatalf("the new edit was lost: %q, %v", data, err)
	}
	if err := os.Remove(filepath.Join(dir, "new.txt")); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "update-ref", "refs/heads/topic", base)
	if _, err := plan.Apply(t.Context()); err == nil {
		t.Fatal("checkout silently accepted a moved destination")
	}
}

func TestCheckoutRejectsOverwritingIgnoredFileAndExternalFilter(t *testing.T) {
	dir, repo, _, _ := checkoutFixture(t)
	writeFixture(t, dir, ".gitignore", "reserved.txt\n")
	fixtureGit(t, dir, "add", ".gitignore")
	fixtureGit(t, dir, "commit", "-m", "Ignore local file")
	fixtureGit(t, dir, "switch", "topic")
	writeFixture(t, dir, "reserved.txt", "versioned\n")
	fixtureGit(t, dir, "add", "reserved.txt")
	fixtureGit(t, dir, "commit", "-m", "Track reserved file")
	topic := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "switch", "main")
	writeFixture(t, dir, "reserved.txt", "private ignored content\n")
	plan, err := repo.PrepareCheckout(t.Context(), CheckoutTarget{Ref: "refs/heads/topic", OID: topic})
	if err != nil || plan.DirtyFiles != 0 {
		t.Fatalf("prepare ignored collision: %+v, %v", plan, err)
	}
	if _, err := plan.Apply(t.Context()); err == nil {
		t.Fatal("checkout accepted an ignored file collision")
	}
	data, err := os.ReadFile(filepath.Join(dir, "reserved.txt"))
	if err != nil || string(data) != "private ignored content\n" {
		t.Fatalf("ignored content modified: %q, %v", data, err)
	}
	if err := os.Remove(filepath.Join(dir, "reserved.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo.CommonDir, "info"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo.CommonDir, "info", "attributes"), []byte("mixed.txt filter=external\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "config", "filter.external.smudge", "gitwave-must-not-execute-this")
	plan, err = repo.PrepareCheckout(t.Context(), CheckoutTarget{Ref: "refs/heads/topic", OID: topic})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(t.Context()); err == nil || !strings.Contains(err.Error(), "external Git filter") {
		t.Fatalf("checkout accepted external filter: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "reserved.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("filter refusal still updated the worktree: %v", err)
	}
}

func TestCheckoutMissingTargetBlobLeavesHeadIndexAndFilesAlone(t *testing.T) {
	dir, repo, base, topic := checkoutFixture(t)
	indexPath := filepath.Join(repo.GitDir, "index")
	before, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	missing := gitValue(t, dir, "rev-parse", "topic:mixed.txt")
	if err := os.Remove(filepath.Join(repo.CommonDir, "objects", missing[:2], missing[2:])); err != nil {
		t.Fatal(err)
	}
	plan, err := repo.PrepareCheckout(t.Context(), CheckoutTarget{Ref: "refs/heads/topic", OID: topic})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(t.Context()); err == nil {
		t.Fatal("checkout accepted a commit with a missing changed blob")
	}
	if got := gitValue(t, dir, "rev-parse", "HEAD"); got != base {
		t.Fatalf("checkout published a broken target: %q", got)
	}
	after, err := os.ReadFile(indexPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("checkout with a missing blob changed the index: %v", err)
	}
	actual, err := os.ReadFile(filepath.Join(dir, "mixed.txt"))
	if err != nil || string(actual) != "original version\n" {
		t.Fatalf("checkout with a missing blob changed the working file: %q %v", actual, err)
	}
}

func TestCheckoutRejectsUnrepresentableTargetPathAliases(t *testing.T) {
	dir := newFixture(t)
	base := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "update-index", "--add", "--cacheinfo", "100644,"+gitValue(t, dir, "rev-parse", "HEAD:mixed.txt")+",Report.txt")
	fixtureGit(t, dir, "update-index", "--add", "--cacheinfo", "100644,"+gitValue(t, dir, "rev-parse", "HEAD:conflict.txt")+",report.txt")
	tree := gitValue(t, dir, "write-tree")
	tip := gitValue(t, dir, "commit-tree", tree, "-p", base, "-m", "Two files with distinct cases")
	fixtureGit(t, dir, "reset", "--mixed", "HEAD")
	repo := openFixture(t, dir)
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	caseInsensitive, _, err := probeCheckoutNames(root)
	root.Close()
	if err != nil {
		t.Fatal(err)
	}
	plan, err := repo.PrepareCheckout(t.Context(), CheckoutTarget{OID: tip})
	if err != nil {
		t.Fatal(err)
	}
	_, err = plan.Apply(t.Context())
	if !caseInsensitive {
		if err != nil {
			t.Fatalf("case-sensitive worktree rejected distinct names: %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), "cannot represent both") {
		t.Fatalf("case-insensitive checkout accepted two path aliases: %v", err)
	}
	if got := gitValue(t, dir, "rev-parse", "HEAD"); got != base {
		t.Fatalf("unrepresentable tree moved HEAD to %q", got)
	}
}

func TestCheckoutDetachesFromUnbornBranch(t *testing.T) {
	dir := newFixture(t)
	tip := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "switch", "--orphan", "unborn")
	repo := openFixture(t, dir)
	plan, err := repo.PrepareCheckout(t.Context(), CheckoutTarget{OID: tip})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := gitValue(t, dir, "rev-parse", "HEAD"); got != tip {
		t.Fatalf("wrong detached checkout: %s", got)
	}
	if got := gitValue(t, dir, "branch", "--show-current"); got != "" {
		t.Fatalf("checkout from an unborn branch ended on %q", got)
	}
}

func TestCheckoutFinishesIfContextCancelsDuringWorktreeWrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a shell wrapper")
	}
	dir, repo, _, topic := checkoutFixture(t)
	scratch := t.TempDir()
	wrapper := filepath.Join(scratch, "git")
	enter := filepath.Join(scratch, "entered")
	proceed := filepath.Join(scratch, "continue")
	t.Setenv("GITWAVE_REAL_GIT", repo.git)
	t.Setenv("GITWAVE_CHECKOUT_ENTER", enter)
	t.Setenv("GITWAVE_CHECKOUT_PROCEED", proceed)
	if err := os.WriteFile(wrapper, []byte(`#!/bin/sh
for arg do
  if [ "$arg" = checkout ]; then
    : > "$GITWAVE_CHECKOUT_ENTER"
    while [ ! -f "$GITWAVE_CHECKOUT_PROCEED" ]; do sleep 0.01; done
    break
  fi
done
exec "$GITWAVE_REAL_GIT" "$@"
`), 0o700); err != nil {
		t.Fatal(err)
	}
	plan, err := repo.PrepareCheckout(t.Context(), CheckoutTarget{Ref: "refs/heads/topic", OID: topic})
	if err != nil {
		t.Fatal(err)
	}
	repo.git = wrapper
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	defer os.WriteFile(proceed, nil, 0o600)
	done := make(chan error, 1)
	go func() {
		_, err := plan.Apply(ctx)
		done <- err
	}()
	timeout := time.After(10 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, err := os.Stat(enter); err == nil {
			break
		}
		select {
		case err := <-done:
			t.Fatalf("checkout exited before worktree write: %v", err)
		case <-timeout:
			t.Fatal("checkout did not start writing files")
		case <-tick.C:
		}
	}
	cancel()
	if err := os.WriteFile(proceed, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("checkout did not finish publication after cancellation: %v", err)
		}
	case <-timeout:
		t.Fatal("checkout did not finish after releasing the wrapper")
	}
	if status := readFixtureStatus(t, repo); status.HeadOID != topic || len(status.Changes) != 0 {
		t.Fatalf("cancellation left an incomplete checkout: %+v", status)
	}
	if got := gitValue(t, dir, "branch", "--show-current"); got != "topic" {
		t.Fatalf("cancellation left checkout on %q", got)
	}
}

func TestCheckoutRestoresSourceAfterPrivateCheckoutReportsLateFailure(t *testing.T) {
	dir, repo, base, topic := checkoutFixture(t)
	plan, err := repo.PrepareCheckout(t.Context(), CheckoutTarget{Ref: "refs/heads/topic", OID: topic})
	if err != nil {
		t.Fatal(err)
	}
	simulateLateCheckoutFailure(t, repo)
	if _, err := plan.Apply(t.Context()); err == nil || !strings.Contains(err.Error(), "simulated late checkout failure") {
		t.Fatalf("late failure was not reported: %v", err)
	}
	status := readFixtureStatus(t, repo)
	if status.HeadOID != base || status.HeadRef != "refs/heads/main" || len(status.Changes) != 0 {
		t.Fatalf("checkout recovery left inconsistent source state: %+v", status)
	}
	got, err := os.ReadFile(filepath.Join(dir, "mixed.txt"))
	if err != nil || string(got) != "original version\n" {
		t.Fatalf("checkout recovery lost the original file: %q %v", got, err)
	}
}

func simulateLateCheckoutFailure(t *testing.T, repo *Repository) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a shell wrapper")
	}
	scratch := t.TempDir()
	wrapper := filepath.Join(scratch, "git")
	t.Setenv("GITWAVE_REAL_GIT", repo.git)
	t.Setenv("GITWAVE_CHECKOUT_FAILURE", filepath.Join(scratch, "reported"))
	if err := os.WriteFile(wrapper, []byte(`#!/bin/sh
for arg do
  if [ "$arg" = checkout ] && [ ! -f "$GITWAVE_CHECKOUT_FAILURE" ]; then
    : > "$GITWAVE_CHECKOUT_FAILURE"
    "$GITWAVE_REAL_GIT" "$@" || exit "$?"
    echo "simulated late checkout failure" >&2
    exit 91
  fi
done
exec "$GITWAVE_REAL_GIT" "$@"
`), 0o700); err != nil {
		t.Fatal(err)
	}
	repo.git = wrapper
}

func gitValue(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return strings.TrimSpace(fixtureGit(t, dir, args...))
}
