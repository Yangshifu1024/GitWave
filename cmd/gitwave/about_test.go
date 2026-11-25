package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

func TestNativeAboutShowsBuildAndDataLocation(t *testing.T) {
	store := settingsStore(t)
	app, _ := refreshApp(t, store, "")
	const timeout = 10 * time.Second
	app.WaitFor("label:/^Settings test repository,/", timeout)
	app.ChooseMenu("GitWave", "About GitWave")
	app.WaitFor("id:about-dialog", timeout)
	app.WaitFor("id:about-app-icon", timeout)
	if version := app.Find("id:about-version").Text; !strings.HasPrefix(version, "v"+appVersion()) || appVersion() == "?.?.?" {
		t.Fatalf("About version = %q", version)
	}
	if os.Getenv("NSGO_TEST_TARGET") == "" {
		// Exercise the platform requests in the model without opening apps.
		app.OpenFile = func(file core.SharedFile) bool { return file.Path == store.DataDirectory() }
		app.Press("id:about-app-data")
		if len(app.OpenedFiles) != 1 || app.OpenedFiles[0].Path != store.DataDirectory() {
			t.Fatalf("opened files: %+v", app.OpenedFiles)
		}
		app.OpenURL = func(string) bool { return false }
		app.Press("id:about-github")
		app.WaitFor("id:about-error", timeout)
		if len(app.OpenedURLs) != 1 || app.OpenedURLs[0] != projectURL {
			t.Fatalf("opened URLs: %+v", app.OpenedURLs)
		}
	}
	app.Press("label:Close")
	app.WaitForGone("id:about-dialog", timeout)
	app.ChooseMenu("GitWave", "About GitWave")
	app.WaitFor("id:about-dialog", timeout)
	app.WaitForGone("id:about-error", timeout)
}
