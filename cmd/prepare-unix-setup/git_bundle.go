package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"gitwave.work/internal/distasset"
	"gitwave.work/internal/gittool"
)

const gitVersion = "2.56.1"
const lfsPath = "git-lfs-3.8.1/git-lfs"

var gitSource = distasset.Asset{
	Name:   "git-" + gitVersion + ".tar.xz",
	URL:    "https://www.kernel.org/pub/software/scm/git/git-" + gitVersion + ".tar.xz",
	SHA256: "f0b0bf89264b9bd2e1c640363e7529643989da76281600473c50745cd34a607f",
}

var unixLFS = map[string]distasset.Asset{
	"darwin": {
		Name:   "git-lfs-darwin-arm64-v3.8.1.zip",
		URL:    "https://github.com/git-lfs/git-lfs/releases/download/v3.8.1/git-lfs-darwin-arm64-v3.8.1.zip",
		SHA256: "0ba10e897e0496412cd86eacaed4ed554c97ddc29bc5c709d28faa5eaaa2222c",
	},
	"linux": {
		Name:   "git-lfs-linux-amd64-v3.8.1.tar.gz",
		URL:    "https://github.com/git-lfs/git-lfs/releases/download/v3.8.1/git-lfs-linux-amd64-v3.8.1.tar.gz",
		SHA256: "e72cf723a4a660438862282e8aecf3885fc8123e88bfcad534cf6b941a354b75",
	},
}

func buildGitBundle(work, assets, platform string) (string, error) {
	archive, err := writeAsset(gitSource, work, assets)
	if err != nil {
		return "", err
	}
	if err := run(exec.Command("tar", "-xf", archive, "-C", work)); err != nil {
		return "", err
	}
	source := filepath.Join(work, "git-"+gitVersion)
	stage := filepath.Join(work, "install")
	bundle := filepath.Join(stage, gittool.Directory)
	makeFlags := []string{"prefix=/" + gittool.Directory, "RUNTIME_PREFIX=YesPlease", "INSTALL_SYMLINKS=YesPlease",
		"NO_INSTALL_HARDLINKS=YesPlease", "NO_RUST=YesPlease", "NO_TCLTK=YesPlease", "NO_GETTEXT=YesPlease",
		"NO_OPENSSL=YesPlease"}
	if platform == "darwin" {
		makeFlags = append(makeFlags, "MACOSX_DEPLOYMENT_TARGET="+macOSMinimum, "install", "install-git-credential-osxkeychain")
	} else {
		makeFlags = append(makeFlags, "sysconfdir=/etc", "install")
	}
	makeArgs := append([]string{"-C", source, "-j" + strconv.Itoa(min(runtime.GOMAXPROCS(0), 8)), "DESTDIR=" + stage}, makeFlags...)
	if err := run(exec.Command("make", makeArgs...)); err != nil {
		return "", err
	}
	if platform == "linux" {
		if err := stageLinuxRuntime(bundle); err != nil {
			return "", err
		}
		config := filepath.Join(bundle, "etc", "gitconfig-rpm")
		if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
			return "", err
		}
		const systemConfig = "[http]\n\tsslCAInfo = /etc/pki/tls/certs/ca-bundle.crt\n" +
			"\tproxySSLCAInfo = /etc/pki/tls/certs/ca-bundle.crt\n[include]\n\tpath = /etc/gitconfig\n"
		if err := os.WriteFile(config, []byte(systemConfig), 0o644); err != nil {
			return "", err
		}
	}
	if platform == "darwin" {
		config := filepath.Join(bundle, "etc", "gitconfig")
		if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
			return "", err
		}
		const systemConfig = "[credential]\n\thelper = osxkeychain\n[include]\n\tpath = /etc/gitconfig\n"
		if err := os.WriteFile(config, []byte(systemConfig), 0o644); err != nil {
			return "", err
		}
	}
	lfs := unixLFS[platform]
	lfsArchive, err := writeAsset(lfs, work, assets)
	if err != nil {
		return "", err
	}
	cmd := exec.Command("tar", "-xOzf", lfsArchive, lfsPath)
	if platform == "darwin" {
		cmd = exec.Command("unzip", "-p", lfsArchive, lfsPath)
	}
	data, err := cmd.Output()
	if err != nil || len(data) == 0 {
		return "", fmt.Errorf("extract the Git LFS executable from %s: %v", lfs.Name, err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "bin", "git-lfs"), data, 0o755); err != nil {
		return "", err
	}
	licenses, err := distasset.LFSLicense.Read(assets)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(bundle, "LICENSE-git-lfs.md"), licenses, 0o644); err != nil {
		return "", err
	}
	copying, err := os.ReadFile(filepath.Join(source, "COPYING"))
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(bundle, "COPYING-git"), copying, 0o644); err != nil {
		return "", err
	}
	provenance := "Git release source and Git LFS distribution\n" + gitSource.Provenance() + lfs.Provenance() +
		distasset.LFSLicense.Provenance() + "\nBuild from the Git release source with GNU Make:\nmake DESTDIR=/tmp/gitwave-stage " + strings.Join(makeFlags, " ") + "\n"
	if err := os.WriteFile(filepath.Join(bundle, "GitWave-third-party-sources.txt"), []byte(provenance), 0o644); err != nil {
		return "", err
	}
	return bundle, nil
}

func writeAsset(asset distasset.Asset, work, assets string) (string, error) {
	data, err := asset.Read(assets)
	if err != nil {
		return "", err
	}
	file := filepath.Join(work, asset.Name)
	return file, os.WriteFile(file, data, 0o644)
}

func publishGitSource(work, output string) error {
	dir := filepath.Join(output, "git-source")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(work, gitSource.Name))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, gitSource.Name), data, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, gitSource.Name+".sha256"), []byte(gitSource.SHA256+"  "+gitSource.Name+"\n"), 0o644)
}
