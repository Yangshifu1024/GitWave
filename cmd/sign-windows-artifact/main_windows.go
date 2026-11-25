package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const timestamp = "http://timestamp.acs.microsoft.com"

func main() {
	if err := sign(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func sign() error {
	if len(os.Args) != 2 {
		return errors.New("sign-windows-artifact requires a single file path")
	}
	signtool, dlib, metadata := os.Getenv("GITWAVE_WINDOWS_SIGNTOOL"), os.Getenv("GITWAVE_WINDOWS_SIGNING_DLIB"), os.Getenv("GITWAVE_WINDOWS_SIGNING_METADATA")
	for _, path := range []string{signtool, dlib, metadata} {
		if !filepath.IsAbs(path) {
			return errors.New("Windows signing requires absolute SignTool, Dlib, and metadata paths")
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("missing Windows signing file: %s", path)
		}
	}
	file, err := filepath.Abs(os.Args[1])
	if err != nil {
		return err
	}
	if err := run(signtool, "sign", "/fd", "SHA256", "/td", "SHA256", "/tr", timestamp, "/dlib", dlib, "/dmdf", metadata, file); err != nil {
		return err
	}
	return run(signtool, "verify", "/pa", "/all", "/tw", file)
}

func run(executable string, args ...string) error {
	cmd := exec.Command(executable, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("SignTool %s failed: %w", args[0], err)
	}
	return nil
}
