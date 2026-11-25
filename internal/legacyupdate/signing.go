package legacyupdate

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aead.dev/minisign"
)

func LoadSigningKey(encoded, password, expected string) (minisign.PrivateKey, error) {
	var private minisign.PrivateKey
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(key) == 0 {
		return private, errors.New("TAURI_SIGNING_PRIVATE_KEY must contain a base64-encoded Minisign private key")
	}
	defer clear(key)
	if minisign.IsEncrypted(key) {
		private, err = minisign.DecryptKey(password, key)
	} else {
		err = private.UnmarshalText(key)
	}
	if err != nil {
		return private, errors.New("could not load the legacy Minisign signing key with the supplied password")
	}
	publicBytes, err := base64.StdEncoding.DecodeString(expected)
	var public minisign.PublicKey
	if err != nil || public.UnmarshalText(publicBytes) != nil || !public.Equal(private.Public()) {
		return minisign.PrivateKey{}, errors.New("the signing key does not match the installed GitWave updater public key")
	}
	return private, nil
}

func SignArtifact(path, version string, private minisign.PrivateKey) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := minisign.NewReader(file)
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return nil, err
	}
	trusted := fmt.Sprintf("timestamp:%d\tfile:%s\tversion:%s", time.Now().Unix(), filepath.Base(path), version)
	signature := reader.SignWithComments(private, trusted, "signature from GitWave legacy updater key")
	if !reader.Verify(private.Public().(minisign.PublicKey), signature) {
		return nil, errors.New("could not verify the generated legacy Minisign signature")
	}
	return base64.StdEncoding.AppendEncode(nil, signature), nil
}
