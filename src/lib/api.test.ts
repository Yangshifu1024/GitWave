import { describe, expect, it } from "vitest";

import { initI18n } from "@/i18n";

import { SYNC_CANCELLED_CODE, formatAppError, isCancelledSyncError } from "./api";

const i18n = initI18n();
if (i18n.language !== "en") await i18n.changeLanguage("en");

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

describe("formatAppError", () => {
  it("interpolates known codes and drops i18next-reserved param keys", () => {
    expect(
      formatAppError({
        category: "Network",
        message: "fetch failed: boom",
        trace_id: "t",
        code: "git.fetch_failed",
        params: { error: "boom", nsSeparator: "@", keySeparator: "@" },
      }),
    ).toBe("Fetch failed: boom");
  });

  it("falls back to category and message for unknown codes", () => {
    expect(
      formatAppError({
        category: "Protocol",
        message: "something odd",
        trace_id: "t",
        code: "nope.missing",
      }),
    ).toBe("Protocol: something odd");
  });
});
