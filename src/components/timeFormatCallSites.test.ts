// Call-site guard for docs/tasks/refactor-unify-time-format:
// the whole repo formats absolute times through src/lib/commitTime.ts, so each
// screen used to carry its own `toLocaleDateString` / `toLocaleString` call with
// its own options. Divergent options are invisible in review and wrong cache
// granularity silently collapses different times of one day into one label.
// These are static source assertions — timezone- and locale-independent.

import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");

function readSource(relativePath: string): string {
  return readFileSync(path.join(repoRoot, relativePath), "utf8");
}

const COMPONENTS = [
  "src/components/BranchList.tsx",
  "src/components/ReflogPanel.tsx",
  "src/components/BlameView.tsx",
  "src/components/CommitInfoHeader.tsx",
];

describe("time format call sites", () => {
  for (const file of COMPONENTS) {
    it(`routes ${file} through the shared formatter`, () => {
      const source = readSource(file);
      expect(source).not.toContain("toLocaleDateString(");
      expect(source).not.toContain("toLocaleString(");
      expect(source).toContain("formatAbsoluteTime(");
    });
  }

  it("keeps BranchList's relative-time ladder on the shared helper", () => {
    // BranchList carried a full copy of formatCommitTime's four buckets.
    expect(readSource("src/components/BranchList.tsx")).toContain("formatCommitTime(");
  });

  it("declares every absolute style in one table", () => {
    const source = readSource("src/lib/commitTime.ts");
    expect(source).toContain("ABSOLUTE_STYLES");
    expect(source).toContain("formatAbsoluteTime(");
    // The cache key must carry both the style and the locale: a time-only key
    // hands one call site the label of another (or of another language).
    expect(source).toContain('`${style}|${locale ?? ""}|${stamp}`');
  });

  it("still keys the cache by local calendar day, never a UTC bucket", () => {
    const source = readSource("src/lib/commitTime.ts");
    expect(source).toMatch(/getFullYear\(\)/);
    expect(source).toMatch(/getMonth\(\)/);
    expect(source).toMatch(/getDate\(\)/);
    expect(source).not.toMatch(/Math\.floor\(\s*time\s*\/\s*86400\s*\)/);
  });
});
