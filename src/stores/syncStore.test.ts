import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { operationLabel, useSyncStore } from "./syncStore";

describe("syncStore op lifecycle", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    useSyncStore.setState({
      activeOp: null,
      receivedObjects: 0,
      totalObjects: 0,
      receivedBytes: 0,
      fading: false,
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("keeps the op visible during the fade, then clears it", () => {
    useSyncStore.getState().startOp("fetch");
    useSyncStore.getState().endOp("fetch");

    expect(useSyncStore.getState().fading).toBe(true);
    expect(useSyncStore.getState().activeOp).toBe("fetch");

    vi.advanceTimersByTime(150);

    expect(useSyncStore.getState().fading).toBe(false);
    expect(useSyncStore.getState().activeOp).toBeNull();
  });

  it("ignores endOp for an op that no longer owns the slot", () => {
    useSyncStore.getState().startOp("fetch");
    useSyncStore.getState().startOp("checkout");
    useSyncStore.getState().endOp("fetch");

    expect(useSyncStore.getState().fading).toBe(false);
    expect(useSyncStore.getState().activeOp).toBe("checkout");

    vi.advanceTimersByTime(150);
    expect(useSyncStore.getState().activeOp).toBe("checkout");
  });

  it("survives a double endOp for the same op", () => {
    useSyncStore.getState().startOp("push");
    useSyncStore.getState().endOp("push");
    useSyncStore.getState().endOp("push");

    vi.advanceTimersByTime(150);

    expect(useSyncStore.getState().activeOp).toBeNull();
    expect(useSyncStore.getState().fading).toBe(false);
  });

  it("lets a new op start inside the previous fade window", () => {
    useSyncStore.getState().startOp("fetch");
    useSyncStore.getState().endOp("fetch");
    useSyncStore.getState().startOp("push");

    vi.advanceTimersByTime(150);

    expect(useSyncStore.getState().activeOp).toBe("push");
    expect(useSyncStore.getState().fading).toBe(false);
  });

  it("accepts progress events while the op is running", () => {
    useSyncStore.getState().startOp("push");
    useSyncStore.getState().updateProgress({
      operation: "push",
      receivedObjects: 3,
      totalObjects: 9,
      receivedBytes: 512,
    });

    expect(useSyncStore.getState().receivedObjects).toBe(3);
    expect(useSyncStore.getState().totalObjects).toBe(9);
  });

  it("ignores progress events arriving under a UI-started op", () => {
    useSyncStore.getState().startOp("checkout");
    useSyncStore.getState().updateProgress({
      operation: "fetch",
      receivedObjects: 5,
      totalObjects: 10,
      receivedBytes: 1024,
    });

    expect(useSyncStore.getState().activeOp).toBe("checkout");
    expect(useSyncStore.getState().receivedObjects).toBe(0);
  });

  it("ignores progress events after the op has fully cleared", () => {
    useSyncStore.getState().startOp("pull");
    useSyncStore.getState().endOp("pull");
    vi.advanceTimersByTime(150);

    useSyncStore.getState().updateProgress({
      operation: "pull",
      receivedObjects: 5,
      totalObjects: 10,
      receivedBytes: 1024,
    });

    expect(useSyncStore.getState().activeOp).toBeNull();
  });
});

describe("operationLabel", () => {
  it("labels backend sync ops and UI operations", () => {
    expect(operationLabel("fetch")).toBe("Fetching from origin…");
    expect(operationLabel("checkout")).toBe("Checking out branch…");
    expect(operationLabel("remote-op")).toBe("Running remote operation…");
    expect(operationLabel(null)).toBeNull();
  });
});
