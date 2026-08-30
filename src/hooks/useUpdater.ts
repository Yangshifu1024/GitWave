// App self-update: checks GitHub Releases for a `latest.json` manifest via
// tauri-plugin-updater. macOS / Windows / Linux AppImage installs download,
// install and relaunch in-app; deb/rpm installs cannot self-replace (system
// paths, no APPIMAGE env) so they degrade to opening the releases page. The
// preference follows the app's hook + localStorage convention (see
// useAutoRefresh) and defaults to ON; startup checks are silently dropped on
// any failure so offline launches are never nagged.

import { useCallback, useEffect, useRef, useState } from "react";
import { check, type Update } from "@tauri-apps/plugin-updater";
import { relaunch } from "@tauri-apps/plugin-process";
import { platform } from "@tauri-apps/plugin-os";
import { openUrl } from "@tauri-apps/plugin-opener";
import { formatAppError, getAppVersion, isAppimage } from "@/lib/api";
import { useUpdaterStore } from "@/stores/updaterStore";

const STORAGE_KEY = "gitwave-auto-update";
const RELEASES_URL = "https://github.com/Yangshifu1024/GitWave/releases/latest";
/** Startup checks let the app settle (and restore its workspace) first. */
const STARTUP_DELAY_MS = 3000;

function readStoredAutoUpdate(): boolean {
  try {
    return window.localStorage.getItem(STORAGE_KEY) !== "false";
  } catch {
    return true;
  }
}

export interface UseAutoUpdateSettingReturn {
  autoUpdate: boolean;
  setAutoUpdate: (enabled: boolean) => void;
}

export function useAutoUpdateSetting(): UseAutoUpdateSettingReturn {
  const [autoUpdate, setAutoUpdateState] = useState(readStoredAutoUpdate);

  const setAutoUpdate = useCallback((enabled: boolean) => {
    try {
      window.localStorage.setItem(STORAGE_KEY, String(enabled));
    } catch {
      // Persistence is best-effort; the in-memory value still applies.
    }
    setAutoUpdateState(enabled);
  }, []);

  return { autoUpdate, setAutoUpdate };
}

// The plugin's Update object owns the download/install calls — kept outside
// the zustand store, which mirrors only render-facing state. It survives so
// a failed download can be retried without re-checking.
let pendingUpdate: Update | null = null;
let appimageKnown: boolean | null = null;
let installInFlight = false;

async function resolveManualDownload(): Promise<boolean> {
  // platform() is sync in @tauri-apps/plugin-os v2.
  if (platform() !== "linux") return false;
  if (appimageKnown === null) {
    try {
      appimageKnown = await isAppimage();
    } catch {
      appimageKnown = false;
    }
  }
  return !appimageKnown;
}

async function checkForUpdate(options: { silent?: boolean } = {}): Promise<void> {
  if (!options.silent) useUpdaterStore.getState().beginCheck();
  try {
    // Bounded so a hanging network can't leave "Checking…" up forever.
    const update = await check({ timeout: 15_000 });
    if (!update) {
      const version = await getAppVersion();
      if (!options.silent) useUpdaterStore.getState().markUpToDate(version);
      return;
    }
    void pendingUpdate?.close().catch(() => undefined);
    pendingUpdate = update;
    const manual = await resolveManualDownload();
    useUpdaterStore.getState().markAvailable({
      currentVersion: update.currentVersion,
      newVersion: update.version,
      manual,
    });
  } catch (e) {
    // Startup checks stay invisible: offline / no published manifest yet is
    // normal life, only an explicit check reports the failure.
    if (options.silent) return;
    useUpdaterStore.getState().fail(formatAppError(e));
  }
}

export interface UseCheckForUpdatesReturn {
  check: () => Promise<void>;
  busy: boolean;
}

/**
 * Menu-entry entry point (both menu surfaces): same flow as the Settings
 * button. Callers open Settings alongside so an "up to date" outcome is
 * visible — the update modal itself only pops when a release was found.
 */
export function runUpdateCheck(): void {
  void checkForUpdate();
}

/** Manual check (Settings button). Reuses the modal + status text surfaces. */
export function useCheckForUpdates(): UseCheckForUpdatesReturn {
  const phase = useUpdaterStore((s) => s.phase);
  const check = useCallback(() => checkForUpdate(), []);
  // A mid-flight download or installed-but-unrestarted app must not be
  // reset by a re-check — it would drop the install/relaunch affordance.
  const busy = phase === "checking" || phase === "downloading" || phase === "ready";
  return { check, busy };
}

export function useInstallUpdate(): () => void {
  return useCallback(() => {
    if (installInFlight) return;
    const update = pendingUpdate;
    if (!update) return;
    installInFlight = true;
    useUpdaterStore.getState().beginDownload();
    let downloaded = 0;
    let total: number | null = null;
    void update
      .downloadAndInstall((event) => {
        if (event.event === "Started") {
          total = event.data.contentLength ?? null;
        } else if (event.event === "Progress") {
          downloaded += event.data.chunkLength;
        } else {
          // Finished: clamp so the bar reads 100% even if chunks miscount.
          downloaded = total ?? downloaded;
        }
        useUpdaterStore.getState().setProgress(downloaded, total);
      })
      .then(() => {
        installInFlight = false;
        // Applied — release the Rust-side handle and stop offering the
        // object as a retry target (a fresh check starts the next cycle).
        void pendingUpdate?.close().catch(() => undefined);
        pendingUpdate = null;
        useUpdaterStore.getState().markReady();
      })
      .catch((e) => {
        installInFlight = false;
        useUpdaterStore.getState().fail(formatAppError(e));
      });
  }, []);
}

export function useRestartApp(): () => void {
  return useCallback(() => {
    void relaunch();
  }, []);
}

export function useOpenReleases(): () => void {
  return useCallback(() => {
    void openUrl(RELEASES_URL).catch((e) => useUpdaterStore.getState().fail(formatAppError(e)));
  }, []);
}

/**
 * Retry after an error: a held Update object means the failure happened
 * mid-download/install — resume that; otherwise the check itself failed.
 */
export function useRetryUpdate(): () => void {
  const install = useInstallUpdate();
  const { check } = useCheckForUpdates();
  return useCallback(() => {
    if (pendingUpdate) install();
    else void check();
  }, [install, check]);
}

/** Runs once per app start when the preference allows it. */
export function useStartupUpdateCheck(): void {
  const { autoUpdate } = useAutoUpdateSetting();
  const startedRef = useRef(false);

  useEffect(() => {
    // Toggling the preference mid-session does not re-run the startup check;
    // it takes effect on the next launch. The started flag flips inside the
    // timer: React StrictMode's mount→unmount→remount cycle cancels the
    // first timer, so setting it eagerly would skip the check entirely.
    if (!autoUpdate || startedRef.current) return;
    const timer = window.setTimeout(() => {
      startedRef.current = true;
      void checkForUpdate({ silent: true });
    }, STARTUP_DELAY_MS);
    return () => window.clearTimeout(timer);
  }, [autoUpdate]);
}
