import { describe, expect, it } from "vitest";

import { SYNC_CANCELLED_CODE, isCancelledSyncError } from "./api";

describe("isCancelledSyncError", () => {
  it("matches the backend cancel code", () => {
    expect(
      isCancelledSyncError({
        category: "Network",
        message: "sync cancelled by user",
        trace_id: "t",
        code: SYNC_CANCELLED_CODE,
      }),
    ).toBe(true);
  });

  it("rejects other error codes, plain values and null", () => {
    expect(
      isCancelledSyncError({
        category: "Network",
        message: "fetch failed",
        trace_id: "t",
        code: "git.fetch_failed",
      }),
    ).toBe(false);
    expect(isCancelledSyncError({ category: "Network", message: "no code" })).toBe(false);
    expect(isCancelledSyncError("git.sync_cancelled")).toBe(false);
    expect(isCancelledSyncError(null)).toBe(false);
  });
});
