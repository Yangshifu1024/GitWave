//go:build !ios

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gitwave.work/internal/appupdate"
	"gitwave.work/internal/distasset"
)

func publishMacRelease(stage, output, version string) error {
	if err := prepareMacUpdate(stage, version); err != nil {
		return err
	}
	if err := distasset.PublishDirectory(stage, output); err != nil {
		return fmt.Errorf("publish macOS release to %s: %w", output, err)
	}
	return nil
}

func prepareMacUpdate(output, version string) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	name, err := appupdate.ArtifactName(version, appupdate.MacOSArm64)
	if err != nil {
		return err
	}
	releases := filepath.Join(output, "releases")
	dmg := filepath.Join(releases, strings.TrimSuffix(name, ".zip")+".dmg")
	bundle := filepath.Join(output, "GitWave-"+version+"-macos_arm64", "GitWave.app")
	hash, err := appupdate.VerifyMacReleaseBundle(ctx, bundle, version, appupdate.MacOSDeveloperIDTeam)
	if err != nil {
		return err
	}
	requirement := `anchor apple generic and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and certificate leaf[subject.OU] = "` + appupdate.MacOSDeveloperIDTeam + `"`
	if err := run(exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--strict", "-R="+requirement, "--", dmg)); err != nil {
		return err
	}
	if err := run(exec.CommandContext(ctx, "/usr/bin/xcrun", "stapler", "validate", dmg)); err != nil {
		return err
	}
	mount, err := os.MkdirTemp("", "gitwave-macos-release-check-")
	if err != nil {
		return err
	}
	defer os.Remove(mount)
	if err := run(exec.CommandContext(ctx, "/usr/bin/hdiutil", "attach", "-readonly", "-nobrowse", "-mountpoint", mount, dmg)); err != nil {
		return err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		err = errors.Join(err, run(exec.CommandContext(cleanup, "/usr/bin/hdiutil", "detach", mount)))
	}()
	diskHash, err := appupdate.VerifyMacReleaseBundle(ctx, filepath.Join(mount, "GitWave.app"), version, appupdate.MacOSDeveloperIDTeam)
	if err != nil {
		return err
	}
	if diskHash != hash {
		return errors.New("the macOS DMG contains a different application")
	}
	archive := filepath.Join(releases, name)
	if err := run(exec.CommandContext(ctx, "/usr/bin/ditto", "-c", "-k", "--sequesterRsrc", "--keepParent", bundle, archive)); err != nil {
		return err
	}
	if err := appupdate.VerifyMacUpdateArchive(ctx, archive, version, appupdate.MacOSDeveloperIDTeam, hash); err != nil {
		return err
	}
	for _, artifact := range []string{dmg, archive} {
		pkg, err := appupdate.DescribePackage(artifact)
		if err != nil {
			return err
		}
		if err := os.WriteFile(artifact+".sha256", fmt.Appendf(nil, "%s  %s\n", pkg.SHA256, filepath.Base(artifact)), 0o644); err != nil {
			return err
		}
	}
	return nil
}
