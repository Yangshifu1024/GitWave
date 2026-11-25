package appupdate

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"gitwave.work/internal/proxy"
)

func TestSignedUpdateChecksWithoutDownloadingAndRejectsCorruptPackages(t *testing.T) {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	const payload = "isolated update package"
	digest := sha256.Sum256([]byte(payload))
	packageInfo := Package{Size: int64(len(payload)), SHA256: hex.EncodeToString(digest[:])}
	windowsInfo := packageInfo
	windowsInfo.ExecutableSHA256 = packageInfo.SHA256
	manifest := Manifest{Schema: 1, Version: "0.12.0", Notes: "Release notes\nSecond line",
		Packages: map[string]Package{MacOSArm64: packageInfo, WindowsX64: windowsInfo, LinuxX64: packageInfo}}
	sealed, err := Sign(manifest, private)
	if err != nil {
		t.Fatal(err)
	}
	var corrupt, badSignature atomic.Bool
	var downloads atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/project/releases/latest/download/"+ManifestName, func(w http.ResponseWriter, _ *http.Request) {
		if badSignature.Load() {
			var changed envelope
			_ = json.Unmarshal(sealed, &changed)
			changed.Manifest = bytes.Replace(changed.Manifest, []byte("0.12.0"), []byte("0.13.0"), 1)
			_ = json.NewEncoder(w).Encode(changed)
			return
		}
		_, _ = w.Write(sealed)
	})
	mux.HandleFunc("/project/releases/download/v0.12.0/gitwave-0.12.0-linux-x64.AppImage", func(w http.ResponseWriter, _ *http.Request) {
		downloads.Add(1)
		if corrupt.Load() {
			_, _ = io.WriteString(w, "X"+payload[1:])
			return
		}
		_, _ = io.WriteString(w, payload)
	})
	server := httptest.NewTLSServer(mux)
	defer server.Close()
	client := newClient(server.URL+"/project", public, server.Client().Transport)
	for _, current := range []string{"0.12.0", "0.13.0", "1.0.0"} {
		if offer, err := client.Check(t.Context(), current, LinuxX64); err != nil || offer != nil {
			t.Fatalf("release %s got an offer %v, %v", current, offer, err)
		}
	}
	update, err := client.Check(t.Context(), "0.12.0-rc.1", LinuxX64)
	if err != nil || update == nil || update.Version() != manifest.Version || update.Notes() != manifest.Notes ||
		update.ReleaseNotesURL() != server.URL+"/project/releases/tag/v0.12.0" || downloads.Load() != 0 {
		t.Fatalf("check result = %v, %v, downloads = %d", update, err, downloads.Load())
	}
	var received int64
	root := t.TempDir()
	path, err := update.Download(t.Context(), root, func(soFar, total int64) {
		if total != int64(len(payload)) || soFar < received || soFar > total {
			t.Fatalf("invalid update progress: %d / %d", soFar, total)
		}
		received = soFar
	})
	if err != nil || filepath.Base(path) != "gitwave-0.12.0-linux-x64.AppImage" || received != int64(len(payload)) {
		t.Fatalf("download = %s, %v, received = %d", path, err, received)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != payload {
		t.Fatalf("downloaded contents = %q, %v", data, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if path, err := update.Download(ctx, root, func(soFar, total int64) {
		if soFar == total {
			cancel()
		}
	}); !errors.Is(err, context.Canceled) || path != "" {
		t.Fatalf("canceled download got path %q, err %v", path, err)
	}
	corrupt.Store(true)
	if path, err := update.Download(t.Context(), root, nil); !errors.Is(err, ErrPackage) || path != "" {
		t.Fatalf("corrupt download got path %q, err %v", path, err)
	}
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 1 {
		t.Fatalf("corrupt download leaked files: %v, %v", entries, err)
	}
	badSignature.Store(true)
	if offer, err := client.Check(t.Context(), "0.11.0", LinuxX64); !errors.Is(err, ErrSignature) || offer != nil {
		t.Fatalf("tampered metadata got offer %v, err %v", offer, err)
	}
}

func TestUpdateRequestsUseTheConfiguredProxy(t *testing.T) {
	var hits atomic.Int32
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect && r.Host == "github.com:443" {
			hits.Add(1)
		}
		http.Error(w, "fixture only", http.StatusBadGateway)
	}))
	defer proxyServer.Close()
	public, _, _ := ed25519.GenerateKey(nil)
	client, err := New(base64.StdEncoding.EncodeToString(public))
	if err != nil {
		t.Fatal(err)
	}
	ctx := proxy.WithSettings(t.Context(), proxy.Settings{Mode: proxy.Manual, ManualURL: proxyServer.URL})
	if _, err := client.Check(ctx, "0.11.0", MacOSArm64); !errors.Is(err, ErrNetwork) || hits.Load() != 1 {
		t.Fatalf("check missed configured proxy: %d, %v", hits.Load(), err)
	}
}

func TestUpdateRejectsRedirectToUntrustedOrigin(t *testing.T) {
	var outside atomic.Int32
	destination := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { outside.Add(1) }))
	defer destination.Close()
	for _, address := range []string{destination.URL, "http://127.0.0.1/unsigned"} {
		t.Run(address, func(t *testing.T) {
			source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, address, http.StatusFound)
			}))
			defer source.Close()
			key, _, _ := ed25519.GenerateKey(nil)
			client := newClient(source.URL, key, source.Client().Transport)
			if _, err := client.Check(t.Context(), "0.11.0", WindowsX64); !errors.Is(err, ErrNetwork) {
				t.Fatalf("untrusted redirect returned %v", err)
			}
		})
	}
	if outside.Load() != 0 {
		t.Fatal("the update client contacted an untrusted redirect")
	}
}

func signedUpdateFixture(t *testing.T, payload []byte, target string) *Candidate {
	t.Helper()
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	const version = "0.12.0"
	name, err := ArtifactName(version, target)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	pkg := Package{Size: int64(len(payload)), SHA256: hex.EncodeToString(digest[:])}
	windowsInfo := pkg
	windowsInfo.ExecutableSHA256 = pkg.SHA256
	manifest, err := Sign(Manifest{Schema: 1, Version: version,
		Packages: map[string]Package{MacOSArm64: pkg, WindowsX64: windowsInfo, LinuxX64: pkg}}, private)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest/download/"+ManifestName, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(manifest) })
	mux.HandleFunc("/releases/download/v"+version+"/"+name, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) })
	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)
	client := newClient(server.URL, public, server.Client().Transport)
	update, err := client.Check(t.Context(), "0.11.0", target)
	if err != nil || update == nil {
		t.Fatalf("no fixture update: %v", err)
	}
	return update
}
