package main

import (
	"context"
	"errors"

	"github.com/nsgo-dev/nsgo/core"
)

var errLegacyAIKeyAmbiguous = errors.New("multiple matching legacy keys in the system keyring")

func readAISecret(ctx context.Context, secrets core.Secrets, account string) (string, bool, error) {
	return readKeyringSecret(ctx, secrets, account)
}

func readKeyringSecret(ctx context.Context, secrets core.Secrets, account string) (string, bool, error) {
	value, found, err := secrets.Get(account)
	if err != nil || found {
		return value, found, err
	}
	return readLegacySecret(ctx, secrets.Service, account)
}

func deleteAISecret(ctx context.Context, secrets core.Secrets, account string) error {
	if err := deleteLegacySecret(ctx, secrets.Service, account); err != nil {
		return err
	}
	return secrets.Delete(account)
}
