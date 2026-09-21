// Render-path guards for the macOS commit-history scroll fix
// (docs/tasks/fix-history-scroll-perf): the virtualizer re-renders its whole
// window on every scroll frame, so a row that re-renders unconditionally (or a
// lazy menu body that does ICU work per row) reintroduces the blank/stuttering
// scroll. These are static source assertions — cheap, and they fail loudly if
// someone drops the memoisation or re-inlines the time formatting.

import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

function readSource(relativePath: string): string {
  return readFileSync(path.join(repoRoot, relativePath), "utf8");
}

describe("CommitGraph render guards", () => {
  const source = readSource("src/components/CommitGraph.tsx");

  it("memoises both row components", () => {
    expect(source).toContain("React.memo(function GraphRow");
    expect(source).toContain("React.memo(function CommitRow");
  });

  it("keeps a widened overscan window", () => {
    const args = /useVirtualizer\(\{([\s\S]*?)\}\)/.exec(source)?.[1] ?? "";
    expect(args).not.toBe("");
    expect(args).toMatch(/overscan:\s*14/);
    expect(args).not.toMatch(/overscan:\s*10\b/);
  });

  it("contains layout only for the virtual row wrapper", () => {
    // Anchor the positive assertion to the row wrapper's own style block — the one
    // carrying `transform: translateY(...)`. A bare `toContain` anywhere in the file
    // would still pass if someone moved `contain` up onto the sizer.
    const wrapperStart = source.indexOf("transform: `translateY(");
    expect(wrapperStart).toBeGreaterThan(-1);
    const wrapperEnd = source.indexOf("}}", wrapperStart);
    expect(wrapperEnd).toBeGreaterThan(wrapperStart);
    expect(source.slice(wrapperStart, wrapperEnd)).toContain('contain: "layout"');
    // Exactly one containment declaration in the file: the row wrapper, nowhere else.
    expect(source.match(/contain: "layout"/g) ?? []).toHaveLength(1);
    // Paint containment clips the row SVG's stroke overflow / horizontal bleed;
    // it stays off until it can be A/B'd on a real Mac.
    expect(source).not.toContain('contain: "layout paint"');
  });

  it("keeps the sizer out of its own compositor layer", () => {
    // `will-change: transform` on the sizer was dropped in review: promoting a
    // many-thousand-px element is an unverifiable memory risk on WKWebView.
    expect(source).not.toContain("willChange");
    // The source keeps a "No will-change here on purpose" comment, so the dashed
    // form is checked against comment-stripped source rather than the raw text.
    const codeOnly = source
      .split("\n")
      .filter((line) => !line.trimStart().startsWith("//"))
      .join("\n");
    expect(codeOnly).not.toMatch(/will-?change/i);
  });

  it("delegates time formatting to the cached lib helper", () => {
    expect(source).toContain("formatCommitTime(");
    expect(source).not.toContain("toLocaleDateString(");
  });
});

describe("ContextMenu anchor mount guard", () => {
  it("mounts the invisible anchor only while the menu is open", () => {
    // The Popover body/Trigger stay mounted so the HeroUI exit animation can run,
    // but the body-level fixed anchor only exists while the menu is open.
    const source = readSource("src/components/ui/ContextMenu.tsx");
    expect(source).toMatch(/const mountAnchor = ctx\.isOpen;/);
    // An early `return null` — bare, brace-wrapped, split over several lines, any
    // spacing — unmounts the Popover itself and kills the HeroUI exit animation for
    // all six consumers. (`[!]` instead of `\!`: no-useless-escape rejects the bare
    // escape in a regex literal.)
    expect(source).not.toMatch(/if\s*\(\s*[!]ctx\.isOpen\s*\)\s*\{?\s*return\s+null/);
    // The anchor must stay a *conditional* portal: prettier breaks the ternary over
    // two lines, so normalise whitespace before matching.
    expect(source.replace(/\s+/g, " ")).toMatch(/\{mountAnchor \? createPortal\(/);
  });
});

describe("App commit-select stability guard", () => {
  it("hands CommitGraph a stable handleCommitSelect", () => {
    // A new onCommitSelect identity re-renders the whole virtual window, which
    // silently defeats the row memoisation above.
    const source = readSource("src/App.tsx");
    expect(source).toContain("const handleCommitSelect = useCallback(");
    expect(source).toContain("onCommitSelect={handleCommitSelect}");
  });
});

describe("commit date cache key guard", () => {
  it("keys the date cache by local calendar day, not a UTC bucket", () => {
    // The behavioural assertions for this live in src/lib/commitTime.test.ts, but on a
    // UTC host (CI defaults to TZ=UTC) local day == UTC day, so those assertions pass
    // for a UTC-day bucket too. This source guard is timezone-independent: it fails on
    // the old key whatever TZ the host runs in.
    const source = readSource("src/lib/commitTime.ts");
    // The key must be built from the local calendar components.
    expect(source).toMatch(/getFullYear\(\)/);
    expect(source).toMatch(/getMonth\(\)/);
    expect(source).toMatch(/getDate\(\)/);
    // Must not fall back to a UTC day bucket as the cache key. (The daysAgo bucket
    // divides `diff`, not `time`, and is intentionally out of scope here.)
    expect(source).not.toMatch(/Math\.floor\(\s*time\s*\/\s*86400\s*\)/);
  });
});
