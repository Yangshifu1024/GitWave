package appupdate

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitwave.work/internal/proxy"
	"golang.org/x/mod/semver"
)

const repositoryURL = "https://github.com/Yangshifu1024/GitWave"

var ErrNetwork = errors.New("could not reach the GitWave update server")

type Client struct {
	release *url.URL
	key     ed25519.PublicKey
	http    http.Client
}

type Restart func(context.Context) (finish func(quit bool) error, err error)

type Installer func(context.Context, *Candidate, func(received, total int64)) (Restart, error)

type Candidate struct {
	version, notes, artifact string
	packageInfo              Package
	client                   *Client
}

func (c *Candidate) Version() string { return c.version }
func (c *Candidate) Notes() string   { return c.notes }

func (c *Candidate) ReleaseNotesURL() string {
	return c.client.release.JoinPath("tag", "v"+c.version).String()
}

func New(publicKey string) (*Client, error) {
	key, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("this build has no GitWave update verification key")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = proxy.ForRequest
	return newClient(repositoryURL, key, transport), nil
}

func newClient(repository string, key ed25519.PublicKey, transport http.RoundTripper) *Client {
	release, err := url.Parse(repository + "/releases")
	if err != nil {
		panic(err)
	}
	return &Client{release: release, key: key, http: http.Client{Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || !allowedReleaseURL(release, req.URL) {
				return ErrNetwork
			}
			return nil
		}}}
}

func allowedReleaseURL(origin, address *url.URL) bool {
	if address.Scheme != "https" || address.User != nil {
		return false
	}
	if address.Host == origin.Host {
		return true
	}
	return origin.Host == "github.com" &&
		(address.Host == "release-assets.githubusercontent.com" || address.Host == "objects.githubusercontent.com")
}

// Check only downloads signed release metadata. Packages are fetched separately.
func (c *Client) Check(parent context.Context, current, target string) (*Candidate, error) {
	if _, err := ArtifactName(current, target); err != nil {
		return nil, errors.New("this build has no supported GitWave update target")
	}
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	body, _, err := c.get(ctx, c.release.JoinPath("latest", "download", ManifestName))
	if err != nil {
		return nil, err
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, MaxManifestSize+1))
	if err != nil {
		return nil, networkError(ctx)
	}
	manifest, err := VerifyManifest(data, c.key)
	if err != nil {
		return nil, err
	}
	if semver.Compare("v"+manifest.Version, "v"+current) <= 0 {
		return nil, nil
	}
	artifact, _ := ArtifactName(manifest.Version, target)
	return &Candidate{version: manifest.Version, notes: manifest.Notes, artifact: artifact,
		packageInfo: manifest.Packages[target], client: c}, nil
}

// Download stages a package in a new private directory. A partial or invalid
// download is removed, and a successful path belongs to the caller.
func (c *Candidate) Download(parent context.Context, dir string, progress func(received, total int64)) (string, error) {
	stage, err := os.MkdirTemp(dir, "gitwave-update-")
	if err != nil {
		return "", err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(stage)
		}
	}()
	path := filepath.Join(stage, c.artifact)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	defer file.Close()
	if err := c.downloadTo(parent, file, progress); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := parent.Err(); err != nil {
		return "", err
	}
	keep = true
	return path, nil
}

func (c *Candidate) downloadTo(parent context.Context, file *os.File, progress func(received, total int64)) error {
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	address := c.client.release.JoinPath("download", "v"+c.version, c.artifact)
	body, length, err := c.client.get(ctx, address)
	if err != nil {
		return err
	}
	defer body.Close()
	if length >= 0 && length != c.packageInfo.Size {
		return ErrPackage
	}
	digest := sha256.New()
	writer := &downloadProgress{out: io.MultiWriter(file, digest), report: progress, total: c.packageInfo.Size}
	if progress != nil {
		progress(0, writer.total)
	}
	count, err := io.Copy(writer, io.LimitReader(body, writer.total+1))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var pathError *os.PathError
		if errors.As(err, &pathError) {
			return err
		}
		return networkError(ctx)
	}
	if count != writer.total || hex.EncodeToString(digest.Sum(nil)) != c.packageInfo.SHA256 {
		return ErrPackage
	}
	if err := file.Sync(); err != nil {
		return err
	}
	return ctx.Err()
}

func (c *Client) get(ctx context.Context, address *url.URL) (io.ReadCloser, int64, error) {
	if !allowedReleaseURL(c.release, address) {
		return nil, 0, ErrNetwork
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address.String(), nil)
	if err != nil {
		return nil, 0, ErrNetwork
	}
	request.Header.Set("Accept-Encoding", "identity")
	response, err := c.http.Do(request)
	if err != nil {
		return nil, 0, networkError(ctx)
	}
	if response.StatusCode != http.StatusOK ||
		(response.Header.Get("Content-Encoding") != "" && !strings.EqualFold(response.Header.Get("Content-Encoding"), "identity")) {
		response.Body.Close()
		return nil, 0, fmt.Errorf("%w (HTTP %d)", ErrNetwork, response.StatusCode)
	}
	return response.Body, response.ContentLength, nil
}

func networkError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return ErrNetwork
}

type downloadProgress struct {
	out             io.Writer
	report          func(received, total int64)
	received, total int64
	last            time.Time
}

func (p *downloadProgress) Write(data []byte) (int, error) {
	n, err := p.out.Write(data)
	p.received += int64(n)
	if p.report != nil && (p.received >= p.total || time.Since(p.last) >= 100*time.Millisecond) {
		p.report(min(p.received, p.total), p.total)
		p.last = time.Now()
	}
	return n, err
}
