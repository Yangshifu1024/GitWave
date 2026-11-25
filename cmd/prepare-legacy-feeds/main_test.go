package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aead.dev/minisign"
)

func TestPrepareFeedsRoutesAllPlatformsThroughBothReleases(t *testing.T) {
	key, private, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	oldNames := []string{"GitWave_0.11.1_aarch64.app.tar.gz", "GitWave_0.11.1_x64-setup.exe", "GitWave_0.11.1_amd64.AppImage", "GitWave_0.11.1_amd64.deb", "GitWave-0.11.1-1.x86_64.rpm"}
	newNames := []string{"GitWave_0.12.0_aarch64.app.tar.gz", "gitwave-0.12.0-windows-x64-setup.exe", "gitwave-0.12.0-linux-x64.AppImage", "gitwave-0.12.0-linux-x64.deb", "gitwave-0.12.0-linux-x64.rpm"}
	exporter := release{version: "0.11.1", assets: t.TempDir()}
	native := release{version: "0.12.0", assets: t.TempDir()}
	for _, name := range oldNames {
		fixtureArtifact(t, exporter.assets, name, exporter.version, private)
	}
	for _, name := range newNames {
		fixtureArtifact(t, native.assets, name, native.version, private)
	}
	parent := t.TempDir()
	out := filepath.Join(parent, "ready")
	if err := prepareFeeds(exporter, native, out, key); err != nil {
		t.Fatal(err)
	}
	if err := prepareFeeds(exporter, native, out, key); err == nil {
		t.Fatal("replaced an existing feed directory")
	}
	exporterOut := filepath.Join(parent, "exporter")
	if err := prepareExporterFeed(exporter, exporterOut, key); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(exporterOut)
	if err != nil || len(entries) != 1 || entries[0].Name() != "latest.json" {
		t.Fatalf("unexpected exporter feed output: %v, %v", entries, err)
	}
	standalone, err := os.ReadFile(filepath.Join(exporterOut, "latest.json"))
	if err != nil {
		t.Fatal(err)
	}
	combined, err := os.ReadFile(filepath.Join(out, "latest.json"))
	if err != nil || !bytes.Equal(standalone, combined) {
		t.Fatalf("exporter draft emitted a different feed from the Go release assembly: %v", err)
	}
	for _, stage := range []struct {
		release  release
		manifest string
		names    []string
	}{
		{exporter, "latest.json", oldNames},
		{native, "gitwave-legacy-transition.json", newNames},
	} {
		data, err := os.ReadFile(filepath.Join(out, stage.manifest))
		if err != nil {
			t.Fatal(err)
		}
		var manifest feed
		if err := json.Unmarshal(data, &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest.Version != stage.release.version || len(manifest.Platforms) != 8 {
			t.Fatalf("unexpected %s version or target count: %+v", stage.manifest, manifest)
		}
		base := repository + "/releases/download/v" + stage.release.version + "/"
		for target, index := range map[string]int{
			"darwin-aarch64": 0, "darwin-aarch64-app": 0,
			"windows-x86_64": 1, "windows-x86_64-nsis": 1,
			"linux-x86_64": 2, "linux-x86_64-appimage": 2,
			"linux-x86_64-deb": 3, "linux-x86_64-rpm": 4,
		} {
			if entry := manifest.Platforms[target]; entry.URL != base+stage.names[index] || entry.Signature == "" {
				t.Fatalf("%s target %s = %+v", stage.manifest, target, entry)
			}
		}
	}

	bad := filepath.Join(parent, "mismatched")
	fixtureArtifact(t, native.assets, newNames[1], exporter.version, private)
	if err := prepareFeeds(exporter, native, bad, key); err == nil || !strings.Contains(err.Error(), "different or ambiguous version") {
		t.Fatalf("wrong signed version produced %v", err)
	}
	if _, err := os.Stat(bad); !os.IsNotExist(err) {
		t.Fatalf("a mismatched version left output behind: %v", err)
	}
	fixtureArtifact(t, native.assets, newNames[1], native.version, private)
	if err := os.WriteFile(filepath.Join(native.assets, newNames[1]), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := prepareFeeds(exporter, native, bad, key); err == nil || !strings.Contains(err.Error(), "Minisign verification") {
		t.Fatalf("tampered artifact produced %v", err)
	}
	for _, suffix := range []string{"", ".sig"} {
		content, err := os.ReadFile(filepath.Join(native.assets, newNames[2]+suffix))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(native.assets, newNames[1]+suffix), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := prepareFeeds(exporter, native, bad, key); err == nil || !strings.Contains(err.Error(), "artifact name") {
		t.Fatalf("misrouted artifact with a valid signature produced %v", err)
	}
	fixtureArtifact(t, native.assets, newNames[1], native.version, private)
	path := filepath.Join(native.assets, newNames[1]+".sig")
	encoded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	var parsed minisign.Signature
	if err := parsed.UnmarshalText(signature); err != nil {
		t.Fatal(err)
	}
	parsed.Algorithm = minisign.EdDSA
	signature, err = parsed.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, base64.StdEncoding.AppendEncode(nil, signature), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := prepareFeeds(exporter, native, bad, key); err == nil || !strings.Contains(err.Error(), "prehashed") {
		t.Fatalf("changed signature algorithm produced %v", err)
	}
	if err := os.Remove(filepath.Join(exporter.assets, oldNames[4]+".sig")); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(parent, "partial-exporter")
	if err := prepareExporterFeed(exporter, partial, key); err == nil || !strings.Contains(err.Error(), oldNames[4]) {
		t.Fatalf("an unsigned exporter RPM produced %v", err)
	}
	if _, err := os.Stat(partial); !os.IsNotExist(err) {
		t.Fatalf("an incomplete exporter draft left a feed directory: %v", err)
	}
}

func fixtureArtifact(t *testing.T, dir, name, version string, key minisign.PrivateKey) {
	t.Helper()
	data := []byte("fixture for " + name)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	trusted := fmt.Sprintf("timestamp:1\tfile:%s\tversion:%s", name, version)
	reader := minisign.NewReader(strings.NewReader(string(data)))
	if _, err := io.Copy(io.Discard, reader); err != nil {
		t.Fatal(err)
	}
	signature := reader.SignWithComments(key, trusted, "fixture signature")
	if err := os.WriteFile(path+".sig", base64.StdEncoding.AppendEncode(nil, signature), 0o644); err != nil {
		t.Fatal(err)
	}
}
