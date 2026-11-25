//go:build windows

package appupdate

import (
	"crypto/rand"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func TestWindowsUpdateSnapshotDetectsAPathReplacedAfterCapture(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, windowsAppName)
	writeInstallFixture(t, path, "previous application")
	expected, err := windowsFileSnapshot(path)
	if err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "previous.exe")
	if err := os.Rename(path, old); err != nil {
		t.Fatal(err)
	}
	writeInstallFixture(t, path, "new application")
	if !windowsPathStillIs(old, expected) || windowsPathStillIs(path, expected) {
		t.Fatal("the snapshot did not follow the original file's identity")
	}
}

func TestWindowsSetupMigratesTheMatchingLegacyRegistration(t *testing.T) {
	for _, hasLegacyChild := range []bool{false, true} {
		name := `Software\GitWave-Setup-Test-` + rand.Text()
		parent, _, err := registry.CreateKey(registry.CURRENT_USER, name, registry.ALL_ACCESS)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = registry.DeleteKey(parent, legacyRegistration+`\fixture`)
			_ = registry.DeleteKey(parent, legacyRegistration)
			_ = registry.DeleteKey(parent, stableRegistration)
			_ = parent.Close()
			_ = registry.DeleteKey(registry.CURRENT_USER, name)
		})
		destination, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		old, _, err := registry.CreateKey(parent, legacyRegistration, registry.ALL_ACCESS)
		if err != nil {
			t.Fatal(err)
		}
		if err := old.SetStringValue("InstallLocation", `"`+destination+`"`); err != nil {
			t.Fatal(err)
		}
		if hasLegacyChild {
			child, _, err := registry.CreateKey(old, "fixture", registry.ALL_ACCESS)
			if err != nil {
				t.Fatal(err)
			}
			child.Close()
		}
		old.Close()
		err = publishWindowsRegistration(parent, stableRegistration, destination, "0.12.0", true)
		oldExists, _, oldErr := windowsRegistrationMatches(parent, legacyRegistration, destination)
		currentExists, currentMatches, currentErr := windowsRegistrationMatches(parent, stableRegistration, destination)
		if oldErr != nil || currentErr != nil {
			t.Fatal(errors.Join(oldErr, currentErr))
		}
		if hasLegacyChild {
			if err == nil || !oldExists || currentExists {
				t.Fatalf("failed legacy removal was not rolled back: %v, current=%t, previous=%t", err, currentExists, oldExists)
			}
		} else if err != nil || oldExists || !currentMatches {
			t.Fatalf("did not migrate the old uninstall entry: %v, current=%t, previous=%t", err, currentMatches, oldExists)
		}
	}
}

const testWindowsWaitArg = "--gitwave-test-wait="

func TestMain(m *testing.M) {
	if len(os.Args) > 1 {
		if value, ok := strings.CutPrefix(os.Args[1], testWindowsWaitArg); ok {
			handle, err := windowsInheritedProcess(value)
			if err == nil {
				err = awaitWindowsUpdateProcess(handle, 10*time.Second)
			}
			if err == nil {
				err = os.WriteFile(os.Args[2], []byte(os.Args[3]), 0o600)
			}
			if err != nil {
				os.Exit(1)
			}
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}

func TestWindowsUpdateStageLocksFilesAgainstReplacement(t *testing.T) {
	stage, err := makeWindowsUpdateStage()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { removeWindowsUpdateStage(stage) })
	if !validWindowsStageName(filepath.Base(stage)) {
		t.Fatalf("invalid stage name: %q", stage)
	}
	path := filepath.Join(stage, windowsSetupName)
	if err := os.WriteFile(path, []byte("test package"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := lockWindowsUpdateFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o600); err == nil {
		t.Error("update package could be modified while it was open")
	}
	if err := os.Remove(path); err == nil {
		t.Error("update package could be removed while it was open")
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func TestWindowsUpdateRejectsWrongDigestAndUnsignedSetup(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := DescribePackage(path)
	if err != nil {
		t.Fatal(err)
	}
	wrong := pkg
	wrong.SHA256 = strings.Repeat("0", 64)
	if file, err := openVerifiedWindowsSetup(path, wrong); !errors.Is(err, ErrPackage) {
		if file != nil {
			file.Close()
		}
		t.Fatalf("wrong digest returned %v", err)
	}
	if file, err := openVerifiedWindowsSetup(path, pkg); err == nil || !strings.Contains(err.Error(), "publisher signature") {
		if file != nil {
			file.Close()
		}
		t.Fatalf("unsigned Go test executable returned %v", err)
	}
}

func TestWindowsUpdateAllowsOnlyOneWorkerPerInstallation(t *testing.T) {
	path := filepath.Join(t.TempDir(), windowsAppName)
	first, err := lockWindowsInstallationUpdate(path)
	if err != nil {
		t.Fatal(err)
	}
	concurrent, err := lockWindowsInstallationUpdate(strings.ToUpper(path))
	if err == nil {
		windows.CloseHandle(concurrent)
		t.Error("two workers acquired the same installation")
	}
	windows.CloseHandle(first)
	next, err := lockWindowsInstallationUpdate(path)
	if err != nil {
		t.Fatal(err)
	}
	windows.CloseHandle(next)
}

func TestWindowsUpdateHandoffWaitsForThePreviousProcess(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "new process.txt")
	previous := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestWindowsUpdateHandoffFixture$", "-test.v")
	previous.Env = append(os.Environ(), "GITWAVE_TEST_WINDOWS_WAIT="+marker)
	if output, err := previous.CombinedOutput(); err != nil {
		t.Fatalf("old app fixture: %v\n%s", err, output)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if result, err := os.ReadFile(marker); err == nil {
			if string(result) != `a quoted "value"` {
				t.Fatalf("restart arguments changed: %q", result)
			}
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("the new process did not start after the previous one exited")
}

func TestWindowsUpdateHandoffFixture(t *testing.T) {
	marker := os.Getenv("GITWAVE_TEST_WINDOWS_WAIT")
	if marker == "" {
		t.Skip("runs inside the isolated process handoff fixture")
	}
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd, handle, err := windowsHandoffCommand(path, testWindowsWaitArg, marker, `a quoted "value"`)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Process.Release(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the replacement started before the old process exited: %v", err)
	}
}
