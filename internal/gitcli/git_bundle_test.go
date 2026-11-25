package gitcli

import (
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gitwave.work/internal/gittool"
)

func TestInstalledGitBundle(t *testing.T) {
	if os.Getenv("GITWAVE_TEST_INSTALLED_GIT") != "1" {
		t.Skip("requires the installed Git bundle")
	}
	if _, err := exec.LookPath("git"); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("test PATH must not provide Git, got %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	repo, err := Init(t.Context(), filepath.Join(t.TempDir(), "new repository"))
	if err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Dir(executable)
	if runtime.GOOS == "darwin" {
		directory = filepath.Join(filepath.Dir(directory), "Resources")
	}
	root := filepath.Join(directory, gittool.Directory)
	want := filepath.Join(root, "bin", "git")
	checks := [][]string{{"--version"}, {"lfs", "version"}}
	if runtime.GOOS == "windows" {
		want = filepath.Join(root, "cmd", "git.exe")
		checks = append(checks, []string{"credential-manager", "--version"})
	}
	if repo.git != want && (runtime.GOOS != "windows" || !strings.EqualFold(repo.git, want)) {
		t.Fatalf("application used %s instead of %s", repo.git, want)
	}
	for _, args := range checks {
		if output, err := repo.readLimited(t.Context(), "", 16<<10, args...); err != nil || len(output) == 0 {
			t.Fatalf("Git %v: %s, %v", args, output, err)
		}
	}
	if _, err := repo.InstallLFS(t.Context()); err != nil {
		t.Fatal(err)
	}
	if status, err := repo.LFSStatus(t.Context()); err != nil || !status.Installed {
		t.Fatalf("bundled Git LFS status: %+v, %v", status, err)
	}
	if runtime.GOOS != "windows" {
		if execPath, err := repo.readLimited(t.Context(), "", 16<<10, "--exec-path"); err != nil ||
			strings.TrimSpace(string(execPath)) != filepath.Join(root, "libexec", "git-core") {
			t.Fatalf("Git helper directory = %q, %v", execPath, err)
		}
		probeGitBundleHTTPS(t, repo)
		return
	}
	ssh, err := gittool.WindowsSSH("ssh")
	if err != nil {
		t.Fatal(err)
	}
	if version, err := exec.CommandContext(t.Context(), ssh, "-V").CombinedOutput(); err != nil || !strings.Contains(string(version), "OpenSSH") {
		t.Fatalf("bundled SSH: %s, %v", version, err)
	}
}

func probeGitBundleHTTPS(t *testing.T, repo *Repository) {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repo/info/refs" || r.URL.Query().Get("service") != "git-upload-pack" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
		_, _ = w.Write([]byte("001e# service=git-upload-pack\n00000000"))
	}))
	t.Cleanup(server.Close)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(ca, certificate, 0o600); err != nil {
		t.Fatal(err)
	}
	checkHTTPS := func(config ...string) {
		options := append([]string{"-c", "protocol.version=0", "-c", "http.proxy="}, config...)
		cmd := gitCommand(t.Context(), repo.git, repo.Path(), options, "", "ls-remote", server.URL+"/repo")
		cmd.Env = append(cmd.Env, "GIT_ALLOW_PROTOCOL=https", "NO_PROXY=", "no_proxy=")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("bundled Git HTTPS helper: %s, %v", output, err)
		}
	}
	checkHTTPS("-c", "http.sslCAInfo="+ca)
	if systemCA := os.Getenv("GITWAVE_TEST_GIT_SYSTEM_CA"); systemCA != "" {
		// The Linux package fixture runs in a disposable container.
		file, err := os.OpenFile(systemCA, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(systemCA) })
		_, writeErr := file.Write(certificate)
		if err := errors.Join(writeErr, file.Close()); err != nil {
			t.Fatal(err)
		}
		checkHTTPS()
		proxy := gitBundleTLSProxy(strings.TrimPrefix(server.URL, "https://"))
		t.Cleanup(proxy.Close)
		checkHTTPS("-c", "http.proxy="+proxy.URL)
	}
}

func gitBundleTLSProxy(target string) *httptest.Server {
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != target {
			http.Error(w, "Unexpected proxy target", http.StatusBadRequest)
			return
		}
		upstream, err := net.DialTimeout("tcp", target, 2*time.Second)
		if err != nil {
			http.Error(w, "Unable to connect to the fixture", http.StatusBadGateway)
			return
		}
		defer upstream.Close()
		client, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer client.Close()
		if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			return
		}
		go func() {
			_, _ = io.Copy(upstream, buffered)
			_ = upstream.(*net.TCPConn).CloseWrite()
		}()
		_, _ = io.Copy(client, upstream)
	}))
}
