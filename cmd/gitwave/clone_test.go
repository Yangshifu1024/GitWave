package main

import (
	"crypto/sha256"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitwave.work/internal/workspace"

	"github.com/nsgo-dev/nsgo/core"
)

func TestNativeCloneAuthenticatesTheLFSAddressBeforeRegisteringTheRepo(t *testing.T) {
	if _, err := exec.LookPath("git-lfs"); err != nil {
		t.Skip("Git LFS CLI is unavailable on PATH")
	}
	source, git := historyRepository(t)
	git("", "config", "filter.lfs.process", "git-lfs filter-process")
	git("", "config", "filter.lfs.required", "true")
	const payload = "media obtained during the native clone\x00"
	const name = "asset.asset"
	oid := fmt.Sprintf("%x", sha256.Sum256([]byte(payload)))
	if err := os.WriteFile(filepath.Join(source, ".gitattributes"), []byte("*.asset filter=lfs diff=lfs merge=lfs -text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, name), []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	const gitUser, gitToken = "fixture-git-user", "fixture-git-token"
	const mediaUser, mediaToken = "fixture-lfs-user", "fixture-lfs-token"
	gitFiles := http.StripPrefix("/project.git/", http.FileServer(http.Dir(filepath.Join(source, ".git"))))
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, token, ok := r.BasicAuth()
		switch r.URL.Path {
		case "/media/objects/batch":
			if !ok || user != mediaUser || token != mediaToken {
				w.Header().Set("WWW-Authenticate", "Basic realm=\"LFS Fixture\"")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.git-lfs+json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"transfer": "basic", "objects": []any{map[string]any{
					"oid": oid, "size": len(payload), "authenticated": true,
					"actions": map[string]any{"download": map[string]any{"href": server.URL + "/object/" + oid}},
				}},
			})
		case "/object/" + oid:
			_, _ = io.WriteString(w, payload)
		default:
			if !ok || user != gitUser || token != gitToken {
				w.Header().Set("WWW-Authenticate", "Basic realm=\"Git Fixture\"")
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			gitFiles.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(server.Close)
	git("", "config", "--file", ".lfsconfig", "lfs.url", server.URL+"/media")
	git("", "add", "--", ".gitattributes", ".lfsconfig", name)
	git("", "commit", "-m", "Commit with the protected media")
	git("", "update-server-info")
	root := t.TempDir()
	cert := filepath.Join(root, "fixture.pem")
	if err := os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "gitconfig")
	git("", "config", "--file", config, "http.sslCAInfo", cert)
	git("", "config", "--file", config, "http.schannelUseSSLCAInfo", "true")
	git("", "config", "--file", config, "filter.lfs.process", "git-lfs filter-process")
	git("", "config", "--file", config, "filter.lfs.required", "true")
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	store, err := workspace.Open(t.Context(), filepath.Join(root, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.CreateWithRepos(t.Context(), "Clone media", nil); err != nil {
		t.Fatal(err)
	}
	app := workspaceApp(t, store)
	const timeout = 15 * time.Second
	dest := filepath.Join(root, "checkout")
	app.WaitFor("text:Clone media", timeout)
	app.Press("id:workspace-selector")
	app.ChoosePopup("Clone remote repo")
	app.WaitFor("id:clone-url", timeout)
	app.Type("id:clone-url", server.URL+"/project.git")
	app.Clear("id:clone-dest")
	app.Type("id:clone-dest", dest)
	app.Press("id:clone-confirm")
	app.WaitFor("id:clone-auth-username", timeout)
	app.Type("id:clone-auth-username", gitUser)
	app.Type("id:clone-auth-password", gitToken)
	app.Press("id:clone-auth-remember")
	app.Press("id:clone-confirm")
	app.WaitFor("text:/Git LFS at.*media.*requires authentication/", timeout)
	if state, err := store.Load(t.Context()); err != nil || len(state.Repos) != 0 {
		t.Fatalf("the workspace registered the clone before media authentication: %+v, %v", state, err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("clone was published before receiving LFS credentials: %v", err)
	}
	app.Type("id:clone-auth-username", mediaUser)
	app.Type("id:clone-auth-password", mediaToken)
	app.Press("id:clone-auth-remember")
	app.Press("id:clone-confirm")
	app.WaitForGone("id:clone-dialog", timeout)
	app.WaitFor("label:/Commit with the protected media/", timeout)
	if bytes, err := os.ReadFile(filepath.Join(dest, name)); err != nil || string(bytes) != payload {
		t.Fatalf("the clone registered a checkout without its LFS media: %q, %v", bytes, err)
	}
}

func TestNativeWorkspaceClonesAuthenticatedRemoteAndRetainsDestination(t *testing.T) {
	source, git := historyRepository(t)
	git("", "commit", "--allow-empty", "-m", "Commit from the authenticated remote")
	git("", "update-server-info")
	const username, token = "fixture-user", "fixture-token"
	var usedAtSibling atomic.Bool
	files := http.StripPrefix("/project.git/", http.FileServer(http.Dir(filepath.Join(source, ".git"))))
	siblingFiles := http.StripPrefix("/sibling.git/", http.FileServer(http.Dir(filepath.Join(source, ".git"))))
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/sibling.git/") && r.Header.Get("Authorization") != "" {
			usedAtSibling.Store(true)
		}
		if user, pass, ok := r.BasicAuth(); !ok || user != username || pass != token {
			w.Header().Set("WWW-Authenticate", "Basic realm=\"GitWave Test\"")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/sibling.git/") {
			siblingFiles.ServeHTTP(w, r)
		} else {
			files.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(server.Close)
	root := t.TempDir()
	cert := filepath.Join(root, "fixture.pem")
	if err := os.WriteFile(cert, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "gitconfig")
	git("", "config", "--file", config, "http.sslCAInfo", cert)
	git("", "config", "--file", config, "http.schannelUseSSLCAInfo", "true")
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	dest := filepath.Join(root, "Clone destination")
	if err := os.Mkdir(dest, 0o700); err != nil {
		t.Fatal(err)
	}
	prior := filepath.Join(dest, "old file.txt")
	if err := os.WriteFile(prior, []byte("previous local contents"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := workspace.Open(t.Context(), filepath.Join(root, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.CreateWithRepos(t.Context(), "Clone project", nil); err != nil {
		t.Fatal(err)
	}
	app := workspaceApp(t, store)
	const timeout = 10 * time.Second
	app.WaitFor("text:Clone project", timeout)
	app.Press("id:workspace-selector")
	app.ChoosePopup("Clone remote repo")
	app.WaitFor("id:clone-url", timeout)
	app.Type("id:clone-url", server.URL+"/project.git")
	app.Clear("id:clone-dest")
	app.Type("id:clone-dest", dest)
	app.Press("id:clone-confirm")
	app.WaitFor("label:Replace and clone", timeout)
	app.Press("id:clone-confirm")
	app.WaitFor("id:clone-auth-username", timeout)
	app.Press("label:Cancel")
	app.WaitFor("text:Clone cancelled", timeout)
	if contents, err := os.ReadFile(prior); err != nil || string(contents) != "previous local contents" {
		t.Fatalf("declining credentials changed the destination: %q, %v", contents, err)
	}

	app.Press("id:clone-confirm")
	app.WaitFor("label:Replace and clone", timeout)
	app.Press("id:clone-confirm")
	app.WaitFor("id:clone-auth-username", timeout)
	app.Type("id:clone-auth-username", username)
	app.Type("id:clone-auth-password", token)
	app.Press("id:clone-auth-remember")
	app.Press("id:clone-confirm")
	app.WaitForGone("id:clone-dialog", timeout)
	app.WaitFor("label:/Commit from the authenticated remote/", timeout)
	state, err := store.Load(t.Context())
	if err != nil || len(state.Repos) != 1 || filepath.Base(state.Repo.Path) != filepath.Base(dest) {
		t.Fatalf("cloned repository did not activate: %+v, %v", state, err)
	}
	git("", "commit", "--allow-empty", "-m", "Fetched from the authenticated remote")
	git("", "update-server-info")
	app.Press("id:fetch")
	app.WaitFor("id:fetch-auth-username", timeout)
	app.Type("id:fetch-auth-username", username)
	app.Type("id:fetch-auth-password", token)
	app.Press("label:Save and retry")
	app.WaitForGone("id:fetch-auth-dialog", timeout)
	app.WaitFor("label:/Fetched from the authenticated remote/", timeout)
	path, _, err := remoteCredentialScope(server.URL + "/project.git")
	if err != nil {
		t.Fatal(err)
	}
	item := core.SecretItem{Service: remoteSecretService, Account: remoteCredentialKey("account", path+"\n"+username)}
	if value, found, err := app.Secrets.ReadSecret(item); err != nil || !found || value != username+"\n"+token {
		t.Fatalf("the accepted credential did not reach the system keyring: found = %v, err = %v", found, err)
	}
	app.Press("id:pull")
	app.WaitFor("id:pull-dialog", timeout)
	app.WaitUntil(timeout, func() bool { return app.Enabled("id:pull-confirm") })
	app.Press("id:pull-confirm")
	app.WaitFor("text:Pulled from origin", timeout)
	git("", "-C", dest, "remote", "set-url", "origin", server.URL+"/sibling.git")
	app.Press("id:fetch")
	app.WaitFor("id:fetch-auth-username", timeout)
	app.WaitFor(`id:fetch-auth-dialog >> text:/sibling\.git/`, timeout)
	if usedAtSibling.Load() {
		t.Fatal("the stored credential was sent to a different repository path")
	}
	app.Type("id:fetch-auth-username", username)
	app.Press("label:Save and retry")
	app.WaitFor("text:/The saved credential for this account is unavailable/", timeout)
	if app.Find("id:fetch-auth-username").Text != username || usedAtSibling.Load() {
		t.Fatal("the unrecognized account changed the authentication state or contacted the remote")
	}
	_, legacyHost, err := remoteCredentialScope(server.URL + "/sibling.git")
	if err != nil {
		t.Fatal(err)
	}
	legacy := core.SecretItem{Service: remoteSecretService, Account: "https/" + legacyHost + "\x1f" + username}
	if err := app.Secrets.WriteSecret(legacy, username+"\n"+token); err != nil {
		t.Fatal(err)
	}
	app.Press("label:Save and retry")
	app.WaitForGone("id:fetch-auth-dialog", timeout)
	app.WaitFor("text:Fetched from origin", timeout)
	if !usedAtSibling.Load() {
		t.Fatal("selecting the legacy account did not authenticate the new remote")
	}
	app.Press("id:workspace-selector")
	app.ChoosePopup("Clone remote repo")
	app.WaitFor("id:clone-url", timeout)
	app.Type("id:clone-url", server.URL+"/project.git")
	app.Clear("id:clone-dest")
	app.Type("id:clone-dest", dest)
	app.Press("id:clone-confirm")
	app.WaitFor("text:The destination contains a repository registered in a workspace", timeout)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".gitwave-original-") {
			continue
		}
		if contents, err := os.ReadFile(filepath.Join(root, entry.Name(), filepath.Base(prior))); err != nil || string(contents) != "previous local contents" {
			t.Fatalf("confirmed clone lost the original directory: %q, %v", contents, err)
		}
		return
	}
	t.Fatal("confirmed clone left no backup of the original directory")
}
