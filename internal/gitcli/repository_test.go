package gitcli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestHasCommitDistinguishesMissingFromCorruptObjects(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	old := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	fixtureGit(t, dir, "commit", "--allow-empty", "-m", "New commit")
	next := strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD"))
	path := filepath.Join(repo.CommonDir, "objects", old[:2], old[2:])
	if exists, err := repo.HasCommit(t.Context(), old); err != nil || !exists {
		t.Fatalf("intact commit: exists = %t, err = %v", exists, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if exists, err := repo.HasCommit(t.Context(), old); err != nil || exists {
		t.Fatalf("missing commit: exists = %t, err = %v", exists, err)
	}
	wrong, err := os.ReadFile(filepath.Join(repo.CommonDir, "objects", next[:2], next[2:]))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, wrong, 0o600); err != nil {
		t.Fatal(err)
	}
	if exists, err := repo.HasCommit(t.Context(), old); err == nil || exists {
		t.Fatalf("corrupt commit: exists = %t, err = %v", exists, err)
	}
}

func TestInitCreatesUnbornMainAndReopensWithoutChangingFiles(t *testing.T) {
	parent := newFixture(t)
	outer := openFixture(t, parent)
	target := filepath.Join(parent, "nested parent", "new repo")
	t.Setenv("GIT_DIR", outer.GitDir)
	t.Setenv("GIT_WORK_TREE", outer.WorkTree)
	t.Setenv("GIT_DEFAULT_HASH", "sha256")
	repo, err := Init(t.Context(), target)
	if err != nil {
		t.Fatal(err)
	}
	if repo.Path() == outer.Path() || repo.GitDir == outer.GitDir {
		t.Fatalf("init reused the outer checkout: %+v", repo)
	}
	if status := readFixtureStatus(t, repo); status.HeadRef != "refs/heads/main" || status.HeadOID != "(initial)" {
		t.Fatalf("new repository has an unexpected HEAD: %+v", status)
	}
	if got := gitValue(t, target, "rev-parse", "--show-object-format"); got != "sha1" {
		t.Fatalf("unexpected object format in the new repository: %s", got)
	}
	writeFixture(t, target, "source.txt", "existing content\n")
	fixtureGit(t, target, "symbolic-ref", "HEAD", "refs/heads/chosen")
	reopened, err := Init(t.Context(), target)
	if err != nil || !repo.SameCheckout(reopened) {
		t.Fatalf("init did not reopen the same checkout: %+v, %v", reopened, err)
	}
	if status := readFixtureStatus(t, reopened); status.HeadRef != "refs/heads/chosen" ||
		len(status.Changes) != 1 || status.Changes[0].Path != "source.txt" {
		t.Fatalf("reopening changed the existing checkout: %+v", status)
	}
}

func TestInitLeavesExistingBareRepositoryAndInvalidGitMetadataInPlace(t *testing.T) {
	bare := t.TempDir()
	fixtureGit(t, bare, "init", "--bare")
	if _, err := Init(t.Context(), bare); err == nil || !strings.Contains(err.Error(), "bare repository") {
		t.Fatalf("expected refusal to initialize a worktree inside a bare repository: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(bare, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("init created a separate .git in the bare repository: %v", err)
	}
	outer := openFixture(t, newFixture(t))
	metadata := []string{bare, outer.GitDir}
	upper := filepath.Join(outer.WorkTree, ".GIT")
	if _, err := os.Stat(upper); err == nil {
		metadata = append(metadata, upper)
	}
	for _, parent := range metadata {
		target := filepath.Join(parent, "refs", "heads", "nested", "repo")
		if _, err := Init(t.Context(), target); err == nil || !strings.Contains(err.Error(), "Git metadata") {
			t.Fatalf("expected refusal to initialize within %q: %v", parent, err)
		}
		if _, err := os.Lstat(filepath.Dir(target)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("init modified the refs namespace in %q: %v", parent, err)
		}
	}
	dir := t.TempDir()
	writeFixture(t, dir, ".git", "invalid Git metadata\n")
	if _, err := Init(t.Context(), dir); err == nil {
		t.Fatal("init accepted invalid Git metadata")
	}
	if got, err := os.ReadFile(filepath.Join(dir, ".git")); err != nil || string(got) != "invalid Git metadata\n" {
		t.Fatalf("init modified existing metadata: %q, %v", got, err)
	}
}

func TestOpenLinkedWorktree(t *testing.T) {
	main := newFixture(t)
	linked := filepath.Join(t.TempDir(), "linked worktree")
	fixtureGit(t, main, "worktree", "add", "-b", "linked", linked)
	subdir := filepath.Join(linked, "subdir")
	if err := os.Mkdir(subdir, 0o700); err != nil {
		t.Fatal(err)
	}
	original := openFixture(t, main)
	t.Setenv("GIT_DIR", original.GitDir)
	t.Setenv("GIT_WORK_TREE", original.WorkTree)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(original.GitDir, "index"))
	worktree := openFixture(t, subdir)
	wantRoot, err := filepath.EvalSymlinks(linked)
	if err != nil {
		t.Fatal(err)
	}
	if worktree.WorkTree != wantRoot || worktree.CommonDir != original.CommonDir || worktree.GitDir == original.GitDir {
		t.Fatalf("worktree identity: linked %+v, original %+v", worktree, original)
	}
	writeFixture(t, linked, "conflict.txt", "only changed in linked worktree\n")
	status := readFixtureStatus(t, worktree)
	if status.Branch != "linked" || len(status.Changes) != 1 || status.Changes[0].Path != "conflict.txt" {
		t.Fatalf("status used the wrong worktree: %+v", status)
	}
}

func TestWorkingCopyStatus(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	writeFixture(t, dir, "mixed.txt", "staged version\n")
	fixtureGit(t, dir, "add", "mixed.txt")
	writeFixture(t, dir, "mixed.txt", "unstaged version\n")
	fixtureGit(t, dir, "mv", "old name.txt", "new name.txt")
	writeFixture(t, dir, "空 格.txt", "untracked\n")
	indexPath := filepath.Join(repo.GitDir, "index")
	before, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	status := readFixtureStatus(t, repo)
	if status.HeadOID != strings.TrimSpace(fixtureGit(t, dir, "rev-parse", "HEAD")) || status.Branch != "main" {
		t.Fatalf("incorrect branch headers: %+v", status)
	}
	want := []Change{
		{Kind: '1', Index: 'M', WorkTree: 'M', Submodule: "N...", Path: "mixed.txt",
			HeadOID: gitValue(t, dir, "rev-parse", "HEAD:mixed.txt"), IndexOID: gitValue(t, dir, "rev-parse", ":mixed.txt"), HeadMode: "100644", IndexMode: "100644", WorkTreeMode: "100644"},
		{Kind: '2', Index: 'R', WorkTree: '.', Submodule: "N...", Path: "new name.txt", PreviousPath: "old name.txt",
			HeadOID: gitValue(t, dir, "rev-parse", "HEAD:old name.txt"), IndexOID: gitValue(t, dir, "rev-parse", ":new name.txt"), HeadMode: "100644", IndexMode: "100644", WorkTreeMode: "100644"},
		{Kind: '?', Path: "空 格.txt"},
	}
	slices.SortFunc(status.Changes, func(a, b Change) int { return strings.Compare(a.Path, b.Path) })
	if !slices.Equal(status.Changes, want) {
		t.Fatalf("changes = %+v, want %+v", status.Changes, want)
	}
	after, err := os.ReadFile(indexPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("status modified the Git index: %v", err)
	}
}

func TestDetectsConflictDuringMergeAndInteractiveRebase(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	fixtureGit(t, dir, "switch", "-c", "topic")
	writeFixture(t, dir, "conflict.txt", "change from topic\n")
	fixtureGit(t, dir, "commit", "-am", "Topic change")
	topic := readFixtureStatus(t, repo).HeadOID
	fixtureGit(t, dir, "switch", "main")
	writeFixture(t, dir, "conflict.txt", "change from main\n")
	fixtureGit(t, dir, "commit", "-am", "Main change")
	checkConflict := func(branch string) {
		t.Helper()
		status := readFixtureStatus(t, repo)
		want := Change{Kind: 'u', Index: 'U', WorkTree: 'U', Submodule: "N...", Path: "conflict.txt", WorkTreeMode: "100644"}
		for i := range want.Conflict {
			want.Conflict[i] = IndexStage{Mode: "100644", OID: gitValue(t, dir, "rev-parse", fmt.Sprintf(":%d:conflict.txt", i+1))}
		}
		if status.Branch != branch || !slices.Equal(status.Changes, []Change{want}) {
			t.Fatalf("conflicted status: %+v, want branch %q and %+v", status, branch, want)
		}
	}
	fixtureConflict(t, dir, "merge", "topic")
	checkConflict("main")
	fixtureGit(t, dir, "merge", "--abort")
	fixtureGit(t, dir, "switch", "topic")
	t.Setenv("GIT_SEQUENCE_EDITOR", ":")
	fixtureConflict(t, dir, "rebase", "-i", "main")
	checkConflict("(detached)")
	fixtureGit(t, dir, "rebase", "--abort")
	if status := readFixtureStatus(t, repo); status.HeadOID != topic || status.Branch != "topic" || len(status.Changes) != 0 {
		t.Fatalf("abort did not restore the topic: %+v", status)
	}
}

func TestStatusReturnsContextCancellation(t *testing.T) {
	repo := openFixture(t, newFixture(t))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := repo.Status(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled status returned %v", err)
	}
}

func TestStatusDoesNotFallBackToEnclosingRepository(t *testing.T) {
	parent := newFixture(t)
	child := filepath.Join(parent, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, child, "init", "--template=", "-b", "child")
	repo := openFixture(t, child)
	if err := os.Rename(repo.GitDir, filepath.Join(t.TempDir(), "moved-gitdir")); err != nil {
		t.Fatal(err)
	}
	if status, err := repo.Status(t.Context()); err == nil {
		t.Fatalf("status after moving the child Git directory: %+v", status)
	}
}

func newFixture(t *testing.T, initOptions ...string) string {
	t.Helper()
	// Test writes and Git config are confined to a temporary repository.
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	dir := t.TempDir()
	fixtureGit(t, dir, append([]string{"init", "--template=", "-b", "main"}, initOptions...)...)
	fixtureGit(t, dir, "config", "user.name", "GitWave Test")
	fixtureGit(t, dir, "config", "user.email", "test@example.invalid")
	fixtureGit(t, dir, "config", "core.precomposeunicode", "true")
	writeFixture(t, dir, "mixed.txt", "original version\n")
	writeFixture(t, dir, "old name.txt", "unique rename contents\n")
	writeFixture(t, dir, "conflict.txt", "common base\n")
	fixtureGit(t, dir, "add", ".")
	fixtureGit(t, dir, "commit", "-m", "Initial version")
	return dir
}

func openFixture(t *testing.T, path string) *Repository {
	t.Helper()
	repo, err := Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func readFixtureStatus(t *testing.T, repo *Repository) Status {
	t.Helper()
	status, err := repo.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func writeFixture(t *testing.T, dir, path, value string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, path), []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func fixtureGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	output, err := fixtureCommand(t, dir, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func fixtureConflict(t *testing.T, dir string, args ...string) {
	t.Helper()
	output, err := fixtureCommand(t, dir, args...).CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 1 {
		t.Fatalf("git %v: expected a conflict, got %v\n%s", args, err, output)
	}
}

func fixtureCommand(t *testing.T, dir string, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	// Inherit the dedicated config and editor, not a surrounding repository.
	env := slices.DeleteFunc(os.Environ(), func(entry string) bool {
		name, _, _ := strings.Cut(entry, "=")
		return strings.HasPrefix(strings.ToUpper(name), "GIT_")
	})
	cmd.Env = append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_SEQUENCE_EDITOR="+os.Getenv("GIT_SEQUENCE_EDITOR"))
	return cmd
}
