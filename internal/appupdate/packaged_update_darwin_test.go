//go:build darwin && !ios

package appupdate

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gitwave.work/internal/workspace"
	"golang.org/x/sys/unix"
)

func TestMacPackagedUpdateRelaunchesWorkspace(t *testing.T) {
	previous := os.Getenv("GITWAVE_TEST_MAC_PREVIOUS_ZIP")
	target := os.Getenv("GITWAVE_TEST_MAC_TARGET_ZIP")
	if previous == "" && target == "" {
		t.Skip("provide both macOS update ZIPs to exercise the packaged applications")
	}
	previousVersion := os.Getenv("GITWAVE_TEST_MAC_PREVIOUS_VERSION")
	targetVersion := os.Getenv("GITWAVE_TEST_MAC_TARGET_VERSION")
	if !filepath.IsAbs(previous) || !filepath.IsAbs(target) || previousVersion == "" || targetVersion == "" {
		t.Fatal("provide absolute macOS ZIP paths and both release versions")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	team := ""
	if publishedFeed := os.Getenv("GITWAVE_TEST_MAC_PREVIOUS_FEED"); publishedFeed != "" {
		verifyPackagedMacPredecessor(t, publishedFeed, previous, previousVersion)
		team = MacOSDeveloperIDTeam
	}
	// Hosted macOS runner temporary files can live inside its real home.
	work, err := os.MkdirTemp("/private/tmp", "gitwave-packaged-update-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(work); err != nil {
			t.Error(err)
		}
	})
	input, err := os.Open(previous)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	extracted, err := extractMacBundle(ctx, work, input)
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(extracted, macArchiveBundle)
	entry := macExecutable(bundle)
	predecessor := macBundle{id: DesktopAppID, requirement: packagedMacRequirement(team)}
	if err := predecessor.verifyMacUpdate(ctx, bundle, previousVersion); err != nil {
		t.Fatal(err)
	}
	if err := requireMacArm64(entry); err != nil {
		t.Fatal(err)
	}
	oldHash, err := macCodeHash(ctx, bundle)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopPackagedMacApp(entry) })
	home := filepath.Join(work, "profile")
	db := filepath.Join(home, "Library", "Application Support", "GitWave", "state.db")
	state, err := workspace.OpenStable(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	repo := filepath.Join(work, "retained repo")
	initialize := exec.CommandContext(ctx, "/usr/bin/git", "init", "--quiet", "--template=", "-b", "main", repo)
	initialize.Env = []string{"PATH=/usr/bin:/bin", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null"}
	if output, err := initialize.CombinedOutput(); err != nil {
		t.Fatalf("initialize repository fixture: %v\n%s", err, output)
	}
	fixture, err := state.CreateWithRepos(ctx, "Retained through update", []string{repo})
	if err != nil {
		t.Fatal(err)
	}
	if err := state.SaveAutoUpdate(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	window := filepath.Join(work, "check-window")
	if _, err := macTool(ctx, "/usr/bin/xcrun", "swiftc", "-module-cache-path", filepath.Join(work, "swift-cache"),
		"-o", window, filepath.Join("..", "..", "scripts", "check-macos-window.swift")); err != nil {
		t.Fatal(err)
	}
	hostHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	hostHome, err = filepath.EvalSymlinks(hostHome)
	if err != nil {
		t.Fatal(err)
	}
	// Block IP connections while allowing the updater's private Unix handoff.
	const profile = `(version 1)
(allow default)
(deny network-outbound (remote ip "*:*"))
(deny file-read* file-write* (subpath (param "GITWAVE_TEST_HOST_HOME")))`
	sandbox := []string{"-D", "GITWAVE_TEST_HOST_HOME=" + hostHome, "-p", profile}
	env := []string{"HOME=" + home, "CFFIXED_USER_HOME=" + home, "TMPDIR=" + work + "/", "PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	old := exec.CommandContext(ctx, "/usr/bin/sandbox-exec", append(sandbox, entry)...)
	old.Env = env
	oldLog, err := os.Create(filepath.Join(work, "predecessor.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer oldLog.Close()
	old.Stdout, old.Stderr = oldLog, oldLog
	if err := old.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = old.Process.Kill(); _ = old.Wait() }()
	if pid := waitForPackagedMacWorkspace(ctx, entry, db, window); pid != strconv.Itoa(old.Process.Pid) {
		log, _ := os.ReadFile(oldLog.Name())
		t.Fatalf("predecessor did not open the isolated workspace: %s", log)
	}
	if err := old.Process.Signal(unix.SIGTERM); err != nil {
		t.Fatal(err)
	}
	_ = old.Wait()
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	workerPath := filepath.Join(work, "updater.test")
	stagedTarget := filepath.Join(work, filepath.Base(target))
	if _, err := macTool(ctx, "/usr/bin/install", "-m", "700", testBinary, workerPath); err != nil {
		t.Fatal(err)
	}
	if _, err := macTool(ctx, "/usr/bin/install", "-m", "600", target, stagedTarget); err != nil {
		t.Fatal(err)
	}
	worker := exec.CommandContext(ctx, "/usr/bin/sandbox-exec", append(sandbox, workerPath, "-test.run=^TestMacPackagedUpdateWorker$", "-test.v")...)
	worker.Env = append(env, "GITWAVE_TEST_MAC_WORKER_BUNDLE="+bundle, "GITWAVE_TEST_MAC_WORKER_ZIP="+stagedTarget,
		"GITWAVE_TEST_MAC_WORKER_VERSION="+targetVersion, "GITWAVE_TEST_MAC_WORKER_TEAM="+team)
	if log, err := worker.CombinedOutput(); err != nil {
		t.Fatalf("install and relaunch the packaged update: %v\n%s", err, log)
	}
	assertMacFixtureVersion(t, bundle, targetVersion)
	newHash, err := macCodeHash(ctx, bundle)
	if err != nil || oldHash == newHash {
		t.Fatalf("the replacement has no distinct signed executable: %v", err)
	}
	pid, err := strconv.Atoi(waitForPackagedMacWorkspace(ctx, entry, db, window))
	if err != nil {
		t.Fatal("the replacement did not show its window with the original workspace database open")
	}
	if hash, err := macProcessCodeHash(pid); err != nil || hash != newHash {
		t.Fatalf("the process opening the workspace does not match the replacement: %v", err)
	}
	if staging, err := filepath.Glob(filepath.Join(extracted, ".gitwave-update-*")); err != nil || len(staging) != 0 {
		t.Fatalf("completed update left staging directories: %v, %v", staging, err)
	}
	read, err := workspace.Open(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	selection, err := read.Load(ctx)
	if err != nil || selection.Workspace.ID != fixture.ID || selection.Workspace.Name != fixture.Name ||
		selection.Repo.ID != fixture.LastRepoID || selection.Repo.Path != repo {
		t.Fatalf("the updated application did not retain the workspace: %+v, %v", selection, err)
	}
	if enabled, err := read.AutoUpdate(ctx); err != nil || enabled {
		t.Fatalf("the updated application did not retain the auto-update preference: %t, %v", enabled, err)
	}
}

func verifyPackagedMacPredecessor(t *testing.T, feed, archive, version string) {
	t.Helper()
	input, err := os.Open(feed)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	data, err := io.ReadAll(io.LimitReader(input, MaxManifestSize+1))
	if err != nil {
		t.Fatal(err)
	}
	public, err := base64.StdEncoding.DecodeString(os.Getenv("GITWAVE_UPDATE_PUBLIC_KEY"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := VerifyManifest(data, public)
	if err != nil || manifest.Version != version {
		t.Fatalf("the predecessor feed does not authenticate v%s: %v", version, err)
	}
	pkg, err := DescribePackage(archive)
	if err != nil || pkg != manifest.Packages[MacOSArm64] {
		t.Fatalf("the predecessor archive does not match its published feed: %v", err)
	}
}

func packagedMacRequirement(team string) string {
	if team == "" {
		return `identifier "` + DesktopAppID + `"`
	}
	return macApplicationRequirement(team)
}

func packagedMacPIDs(ctx context.Context, entry string) []string {
	output, _ := exec.CommandContext(ctx, "/usr/sbin/lsof", "-a", "-d", "txt", "-t", "--", entry).Output()
	return strings.Fields(string(output))
}

func stopPackagedMacApp(entry string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, value := range packagedMacPIDs(ctx, entry) {
		if pid, err := strconv.Atoi(value); err == nil {
			_ = unix.Kill(pid, unix.SIGKILL)
		}
	}
}

func waitForPackagedMacWorkspace(parent context.Context, entry, db, window string) string {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		for _, pid := range packagedMacPIDs(ctx, entry) {
			if exec.CommandContext(ctx, window, pid).Run() != nil {
				continue
			}
			open, err := exec.CommandContext(ctx, "/usr/sbin/lsof", "-a", "-p", pid, "-t", "--", db).Output()
			if err == nil && strings.TrimSpace(string(open)) == pid {
				return pid
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return ""
}

func TestMacPackagedUpdateWorker(t *testing.T) {
	bundle := os.Getenv("GITWAVE_TEST_MAC_WORKER_BUNDLE")
	if bundle == "" {
		t.Skip("run only as the updater in the isolated packaged app test")
	}
	archive := os.Getenv("GITWAVE_TEST_MAC_WORKER_ZIP")
	version := os.Getenv("GITWAVE_TEST_MAC_WORKER_VERSION")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	current, err := macBundleProperty(ctx, bundle, "CFBundleShortVersionString")
	if err != nil {
		t.Fatal(err)
	}
	installed, err := bindMacBundle(ctx, bundle, DesktopAppID, packagedMacRequirement(os.Getenv("GITWAVE_TEST_MAC_WORKER_TEAM")))
	if err != nil {
		t.Fatal(err)
	}
	client := packagedMacUpdateClient(t, archive, version)
	candidate, err := client.Check(ctx, current, MacOSArm64)
	if err != nil || candidate == nil {
		t.Fatalf("no packaged update from %s to %s: %v", current, version, err)
	}
	restart, err := installed.install(ctx, candidate, nil)
	if err != nil {
		t.Fatal(err)
	}
	// The replacement is a regular desktop binary and only accepts desktop flags.
	os.Args = os.Args[:1]
	finish, err := restart(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := finish(true); err != nil {
		t.Fatal(err)
	}
}

type packagedMacTransport func(*http.Request) (*http.Response, error)

func (transport packagedMacTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return transport(req)
}

func packagedMacUpdateClient(t *testing.T, archive, version string) *Client {
	t.Helper()
	name, err := ArtifactName(version, MacOSArm64)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := DescribePackage(archive)
	if err != nil {
		t.Fatal(err)
	}
	windows := pkg
	windows.ExecutableSHA256 = pkg.SHA256
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := Sign(Manifest{Schema: 1, Version: version,
		Packages: map[string]Package{MacOSArm64: pkg, WindowsX64: windows, LinuxX64: pkg}}, private)
	if err != nil {
		t.Fatal(err)
	}
	transport := packagedMacTransport(func(req *http.Request) (*http.Response, error) {
		var body io.ReadCloser
		var size int64
		switch req.URL.Path {
		case "/releases/latest/download/" + ManifestName:
			body, size = io.NopCloser(bytes.NewReader(sealed)), int64(len(sealed))
		case "/releases/download/v" + version + "/" + name:
			file, err := os.Open(archive)
			if err != nil {
				return nil, err
			}
			body, size = file, pkg.Size
		default:
			return nil, fmt.Errorf("unexpected fixture download %s", req.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: body, ContentLength: size}, nil
	})
	return newClient("https://update-fixture.invalid", public, transport)
}
