// Quit guard: intercepts window close and in-app quit (File → Exit), checks
// every repo in the active workspace for uncommitted changes, and asks before
// exiting. Fails open: any error or a 500 ms timeout lets the app quit
// without prompting — the guard must never trap the user in the app.

import { useEffect, useSyncExternalStore } from "react";
import { getCurrentWindow } from "@tauri-apps/api/window";

import { getDirtyRepos, quitApp, type DirtyRepoSummary } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

const STATUS_TIMEOUT_MS = 500;

// Guards against StrictMode double-mount registering two close listeners.
let closeListenerRegistered = false;

export interface QuitGuardState {
  open: boolean;
  dirtyRepos: DirtyRepoSummary[];
}

let state: QuitGuardState = { open: false, dirtyRepos: [] };
const listeners = new Set<() => void>();

function setState(next: QuitGuardState): void {
  state = next;
  for (const listener of listeners) listener();
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function withTimeout<T>(promise: Promise<T>, ms: number): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error("quit guard timed out")), ms);
    promise.then(
      (value) => {
        clearTimeout(timer);
        resolve(value);
      },
      (error: unknown) => {
        clearTimeout(timer);
        // Tauri invoke rejections are usually plain strings; never rely on
        // implicit toString ([object Object]).
        const message =
          typeof error === "string"
            ? error
            : error instanceof Error
              ? error.message
              : "unknown quit guard failure";
        reject(new Error(message));
      },
    );
  });
}

function destroyApp(): void {
  // window.destroy() requires core:window:allow-destroy, which is not in the
  // app capabilities — the call fails silently and the window gets stuck.
  // quit_app (app.exit(0)) is a custom command and always works; destroy is
  // only a last-resort fallback.
  void quitApp()
    .catch(() => getCurrentWindow().destroy())
    .catch(() => undefined);
}

/** Unified quit entry: window close button and File → Exit both land here. */
export async function requestQuit(): Promise<void> {
  const workspaceId = useWorkspaceUiStore.getState().activeWorkspaceId;
  if (!workspaceId) {
    destroyApp();
    return;
  }
  let dirty: DirtyRepoSummary[];
  try {
    dirty = await withTimeout(getDirtyRepos(workspaceId), STATUS_TIMEOUT_MS);
  } catch {
    destroyApp();
    return;
  }
  if (dirty.length > 0) setState({ open: true, dirtyRepos: dirty });
  else destroyApp();
}

/** "Quit anyway": really exit, keeping the working copies untouched. */
export function confirmQuit(): void {
  setState({ open: false, dirtyRepos: [] });
  destroyApp();
}

/** "Cancel": close the dialog and stay in the app. */
export function cancelQuit(): void {
  setState({ open: false, dirtyRepos: [] });
}

/** Mount once in App: registers the Tauri close-requested interceptor. */
export function useQuitGuard(): QuitGuardState {
  const snapshot = useSyncExternalStore(subscribe, () => state);

  useEffect(() => {
    // The effect body runs twice under StrictMode; the async unlisten dance
    // races the cleanup and can leave two close listeners behind. Register
    // exactly once for the process lifetime instead.
    if (closeListenerRegistered) return;
    closeListenerRegistered = true;
    void getCurrentWindow()
      .onCloseRequested(async (event) => {
        event.preventDefault();
        await requestQuit();
      })
      .catch(() => {
        closeListenerRegistered = false;
      });
  }, []);

  return snapshot;
}
