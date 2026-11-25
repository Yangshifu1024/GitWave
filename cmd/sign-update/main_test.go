package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gitwave.work/internal/appupdate"
)

func TestSigningKeyRequiresConfiguration(t *testing.T) {
	t.Setenv("GITWAVE_UPDATE_PUBLIC_KEY", "")
	t.Setenv("GITWAVE_UPDATE_SIGNING_SEED", "")
	if key, err := signingKey(); key != nil || err == nil || err.Error() != "missing GITWAVE_UPDATE_PUBLIC_KEY" {
		t.Fatal("missing public key was not reported")
	}
	t.Setenv("GITWAVE_UPDATE_PUBLIC_KEY", "configured")
	if key, err := signingKey(); key != nil || err == nil || err.Error() != "missing GITWAVE_UPDATE_SIGNING_SEED" {
		t.Fatal("missing signing seed was not reported")
	}
	t.Setenv("GITWAVE_UPDATE_SIGNING_SEED", "not Base64")
	if key, err := signingKey(); key != nil || err == nil {
		t.Fatal("invalid signing seed was accepted")
	}
}

func TestSignUpdateRequiresMatchingKeyAndEveryArtifact(t *testing.T) {
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITWAVE_UPDATE_SIGNING_SEED", base64.StdEncoding.EncodeToString(private.Seed()))
	t.Setenv("GITWAVE_UPDATE_PUBLIC_KEY", "wrong public key")
	dir := t.TempDir()
	windowsApp := filepath.Join(dir, "GitWave.exe")
	if err := sign("0.12.0", dir, windowsApp, ""); err == nil {
		t.Fatal("signed a manifest using a key that differs from the build's verification key")
	}
	t.Setenv("GITWAVE_UPDATE_PUBLIC_KEY", base64.StdEncoding.EncodeToString(public))
	if err := sign("0.12.0", dir, windowsApp, ""); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsigned missing release files produced %v", err)
	}
	for _, target := range []string{appupdate.MacOSArm64, appupdate.WindowsX64, appupdate.LinuxX64} {
		name, err := appupdate.ArtifactName("0.12.0", target)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(target+" release contents"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := sign("0.12.0", dir, windowsApp, ""); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing Windows executable produced %v", err)
	}
	if err := os.WriteFile(windowsApp, []byte("Windows application"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := sign("0.12.0", dir, windowsApp, ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, appupdate.ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	var signed struct{ Manifest, Signature []byte }
	var manifest appupdate.Manifest
	if json.Unmarshal(data, &signed) != nil || !ed25519.Verify(public, signed.Manifest, signed.Signature) ||
		json.Unmarshal(signed.Manifest, &manifest) != nil || manifest.Version != "0.12.0" || len(manifest.Packages) != 3 {
		t.Fatal("the CLI did not publish an authenticated three-platform manifest")
	}
	executable, err := appupdate.DescribePackage(windowsApp)
	if err != nil || manifest.Packages[appupdate.WindowsX64].ExecutableSHA256 != executable.SHA256 {
		t.Fatalf("the Windows executable digest was not included: %v", err)
	}

	published := filepath.Join(dir, appupdate.ManifestName)
	t.Setenv("GITWAVE_UPDATE_SIGNING_SEED", "")
	if err := verifyPublishedManifest(published, "0.12.0"); err != nil {
		t.Fatalf("the published feed should verify with the public key alone: %v", err)
	}
	if err := verifyPublishedManifest(published, "0.12.1"); err == nil {
		t.Fatal("accepted a valid manifest for a different release tag")
	}
	rotatedPublic, rotatedPrivate, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITWAVE_UPDATE_PUBLIC_KEY", base64.StdEncoding.EncodeToString(rotatedPublic))
	t.Setenv("GITWAVE_UPDATE_SIGNING_SEED", base64.StdEncoding.EncodeToString(rotatedPrivate.Seed()))
	key, err := signingKey()
	clear(key)
	if err != nil {
		t.Fatalf("the new key pair should pass the pairing check: %v", err)
	}
	if err := verifyPublishedManifest(published, "0.12.0"); !errors.Is(err, appupdate.ErrSignature) {
		t.Fatalf("replacing both keys should fail the published feed check: %v", err)
	}
}
