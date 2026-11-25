package gitcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	sharedScopeMedia = "same cached media for every path\n"
	allowedScopePath = "Public/nested/keep.asset"
	blockedScopePath = "Public/nested/blocked.asset"
	outsideScopePath = "Private/outside.asset"
)

func scopeFixture(t *testing.T) (string, *Repository, CheckoutTarget, CheckoutTarget) {
	t.Helper()
	dir := newFixture(t)
	configureLFSFixture(t, dir)
	fixtureGit(t, dir, "config", "core.ignorecase", "true")
	writeFixture(t, dir, ".lfsconfig", "[lfs]\nfetchinclude = private/\n")
	fixtureGit(t, dir, "add", ".lfsconfig")
	fixtureGit(t, dir, "commit", "-m", "Source scope")
	base := gitValue(t, dir, "rev-parse", "HEAD")
	fixtureGit(t, dir, "branch", "blank")
	repo := openFixture(t, dir)
	writeFixture(t, dir, ".lfsconfig", "[lfs]\nfetchinclude = public/, uncached/**\nfetchexclude = **/blocked.asset\n")
	for _, path := range []string{allowedScopePath, blockedScopePath, outsideScopePath} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, filepath.FromSlash(path))), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFixture(t, dir, path, sharedScopeMedia)
	}
	status := readFixtureStatus(t, repo)
	if err := repo.SetStaged(t.Context(), status, status.Changes, true); err != nil {
		t.Fatal(err)
	}
	commit, err := repo.Commit(t.Context(), readFixtureStatus(t, repo), "Track cached assets", "")
	if err != nil {
		t.Fatal(err)
	}
	return dir, repo, CheckoutTarget{Ref: "refs/heads/blank", OID: base}, CheckoutTarget{Ref: "refs/heads/main", OID: commit.SHA}
}

func applyScopeCheckout(t *testing.T, repo *Repository, target CheckoutTarget) {
	t.Helper()
	plan, err := repo.PrepareCheckout(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestLFSCheckoutAppliesTargetScopeToEachPath(t *testing.T) {
	dir, repo, blank, assets := scopeFixture(t)
	pointer := fixtureGit(t, dir, "show", assets.OID+":"+blockedScopePath)
	applyScopeCheckout(t, repo, blank)
	applyScopeCheckout(t, repo, assets)
	for path, want := range map[string]string{
		allowedScopePath: sharedScopeMedia, blockedScopePath: pointer, outsideScopePath: pointer,
	} {
		if got := lfsRead(t, dir, path); got != want {
			t.Fatalf("checkout %s = %q, want %q", path, got, want)
		}
	}
	t.Run("matches Git LFS smudge", func(t *testing.T) {
		if _, err := exec.LookPath("git-lfs"); err != nil {
			t.Skip("Git LFS is unavailable")
		}
		for _, path := range []string{allowedScopePath, blockedScopePath, outsideScopePath} {
			cmd := fixtureCommand(t, dir, "lfs", "smudge", "--", path)
			cmd.Stdin = strings.NewReader(pointer)
			data, err := cmd.Output()
			if err != nil || string(data) != lfsRead(t, dir, path) {
				t.Fatalf("Git LFS smudge %s = %q, %v", path, data, err)
			}
		}
	})
	applyScopeCheckout(t, repo, blank)
	fixtureGit(t, dir, "config", "lfs.fetchinclude", "")
	fixtureGit(t, dir, "config", "lfs.fetchexclude", "")
	applyScopeCheckout(t, repo, assets)
	for _, path := range []string{allowedScopePath, blockedScopePath, outsideScopePath} {
		if got := lfsRead(t, dir, path); got != sharedScopeMedia {
			t.Fatalf("empty local scope override for %s left %q", path, got)
		}
	}
}

func TestLFSCheckoutFallsBackToSourceRulesWhenRemovingLFSConfig(t *testing.T) {
	dir, repo, blank, _ := scopeFixture(t)
	fixtureGit(t, dir, "rm", ".lfsconfig")
	fixtureGit(t, dir, "commit", "-m", "Remove project rules")
	assets := CheckoutTarget{Ref: "refs/heads/main", OID: gitValue(t, dir, "rev-parse", "HEAD")}
	pointer := fixtureGit(t, dir, "show", assets.OID+":"+allowedScopePath)
	applyScopeCheckout(t, repo, blank)
	applyScopeCheckout(t, repo, assets)
	if got := lfsRead(t, dir, allowedScopePath); got != pointer {
		t.Fatalf("checkout ignored the source include rule for %s: %q", allowedScopePath, got)
	}
	if got := lfsRead(t, dir, outsideScopePath); got != sharedScopeMedia {
		t.Fatalf("checkout did not hydrate source-included path %s: %q", outsideScopePath, got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".lfsconfig")); !os.IsNotExist(err) {
		t.Fatalf("checkout did not remove .lfsconfig: %v", err)
	}
}

func TestLFSCheckoutUsesRestoredPolicyWhenCleaningDirtyConfig(t *testing.T) {
	for _, mode := range []struct {
		name  string
		value CheckoutMode
	}{{"discard", CheckoutDiscard}, {"stash", CheckoutStash}} {
		t.Run(mode.name, func(t *testing.T) {
			dir, repo, blank, assets := scopeFixture(t)
			committed := fixtureGit(t, dir, "show", assets.OID+":.lfsconfig")
			pointer := fixtureGit(t, dir, "show", assets.OID+":"+outsideScopePath)
			applyScopeCheckout(t, repo, blank)
			writeFixture(t, dir, ".lfsconfig", committed)
			fixtureGit(t, dir, "add", ".lfsconfig")
			fixtureGit(t, dir, "commit", "-m", "Use the eventual checkout rules")
			const draft = "[lfs]\nfetchinclude = private/\n"
			writeFixture(t, dir, ".lfsconfig", draft)
			plan, err := repo.PrepareCheckout(t.Context(), assets)
			if err != nil {
				t.Fatal(err)
			}
			if result, err := plan.ApplyDirty(t.Context(), mode.value, "local LFS policy"); err != nil ||
				mode.value == CheckoutStash && !result.Reapplied {
				t.Fatalf("clean and switch: %+v, %v", result, err)
			}
			if got := lfsRead(t, dir, allowedScopePath); got != sharedScopeMedia {
				t.Fatalf("checkout used the uncommitted rules: %q", got)
			}
			if got := lfsRead(t, dir, outsideScopePath); got != pointer {
				t.Fatalf("checkout hydrated a path allowed only by the draft: %q", got)
			}
			want := committed
			if mode.value == CheckoutStash {
				want = draft
			}
			if got := lfsRead(t, dir, ".lfsconfig"); got != want {
				t.Fatalf("configuration after %s = %q, want %q", mode.name, got, want)
			}
		})
	}
}

func TestLFSCheckoutKeepsDiscardedUnchangedAssetAsPointer(t *testing.T) {
	dir, repo, blank, assets := scopeFixture(t)
	applyScopeCheckout(t, repo, blank)
	applyScopeCheckout(t, repo, assets)
	pointer := fixtureGit(t, dir, "show", assets.OID+":"+blockedScopePath)
	fixtureGit(t, dir, "switch", "-c", "plain-update")
	writeFixture(t, dir, "ordinary.txt", "the target only changes this file\n")
	fixtureGit(t, dir, "add", "ordinary.txt")
	fixtureGit(t, dir, "commit", "-m", "Ordinary file change")
	target := CheckoutTarget{Ref: "refs/heads/plain-update", OID: gitValue(t, dir, "rev-parse", "HEAD")}
	applyScopeCheckout(t, repo, assets)
	writeFixture(t, dir, blockedScopePath, "discard this edit\n")
	plan, err := repo.PrepareCheckout(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.ApplyDirty(t.Context(), CheckoutDiscard, ""); err != nil {
		t.Fatal(err)
	}
	if got := lfsRead(t, dir, blockedScopePath); got != pointer {
		t.Fatalf("dirty checkout materialized an unchanged excluded path: %q", got)
	}
}

func TestLFSDiscardUsesRulesFromTheRestoredIndex(t *testing.T) {
	dir, repo, blank, assets := scopeFixture(t)
	applyScopeCheckout(t, repo, blank)
	applyScopeCheckout(t, repo, assets)
	pointer := fixtureGit(t, dir, "show", assets.OID+":"+blockedScopePath)
	writeFixture(t, dir, ".lfsconfig", "[lfs]\nfetchinclude = public/\n")
	writeFixture(t, dir, blockedScopePath, "discard this edit\n")
	status := readFixtureStatus(t, repo)
	discard, err := repo.PrepareDiscard(t.Context(), status, status.Changes)
	if err != nil {
		t.Fatal(err)
	}
	if err := discard.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := lfsRead(t, dir, blockedScopePath); got != pointer {
		t.Fatalf("discard materialized a path excluded by the restored rules: %q", got)
	}
}

func TestLFSCheckoutDoesNotReadUnusedProjectRules(t *testing.T) {
	dir := newFixture(t)
	configureLFSFixtureFilters(t, dir)
	fixtureGit(t, dir, "switch", "-c", "ordinary")
	writeFixture(t, dir, ".lfsconfig", "this config is invalid\n")
	writeFixture(t, dir, "ordinary.txt", "no LFS filters apply\n")
	fixtureGit(t, dir, "add", "--", ".lfsconfig", "ordinary.txt")
	fixtureGit(t, dir, "commit", "-m", "Ordinary changes")
	target := CheckoutTarget{Ref: "refs/heads/ordinary", OID: gitValue(t, dir, "rev-parse", "HEAD")}
	fixtureGit(t, dir, "switch", "main")
	applyScopeCheckout(t, openFixture(t, dir), target)
	if got := lfsRead(t, dir, "ordinary.txt"); got != "no LFS filters apply\n" {
		t.Fatalf("ordinary checkout did not write the file: %q", got)
	}
}

func TestLFSCheckoutUsesSourceRulesWhenTargetConfigIsSparse(t *testing.T) {
	dir, repo, blank, assets := scopeFixture(t)
	applyScopeCheckout(t, repo, blank)
	cmd := fixtureCommand(t, dir, "sparse-checkout", "set", "--no-cone", "--stdin")
	cmd.Stdin = strings.NewReader("/Public/\n/Private/\n/.gitattributes\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("configure sparse checkout: %s (%v)", out, err)
	}
	applyScopeCheckout(t, repo, assets)
	pointer := fixtureGit(t, dir, "show", assets.OID+":"+allowedScopePath)
	if got := lfsRead(t, dir, allowedScopePath); got != pointer {
		t.Fatalf("sparse checkout used the omitted project's rules: %q", got)
	}
	if got := lfsRead(t, dir, outsideScopePath); got != sharedScopeMedia {
		t.Fatalf("sparse checkout did not hydrate the source-included path: %q", got)
	}
}

func TestLFSCheckoutReadsProjectConfigCaseAlias(t *testing.T) {
	dir, repo, blank, assets := scopeFixture(t)
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	insensitive, _, err := probeCheckoutNames(root)
	root.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !insensitive {
		t.Skip("working filesystem is case-sensitive")
	}
	config := lfsRead(t, dir, ".lfsconfig")
	fixtureGit(t, dir, "rm", ".lfsconfig")
	writeFixture(t, dir, ".LFSCONFIG", config)
	fixtureGit(t, dir, "add", ".LFSCONFIG")
	fixtureGit(t, dir, "commit", "-m", "Restore the configuration with a case alias")
	assets.OID = gitValue(t, dir, "rev-parse", "HEAD")
	// Move to the other branch with the same private Git checkout used by the app.
	applyScopeCheckout(t, repo, blank)
	applyScopeCheckout(t, repo, assets)
	pointer := fixtureGit(t, dir, "show", assets.OID+":"+outsideScopePath)
	if got := lfsRead(t, dir, allowedScopePath); got != sharedScopeMedia {
		t.Fatalf("checkout did not load the case alias: %q", got)
	}
	if got := lfsRead(t, dir, outsideScopePath); got != pointer {
		t.Fatalf("checkout used the source rules instead of the case alias: %q", got)
	}
}

func TestLFSCheckoutDoesNotUseDiscardedCaseAliasConfig(t *testing.T) {
	dir := newFixture(t)
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	insensitive, _, err := probeCheckoutNames(root)
	root.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !insensitive {
		t.Skip("working filesystem is case-sensitive")
	}
	configureLFSFixture(t, dir)
	const stable = "[lfs]\nfetchinclude = public/\n"
	writeFixture(t, dir, ".LFSCONFIG", stable)
	fixtureGit(t, dir, "add", ".LFSCONFIG")
	fixtureGit(t, dir, "commit", "-m", "Set checkout policy via case alias")
	fixtureGit(t, dir, "branch", "blank")
	blank := CheckoutTarget{Ref: "refs/heads/blank", OID: gitValue(t, dir, "rev-parse", "HEAD")}
	repo := openFixture(t, dir)
	if err := os.Mkdir(filepath.Join(dir, "public"), 0o755); err != nil {
		t.Fatal(err)
	}
	const asset = "public/alias.asset"
	commitLFSPushAsset(t, dir, repo, asset, sharedScopeMedia)
	target := CheckoutTarget{Ref: "refs/heads/main", OID: gitValue(t, dir, "rev-parse", "HEAD")}
	applyScopeCheckout(t, repo, blank)
	writeFixture(t, dir, ".LFSCONFIG", "[lfs]\nfetchinclude = private/\n")
	plan, err := repo.PrepareCheckout(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.ApplyDirty(t.Context(), CheckoutDiscard, ""); err != nil {
		t.Fatal(err)
	}
	if got := lfsRead(t, dir, asset); got != sharedScopeMedia {
		t.Fatalf("checkout used the discarded case alias rules: %q", got)
	}
}

func TestLFSCheckoutRejectsChangedScopeBeforeWriting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a POSIX shell Git wrapper")
	}
	dir, repo, blank, assets := scopeFixture(t)
	applyScopeCheckout(t, repo, blank)
	temp := t.TempDir()
	wrapper, marker := filepath.Join(temp, "git"), filepath.Join(temp, "scope-edited")
	t.Setenv("GITWAVE_REAL_GIT", repo.git)
	t.Setenv("GITWAVE_SCOPE_CONFIG", filepath.Join(repo.CommonDir, "config"))
	t.Setenv("GITWAVE_SCOPE_MARKER", marker)
	const script = `#!/bin/sh
for arg do
  if [ "$arg" = checkout-index ] && [ ! -f "$GITWAVE_SCOPE_MARKER" ]; then
    "$GITWAVE_REAL_GIT" config --file "$GITWAVE_SCOPE_CONFIG" lfs.fetchinclude 'private/**' || exit 90
    touch "$GITWAVE_SCOPE_MARKER" || exit 91
    break
  fi
done
exec "$GITWAVE_REAL_GIT" "$@"
`
	if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	plan, err := repo.PrepareCheckout(t.Context(), assets)
	if err != nil {
		t.Fatal(err)
	}
	repo.git = wrapper
	if _, err := plan.Apply(t.Context()); err == nil || !strings.Contains(err.Error(), "path filters changed") {
		t.Fatalf("checkout after a scope change: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("fixture missed private checkout: %v", err)
	}
	if got := gitValue(t, dir, "rev-parse", "HEAD"); got != blank.OID {
		t.Fatalf("scope change moved HEAD to %s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, allowedScopePath)); !os.IsNotExist(err) {
		t.Fatalf("scope change touched the working copy: %v", err)
	}
}

func TestLFSCheckoutRollbackPreservesExistingPointersAndMedia(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a POSIX shell Git wrapper")
	}
	dir, repo, blank, assets := scopeFixture(t)
	applyScopeCheckout(t, repo, blank)
	applyScopeCheckout(t, repo, assets)
	pointer := fixtureGit(t, dir, "show", assets.OID+":"+blockedScopePath)
	scratch := t.TempDir()
	wrapper, marker := filepath.Join(scratch, "git"), filepath.Join(scratch, "checkout-failed")
	t.Setenv("GITWAVE_REAL_GIT", repo.git)
	t.Setenv("GITWAVE_CHECKOUT_FAILED", marker)
	const script = `#!/bin/sh
for arg do
  if [ "$arg" = checkout ] && [ ! -f "$GITWAVE_CHECKOUT_FAILED" ]; then
    "$GITWAVE_REAL_GIT" "$@" || exit 90
    touch "$GITWAVE_CHECKOUT_FAILED" || exit 91
    exit 92
  fi
done
exec "$GITWAVE_REAL_GIT" "$@"
`
	if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	plan, err := repo.PrepareCheckout(t.Context(), blank)
	if err != nil {
		t.Fatal(err)
	}
	repo.git = wrapper
	if _, err := plan.Apply(t.Context()); err == nil {
		t.Fatal("injected checkout failure was ignored")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("fixture missed live checkout: %v", err)
	}
	if got := gitValue(t, dir, "rev-parse", "HEAD"); got != assets.OID {
		t.Fatalf("rollback changed HEAD to %s", got)
	}
	if got := lfsRead(t, dir, allowedScopePath); got != sharedScopeMedia {
		t.Fatalf("rollback failed to restore hydrated content: %q", got)
	}
	if got := lfsRead(t, dir, blockedScopePath); got != pointer {
		t.Fatalf("rollback materialized a path that started as a pointer: %q", got)
	}
}
