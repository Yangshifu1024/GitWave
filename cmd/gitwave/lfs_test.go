package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestNativeLFSCheckoutHonorsPathScope(t *testing.T) {
	if _, err := exec.LookPath("git-lfs"); err != nil {
		t.Skip("Git LFS CLI is unavailable on PATH")
	}
	dir, git := historyRepository(t)
	git("", "config", "filter.lfs.process", "git-lfs filter-process")
	git("", "config", "filter.lfs.required", "true")
	const asset = "same locally cached media\n"
	const included, excluded = "included/asset.bin", "excluded/asset.bin"
	writeRepositoryFixtureFile(t, dir, ".gitattributes", "*.bin filter=lfs diff=lfs merge=lfs -text\n")
	git("", "add", ".gitattributes")
	git("", "commit", "-m", "Base checkout without assets")
	git("", "switch", "-c", "with-assets")
	writeRepositoryFixtureFile(t, dir, included, asset)
	writeRepositoryFixtureFile(t, dir, excluded, asset)
	writeRepositoryFixtureFile(t, dir, ".lfsconfig", "[lfs]\nfetchinclude = included/\n")
	git("", "add", ".")
	git("", "commit", "-m", "Two paths for the same asset")
	pointer := git("", "show", "HEAD:"+excluded) + "\n"
	git("", "switch", "main")
	app := refTestApp(t, dir)
	const timeout = 12 * time.Second
	app.WaitFor("label:with-assets", timeout)
	app.RightClick("label:with-assets")
	app.ChoosePopup("Checkout")
	app.WaitFor("text:Checked out with-assets", timeout)
	for name, want := range map[string]string{included: asset, excluded: pointer} {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil || string(data) != want {
			t.Fatalf("checkout %s = %q, %v", name, data, err)
		}
	}
}

func TestNativeLFSPullMaterializesDownloadedMedia(t *testing.T) {
	if _, err := exec.LookPath("git-lfs"); err != nil {
		t.Skip("Git LFS CLI is unavailable on PATH")
	}
	source, git := historyRepository(t)
	git("", "config", "--local", "filter.lfs.process", "git-lfs filter-process")
	git("", "config", "--local", "filter.lfs.required", "true")
	if err := os.WriteFile(filepath.Join(source, ".gitattributes"),
		[]byte("*.asset filter=lfs diff=lfs merge=lfs -text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("", "add", "--", ".gitattributes")
	git("", "commit", "-m", "Base before pulling media")
	client, gitClient := historyRepository(t)
	gitClient("", "config", "--local", "filter.lfs.process", "git-lfs filter-process")
	gitClient("", "config", "--local", "filter.lfs.required", "true")
	gitClient("", "remote", "add", "origin", source)
	gitClient("", "fetch", "origin")
	gitClient("", "reset", "--hard", "origin/main")
	app := refTestApp(t, client)
	const timeout = 12 * time.Second
	app.WaitFor("label:/^Base before pulling media,/", timeout)

	const name = "new texture.asset"
	const payload = "the remote's real media bytes\x00"
	if err := os.WriteFile(filepath.Join(source, name), []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	git("", "add", "--", name)
	git("", "commit", "-m", "Pull the new asset")
	app.Press("id:pull")
	app.WaitFor("id:pull-dialog", timeout)
	app.WaitUntil(timeout, func() bool { return app.Enabled("id:pull-confirm") })
	app.Press("id:pull-confirm")
	app.WaitFor("text:Pulled from origin", timeout)
	app.WaitFor("label:/^Pull the new asset,/", timeout)
	if contents, err := os.ReadFile(filepath.Join(client, name)); err != nil || string(contents) != payload {
		t.Fatalf("Pull did not hydrate the received media: %q, %v", contents, err)
	}
	if got, want := gitClient("", "rev-parse", "HEAD"), git("", "rev-parse", "HEAD"); got != want {
		t.Fatalf("Pull left HEAD at %s, want %s", got, want)
	}
}

func TestNativeLFSInstallTrackAndUntrack(t *testing.T) {
	dir, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Base for LFS")
	path := filepath.Join(dir, ".gitattributes")
	const original = "# user rules\n*.txt text\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	app := refTestApp(t, dir)
	const timeout = 12 * time.Second
	app.WaitFor("label:/^Base for LFS,/", timeout)
	app.ChooseMenu("Repository", "LFS")
	app.WaitFor("id:lfs-dialog", timeout)
	app.WaitUntil(timeout, func() bool { return app.Exists("id:lfs-install") || app.Exists("id:lfs-cli-unavailable") })
	if app.Exists("id:lfs-install") {
		app.Press("id:lfs-install")
		app.WaitFor("id:lfs-installed", timeout)
		if got := git("", "config", "--local", "--get", "filter.lfs.process"); got != "git-lfs filter-process" {
			t.Fatalf("installation did not configure this repository: %q", got)
		}
	}
	app.Type("id:lfs-pattern", "directory/")
	app.Press("id:lfs-track")
	app.WaitFor("id:lfs-error", timeout)
	if got, err := os.ReadFile(path); err != nil || string(got) != original {
		t.Fatalf("invalid input changed the attributes: %q, %v", got, err)
	}
	app.Clear("id:lfs-pattern")
	app.Type("id:lfs-pattern", "large assets/**")
	app.WaitUntil(timeout, func() bool { return app.Find("id:lfs-pattern").Text == "large assets/**" })
	app.Submit("id:lfs-pattern")
	app.WaitFor("id:lfs-untrack", timeout)
	app.WaitUntil(timeout, func() bool { return app.Enabled("id:lfs-untrack") })
	if got := git("", "check-attr", "filter", "--", "large assets/file.psd"); got != "large assets/file.psd: filter: lfs" {
		t.Fatalf("Git did not recognize the tracked rule: %q", got)
	}
	app.Press("label:Stop tracking large assets/**")
	app.WaitForGone("id:lfs-untrack", timeout)
	app.Press("id:lfs-close")
	app.WaitForGone("id:lfs-dialog", timeout)
	if got, err := os.ReadFile(path); err != nil || string(got) != original {
		t.Fatalf("untrack modified the independent attributes: %q, %v", got, err)
	}
}

func TestNativeLFSStageFromChanges(t *testing.T) {
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	dir, git := historyRepository(t)
	git("", "config", "--local", "filter.lfs.process", "git-lfs filter-process")
	git("", "config", "--local", "filter.lfs.required", "true")
	if err := os.WriteFile(filepath.Join(dir, ".gitattributes"),
		[]byte("*.asset filter=lfs diff=lfs merge=lfs -text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("", "add", "--", ".gitattributes")
	git("", "commit", "-m", "Base for LFS Changes")
	const name = "texture.asset"
	const data = "asset contents with \x00binary data\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	app := refTestApp(t, dir)
	const timeout = 12 * time.Second
	app.WaitFor("label:/^Base for LFS Changes,/", timeout)
	app.Press("id:navigate-changes")
	app.WaitFor("label:Stage texture.asset", timeout)
	app.Press("label:Stage texture.asset")
	app.WaitFor("text:Staged (1)", timeout)
	app.WaitFor("text:Unstaged (0)", timeout)
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
	want := fmt.Sprintf("version https://git-lfs.github.com/spec/v1\noid sha256:%s\nsize %d", sum, len(data))
	if got := git("", "show", ":"+name); got != want {
		t.Fatalf("Changes staged %q, want %q", got, want)
	}
	if contents, err := os.ReadFile(filepath.Join(dir, name)); err != nil || string(contents) != data {
		t.Fatalf("stage changed the original working file: %q, %v", contents, err)
	}
}

func TestNativeLFSPushSendsCommittedMedia(t *testing.T) {
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	client, git := historyRepository(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	git("", "init", "--quiet", "--bare", "-b", "main", remote)
	git("", "remote", "add", "origin", remote)
	git("", "config", "--file", ".lfsconfig", "lfs.pushurl", remote)
	git("", "config", "--local", "filter.lfs.process", "git-lfs filter-process")
	git("", "config", "--local", "filter.lfs.required", "true")
	if err := os.WriteFile(filepath.Join(client, ".gitattributes"), []byte("*.asset filter=lfs diff=lfs merge=lfs -text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("", "add", "--", ".gitattributes")
	git("", "commit", "-m", "Base for LFS Push")
	const name = "sound.asset"
	const media = "LFS Push writes these bytes\x00"
	if err := os.WriteFile(filepath.Join(client, name), []byte(media), 0o600); err != nil {
		t.Fatal(err)
	}
	app := refTestApp(t, client)
	const timeout = 12 * time.Second
	app.WaitFor("label:/^Base for LFS Push,/", timeout)
	app.Press("id:navigate-changes")
	app.WaitFor("label:Stage sound.asset", timeout)
	app.Press("label:Stage sound.asset")
	app.WaitFor("text:Staged (1)", timeout)
	app.Type("id:commit-message", "Commit sound asset")
	app.Press("id:write-commit")
	app.WaitFor("id:commit-success", timeout)
	app.Press("id:push")
	app.WaitFor("id:push-dialog", timeout)
	app.WaitFor("id:push-lfs-destination", timeout)
	app.WaitUntil(timeout, func() bool { return app.Enabled("id:push-confirm") })
	app.Press("id:push-confirm")
	app.WaitFor("text:Pushed to origin", timeout)
	if got := git("", "--git-dir", remote, "rev-parse", "HEAD"); got != git("", "rev-parse", "HEAD") {
		t.Fatalf("the remote branch did not reach the committed LFS pointer: %q", got)
	}
	oid := fmt.Sprintf("%x", sha256.Sum256([]byte(media)))
	path := filepath.Join(remote, "lfs", "objects", oid[:2], oid[2:4], oid)
	if contents, err := os.ReadFile(path); err != nil || string(contents) != media {
		t.Fatalf("Push did not upload the committed media: %q, %v", contents, err)
	}
}
