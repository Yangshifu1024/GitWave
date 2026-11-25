//go:build !darwin || ios

package main

import "errors"

func publishMacRelease(_, _, _ string) error {
	return errors.New("preparing the macOS update requires macOS")
}
