package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gitwave.work/internal/appupdate"
	"gitwave.work/internal/distasset"
)

func verifyLinuxRelease(stage, version string) error {
	image, err := appupdate.ArtifactName(version, appupdate.LinuxX64)
	if err != nil {
		return err
	}
	base := strings.TrimSuffix(image, ".AppImage")
	debian, rpm := base+".deb", base+".rpm"
	releases := filepath.Join(stage, "releases")
	debPath := filepath.Join(releases, debian)
	fields, err := exec.Command("dpkg-deb", "-f", debPath, "Package", "Version", "Architecture").CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect the Linux release DEB: %w: %s", err, fields)
	}
	wantDeb := fmt.Sprintf("Package: %s\nVersion: %s\nArchitecture: amd64", distasset.LinuxPackageName, version)
	if strings.TrimSpace(string(fields)) != wantDeb {
		return fmt.Errorf("unexpected Linux release DEB metadata: %s", fields)
	}
	if err := run(exec.Command("go", "run", "./cmd/prepare-linux-rpm", "-deb", debPath, "-out", releases)); err != nil {
		return err
	}
	rpmPath := filepath.Join(releases, rpm)
	fields, err = exec.Command("rpm", "-qp", "--queryformat", "%{NAME} %{VERSION} %{ARCH}", rpmPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect the Linux release RPM: %w: %s", err, fields)
	}
	if want := fmt.Sprintf("%s %s x86_64", distasset.LinuxPackageName, version); string(fields) != want {
		return fmt.Errorf("unexpected Linux release RPM metadata: %s", fields)
	}
	for _, name := range []string{image, debian, rpm} {
		path := filepath.Join(releases, name)
		pkg, err := appupdate.DescribePackage(path)
		if err != nil {
			return err
		}
		digest, err := os.ReadFile(path + ".sha256")
		if err != nil {
			return err
		}
		if string(digest) != fmt.Sprintf("%s  %s\n", pkg.SHA256, name) {
			return fmt.Errorf("Linux release checksum does not match %s", name)
		}
	}
	return nil
}
