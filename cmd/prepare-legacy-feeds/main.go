package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aead.dev/minisign"
	"golang.org/x/mod/semver"

	"gitwave.work/internal/appupdate"
	"gitwave.work/internal/distasset"
	"gitwave.work/internal/legacyupdate"
)

const repository = "https://github.com/Yangshifu1024/GitWave"
const tauriMacArchiveName = "GitWave.app.tar.gz"

type release struct {
	version, assets, notes string
}

type platform struct {
	URL       string `json:"url"`
	Signature string `json:"signature"`
}

type feed struct {
	Version   string              `json:"version"`
	Notes     string              `json:"notes,omitempty"`
	Platforms map[string]platform `json:"platforms"`
}

func main() {
	var exporter, native release
	flag.StringVar(&exporter.version, "export-version", "", "Tauri preference exporter version")
	flag.StringVar(&exporter.assets, "export-assets", "", "directory of signed Tauri preference exporter assets")
	flag.StringVar(&exporter.notes, "export-notes", "", "optional preference exporter release notes file")
	flag.StringVar(&native.version, "go-version", "", "first Go release version")
	flag.StringVar(&native.assets, "go-assets", "", "directory of signed Go transition assets")
	flag.StringVar(&native.notes, "go-notes", "", "optional first Go release notes file")
	output := flag.String("out", "", "new directory for verified legacy update feeds")
	verifyMac := flag.Bool("verify-export-macos", false, "verify the macOS exporter archive in -export-assets for -export-version")
	exportOnly := flag.Bool("export-only", false, "verify the exporter assets and generate its latest.json before publication")
	flag.Parse()
	keyBytes, err := base64.StdEncoding.DecodeString(legacyupdate.PublicKeyBase64)
	var key minisign.PublicKey
	if err == nil {
		err = key.UnmarshalText(keyBytes)
	}
	if err == nil {
		switch {
		case *verifyMac && *exportOnly:
			err = errors.New("-verify-export-macos and -export-only are mutually exclusive")
		case *verifyMac:
			err = verifyMacExporter(exporter, key)
		case *exportOnly:
			err = prepareExporterFeed(exporter, *output, key)
		default:
			err = prepareFeeds(exporter, native, *output, key)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func verifyMacExporter(exporter release, key minisign.PublicKey) error {
	if exporter.assets == "" {
		return errors.New("-export-assets is required")
	}
	if err := validateReleaseVersion(exporter.version); err != nil {
		return err
	}
	_, err := verifyAsset(exporter, macArchiveName(exporter.version), tauriMacArchiveName, key)
	return err
}

func validateReleaseVersion(version string) error {
	if _, err := appupdate.ArtifactName(version, appupdate.MacOSArm64); err != nil || strings.Contains(version, "-") {
		return errors.New("the transition requires numeric X.Y.Z release versions")
	}
	return nil
}

func macArchiveName(version string) string {
	return "GitWave_" + version + "_aarch64.app.tar.gz"
}

func verifyAsset(r release, name, alternateName string, key minisign.PublicKey) (string, error) {
	signature, err := legacyupdate.VerifyArtifact(filepath.Join(r.assets, name), r.version, alternateName, key)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return signature, nil
}

func prepareExporterFeed(exporter release, output string, key minisign.PublicKey) error {
	if exporter.assets == "" || output == "" {
		return errors.New("-export-assets and -out are required")
	}
	if err := validateReleaseVersion(exporter.version); err != nil {
		return err
	}
	if semver.Compare("v"+exporter.version, "v0.11.0") <= 0 {
		return errors.New("the preference exporter must be newer than v0.11.0")
	}
	data, err := buildFeed(exporter, false, key)
	if err != nil {
		return fmt.Errorf("verify the Tauri preference exporter feed: %w", err)
	}
	return publishFeeds(output, map[string][]byte{"latest.json": data})
}

func prepareFeeds(exporter, native release, output string, key minisign.PublicKey) error {
	if exporter.assets == "" || native.assets == "" || output == "" {
		return errors.New("-export-assets, -go-assets and -out are required")
	}
	for _, r := range []release{exporter, native} {
		if err := validateReleaseVersion(r.version); err != nil {
			return err
		}
	}
	if semver.Compare("v"+exporter.version, "v0.11.0") <= 0 || semver.Compare("v"+native.version, "v"+exporter.version) <= 0 {
		return errors.New("release versions must increase from v0.11.0 to the preference exporter to the first Go release")
	}
	oldFeed, err := buildFeed(exporter, false, key)
	if err != nil {
		return fmt.Errorf("verify the Tauri preference exporter feed: %w", err)
	}
	goFeed, err := buildFeed(native, true, key)
	if err != nil {
		return fmt.Errorf("verify the first Go release feed: %w", err)
	}
	return publishFeeds(output, map[string][]byte{"latest.json": oldFeed, "gitwave-legacy-transition.json": goFeed})
}

func publishFeeds(output string, manifests map[string][]byte) error {
	stage, err := os.MkdirTemp(filepath.Dir(output), ".gitwave-legacy-feeds-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for name, data := range manifests {
		if err := os.WriteFile(filepath.Join(stage, name), data, 0o644); err != nil {
			return err
		}
	}
	return distasset.PublishDirectory(stage, output)
}

func buildFeed(r release, native bool, key minisign.PublicKey) ([]byte, error) {
	mac := macArchiveName(r.version)
	windows := "GitWave_" + r.version + "_x64-setup.exe"
	appImage := "GitWave_" + r.version + "_amd64.AppImage"
	deb := "GitWave_" + r.version + "_amd64.deb"
	rpm := "GitWave-" + r.version + "-1.x86_64.rpm"
	if native {
		windows, _ = appupdate.ArtifactName(r.version, appupdate.WindowsX64)
		appImage, _ = appupdate.ArtifactName(r.version, appupdate.LinuxX64)
		linux := strings.TrimSuffix(appImage, ".AppImage")
		deb, rpm = linux+".deb", linux+".rpm"
	}
	manifest := feed{Version: r.version, Platforms: make(map[string]platform)}
	if r.notes != "" {
		notes, err := os.ReadFile(r.notes)
		if err != nil {
			return nil, err
		}
		manifest.Notes = string(notes)
	}
	base := repository + "/releases/download/v" + r.version + "/"
	for _, artifact := range []struct {
		name    string
		targets []string
	}{
		{mac, []string{"darwin-aarch64", "darwin-aarch64-app"}},
		{windows, []string{"windows-x86_64", "windows-x86_64-nsis"}},
		{appImage, []string{"linux-x86_64", "linux-x86_64-appimage"}},
		{deb, []string{"linux-x86_64-deb"}},
		{rpm, []string{"linux-x86_64-rpm"}},
	} {
		var alternateName string
		if !native && artifact.name == mac {
			// Tauri signs the archive before renaming the release asset.
			alternateName = tauriMacArchiveName
		}
		signature, err := verifyAsset(r, artifact.name, alternateName, key)
		if err != nil {
			return nil, err
		}
		for _, target := range artifact.targets {
			manifest.Platforms[target] = platform{URL: base + artifact.name, Signature: signature}
		}
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	return append(data, '\n'), err
}
