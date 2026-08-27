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
    staged: null,
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

  it("matches paths regardless of slash direction", () => {
    const windowsDiff: DiffSummary = {
      files: [fileDiff("src\\lib.rs", 1, 0)],
      total_additions: 1,
      total_deletions: 0,
    };
    expect(filterDiffSummary(windowsDiff, "src/lib.rs").files).toHaveLength(1);
  });

  it("keeps only the requested working-copy side when a path is on both", () => {
    const mixed: DiffSummary = {
      files: [
        { ...fileDiff("a.ts", 4, 0), staged: true },
        { ...fileDiff("a.ts", 1, 2), staged: false },
      ],
      total_additions: 5,
      total_deletions: 2,
    };
    expect(filterDiffSummary(mixed, "a.ts", true)).toEqual({
      files: [mixed.files[0]],
      total_additions: 4,
      total_deletions: 0,
    });
    expect(filterDiffSummary(mixed, "a.ts", false)).toEqual({
      files: [mixed.files[1]],
      total_additions: 1,
      total_deletions: 2,
    });
  });
});
