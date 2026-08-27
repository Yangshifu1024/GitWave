import { describe, expect, it } from "vitest";
import { resolveLocateIndex } from "./commitLocate";

const sha = "a".repeat(40);
const shaToIndex = new Map<string, number>([[sha, 3]]);

describe("resolveLocateIndex", () => {
  it("resolves the index for a fresh request in the active repo", () => {
    expect(resolveLocateIndex({ repoId: "r1", sha, seq: 1 }, -1, "r1", shaToIndex)).toBe(3);
  });

  it("returns null for an already handled request (one-shot per seq)", () => {
    expect(resolveLocateIndex({ repoId: "r1", sha, seq: 1 }, 1, "r1", shaToIndex)).toBe(null);
  });

  it("returns null when the request targets another repo", () => {
    const request = { repoId: "r1", sha, seq: 1 };
    expect(resolveLocateIndex(request, -1, "r2", shaToIndex)).toBe(null);
    expect(resolveLocateIndex(request, -1, null, shaToIndex)).toBe(null);
  });

  it("returns null when the commit is outside the loaded log window", () => {
    expect(
      resolveLocateIndex({ repoId: "r1", sha: "b".repeat(40), seq: 1 }, -1, "r1", shaToIndex),
    ).toBe(null);
  });

  it("returns null without a request", () => {
    expect(resolveLocateIndex(null, -1, "r1", shaToIndex)).toBe(null);
  });
});
