package main

import (
	"context"
	"errors"
	"runtime"
	"time"

	"gitwave.work/internal/appupdate"

	"github.com/nsgo-dev/nsgo/core"
)

// A release build receives its public key from the packaging workflow.
var releaseUpdateKey string

const startupUpdateDelay = 3 * time.Second

type updatePhase uint8

const (
	updateIdle updatePhase = iota
	updateChecking
	updateCurrent
	updateAvailable
	updateDownloading
	updateReady
	updateFailed
)

type updateOffer struct {
	version, notes, page string
	candidate            *appupdate.Candidate
}

type updateState struct {
	phase           updatePhase
	offer           updateOffer
	err             string
	received, total int64
	restarting      bool
	retryInstall    bool
}

type appUpdateUI struct {
	autoUpdate          *core.Signal[bool]
	autoUpdateSaving    *core.Signal[bool]
	autoUpdateError     *core.Signal[string]
	startupPolicyLoaded bool
	updateState         *core.Signal[updateState]
	updateDialogOpen    *core.Signal[bool]
	checkRelease        func(context.Context) (updateOffer, error)
	installUpdate       func(context.Context, updateOffer, func(int64, int64)) (appupdate.Restart, error)
	installUpdateOnExit bool
	restartAfterUpdate  appupdate.Restart
	stopUpdateCheck     context.CancelFunc
	updateCheckID       uint64
}

func newAppUpdateUI(cx *core.Scope) appUpdateUI {
	return appUpdateUI{autoUpdate: core.NewSignal(cx, true), autoUpdateSaving: core.NewSignal(cx, false),
		autoUpdateError: core.NewSignal(cx, ""), updateState: core.NewSignal(cx, updateState{}),
		updateDialogOpen: core.NewSignal(cx, false)}
}

func (w *workbench) configureUpdates() {
	if releaseUpdateKey == "" {
		return
	}
	client, err := appupdate.New(releaseUpdateKey)
	if err != nil {
		w.autoUpdateError.Set(w.tr("The update configuration is invalid.", "更新配置无效。"))
		return
	}
	var target string
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "darwin/arm64":
		target = appupdate.MacOSArm64
	case "windows/amd64":
		target = appupdate.WindowsX64
	case "linux/amd64":
		target = appupdate.LinuxX64
	default:
		return
	}
	current := appVersion()
	if installer := appupdate.InstallerForRunningApp(); installer != nil {
		w.installUpdateOnExit = runtime.GOOS == "windows"
		w.installUpdate = func(ctx context.Context, offer updateOffer, progress func(int64, int64)) (appupdate.Restart, error) {
			return installer(ctx, offer.candidate, progress)
		}
	}
	w.checkRelease = func(ctx context.Context) (updateOffer, error) {
		candidate, err := client.Check(ctx, current, target)
		if candidate == nil || err != nil {
			return updateOffer{}, err
		}
		return updateOffer{version: candidate.Version(), notes: candidate.Notes(), page: candidate.ReleaseNotesURL(), candidate: candidate}, nil
	}
}

func (w *workbench) loadAutoUpdate(proxyReady bool) {
	if w.workspaceStore == nil {
		return
	}
	ctx, cancel := context.WithTimeout(w.appCtx, time.Second)
	defer cancel()
	enabled, err := w.workspaceStore.AutoUpdate(ctx)
	w.startupPolicyLoaded = proxyReady && err == nil
	w.autoUpdate.Set(enabled)
	if err != nil {
		w.autoUpdateError.Set(w.tr("Could not read the update preference.", "无法读取更新偏好。"))
	}
}

func (w *workbench) saveAutoUpdate(enabled bool) {
	if w.autoUpdateSaving.Get() || w.autoUpdate.Get() == enabled {
		return
	}
	store := w.workspaceStore
	w.autoUpdateError.Set("")
	if !enabled && w.stopUpdateCheck != nil && w.updateState.Get().phase != updateChecking {
		w.stopUpdateCheck()
		w.stopUpdateCheck = nil
		w.updateCheckID++
	}
	if store == nil {
		w.autoUpdate.Set(enabled)
		return
	}
	w.autoUpdateSaving.Set(true)
	go func() {
		ctx, cancel := context.WithTimeout(w.appCtx, 3*time.Second)
		defer cancel()
		err := store.SaveAutoUpdate(ctx, enabled)
		w.cx.Post(func() {
			w.autoUpdateSaving.Set(false)
			if err != nil {
				w.autoUpdateError.Set(w.tr("Could not save the update preference.", "无法保存更新偏好。"))
				return
			}
			w.autoUpdate.Set(enabled)
		})
	}()
}

func (w *workbench) watchStartupUpdate() {
	if w.checkRelease == nil || !w.startupPolicyLoaded {
		return
	}
	var stop func() bool
	w.cx.Effect(func() {
		if stop != nil {
			stop()
			stop = nil
		}
		if w.autoUpdate.Get() && !w.autoUpdateSaving.Get() && w.updateCheckID == 0 {
			stop = w.cx.AfterFunc(startupUpdateDelay, func() { w.checkForAppUpdate(true) })
		}
	})
	w.cx.Cleanup(func() {
		if stop != nil {
			stop()
		}
	})
}

func (w *workbench) checkUpdatesFromMenu() {
	w.openRefreshSettings()
	if w.refreshSettingsOpen.Get() {
		w.checkForAppUpdate(false)
	}
}

func (w *workbench) checkForAppUpdate(silent bool) {
	if silent && (!w.startupPolicyLoaded || w.updateCheckID != 0 || !w.autoUpdate.Get() || w.autoUpdateSaving.Get()) {
		return
	}
	if w.updateCheckDisabled() {
		return
	}
	check := w.checkRelease
	if check == nil {
		if !silent {
			w.updateState.Set(updateState{phase: updateFailed, err: w.tr("Update checks are unavailable in this build.", "当前构建无法检查更新。")})
		}
		return
	}
	w.updateCheckID++
	id := w.updateCheckID
	if w.stopUpdateCheck != nil {
		w.stopUpdateCheck()
	}
	ctx, cancel := context.WithCancel(w.networkContext(w.appCtx))
	w.stopUpdateCheck = cancel
	if !silent {
		w.updateDialogOpen.Set(false)
		w.updateState.Set(updateState{phase: updateChecking})
	}
	go func() {
		defer cancel()
		offer, err := check(ctx)
		w.cx.Post(func() {
			if id != w.updateCheckID || w.appCtx.Err() != nil {
				return
			}
			w.stopUpdateCheck = nil
			switch {
			case err != nil:
				if !silent {
					w.updateState.Set(updateState{phase: updateFailed, err: w.updateCheckError(err)})
				}
			case offer.version == "":
				if !silent {
					w.updateState.Set(updateState{phase: updateCurrent})
				}
			default:
				w.updateState.Set(updateState{phase: updateAvailable, offer: offer})
				w.updateDialogOpen.Set(true)
			}
		})
	}()
}

func (w *workbench) updateCheckDisabled() bool {
	switch w.updateState.Get().phase {
	case updateChecking, updateDownloading, updateReady:
		return true
	default:
		return false
	}
}

func (w *workbench) downloadAppUpdate() {
	state := w.updateState.Get()
	install := w.installUpdate
	if state.phase != updateAvailable || install == nil {
		return
	}
	state.phase, state.err, state.received, state.total, state.retryInstall = updateDownloading, "", 0, 0, false
	w.updateState.Set(state)
	ctx := w.networkContext(w.appCtx)
	go func() {
		restart, err := install(ctx, state.offer, func(received, total int64) {
			w.cx.Post(func() {
				progress := w.updateState.Get()
				if progress.phase == updateDownloading {
					progress.received, progress.total = received, total
					w.updateState.Set(progress)
				}
			})
		})
		w.cx.Post(func() {
			if w.appCtx.Err() != nil {
				return
			}
			state := w.updateState.Get()
			if err != nil {
				state.phase = updateAvailable
				state.retryInstall = true
				state.err = w.tr("Could not install the update: ", "无法安装更新：") + err.Error()
			} else {
				w.restartAfterUpdate = restart
				state.phase = updateReady
			}
			w.updateState.Set(state)
			w.updateDialogOpen.Set(true)
			if err == nil && w.installUpdateOnExit {
				w.restartAppUpdate()
			}
		})
	}()
}

func (w *workbench) restartAppUpdate() {
	state := w.updateState.Get()
	restart := w.restartAfterUpdate
	if state.phase != updateReady || state.restarting || restart == nil {
		return
	}
	if reason := w.updateRestartBlock(); reason != "" {
		state.err = reason
		w.updateState.Set(state)
		return
	}
	state.restarting, state.err = true, ""
	w.updateState.Set(state)
	go func() {
		finish, err := restart(w.appCtx)
		w.cx.Post(func() {
			if w.appCtx.Err() != nil {
				if finish != nil {
					_ = finish(false)
				}
				return
			}
			state := w.updateState.Get()
			state.restarting = false
			if err == nil {
				if reason := w.updateRestartBlock(); reason != "" {
					if finish != nil {
						_ = finish(false)
					}
					state.err = reason
				} else {
					if finish != nil {
						err = finish(true)
					}
					if err == nil {
						w.cx.Quit()
						return
					}
				}
			}
			if err != nil {
				state.err = w.tr("Could not restart GitWave: ", "无法重启 GitWave：") + err.Error()
			}
			w.updateState.Set(state)
			w.updateDialogOpen.Set(true)
		})
	}()
}

func (w *workbench) updateRestartBlock() string {
	wait := w.tr("Wait for the current operation to finish and try again.", "请等待当前操作结束后重试。")
	if w.workspaceBusy.Get() || w.desktopPrefsBusy.Get() || w.refreshSettingsBusy.Get() ||
		w.autoUpdateSaving.Get() || w.proxyBusy || w.fetch.running.Get() != nil {
		return wait
	}
	for _, write := range w.changesWrites.Get() {
		if write.pending {
			return wait
		}
	}
	if session := w.conflictPanel.Get(); session != nil {
		for _, draft := range session.drafts {
			if draft.dirty() {
				return w.tr("Save or discard the conflict edits before restarting.", "请先保存或丢弃冲突编辑，再重启应用。")
			}
		}
	}
	return ""
}

func (w *workbench) updateCheckError(err error) string {
	switch {
	case errors.Is(err, appupdate.ErrManifest), errors.Is(err, appupdate.ErrSignature):
		return w.tr("Could not verify the update information.", "无法验证更新信息。")
	case errors.Is(err, context.DeadlineExceeded):
		return w.tr("The update check timed out.", "检查更新超时。")
	default:
		return w.tr("Could not check for updates.", "检查更新失败。")
	}
}
