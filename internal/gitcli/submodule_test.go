package gitcli

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func fixtureSubmodule(t *testing.T, parent, source, name, path string) {
	t.Helper()
	fixtureGit(t, parent, "-c", "protocol.file.allow=always", "submodule", "add", "--name", name, "--", source, path)
}

func requireSubmodule(t *testing.T, repo *Repository, name string) SubmoduleInfo {
	t.Helper()
	items, err := repo.Submodules(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Name == name {
			return item
		}
	}
	t.Fatalf("missing submodule %s among %+v", name, items)
	return SubmoduleInfo{}
}

func TestSubmoduleStatusInitAndDeinitPreserveCheckout(t *testing.T) {
	dir, source := newFixture(t), newFixture(t)
	const name, local = "logical.name", "deps/child repo"
	fixtureSubmodule(t, dir, source, name, local)
	repo := openFixture(t, dir)
	child := filepath.Join(dir, filepath.FromSlash(local))
	initial := requireSubmodule(t, repo, name)
	if initial.Path != local || !initial.InSync || !initial.Initialized || initial.Head != initial.IndexOID {
		t.Fatalf("incorrect original submodule state: %+v", initial)
	}
	writeFixture(t, child, "local note.txt", "Preserve my local file\n")
	if err := repo.DeinitSubmodule(t.Context(), initial); err != nil {
		t.Fatal(err)
	}
	after := requireSubmodule(t, repo, name)
	if after.Initialized || !after.InSync || after.Head != initial.Head {
		t.Fatalf("deinit changed more than registration: %+v", after)
	}
	if got := gitValue(t, dir, "config", "--file", ".gitmodules", "--get", "submodule."+name+".path"); got != local {
		t.Fatalf(".gitmodules path changed: %q", got)
	}
	path := filepath.Join(child, "local note.txt")
	if note, err := os.ReadFile(path); err != nil || string(note) != "Preserve my local file\n" {
		t.Fatalf("deinit removed a local file: %q, %v", note, err)
	}
	if got, err := repo.Submodules(t.Context()); err != nil || len(got) != 1 || got[0].Initialized {
		t.Fatalf("list registered the module: %+v, %v", got, err)
	}
	if err := repo.InitSubmodule(t.Context(), after); err != nil {
		t.Fatal(err)
	}
	if got := requireSubmodule(t, repo, name); !got.Initialized || !got.InSync || got.Head != initial.Head {
		t.Fatalf("incorrect registration after init: %+v", got)
	}
}

func TestSubmoduleInitUsesGitRelativeURLAndSelectedGitlink(t *testing.T) {
	dir, source := newFixture(t), newFixture(t)
	fixtureSubmodule(t, dir, source, "chosen", "deps chosen")
	fixtureSubmodule(t, dir, source, "other", "deps other")
	fixtureGit(t, dir, "config", "--file", ".gitmodules", "submodule.chosen.url", "../child.git")
	fixtureGit(t, dir, "config", "--remove-section", "submodule.chosen")
	fixtureGit(t, dir, "config", "--remove-section", "submodule.other")
	fixtureGit(t, dir, "remote", "add", "origin", "https://example.invalid/projects/super.git")
	repo := openFixture(t, dir)
	chosen := requireSubmodule(t, repo, "chosen")
	if err := repo.InitSubmodule(t.Context(), chosen); err != nil {
		t.Fatal(err)
	}
	if got := gitValue(t, dir, "config", "--local", "--get", "submodule.chosen.url"); got != "https://example.invalid/projects/child.git" {
		t.Fatalf("relative URL was not resolved by Git: %q", got)
	}
	if other := requireSubmodule(t, repo, "other"); other.Initialized {
		t.Fatal("initializing selected submodule registered another gitlink")
	}
	if err := repo.InitSubmodule(t.Context(), chosen); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("the stale selection was accepted: %v", err)
	}
}

func TestSubmoduleUpdateRecursesAndChecksOutIndexedCommits(t *testing.T) {
	outer, middle, leaf := newFixture(t), newFixture(t), newFixture(t)
	fixtureSubmodule(t, middle, leaf, "leaf", "deps/leaf")
	fixtureGit(t, middle, "commit", "-m", "Register nested submodule")
	fixtureSubmodule(t, outer, middle, "middle", "deps/middle")
	fixtureGit(t, outer, "commit", "-m", "Register middle submodule")
	client := t.TempDir()
	fixtureGit(t, client, "clone", "--quiet", "--", outer, client)
	repo := openFixture(t, client)
	middleTip := gitValue(t, middle, "rev-parse", "HEAD")
	leafTip := gitValue(t, leaf, "rev-parse", "HEAD")
	fixtureGit(t, leaf, "commit", "--allow-empty", "-m", "Later remote commit")
	plan, err := repo.PrepareSubmoduleUpdate(t.Context(), requireSubmodule(t, repo, "middle"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.Apply(t.Context(), FetchOptions{})
	if err != nil || strings.Join(result.Updated, ",") != "deps/middle,deps/middle/deps/leaf" {
		t.Fatalf("recursive checkout: %+v, %v", result, err)
	}
	first := requireSubmodule(t, repo, "middle")
	if !first.InSync || first.Head != middleTip {
		t.Fatalf("incorrect middle checkout: %+v", first)
	}
	child := openFixture(t, filepath.Join(client, "deps", "middle"))
	if second := requireSubmodule(t, child, "leaf"); !second.InSync || second.Head != leafTip {
		t.Fatalf("nested checkout did not use the indexed commit: %+v", second)
	}
}

func TestSubmoduleUpdateKeepsConflictingWorktreeEditsAndSkipsConfiguredCommand(t *testing.T) {
	dir, source := newFixture(t), newFixture(t)
	fixtureSubmodule(t, dir, source, "child", "child")
	local := filepath.Join(dir, "child")
	old := gitValue(t, local, "rev-parse", "HEAD")
	writeFixture(t, source, "tracked.txt", "new source content\n")
	fixtureGit(t, source, "add", "--", "tracked.txt")
	fixtureGit(t, source, "commit", "-m", "Publish tracked file")
	next := gitValue(t, source, "rev-parse", "HEAD")
	fixtureGit(t, dir, "update-index", "--cacheinfo", "160000,"+next+",child")
	writeFixture(t, local, "tracked.txt", "my untracked file\n")
	fixtureGit(t, dir, "config", "submodule.child.update", "!exit 77")
	repo := openFixture(t, dir)
	plan, err := repo.PrepareSubmoduleUpdate(t.Context(), requireSubmodule(t, repo, "child"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = plan.Apply(t.Context(), FetchOptions{})
	if err == nil || !strings.Contains(err.Error(), "would be overwritten") {
		t.Fatalf("expected Git to protect the conflicting file: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(local, "tracked.txt")); err != nil || string(data) != "my untracked file\n" {
		t.Fatalf("submodule update changed the file: %q, %v", data, err)
	}
	if got := gitValue(t, local, "rev-parse", "HEAD"); got != old {
		t.Fatalf("the submodule head moved despite the conflict: %q", got)
	}
}

func TestSubmoduleUpdateAuthenticationResumesAtTheNestedEndpoint(t *testing.T) {
	outer, middle, leaf := newFixture(t), newFixture(t), newFixture(t)
	fixtureGit(t, leaf, "update-server-info")
	const user, password = "submodule-user", "submodule-token"
	var authorized atomic.Int32
	files := http.StripPrefix("/leaf.git/", http.FileServer(http.Dir(filepath.Join(leaf, ".git"))))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name, token, ok := r.BasicAuth()
		if !ok || name != user || token != password {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"Submodule fixture\"")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		authorized.Add(1)
		files.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	useTestTLSConfig(t, leaf, server)
	remote := server.URL + "/leaf.git"
	fixtureSubmodule(t, middle, leaf, "leaf", "leaf")
	fixtureGit(t, middle, "config", "--file", ".gitmodules", "submodule.leaf.url", remote)
	fixtureGit(t, middle, "add", ".gitmodules")
	fixtureGit(t, middle, "commit", "-m", "Register authenticated leaf")
	fixtureSubmodule(t, outer, middle, "middle", "middle")
	fixtureGit(t, outer, "commit", "-m", "Register middle submodule")
	client := t.TempDir()
	fixtureGit(t, client, "clone", "--quiet", "--", outer, client)
	repo := openFixture(t, client)
	plan, err := repo.PrepareSubmoduleUpdate(t.Context(), requireSubmodule(t, repo, "middle"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.Apply(t.Context(), FetchOptions{})
	var challenge *RemoteAuthError
	if !errors.As(err, &challenge) || challenge.Remote != "middle/leaf" ||
		strings.Join(result.Updated, ",") != "middle" {
		t.Fatalf("nested authentication challenge was lost: %+v, %+v, %v", result, challenge, err)
	}
	options := FetchOptions{Challenge: challenge, Credentials: &HTTPSCredential{Username: user, Password: password}}
	childPath := filepath.Join(client, "middle")
	fixtureGit(t, childPath, "config", "submodule.leaf.url", server.URL+"/different.git")
	if _, err := plan.Apply(t.Context(), options); err == nil || authorized.Load() != 0 {
		t.Fatalf("retry followed changed submodule configuration: %v", err)
	}
	fixtureGit(t, childPath, "config", "submodule.leaf.url", remote)
	result, err = plan.Apply(t.Context(), options)
	if err != nil || authorized.Load() == 0 || strings.Join(result.Updated, ",") != "middle,middle/leaf" {
		t.Fatalf("retry did not resume at the selected submodule: %+v, %v", result, err)
	}
}

func TestSubmoduleAddStagesGitlinkAndGitmodulesWithoutCommitting(t *testing.T) {
	super, child := newFixture(t), newFixture(t)
	fixtureGit(t, super, "remote", "add", "origin", filepath.Join(filepath.Dir(child), "super.git"))
	head := gitValue(t, super, "rev-parse", "HEAD")
	tip := gitValue(t, child, "rev-parse", "HEAD")
	remote := "../" + filepath.Base(child)
	repo := openFixture(t, super)
	plan, err := repo.PrepareSubmoduleAdd(t.Context(), remote, "deps/new child")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(t.Context(), FetchOptions{}); err != nil {
		t.Fatal(err)
	}
	got := requireSubmodule(t, repo, "deps/new child")
	if !got.Initialized || !got.InSync || got.URL != remote || got.IndexOID != tip || got.Head != tip {
		t.Fatalf("incorrect staged submodule: %+v", got)
	}
	if staged := gitValue(t, super, "diff", "--cached", "--name-only"); staged != ".gitmodules\ndeps/new child" {
		t.Fatalf("unexpected staged paths: %q", staged)
	}
	if current := gitValue(t, super, "rev-parse", "HEAD"); current != head {
		t.Fatalf("adding a submodule changed HEAD: %q", current)
	}
	if origin := gitValue(t, filepath.Join(super, "deps", "new child"), "remote", "get-url", "origin"); origin != child {
		t.Fatalf("cloned from the wrong relative URL: %q", origin)
	}
}

func TestSubmoduleAddFailedClonePreservesExistingMetadataAndFiles(t *testing.T) {
	super, child := newFixture(t), newFixture(t)
	fixtureSubmodule(t, super, child, "existing", "existing")
	fixtureGit(t, super, "commit", "-m", "Keep the existing submodule")
	original := gitValue(t, super, "status", "--porcelain=v2")
	repo := openFixture(t, super)
	config, err := os.ReadFile(filepath.Join(repo.CommonDir, "config"))
	if err != nil {
		t.Fatal(err)
	}
	modules, err := os.ReadFile(filepath.Join(super, ".gitmodules"))
	if err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(super, "new")
	if err := os.Mkdir(local, 0o700); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(local)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "not a repository")
	plan, err := repo.PrepareSubmoduleAdd(t.Context(), source, "new")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(t.Context(), FetchOptions{}); err == nil {
		t.Fatal("added a submodule without a remote repository")
	}
	if after, err := os.Stat(local); err != nil || !os.SameFile(before, after) {
		t.Fatalf("failed clone replaced the original directory: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(repo.CommonDir, "config")); err != nil || string(got) != string(config) {
		t.Fatalf("failed clone changed the Git config: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(super, ".gitmodules")); err != nil || string(got) != string(modules) {
		t.Fatalf("failed clone changed .gitmodules: %v", err)
	}
	if got := gitValue(t, super, "status", "--porcelain=v2"); got != original {
		t.Fatalf("failed clone changed the superproject: %q", got)
	}
}

func TestSubmoduleAddRollsBackAfterARefusedIndexPublication(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a POSIX Git wrapper")
	}
	dir, source := newFixture(t), newFixture(t)
	repo := openFixture(t, dir)
	target := filepath.Join(dir, "candidate")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	old, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := repo.PrepareSubmoduleAdd(t.Context(), source, "candidate")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITWAVE_SUBMODULE_REAL_GIT", repo.git)
	t.Setenv("GITWAVE_SUBMODULE_INDEX_LOCK", filepath.Join(repo.GitDir, "index.lock"))
	t.Setenv("GITWAVE_SUBMODULE_CANDIDATE", target)
	wrapper := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(wrapper, []byte(`#!/bin/sh
if [ -d "$GITWAVE_SUBMODULE_CANDIDATE/.git" ] && [ ! -e "$GITWAVE_SUBMODULE_INDEX_LOCK" ]; then
    mkdir -p "$GITWAVE_SUBMODULE_CANDIDATE/.git/hooks" || exit 89
    printf '%s\n' 'preserve the local hook' >"$GITWAVE_SUBMODULE_CANDIDATE/.git/hooks/pre-commit" || exit 89
fi
for arg do
    if [ "$arg" = "--get-url" ] && [ -s "$GITWAVE_SUBMODULE_INDEX_LOCK" ]; then
        rm "$GITWAVE_SUBMODULE_INDEX_LOCK" || exit 90
        break
    fi
done
exec "$GITWAVE_SUBMODULE_REAL_GIT" "$@"
`), 0o700); err != nil {
		t.Fatal(err)
	}
	repo.git = wrapper
	t.Setenv("PATH", filepath.Dir(wrapper)+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := plan.Apply(t.Context(), FetchOptions{}); err == nil ||
		!strings.Contains(err.Error(), "index lock was replaced") {
		t.Fatalf("expected a failed index publication: %v", err)
	}
	if current, err := os.Stat(target); err != nil || !os.SameFile(current, old) {
		t.Fatalf("the original empty directory was not restored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitmodules")); !os.IsNotExist(err) {
		t.Fatalf("the failed registration left a .gitmodules file: %v", err)
	}
	if got := gitValue(t, dir, "status", "--porcelain=v2"); got != "" {
		t.Fatalf("the failed registration changed the parent checkout: %q", got)
	}
	recovery, err := filepath.Glob(filepath.Join(repo.GitDir, "gitwave-submodule-recovery-*", ".git", "hooks", "pre-commit"))
	if err != nil || len(recovery) != 1 {
		t.Fatalf("the checkout and a newly installed hook were not saved: %v, %v", recovery, err)
	}
	if data, err := os.ReadFile(recovery[0]); err != nil || string(data) != "preserve the local hook\n" {
		t.Fatalf("the recovered hook changed: %q, %v", data, err)
	}
}

func TestSubmoduleAddDoesNotStageAnotherGitmodulesEdit(t *testing.T) {
	dir, source := newFixture(t), newFixture(t)
	fixtureSubmodule(t, dir, source, "old", "old")
	fixtureGit(t, dir, "commit", "-m", "Existing module")
	file := gitValue(t, dir, "show", ":.gitmodules")
	writeFixture(t, dir, ".gitmodules", file+"\n# keep this change unstaged\n")
	fixtureGit(t, dir, "update-index", "--assume-unchanged", "--", ".gitmodules")
	repo := openFixture(t, dir)
	plan, err := repo.PrepareSubmoduleAdd(t.Context(), source, "new")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(t.Context(), FetchOptions{}); err == nil || !strings.Contains(err.Error(), "existing .gitmodules edits") {
		t.Fatalf("the independent edit was staged: %v", err)
	}
	if got := gitValue(t, dir, "show", ":.gitmodules"); got != file {
		t.Fatalf("the staged .gitmodules changed: %q", got)
	}
	if got, err := os.ReadFile(filepath.Join(dir, ".gitmodules")); err != nil || !strings.HasSuffix(string(got), "# keep this change unstaged\n") {
		t.Fatalf("the working .gitmodules was overwritten: %q, %v", got, err)
	}
}

func TestSubmoduleAddPreservesExistingDirectoryPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX directory permissions")
	}
	dir, source := newFixture(t), newFixture(t)
	target := filepath.Join(dir, "private-module")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	repo := openFixture(t, dir)
	plan, err := repo.PrepareSubmoduleAdd(t.Context(), source, "private-module")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(t.Context(), FetchOptions{}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(target); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("the destination permissions changed: %v, %v", info, err)
	}
}

func TestSubmoduleAddRejectsAnAliasOfAnExistingDirectory(t *testing.T) {
	dir, source := newFixture(t), newFixture(t)
	fixtureSubmodule(t, dir, source, "existing", "Child")
	upper, err := os.Stat(filepath.Join(dir, "Child"))
	if err != nil {
		t.Fatal(err)
	}
	lower, err := os.Stat(filepath.Join(dir, "child"))
	if err != nil || !os.SameFile(upper, lower) {
		t.Skip("the filesystem distinguishes uppercase and lowercase paths")
	}
	repo := openFixture(t, dir)
	if _, err := repo.PrepareSubmoduleAdd(t.Context(), source, "child"); err == nil {
		t.Fatal("accepted a second registration for the same directory")
	}
	if got := gitValue(t, dir, "ls-files", "--stage"); strings.Contains(got, "\tchild") {
		t.Fatalf("added an aliased gitlink: %s", got)
	}
}

func TestSubmoduleUpdateKeepsUnrelatedLocalEdits(t *testing.T) {
	dir, source := newFixture(t), newFixture(t)
	writeFixture(t, source, "notes.txt", "original notes\n")
	fixtureGit(t, source, "add", "--", "notes.txt")
	fixtureGit(t, source, "commit", "-m", "Publish notes")
	fixtureSubmodule(t, dir, source, "child", "child")
	local := filepath.Join(dir, "child")
	writeFixture(t, local, "notes.txt", "my edited notes\n")
	writeFixture(t, source, "upstream.txt", "new source content\n")
	fixtureGit(t, source, "add", "--", "upstream.txt")
	fixtureGit(t, source, "commit", "-m", "Publish an independent file")
	next := gitValue(t, source, "rev-parse", "HEAD")
	fixtureGit(t, dir, "update-index", "--cacheinfo", "160000,"+next+",child")
	repo := openFixture(t, dir)
	plan, err := repo.PrepareSubmoduleUpdate(t.Context(), requireSubmodule(t, repo, "child"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(t.Context(), FetchOptions{}); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(local, "notes.txt")); err != nil || string(data) != "my edited notes\n" {
		t.Fatalf("submodule update changed an unrelated file: %q, %v", data, err)
	}
	if got := gitValue(t, local, "rev-parse", "HEAD"); got != next {
		t.Fatalf("the submodule did not advance to the indexed commit: %q", got)
	}
}

func TestSubmoduleUpdateDoesNotModifyAReplacementCheckout(t *testing.T) {
	dir, source := newFixture(t), newFixture(t)
	fixtureSubmodule(t, dir, source, "child", "child")
	local := filepath.Join(dir, "child")
	old := gitValue(t, local, "rev-parse", "HEAD")
	fixtureGit(t, source, "commit", "--allow-empty", "-m", "Publish next commit")
	next := gitValue(t, source, "rev-parse", "HEAD")
	fixtureGit(t, dir, "update-index", "--cacheinfo", "160000,"+next+",child")
	repo := openFixture(t, dir)
	plan, err := repo.PrepareSubmoduleUpdate(t.Context(), requireSubmodule(t, repo, "child"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(local, filepath.Join(t.TempDir(), "original")); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "clone", "--quiet", "--", source, local)
	fixtureGit(t, local, "switch", "-c", "local-changes", old)
	if _, err := plan.Apply(t.Context(), FetchOptions{}); err == nil || !strings.Contains(err.Error(), "replaced") {
		t.Fatalf("the replacement checkout was accepted: %v", err)
	}
	if got := gitValue(t, local, "symbolic-ref", "HEAD"); got != "refs/heads/local-changes" {
		t.Fatalf("the replacement checkout's current branch changed: %q", got)
	}
	if got := gitValue(t, local, "rev-parse", "HEAD"); got != old {
		t.Fatalf("the replacement checkout's HEAD changed: %q", got)
	}
}

func TestSubmoduleAddKeepsAnUnstagedGitmodulesMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file mode")
	}
	dir, source := newFixture(t), newFixture(t)
	fixtureSubmodule(t, dir, source, "existing", "existing")
	fixtureGit(t, dir, "commit", "-m", "Register existing module")
	file := filepath.Join(dir, ".gitmodules")
	if err := os.Chmod(file, 0o755); err != nil {
		t.Fatal(err)
	}
	repo := openFixture(t, dir)
	plan, err := repo.PrepareSubmoduleAdd(t.Context(), source, "new")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(t.Context(), FetchOptions{}); err != nil {
		t.Fatal(err)
	}
	if entry := gitValue(t, dir, "ls-files", "--stage", "--", ".gitmodules"); !strings.HasPrefix(entry, "100644 ") {
		t.Fatalf("an independently modified mode was staged: %s", entry)
	}
	if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("the unstaged mode changed: %v, %v", info, err)
	}
}

func TestSubmoduleAddAcceptsAnEqualsSignInTheSourcePath(t *testing.T) {
	dir, source := newFixture(t), newFixture(t)
	renamed := filepath.Join(t.TempDir(), "repo=source")
	if err := os.Rename(source, renamed); err != nil {
		t.Fatal(err)
	}
	repo := openFixture(t, dir)
	plan, err := repo.PrepareSubmoduleAdd(t.Context(), renamed, "child")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := plan.Apply(t.Context(), FetchOptions{}); err != nil {
		t.Fatal(err)
	}
	if got := requireSubmodule(t, repo, "child"); !got.InSync || got.URL != renamed {
		t.Fatalf("incorrect submodule entry: %+v", got)
	}
}
