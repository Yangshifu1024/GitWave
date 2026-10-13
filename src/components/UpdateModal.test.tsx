// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { UpdateModal } from "./UpdateModal";
import { useUpdaterStore } from "@/stores/updaterStore";
import english from "@/i18n/locales/en/updater.json";

const { restart } = vi.hoisted(() => ({ restart: vi.fn() }));
vi.mock("@/hooks/useUpdater", () => ({
  useRestartApp: () => restart,
  useInstallUpdate: () => vi.fn(),
  useOpenReleases: () => vi.fn(),
  useRetryUpdate: () => vi.fn(),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, values?: { error: string }) => {
      if (key === "updater.restartFailed") {
        return english.updater.restartFailed.replace("{{error}}", values?.error ?? "");
      }
      return key;
    },
  }),
}));
vi.mock("@/components/ui/Modal", () => ({
  Modal: ({ children, footer }: { children: React.ReactNode; footer: React.ReactNode }) => (
    <div>
      {children}
      {footer}
    </div>
  ),
}));
vi.mock("@/components/ui/Button", () => ({
  Button: ({
    children,
    onClick,
    disabled,
  }: {
    children: React.ReactNode;
    onClick: () => void;
    disabled?: boolean;
  }) => (
    <button onClick={onClick} disabled={disabled}>
      {children}
    </button>
  ),
}));

describe("UpdateModal restart", () => {
  beforeEach(() => {
    restart.mockClear();
    useUpdaterStore.getState().markAvailable({
      currentVersion: "0.10.1",
      newVersion: "0.10.2",
      notes: null,
      manual: false,
    });
    useUpdaterStore.getState().markReady();
  });
  afterEach(cleanup);

  it("shows restart failure while keeping the installed version and restart button", () => {
    useUpdaterStore.getState().finishRestart("IPC denied");
    render(<UpdateModal />);
    expect(screen.getByRole("alert").textContent).toContain(
      "Could not restart GitWave: IPC denied",
    );
    expect(screen.getByText("v0.10.2")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "updater.actions.restartNow" }));
    expect(restart).toHaveBeenCalledOnce();
  });

  it("disables restart and labels the pending request", () => {
    useUpdaterStore.getState().beginRestart();
    render(<UpdateModal />);
    const button = screen.getByRole<HTMLButtonElement>("button", {
      name: "updater.actions.restarting",
    });
    expect(button.disabled).toBe(true);
    fireEvent.click(button);
    expect(restart).not.toHaveBeenCalled();
  });
});
