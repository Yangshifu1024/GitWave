import { beforeEach, describe, expect, it, vi } from "vitest";
import { STATUS_TTL_MS, useStatusAreaStore } from "./statusAreaStore";

describe("statusAreaStore", () => {
  beforeEach(() => {
    useStatusAreaStore.getState().clearStatus();
  });

  it("defaults to the success variant", () => {
    useStatusAreaStore.getState().setStatus("Fetched from origin");
    expect(useStatusAreaStore.getState().status).toEqual({
      text: "Fetched from origin",
      variant: "success",
    });
  });

  it("carries danger and info variants through", () => {
    useStatusAreaStore.getState().setStatus("Push failed", "danger");
    expect(useStatusAreaStore.getState().status?.variant).toBe("danger");

    useStatusAreaStore.getState().setStatus("Primary AI provider failed", "info");
    expect(useStatusAreaStore.getState().status?.variant).toBe("info");
  });

  it("is overwritten by the next setStatus and reset by clearStatus", () => {
    useStatusAreaStore.getState().setStatus("first");
    useStatusAreaStore.getState().setStatus("second", "danger");
    expect(useStatusAreaStore.getState().status?.text).toBe("second");

    useStatusAreaStore.getState().clearStatus();
    expect(useStatusAreaStore.getState().status).toBeNull();
  });

  it("auto-clears after the 15s idle window", () => {
    vi.useFakeTimers();
    try {
      useStatusAreaStore.getState().setStatus("Fetched from origin");
      vi.advanceTimersByTime(STATUS_TTL_MS - 1);
      expect(useStatusAreaStore.getState().status).not.toBeNull();

      vi.advanceTimersByTime(1);
      expect(useStatusAreaStore.getState().status).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });

  it("restarts the idle window on every setStatus", () => {
    vi.useFakeTimers();
    try {
      const { setStatus } = useStatusAreaStore.getState();
      setStatus("first");
      vi.advanceTimersByTime(STATUS_TTL_MS - 1);
      setStatus("second", "danger");
      vi.advanceTimersByTime(STATUS_TTL_MS - 1);
      expect(useStatusAreaStore.getState().status?.text).toBe("second");

      vi.advanceTimersByTime(1);
      expect(useStatusAreaStore.getState().status).toBeNull();
    } finally {
      vi.useRealTimers();
    }
  });
});
