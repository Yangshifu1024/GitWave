package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gitwave.work/internal/distasset"
)

var rpmVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[.+~][A-Za-z0-9.+~]+)?$`)

func main() {
	deb := flag.String("deb", "", "NSGo-generated Linux x64 deb to package as an RPM")
	out := flag.String("out", "", "directory for the RPM and its SHA-256 file")
	flag.Parse()
	if err := buildRPM(*deb, *out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func buildRPM(deb, output string) error {
	if deb == "" || output == "" {
		return fmt.Errorf("-deb and -out are required")
	}
	fields, err := exec.Command("dpkg-deb", "-f", deb, "Package", "Version", "Architecture").Output()
	if err != nil {
		return fmt.Errorf("read the Debian package metadata: %w", err)
	}
	metadata := make(map[string]string)
	for _, line := range strings.Split(string(fields), "\n") {
		key, value, ok := strings.Cut(line, ": ")
		if ok {
			metadata[key] = value
		}
	}
	version := metadata["Version"]
	name := metadata["Package"]
	if (name != distasset.LinuxPackageName && name != "gitwave" && name != "gitwave-preview") || metadata["Architecture"] != "amd64" || !rpmVersion.MatchString(version) {
		return fmt.Errorf("expected an x64 GitWave deb with an RPM-compatible version")
	}
	work, err := os.MkdirTemp("", "gitwave-rpm-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	payload := filepath.Join(work, "SOURCES", "payload")
	if err := os.MkdirAll(payload, 0o755); err != nil {
		return err
	}
	extract := exec.Command("dpkg-deb", "-x", deb, payload)
	extract.Stderr = os.Stderr
	if err := extract.Run(); err != nil {
		return fmt.Errorf("extract the Debian payload: %w", err)
	}
	slug := name
	if name == distasset.LinuxPackageName {
		slug = "gitwave"
	}
	files, err := rpmFileList(payload, slug)
	if err != nil {
		return err
	}
	spec := filepath.Join(work, "gitwave.spec")
	if err := os.WriteFile(spec, []byte(fmt.Sprintf(rpmSpec, name, version, files)), 0o644); err != nil {
		return err
	}
	pack := exec.Command("rpmbuild", "-bb", "--define", "_topdir "+work, spec)
	pack.Stdout, pack.Stderr = os.Stdout, os.Stderr
	if err := pack.Run(); err != nil {
		return fmt.Errorf("build the GitWave RPM: %w", err)
	}
	source := filepath.Join(work, "RPMS", "x86_64", name+"-"+version+"-1.x86_64.rpm")
	return publishRPM(source, output, strings.TrimSuffix(filepath.Base(deb), ".deb")+".rpm")
}

func rpmFileList(payload, name string) (string, error) {
	root, err := os.ReadDir(payload)
	if err != nil {
		return "", err
	}
	if len(root) != 2 || root[0].Name() != "opt" || !root[0].IsDir() || root[1].Name() != "usr" || !root[1].IsDir() {
		return "", fmt.Errorf("unexpected top-level GitWave package contents")
	}
	apps, err := os.ReadDir(filepath.Join(payload, "opt"))
	if err != nil {
		return "", err
	}
	if len(apps) != 1 || apps[0].Name() != name || !apps[0].IsDir() {
		return "", fmt.Errorf("expected the GitWave payload at /opt/%s", name)
	}
	var files strings.Builder
	fmt.Fprintf(&files, "/opt/%s\n", name)
	err = fs.WalkDir(os.DirFS(payload), "usr", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if (!entry.Type().IsRegular() && entry.Type() != fs.ModeSymlink) || strings.ContainsAny(path, "\r\n\t %\\*?[]\"") {
			return fmt.Errorf("unexpected desktop package entry %q", path)
		}
		files.WriteString("/" + path + "\n")
		return nil
	})
	return files.String(), err
}

func publishRPM(source, output, name string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := os.MkdirAll(output, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(output, ".gitwave-rpm-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	sha := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(temp, sha), file)
	if err := errors.Join(copyErr, temp.Chmod(0o644), temp.Close()); err != nil {
		return err
	}
	if err := os.Rename(temp.Name(), filepath.Join(output, name)); err != nil {
		return err
	}
	digest := fmt.Sprintf("%x  %s\n", sha.Sum(nil), name)
	return os.WriteFile(filepath.Join(output, name+".sha256"), []byte(digest), 0o644)
}

const rpmSpec = `%%global _build_id_links none
%%global __os_install_post %%{nil}

Name: %s
Version: %s
Release: 1
Summary: A local-first Git desktop client with optional AI assistance
License: MIT and bundled third-party licenses
URL: https://gitwave.work
BuildArch: x86_64
AutoReqProv: no
Requires: gtk4%%{?_isa} >= 4.14
Requires: glibc%%{?_isa} >= 2.35
Requires: ca-certificates
Requires: openssh-clients

%%description
GitWave is a local-first Git desktop client with optional AI assistance.

%%install
mkdir -p "%%{buildroot}"
cp -a -- "%%{_sourcedir}/payload/." "%%{buildroot}/"

%%files
%%defattr(-,root,root,-)
%s`
