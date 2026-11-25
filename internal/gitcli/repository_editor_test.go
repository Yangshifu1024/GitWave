package gitcli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGitignoreEditorPreservesNewlinesAndRejectsStaleChanges(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	editor, err := repo.OpenGitignore(t.Context())
	if err != nil || editor.Text() != "" {
		t.Fatalf("open a missing .gitignore: %v", err)
	}
	const draft = "# local rules\r\n/private"
	if err := editor.Save(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	if got := lfsRead(t, dir, ".gitignore"); got != draft+"\r\n" {
		t.Fatalf("saved .gitignore bytes = %q", got)
	}
	editor, err = repo.OpenGitignore(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	const external = "# edited externally\n"
	writeFixture(t, dir, ".gitignore", external)
	if err := editor.Save(t.Context(), "old draft"); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("stale editor result = %v", err)
	}
	if got := lfsRead(t, dir, ".gitignore"); got != external {
		t.Fatalf("a stale save overwrote .gitignore: %q", got)
	}
}

func TestGitignoreRejectsARuleOverriddenByANestedGitignore(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "nested/.gitignore", "!chosen.log\n")
	writeFixture(t, dir, "nested/chosen.log", "keep")
	writeFixture(t, dir, "unrelated.log", "keep")
	for _, scope := range []IgnoreScope{IgnoreFile, IgnoreExtension} {
		if added, err := repo.IgnoreUntracked(t.Context(), "nested/chosen.log", scope); added ||
			err == nil || !strings.Contains(err.Error(), "overrides") {
			t.Fatalf("scope %d accepted an ineffective root rule: %t, %v", scope, added, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Fatalf("a shadowed rule was written: %v", err)
	}
	if added, err := repo.IgnoreUntracked(t.Context(), "nested/chosen.log", IgnoreDirectory); !added || err != nil {
		t.Fatalf("the directory rule should ignore its descendants: %t, %v", added, err)
	}
}

func TestGitignoreRechecksRepositoryAfterRulePreview(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the Git wrapper requires a POSIX shell")
	}
	const script = `#!/bin/sh
for arg do
  if [ "$arg" = check-ignore ] && [ -f "$GITWAVE_TEST_PREVIEW_REPO/.gitignore.lock" ] && [ ! -f "$GITWAVE_TEST_PREVIEW_MARKER" ]; then
    case "$GITWAVE_TEST_PREVIEW_CHANGE" in
      index) mv "$GITWAVE_TEST_PREVIEW_REPO/.git/index.lock" "$GITWAVE_TEST_PREVIEW_MARKER" || exit 88
             "$GITWAVE_TEST_PREVIEW_REAL" -C "$GITWAVE_TEST_PREVIEW_REPO" add -- nested/chosen.log || exit 89 ;;
      rules) printf '!chosen.log\n' > "$GITWAVE_TEST_PREVIEW_REPO/nested/.gitignore" || exit 88
             touch "$GITWAVE_TEST_PREVIEW_MARKER" || exit 89 ;;
    esac
    break
  fi
done
exec "$GITWAVE_TEST_PREVIEW_REAL" "$@"
`
	for _, test := range []struct{ change, message string }{
		{"index", "index lock changed"},
		{"rules", "changed during the ignore rule preview"},
	} {
		t.Run(test.change, func(t *testing.T) {
			dir := newFixture(t)
			repo := openFixture(t, dir)
			if err := os.Mkdir(filepath.Join(dir, "nested"), 0o755); err != nil {
				t.Fatal(err)
			}
			const selected = "nested/chosen.log"
			writeFixture(t, dir, selected, "keep")
			writeFixture(t, dir, "other.log", "keep")
			temp := t.TempDir()
			wrapper, marker := filepath.Join(temp, "git"), filepath.Join(temp, "triggered")
			t.Setenv("GITWAVE_TEST_PREVIEW_REAL", repo.git)
			t.Setenv("GITWAVE_TEST_PREVIEW_REPO", dir)
			t.Setenv("GITWAVE_TEST_PREVIEW_MARKER", marker)
			t.Setenv("GITWAVE_TEST_PREVIEW_CHANGE", test.change)
			if err := os.WriteFile(wrapper, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			repo.git = wrapper
			added, err := repo.IgnoreUntracked(t.Context(), selected, IgnoreExtension)
			if _, markerErr := os.Stat(marker); markerErr != nil {
				t.Fatalf("fixture missed the preview: %v (result %t, %v)", markerErr, added, err)
			}
			if added || err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("the rule was accepted after the repository changed: %t, %v", added, err)
			}
			if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
				t.Fatalf("failed preview wrote .gitignore: %v", err)
			}
		})
	}
}

func TestRepoEditorRejectsInPlaceChangeToPreparedLock(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	const original = "# original rules\n"
	const wanted = "# user draft\n"
	writeFixture(t, dir, ".gitignore", original)
	editor, err := repo.OpenGitignore(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	changed := false
	err = editor.file.save(t.Context(), wanted, 0o600, func() error {
		lock := filepath.Join(dir, ".gitignore.lock")
		info, err := os.Stat(lock)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Size() == int64(len(wanted)) {
			changed = true
			return os.WriteFile(lock, []byte(strings.Repeat("x", len(wanted))), 0o600)
		}
		return nil
	})
	if !changed || err == nil || lfsRead(t, dir, ".gitignore") != original {
		t.Fatalf("the editor published changed lock contents: changed=%t, %v", changed, err)
	}
}

func TestGitignoreFileRuleEscapesAndAnchorsLiteralPath(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	const path = "cache [1].log"
	writeFixture(t, dir, path, "keep the untracked data")
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "nested/"+path, "do not ignore this file")
	writeFixture(t, dir, "cache 1.log", "do not match brackets as a wildcard")
	const initial = "# prior settings\r\n"
	writeFixture(t, dir, ".gitignore", initial)
	if added, err := repo.IgnoreUntracked(t.Context(), path, IgnoreFile); !added || err != nil {
		t.Fatalf("add .gitignore rule: %t, %v", added, err)
	}
	if added, err := repo.IgnoreUntracked(t.Context(), path, IgnoreFile); added || err != nil {
		t.Fatalf("repeat the same ignore action: %t, %v", added, err)
	}
	want := initial + "/cache\\ \\[1\\].log\r\n"
	if got := lfsRead(t, dir, ".gitignore"); got != want {
		t.Fatalf("ignore rule = %q, want %q", got, want)
	}
	visible := fixtureGit(t, dir, "ls-files", "--others", "--exclude-standard")
	if !strings.Contains(visible, "nested/"+path) || !strings.Contains(visible, "cache 1.log") ||
		strings.Contains(visible, "\n"+path+"\n") {
		t.Fatalf("unexpected remaining untracked files: %q", visible)
	}
	if got := lfsRead(t, dir, path); got != "keep the untracked data" {
		t.Fatalf("ignore altered the selected file: %q", got)
	}
	fixtureGit(t, dir, "add", "--", "cache 1.log")
	if added, err := repo.IgnoreUntracked(t.Context(), "cache 1.log", IgnoreExtension); added || err == nil {
		t.Fatalf("ignore accepted a now-tracked file: %t, %v", added, err)
	}
	if got := lfsRead(t, dir, ".gitignore"); got != want {
		t.Fatalf("stale action appended a broad rule: %q", got)
	}
}

func TestGitignoreDirectoryAndExtensionScopes(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	if err := os.Mkdir(filepath.Join(dir, "generated [1]"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "generated [1]/one.tmp", "cache")
	writeFixture(t, dir, "generated [1]/two.txt", "cache")
	writeFixture(t, dir, "one.tmp", "generated output")
	writeFixture(t, dir, "two.txt", "user file")
	if added, err := repo.IgnoreUntracked(t.Context(), "generated [1]/one.tmp", IgnoreDirectory); !added || err != nil {
		t.Fatalf("ignore directory: %t, %v", added, err)
	}
	if added, err := repo.IgnoreUntracked(t.Context(), "one.tmp", IgnoreExtension); !added || err != nil {
		t.Fatalf("ignore extension: %t, %v", added, err)
	}
	if seen := fixtureGit(t, dir, "ls-files", "--others", "--exclude-standard"); seen != ".gitignore\ntwo.txt\n" {
		t.Fatalf("unexpected untracked paths: %q", seen)
	}
}

func TestHooksEditorUsesCommonAndConfiguredDirectories(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	linked := filepath.Join(t.TempDir(), "linked")
	fixtureGit(t, dir, "worktree", "add", "--detach", "--", linked)
	worker := openFixture(t, linked)
	hooks, err := worker.OpenHooksEditor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	list, err := hooks.List(t.Context())
	if err != nil || len(list) != 8 || list[0].Name != "pre-commit" || list[0].Exists ||
		list[0].Path != filepath.Join(repo.CommonDir, "hooks", "pre-commit") {
		t.Fatalf("linked worktree hooks: %+v, %v", list, err)
	}
	configured := filepath.Join(linked, "custom", "hooks")
	fixtureGit(t, linked, "config", "core.hooksPath", "custom/hooks")
	if _, err := hooks.List(t.Context()); err == nil {
		t.Fatal("stale hooks editor ignored core.hooksPath change")
	}
	hooks, err = worker.OpenHooksEditor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	edit, err := hooks.Open(t.Context(), "pre-commit")
	if err != nil || edit.Text() != "" {
		t.Fatalf("open absent script: %v", err)
	}
	const script = "#!/bin/sh\nprintf ran > hook-ran\n"
	if err := edit.Save(t.Context(), script); err != nil {
		t.Fatal(err)
	}
	if got := lfsRead(t, configured, "pre-commit"); got != script {
		t.Fatalf("wrong script at configured path: %q", got)
	}
	list, err = hooks.List(t.Context())
	if err != nil || !list[0].Exists || list[0].Executable != (runtime.GOOS != "windows") {
		t.Fatalf("saved hook status: %+v, %v", list, err)
	}
	if _, err := os.Stat(filepath.Join(linked, "hook-ran")); !os.IsNotExist(err) {
		t.Fatalf("editing a hook ran it: %v", err)
	}
	if _, err := hooks.Open(t.Context(), "../pre-commit"); err == nil {
		t.Fatal("opened a path outside the known hook names")
	}
}

func TestHookSaveRefusesAChangedScriptOrDirectory(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	first := t.TempDir()
	second := t.TempDir()
	fixtureGit(t, dir, "config", "core.hooksPath", first)
	writeFixture(t, first, "post-checkout", "# initial script")
	hooks, err := repo.OpenHooksEditor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	edit, err := hooks.Open(t.Context(), "post-checkout")
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, first, "post-checkout", "# an external edit")
	if err := edit.Save(t.Context(), "# stale draft"); err == nil {
		t.Fatal("hook editor overwrote an external edit")
	}
	fixtureGit(t, dir, "config", "core.hooksPath", second)
	if err := edit.Save(t.Context(), "# stale draft"); err == nil {
		t.Fatal("hook editor crossed into a different configured directory")
	}
	if got := lfsRead(t, first, "post-checkout"); got != "# an external edit" {
		t.Fatalf("original path was modified: %q", got)
	}
	if _, err := os.Stat(filepath.Join(second, "post-checkout")); !os.IsNotExist(err) {
		t.Fatalf("stale edit wrote to the new directory: %v", err)
	}
}

func TestGitignoreEditorRefusesASymlinkedIgnoreFile(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	outside := filepath.Join(t.TempDir(), "unrelated")
	if err := os.WriteFile(outside, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, ".gitignore")); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	if _, err := repo.OpenGitignore(t.Context()); err == nil {
		t.Fatal("gitignore editor followed a symbolic link")
	}
	if got := lfsRead(t, filepath.Dir(outside), filepath.Base(outside)); got != "untouched" {
		t.Fatalf("external file changed: %q", got)
	}
}

func TestHooksEditorEditsTheSelectedLinkTargetAndRejectsRetargeting(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	shared := t.TempDir()
	first, second := filepath.Join(shared, "first.sh"), filepath.Join(shared, "second.sh")
	writeFixture(t, shared, "first.sh", "# first")
	writeFixture(t, shared, "second.sh", "# second")
	if err := os.MkdirAll(filepath.Join(repo.CommonDir, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(repo.CommonDir, "hooks", "pre-commit")
	if err := os.Symlink(first, link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	hooks, err := repo.OpenHooksEditor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(first)
	if err != nil {
		t.Fatal(err)
	}
	edit, err := hooks.Open(t.Context(), "pre-commit")
	if err != nil || edit.Text() != "# first" || edit.Path() != resolved {
		t.Fatalf("open linked hook: %+v, %v", edit, err)
	}
	if err := edit.Save(t.Context(), "# chosen content"); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(link); err != nil || got != first || lfsRead(t, shared, "first.sh") != "# chosen content" {
		t.Fatalf("the edit replaced the link or missed its target: %q, %v", got, err)
	}
	edit, err = hooks.Open(t.Context(), "pre-commit")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(second, link); err != nil {
		t.Fatal(err)
	}
	if err := edit.Save(t.Context(), "# stale edit"); err == nil {
		t.Fatal("the old hook editor accepted a retargeted link")
	}
	if lfsRead(t, shared, "first.sh") != "# chosen content" || lfsRead(t, shared, "second.sh") != "# second" {
		t.Fatal("retargeting changed a script")
	}
}

func TestHooksEditorListsOtherHooksWhenOneLinkIsBroken(t *testing.T) {
	dir := newFixture(t)
	repo := openFixture(t, dir)
	hooksDir := filepath.Join(repo.CommonDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("pre-commit", filepath.Join(hooksDir, "pre-commit")); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	writeFixture(t, hooksDir, "post-checkout", "# healthy")
	hooks, err := repo.OpenHooksEditor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	entries, err := hooks.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		switch entry.Name {
		case "pre-commit":
			if entry.Exists {
				t.Fatal("a cyclic hook was reported as present")
			}
		case "post-checkout":
			if !entry.Exists {
				t.Fatal("the normal hook was not listed")
			}
		}
	}
	if _, err := hooks.Open(t.Context(), "pre-commit"); err == nil {
		t.Fatal("opening a cyclic hook must report an error")
	}
}

func TestGitignoreEditorRejectsReplacementLinkedCommonDirectory(t *testing.T) {
	main := newFixture(t)
	linked := filepath.Join(t.TempDir(), "linked")
	fixtureGit(t, main, "worktree", "add", "--detach", "--", linked)
	repo := openFixture(t, linked)
	editor, err := repo.OpenGitignore(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	previous := filepath.Join(main, "previous-metadata")
	if err := os.Rename(repo.CommonDir, previous); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, main, "init", "--quiet")
	if err := os.Rename(filepath.Join(previous, "worktrees"), filepath.Join(repo.CommonDir, "worktrees")); err != nil {
		t.Fatal(err)
	}
	fresh := openFixture(t, linked)
	if !os.SameFile(repo.gitDirInfo, fresh.gitDirInfo) || repo.SameCheckout(fresh) {
		t.Fatal("a replacement common directory was accepted as the original checkout")
	}
	if err := editor.Save(t.Context(), "/draft"); err == nil {
		t.Fatal("the editor saved into the replacement repository")
	}
}
