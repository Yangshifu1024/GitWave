package main

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"gitwave.work/internal/appupdate"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestNativeUpdateMenuUsesImportedPreferenceAndOffersARelease(t *testing.T) {
	store := settingsStore(t)
	db, err := sql.Open("sqlite", filepath.Join(store.DataDirectory(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(t.Context(), "INSERT INTO app_settings VALUES (?, ?, 0)",
		"legacy.webview-preferences.v1", `{"version":1,"values":{"gitwave-auto-update":"false"}}`); err != nil {
		t.Fatal(err)
	}
	if err := store.ImportLegacyPreferences(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	const timeout = 10 * time.Second
	var checks atomic.Int32
	var startupPreferenceChecked atomic.Bool
	allowReply := make(chan struct{})
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.connectWorkspace(store, nil)
		w.checkRelease = func(ctx context.Context) (updateOffer, error) {
			checks.Add(1)
			select {
			case <-ctx.Done():
				return updateOffer{}, ctx.Err()
			case <-allowReply:
				return updateOffer{version: "0.12.0", notes: "Release note fixture", page: projectURL + "/releases/tag/v0.12.0"}, nil
			}
		}
		cx.AfterFunc(10*time.Millisecond, func() {
			w.checkForAppUpdate(true)
			startupPreferenceChecked.Store(true)
		})
		return w.view()
	}, nsgotest.Size(1200, 800), nsgotest.Environment(refreshEnvironment(core.LifecycleActive)))
	app.Advance(20 * time.Millisecond)
	app.WaitUntil(timeout, startupPreferenceChecked.Load)
	if checks.Load() != 0 {
		t.Fatal("the imported preference did not suppress an automatic check")
	}
	app.WaitFor("label:/^Settings test repository,/", timeout)
	app.ChooseMenu("GitWave", "Check for Updates…")
	app.WaitFor("id:settings-dialog", timeout)
	app.WaitFor("text:/Checking…/", timeout)
	if app.Find("id:settings-auto-update").Props.Checked {
		t.Fatal("startup updates are enabled after importing false")
	}
	close(allowReply)
	app.WaitFor("id:app-update-dialog", timeout)
	app.WaitFor("text:Release note fixture", timeout)
	app.Press("label:Later")
	app.WaitForGone("id:app-update-dialog", timeout)
	app.WaitFor("id:settings-view-update", timeout)
	app.Press("id:settings-view-update")
	app.WaitFor("id:app-update-dialog", timeout)
	app.Key("escape")
	app.WaitForGone("id:app-update-dialog", timeout)
	app.Press("id:settings-auto-update")
	app.WaitUntil(timeout, func() bool {
		enabled, err := store.AutoUpdate(t.Context())
		return err == nil && enabled
	})
	if checks.Load() != 1 {
		t.Fatalf("checking once fetched %d manifests", checks.Load())
	}
}

func TestNativeUpdateOptOutCancelsTheStartupCheck(t *testing.T) {
	store := settingsStore(t)
	const timeout = 10 * time.Second
	var checking, canceled atomic.Bool
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.connectWorkspace(store, nil)
		w.checkRelease = func(ctx context.Context) (updateOffer, error) {
			checking.Store(true)
			<-ctx.Done()
			canceled.Store(true)
			return updateOffer{version: "0.12.0"}, nil
		}
		return w.view()
	}, nsgotest.Size(1200, 800), nsgotest.Environment(refreshEnvironment(core.LifecycleActive)))
	app.WaitFor("label:/^Settings test repository,/", timeout)
	app.Advance(startupUpdateDelay + 100*time.Millisecond)
	app.WaitUntil(timeout, checking.Load)
	app.Key("mod+,")
	app.WaitFor("id:settings-dialog", timeout)
	app.Press("id:settings-auto-update")
	app.WaitUntil(timeout, canceled.Load)
	app.WaitUntil(timeout, func() bool {
		enabled, err := store.AutoUpdate(t.Context())
		return err == nil && !enabled
	})
	app.WaitForGone("id:app-update-dialog", timeout)
}

func TestNativeUpdateCanHideDownloadAndRetryRestart(t *testing.T) {
	store := settingsStore(t)
	if err := store.SaveAutoUpdate(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	const timeout = 10 * time.Second
	finishDownload := make(chan struct{})
	var downloads, restarts atomic.Int32
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.connectWorkspace(store, nil)
		w.checkRelease = func(context.Context) (updateOffer, error) {
			return updateOffer{version: "0.12.0"}, nil
		}
		w.installUpdate = func(ctx context.Context, _ updateOffer, progress func(int64, int64)) (appupdate.Restart, error) {
			if downloads.Add(1) == 1 {
				return nil, errors.New("temporary download error")
			}
			progress(25, 100)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-finishDownload:
			}
			progress(100, 100)
			return func(context.Context) (func(bool) error, error) {
				if restarts.Add(1) == 1 {
					return nil, errors.New("fixture launch error")
				}
				return func(quit bool) error {
					if !quit {
						return nil
					}
					return errors.New("fixture restart could not be committed")
				}, nil
			}, nil
		}
		return w.view()
	}, nsgotest.Size(1200, 800), nsgotest.Environment(refreshEnvironment(core.LifecycleActive)))
	app.WaitFor("label:/^Settings test repository,/", timeout)
	app.ChooseMenu("GitWave", "Check for Updates…")
	app.WaitFor("id:app-update-dialog", timeout)
	app.Press("label:Download & Install")
	app.WaitFor("text:/temporary download error/", timeout)
	app.Press("label:Try Again")
	app.WaitFor("text:25 B / 100 B", timeout)
	app.Press("label:Hide")
	app.WaitForGone("id:app-update-dialog", timeout)
	close(finishDownload)
	app.WaitFor("label:Update installed", timeout)
	app.Press("label:Restart Now")
	app.WaitFor("text:/fixture launch error/", timeout)
	app.Press("label:Restart Now")
	app.WaitFor("text:/fixture restart could not be committed/", timeout)
	app.WaitFor("label:Restart Now", timeout)
	if restarts.Load() != 2 {
		t.Fatalf("restart attempts = %d", restarts.Load())
	}
	if downloads.Load() != 2 {
		t.Fatalf("restart retried installation, downloads = %d", downloads.Load())
	}
}

func TestNativeUpdateCanStartTheWindowsInstallerAfterDownload(t *testing.T) {
	store := settingsStore(t)
	if err := store.SaveAutoUpdate(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	const timeout = 10 * time.Second
	var attempts atomic.Int32
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.connectWorkspace(store, nil)
		w.installUpdateOnExit = true
		w.checkRelease = func(context.Context) (updateOffer, error) {
			return updateOffer{version: "0.12.0"}, nil
		}
		w.installUpdate = func(context.Context, updateOffer, func(int64, int64)) (appupdate.Restart, error) {
			return func(context.Context) (func(bool) error, error) {
				attempts.Add(1)
				return nil, errors.New("fixture installer unavailable")
			}, nil
		}
		return w.view()
	}, nsgotest.Size(1200, 800), nsgotest.Environment(refreshEnvironment(core.LifecycleActive)))
	app.WaitFor("label:/^Settings test repository,/", timeout)
	app.ChooseMenu("GitWave", "Check for Updates…")
	app.WaitFor("id:app-update-dialog", timeout)
	app.Press("label:Download & Install")
	app.WaitFor("text:/fixture installer unavailable/", timeout)
	app.WaitFor("label:Update ready to install", timeout)
	if attempts.Load() != 1 {
		t.Fatalf("installer startup attempts = %d", attempts.Load())
	}
}

func TestNativeUpdateRestartWaitsForWritesInOtherRepositories(t *testing.T) {
	const timeout = 10 * time.Second
	var attempts atomic.Int32
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		w := newWorkbench(cx)
		w.changesWrites.Set(map[changesWriteKey]changesWrite{{path: "/other-repository"}: {pending: true}})
		w.updateState.Set(updateState{phase: updateReady, offer: updateOffer{version: "0.12.0"}})
		w.updateDialogOpen.Set(true)
		w.restartAfterUpdate = func(context.Context) (func(bool) error, error) {
			attempts.Add(1)
			return nil, nil
		}
		return w.view()
	}, nsgotest.Size(1200, 800), nsgotest.Environment(refreshEnvironment(core.LifecycleActive)))
	app.WaitFor("id:app-update-dialog", timeout)
	app.Press("label:Restart Now")
	app.WaitFor("text:Wait for the current operation to finish and try again.", timeout)
	if attempts.Load() != 0 {
		t.Fatal("started a replacement while another repository was being written")
	}
}
