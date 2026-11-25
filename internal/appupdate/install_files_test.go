package appupdate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitwave.work/internal/gittool"
)

func TestInstallFilesRestoresThePreviousPackageOnFailure(t *testing.T) {
	for _, failure := range []string{"none", "file publication", "registration", "concurrent replacement"} {
		t.Run(failure, func(t *testing.T) {
			payload, destination := t.TempDir(), t.TempDir()
			stage, err := os.MkdirTemp(destination, ".gitwave-install-")
			if err != nil {
				t.Fatal(err)
			}
			const app, uninstaller = "GitWave.exe", "Uninstall.exe"
			for _, name := range []string{app, uninstaller} {
				writeInstallFixture(t, filepath.Join(payload, name), "new "+name)
				writeInstallFixture(t, filepath.Join(destination, name), "previous "+name)
			}
			writeInstallFixture(t, filepath.Join(destination, "user-file.txt"), "user data")
			failureErr := errors.New("fixture installation failure")
			move := func(source, target string) error {
				if failure == "file publication" && filepath.Base(source) == uninstaller+".new" {
					return failureErr
				}
				return moveInstallFixture(source, target)
			}
			commit := func() error {
				if failure == "none" {
					return nil
				}
				if failure == "concurrent replacement" {
					path := filepath.Join(destination, app)
					if err := os.Rename(path, filepath.Join(destination, "other GitWave.exe")); err != nil {
						return err
					}
					writeInstallFixture(t, path, "concurrent GitWave.exe")
				}
				return failureErr
			}
			err = installFiles(payload, destination, stage, []string{app, uninstaller}, move, commit)
			if failure == "none" && err != nil || failure != "none" && !errors.Is(err, failureErr) {
				t.Fatalf("install result: %v", err)
			}
			assertInstallFixture(t, filepath.Join(destination, "user-file.txt"), "user data")
			switch failure {
			case "none":
				assertInstallFixture(t, filepath.Join(destination, app), "new "+app)
				assertInstallFixture(t, filepath.Join(destination, uninstaller), "new "+uninstaller)
			case "concurrent replacement":
				if !strings.Contains(err.Error(), stage) {
					t.Fatalf("error did not identify the recovery directory: %v", err)
				}
				assertInstallFixture(t, filepath.Join(destination, app), "concurrent "+app)
				assertInstallFixture(t, filepath.Join(stage, app+".old"), "previous "+app)
				assertInstallFixture(t, filepath.Join(destination, uninstaller), "previous "+uninstaller)
			default:
				assertInstallFixture(t, filepath.Join(destination, app), "previous "+app)
				assertInstallFixture(t, filepath.Join(destination, uninstaller), "previous "+uninstaller)
			}
			if _, err := os.Stat(stage); failure != "concurrent replacement" && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("temporary files remained after the operation: %v", err)
			}
		})
	}
}

func TestInstallFilesRollsBackTheGitBundleAndPreservesExtraFiles(t *testing.T) {
	for _, mode := range []string{"publish", "old file", "old configuration", "new file"} {
		payload, destination := t.TempDir(), t.TempDir()
		for _, dir := range []string{payload, destination} {
			bundle := filepath.Join(dir, gittool.Directory)
			for _, entry := range []string{"cmd/git.exe", "ucrt64/bin/git-lfs.exe", "ucrt64/bin/git-credential-manager.exe",
				"ucrt64/bin/git-remote-https.exe", "usr/bin/ssh.exe", "usr/bin/ssh-add.exe", "etc/gitconfig"} {
				path := filepath.Join(bundle, filepath.FromSlash(entry))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				writeInstallFixture(t, path, dir)
			}
			if err := gittool.WriteWindowsInventory(bundle); err != nil {
				t.Fatal(err)
			}
		}
		const uninstaller = "Uninstall.exe"
		writeInstallFixture(t, filepath.Join(payload, uninstaller), "new uninstaller")
		writeInstallFixture(t, filepath.Join(destination, uninstaller), "old uninstaller")
		extra := filepath.Join(destination, gittool.Directory, "user-file.txt")
		config := filepath.Join(destination, gittool.Directory, "etc", "gitconfig")
		if mode == "old file" {
			writeInstallFixture(t, extra, "user data")
		}
		if mode == "old configuration" {
			writeInstallFixture(t, config, "user configuration")
		}
		stage := t.TempDir()
		failure := errors.New("cannot replace the old uninstaller")
		git := filepath.Join(destination, gittool.Directory, "cmd", "git.exe")
		move := func(source, target string) error {
			if mode == "publish" && filepath.Base(source) == uninstaller+".new" {
				assertInstallFixture(t, git, payload)
				return failure
			}
			return moveInstallFixture(source, target)
		}
		err := installFiles(payload, destination, stage, []string{gittool.Directory, uninstaller}, move, func() error {
			if mode != "new file" {
				t.Fatal("installation should not be registered")
			}
			writeInstallFixture(t, extra, "user data")
			return failure
		})
		switch mode {
		case "old file":
			if err == nil || !strings.Contains(err.Error(), "user-file.txt") {
				t.Fatalf("installer did not detect an unowned file: %v", err)
			}
			assertInstallFixture(t, extra, "user data")
		case "old configuration":
			if err == nil || !strings.Contains(err.Error(), "was modified: etc/gitconfig") {
				t.Fatalf("installer did not detect the modified configuration: %v", err)
			}
			assertInstallFixture(t, config, "user configuration")
		case "new file":
			if !errors.Is(err, failure) || !strings.Contains(err.Error(), stage) {
				t.Fatalf("rollback did not identify the recoverable folder: %v", err)
			}
			assertInstallFixture(t, filepath.Join(stage, gittool.Directory+".new", "user-file.txt"), "user data")
		case "publish":
			if !errors.Is(err, failure) {
				t.Fatalf("wrong installation failure: %v", err)
			}
		}
		assertInstallFixture(t, git, destination)
		assertInstallFixture(t, filepath.Join(destination, uninstaller), "old uninstaller")
	}
}

func moveInstallFixture(source, destination string) error {
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(os.ErrExist, err)
	}
	return os.Rename(source, destination)
}

func writeInstallFixture(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertInstallFixture(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("%s = %q, %v; want %q", path, got, err, want)
	}
}
