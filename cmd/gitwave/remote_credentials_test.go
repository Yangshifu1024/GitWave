package main

import (
	"testing"

	"gitwave.work/internal/gitcli"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestRemoteCredentialsSelectAccountsAndReadLegacyKeys(t *testing.T) {
	var vault remoteCredentials
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		secrets := cx.Secrets()
		secrets.Service = remoteSecretService
		vault = remoteCredentials{secrets: secrets}
		return core.Text{Content: "Test keyring"}
	})
	ctx := t.Context()
	const remote = "https://git.example.invalid:9443/project.git"
	const sibling = "https://git.example.invalid:9443/other.git"
	const legacy = "https://legacy.example.invalid:9443/project.git"
	check := func(endpoint, username, wantUser, wantPassword string, migrate bool) {
		t.Helper()
		credential, found, err := vault.Lookup(ctx, endpoint, username)
		if err != nil || found != (wantUser != "") || credential.Username != wantUser ||
			credential.Password != wantPassword || credential.Remember != migrate {
			t.Fatalf("unexpected credential result (found = %t, needs migration = %t): %v", found, credential.Remember, err)
		}
	}
	store := func(username, secret string) {
		t.Helper()
		if err := vault.Store(ctx, remote, gitcli.HTTPSCredential{Username: username, Password: secret}); err != nil {
			t.Fatal(err)
		}
	}
	store("first", "synthetic-first")
	store("second", "synthetic-second")
	check(remote, "", "second", "synthetic-second", false)
	check(remote, "first", "first", "synthetic-first", false)
	check(remote, "different", "", "", false)
	check(sibling, "", "", "", false)
	check("https://git.example.invalid/project.git", "second", "", "", false)
	for account, value := range map[string]string{
		"https/legacy.example.invalid:9443":          "old-host-user\nsynthetic-old-host",
		"https/legacy.example.invalid:9443\x1fknown": "known\nsynthetic-old-account",
	} {
		item := core.SecretItem{Service: remoteSecretService, Account: account}
		if err := app.Secrets.WriteSecret(item, value); err != nil {
			t.Fatal(err)
		}
	}
	check(legacy, "", "old-host-user", "synthetic-old-host", true)
	check(legacy, "known", "known", "synthetic-old-account", true)
	check(legacy, "different", "", "", false)
	check("https://legacy.example.invalid/project.git", "known", "", "", false)
	path, _, err := remoteCredentialScope(legacy)
	if err != nil {
		t.Fatal(err)
	}
	selected := core.SecretItem{Service: remoteSecretService, Account: remoteCredentialKey("selected", path)}
	if err := app.Secrets.WriteSecret(selected, "different"); err != nil {
		t.Fatal(err)
	}
	check(legacy, "", "", "", false)
}
