package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRPMFileListKeepsThePreviewInstallSeparate(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"opt/gitwave-preview", "usr/bin"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("/opt/gitwave-preview/GitWave", filepath.Join(root, "usr/bin/gitwave-preview")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	got, err := rpmFileList(root, "gitwave-preview")
	if err != nil || got != "/opt/gitwave-preview\n/usr/bin/gitwave-preview\n" {
		t.Fatalf("preview file list = %q, %v", got, err)
	}
	if _, err := rpmFileList(root, "gitwave"); err == nil {
		t.Fatal("a preview payload was accepted as a stable package")
	}
	bad := filepath.Join(root, "usr/bin/invalid%name")
	if err := os.WriteFile(bad, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := rpmFileList(root, "gitwave-preview"); err == nil || !strings.Contains(err.Error(), "invalid%name") {
		t.Fatalf("unsafe spec filename returned %v", err)
	}
}
