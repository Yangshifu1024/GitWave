import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

// The ActionBar wrapper around <SyncStatusArea /> is pointer-events-none so
// a narrow window's card never blocks the sync buttons it overlaps, and
// pointer-events inherits to every descendant. Any interactive element the
// status card grows must therefore opt back in with pointer-events-auto —
// the cancel button missing that class is what made it unclickable
// (docs/tasks/fix-status-area-cancel-button/plan.md).
const actionBar = readFileSync(new URL("./ActionBar.tsx", import.meta.url), "utf8");
const statusArea = readFileSync(new URL("./SyncStatusArea.tsx", import.meta.url), "utf8");

describe("status area hit-testing", () => {
  it("keeps the cancel button clickable inside the pointer-events-none wrapper", () => {
    // Premise of the invariant: the wrapper still disables hit-testing.
    expect(actionBar).toContain("pointer-events-none");
    expect(statusArea).toContain("pointer-events-auto");
  });
});
