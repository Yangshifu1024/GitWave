//go:build darwin && !ios

package appupdate

import (
	"archive/zip"
	"context"
	"debug/macho"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"hash/crc32"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

const macTestBundleID = "desktop.gitwave.work.updatefixture"
const macTestRequirement = `identifier "` + macTestBundleID + `"`
const macFixtureXattr = "com.gitwave.update-fixture"

var macFixtureDir = flag.String("gitwave-update-fixture", "", "data directory of an isolated update fixture")

func TestMain(m *testing.M) {
	if err := WaitForUpdateParent(); err != nil {
		for _, arg := range os.Args[1:] {
			if dir, ok := strings.CutPrefix(arg, "-gitwave-update-fixture="); ok {
				_ = os.WriteFile(filepath.Join(dir, "launch-error"), []byte(err.Error()), 0o600)
			}
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestMacReleaseRequiresArm64(t *testing.T) {
	path := filepath.Join(t.TempDir(), "GitWave")
	for _, cpu := range []macho.Cpu{macho.CpuAmd64, macho.CpuArm64} {
		header, err := binary.Append(nil, binary.LittleEndian, macho.FileHeader{Magic: macho.Magic64, Cpu: cpu, Type: macho.TypeExec})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(header, 0, 0, 0, 0), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := requireMacArm64(path); (err == nil) != (cpu == macho.CpuArm64) {
			t.Fatalf("Mach-O CPU %v: %v", cpu, err)
		}
	}
}

func TestMacBundleInstallVerifiesSignerVersionAndPreservesSymlinks(t *testing.T) {
	installed := newMacFixtureBundle(t, filepath.Join(t.TempDir(), "Renamed GitWave.app"), "0.11.0", macTestBundleID)
	bound, err := bindMacBundle(t.Context(), installed, macTestBundleID, macTestRequirement)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		version, signer string
		valid           bool
	}{
		{"0.12.0", macTestBundleID + ".different", false},
		{"0.13.0", macTestBundleID, false},
		{"0.12.0", macTestBundleID, true},
	} {
		staged := newMacFixtureBundle(t, filepath.Join(t.TempDir(), macArchiveBundle), tc.version, tc.signer)
		restart, err := bound.install(t.Context(), signedUpdateFixture(t, zipMacFixtureBundle(t, staged), MacOSArm64), nil)
		if !tc.valid {
			if err == nil || restart != nil {
				t.Fatalf("installed bundle %s signed as %s", tc.version, tc.signer)
			}
			assertMacFixtureVersion(t, installed, "0.11.0")
			continue
		}
		if err != nil || restart == nil {
			t.Fatalf("install signed replacement: %v", err)
		}
		assertMacFixtureVersion(t, installed, "0.12.0")
		link, err := os.Readlink(filepath.Join(installed, "Contents", "Resources", "Current"))
		if err != nil || link != "Versions/A" {
			t.Fatalf("resource link = %q, %v", link, err)
		}
		var attr [32]byte
		n, err := unix.Getxattr(macFixtureResource(installed), macFixtureXattr, attr[:])
		if err != nil || string(attr[:n]) != "0.12.0" {
			t.Fatalf("resource attribute was lost: %v, %v", n, err)
		}
		assertMacFixtureVersion(t, macFixtureBackup(t, installed), "0.11.0")
	}
}

func TestMacBundleRetainsBothBundlesWhenSignedResourcesChange(t *testing.T) {
	for _, name := range []string{"installed", "replacement"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			installed := newMacFixtureBundle(t, filepath.Join(dir, "Installed GitWave.app"), "0.11.0", macTestBundleID)
			bound, err := bindMacBundle(t.Context(), installed, macTestBundleID, macTestRequirement)
			if err != nil {
				t.Fatal(err)
			}
			stage := filepath.Join(dir, ".gitwave-update-test")
			extracted := filepath.Join(stage, "extracted")
			replacement := newMacFixtureBundle(t, filepath.Join(extracted, macArchiveBundle), "0.12.0", macTestBundleID)
			stageInfo, err := os.Lstat(stage)
			if err != nil {
				t.Fatal(err)
			}
			newInfo, err := os.Lstat(replacement)
			if err != nil {
				t.Fatal(err)
			}
			hash, err := macCodeHash(t.Context(), replacement)
			if err != nil {
				t.Fatal(err)
			}
			modified := installed
			if name == "replacement" {
				modified = replacement
			}
			if err := os.WriteFile(macFixtureResource(modified), []byte("changed after verification"), 0o644); err != nil {
				t.Fatal(err)
			}
			parent, err := os.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer parent.Close()
			retained, err := bound.publishMacBundle(t.Context(), parent, stage, extracted, stageInfo, newInfo, "0.12.0", hash)
			if retained != replacement || !errors.Is(err, errMacInstallationChanged) {
				t.Fatalf("changed bundles were not retained: %q, %v", retained, err)
			}
			assertMacFixtureVersion(t, installed, "0.11.0")
			assertMacFixtureVersion(t, retained, "0.12.0")
			if data, err := os.ReadFile(macFixtureResource(modified)); err != nil || string(data) != "changed after verification" {
				t.Fatalf("changed resource was lost: %q, %v", data, err)
			}
		})
	}
}

func TestMacArchiveRefusesAliasedAndEscapingPaths(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
	}{
		{"traversal", map[string]string{"GitWave.app/../outside": ""}},
		{"case alias", map[string]string{"GitWave.app/Contents/a": "", "GitWave.app/Contents/A": ""}},
		{"HFS alias", map[string]string{"GitWave.app/Contents/A\u200c": ""}},
		{"HFS link alias", map[string]string{"GitWave.app/Contents/link": "target\u202a"}},
		{"symbolic link chain", map[string]string{"GitWave.app/Contents/to-root": "..", "GitWave.app/Contents/escape": "to-root/../outside"}},
		{"entry under symlink", map[string]string{"GitWave.app/Contents/Resources": "Other", "GitWave.app/Contents/Resources/file": ""}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			file, err := os.Create(filepath.Join(dir, "test.zip"))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			archive := zip.NewWriter(file)
			for name, link := range tc.files {
				header := &zip.FileHeader{Name: name}
				if link != "" {
					header.SetMode(os.ModeSymlink | 0o777)
				}
				content, err := archive.CreateHeader(header)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := content.Write([]byte(link)); err != nil {
					t.Fatal(err)
				}
			}
			if err := archive.Close(); err != nil {
				t.Fatal(err)
			}
			output, err := os.Create(filepath.Join(dir, "canonical.zip"))
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			if err := rewriteMacArchive(t.Context(), file, output); !errors.Is(err, ErrPackage) {
				t.Fatalf("unsafe archive returned %v", err)
			}
		})
	}
}

func TestMacArchiveChecksActualPayloadSize(t *testing.T) {
	dir := t.TempDir()
	file, err := os.Create(filepath.Join(dir, "test.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	archive := zip.NewWriter(file)
	data := []byte("signed content")
	entry, err := archive.CreateRaw(&zip.FileHeader{
		Name:               "GitWave.app/Contents/resource.txt",
		Method:             zip.Store,
		CompressedSize64:   uint64(len(data)),
		UncompressedSize64: uint64(len(data) - 1),
		CRC32:              crc32.ChecksumIEEE(data),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := os.Create(filepath.Join(dir, "canonical.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	if err := rewriteMacArchive(t.Context(), file, output); !errors.Is(err, ErrPackage) {
		t.Fatalf("archive with a wrong payload size returned %v", err)
	}
}

func TestMacBundleRelaunchesVerifiedExecutable(t *testing.T) {
	if os.Getenv("GITWAVE_TEST_MAC_RESTART") != "1" {
		t.Skip("enable the isolated macOS restart fixture")
	}
	t.Run("replacement waits for previous process", func(t *testing.T) {
		testMacBundleRelaunch(t, "")
	})
	t.Run("startup failure retains the previous application", func(t *testing.T) {
		testMacBundleRelaunch(t, "startup-failure")
	})
	t.Run("path change after startup cancels the restart", func(t *testing.T) {
		testMacBundleRelaunch(t, "path-change")
	})
}

func TestMacSpawnRejectsDifferentCodeBeforeExecution(t *testing.T) {
	trusted := newMacFixtureBundle(t, filepath.Join(t.TempDir(), "Trusted.app"), "0.12.0", macTestBundleID)
	other := newMacFixtureBundle(t, filepath.Join(t.TempDir(), "Other.app"), "0.12.0", macTestBundleID+".other")
	expectedHash, err := macCodeHash(t.Context(), trusted)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	args := []string{"-test.run=^TestMacSpawnFixtureProcess$", "-gitwave-update-fixture=" + dir}
	process, err := spawnVerifiedMacExecutable(macExecutable(other), expectedHash, args)
	if err == nil {
		_, _ = process.Wait()
		t.Fatal("the different executable was not rejected")
	}
	if !strings.Contains(err.Error(), "does not match") {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "spawn-executed")
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the rejected executable ran: %v", err)
	}
	process, err = spawnVerifiedMacExecutable(macExecutable(trusted), expectedHash, args)
	if err != nil {
		t.Fatal(err)
	}
	state, err := process.Wait()
	if err != nil || !state.Success() {
		t.Fatalf("the trusted executable did not exit cleanly: %v, %v", state, err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal(err)
	}
}

func TestMacSpawnFixtureProcess(t *testing.T) {
	if *macFixtureDir == "" {
		t.Skip("runs only inside an isolated fixture application")
	}
	if err := os.WriteFile(filepath.Join(*macFixtureDir, "spawn-executed"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func testMacBundleRelaunch(t *testing.T, failure string) {
	t.Helper()
	dir := t.TempDir()
	installed := newMacFixtureBundle(t, filepath.Join(dir, "Installed GitWave.app"), "0.11.0", macTestBundleID)
	packageDir := t.TempDir()
	replacement := newMacFixtureBundle(t, filepath.Join(packageDir, macArchiveBundle), "0.12.0", macTestBundleID)
	if failure == "startup-failure" {
		data, err := os.ReadFile("/usr/bin/true")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(macExecutable(replacement), data, 0o755); err != nil {
			t.Fatal(err)
		}
		signMacFixture(t, replacement, macTestBundleID)
	}
	if err := os.WriteFile(filepath.Join(dir, "restart-fixture"), []byte(failure), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "replacement.zip"), zipMacFixtureBundle(t, replacement), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, macExecutable(installed), "-test.v", "-test.run=^TestMacBundleFixtureProcess$", "-gitwave-update-fixture="+dir)
	if output, err := cmd.CombinedOutput(); err != nil {
		launchError, _ := os.ReadFile(filepath.Join(dir, "launch-error"))
		t.Fatalf("relaunch the isolated application: %v\n%s\n%s", err, output, launchError)
	}
	if failure != "" {
		if _, err := os.Stat(filepath.Join(dir, "old-remained-running")); err != nil {
			t.Fatal(err)
		}
		if failure == "path-change" {
			assertMacFixtureVersion(t, filepath.Join(dir, "Moved GitWave.app"), "0.12.0")
		} else {
			assertMacFixtureVersion(t, installed, "0.12.0")
		}
		assertMacFixtureVersion(t, macFixtureBackup(t, installed), "0.11.0")
		return
	}
	for ctx.Err() == nil {
		if got, err := os.ReadFile(filepath.Join(dir, "new-running")); err == nil && string(got) == "0.12.0" {
			assertMacFixtureVersion(t, installed, "0.12.0")
			if staging, err := filepath.Glob(filepath.Join(dir, ".gitwave-update-*")); err != nil || len(staging) != 0 {
				t.Fatalf("finished update left staging directories: %v, %v", staging, err)
			}
			return
		}
		if data, err := os.ReadFile(filepath.Join(dir, "launch-error")); err == nil {
			t.Fatalf("replacement process did not finish: %s", data)
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatal("replacement process did not finish before the timeout")
}

func TestMacBundleFixtureProcess(t *testing.T) {
	dir := *macFixtureDir
	if dir == "" {
		t.Skip("runs only inside an isolated fixture application")
	}
	installed := filepath.Join(dir, "Installed GitWave.app")
	version, err := macBundleProperty(t.Context(), installed, "CFBundleShortVersionString")
	if err != nil {
		t.Fatal(err)
	}
	if version == "0.12.0" {
		if _, err := os.Stat(filepath.Join(dir, "old-exiting")); err != nil {
			t.Fatal("replacement entered the application before its predecessor exited")
		}
		exe, err := os.Executable()
		if err != nil || exe != macExecutable(installed) {
			t.Fatalf("the replacement opened at %q, %v", exe, err)
		}
		if group, err := unix.Getpgid(0); err != nil || group != os.Getpid() {
			t.Fatalf("the replacement did not start in its own process group: %d, %v", group, err)
		}
		if _, err := fmt.Fprintln(os.Stdout, "replacement started"); err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Fprintln(os.Stderr, "replacement started"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "new-running"), []byte(version), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	bound, err := bindMacBundle(t.Context(), installed, macTestBundleID, macTestRequirement)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "replacement.zip"))
	if err != nil {
		t.Fatal(err)
	}
	restart, err := bound.install(t.Context(), signedUpdateFixture(t, data, MacOSArm64), nil)
	if err != nil {
		t.Fatal(err)
	}
	failure, err := os.ReadFile(filepath.Join(dir, "restart-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	if string(failure) == "startup-failure" {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		if finish, err := restart(ctx); err == nil || finish != nil || !strings.Contains(err.Error(), "was retained at") {
			t.Fatalf("failed restart did not report the retained bundle: %v", err)
		}
		assertMacFixtureVersion(t, macFixtureBackup(t, installed), "0.11.0")
		if err := os.WriteFile(filepath.Join(dir, "old-remained-running"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	finish, err := restart(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if string(failure) == "path-change" {
		if err := os.Rename(installed, filepath.Join(dir, "Moved GitWave.app")); err != nil {
			t.Fatal(err)
		}
		if err := finish(true); !errors.Is(err, errMacInstallationChanged) {
			t.Fatalf("restart was not canceled after the installation changed: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "old-remained-running"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := finish(true); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if _, err := os.Stat(filepath.Join(dir, "new-running")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the replacement started before the old application had closed")
	}
	if err := os.WriteFile(filepath.Join(dir, "old-exiting"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

func newMacFixtureBundle(t *testing.T, path, version, signer string) string {
	t.Helper()
	contents := filepath.Join(path, "Contents")
	for _, dir := range []string{filepath.Dir(macExecutable(path)), filepath.Join(contents, "Resources", "Versions", "A")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("Versions/A", filepath.Join(contents, "Resources", "Current")); err != nil {
		t.Fatal(err)
	}
	resource := macFixtureResource(path)
	if err := os.WriteFile(resource, []byte(version), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := unix.Setxattr(resource, macFixtureXattr, []byte(version), 0); err != nil {
		t.Fatal(err)
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>%s</string>
<key>CFBundleName</key><string>GitWave Update Fixture</string>
<key>CFBundleExecutable</key><string>GitWave</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>%s</string>
<key>CFBundleVersion</key><string>%s</string>
<key>LSBackgroundOnly</key><true/>
</dict></plist>`, macTestBundleID, version, version)
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(macExecutable(path), data, 0o755); err != nil {
		t.Fatal(err)
	}
	signMacFixture(t, path, signer)
	return path
}

func macFixtureResource(bundle string) string {
	return filepath.Join(bundle, "Contents", "Resources", "Versions", "A", "fixture.txt")
}

func signMacFixture(t *testing.T, path, signer string) {
	t.Helper()
	if _, err := macTool(t.Context(), "/usr/bin/codesign", "--force", "--sign", "-", "--identifier", signer, "--", path); err != nil {
		t.Fatal(err)
	}
}

func zipMacFixtureBundle(t *testing.T, path string) []byte {
	t.Helper()
	zip := filepath.Join(t.TempDir(), "bundle.zip")
	if _, err := macTool(t.Context(), "/usr/bin/ditto", "-c", "-k", "--sequesterRsrc", "--keepParent", path, zip); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(zip)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertMacFixtureVersion(t *testing.T, path, want string) {
	t.Helper()
	got, err := macBundleProperty(t.Context(), path, "CFBundleShortVersionString")
	if err != nil || got != want {
		t.Fatalf("bundle version = %q, %v, want %q", got, err, want)
	}
}

func macFixtureBackup(t *testing.T, installed string) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(filepath.Dir(installed), ".gitwave-update-*", "extracted", macArchiveBundle))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected one recovery bundle: %v, %v", files, err)
	}
	return files[0]
}
