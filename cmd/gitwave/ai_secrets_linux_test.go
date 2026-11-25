package main

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/google/uuid"
)

func TestLinuxLegacyAISecretsWithSecretService(t *testing.T) {
	if os.Getenv("GITWAVE_TEST_LINUX_KEYRING") != "1" {
		t.Skip("requires an isolated D-Bus session and an unlocked Secret Service")
	}
	ctx := t.Context()
	bus, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { bus.Close() })
	root := bus.Object(linuxSecretsName, linuxSecretsRoot)
	var output dbus.Variant
	var session, collection dbus.ObjectPath
	if err := root.CallWithContext(ctx, linuxSecretService+".OpenSession", 0, "plain", dbus.MakeVariant("")).Store(&output, &session); err != nil {
		t.Fatal(err)
	}
	if err := root.CallWithContext(ctx, linuxSecretService+".ReadAlias", 0, "default").Store(&collection); err != nil {
		t.Fatal(err)
	}
	if collection == linuxNoSecretObject {
		t.Fatal("the fixture needs an unlocked default collection")
	}
	account := "gitwave-test-" + uuid.NewString()
	write := func(attributes map[string]string, secret string) {
		t.Helper()
		if attributes["service"] == "" {
			attributes["service"] = aiSecretService
		}
		properties := map[string]dbus.Variant{
			linuxSecretItem + ".Label":      dbus.MakeVariant("GitWave isolated credential test"),
			linuxSecretItem + ".Attributes": dbus.MakeVariant(attributes),
		}
		password := linuxSecretValue{Session: session, Value: []byte(secret), ContentType: "text/plain"}
		var item, prompt dbus.ObjectPath
		if err := bus.Object(linuxSecretsName, collection).CallWithContext(ctx, linuxSecretCollection+".CreateItem", 0,
			properties, password, false).Store(&item, &prompt); err != nil {
			t.Fatal(err)
		}
		if item == linuxNoSecretObject || prompt != linuxNoSecretObject {
			t.Fatal("Secret Service asked to unlock the test collection")
		}
		t.Cleanup(func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var unlocked, locked []dbus.ObjectPath
			if err := root.CallWithContext(cleanup, linuxSecretService+".SearchItems", 0, attributes).Store(&unlocked, &locked); err != nil {
				t.Errorf("find isolated test credential for cleanup: %v", err)
				return
			}
			if !slices.Contains(unlocked, item) && !slices.Contains(locked, item) {
				return
			}
			var prompt dbus.ObjectPath
			err := bus.Object(linuxSecretsName, item).CallWithContext(cleanup, linuxSecretItem+".Delete", 0).Store(&prompt)
			if err != nil || prompt != linuxNoSecretObject {
				t.Errorf("remove isolated test credential: %v", err)
			}
		})
	}
	check := func(want string, wantFound bool) {
		t.Helper()
		value, found, err := readLegacySecret(ctx, aiSecretService, account)
		if err != nil || found != wantFound || value != want {
			t.Fatalf("unexpected legacy lookup result (found %v): %v", found, err)
		}
	}
	clearAIKey := func() {
		t.Helper()
		if err := deleteLegacySecret(ctx, aiSecretService, account); err != nil {
			t.Fatal(err)
		}
	}
	write(map[string]string{"account": account, "xdg:schema": "org.freedesktop.Secret.Generic"}, "NSGo test key")
	write(map[string]string{"username": account, "target": "another-target", "application": "rust-keyring"}, "unrelated test key")
	check("", false)
	write(map[string]string{"username": account, "application": "rust-keyring"}, "targetless test key")
	check("targetless test key", true)
	write(map[string]string{"username": account, "target": "default", "application": "rust-keyring"}, "旧版 fixture 🔐")
	check("旧版 fixture 🔐", true)
	clearAIKey()
	check("", false)
	clearAIKey()
	var remaining []dbus.ObjectPath
	for _, attributes := range []map[string]string{{"account": account}, {"username": account, "target": "another-target"}} {
		attributes["service"] = aiSecretService
		if err := bus.Object(linuxSecretsName, collection).CallWithContext(ctx, linuxSecretCollection+".SearchItems", 0, attributes).Store(&remaining); err != nil || len(remaining) != 1 {
			t.Fatalf("legacy clear changed an unrelated item: %v", err)
		}
	}
	write(map[string]string{"username": account, "target": "default", "application": "rust-keyring"}, "primary test key")
	write(map[string]string{"username": account, "application": "older-client-a"}, "first fallback test key")
	write(map[string]string{"username": account, "application": "older-client-b"}, "second fallback test key")
	check("primary test key", true)
	if err := deleteLegacySecret(ctx, aiSecretService, account); !errors.Is(err, errLegacyAIKeyAmbiguous) {
		t.Fatalf("clear should refuse ambiguous fallback keys: %v", err)
	}
	check("primary test key", true)
	remoteAccount := "https/repo.example.invalid:9443\x1f" + account
	write(map[string]string{"service": remoteSecretService, "username": remoteAccount, "target": "default", "application": "rust-keyring"},
		"remote user\nfixture remote credential")
	if value, found, err := readLegacySecret(ctx, remoteSecretService, remoteAccount); err != nil || !found || value != "remote user\nfixture remote credential" {
		t.Fatalf("unexpected lookup for the legacy remote credential (found %v): %v", found, err)
	}
	if _, found, err := readLegacySecret(ctx, aiSecretService, remoteAccount); err != nil || found {
		t.Fatalf("the legacy remote credential was visible to the AI secret service (found %v): %v", found, err)
	}
}
