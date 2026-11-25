package appupdate

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/mod/semver"
)

const ManifestName = "gitwave-update.json"
const MaxManifestSize = 512 << 10
const DesktopAppID = "desktop.gitwave.work"
const MacOSDeveloperIDTeam = "ZE5SZ85EZQ"

const (
	MacOSArm64 = "macos-arm64"
	WindowsX64 = "windows-x64"
	LinuxX64   = "linux-x64"

	maxNotesSize   = 32 << 10
	maxPackageSize = 512 << 20
)

var (
	ErrManifest  = errors.New("invalid GitWave update manifest")
	ErrSignature = errors.New("the GitWave update manifest signature is invalid")
	ErrPackage   = errors.New("the downloaded GitWave update does not match the signed manifest")
)

type Manifest struct {
	Schema   int                `json:"schema"`
	Version  string             `json:"version"`
	Notes    string             `json:"notes,omitempty"`
	Packages map[string]Package `json:"packages"`
}

type Package struct {
	Size             int64  `json:"size"`
	SHA256           string `json:"sha256"`
	ExecutableSHA256 string `json:"executableSHA256,omitempty"`
}

type envelope struct {
	Manifest  []byte `json:"manifest"`
	Signature []byte `json:"signature"`
}

func DescribePackage(path string) (Package, error) {
	file, err := os.Open(path)
	if err != nil {
		return Package{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Package{}, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxPackageSize {
		return Package{}, errors.New("the release artifact has an invalid size or file type")
	}
	digest := sha256.New()
	size, err := io.Copy(digest, io.LimitReader(file, info.Size()+1))
	if err != nil {
		return Package{}, err
	}
	if size != info.Size() {
		return Package{}, errors.New("the release artifact changed while it was read")
	}
	return Package{Size: size, SHA256: hex.EncodeToString(digest.Sum(nil))}, nil
}

// ArtifactName is shared by the feed producer and the update client.
func ArtifactName(version, target string) (string, error) {
	if !validVersion(version) {
		return "", ErrManifest
	}
	var suffix string
	switch target {
	case MacOSArm64:
		suffix = ".zip"
	case WindowsX64:
		suffix = "-setup.exe"
	case LinuxX64:
		suffix = ".AppImage"
	default:
		return "", ErrManifest
	}
	return "gitwave-" + version + "-" + target + suffix, nil
}

func validVersion(version string) bool {
	full := "v" + version
	return len(version) <= 80 && semver.IsValid(full) && semver.Canonical(full) == full
}

// Sign seals the exact manifest bytes, including version, target and digest.
func Sign(manifest Manifest, key ed25519.PrivateKey) ([]byte, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid GitWave update signing key")
	}
	if err := manifest.validate(); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{Manifest: payload, Signature: ed25519.Sign(key, payload)})
}

// VerifyManifest authenticates a published feed using the desktop client's checks.
func VerifyManifest(data []byte, key ed25519.PublicKey) (Manifest, error) {
	if len(data) > MaxManifestSize {
		return Manifest{}, ErrManifest
	}
	var sealed envelope
	if err := json.Unmarshal(data, &sealed); err != nil || len(sealed.Manifest) == 0 {
		return Manifest{}, ErrManifest
	}
	if len(key) != ed25519.PublicKeySize || !ed25519.Verify(key, sealed.Manifest, sealed.Signature) {
		return Manifest{}, ErrSignature
	}
	var manifest Manifest
	if err := json.Unmarshal(sealed.Manifest, &manifest); err != nil {
		return Manifest{}, ErrManifest
	}
	return manifest, manifest.validate()
}

func (m Manifest) validate() error {
	if m.Schema != 1 || !validVersion(m.Version) || len(m.Notes) > maxNotesSize || len(m.Packages) != 3 {
		return ErrManifest
	}
	for _, target := range []string{MacOSArm64, WindowsX64, LinuxX64} {
		pkg := m.Packages[target]
		if !validSHA256(pkg.SHA256) || pkg.Size <= 0 || pkg.Size > maxPackageSize {
			return fmt.Errorf("%w: %s", ErrManifest, target)
		}
		if target == WindowsX64 && !validSHA256(pkg.ExecutableSHA256) {
			return fmt.Errorf("%w: missing or invalid Windows executable hash", ErrManifest)
		}
	}
	return nil
}

func validSHA256(value string) bool {
	digest, err := hex.DecodeString(value)
	return err == nil && len(digest) == sha256.Size && strings.ToLower(value) == value
}
