//go:build !ios

package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/semver"

	"gitwave.work/internal/appupdate"
	"gitwave.work/internal/distasset"
	"gitwave.work/internal/legacyupdate"
)

func main() {
	bundle := flag.String("app", "", "path to the signed and notarized GitWave.app")
	version := flag.String("version", "", "version of the signed application (X.Y.Z)")
	output := flag.String("out", "", "new directory for the legacy macOS update archive and signature")
	flag.Parse()
	encoded := os.Getenv("TAURI_SIGNING_PRIVATE_KEY")
	password := os.Getenv("TAURI_SIGNING_PRIVATE_KEY_PASSWORD")
	_ = os.Unsetenv("TAURI_SIGNING_PRIVATE_KEY")
	_ = os.Unsetenv("TAURI_SIGNING_PRIVATE_KEY_PASSWORD")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if err := prepareBridge(ctx, *bundle, *version, *output, encoded, password); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func prepareBridge(ctx context.Context, bundle, version, output, encoded, password string) error {
	if bundle == "" || output == "" {
		return errors.New("-app, -version and -out are required")
	}
	if _, err := appupdate.ArtifactName(version, appupdate.MacOSArm64); err != nil || strings.Contains(version, "-") || semver.Compare("v"+version, "v0.11.0") <= 0 {
		return errors.New("the macOS bridge requires a numeric X.Y.Z version newer than 0.11.0")
	}
	bundle, err := filepath.EvalSymlinks(bundle)
	if err != nil {
		return err
	}
	bundle, err = filepath.Abs(bundle)
	if err != nil {
		return err
	}
	if filepath.Base(bundle) != "GitWave.app" {
		return errors.New("the macOS bridge requires GitWave.app")
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(output))
	if err != nil {
		return err
	}
	if err := checkOutputParent(bundle, parent); err != nil {
		return err
	}
	output = filepath.Join(parent, filepath.Base(output))
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		if err != nil {
			return err
		}
		return fmt.Errorf("the macOS bridge output already exists: %s", output)
	}
	private, err := legacyupdate.LoadSigningKey(encoded, password, legacyupdate.PublicKeyBase64)
	if err != nil {
		return err
	}
	hash, err := appupdate.VerifyMacReleaseBundle(ctx, bundle, version, appupdate.MacOSDeveloperIDTeam)
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".gitwave-macos-bridge-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	name := "GitWave_" + version + "_aarch64.app.tar.gz"
	archive := filepath.Join(stage, name)
	if err := writeArchive(ctx, bundle, archive); err != nil {
		return err
	}
	if err := verifyArchive(ctx, archive, version, hash); err != nil {
		return err
	}
	signature, err := legacyupdate.SignArtifact(archive, version, private)
	if err != nil {
		return err
	}
	if err := os.WriteFile(archive+".sig", signature, 0o644); err != nil {
		return err
	}
	pkg, err := appupdate.DescribePackage(archive)
	if err != nil {
		return err
	}
	if err := os.WriteFile(archive+".sha256", fmt.Appendf(nil, "%s  %s\n", pkg.SHA256, name), 0o644); err != nil {
		return err
	}
	return distasset.PublishDirectory(stage, output)
}

func checkOutputParent(bundle, parent string) error {
	source, err := os.Stat(bundle)
	if err != nil {
		return err
	}
	for dir := parent; ; dir = filepath.Dir(dir) {
		info, err := os.Stat(dir)
		if err != nil {
			return err
		}
		if os.SameFile(source, info) {
			return errors.New("the macOS bridge output must be outside the application bundle")
		}
		if filepath.Dir(dir) == dir {
			return nil
		}
	}
}

func writeArchive(ctx context.Context, bundle, archive string) (err error) {
	file, err := os.OpenFile(archive, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	compressed := gzip.NewWriter(file)
	defer func() { err = errors.Join(err, compressed.Close()) }()
	writer := tar.NewWriter(compressed)
	defer func() { err = errors.Join(err, writer.Close()) }()
	root := filepath.Dir(bundle)
	return filepath.WalkDir(bundle, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		var link string
		if info.Mode()&os.ModeSymlink != 0 {
			link, err = os.Readlink(path)
			if err != nil {
				return err
			}
		} else if !info.IsDir() && !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file type in application bundle: %s", path)
		}
		header, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		header.Name, err = filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			header.Name += "/"
		}
		header.Uid, header.Gid = 0, 0
		header.Uname, header.Gname = "", ""
		if err := writer.WriteHeader(header); err != nil || !info.Mode().IsRegular() {
			return err
		}
		content, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(writer, content)
		return errors.Join(err, content.Close())
	})
}

func verifyArchive(ctx context.Context, archive, version, expectedHash string) error {
	root, err := os.MkdirTemp("", "gitwave-macos-legacy-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	bundle := filepath.Join(root, "GitWave.app")
	if err := os.Mkdir(bundle, 0o755); err != nil {
		return err
	}
	// The installed legacy updater strips the archive's first path component.
	if output, err := exec.CommandContext(ctx, "/usr/bin/tar", "-xzf", archive, "--strip-components", "1", "-C", bundle).CombinedOutput(); err != nil {
		return fmt.Errorf("extract the macOS bridge: %w: %s", err, output)
	}
	hash, err := appupdate.VerifyMacReleaseBundle(ctx, bundle, version, appupdate.MacOSDeveloperIDTeam)
	if err != nil {
		return err
	}
	if hash != expectedHash {
		return errors.New("the macOS legacy archive contains a different application")
	}
	if output, err := exec.CommandContext(ctx, "/usr/bin/xcrun", "stapler", "validate", bundle).CombinedOutput(); err != nil {
		return fmt.Errorf("verify the macOS bridge notarization ticket: %w: %s", err, output)
	}
	return nil
}
