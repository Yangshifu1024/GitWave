package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aead.dev/minisign"

	"gitwave.work/internal/appupdate"
	"gitwave.work/internal/distasset"
	"gitwave.work/internal/legacyupdate"
)

func main() {
	version := flag.String("version", "", "first Go release version (X.Y.Z)")
	assets := flag.String("assets", "", "directory of verified Windows and Linux release assets")
	output := flag.String("out", "", "new directory for the complete signature set")
	flag.Parse()
	encoded := os.Getenv("TAURI_SIGNING_PRIVATE_KEY")
	password := os.Getenv("TAURI_SIGNING_PRIVATE_KEY_PASSWORD")
	_ = os.Unsetenv("TAURI_SIGNING_PRIVATE_KEY")
	_ = os.Unsetenv("TAURI_SIGNING_PRIVATE_KEY_PASSWORD")
	key, err := legacyupdate.LoadSigningKey(encoded, password, legacyupdate.PublicKeyBase64)
	if err == nil {
		err = signAssets(*version, *assets, *output, key)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func signAssets(version, assets, output string, key minisign.PrivateKey) error {
	setup, err := appupdate.ArtifactName(version, appupdate.WindowsX64)
	if err != nil || strings.Contains(version, "-") || assets == "" || output == "" {
		return errors.New("-assets, -out and a numeric X.Y.Z -version are required")
	}
	image, _ := appupdate.ArtifactName(version, appupdate.LinuxX64)
	linux := strings.TrimSuffix(image, ".AppImage")
	names := []string{setup, image, linux + ".deb", linux + ".rpm"}
	for _, name := range names {
		path := filepath.Join(assets, name)
		pkg, err := appupdate.DescribePackage(path)
		if err != nil {
			return err
		}
		digest, err := os.ReadFile(path + ".sha256")
		if err != nil {
			return err
		}
		if string(digest) != fmt.Sprintf("%s  %s\n", pkg.SHA256, name) {
			return fmt.Errorf("release checksum does not match %s", name)
		}
	}
	stage, err := os.MkdirTemp(filepath.Dir(output), ".gitwave-legacy-signatures-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for _, name := range names {
		path := filepath.Join(assets, name)
		signature, err := legacyupdate.SignArtifact(path, version, key)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(stage, name+".sig"), signature, 0o644); err != nil {
			return err
		}
	}
	return distasset.PublishDirectory(stage, output)
}
