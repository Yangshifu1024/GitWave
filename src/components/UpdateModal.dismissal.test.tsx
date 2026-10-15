// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { UpdateModal } from "./UpdateModal";
import { useUpdaterStore } from "@/stores/updaterStore";

const { install, restart } = vi.hoisted(() => ({ install: vi.fn(), restart: vi.fn() }));
vi.mock("@/hooks/useUpdater", () => ({
  useInstallUpdate: () => install,
  useRestartApp: () => restart,
  useOpenReleases: () => vi.fn(),
  useRetryUpdate: () => vi.fn(),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

const originalGetAnimations = Object.getOwnPropertyDescriptor(Element.prototype, "getAnimations");

beforeEach(() => {
  vi.clearAllMocks();
  // jsdom has no CSS animations. Model a running exit animation whose finished
  // promise rejects when canceled, as WebKit/browser animations can do.
  Object.defineProperty(Element.prototype, "getAnimations", {
    configurable: true,
    value: function (this: Element) {
      return this.hasAttribute("data-exiting")
        ? [{ playState: "running", finished: Promise.reject(new Error("Animation canceled")) }]
        : [];
    },
  });
  useUpdaterStore.setState(useUpdaterStore.getInitialState());
  useUpdaterStore.getState().markAvailable({
    currentVersion: "0.10.3",
    newVersion: "0.10.4",
    notes: "Release notes",
    manual: false,
  });
});

afterEach(() => {
  cleanup();
  if (originalGetAnimations) {
    Object.defineProperty(Element.prototype, "getAnimations", originalGetAnimations);
  } else {
    Reflect.deleteProperty(Element.prototype, "getAnimations");
  }
});

it.each(["later", "close", "escape"])(
  "dismisses via %s even when exit animations are canceled, then reopens",
  async (method) => {
    render(<UpdateModal />);
    const dialog = await screen.findByRole("dialog");
    if (method === "escape") {
      fireEvent.keyDown(dialog, { key: "Escape", code: "Escape" });
    } else {
      fireEvent.click(
        screen.getByRole("button", {
          name: method === "later" ? "updater.actions.later" : "Close",
        }),
      );
    }
    expect(useUpdaterStore.getState().modalOpen).toBe(false);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.querySelector("[data-exiting]")).toBeNull();
    expect(install).not.toHaveBeenCalled();
    expect(restart).not.toHaveBeenCalled();
    act(() => useUpdaterStore.getState().setModalOpen(true));
    expect(await screen.findByRole("dialog")).toBeTruthy();
  },
);

it("hides an active download without resetting it and reopens when ready", async () => {
  useUpdaterStore.getState().beginDownload();
  useUpdaterStore.getState().setProgress(512, 1024);
  render(<UpdateModal />);
  fireEvent.click(screen.getByRole("button", { name: "updater.actions.hide" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(useUpdaterStore.getState()).toMatchObject({
    phase: "downloading",
    downloadedBytes: 512,
    totalBytes: 1024,
    modalOpen: false,
  });
  act(() => useUpdaterStore.getState().markReady());
  expect(await screen.findByRole("dialog", { name: "updater.title.installed" })).toBeTruthy();
  expect(install).not.toHaveBeenCalled();
  expect(restart).not.toHaveBeenCalled();
});
