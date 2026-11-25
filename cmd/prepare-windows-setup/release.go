package main

import (
	"debug/pe"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gitwave.work/internal/appupdate"
)

func releaseSignHook(script, version, signer string) (string, error) {
	if _, err := appupdate.ArtifactName(version, appupdate.WindowsX64); err != nil || strings.Contains(version, "-") {
		return "", errors.New("the Windows release requires a three-part numeric version (X.Y.Z)")
	}
	script = strings.ReplaceAll(strings.TrimPrefix(script, "\ufeff"), "\r\n", "\n")
	id, product := setupIdentity.FindStringSubmatch(script), setupVersion.FindStringSubmatch(script)
	if len(id) != 2 || id[1] != appupdate.DesktopAppID || len(product) != 2 || product[1] != version {
		return "", errors.New("the Windows release needs a stable NSGo script for the requested version")
	}
	if strings.Contains(strings.ToLower(script), "!uninstfinalize") {
		return "", errors.New("the Windows release expects an NSGo script without a signing hook")
	}
	if strings.ContainsAny(signer, "\r\n\x00\"'%$") {
		return "", errors.New("the Windows signer path contains an invalid character")
	}
	return fmt.Sprintf("!uninstfinalize '\"%s\" \"%%1\"' = 0\n", signer), nil
}

func stageSignedWindowsApp(source, work, signer string) (string, error) {
	dir := filepath.Join(work, "windows-app")
	if err := os.Mkdir(dir, 0o700); err != nil {
		return "", err
	}
	for _, name := range []string{"GitWave.exe", thirdPartyNoticesName, "GitWave-setup.ico", "GitWave-uninstall.ico"} {
		path := filepath.Join(source, name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) && strings.HasSuffix(name, ".ico") {
			continue
		}
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("not a regular Windows build input: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			return "", err
		}
	}
	app := filepath.Join(dir, "GitWave.exe")
	file, err := pe.Open(app)
	if err != nil {
		return "", err
	}
	machine := file.FileHeader.Machine
	file.Close()
	if machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		return "", errors.New("the Windows release application must target AMD64")
	}
	return dir, signWindowsFile(signer, app)
}

func signWindowsFile(signer, path string) error {
	cmd := exec.Command(signer, path)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("sign %s: %w", filepath.Base(path), err)
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := checkWindowsCertificate(file); err != nil {
		return fmt.Errorf("check signed %s: %w", filepath.Base(path), err)
	}
	return nil
}

// The Windows workflow checks native trust in addition to this PE structure check.
func checkWindowsCertificate(file *os.File) error {
	peFile, err := pe.NewFile(file)
	if err != nil {
		return err
	}
	defer peFile.Close()
	var cert pe.DataDirectory
	switch header := peFile.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		cert = header.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_SECURITY]
	case *pe.OptionalHeader64:
		cert = header.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_SECURITY]
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	offset, size := int64(cert.VirtualAddress), int64(cert.Size)
	if offset < 8 || offset%8 != 0 || size <= 8 || offset+size > info.Size() {
		return errors.New("the signer did not produce an Authenticode certificate table")
	}
	var header [8]byte
	if _, err := file.ReadAt(header[:], offset); err != nil {
		return err
	}
	length := int64(binary.LittleEndian.Uint32(header[:4]))
	if length <= 8 || length > size || binary.LittleEndian.Uint16(header[4:6]) != 0x0200 || binary.LittleEndian.Uint16(header[6:]) != 0x0002 {
		return errors.New("invalid Windows Authenticode certificate entry")
	}
	return nil
}
