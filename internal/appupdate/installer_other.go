//go:build !linux && !windows && (!darwin || ios)

package appupdate

func InstallerForRunningApp() Installer { return nil }

func WaitForUpdateParent() error { return nil }
