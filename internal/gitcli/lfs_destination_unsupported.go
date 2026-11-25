//go:build !windows && !unix

package gitcli

import "fmt"

func lfsDirectoryIdentity(string) (string, error) {
	return "", fmt.Errorf("local LFS push is unsupported on this platform")
}
