package gitcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckoutDiscardClearsStagedAndUnstagedChanges(t *testing.T) {
	dir, repo, _, topic := checkoutFixture(t)
	writeFixture(t, dir, "mixed.txt", "staged\n")
	writeFixture(t, dir, "staged-new.txt", "staged addition\n")
	fixtureGit(t, dir, "add", "mixed.txt", "staged-new.txt")
	writeFixture(t, dir, "mixed.txt", "later worktree edit\n")
	writeFixture(t, dir, "untracked.txt", "untracked data\n")
	if err := os.Remove(filepath.Join(dir, "conflict.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo.CommonDir, "info"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, repo.CommonDir, "info/exclude", "private.txt\n")
	writeFixture(t, dir, "private.txt", "ignored data\n")

	plan, err := repo.PrepareCheckout(t.Context(), CheckoutTarget{Ref: "refs/heads/topic", OID: topic})
	if err != nil || plan.DirtyFiles != 4 {
		t.Fatalf("unexpected changes to discard: %+v, %v", plan, err)
	}
	if result, err := plan.ApplyDirty(t.Context(), CheckoutDiscard, "switch to topic"); err != nil || result.Warning != "" {
		t.Fatalf("discard and switch: %+v, %v", result, err)
	}
	status := readFixtureStatus(t, repo)
	if status.HeadOID != topic || status.HeadRef != "refs/heads/topic" || len(status.Changes) != 0 {
		t.Fatalf("unexpected status after discarding: %+v", status)
	}
	for name, want := range map[string]string{
		"mixed.txt": "topic version\r\n", "conflict.txt": "common base\n", "private.txt": "ignored data\n",
	} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s: got %q, want %q, error %v", name, got, want, err)
		}
	}
	for _, path := range []string{"staged-new.txt", "untracked.txt"} {
		if _, err := os.Lstat(filepath.Join(dir, path)); !os.IsNotExist(err) {
			t.Fatalf("discard left %s: %v", path, err)
		}
	}
	assertNoCheckoutRecovery(t, dir)
	if got := gitValue(t, dir, "stash", "list"); got != "" {
		t.Fatalf("discard left a visible stash: %q", got)
	}
}

func TestCheckoutDirtyConfirmationRejectsLaterSameSizeEdit(t *testing.T) {
	dir, repo, base, topic := checkoutFixture(t)
	writeFixture(t, dir, "untracked.txt", "before\n")
	plan, err := repo.PrepareCheckout(t.Context(), CheckoutTarget{OID: topic})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "untracked.txt")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "untracked.txt", "newest\n")
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []CheckoutMode{CheckoutDiscard, CheckoutStash} {
		if _, err := plan.ApplyDirty(t.Context(), mode, "switch"); err == nil || !strings.Contains(err.Error(), "changed") {
			t.Fatalf("mode %v accepted a stale confirmation: %v", mode, err)
		}
	}
	if head := gitValue(t, dir, "rev-parse", "HEAD"); head != base {
		t.Fatalf("the stale request moved HEAD to %s", head)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "newest\n" {
		t.Fatalf("the later edit was lost: %q %v", got, err)
	}
	assertNoCheckoutRecovery(t, dir)
}

func TestCheckoutDirtyRejectsIgnoredCollisionBeforeClearingChanges(t *testing.T) {
	dir, repo, base, _ := checkoutFixture(t)
	if err := os.MkdirAll(filepath.Join(repo.CommonDir, "info"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, repo.CommonDir, "info/exclude", "private.txt\n")
	fixtureGit(t, dir, "switch", "topic")
	writeFixture(t, dir, "private.txt", "tracked on topic\n")
	fixtureGit(t, dir, "add", "-f", "private.txt")
	fixtureGit(t, dir, "commit", "-m", "Track the private filename")
	topic := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "switch", "main")
	writeFixture(t, dir, "private.txt", "ignored personal data\n")
	writeFixture(t, dir, "mixed.txt", "staged work\n")
	fixtureGit(t, dir, "add", "mixed.txt")
	writeFixture(t, dir, "mixed.txt", "additional work\n")
	writeFixture(t, dir, "untracked.txt", "new file\n")
	plan, err := repo.PrepareCheckout(t.Context(), CheckoutTarget{Ref: "refs/heads/topic", OID: topic})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.ApplyDirty(t.Context(), CheckoutDiscard, "switch"); err == nil || !strings.Contains(err.Error(), "ignored path") {
		t.Fatalf("expected refusal for an ignored collision: %v", err)
	}
	if gitValue(t, dir, "rev-parse", "HEAD") != base || gitValue(t, dir, "show", ":mixed.txt") != "staged work" {
		t.Fatal("checkout changed the source HEAD or index")
	}
	for file, want := range map[string]string{"private.txt": "ignored personal data\n", "mixed.txt": "additional work\n", "untracked.txt": "new file\n"} {
		if got, err := os.ReadFile(filepath.Join(dir, file)); err != nil || string(got) != want {
			t.Fatalf("%s changed after refusal: %q %v", file, got, err)
		}
	}
	assertNoCheckoutRecovery(t, dir)
	if got := gitValue(t, dir, "stash", "list"); got != "" {
		t.Fatalf("a preflight refusal added a stash: %q", got)
	}
}

func TestCheckoutDiscardRestoresSourceWithoutAttributesThatWillBeRemoved(t *testing.T) {
	dir, repo, _, topic := checkoutFixture(t)
	writeFixture(t, dir, ".gitattributes", "conflict.txt text eol=crlf\n")
	writeFixture(t, dir, "conflict.txt", "dirty contents\r\n")
	plan, err := repo.PrepareCheckout(t.Context(), CheckoutTarget{Ref: "refs/heads/topic", OID: topic})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.ApplyDirty(t.Context(), CheckoutDiscard, "switch"); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "conflict.txt")); err != nil || string(got) != "common base\n" {
		t.Fatalf("restored bytes used the discarded attributes: %q %v", got, err)
	}
	if status := readFixtureStatus(t, repo); status.HeadOID != topic || len(status.Changes) != 0 {
		t.Fatalf("checkout left changes after dropping worktree attributes: %+v", status)
	}
}

func TestCheckoutDiscardKeepsFilesThatBecomeUntracked(t *testing.T) {
	dir, repo, _, topic := checkoutFixture(t)
	writeFixture(t, dir, ".gitignore", "scratch/\n")
	if err := os.Mkdir(filepath.Join(dir, "scratch"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "scratch/private.txt", "private data\n")

	plan, err := repo.PrepareCheckout(t.Context(), CheckoutTarget{Ref: "refs/heads/topic", OID: topic})
	if err != nil || plan.DirtyFiles != 1 {
		t.Fatalf("only the untracked ignore file should need confirmation: %+v, %v", plan, err)
	}
	if _, err := plan.ApplyDirty(t.Context(), CheckoutDiscard, "switch"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Fatalf("discard kept the ignore file: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "scratch/private.txt")); err != nil || string(got) != "private data\n" {
		t.Fatalf("discard modified a previously ignored file: %q, %v", got, err)
	}
	status := readFixtureStatus(t, repo)
	if status.HeadOID != topic || len(status.Changes) != 1 || status.Changes[0].Kind != '?' ||
		status.Changes[0].Path != "scratch/private.txt" {
		t.Fatalf("unexpected status after discarding the ignore file: %+v", status)
	}
}

func TestCheckoutDirtyRestoresSourceAfterLateFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode CheckoutMode
	}{{"discard", CheckoutDiscard}, {"stash", CheckoutStash}} {
		t.Run(tc.name, func(t *testing.T) {
			dir, repo, base, topic := checkoutFixture(t)
			writeFixture(t, dir, "old name.txt", "staged contents\n")
			fixtureGit(t, dir, "add", "old name.txt")
			writeFixture(t, dir, "old name.txt", "later working contents\n")
			writeFixture(t, dir, "untracked.txt", "untracked contents\n")
			plan, err := repo.PrepareCheckout(t.Context(), CheckoutTarget{Ref: "refs/heads/topic", OID: topic})
			if err != nil {
				t.Fatal(err)
			}
			simulateLateCheckoutFailure(t, repo)
			if _, err := plan.ApplyDirty(t.Context(), tc.mode, "switch"); err == nil ||
				!strings.Contains(err.Error(), "simulated late checkout failure") {
				t.Fatalf("late failure was not reported: %v", err)
			}
			status := readFixtureStatus(t, repo)
			if status.HeadOID != base || status.HeadRef != "refs/heads/main" {
				t.Fatalf("checkout did not restore HEAD: %+v", status)
			}
			if got := gitValue(t, dir, "show", ":old name.txt"); got != "staged contents" {
				t.Fatalf("checkout changed the source index: %q", got)
			}
			for name, want := range map[string]string{
				"mixed.txt": "original version\n", "old name.txt": "later working contents\n", "untracked.txt": "untracked contents\n",
			} {
				if got, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(got) != want {
					t.Fatalf("recovery did not restore %s: %q, %v", name, got, err)
				}
			}
			assertNoCheckoutRecovery(t, dir)
			if got := gitValue(t, dir, "stash", "list"); got != "" {
				t.Fatalf("automatic recovery left an entry in the stash list: %q", got)
			}
		})
	}
}

func TestCheckoutStashReplaysTrackedAndUntrackedContentsAndLeavesOlderStashes(t *testing.T) {
	dir, repo, _, topic := checkoutFixture(t)
	writeFixture(t, dir, "older.txt", "older stash\n")
	fixtureGit(t, dir, "stash", "push", "-u", "-m", "older stash")
	older := gitValue(t, dir, "stash", "list", "--format=%H")
	writeFixture(t, dir, "old name.txt", "staged contents\n")
	fixtureGit(t, dir, "add", "old name.txt")
	writeFixture(t, dir, "old name.txt", "later working contents\n")
	writeFixture(t, dir, "untracked.txt", "untracked contents\n")

	plan, err := repo.PrepareCheckout(t.Context(), CheckoutTarget{Ref: "refs/heads/topic", OID: topic})
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.ApplyDirty(t.Context(), CheckoutStash, "switch to topic")
	if err != nil || result.Warning != "" || !result.Reapplied || result.OID != topic {
		t.Fatalf("stash and switch: %+v, %v", result, err)
	}
	for path, want := range map[string]string{"old name.txt": "later working contents\n", "untracked.txt": "untracked contents\n"} {
		got, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil || string(got) != want {
			t.Fatalf("%s: got %q, want %q, error %v", path, got, want, err)
		}
	}
	if got := gitValue(t, dir, "show", ":old name.txt"); got != "staged contents" {
		t.Fatalf("successful replay lost a different staged version: %q", got)
	}
	if list := gitValue(t, dir, "stash", "list", "--format=%H"); list != older {
		t.Fatalf("successful replay changed the existing stash list: %q want %q", list, older)
	}
	assertNoCheckoutRecovery(t, dir)
}

func TestCheckoutStashKeepsConflictedReplayAndAllStashParents(t *testing.T) {
	dir, repo, base, topic := checkoutFixture(t)
	writeFixture(t, dir, "old name.txt", "staged contents for replay\n")
	fixtureGit(t, dir, "add", "old name.txt")
	writeFixture(t, dir, "mixed.txt", "working contents for replay\n")
	writeFixture(t, dir, "untracked.txt", "untracked contents for replay\n")

	plan, err := repo.PrepareCheckout(t.Context(), CheckoutTarget{Ref: "refs/heads/topic", OID: topic})
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.ApplyDirty(t.Context(), CheckoutStash, "switch to topic")
	if err != nil || result.OID != topic || result.Reapplied || !strings.Contains(result.Warning, "stash list") {
		t.Fatalf("conflicted replay: %+v, %v", result, err)
	}
	if got := gitValue(t, dir, "rev-parse", "HEAD"); got != topic {
		t.Fatalf("conflicted replay left HEAD at %s", got)
	}
	if got := gitValue(t, dir, "ls-files", "--unmerged", "mixed.txt"); got == "" {
		t.Fatal("the real index is missing the replay conflict")
	}
	for key, want := range map[string]string{
		"stash@{0}^1":               base,
		"stash@{0}^2:old name.txt":  "staged contents for replay",
		"stash@{0}:mixed.txt":       "working contents for replay",
		"stash@{0}^3:untracked.txt": "untracked contents for replay",
	} {
		var got string
		if strings.ContainsRune(key, ':') {
			got = gitValue(t, dir, "show", key)
		} else {
			got = gitValue(t, dir, "rev-parse", key)
		}
		if got != want {
			t.Fatalf("%s: got %q, want %q", key, got, want)
		}
	}
	assertNoCheckoutRecovery(t, dir)
}

func TestCheckoutStashKeepsStagedVersionWhenIndexReplayIsBlocked(t *testing.T) {
	dir, repo, _, topic := checkoutFixture(t)
	writeFixture(t, dir, "mixed.txt", "a different staged version\n")
	fixtureGit(t, dir, "add", "mixed.txt")
	writeFixture(t, dir, "mixed.txt", "later working version\n")
	plan, err := repo.PrepareCheckout(t.Context(), CheckoutTarget{Ref: "refs/heads/topic", OID: topic})
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.ApplyDirty(t.Context(), CheckoutStash, "switch to topic")
	if err != nil || result.OID != topic || result.Reapplied || !strings.Contains(result.Warning, "stash list") {
		t.Fatalf("blocked index replay: %+v, %v", result, err)
	}
	if got := gitValue(t, dir, "show", "stash@{0}^2:mixed.txt"); got != "a different staged version" {
		t.Fatalf("stash lost the staged content: %q", got)
	}
	if got := gitValue(t, dir, "show", "stash@{0}:mixed.txt"); got != "later working version" {
		t.Fatalf("stash lost the working content: %q", got)
	}
}

func assertNoCheckoutRecovery(t *testing.T, dir string) {
	t.Helper()
	if got := gitValue(t, dir, "for-each-ref", "--format=%(refname)", "refs/gitwave/checkout-recovery"); got != "" {
		t.Fatalf("finished checkout kept a temporary recovery reference: %q", got)
	}
}
