package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseVersionPreservesOtherConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nsgo.yaml")
	original := "name: GitWave\r\nversion: '0.11.0' # desktop version\r\nbuild:\r\n  output: dist\r\n"
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	if got, err := releaseVersion(path, nil); err != nil || got != "0.11.0" {
		t.Fatalf("read version = %q, %v", got, err)
	}
	if got, err := releaseVersion(path, []string{"v0.12.0"}); err != nil || got != "0.12.0" {
		t.Fatalf("updated version = %q, %v", got, err)
	}
	if _, err := releaseVersion(path, []string{"0.13.0-rc.1"}); err == nil {
		t.Fatal("accepted an installer version with a prerelease suffix")
	}
	data, err := os.ReadFile(path)
	if want := strings.Replace(original, "'0.11.0'", "'0.12.0'", 1); err != nil || string(data) != want {
		t.Fatalf("configuration = %q, %v; want %q", data, err, want)
	}
}
