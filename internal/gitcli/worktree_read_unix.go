//go:build unix

package gitcli

import "golang.org/x/sys/unix"

const workFileOpenFlags = unix.O_NONBLOCK | unix.O_NOFOLLOW
