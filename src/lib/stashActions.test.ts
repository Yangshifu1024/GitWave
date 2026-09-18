import { describe, expect, it } from "vitest";
import type { FileDiff, StashEntry } from "@/lib/api";
import { fileDiffKind, stashFileSummary, stashLabel, stashTitle } from "@/lib/stashActions";

function file(path: string, overrides: Partial<FileDiff> = {}): FileDiff {
  return {
    path,
    old_sha: "old",
    new_sha: "new",
    additions: 1,
    deletions: 1,
    hunks: [],
    ...overrides,
  };
}

function entry(index: number, message: string): StashEntry {
  return { index, message, oid: `oid-${index}` };
}

describe("fileDiffKind", () => {
  it("derives added when the old side is missing", () => {
    expect(fileDiffKind(file("new.ts", { old_sha: null }))).toBe("added");
  });

  it("derives deleted when the new side is missing", () => {
    expect(fileDiffKind(file("gone.ts", { new_sha: null }))).toBe("deleted");
  });

  it("derives modified when both sides exist", () => {
    expect(fileDiffKind(file("edit.ts"))).toBe("modified");
  });

  it("prefers the untracked flag over the sha derivation", () => {
    // A stashed untracked file has an empty old side: without the flag it would
    // be reported as "added" (A), contradicting the row's "untracked" badge.
    expect(fileDiffKind(file("new.ts", { old_sha: null, untracked: true }))).toBe("untracked");
  });
});

describe("stashFileSummary", () => {
  it("keeps the full count but truncates the names to maxNames", () => {
    const files = ["a", "b", "c", "d", "e", "f", "g"].map((name) => file(`${name}.ts`));
    expect(stashFileSummary(files, 3)).toEqual({
      count: 7,
      names: ["a.ts", "b.ts", "c.ts"],
    });
  });

  it("defaults to five names", () => {
    const files = ["a", "b", "c", "d", "e", "f"].map((name) => file(`${name}.ts`));
    expect(stashFileSummary(files).names).toEqual(["a.ts", "b.ts", "c.ts", "d.ts", "e.ts"]);
  });

  it("handles an empty diff", () => {
    expect(stashFileSummary([])).toEqual({ count: 0, names: [] });
  });
});

describe("stash labels", () => {
  it("uses the git stash label", () => {
    expect(stashLabel(entry(2, "wip"))).toBe("stash@{2}");
  });

  it("falls back to the no-message label for an empty message", () => {
    expect(stashTitle(entry(0, ""), "(no message)")).toBe("stash@{0} · (no message)");
    expect(stashTitle(entry(1, "wip"), "(no message)")).toBe("stash@{1} · wip");
  });
});
