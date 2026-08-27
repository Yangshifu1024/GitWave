import { describe, expect, it } from "vitest";
import { DEFAULT_PALETTE, normalizePalette, PALETTES, PALETTE_META } from "./palette";

describe("normalizePalette", () => {
  it("passes known palettes through", () => {
    for (const id of PALETTES) {
      expect(normalizePalette(id)).toBe(id);
    }
  });

  it("falls back to the default palette for unknown values", () => {
    expect(normalizePalette("solarized")).toBe(DEFAULT_PALETTE);
    expect(normalizePalette("")).toBe(DEFAULT_PALETTE);
    expect(normalizePalette(null)).toBe(DEFAULT_PALETTE);
    expect(normalizePalette(undefined)).toBe(DEFAULT_PALETTE);
  });
});

describe("palette registry", () => {
  it("defaults to native-blue", () => {
    expect(DEFAULT_PALETTE).toBe("native-blue");
  });

  it("has unique palette ids", () => {
    expect(new Set(PALETTES).size).toBe(PALETTES.length);
  });

  it("has meta with complete swatches for every palette", () => {
    for (const id of PALETTES) {
      const meta = PALETTE_META[id];
      expect(meta.id).toBe(id);
      expect(meta.name.trim()).not.toBe("");
      expect(meta.swatch.canvas).toMatch(/^#[0-9a-f]{6}$/);
      expect(meta.swatch.sidebar).toMatch(/^#[0-9a-f]{6}$/);
      expect(meta.swatch.accent).toMatch(/^#[0-9a-f]{6}$/);
      expect(meta.swatch.lanes).toHaveLength(3);
      for (const lane of meta.swatch.lanes) {
        expect(lane).toMatch(/^#[0-9a-f]{6}$/);
      }
    }
  });
});
