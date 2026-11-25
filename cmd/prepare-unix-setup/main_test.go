package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"slices"
	"strings"
	"testing"

	"gitwave.work/internal/appupdate"
)

func TestUnixReleaseFlags(t *testing.T) {
	public := base64.StdEncoding.EncodeToString(make([]byte, ed25519.PublicKeySize))
	t.Setenv("GITWAVE_UPDATE_PUBLIC_KEY", public)
	t.Setenv("NSGO_NOTARY_KEY", "")
	const identity = "Developer ID Application: test fixture"
	want := []string{"-id", appupdate.DesktopAppID, "-version", "0.12.0", "-ldflags", "-X main.releaseUpdateKey=" + public}
	for _, test := range []struct{ target, identity, profile string }{
		{"macos_arm64", identity, "test-profile"},
		{"linux_x64", "", ""},
	} {
		got, err := unixReleaseFlags(test.target, "0.12.0", test.identity, test.profile)
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("%s release flags = %v, %v; want %v", test.target, got, err, want)
		}
	}
	for _, test := range []struct{ target, version, identity, profile, message string }{
		{"linux_x64", "0.12.0-rc.1", "", "", "three-part numeric version"},
		{"macos_arm64", "v0.12.0", identity, "test-profile", "three-part numeric version"},
		{"macos_arm64", "0.12.0-rc.1", identity, "test-profile", "three-part numeric version"},
		{"macos_arm64", "0.12.0", "-", "test-profile", "Developer ID"},
		{"macos_arm64", "0.12.0", identity, "", "notarytool credentials"},
	} {
		_, err := unixReleaseFlags(test.target, test.version, test.identity, test.profile)
		if err == nil || !strings.Contains(err.Error(), test.message) {
			t.Fatalf("want %q, got %v", test.message, err)
		}
	}
	t.Setenv("NSGO_NOTARY_KEY", "test.p8")
	t.Setenv("NSGO_NOTARY_KEY_ID", "")
	t.Setenv("NSGO_NOTARY_ISSUER", "")
	if _, err := unixReleaseFlags("macos_arm64", "0.12.0", identity, "test-profile"); err == nil {
		t.Fatal("an incomplete API key would override a valid keychain profile")
	}
	t.Setenv("NSGO_NOTARY_KEY_ID", "test-key")
	t.Setenv("NSGO_NOTARY_ISSUER", "test-issuer")
	if _, err := unixReleaseFlags("macos_arm64", "0.12.0", identity, ""); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GITWAVE_UPDATE_PUBLIC_KEY", public+"\n")
	if _, err := unixReleaseFlags("macos_arm64", "0.12.0", identity, ""); err == nil {
		t.Fatal("a key with a newline could alter the Go linker flags")
	}
}
