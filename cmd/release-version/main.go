package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gitwave.work/internal/appupdate"
	"go.yaml.in/yaml/v3"
)

const appConfig = "cmd/gitwave/nsgo.yaml"
const numericVersion = `[0-9]+\.[0-9]+\.[0-9]+`

var versionLine = regexp.MustCompile(`(?m)^version:[ \t]+(?:"(` + numericVersion + `)"|'(` + numericVersion + `)'|(` + numericVersion + `))[ \t]*(?:#[^\r\n]*)?\r?$`)

func main() {
	version, err := releaseVersion(appConfig, os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(version)
}

func releaseVersion(path string, args []string) (string, error) {
	if len(args) > 1 {
		return "", errors.New("usage: go run ./cmd/release-version [X.Y.Z]")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("the NSGo configuration must be a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var config struct {
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("read the NSGo configuration: %w", err)
	}
	if err := validateVersion(config.Version); err != nil {
		return "", err
	}
	if len(args) == 0 {
		return config.Version, nil
	}
	version := strings.TrimPrefix(args[0], "v")
	if err := validateVersion(version); err != nil {
		return "", err
	}
	if version == config.Version {
		return version, nil
	}
	indices := versionLine.FindAllSubmatchIndex(data, 2)
	if len(indices) != 1 {
		return "", errors.New("expected one top-level scalar version line in the NSGo configuration")
	}
	match := indices[0]
	for i := 2; i < len(match); i += 2 {
		start, end := match[i], match[i+1]
		if start >= 0 && string(data[start:end]) == config.Version {
			updated := make([]byte, 0, len(data)-end+start+len(version))
			updated = append(updated, data[:start]...)
			updated = append(updated, version...)
			updated = append(updated, data[end:]...)
			return version, replaceVersionFile(path, info.Mode().Perm(), data, updated)
		}
	}
	return "", errors.New("the scalar version line does not match the NSGo configuration")
}

func validateVersion(version string) error {
	if _, err := appupdate.ArtifactName(version, appupdate.MacOSArm64); err != nil || strings.Contains(version, "-") {
		return fmt.Errorf("invalid release version %q: expected X.Y.Z", version)
	}
	return nil
}

func replaceVersionFile(path string, mode os.FileMode, before, after []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".gitwave-version-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(after)
	if err := errors.Join(err, file.Chmod(mode), file.Close()); err != nil {
		return err
	}
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, before) {
		return errors.New("the NSGo configuration changed during the version update")
	}
	return os.Rename(file.Name(), path)
}
