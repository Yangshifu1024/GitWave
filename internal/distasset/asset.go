// Package distasset handles pinned build inputs and staged desktop releases.
package distasset

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// LinuxPackageName preserves the published DEB and RPM installation identity.
const LinuxPackageName = "git-wave"

type Asset struct {
	Name, URL, SHA256 string
}

var LFSLicense = Asset{
	Name:   "git-lfs-v3.8.1-LICENSE.md",
	URL:    "https://raw.githubusercontent.com/git-lfs/git-lfs/v3.8.1/LICENSE.md",
	SHA256: "4fae9062ab5cdd5fb15486b728534d8aded8b4ae9d84b6d66a956f5162c366b6",
}

func (a Asset) Read(directory string) ([]byte, error) {
	var input io.ReadCloser
	if directory != "" {
		file, err := os.Open(filepath.Join(directory, a.Name))
		if err != nil {
			return nil, err
		}
		input = file
	} else {
		client := http.Client{Timeout: 2 * time.Minute}
		resp, err := client.Get(a.URL)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("download %s: %s", a.Name, resp.Status)
		}
		input = resp.Body
	}
	defer input.Close()
	const maxSize = 64 << 20
	data, err := io.ReadAll(io.LimitReader(input, maxSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxSize || fmt.Sprintf("%x", sha256.Sum256(data)) != a.SHA256 {
		return nil, fmt.Errorf("SHA-256 mismatch for %s", a.Name)
	}
	return data, nil
}

func (a Asset) Provenance() string {
	return fmt.Sprintf("\n%s\n%s\nSHA-256: %s\n", a.Name, a.URL, a.SHA256)
}
