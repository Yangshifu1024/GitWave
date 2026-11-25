package gitcli

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gitwave.work/internal/process"
	"gitwave.work/internal/proxy"
)

const (
	credentialHelperArg = "--gitwave-credential-helper"
	credentialBrokerURL = "GITWAVE_CREDENTIAL_URL"
	credentialBrokerKey = "GITWAVE_CREDENTIAL_KEY"
)

// CredentialHelper handles Git's helper subcommand before the GUI starts.
func CredentialHelper(args []string, stdin io.Reader, stdout io.Writer) bool {
	if len(args) == 0 || args[0] != credentialHelperArg {
		return false
	}
	if len(args) != 2 || args[1] != "get" {
		return true
	}
	endpoint, err := url.Parse(os.Getenv(credentialBrokerURL))
	key := os.Getenv(credentialBrokerKey)
	if err != nil || endpoint.Scheme != "http" || endpoint.Hostname() != "127.0.0.1" || endpoint.Port() == "" ||
		endpoint.User != nil || len(key) < 26 {
		return true
	}
	client := &http.Client{Timeout: 4 * time.Second, Transport: &http.Transport{},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	// Consume one credential record, which Git terminates with a blank line.
	scanner := bufio.NewScanner(io.LimitReader(stdin, 16<<10))
	var record strings.Builder
	for scanner.Scan() && scanner.Text() != "" {
		record.WriteString(scanner.Text())
		record.WriteByte('\n')
	}
	req, err := http.NewRequest(http.MethodPost, endpoint.String(), strings.NewReader(record.String()))
	if err != nil {
		return true
	}
	req.Header.Set("Authorization", "Bearer "+key)
	response, err := client.Do(req)
	if err == nil {
		defer response.Body.Close()
		if response.StatusCode == http.StatusOK {
			_, _ = io.Copy(stdout, io.LimitReader(response.Body, 16<<10))
		}
	}
	return true
}

type HTTPSCredential struct {
	Username string
	Password string
	Remember bool
}

type credentialBroker struct {
	server    *http.Server
	address   string
	key       string
	endpoints []string
	used      sync.Map
}

func newCredentialBroker(remote string, auth HTTPSCredential, additionalURLs ...string) (*credentialBroker, error) {
	address, err := url.Parse(remote)
	if err != nil || address.Scheme != "https" {
		return nil, fmt.Errorf("inline credentials require an HTTPS URL")
	}
	if auth.Username == "" || auth.Password == "" || strings.ContainsAny(auth.Username+auth.Password+address.Path, "\x00\r\n") ||
		len(auth.Username)+len(auth.Password) > 8<<10 {
		return nil, fmt.Errorf("enter a valid HTTPS username and password or token")
	}
	paths := map[string]string{strings.Trim(address.Path, "/"): remote}
	endpoints := []string{remote}
	for _, extra := range additionalURLs {
		u, err := url.Parse(extra)
		if err == nil && u.Scheme == "https" && strings.EqualFold(u.Host, address.Host) && u.User == nil {
			paths[strings.Trim(u.Path, "/")] = extra
			endpoints = append(endpoints, extra)
		}
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	b := &credentialBroker{address: "http://" + listener.Addr().String() + "/credential", key: rand.Text(), endpoints: endpoints}
	b.server = &http.Server{
		ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, MaxHeaderBytes: 4096,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || r.URL.Path != "/credential" ||
				subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+b.key)) != 1 {
				http.NotFound(w, r)
				return
			}
			fields := make(map[string]string)
			scanner := bufio.NewScanner(http.MaxBytesReader(w, r.Body, 16<<10))
			for scanner.Scan() && scanner.Text() != "" {
				name, value, ok := strings.Cut(scanner.Text(), "=")
				if ok {
					fields[name] = value
				}
			}
			requested := paths[strings.Trim(fields["path"], "/")]
			if scanner.Err() != nil || fields["protocol"] != "https" ||
				!strings.EqualFold(fields["host"], address.Host) ||
				requested == "" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			b.used.Store(requested, struct{}{})
			_, _ = fmt.Fprintf(w, "username=%s\npassword=%s\n\n", auth.Username, auth.Password)
		}),
	}
	go func() { _ = b.server.Serve(listener) }()
	return b, nil
}

func (b *credentialBroker) commandConfig() ([]string, []string, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, nil, err
	}
	// Git executes shell-form helpers via its own POSIX shell, including
	// Git for Windows. Quote the executable as one literal argument.
	command := "!'" + strings.ReplaceAll(filepath.ToSlash(exe), "'", "'\\''") + "' " + credentialHelperArg
	options := []string{"-c", "credential.helper=", "-c", "credential.helper=" + command,
		"-c", "credential.useHttpPath=true", "-c", "http.followRedirects=false"}
	for _, endpoint := range b.endpoints {
		options = append(options, "-c", "http."+endpoint+".followRedirects=false")
	}
	return options, []string{credentialBrokerURL + "=" + b.address, credentialBrokerKey + "=" + b.key}, nil
}

func rememberHTTPSCredential(ctx context.Context, git, dir, remote string, auth HTTPSCredential) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	input := fmt.Sprintf("url=%s\nusername=%s\npassword=%s\n\n", remote, auth.Username, auth.Password)
	configRoot := filepath.Join(dir, "credential-config")
	if _, err := readGit(ctx, git, dir, nil, "", 8192, "init", "--quiet", "--bare", "--template=", "--", configRoot); err != nil {
		return fmt.Errorf("could not open an isolated directory for the Git credential helper: %w", err)
	}
	options := []string{"--git-dir=" + configRoot, "-c", "credential.useHttpPath=true"}
	cmd := gitCommand(ctx, git, dir, options, input, "credential", "approve")
	cmd.Env = proxy.CommandEnvironment(ctx, cmd.Env)
	if err := process.Run(cmd); err != nil {
		return fmt.Errorf("the Git credential helper could not save the credentials: %w", err)
	}
	// Git can exit successfully even when no helper accepted the store.
	cmd = gitCommand(ctx, git, dir, options, fmt.Sprintf("url=%s\nusername=%s\n\n", remote, auth.Username), "credential", "fill")
	cmd.Env = append(proxy.CommandEnvironment(ctx, cmd.Env), "GCM_INTERACTIVE=Never")
	var filled gitDiagnostic
	cmd.Stdout = &filled
	if err := process.Run(cmd); err != nil {
		return fmt.Errorf("the Git credential helper could not verify the saved credentials")
	}
	fields := make(map[string]string)
	for line := range strings.SplitSeq(filled.buffer.String(), "\n") {
		name, value, ok := strings.Cut(line, "=")
		if ok {
			fields[name] = value
		}
	}
	if fields["username"] != auth.Username || fields["password"] != auth.Password {
		return fmt.Errorf("the Git credential helper could not verify the saved credentials")
	}
	return nil
}
