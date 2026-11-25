package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gitwave.work/internal/distasset"
	"gitwave.work/internal/gittool"
)

var minGit = distasset.Asset{
	Name:   "MinGit-2.56.1-64-bit.zip",
	URL:    "https://github.com/git-for-windows/git/releases/download/v2.56.1.windows.1/MinGit-2.56.1-64-bit.zip",
	SHA256: "4a92850b6e917da4fd010669b614b55dccf5b0ec333a534cc4be4700dfefdafd",
}

var gitLFS = distasset.Asset{
	Name:   "git-lfs-windows-amd64-v3.8.1.zip",
	URL:    "https://github.com/git-lfs/git-lfs/releases/download/v3.8.1/git-lfs-windows-amd64-v3.8.1.zip",
	SHA256: "5bb771c12b3dacc535e912d0270a010c209d4da2e63819c5918e3547c2defed9",
}

func stageWindowsGit(work, localAssets, noticesPath string) error {
	notices, err := os.ReadFile(noticesPath)
	if err != nil {
		return fmt.Errorf("read the NSGo license notices: %w", err)
	}
	directory := filepath.Join(work, gittool.Directory)
	if err := os.Mkdir(directory, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	gitArchive, err := openGitZIP(minGit, localAssets)
	if err != nil {
		return err
	}
	var expanded uint64
	seen := make(map[string]bool, len(gitArchive.File))
	for _, entry := range gitArchive.File {
		name := strings.TrimSuffix(entry.Name, "/")
		key := strings.ToLower(name)
		if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\:*?\"<>|$\r\n") || seen[key] {
			return fmt.Errorf("invalid MinGit archive path %q", entry.Name)
		}
		seen[key] = true
		const maxExpanded = 160 << 20
		if entry.UncompressedSize64 > maxExpanded-expanded {
			return fmt.Errorf("MinGit archive is too large")
		}
		expanded += entry.UncompressedSize64
		if entry.FileInfo().IsDir() {
			err = root.MkdirAll(name, 0o755)
		} else {
			err = unpackGitFile(root, entry, name)
		}
		if err != nil {
			return err
		}
	}
	lfsArchive, err := openGitZIP(gitLFS, localAssets)
	if err != nil {
		return err
	}
	for _, entry := range lfsArchive.File {
		if entry.Name == "git-lfs-3.8.1/git-lfs.exe" {
			if err := unpackGitFile(root, entry, "ucrt64/bin/git-lfs.exe"); err != nil {
				return err
			}
		}
	}
	license, err := distasset.LFSLicense.Read(localAssets)
	if err != nil {
		return err
	}
	if err := root.WriteFile("LICENSE-git-lfs.md", license, 0o644); err != nil {
		return err
	}
	sources := "Git for Windows (MinGit)\nSource: https://github.com/git-for-windows/git/tree/v2.56.1.windows.1\n" +
		"Packaging: https://github.com/git-for-windows/build-extra\n" +
		"\nGit LFS\nSource: https://github.com/git-lfs/git-lfs/tree/v3.8.1\n"
	for _, asset := range []distasset.Asset{minGit, gitLFS, distasset.LFSLicense} {
		sources += asset.Provenance()
	}
	if err := root.WriteFile("GitWave-third-party-sources.txt", []byte(sources), 0o644); err != nil {
		return err
	}
	if err := root.WriteFile("GitWave-"+thirdPartyNoticesName, notices, 0o644); err != nil {
		return err
	}
	return gittool.WriteWindowsInventory(directory)
}

func openGitZIP(asset distasset.Asset, localAssets string) (*zip.Reader, error) {
	data, err := asset.Read(localAssets)
	if err != nil {
		return nil, err
	}
	return zip.NewReader(bytes.NewReader(data), int64(len(data)))
}

func unpackGitFile(root *os.Root, entry *zip.File, name string) error {
	if !entry.Mode().IsRegular() {
		return fmt.Errorf("unexpected Git archive entry %q", entry.Name)
	}
	if err := root.MkdirAll(path.Dir(name), 0o755); err != nil {
		return err
	}
	file, err := entry.Open()
	if err != nil {
		return err
	}
	defer file.Close()
	output, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, file)
	closeErr := output.Close()
	return errors.Join(copyErr, closeErr)
}
