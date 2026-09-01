import { afterEach, describe, expect, it, vi } from "vitest";

// The store reads localStorage once at module load, and vitest runs in a
// bare node environment — stub window.localStorage before each (re)import
// so the initial value can be exercised like in the real app.
async function loadStore() {
  vi.resetModules();
  return import("./autoRefreshStore");
}

function stubStorage(storage: {
  getItem: (key: string) => string | null;
  setItem: (key: string, value: string) => void;
}): void {
  vi.stubGlobal("window", { localStorage: storage });
}

function memoryStorage(initial?: Record<string, string>) {
  const data = new Map(Object.entries(initial ?? {}));
  return {
    data,
    getItem: (key: string): string | null => data.get(key) ?? null,
    setItem: (key: string, value: string): void => void data.set(key, value),
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("autoRefreshStore", () => {
  it("defaults to off when nothing is stored", async () => {
    const storage = memoryStorage();
    stubStorage(storage);
    const { useAutoRefreshStore } = await loadStore();

    expect(useAutoRefreshStore.getState().autoRefresh).toBe(false);
  });

  it("reads the persisted preference at module load", async () => {
    stubStorage(memoryStorage({ "gitwave-auto-refresh": "true" }));
    const { useAutoRefreshStore } = await loadStore();

    expect(useAutoRefreshStore.getState().autoRefresh).toBe(true);
  });

  it("setAutoRefresh updates the shared state and persists the value", async () => {
    const storage = memoryStorage();
    stubStorage(storage);
    const { useAutoRefreshStore } = await loadStore();

    useAutoRefreshStore.getState().setAutoRefresh(true);

    expect(useAutoRefreshStore.getState().autoRefresh).toBe(true);
    expect(storage.data.get("gitwave-auto-refresh")).toBe("true");

    useAutoRefreshStore.getState().setAutoRefresh(false);

    expect(useAutoRefreshStore.getState().autoRefresh).toBe(false);
    expect(storage.data.get("gitwave-auto-refresh")).toBe("false");
  });

  it("still applies the in-memory value when persistence throws", async () => {
    stubStorage({
      getItem: () => null,
      setItem: () => {
        throw new Error("quota exceeded");
      },
    });
    const { useAutoRefreshStore } = await loadStore();

    useAutoRefreshStore.getState().setAutoRefresh(true);

    expect(useAutoRefreshStore.getState().autoRefresh).toBe(true);
  });
});
