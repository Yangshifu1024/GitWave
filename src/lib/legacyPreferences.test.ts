// @vitest-environment jsdom

import { invoke } from "@tauri-apps/api/core";
import { expect, it, vi } from "vitest";
import {
  flushLegacyPreferences,
  queueLegacyPreferenceExport,
  startLegacyPreferenceExport,
} from "./legacyPreferences";

vi.mock("@tauri-apps/api/core", () => ({ invoke: vi.fn(), isTauri: () => true }));

it("exports only allowlisted preferences in order and flushes the latest pending change", async () => {
  let finish = (): void => {};
  const write = vi.mocked(invoke<void>);
  write.mockImplementationOnce(
    () =>
      new Promise<void>((resolve) => {
        finish = resolve;
      }),
  );
  write.mockResolvedValue(undefined);
  localStorage.setItem("gitwave-theme", "dark");
  localStorage.setItem("gitwave.split.navigator", "20");
  localStorage.setItem("gitwave.private-token", "fixture token");
  startLegacyPreferenceExport();
  await Promise.resolve();
  expect(invoke).toHaveBeenCalledExactlyOnceWith("cmd_write_legacy_preferences", {
    values: { "gitwave-theme": "dark", "gitwave.split.navigator": "20" },
  });

  localStorage.removeItem("gitwave-theme");
  localStorage.setItem("gitwave.split.navigator", "25");
  queueLegacyPreferenceExport();
  const flushed = flushLegacyPreferences();
  await Promise.resolve();
  expect(invoke).toHaveBeenCalledTimes(1);
  finish();
  await flushed;
  expect(invoke).toHaveBeenLastCalledWith("cmd_write_legacy_preferences", {
    values: { "gitwave.split.navigator": "25" },
  });
  expect(invoke).toHaveBeenCalledTimes(2);
  write.mockRejectedValueOnce(new Error("fixture database busy"));
  await expect(flushLegacyPreferences()).rejects.toThrow("fixture database busy");
  await expect(flushLegacyPreferences()).resolves.toBeUndefined();
});
