package appupdate

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAppImageInstallPublishesOnlyTheVerifiedReplacement(t *testing.T) {
	old := appImageBytes("previous version")
	next := appImageBytes("new verified version")
	path := writeAppImage(t, t.TempDir(), "GitWave.AppImage", old)
	image, err := bindAppImage(path)
	if err != nil {
		t.Fatal(err)
	}
	bad := appImageBytes("signed file for a different architecture")
	bad[18] = 183
	if restart, err := image.Install(t.Context(), signedUpdateFixture(t, bad, LinuxX64), nil); !errors.Is(err, ErrPackage) || restart != nil {
		t.Fatalf("installed an invalid image: %v", err)
	}
	assertImageBytes(t, path, old)
	restart, err := image.Install(t.Context(), signedUpdateFixture(t, next, LinuxX64), nil)
	if err != nil || restart == nil {
		t.Fatalf("did not install the valid AppImage: %v", err)
	}
	assertImageBytes(t, path, next)
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o750 {
		t.Fatalf("installed image permissions = %v, %v", info, err)
	}
	assertAppImageBackup(t, filepath.Dir(path), old)
}

func TestAppImageInstallPreservesAConcurrentReplacement(t *testing.T) {
	dir := t.TempDir()
	old := appImageBytes("previous version")
	official := appImageBytes("approved update")
	local := appImageBytes("a separate local replacement")
	path := writeAppImage(t, dir, "GitWave.AppImage", old)
	updatedElsewhere := writeAppImage(t, dir, "other.AppImage", local)
	image, err := bindAppImage(path)
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	_, err = image.Install(t.Context(), signedUpdateFixture(t, official, LinuxX64), func(received, total int64) {
		if received == total {
			once.Do(func() {
				if err := os.Rename(updatedElsewhere, path); err != nil {
					t.Fatal(err)
				}
			})
		}
	})
	if !errors.Is(err, ErrInstallationChanged) {
		t.Fatalf("changed image was not detected: %v", err)
	}
	assertImageBytes(t, path, local)
}

func appImageBytes(body string) []byte {
	header := []byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0, 'A', 'I', 2, 0, 0, 0, 0, 0, 3, 0, 62, 0}
	return append(header, []byte(body)...)
}

func writeAppImage(t *testing.T, dir, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o750); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertImageBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	if contents, err := os.ReadFile(path); err != nil || !bytes.Equal(contents, want) {
		t.Fatalf("AppImage = %q, %v, want %q", contents, err, want)
	}
}

func assertAppImageBackup(t *testing.T, dir string, want []byte) {
	t.Helper()
	stages, err := filepath.Glob(filepath.Join(dir, ".gitwave-update-[0-9]*"))
	if err != nil || len(stages) != 1 {
		t.Fatalf("expected one recovery directory: %v, %v", stages, err)
	}
	entries, err := os.ReadDir(stages[0])
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected one recovery image: %v, %v", entries, err)
	}
	assertImageBytes(t, filepath.Join(stages[0], entries[0].Name()), want)
}

func TestAppImageRelaunchesAfterThePreviousProcessExits(t *testing.T) {
	before, after := os.Getenv("GITWAVE_TEST_APPIMAGE_BEFORE"), os.Getenv("GITWAVE_TEST_APPIMAGE_AFTER")
	if before == "" || after == "" {
		t.Skip("set paths to the isolated AppImage test bundles")
	}
	for _, tc := range []struct{ name, replacement, marker string }{
		{"opens after the previous process", after, "new version"},
		{"keeps the previous image if launching fails", os.Getenv("GITWAVE_TEST_APPIMAGE_BROKEN"), "old process survived"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.replacement == "" {
				t.Skip("no broken AppImage fixture")
			}
			runAppImageRelaunchFixture(t, before, tc.replacement, tc.marker)
		})
	}
}

func runAppImageRelaunchFixture(t *testing.T, before, after, markerText string) {
	t.Helper()
	oldBytes, err := os.ReadFile(before)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	installed := writeAppImage(t, dir, "GitWave.AppImage", oldBytes)
	marker := filepath.Join(dir, "relaunched")
	cmd := exec.CommandContext(t.Context(), installed, "-test.v", "-test.run=^TestAppImageFixtureProcess$")
	cmd.Env = append(os.Environ(), "APPIMAGE_EXTRACT_AND_RUN=1", "TMPDIR="+dir,
		"GITWAVE_TEST_APPIMAGE_FIXTURE=1", "GITWAVE_TEST_APPIMAGE_MARKER="+marker,
		"GITWAVE_TEST_APPIMAGE_AFTER="+after, "GITWAVE_TEST_APPIMAGE_EXPECT="+markerText)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run both versions of the AppImage: %v\n%s", err, output)
	}
	if contents, err := os.ReadFile(marker); err != nil || string(contents) != markerText {
		t.Fatalf("replacement did not finish relaunch: %q, %v\n%s", contents, err, output)
	}
	newBytes, err := os.ReadFile(after)
	if err != nil {
		t.Fatal(err)
	}
	assertImageBytes(t, installed, newBytes)
	if markerText == "old process survived" {
		assertAppImageBackup(t, dir, oldBytes)
	} else if stages, err := filepath.Glob(filepath.Join(dir, ".gitwave-update-[0-9]*")); err != nil || len(stages) != 0 {
		t.Fatalf("restart did not clean up the completed installation: %v, %v", stages, err)
	}
}

func TestAppImageFixtureProcess(t *testing.T) {
	if os.Getenv("GITWAVE_TEST_APPIMAGE_FIXTURE") != "1" {
		t.Skip("only run inside an isolated AppImage fixture")
	}
	if err := WaitForUpdateParent(); err != nil {
		t.Fatal(err)
	}
	version, err := os.ReadFile(filepath.Join(os.Getenv("APPDIR"), "fixture-release"))
	if err != nil {
		t.Fatal(err)
	}
	marker := os.Getenv("GITWAVE_TEST_APPIMAGE_MARKER")
	if string(version) == "new version" {
		if _, err := os.Stat(marker + ".exiting"); err != nil {
			t.Fatal("replacement started before the previous process was ready to exit")
		}
		if err := os.WriteFile(marker, version, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	if string(version) != "old version" {
		t.Fatalf("unexpected fixture release: %q", version)
	}
	image := CurrentAppImage()
	if image == nil {
		t.Fatal("running AppImage was not detected")
	}
	payload, err := os.ReadFile(os.Getenv("GITWAVE_TEST_APPIMAGE_AFTER"))
	if err != nil {
		t.Fatal(err)
	}
	restart, err := image.Install(t.Context(), signedUpdateFixture(t, payload, LinuxX64), nil)
	if err != nil {
		t.Fatal(err)
	}
	finish, err := restart(t.Context())
	if os.Getenv("GITWAVE_TEST_APPIMAGE_EXPECT") == "old process survived" {
		if err == nil || finish != nil || !strings.Contains(err.Error(), "the replacement AppImage did not start") {
			t.Fatalf("unbootable replacement received a successful handoff: %v", err)
		}
		if err := os.WriteFile(marker, []byte("old process survived"), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(350 * time.Millisecond)
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("replacement opened before the installed process exited")
	}
	if err := os.WriteFile(marker+".exiting", nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := finish(true); err != nil {
		t.Fatal(err)
	}
}
