// Package legacyupdate verifies artifacts for the published Tauri updater.
package legacyupdate

import (
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"aead.dev/minisign"
)

// PublicKeyBase64 is the updater key pinned in the published Tauri GitWave client.
const PublicKeyBase64 = "dW50cnVzdGVkIGNvbW1lbnQ6IG1pbmlzaWduIHB1YmxpYyBrZXk6IDU5NkQ1NzdFM0M5ODA0NjQKUldSa0JKZzhmbGR0V1lORnhJTHg5bGgxV0E0NVIxdE9kT2tkWUpqN2FqMTk1OXZMYmpyZW9RdEwK"

func VerifyArtifact(path, version, alternateName string, key minisign.PublicKey) (string, error) {
	const signatureLimit = 4096
	sigFile, err := os.Open(path + ".sig")
	if err != nil {
		return "", err
	}
	defer sigFile.Close()
	encoded, err := io.ReadAll(io.LimitReader(sigFile, signatureLimit+1))
	if err != nil {
		return "", err
	}
	if len(encoded) > signatureLimit {
		return "", errors.New("legacy signature exceeds its size limit")
	}
	text := strings.TrimSpace(string(encoded))
	signature, err := base64.StdEncoding.DecodeString(text)
	if err != nil || base64.StdEncoding.EncodeToString(signature) != text {
		return "", errors.New("legacy signature is not canonical base64")
	}
	if !utf8.Valid(signature) {
		return "", errors.New("legacy signature is not valid UTF-8")
	}
	var parsed minisign.Signature
	if err := parsed.UnmarshalText(signature); err != nil {
		return "", err
	}
	if parsed.Algorithm != minisign.HashEdDSA {
		return "", errors.New("legacy release artifacts require a prehashed Minisign signature")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return "", errors.New("legacy update artifact must be a nonempty regular file")
	}
	reader := minisign.NewReader(file)
	size, err := io.Copy(io.Discard, io.LimitReader(reader, info.Size()+1))
	if err != nil {
		return "", err
	}
	if size != info.Size() || !reader.Verify(key, signature) {
		return "", errors.New("legacy update artifact failed Minisign verification")
	}
	var announced, filename string
	for field := range strings.SplitSeq(parsed.TrustedComment, "\t") {
		name, value, _ := strings.Cut(field, ":")
		switch name {
		case "version":
			if announced != "" || strings.TrimPrefix(value, "v") != version {
				return "", errors.New("legacy signature is bound to a different or ambiguous version")
			}
			announced = value
		case "file":
			if filename != "" || value == "" || (value != filepath.Base(path) && value != alternateName) {
				return "", errors.New("legacy signature is bound to a different or ambiguous artifact name")
			}
			filename = value
		}
	}
	if announced == "" || filename == "" {
		return "", errors.New("legacy signature lacks a signed version or artifact name")
	}
	return text, nil
}
