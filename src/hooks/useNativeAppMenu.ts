// macOS: install the app menu into the system menu bar, replacing both
// Tauri's default menu and the in-app AppMenuBar (which stays for
// Windows / Linux). The structure comes from appMenuSpec.ts via
// nativeMenuBuild.ts so items, gating and dispatch stay identical to the
// in-app bar by construction; clicks re-enter the same ui-store action bus
// that ActionBar consumes. No-op on other platforms. The menu is rebuilt
// when gating changes (workspace / repo switch, sync busy, branch state) —
// moments when the menu is normally closed, so replacing the NSMenu
// mid-open is unlikely.

import { useEffect, useRef, type RefObject } from "react";
import { Menu } from "@tauri-apps/api/menu";

import { buildAppMenuSpec, type AppMenuItemHandler, type MenuGating } from "@/lib/appMenuSpec";
import { buildNativeAppMenuOptions } from "@/lib/nativeMenuBuild";
import { isMacOS } from "@/lib/platform";
import { runUpdateCheck } from "@/hooks/useUpdater";
import { useUiStore, type AppMenuAction } from "@/stores/uiStore";
import { readLastActive } from "@/stores/workspaceStore";
import { useAppMenuGating } from "./useAppMenuGating";

/** Callbacks resolved at fire time via the latest-callbacks ref below. */
interface LatestCallbacks {
  requestMenuAction: (action: AppMenuAction) => void;
  setSettingsOpen: (open: boolean) => void;
  onAbout: () => void;
}

function handlerFrom(callbacks: RefObject<LatestCallbacks>): AppMenuItemHandler {
  return {
    requestMenuAction: (action) => callbacks.current.requestMenuAction(action),
    openSettings: () => callbacks.current.setSettingsOpen(true),
    openAbout: () => callbacks.current.onAbout(),
    // Settings opens with the check so "up to date" has a visible landing
    // spot; a found release pops the update modal on top of it.
    checkUpdates: () => {
      callbacks.current.setSettingsOpen(true);
      runUpdateCheck();
    },
    // The native Quit is the predefined OS item — macOS handles it and this
    // route never fires; kept explicit so both surfaces route alike.
    quit: () => undefined,
  };
}

async function installMenu(gating: MenuGating, handler: AppMenuItemHandler): Promise<Menu> {
  const menu = await Menu.new(buildNativeAppMenuOptions(buildAppMenuSpec(gating), handler));
  const previous = await menu.setAsAppMenu();
  // Free the replaced menu (ours from a previous install, or Tauri's
  // default on first install).
  if (previous) await previous.close();
  return menu;
}

// Module-level serialization shared by the startup install and the hook's
// rebuilds: two installs racing through async IPC could land out of order
// and leave an older menu as the active one. The rejected-handler form keeps
// one failed install from poisoning the queue for everything after it.
let installQueue: Promise<void> = Promise.resolve();

function enqueueInstall(install: () => Promise<void>): void {
  installQueue = installQueue.then(install, install);
}

/**
 * Fire-and-forget early install for app startup (main.tsx): replaces the
 * Tauri default menu before the window becomes visible. Gating reads the
 * persisted last-active workspace/repo (synchronous localStorage); working
 * copy state always loads async, so the hook's rebuild with live gating
 * refines the item states right after React mounts and supersedes this
 * install.
 */
export function installNativeAppMenuEarly(): void {
  if (!isMacOS()) return;
  enqueueInstall(async () => {
    try {
      const lastActive = readLastActive();
      const gating: MenuGating = {
        noWorkspace: !lastActive.workspaceId,
        noRepo: !lastActive.repoId,
        detached: false,
        hasSha: false,
        syncBusy: false,
      };
      const handler: AppMenuItemHandler = {
        requestMenuAction: (action) => useUiStore.getState().requestMenuAction(action),
        openSettings: () => useUiStore.getState().setSettingsOpen(true),
        // Unreachable pre-mount (the window is hidden until React triggers
        // activate_and_show); this transient install is replaced on mount.
        openAbout: () => undefined,
        checkUpdates: () => undefined,
        quit: () => undefined,
      };
      await installMenu(gating, handler);
    } catch (error) {
      // E.g. missing core:menu capability or an unsupported webview —
      // the app keeps working, only the system menu stays at the default.
      console.error("[app-menu] failed to install native app menu:", error);
    }
  });
}

export function useNativeAppMenu(options: { onAbout: () => void }): void {
  const gating = useAppMenuGating();
  const requestMenuAction = useUiStore((s) => s.requestMenuAction);
  const setSettingsOpen = useUiStore((s) => s.setSettingsOpen);

  // Latest-callbacks ref: callers may pass fresh closures every render, and
  // a rebuild should only be driven by gating changes, not identity churn.
  const callbacks = useRef<LatestCallbacks>({
    requestMenuAction,
    setSettingsOpen,
    onAbout: options.onAbout,
  });
  useEffect(() => {
    // Declared before the install effect so the freshest callbacks are in
    // place before any rebuild it triggers in the same commit.
    callbacks.current = { requestMenuAction, setSettingsOpen, onAbout: options.onAbout };
  });

  useEffect(() => {
    if (!isMacOS()) return undefined;

    const run = { stale: false, installed: null as Menu | null };
    const handler = handlerFrom(callbacks);

    enqueueInstall(async () => {
      try {
        const menu = await installMenu(gating, handler);
        // Post-install stale recheck: cleanup may have run during the awaits.
        // Ours stays the live menu either way — a stale run implies a newer
        // enqueued install exists and will replace (and close) it; on final
        // unmount the app is quitting.
        if (!run.stale) run.installed = menu;
      } catch (error) {
        // E.g. missing core:menu capability or an unsupported webview —
        // the app keeps working, only the system menu stays at the default.
        console.error("[app-menu] failed to install native app menu:", error);
      }
    });

    return () => {
      run.stale = true;
      if (run.installed) {
        void run.installed.close().catch(() => undefined);
        run.installed = null;
      }
    };
  }, [gating]);
}
