//go:build !linux

package desktopgtk

func Probe() {}

func Prepare() error { return nil }
