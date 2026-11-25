//go:build !ios

package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aead.dev/minisign"

	"gitwave.work/internal/legacyupdate"
)

func TestLegacyMacArchive(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "GitWave.app")
	if err := os.Mkdir(bundle, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "git-remote-http"), []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("git-remote-http", filepath.Join(bundle, "git-remote-https")); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "GitWave_0.12.0_aarch64.app.tar.gz")
	if err := writeArchive(t.Context(), bundle, archive); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer compressed.Close()
	entries := tar.NewReader(compressed)
	want := []tar.Header{
		{Name: "GitWave.app/", Typeflag: tar.TypeDir, Mode: 0o755},
		{Name: "GitWave.app/git-remote-http", Typeflag: tar.TypeReg, Mode: 0o755},
		{Name: "GitWave.app/git-remote-https", Typeflag: tar.TypeSymlink, Linkname: "git-remote-http"},
	}
	for _, entry := range want {
		got, err := entries.Next()
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != entry.Name || got.Typeflag != entry.Typeflag || got.Linkname != entry.Linkname || (entry.Mode != 0 && got.Mode != entry.Mode) {
			t.Fatalf("archive entry = %+v, want %+v", got, entry)
		}
	}
	if _, err := entries.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("archive has an unexpected entry or error: %v", err)
	}

	public, private, err := minisign.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	text, err := private.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	encoded := base64.StdEncoding.EncodeToString(text)
	if _, err := legacyupdate.LoadSigningKey(encoded, "", legacyupdate.PublicKeyBase64); err == nil {
		t.Fatal("accepted a signing key different from the installed app's key")
	}
	text, err = public.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := legacyupdate.LoadSigningKey(encoded, "", base64.StdEncoding.EncodeToString(text))
	if err != nil {
		t.Fatal(err)
	}
	signed, err := legacyupdate.SignArtifact(archive, "0.12.0", loaded)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := base64.StdEncoding.DecodeString(string(signed))
	if err != nil || !minisign.Verify(public, data, signature) || !strings.Contains(string(signature), "\tfile:GitWave_0.12.0_aarch64.app.tar.gz\tversion:0.12.0\n") {
		t.Fatalf("invalid versioned Minisign signature: %v", err)
	}
	data[0] ^= 0xff
	if minisign.Verify(public, data, signature) {
		t.Fatal("the signature accepted a modified archive")
	}
}

func TestLegacyMacBridgeOutputStaysOutsideBundle(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "GitWave.app")
	parent := filepath.Join(bundle, "Contents", "Resources")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := checkOutputParent(bundle, root); err != nil {
		t.Fatal(err)
	}
	if err := checkOutputParent(bundle, parent); err == nil {
		t.Fatal("accepted an output inside the input bundle")
	}
	alias := filepath.Join(root, "gItWaVe.ApP", "Contents", "Resources")
	if _, err := os.Stat(alias); err == nil {
		if err := checkOutputParent(bundle, alias); err == nil {
			t.Fatal("accepted a differently cased path inside the same input bundle")
		}
	}
}
