package gitcli

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func lfsRead(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLFSAttributesPreserveOtherRules(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	initial := "\xef\xbb\xbf# user comment\r\n*.other filter=lfs-other\r\n*.label comment=filter=lfs\r\n" +
		"\"large assets/*.bin\"\tcustom\t-text\tfilter=lfs diff=lfs merge=lfs -text\tident\r\n" +
		"*.psd filter=lfs\r\n*.psd -filter\r\n*.png filter=lfs -text"
	writeFixture(t, dir, lfsAttributes, initial)
	status, err := repo.LFSStatus(t.Context())
	if err != nil || !slices.Equal(status.Patterns, []string{"large assets/*.bin", "*.png"}) {
		t.Fatalf("incorrect pattern list: %+v, %v", status, err)
	}
	if err := repo.TrackLFS(t.Context(), "large assets/*.bin"); err != nil {
		t.Fatal(err)
	}
	if got := lfsRead(t, dir, lfsAttributes); got != initial {
		t.Fatalf("tracking an existing rule rewrote attributes: %q", got)
	}
	for _, pattern := range []string{"#file.asset", "new assets/**", "*.psd"} {
		if err := repo.TrackLFS(t.Context(), pattern); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{"#file.asset", "new assets/deep/file", "picture.psd"} {
		if got := gitValue(t, dir, "check-attr", "filter", "--", path); !strings.HasSuffix(got, ": lfs") {
			t.Fatalf("Git did not recognize the rule for %q: %q", path, got)
		}
	}
	if err := repo.UntrackLFS(t.Context(), "large assets/*.bin"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UntrackLFS(t.Context(), "*.png"); err != nil {
		t.Fatal(err)
	}
	got := lfsRead(t, dir, lfsAttributes)
	if !strings.HasPrefix(got, "\xef\xbb\xbf# user comment\r\n*.other filter=lfs-other\r\n*.label comment=filter=lfs\r\n"+
		"\"large assets/*.bin\"\tcustom\t-text\tident\r\n*.psd filter=lfs\r\n*.psd -filter\r\n*.png -text\r\n") {
		t.Fatalf("untrack discarded independent attributes or line endings: %q", got)
	}
	if cached := gitValue(t, dir, "diff", "--cached", "--name-only"); cached != "" {
		t.Fatalf("editing LFS rules staged files: %q", cached)
	}
	for _, invalid := range []string{"!", "!*.bad", "dir/", "[attr]foo", "bad\nline", strings.Repeat("a", attributesLineLimit)} {
		if err := repo.TrackLFS(t.Context(), invalid); err == nil {
			t.Fatalf("accepted invalid pattern %q", invalid)
		}
	}
}

func TestLFSAttributesRecoverIndexFallbackAndRespectLocks(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	const attributes = "*.asset filter=lfs diff=lfs merge=lfs -text\n*.txt text\n"
	writeFixture(t, dir, lfsAttributes, attributes)
	fixtureGit(t, dir, "add", "--", lfsAttributes)
	if err := os.Remove(filepath.Join(dir, lfsAttributes)); err != nil {
		t.Fatal(err)
	}
	status, err := repo.LFSStatus(t.Context())
	if err != nil || !slices.Equal(status.Patterns, []string{"*.asset"}) {
		t.Fatalf("missing staged rules: %+v, %v", status, err)
	}
	if err := repo.UntrackLFS(t.Context(), "*.missing"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dir, lfsAttributes)); !os.IsNotExist(err) {
		t.Fatalf("a no-op created a working attributes file: %v", err)
	}
	writeFixture(t, dir, lfsAttributes+".lock", "external owner")
	if err := repo.TrackLFS(t.Context(), "*.bin"); err == nil {
		t.Fatal("modified an externally locked file")
	}
	if got := lfsRead(t, dir, lfsAttributes+".lock"); got != "external owner" {
		t.Fatalf("the external lock changed: %q", got)
	}
	if err := os.Remove(filepath.Join(dir, lfsAttributes+".lock")); err != nil {
		t.Fatal(err)
	}
	if err := repo.TrackLFS(t.Context(), "*.bin"); err != nil {
		t.Fatal(err)
	}
	if got := lfsRead(t, dir, lfsAttributes); got != attributes+"*.bin "+lfsAttributeValues+"\n" {
		t.Fatalf("tracking lost the staged fallback attributes: %q", got)
	}
	if got := fixtureGit(t, dir, "show", ":"+lfsAttributes); got != attributes {
		t.Fatalf("tracking rewrote the index: %q", got)
	}
}

func TestLFSAttributesDoNotFollowSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need additional privileges on Windows")
	}
	dir := newFixture(t)
	outside := t.TempDir()
	writeFixture(t, outside, "rules", "*.txt text\n")
	if err := os.Symlink(filepath.Join(outside, "rules"), filepath.Join(dir, lfsAttributes)); err != nil {
		t.Fatal(err)
	}
	repo := openFixture(t, dir)
	if err := repo.TrackLFS(t.Context(), "*.psd"); err == nil {
		t.Fatal("tracking accepted symlink attributes")
	}
	if got := lfsRead(t, outside, "rules"); got != "*.txt text\n" {
		t.Fatalf("modified an external file: %q", got)
	}
}

func requireLFSCLI(t *testing.T, repo *Repository) {
	t.Helper()
	status, err := repo.LFSStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Available {
		t.Skip("Git LFS CLI is unavailable")
	}
}

func TestLFSInstallFromLinkedWorktree(t *testing.T) {
	dir := newFixture(t)
	linked := filepath.Join(t.TempDir(), "linked")
	fixtureGit(t, dir, "worktree", "add", "--detach", "--", linked)
	main, child := openFixture(t, dir), openFixture(t, linked)
	requireLFSCLI(t, child)
	if installed, err := child.InstallLFS(t.Context()); err != nil || installed.ExternalHooks {
		t.Fatalf("failed to install in the common Git directory: %+v, %v", installed, err)
	}
	for _, repo := range []*Repository{main, child} {
		if status, err := repo.LFSStatus(t.Context()); err != nil || !status.Installed {
			t.Fatalf("%s cannot see the common filter config: %+v, %v", repo.WorkTree, status, err)
		}
	}
	for _, name := range lfsHookNames {
		if data := lfsRead(t, main.CommonDir, filepath.Join("hooks", name)); !strings.Contains(data, "git lfs "+name) {
			t.Fatalf("incorrect %s hook: %q", name, data)
		}
	}
	first := lfsRead(t, main.CommonDir, "config")
	if _, err := child.InstallLFS(t.Context()); err != nil {
		t.Fatal(err)
	}
	if second := lfsRead(t, main.CommonDir, "config"); first != second || !strings.Contains(second, "GitWave Test") {
		t.Fatalf("install changed independent Git configuration: %q", second)
	}
	if _, err := os.Lstat(filepath.Join(child.GitDir, "config")); !os.IsNotExist(err) {
		t.Fatalf("isolated linked checkout config was created: %v", err)
	}
}

func TestLFSInstallPreservesCustomHooksAndFiltersOnFailure(t *testing.T) {
	for _, conflict := range []string{"hook", "filter"} {
		t.Run(conflict, func(t *testing.T) {
			dir := newFixture(t)
			repo := openFixture(t, dir)
			requireLFSCLI(t, repo)
			hooks := filepath.Join(repo.CommonDir, "hooks")
			if err := os.Mkdir(hooks, 0o755); err != nil {
				t.Fatal(err)
			}
			const hook = "#!/bin/sh\nexit 0\n"
			if conflict == "hook" {
				writeFixture(t, hooks, "pre-push", hook)
			} else {
				fixtureGit(t, dir, "config", "filter.lfs.smudge", "custom-lfs-filter")
			}
			before := lfsRead(t, repo.CommonDir, "config")
			if _, err := repo.InstallLFS(t.Context()); err == nil {
				t.Fatal("expected Git LFS to reject the existing custom configuration")
			}
			if got := lfsRead(t, repo.CommonDir, "config"); got != before {
				t.Fatalf("failed install published partial filter configuration: %q", got)
			}
			if conflict == "hook" && lfsRead(t, hooks, "pre-push") != hook {
				t.Fatal("failed install replaced the existing hook")
			}
			if _, err := os.Lstat(filepath.Join(hooks, "post-commit")); !os.IsNotExist(err) {
				t.Fatalf("failed install published an unrelated hook: %v", err)
			}
		})
	}
}

func TestLFSInstallUsesLocalCustomHooksAndPreservesExternalDirectory(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	requireLFSCLI(t, repo)
	external := t.TempDir()
	writeFixture(t, external, "pre-push", "user's existing hook")
	fixtureGit(t, dir, "config", "core.hooksPath", external)
	result, err := repo.InstallLFS(t.Context())
	if err != nil || !result.ExternalHooks {
		t.Fatalf("external hooks should be preserved: %+v, %v", result, err)
	}
	if entries, err := os.ReadDir(external); err != nil || len(entries) != 1 || lfsRead(t, external, "pre-push") != "user's existing hook" {
		t.Fatalf("external hooks changed: %v, %v", entries, err)
	}
	fixtureGit(t, dir, "config", "core.hooksPath", "tools/hooks")
	result, err = repo.InstallLFS(t.Context())
	if err != nil || result.ExternalHooks {
		t.Fatalf("local hooks should be installed: %+v, %v", result, err)
	}
	if data := lfsRead(t, dir, filepath.Join("tools", "hooks", "pre-push")); !strings.Contains(data, "git lfs pre-push") {
		t.Fatalf("missing generated hook in repository-local custom directory: %q", data)
	}
}

func TestLFSInstallConfigAppliesAfterLocalIncludes(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	requireLFSCLI(t, repo)
	fixtureGit(t, dir, "config", "filter.lfs.process", "git-lfs filter-process")
	fixtureGit(t, dir, "config", "--add", "include.path", "local.inc")
	const included = "[filter \"lfs\"]\n\tprocess =\n"
	writeFixture(t, repo.CommonDir, "local.inc", included)
	if _, err := repo.InstallLFS(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := gitValue(t, dir, "config", "--local", "--includes", "--get", "filter.lfs.process"); got != "git-lfs filter-process" {
		t.Fatalf("the installed filter did not take precedence over the include: %q", got)
	}
	if got := lfsRead(t, repo.CommonDir, "local.inc"); got != included {
		t.Fatalf("the included Git config was edited: %q", got)
	}
}

func TestLFSStatusAndInstallRespectDisabledOrOverriddenFilters(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	requireLFSCLI(t, repo)
	if _, err := repo.InstallLFS(t.Context()); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "config", "filter.lfs.required", "false")
	if status, err := repo.LFSStatus(t.Context()); err != nil || status.Installed {
		t.Fatalf("a disabled filter was reported as installed: %+v, %v", status, err)
	}
	fixtureGit(t, dir, "config", "filter.lfs.required", "true")
	fixtureGit(t, dir, "config", "extensions.worktreeConfig", "true")
	fixtureGit(t, dir, "config", "--worktree", "filter.lfs.process", "user-lfs-filter")
	before := lfsRead(t, repo.CommonDir, "config")
	if _, err := repo.InstallLFS(t.Context()); err == nil || !strings.Contains(err.Error(), "worktree configuration") {
		t.Fatalf("installation accepted an incompatible worktree override: %v", err)
	}
	if got := lfsRead(t, repo.CommonDir, "config"); got != before {
		t.Fatalf("a rejected worktree override changed local config: %q", got)
	}
	if status, err := repo.LFSStatus(t.Context()); err != nil || status.Installed {
		t.Fatalf("an overridden filter was reported as installed: %+v, %v", status, err)
	}
}

func TestLFSInstallHonorsRepositoryLFSVersion(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	requireLFSCLI(t, repo)
	probe := newFixture(t)
	executable, err := repo.lfsExecutable(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.runLocalLFS(t.Context(), executable, probe, "install", "--local"); err != nil {
		t.Fatal(err)
	}
	result, err := fixtureCommand(t, probe, "config", "--local", "--get", lfsFormatKey).Output()
	if gitExitCode(err, 1) {
		t.Skip("this Git LFS version does not write a repository format version")
	}
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.InstallLFS(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got := fixtureGit(t, dir, "config", "--local", "--get", lfsFormatKey); got != string(result) {
		t.Fatalf("LFS format differs from the CLI: GitWave=%q, CLI=%q", got, result)
	}
	fixtureGit(t, dir, "config", lfsFormatKey, "9999")
	before := lfsRead(t, repo.CommonDir, "config")
	if _, err := repo.InstallLFS(t.Context()); err == nil {
		t.Fatal("LFS installation succeeded with an unsupported repository format")
	}
	if got := lfsRead(t, repo.CommonDir, "config"); got != before {
		t.Fatalf("format version validation changed config: %q", got)
	}
}

func TestLFSPublicationRestoresPreviouslyWrittenFiles(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires unprivileged Unix chmod")
	}
	var files []*lfsInstallFile
	for _, name := range []string{"pre-push", "config"} {
		dir := t.TempDir()
		writeFixture(t, dir, name, "original file")
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { root.Close() })
		lock, err := lockGitStorageFile(root, name)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(lock.close)
		before, err := readGitStorageSnapshot(root, name, lfsHookLimit)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, &lfsInstallFile{root: root, name: name, limit: lfsHookLimit, before: before,
			staged: gitStorageSnapshot{data: []byte("prepared file"), mode: before.mode}, lock: lock})
	}
	configRoot := files[1].root.Name()
	defer os.Chmod(configRoot, 0o700)
	err := publishLFSInstallation(files, func() error {
		return os.Chmod(configRoot, 0o500)
	})
	if err == nil {
		t.Fatal("the config unexpectedly published in a read-only directory")
	}
	for _, file := range files {
		if err := file.verify(); err != nil {
			t.Fatalf("file left changed after partial publication: %v", err)
		}
	}
}
