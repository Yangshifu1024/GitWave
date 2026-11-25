package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/godbus/dbus/v5"
)

const (
	linuxSecretsName       = "org.freedesktop.secrets"
	linuxSecretsRoot       = "/org/freedesktop/secrets"
	linuxSecretService     = "org.freedesktop.Secret.Service"
	linuxSecretCollection  = "org.freedesktop.Secret.Collection"
	linuxSecretItem        = "org.freedesktop.Secret.Item"
	linuxSecretSession     = "org.freedesktop.Secret.Session"
	linuxSecretPrompt      = "org.freedesktop.Secret.Prompt"
	linuxSecretPropertyGet = "org.freedesktop.DBus.Properties.Get"
	linuxNoSecretObject    = "/"
)

type linuxSecretValue struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

type linuxLegacyItem struct {
	path       dbus.ObjectPath
	locked     bool
	targetless bool
}

func readLegacySecret(ctx context.Context, namespace, account string) (string, bool, error) {
	var value string
	found, err := withLinuxLegacyItems(ctx, namespace, account, false, func(ctx context.Context, bus *dbus.Conn, item dbus.BusObject) error {
		var out dbus.Variant
		var session dbus.ObjectPath
		service := bus.Object(linuxSecretsName, linuxSecretsRoot)
		if err := service.CallWithContext(ctx, linuxSecretService+".OpenSession", 0, "plain", dbus.MakeVariant("")).Store(&out, &session); err != nil {
			return err
		}
		if session == linuxNoSecretObject {
			return errors.New("Secret Service returned no session")
		}
		defer bus.Object(linuxSecretsName, session).CallWithContext(ctx, linuxSecretSession+".Close", 0)
		var password linuxSecretValue
		if err := item.CallWithContext(ctx, linuxSecretItem+".GetSecret", 0, session).Store(&password); err != nil {
			return err
		}
		defer clear(password.Value)
		if password.Session != session || len(password.Parameters) != 0 || !utf8.Valid(password.Value) {
			return errors.New("legacy key has invalid encoding")
		}
		value = string(password.Value)
		return nil
	})
	return value, found, err
}

func deleteLegacySecret(ctx context.Context, namespace, account string) error {
	_, err := withLinuxLegacyItems(ctx, namespace, account, true, func(ctx context.Context, bus *dbus.Conn, item dbus.BusObject) error {
		var prompt dbus.ObjectPath
		if err := item.CallWithContext(ctx, linuxSecretItem+".Delete", 0).Store(&prompt); err != nil {
			return err
		}
		return finishLinuxSecretPrompt(ctx, bus, prompt)
	})
	return err
}

func withLinuxLegacyItems(ctx context.Context, namespace, account string, clearing bool, act func(context.Context, *dbus.Conn, dbus.BusObject) error) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	address := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if address == "" || address == "autolaunch:" {
		runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
		if runtimeDir == "" {
			runtimeDir = filepath.Join("/run/user", strconv.Itoa(os.Getuid()))
		}
		socket := filepath.Join(runtimeDir, "bus")
		info, err := os.Stat(socket)
		if err != nil || info.Mode()&os.ModeSocket == 0 {
			return false, nil
		}
		address = "unix:path=" + dbus.EscapeBusAddressValue(socket)
	}
	bus, err := dbus.Connect(address, dbus.WithContext(ctx))
	if err != nil {
		return false, fmt.Errorf("connect to the Secret Service session bus: %w", err)
	}
	defer bus.Close()
	items, err := findLinuxLegacyItems(ctx, bus, namespace, account, clearing)
	if err != nil || len(items) == 0 {
		return false, err
	}
	var locked []dbus.ObjectPath
	for _, item := range items {
		if item.locked {
			locked = append(locked, item.path)
		}
	}
	if len(locked) != 0 {
		service := bus.Object(linuxSecretsName, linuxSecretsRoot)
		var unlocked []dbus.ObjectPath
		var prompt dbus.ObjectPath
		if err := service.CallWithContext(ctx, linuxSecretService+".Unlock", 0, locked).Store(&unlocked, &prompt); err != nil {
			return false, err
		}
		if err := finishLinuxSecretPrompt(ctx, bus, prompt); err != nil {
			return false, err
		}
	}
	// Unlock may succeed for only some items. Verify both forms before removing either.
	if clearing {
		for _, candidate := range items {
			item := bus.Object(linuxSecretsName, candidate.path)
			attributes, err := linuxSecretAttributes(ctx, item)
			if err != nil {
				return false, err
			}
			target, hasTarget := attributes["target"]
			if attributes["service"] != namespace || attributes["username"] != account ||
				candidate.targetless == hasTarget || (hasTarget && target != "default") {
				return false, errors.New("the legacy key changed before it could be removed")
			}
			if locked, err := linuxSecretLocked(ctx, item); err != nil || locked {
				if err == nil {
					err = errors.New("the legacy key is still locked")
				}
				return false, err
			}
		}
	}
	for _, item := range items {
		if err := act(ctx, bus, bus.Object(linuxSecretsName, item.path)); err != nil {
			return false, err
		}
	}
	return true, nil
}

func findLinuxLegacyItems(ctx context.Context, bus *dbus.Conn, namespace, account string, clearing bool) ([]linuxLegacyItem, error) {
	base := bus.Object(linuxSecretsName, linuxSecretsRoot)
	attributes := map[string]string{"service": namespace, "username": account, "target": "default"}
	var unlocked, locked []dbus.ObjectPath
	if err := base.CallWithContext(ctx, linuxSecretService+".SearchItems", 0, attributes).Store(&unlocked, &locked); err != nil {
		var busErr dbus.Error
		if errors.As(err, &busErr) && (busErr.Name == "org.freedesktop.DBus.Error.ServiceUnknown" || busErr.Name == "org.freedesktop.DBus.Error.NameHasNoOwner") {
			return nil, nil
		}
		return nil, err
	}
	if len(unlocked)+len(locked) > 1 {
		return nil, errLegacyAIKeyAmbiguous
	}
	var primary linuxLegacyItem
	if len(unlocked) == 1 {
		primary.path = unlocked[0]
	}
	if len(locked) == 1 {
		primary = linuxLegacyItem{path: locked[0], locked: true}
	}
	if primary.path != "" && !clearing {
		return []linuxLegacyItem{primary}, nil
	}
	older, err := findLinuxTargetlessItem(ctx, bus, namespace, account)
	if err != nil {
		return nil, err
	}
	var items []linuxLegacyItem
	if older.path != "" {
		items = append(items, older)
	}
	if primary.path != "" {
		items = append(items, primary)
	}
	return items, nil
}

func findLinuxTargetlessItem(ctx context.Context, bus *dbus.Conn, namespace, account string) (linuxLegacyItem, error) {
	// Keyring's oldest items omitted target and live only in the default collection.
	base := bus.Object(linuxSecretsName, linuxSecretsRoot)
	var collection dbus.ObjectPath
	if err := base.CallWithContext(ctx, linuxSecretService+".ReadAlias", 0, "default").Store(&collection); err != nil || collection == linuxNoSecretObject {
		return linuxLegacyItem{}, err
	}
	var matches []dbus.ObjectPath
	attributes := map[string]string{"service": namespace, "username": account}
	if err := bus.Object(linuxSecretsName, collection).CallWithContext(ctx, linuxSecretCollection+".SearchItems", 0, attributes).Store(&matches); err != nil {
		return linuxLegacyItem{}, err
	}
	var match dbus.ObjectPath
	for _, path := range matches {
		values, err := linuxSecretAttributes(ctx, bus.Object(linuxSecretsName, path))
		if err != nil {
			return linuxLegacyItem{}, err
		}
		if _, hasTarget := values["target"]; hasTarget {
			continue
		}
		if match != "" {
			return linuxLegacyItem{}, errLegacyAIKeyAmbiguous
		}
		match = path
	}
	if match == "" {
		return linuxLegacyItem{}, nil
	}
	isLocked, err := linuxSecretLocked(ctx, bus.Object(linuxSecretsName, match))
	return linuxLegacyItem{path: match, locked: isLocked, targetless: true}, err
}

func linuxSecretAttributes(ctx context.Context, item dbus.BusObject) (map[string]string, error) {
	var property dbus.Variant
	if err := item.CallWithContext(ctx, linuxSecretPropertyGet, 0, linuxSecretItem, "Attributes").Store(&property); err != nil {
		return nil, err
	}
	attributes, ok := property.Value().(map[string]string)
	if !ok {
		return nil, errors.New("Secret Service returned invalid item attributes")
	}
	return attributes, nil
}

func linuxSecretLocked(ctx context.Context, item dbus.BusObject) (bool, error) {
	var property dbus.Variant
	if err := item.CallWithContext(ctx, linuxSecretPropertyGet, 0, linuxSecretItem, "Locked").Store(&property); err != nil {
		return false, err
	}
	isLocked, ok := property.Value().(bool)
	if !ok {
		return false, errors.New("Secret Service returned an invalid lock state")
	}
	return isLocked, nil
}

func finishLinuxSecretPrompt(ctx context.Context, bus *dbus.Conn, path dbus.ObjectPath) error {
	if path == linuxNoSecretObject {
		return nil
	}
	signals := make(chan *dbus.Signal, 1)
	bus.Signal(signals)
	defer bus.RemoveSignal(signals)
	if err := bus.AddMatchSignalContext(ctx, dbus.WithMatchSender(linuxSecretsName), dbus.WithMatchObjectPath(path),
		dbus.WithMatchInterface(linuxSecretPrompt), dbus.WithMatchMember("Completed")); err != nil {
		return err
	}
	if err := bus.Object(linuxSecretsName, path).CallWithContext(ctx, linuxSecretPrompt+".Prompt", 0, "").Err; err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case signal, open := <-signals:
			if !open {
				return errors.New("Secret Service disconnected while waiting for confirmation")
			}
			if signal.Path != path || signal.Name != linuxSecretPrompt+".Completed" {
				continue
			}
			if len(signal.Body) != 2 {
				return errors.New("Secret Service returned an invalid confirmation")
			}
			dismissed, ok := signal.Body[0].(bool)
			if !ok || dismissed {
				return errors.New("Secret Service did not confirm the request")
			}
			return nil
		}
	}
}
