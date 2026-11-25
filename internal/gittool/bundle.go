package gittool

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

const Directory = "gitwave-git"

func Git() (string, error) {
	root, err := installedRoot()
	if err != nil {
		return "", err
	}
	return findGit(root, runtime.GOOS)
}

func installedRoot() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	return bundleRoot(executable, runtime.GOOS), nil
}

func bundleRoot(executable, platform string) string {
	directory := filepath.Dir(executable)
	if platform == "darwin" && filepath.Base(directory) == "MacOS" && filepath.Base(filepath.Dir(directory)) == "Contents" {
		directory = filepath.Join(filepath.Dir(directory), "Resources")
	}
	return filepath.Join(directory, Directory)
}

func findGit(root, platform string) (string, error) {
	info, err := os.Lstat(root)
	switch {
	case err == nil:
		if !info.IsDir() {
			return "", fmt.Errorf("invalid bundled Git directory %s", root)
		}
		git, err := exec.LookPath(gitPath(root, platform))
		if err != nil {
			return "", fmt.Errorf("find bundled Git: %w", err)
		}
		return git, nil
	case !errors.Is(err, os.ErrNotExist):
		return "", err
	}
	git, err := exec.LookPath("git")
	if err != nil {
		return "", fmt.Errorf("find Git: %w", err)
	}
	if !filepath.IsAbs(git) {
		return "", errors.New("Git executable must resolve to an absolute path")
	}
	return git, nil
}

func gitPath(root, platform string) string {
	if platform == "windows" {
		return filepath.Join(root, "cmd", "git.exe")
	}
	return filepath.Join(root, "bin", "git")
}

func rootForGit(git string) string {
	root := filepath.Dir(filepath.Dir(git))
	if filepath.Base(root) == Directory && git == gitPath(root, runtime.GOOS) {
		return root
	}
	return ""
}

func BundledLFS(git string) (path string, bundled bool) {
	root := rootForGit(git)
	if root == "" {
		return "", false
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(root, "ucrt64", "bin", "git-lfs.exe"), true
	}
	return filepath.Join(root, "bin", "git-lfs"), true
}

func WindowsSSH(name string) (string, error) {
	root, err := installedRoot()
	if err != nil {
		return "", err
	}
	return exec.LookPath(filepath.Join(root, "usr", "bin", name+".exe"))
}

// Environment keeps Git, transport helpers and Git LFS on the same toolchain
// without changing the desktop process's PATH or user configuration.
func Environment(env []string, git, lfs string) []string {
	root := rootForGit(git)
	if root == "" && lfs == "" {
		return env
	}
	paths := []string{filepath.Dir(git)}
	if root != "" && runtime.GOOS == "linux" {
		env = linuxTrustEnvironment(env, root)
	}
	if root != "" && runtime.GOOS == "windows" {
		paths = append(paths, filepath.Join(root, "ucrt64", "bin"), filepath.Join(root, "usr", "bin"))
	}
	if lfs != "" && !slices.Contains(paths, filepath.Dir(lfs)) {
		paths = append(paths, filepath.Dir(lfs))
	}
	separator := string(os.PathListSeparator)
	prefix := strings.Join(paths, separator)
	for i := len(env) - 1; i >= 0; i-- {
		key, value, _ := strings.Cut(env[i], "=")
		if key == "PATH" || (runtime.GOOS == "windows" && strings.EqualFold(key, "PATH")) {
			if value == prefix || strings.HasPrefix(value, prefix+separator) {
				return env
			}
			if value != "" {
				prefix += separator + value
			}
			break
		}
	}
	return append(env, "PATH="+prefix)
}

func linuxTrustEnvironment(env []string, root string) []string {
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if name == "GIT_CONFIG_SYSTEM" || name == "GIT_CONFIG_NOSYSTEM" {
			return env
		}
	}
	// The bundled libcurl is built on Ubuntu. Keep the distro trust for
	// Red Hat hosts without overriding global or repository Git settings.
	if _, err := os.Stat("/etc/ssl/certs/ca-certificates.crt"); !errors.Is(err, os.ErrNotExist) {
		return env
	}
	if _, err := os.Stat("/etc/pki/tls/certs/ca-bundle.crt"); err != nil {
		return env
	}
	config := filepath.Join(root, "etc", "gitconfig-rpm")
	if info, err := os.Lstat(config); err == nil && info.Mode().IsRegular() {
		return append(env, "GIT_CONFIG_SYSTEM="+config)
	}
	return env
}
