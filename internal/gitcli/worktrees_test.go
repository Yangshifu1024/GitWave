package gitcli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestWorktreeCreateOpenAndRemove(t *testing.T) {
	dir, repo, base, topic := checkoutFixture(t)
	destination := filepath.Join(t.TempDir(), "linked folder")
	if err := os.Mkdir(destination, 0o750); err != nil {
		t.Fatal(err)
	}
	request := WorktreeAddRequest{
		Name: "feature/worktree", Destination: destination,
		Start: CheckoutTarget{Ref: "refs/heads/topic", OID: topic},
	}
	result, err := repo.AddWorktree(t.Context(), request)
	if err != nil || result.Path == "" || result.Warning != "" {
		t.Fatalf("create: %+v, %v", result, err)
	}
	if runtime.GOOS != "windows" {
		if now, err := os.Stat(destination); err != nil || now.Mode().Perm() != 0o750 {
			t.Fatalf("worktree did not preserve the original destination permissions: %v, %v", now, err)
		}
	}
	trees, err := repo.Worktrees(t.Context())
	if err != nil || len(trees) != 2 || !trees[0].Main || !trees[0].Current {
		t.Fatalf("list: %+v, %v", trees, err)
	}
	selection := trees[1]
	if selection.Name != request.Name || selection.Branch != request.Name || selection.Head != topic ||
		selection.Current || selection.Locked || selection.Prunable || selection.ID == "" {
		t.Fatalf("new linked worktree: %+v", selection)
	}
	linked, err := repo.OpenWorktree(t.Context(), selection)
	if err != nil {
		t.Fatal(err)
	}
	reverse, err := linked.Worktrees(t.Context())
	if err != nil || len(reverse) != 2 || !reverse[0].Main || reverse[0].Current || !reverse[1].Current {
		t.Fatalf("listing from linked worktree: %+v, %v", reverse, err)
	}
	if got := gitValue(t, result.Path, "symbolic-ref", "HEAD"); got != "refs/heads/"+request.Name {
		t.Fatalf("linked HEAD: %s", got)
	}
	if got := gitValue(t, dir, "rev-parse", "HEAD"); got != base {
		t.Fatalf("main HEAD changed to %s", got)
	}
	if _, err := linked.PrepareWorktreeRemoval(t.Context(), reverse[1]); err == nil {
		t.Fatal("could remove the active worktree")
	}
	plan, err := repo.PrepareWorktreeRemoval(t.Context(), selection)
	if err != nil || plan.MetadataOnly() {
		t.Fatalf("prepare removal: %+v, %v", plan, err)
	}
	if removed, err := plan.Remove(t.Context()); err != nil || removed.Warning != "" {
		t.Fatalf("remove: %+v, %v", removed, err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("worktree directory still present: %v", err)
	}
	if got := gitValue(t, dir, "rev-parse", "refs/heads/"+request.Name); got != topic {
		t.Fatalf("worktree branch changed to %s", got)
	}
}

func TestWorktreeProtectsLocalFilesAndPrunesOnlyMetadata(t *testing.T) {
	dir, repo, base, _ := checkoutFixture(t)
	destination := filepath.Join(t.TempDir(), "linked")
	fixtureGit(t, dir, "worktree", "add", "-b", "linked", destination, base)
	selectWorktree := func() Worktree {
		t.Helper()
		trees, err := repo.Worktrees(t.Context())
		if err != nil || len(trees) != 2 {
			t.Fatalf("list: %+v, %v", trees, err)
		}
		return trees[1]
	}
	first := selectWorktree()
	plan, err := repo.PrepareWorktreeRemoval(t.Context(), first)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, destination, "local-only", "Keep this data")
	if _, err := plan.Remove(t.Context()); err == nil {
		t.Fatal("remove accepted an outdated confirmation after a local file appeared")
	}
	if err := os.Remove(filepath.Join(destination, "local-only")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo.CommonDir, "info"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, repo.CommonDir, "info/exclude", "/ignored-only\n")
	writeFixture(t, destination, "ignored-only", "Keep ignored data")
	if _, err := repo.PrepareWorktreeRemoval(t.Context(), first); err == nil {
		t.Fatal("remove accepted an ignored local file")
	}
	if err := os.Remove(filepath.Join(destination, "ignored-only")); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, dir, "worktree", "lock", destination)
	if locked := selectWorktree(); !locked.Locked {
		t.Fatal("worktree lock missing")
	} else if _, err := repo.PrepareWorktreeRemoval(t.Context(), locked); err == nil {
		t.Fatal("remove accepted a locked worktree")
	}
	fixtureGit(t, dir, "worktree", "unlock", destination)
	if err := os.Remove(filepath.Join(destination, ".git")); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, destination, "reused", "Belongs to another application")
	missing := selectWorktree()
	plan, err = repo.PrepareWorktreeRemoval(t.Context(), missing)
	if err != nil || !plan.MetadataOnly() {
		t.Fatalf("prepare missing registration: %+v, %v (%+v)", plan, err, missing)
	}
	if result, err := plan.Remove(t.Context()); err != nil || result.Warning != "" {
		t.Fatalf("remove metadata: %+v, %v", result, err)
	}
	if data, err := os.ReadFile(filepath.Join(destination, "reused")); err != nil || string(data) != "Belongs to another application" {
		t.Fatalf("reused directory changed: %q, %v", data, err)
	}
}

func TestWorktreeRejectsOccupiedDirectoryAndStaleStart(t *testing.T) {
	dir, repo, base, topic := checkoutFixture(t)
	destination := t.TempDir()
	writeFixture(t, destination, "keep", "Keep this data")
	request := WorktreeAddRequest{Name: "new", Destination: destination, Start: CheckoutTarget{Ref: "refs/heads/topic", OID: topic}}
	if _, err := repo.AddWorktree(t.Context(), request); err == nil {
		t.Fatal("occupied destination accepted")
	}
	fixtureGit(t, dir, "update-ref", "refs/heads/topic", base)
	request.Destination = filepath.Join(t.TempDir(), "new")
	if _, err := repo.AddWorktree(t.Context(), request); err == nil || !strings.Contains(err.Error(), "starting branch changed") {
		t.Fatalf("stale starting point: %v", err)
	}
	if branches := gitValue(t, dir, "branch", "--list", "new"); branches != "" {
		t.Fatalf("found orphan branch: %q", branches)
	}
}

func TestWorktreeRemovalProtectsDataGitStatusDoesNotShow(t *testing.T) {
	dir, repo, base, _ := checkoutFixture(t)
	fixtureGit(t, dir, "update-index", "--add", "--cacheinfo", "160000,"+base+",sub")
	fixtureGit(t, dir, "commit", "-m", "Include an uninitialized submodule")
	destination := filepath.Join(t.TempDir(), "linked")
	fixtureGit(t, dir, "worktree", "add", "-b", "linked", destination)
	trees, err := repo.Worktrees(t.Context())
	if err != nil || len(trees) != 2 {
		t.Fatalf("list: %+v, %v", trees, err)
	}
	plan, err := repo.PrepareWorktreeRemoval(t.Context(), trees[1])
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, destination, "sub/user-file", "independent local data")
	if status := gitValue(t, destination, "status", "--porcelain=v2", "--ignore-submodules=none", "--untracked-files=all"); status != "" {
		t.Fatalf("fixture unexpectedly visible to Git status: %q", status)
	}
	if _, err := plan.Remove(t.Context()); err == nil || !strings.Contains(err.Error(), "local data at submodule path") {
		t.Fatalf("removed dirty uninitialized submodule: %v", err)
	}
	path := filepath.Join(destination, "sub", "user-file")
	if data, err := os.ReadFile(path); err != nil || string(data) != "independent local data" {
		t.Fatalf("submodule contents changed: %q, %v", data, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, destination, "update-index", "--assume-unchanged", "mixed.txt")
	writeFixture(t, destination, "mixed.txt", "concealed local edits")
	if _, err := plan.Remove(t.Context()); err == nil || !strings.Contains(err.Error(), "local changes") {
		t.Fatalf("removed a worktree with assume-unchanged edits: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(destination, "mixed.txt")); err != nil || string(data) != "concealed local edits" {
		t.Fatalf("hidden contents changed: %q, %v", data, err)
	}
}

func TestWorktreeCreateRetainsOriginalDirectoryWhenRefCommitFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture uses a Git process wrapper")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("the Git process wrapper requires python3")
	}
	dir, repo, base, _ := checkoutFixture(t)
	root := t.TempDir()
	destination := filepath.Join(root, "existing empty folder")
	if err := os.Mkdir(destination, 0o750); err != nil {
		t.Fatal(err)
	}
	original, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	wrapper := filepath.Join(bin, "git")
	t.Setenv("GITWAVE_WORKTREE_REAL_GIT", repo.git)
	t.Setenv("GITWAVE_WORKTREE_TEST_TARGET", destination)
	if err := os.WriteFile(wrapper, []byte(`#!/usr/bin/env python3
import os, subprocess, sys
args = [os.environ["GITWAVE_WORKTREE_REAL_GIT"], *sys.argv[1:]]
if "update-ref" not in args or not any(arg.startswith("worktree: add ") for arg in args):
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
if not os.path.isfile(os.path.join(os.environ["GITWAVE_WORKTREE_TEST_TARGET"], ".git")):
    sys.exit(92)
process.stdin.write(b"abort\0")
process.stdin.close()
process.wait()
print("injected worktree ref commit failure", file=sys.stderr)
sys.exit(91)
`), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	request := WorktreeAddRequest{Name: "interrupted", Destination: destination,
		Start: CheckoutTarget{Ref: "refs/heads/main", OID: base}}
	if _, err := repo.AddWorktree(t.Context(), request); err == nil ||
		!strings.Contains(err.Error(), "injected worktree ref commit failure") {
		t.Fatalf("expected injected transaction failure: %v", err)
	}
	if after, err := os.Stat(destination); err != nil || !os.SameFile(original, after) {
		t.Fatalf("failed creation replaced the original empty directory: %v", err)
	}
	if files, err := os.ReadDir(root); err != nil || len(files) != 1 || files[0].Name() != filepath.Base(destination) {
		t.Fatalf("failed creation left provisional files: %v, %v", files, err)
	}
	if branches := gitValue(t, dir, "branch", "--list", "interrupted"); branches != "" {
		t.Fatalf("failed creation left an orphan branch: %q", branches)
	}
	if trees, err := repo.Worktrees(t.Context()); err != nil || len(trees) != 1 {
		t.Fatalf("failed creation left a registration: %+v, %v", trees, err)
	}
}

func TestWorktreeRemovalRechecksContentsWithPreservedModificationTime(t *testing.T) {
	dir, repo, _, _ := checkoutFixture(t)
	destination := filepath.Join(t.TempDir(), "linked")
	fixtureGit(t, dir, "worktree", "add", "-b", "linked", destination)
	fixtureGit(t, destination, "config", "core.trustctime", "false")
	path := filepath.Join(destination, "mixed.txt")
	stamp := time.Unix(1_600_000_000, 0)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, destination, "update-index", "--refresh")
	entries, err := repo.Worktrees(t.Context())
	if err != nil || len(entries) != 2 {
		t.Fatalf("worktree list: %+v, %v", entries, err)
	}
	plan, err := repo.PrepareWorktreeRemoval(t.Context(), entries[1])
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil || len(before) == 0 {
		t.Fatalf("read fixture file: %q, %v", before, err)
	}
	changed := strings.Repeat("x", len(before))
	writeFixture(t, destination, "mixed.txt", changed)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if status := gitValue(t, destination, "status", "--porcelain=v2", "--untracked-files=all"); status != "" {
		t.Fatalf("fixture did not preserve Git's cached stat information: %q", status)
	}
	if _, err := plan.Remove(t.Context()); err == nil || !strings.Contains(err.Error(), "local changes") {
		t.Fatalf("worktree with changed content was not protected: %v", err)
	}
	if files, err := os.ReadFile(path); err != nil || string(files) != changed {
		t.Fatalf("the edited file was not preserved: %q, %v", files, err)
	}
}

func TestWorktreeRemovalProtectsNestedRepositoryWithTrackedFiles(t *testing.T) {
	dir := newFixture(t)
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "nested/file.txt", "shared file\n")
	fixtureGit(t, dir, "add", "--", "nested/file.txt")
	fixtureGit(t, dir, "commit", "-m", "Track a file in the nested directory")
	repo := openFixture(t, dir)
	destination := filepath.Join(t.TempDir(), "linked")
	fixtureGit(t, dir, "worktree", "add", "-b", "linked", destination)
	nested := filepath.Join(destination, "nested")
	fixtureGit(t, nested, "init", "--template=", "-b", "main")
	fixtureGit(t, nested, "-c", "user.name=Nested Author", "-c", "user.email=nested@example.invalid",
		"commit", "--allow-empty", "-m", "Only stored in the independent nested repository")
	before := gitValue(t, nested, "rev-parse", "HEAD")
	if status := gitValue(t, destination, "status", "--porcelain=v2", "--untracked-files=all"); status != "" {
		t.Fatalf("fixture nested metadata is visible to parent Git: %q", status)
	}
	entries, err := repo.Worktrees(t.Context())
	if err != nil || len(entries) != 2 {
		t.Fatalf("worktree list: %+v, %v", entries, err)
	}
	if _, err := repo.PrepareWorktreeRemoval(t.Context(), entries[1]); err == nil ||
		!strings.Contains(err.Error(), "separate Git metadata") {
		t.Fatalf("could prepare removal of independent Git data: %v", err)
	}
	if after := gitValue(t, nested, "rev-parse", "HEAD"); after != before {
		t.Fatalf("nested HEAD changed: %s != %s", after, before)
	}
}

func TestWorktreeRemovalProtectsAnUnreferencedDetachedCommit(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	destination := filepath.Join(t.TempDir(), "detached")
	fixtureGit(t, dir, "worktree", "add", "--detach", destination)
	fixtureGit(t, destination, "commit", "--allow-empty", "-m", "Unpublished detached commit")
	entries, err := repo.Worktrees(t.Context())
	if err != nil || len(entries) != 2 || entries[1].Branch != "" {
		t.Fatalf("worktree list: %+v, %v", entries, err)
	}
	entry := entries[1]
	if _, err := repo.PrepareWorktreeRemoval(t.Context(), entry); err == nil ||
		!strings.Contains(err.Error(), "create a branch or tag") {
		t.Fatalf("detached commit was not protected: %v", err)
	}
	fixtureGit(t, destination, "branch", "saved-detached")
	plan, err := repo.PrepareWorktreeRemoval(t.Context(), entry)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := plan.Remove(t.Context()); err != nil || result.Warning != "" {
		t.Fatalf("could not remove the detached worktree after saving it: %+v, %v", result, err)
	}
	if tip := gitValue(t, dir, "rev-parse", "saved-detached"); tip != entry.Head {
		t.Fatalf("saved detached commit changed: %s", tip)
	}
}

func TestWorktreeAddAndRemoveInSHA256Reftable(t *testing.T) {
	dir := newFixture(t, "--ref-format=reftable", "--object-format=sha256")
	repo := openFixture(t, dir)
	base := gitValue(t, dir, "rev-parse", "HEAD")
	destination := filepath.Join(t.TempDir(), "linked")
	request := WorktreeAddRequest{Name: "feature/reftable", Destination: destination,
		Start: CheckoutTarget{Ref: "refs/heads/main", OID: base}}
	if result, err := repo.AddWorktree(t.Context(), request); err != nil || result.Warning != "" {
		t.Fatalf("SHA-256 reftable add: %+v, %v", result, err)
	}
	trees, err := repo.Worktrees(t.Context())
	if err != nil || len(trees) != 2 || trees[1].Head != base || trees[1].Branch != request.Name {
		t.Fatalf("worktree list: %+v, %v", trees, err)
	}
	plan, err := repo.PrepareWorktreeRemoval(t.Context(), trees[1])
	if err != nil {
		t.Fatal(err)
	}
	if result, err := plan.Remove(t.Context()); err != nil || result.Warning != "" {
		t.Fatalf("SHA-256 reftable remove: %+v, %v", result, err)
	}
	fixtureGit(t, dir, "fsck", "--no-dangling")
}
