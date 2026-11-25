package gittool

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

const inventoryFile = "gitwave-bundle.json"
const inventoryLimit = 256 << 10

var windowsBundleRequired = []string{
	"cmd/git.exe", "ucrt64/bin/git-lfs.exe", "ucrt64/bin/git-credential-manager.exe",
	"ucrt64/bin/git-remote-https.exe", "usr/bin/ssh.exe", "usr/bin/ssh-add.exe",
}

type inventory struct {
	Owner         string            `json:"owner"`
	Entries       map[string]bool   `json:"directories_by_path"`
	Configuration map[string]string `json:"configuration_sha256"`
}

func WriteWindowsInventory(path string) error {
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	defer root.Close()
	entries, err := windowsBundleEntries(root)
	if err != nil {
		return err
	}
	for _, required := range windowsBundleRequired {
		if isDir, found := entries[required]; !found || isDir {
			return fmt.Errorf("Windows Git bundle is missing %s", required)
		}
	}
	configuration := make(map[string]string)
	for name, isDir := range entries {
		if !isDir && windowsConfigurationFile(name) {
			digest, err := windowsConfigurationDigest(root, name)
			if err != nil {
				return err
			}
			configuration[name] = digest
		}
	}
	data, err := json.MarshalIndent(inventory{Owner: "desktop.gitwave.work", Entries: entries, Configuration: configuration}, "", "  ")
	if err != nil {
		return err
	}
	return root.WriteFile(inventoryFile, append(data, '\n'), 0o644)
}

// CheckWindowsBundle verifies ownership before a Windows installer moves or
// removes the private toolchain. Extra entries may belong to the user.
func CheckWindowsBundle(path string, complete bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not an ordinary Git bundle directory", path)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	defer root.Close()
	if marker, err := root.Lstat(inventoryFile); err != nil || !marker.Mode().IsRegular() {
		return fmt.Errorf("%s does not have a GitWave bundle inventory", path)
	}
	file, err := root.Open(inventoryFile)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(file, inventoryLimit+1))
	if err := errors.Join(err, file.Close()); err != nil {
		return err
	}
	var contents inventory
	if len(data) > inventoryLimit || json.Unmarshal(data, &contents) != nil || contents.Owner != "desktop.gitwave.work" {
		return fmt.Errorf("invalid GitWave bundle inventory in %s", path)
	}
	for name := range contents.Entries {
		if !fs.ValidPath(name) || name == "." || name == inventoryFile {
			return fmt.Errorf("invalid GitWave bundle path %q", name)
		}
	}
	entries, err := windowsBundleEntries(root)
	if err != nil {
		return err
	}
	delete(entries, inventoryFile)
	for _, required := range windowsBundleRequired {
		isDir, owned := contents.Entries[required]
		if !owned || isDir {
			return fmt.Errorf("invalid Windows Git bundle entry %s", required)
		}
	}
	for name, isDir := range entries {
		if expected, owned := contents.Entries[name]; !owned || expected != isDir {
			return fmt.Errorf("unrecognized entry in GitWave's Git bundle: %s", name)
		}
		if !isDir && windowsConfigurationFile(name) {
			digest, err := windowsConfigurationDigest(root, name)
			if err != nil {
				return err
			}
			if digest != contents.Configuration[name] {
				return fmt.Errorf("GitWave's bundled configuration was modified: %s", name)
			}
		}
	}
	if complete && len(entries) != len(contents.Entries) {
		return fmt.Errorf("incomplete Windows Git bundle in %s", path)
	}
	return nil
}

func windowsConfigurationFile(name string) bool {
	return strings.HasPrefix(name, "etc/") || strings.HasPrefix(name, "ucrt64/etc/")
}

func windowsConfigurationDigest(root *os.Root, name string) (string, error) {
	file, err := root.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	const maxSize = 8 << 20
	size, err := io.Copy(digest, io.LimitReader(file, maxSize+1))
	if err != nil {
		return "", err
	}
	if size > maxSize {
		return "", fmt.Errorf("Git configuration exceeds the bundle size limit: %s", name)
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}

func windowsBundleEntries(root *os.Root) (map[string]bool, error) {
	entries := make(map[string]bool)
	err := fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || path == "." {
			return err
		}
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return fmt.Errorf("invalid Windows Git bundle entry %s", path)
		}
		entries[path] = entry.IsDir()
		return nil
	})
	return entries, err
}
