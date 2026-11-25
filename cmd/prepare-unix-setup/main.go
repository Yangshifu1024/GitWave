package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"gitwave.work/internal/appupdate"
	"gitwave.work/internal/distasset"
)

const bundleEnvironment = "GITWAVE_PACKAGING_GIT_ROOT"
const macOSMinimum = "13.0"

func main() {
	output := flag.String("out", "", "directory for the NSGo installer and its Git sources")
	assets := flag.String("git-assets", "", "directory with pinned Git downloads for an offline build")
	gtkAssets := flag.String("gtk-assets", "", "directory with pinned Ubuntu GTK sources for a Linux build")
	identity := flag.String("sign", "", "macOS code signing identity")
	notarize := flag.String("notarize", "", "macOS notarytool keychain profile")
	release := flag.String("release-version", "", "build a macOS or Linux release for this version")
	flag.Parse()
	if err := prepareSetup(*output, *assets, *gtkAssets, *identity, *notarize, *release); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func prepareSetup(output, assets, gtkAssets, identity, notarize, release string) error {
	var target string
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/arm64":
		target = "macos_arm64"
	case "linux/amd64":
		target = "linux_x64"
	default:
		return fmt.Errorf("run the Unix installer build on macOS arm64 or Linux x64")
	}
	if output == "" {
		return fmt.Errorf("-out is required")
	}
	if (identity != "" || notarize != "") && runtime.GOOS != "darwin" {
		return fmt.Errorf("the signing options are for the macOS installer")
	}
	if gtkAssets != "" && runtime.GOOS != "linux" {
		return fmt.Errorf("-gtk-assets is for the Linux installer")
	}
	if notarize != "" && (identity == "" || identity == "-") {
		return fmt.Errorf("notarization requires a code signing identity")
	}
	output, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	buildOutput := output
	var releaseFlags []string
	if release != "" {
		releaseFlags, err = unixReleaseFlags(target, release, identity, notarize)
		if err != nil {
			return err
		}
		if target == "linux_x64" {
			for _, tool := range []string{"dpkg-deb", "rpm", "rpmbuild"} {
				if _, err := exec.LookPath(tool); err != nil {
					return fmt.Errorf("the Linux release requires %s: %w", tool, err)
				}
			}
		}
		if _, err := os.Lstat(output); !os.IsNotExist(err) {
			if err != nil {
				return err
			}
			return fmt.Errorf("the release output already exists: %s", output)
		}
		if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
			return err
		}
		buildOutput, err = os.MkdirTemp(filepath.Dir(output), ".gitwave-unix-release-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(buildOutput)
	}
	work, err := os.MkdirTemp("", "gitwave-unix-setup-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	packager, err := buildNSGoPackager(work)
	if err != nil {
		return err
	}
	bundle, err := buildGitBundle(work, assets, runtime.GOOS)
	if err != nil {
		return err
	}
	environment := append(os.Environ(), bundleEnvironment+"="+bundle, gtkEnvironment+"=")
	if runtime.GOOS == "linux" {
		gtk, err := buildGTKBundle(work, gtkAssets)
		if err != nil {
			return err
		}
		environment = append(environment, gtkEnvironment+"="+gtk)
	}
	args := []string{"build", "-target", target, "-installers", "-channel", "none", "-output", buildOutput}
	args = append(args, releaseFlags...)
	if identity != "" {
		args = append(args, "-sign", identity)
	}
	if notarize != "" {
		args = append(args, "-notarize", notarize)
	}
	cmd := exec.Command(packager, args...)
	cmd.Dir = filepath.Join("cmd", "gitwave")
	cmd.Env = environment
	if err := run(cmd); err != nil {
		return err
	}
	if err := publishGitSource(work, buildOutput); err != nil {
		return err
	}
	if runtime.GOOS == "linux" {
		if err := publishGTKSource(work, buildOutput); err != nil {
			return err
		}
		if release != "" {
			if err := verifyLinuxRelease(buildOutput, release); err != nil {
				return err
			}
			return distasset.PublishDirectory(buildOutput, output)
		}
	} else if release != "" {
		return publishMacRelease(buildOutput, output, release)
	}
	return nil
}

func unixReleaseFlags(target, version, identity, notarize string) ([]string, error) {
	if _, err := appupdate.ArtifactName(version, appupdate.MacOSArm64); err != nil || strings.Contains(version, "-") {
		return nil, fmt.Errorf("the release requires a three-part numeric version (X.Y.Z)")
	}
	if target == "macos_arm64" {
		if identity == "" || identity == "-" {
			return nil, fmt.Errorf("the macOS release requires a Developer ID Application identity")
		}
		if os.Getenv("NSGO_NOTARY_KEY") != "" {
			if os.Getenv("NSGO_NOTARY_KEY_ID") == "" || os.Getenv("NSGO_NOTARY_ISSUER") == "" {
				return nil, fmt.Errorf("NSGO_NOTARY_KEY requires NSGO_NOTARY_KEY_ID and NSGO_NOTARY_ISSUER")
			}
		} else if notarize == "" {
			return nil, fmt.Errorf("the macOS release requires notarytool credentials")
		}
	}
	publicKey := os.Getenv("GITWAVE_UPDATE_PUBLIC_KEY")
	key, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil || len(key) != ed25519.PublicKeySize || base64.StdEncoding.EncodeToString(key) != publicKey {
		return nil, fmt.Errorf("GITWAVE_UPDATE_PUBLIC_KEY must be a base64-encoded Ed25519 public key")
	}
	return []string{"-id", appupdate.DesktopAppID, "-version", version, "-ldflags", "-X main.releaseUpdateKey=" + publicKey}, nil
}

func run(cmd *exec.Cmd) error {
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(cmd.Path), err)
	}
	return nil
}
