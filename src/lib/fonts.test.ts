import { describe, expect, it } from "vitest";
import { buildFontOverride, previewFontFamily, sanitizeFontList } from "./fonts";

describe("sanitizeFontList", () => {
  it("trims and joins comma-separated names", () => {
    expect(sanitizeFontList(" JetBrains Mono , Fira Code ")).toBe("JetBrains Mono, Fira Code");
  });

  it("collapses internal whitespace runs", () => {
    expect(sanitizeFontList("JetBrains  Mono")).toBe("JetBrains Mono");
  });

  it("keeps CJK and other non-ASCII names", () => {
    expect(sanitizeFontList("微软雅黑, モトヤLマルベリ3等幅")).toBe(
      "微软雅黑, モトヤLマルベリ3等幅",
    );
  });

  it("strips characters that could break the quoted font-family value", () => {
    expect(sanitizeFontList('Jet"Brains\\ Mono')).toBe("JetBrains Mono");
    expect(sanitizeFontList("a';b{c}<d>")).toBe("abcd");
  });

  it("strips control characters", () => {
    expect(sanitizeFontList("Jet\u0000Brains\u0007 Mono")).toBe("JetBrains Mono");
  });

  it("drops empty segments", () => {
    expect(sanitizeFontList(", , JetBrains Mono,")).toBe("JetBrains Mono");
  });

  it("returns empty for blank or fully-forbidden input", () => {
    expect(sanitizeFontList("")).toBe("");
    expect(sanitizeFontList("   ")).toBe("");
    expect(sanitizeFontList('"\';,\'"')).toBe("");
  });
});

describe("buildFontOverride", () => {
  it("quotes each name and appends the fallback chain var", () => {
    expect(buildFontOverride("JetBrains Mono", "mono")).toBe(
      '"JetBrains Mono", var(--font-mono-fallback)',
    );
  });

  it("quotes every name in a multi-font list", () => {
    expect(buildFontOverride("Source Han Sans SC, 微软雅黑", "sans")).toBe(
      '"Source Han Sans SC", "微软雅黑", var(--font-sans-fallback)',
    );
  });

  it("returns empty for the default chain (no override)", () => {
    expect(buildFontOverride("", "sans")).toBe("");
  });
});

describe("previewFontFamily", () => {
  it("falls back to the default chain var when the draft is blank", () => {
    expect(previewFontFamily("", "sans")).toBe("var(--font-sans-fallback)");
    expect(previewFontFamily("", "mono")).toBe("var(--font-mono-fallback)");
  });

  it("delegates to buildFontOverride for non-blank drafts", () => {
    expect(previewFontFamily("Fira Code", "mono")).toBe(
      '"Fira Code", var(--font-mono-fallback)',
    );
  });
});
