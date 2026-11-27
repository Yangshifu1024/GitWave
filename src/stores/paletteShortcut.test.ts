// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";
import { matchesPaletteShortcut, usePaletteShortcut } from "./paletteShortcut";
afterEach(() => {
  usePaletteShortcut.getState().set("Mod+K");
  localStorage.clear();
  vi.restoreAllMocks();
});

it("persists the chosen shortcut, rehydrates it and falls back from unsupported stored bindings", async () => {
  usePaletteShortcut.getState().set("Mod+Alt+K");
  const key = "gitwave.palette-shortcut";
  const saved = localStorage.getItem(key)!;
  expect(JSON.parse(saved)).toMatchObject({ state: { chord: "Mod+Alt+K" } });
  usePaletteShortcut.getState().set("Mod+K");
  localStorage.setItem(key, saved);
  await usePaletteShortcut.persist.rehydrate();
  expect(usePaletteShortcut.getState().chord).toBe("Mod+Alt+K");
  localStorage.setItem(key, JSON.stringify({ version: 0, state: { chord: "Delete" } }));
  await usePaletteShortcut.persist.rehydrate();
  expect(usePaletteShortcut.getState().chord).toBe("Mod+K");
});
it.each(["ctrlKey", "metaKey"] as const)(
  "matches exact modifiers for %s and ignores IME/repeats",
  (mod) => {
    vi.spyOn(navigator, "userAgent", "get").mockReturnValue(
      mod === "metaKey" ? "Macintosh" : "Linux",
    );
    const event = (changes: KeyboardEventInit = {}) =>
      new KeyboardEvent("keydown", { key: "K", [mod]: true, ...changes });
    expect(matchesPaletteShortcut(event(), "Mod+K")).toBe(true);
    expect(
      matchesPaletteShortcut(event({ altKey: true, key: "Dead", code: "KeyK" }), "Mod+Alt+K"),
    ).toBe(true);
    expect(
      matchesPaletteShortcut(
        event({ [mod]: false, [mod === "metaKey" ? "ctrlKey" : "metaKey"]: true }),
        "Mod+K",
      ),
    ).toBe(false);
    expect(matchesPaletteShortcut(event({ altKey: true }), "Mod+Alt+K")).toBe(true);
    expect(matchesPaletteShortcut(event({ shiftKey: true }), "Mod+Shift+K")).toBe(true);
    for (const changes of [
      { altKey: true },
      { shiftKey: true },
      { key: "X" },
      { [mod]: false },
      { ctrlKey: true, metaKey: true },
      { repeat: true },
      { isComposing: true },
    ]) {
      expect(matchesPaletteShortcut(event(changes), "Mod+K")).toBe(false);
    }
  },
);
