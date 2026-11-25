package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gitwave.work/internal/appupdate"
)

func main() {
	checkKey := flag.Bool("check-key", false, "verify the update signing key without signing files")
	previous := flag.String("verify-manifest", "", "verify a published manifest for -version using the configured public key")
	version := flag.String("version", "", "GitWave release version without the v prefix")
	assets := flag.String("assets", "", "directory containing all three update artifacts")
	windowsApp := flag.String("windows-exe", "", "path to the signed GitWave.exe embedded in the Windows installer")
	notes := flag.String("notes", "", "optional release notes file")
	flag.Parse()
	var err error
	switch {
	case *checkKey && *previous != "":
		err = errors.New("-check-key and -verify-manifest cannot be combined")
	case *checkKey:
		var key ed25519.PrivateKey
		key, err = signingKey()
		clear(key)
	case *previous != "":
		err = verifyPublishedManifest(*previous, *version)
	default:
		err = sign(*version, *assets, *windowsApp, *notes)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func verifyPublishedManifest(path, version string) error {
	if version == "" {
		return errors.New("-verify-manifest requires the published -version")
	}
	public, err := base64.StdEncoding.DecodeString(os.Getenv("GITWAVE_UPDATE_PUBLIC_KEY"))
	if err != nil || len(public) != ed25519.PublicKeySize {
		return errors.New("GITWAVE_UPDATE_PUBLIC_KEY must be a base64-encoded Ed25519 public key")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, appupdate.MaxManifestSize+1))
	if err != nil {
		return err
	}
	manifest, err := appupdate.VerifyManifest(data, public)
	if err != nil {
		return fmt.Errorf("published Go release v%s: %w", version, err)
	}
	if manifest.Version != version {
		return fmt.Errorf("published Go release v%s contains the update manifest for v%s", version, manifest.Version)
	}
	return nil
}

func signingKey() (ed25519.PrivateKey, error) {
	wantPublic := os.Getenv("GITWAVE_UPDATE_PUBLIC_KEY")
	if wantPublic == "" {
		return nil, errors.New("missing GITWAVE_UPDATE_PUBLIC_KEY")
	}
	encodedSeed := os.Getenv("GITWAVE_UPDATE_SIGNING_SEED")
	if encodedSeed == "" {
		return nil, errors.New("missing GITWAVE_UPDATE_SIGNING_SEED")
	}
	seed, err := base64.StdEncoding.DecodeString(encodedSeed)
	defer clear(seed)
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("GITWAVE_UPDATE_SIGNING_SEED must be a base64-encoded Ed25519 seed")
	}
	private := ed25519.NewKeyFromSeed(seed)
	public := base64.StdEncoding.EncodeToString(private.Public().(ed25519.PublicKey))
	if public != wantPublic {
		clear(private)
		return nil, errors.New("the GitWave update signing key does not match GITWAVE_UPDATE_PUBLIC_KEY")
	}
	return private, nil
}

func sign(version, assets, windowsApp, notesPath string) error {
	if version == "" || assets == "" || windowsApp == "" {
		return errors.New("-version, -assets and -windows-exe are required")
	}
	private, err := signingKey()
	if err != nil {
		return err
	}
	defer clear(private)
	manifest := appupdate.Manifest{Schema: 1, Version: version, Packages: make(map[string]appupdate.Package)}
	if notesPath != "" {
		notes, err := os.ReadFile(notesPath)
		if err != nil {
			return err
		}
		manifest.Notes = string(notes)
	}
	for _, target := range []string{appupdate.MacOSArm64, appupdate.WindowsX64, appupdate.LinuxX64} {
		name, err := appupdate.ArtifactName(version, target)
		if err != nil {
			return err
		}
		pkg, err := appupdate.DescribePackage(filepath.Join(assets, name))
		if err != nil {
			return fmt.Errorf("describe %s: %w", name, err)
		}
		if target == appupdate.WindowsX64 {
			executable, err := appupdate.DescribePackage(windowsApp)
			if err != nil {
				return fmt.Errorf("describe the installed Windows executable: %w", err)
			}
			pkg.ExecutableSHA256 = executable.SHA256
		}
		manifest.Packages[target] = pkg
	}
	data, err := appupdate.Sign(manifest, private)
	if err != nil {
		return err
	}
	return writeManifest(assets, data)
}

func writeManifest(dir string, data []byte) error {
	file, err := os.CreateTemp(dir, ".gitwave-update-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(append(data, '\n')); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), filepath.Join(dir, appupdate.ManifestName))
}
