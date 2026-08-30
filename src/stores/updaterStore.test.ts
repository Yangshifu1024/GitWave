import { beforeEach, describe, expect, it } from "vitest";

import { useUpdaterStore } from "./updaterStore";

function reset(): void {
  useUpdaterStore.setState({
    phase: "idle",
    modalOpen: false,
    currentVersion: null,
    newVersion: null,
    downloadedBytes: 0,
    totalBytes: null,
    error: null,
  });
}

describe("updaterStore", () => {
  beforeEach(reset);

  it("beginCheck clears a previous error and enters checking", () => {
    useUpdaterStore.getState().fail("boom");
    useUpdaterStore.getState().beginCheck();
    expect(useUpdaterStore.getState().phase).toBe("checking");
    expect(useUpdaterStore.getState().error).toBeNull();
  });

  it("markUpToDate records the running version without opening the modal", () => {
    useUpdaterStore.getState().markUpToDate("0.5.0");
    const s = useUpdaterStore.getState();
    expect(s.phase).toBe("up-to-date");
    expect(s.currentVersion).toBe("0.5.0");
    expect(s.modalOpen).toBe(false);
  });

  it("markAvailable opens the modal; manual routes to manual-download", () => {
    useUpdaterStore.getState().markAvailable({
      currentVersion: "0.5.0",
      newVersion: "0.6.0",
      manual: false,
    });
    expect(useUpdaterStore.getState().phase).toBe("available");
    expect(useUpdaterStore.getState().modalOpen).toBe(true);

    useUpdaterStore.getState().markAvailable({
      currentVersion: "0.5.0",
      newVersion: "0.6.0",
      manual: true,
    });
    expect(useUpdaterStore.getState().phase).toBe("manual-download");
  });

  it("download lifecycle: begin resets progress, progress accumulates, ready closes it out", () => {
    useUpdaterStore.getState().markAvailable({
      currentVersion: "0.5.0",
      newVersion: "0.6.0",
      manual: false,
    });
    useUpdaterStore.getState().beginDownload();
    useUpdaterStore.getState().setProgress(1024, 4096);
    expect(useUpdaterStore.getState().downloadedBytes).toBe(1024);
    expect(useUpdaterStore.getState().totalBytes).toBe(4096);
    useUpdaterStore.getState().markReady();
    expect(useUpdaterStore.getState().phase).toBe("ready");
  });

  it("fail surfaces the message but keeps the modal openable for retry", () => {
    useUpdaterStore.getState().fail("network down");
    const s = useUpdaterStore.getState();
    expect(s.phase).toBe("error");
    expect(s.error).toBe("network down");
    s.setModalOpen(true);
    expect(useUpdaterStore.getState().modalOpen).toBe(true);
  });
});
