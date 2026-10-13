// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { relaunch } from "@tauri-apps/plugin-process";
import { useRestartApp } from "./useUpdater";
import { useUpdaterStore } from "@/stores/updaterStore";

vi.mock("@tauri-apps/plugin-process", () => ({ relaunch: vi.fn() }));
vi.mock("@tauri-apps/plugin-updater", () => ({ check: vi.fn() }));
vi.mock("@tauri-apps/plugin-os", () => ({ platform: () => "macos" }));
vi.mock("@tauri-apps/plugin-opener", () => ({ openUrl: vi.fn() }));
vi.mock("@/lib/api", () => ({
  formatAppError: (error: unknown) => String(error),
  getAppVersion: vi.fn(),
  isAppimage: vi.fn(),
}));

describe("update restart", () => {
  beforeEach(() => {
    vi.mocked(relaunch).mockReset();
    useUpdaterStore.getState().markAvailable({
      currentVersion: "0.10.1",
      newVersion: "0.10.2",
      notes: "Restart fix",
      manual: false,
    });
    useUpdaterStore.getState().markReady();
  });
  afterEach(cleanup);

  it("preserves the installed update and allows restart retry after rejection", async () => {
    vi.mocked(relaunch).mockRejectedValueOnce(new Error("IPC rejected")).mockResolvedValueOnce();
    const { result } = renderHook(useRestartApp);
    await act(async () => {
      result.current();
      await Promise.resolve();
    });
    expect(useUpdaterStore.getState()).toMatchObject({
      phase: "ready",
      restarting: false,
      modalOpen: true,
      error: "Error: IPC rejected",
      newVersion: "0.10.2",
      notes: "Restart fix",
    });
    await act(async () => {
      result.current();
      await Promise.resolve();
    });
    expect(relaunch).toHaveBeenCalledTimes(2);
    expect(useUpdaterStore.getState()).toMatchObject({ phase: "ready", error: null });
  });

  it("coalesces clicks from multiple hook instances while restart is pending", async () => {
    let resolve!: () => void;
    vi.mocked(relaunch).mockReturnValue(new Promise<void>((done) => (resolve = done)));
    const first = renderHook(useRestartApp);
    const second = renderHook(useRestartApp);
    act(() => {
      first.result.current();
      second.result.current();
    });
    expect(relaunch).toHaveBeenCalledTimes(1);
    expect(useUpdaterStore.getState().restarting).toBe(true);
    await act(async () => {
      resolve();
      await Promise.resolve();
    });
    expect(useUpdaterStore.getState().restarting).toBe(false);
  });

  it("does not restart before installation has completed", () => {
    useUpdaterStore.getState().beginDownload();
    const { result } = renderHook(useRestartApp);
    act(() => result.current());
    expect(relaunch).not.toHaveBeenCalled();
  });

  it("reopens the installed update if restart fails after hiding the modal", async () => {
    let reject!: (error: Error) => void;
    vi.mocked(relaunch).mockReturnValue(new Promise<void>((_, fail) => (reject = fail)));
    const { result } = renderHook(useRestartApp);
    act(() => result.current());
    useUpdaterStore.getState().setModalOpen(false);
    await act(async () => {
      reject(new Error("denied"));
      await Promise.resolve();
    });
    expect(useUpdaterStore.getState()).toMatchObject({
      phase: "ready",
      modalOpen: true,
      restarting: false,
      error: "Error: denied",
    });
  });
});
