package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aead.dev/minisign"

	"gitwave.work/internal/appupdate"
	"gitwave.work/internal/legacyupdate"
)

func TestSignAssetsChecksAllDigestsBeforeSigning(t *testing.T) {
	public, private, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	const version = "0.12.0"
	directory := t.TempDir()
	output := filepath.Join(t.TempDir(), "signatures")
	names := []string{"gitwave-0.12.0-windows-x64-setup.exe", "gitwave-0.12.0-linux-x64.AppImage", "gitwave-0.12.0-linux-x64.deb", "gitwave-0.12.0-linux-x64.rpm"}
	for _, name := range names {
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, []byte("fixture "+name), 0o644); err != nil {
			t.Fatal(err)
		}
		pkg, err := appupdate.DescribePackage(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path+".sha256", fmt.Appendf(nil, "%s  %s\n", pkg.SHA256, name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rpm := filepath.Join(directory, names[3])
	if err := os.WriteFile(rpm, []byte("tampered package"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := signAssets(version, directory, output, private); err == nil || !strings.Contains(err.Error(), "release checksum") {
		t.Fatalf("signing a modified package returned %v", err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("signatures were written before all package digests were verified: %v", err)
	}
	if err := os.WriteFile(rpm, []byte("fixture "+names[3]), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := signAssets(version, directory, output, private); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		signature, err := os.ReadFile(filepath.Join(output, name+".sig"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, name+".sig"), signature, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := legacyupdate.VerifyArtifact(filepath.Join(directory, name), version, "", public); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if err := signAssets(version, directory, output, private); err == nil {
		t.Fatal("overwrote an existing signature")
	}
}
