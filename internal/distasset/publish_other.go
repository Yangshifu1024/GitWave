//go:build !darwin && !linux && !windows

package distasset

import "errors"

func PublishDirectory(_, _ string) error {
	return errors.New("publishing a release directory requires macOS, Linux, or Windows")
}
