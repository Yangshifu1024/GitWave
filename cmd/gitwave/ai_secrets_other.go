//go:build !windows && !linux

package main

import "context"

func readLegacySecret(context.Context, string, string) (string, bool, error) {
	return "", false, nil
}

func deleteLegacySecret(context.Context, string, string) error {
	return nil
}
