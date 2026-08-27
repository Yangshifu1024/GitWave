import { describe, expect, it } from "vitest";
import type { DiffSummary, FileChange, FileDiff } from "@/lib/api";
import { filterDiffSummary, partitionFileChanges } from "@/lib/diff";

function change(path: string, staged: boolean): FileChange {
  return {
    path,
    kind: "modified",
    staged,
    additions: 1,
    deletions: 0,
  };
}

function fileDiff(path: string, additions: number, deletions: number): FileDiff {
  return {
    path,
    old_sha: null,
    new_sha: null,
    additions,
    deletions,
    hunks: [],
  };
}

describe("partitionFileChanges", () => {
  it("splits staged and unstaged files", () => {
    const files = [change("a.ts", false), change("b.ts", true), change("c.ts", false)];
    expect(partitionFileChanges(files)).toEqual({
      unstaged: [files[0], files[2]],
      staged: [files[1]],
    });
  });

  it("returns empty lists when there are no files", () => {
    expect(partitionFileChanges([])).toEqual({ unstaged: [], staged: [] });
  });
});

describe("filterDiffSummary", () => {
  const diff: DiffSummary = {
    files: [fileDiff("a.ts", 3, 1), fileDiff("b.ts", 2, 4)],
    total_additions: 5,
    total_deletions: 5,
  };

  it("returns the original summary when path is omitted", () => {
    expect(filterDiffSummary(diff)).toBe(diff);
    expect(filterDiffSummary(diff, null)).toBe(diff);
  });

  it("keeps only the matching file and recomputes totals", () => {
    expect(filterDiffSummary(diff, "b.ts")).toEqual({
      files: [diff.files[1]],
      total_additions: 2,
      total_deletions: 4,
    });
  });

  it("returns an empty summary when the path is missing", () => {
    expect(filterDiffSummary(diff, "missing.ts")).toEqual({
      files: [],
      total_additions: 0,
      total_deletions: 0,
    });
  });
});
