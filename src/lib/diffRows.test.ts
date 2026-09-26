import { describe, expect, it } from "vitest";
import { changedSpan, diffRows } from "./diffRows";
import type { DiffHunk, DiffLine } from "./api";
const line = (kind: DiffLine["kind"], content: string): DiffLine => ({
  kind,
  content,
  old_line_no: kind === "added" ? null : 1,
  new_line_no: kind === "removed" ? null : 1,
});
describe("diff replacement blocks", () => {
  it("pairs unequal runs without losing blank lines", () => {
    const hunk: DiffHunk = {
      old_start: 1,
      old_lines: 3,
      new_start: 1,
      new_lines: 2,
      lines: [
        line("removed", "a"),
        line("removed", ""),
        line("removed", "c"),
        line("added", "A"),
        line("added", ""),
      ],
    };
    const rows = diffRows([hunk], true);
    expect(rows.slice(1)).toEqual([
      { left: hunk.lines[0], right: hunk.lines[3] },
      { left: hunk.lines[1], right: hunk.lines[4] },
      { left: hunk.lines[2], right: undefined },
    ]);
    expect(diffRows([hunk], false)).toHaveLength(6);
  });
  it("marks complete Unicode characters", () => {
    expect(changedSpan("x😀z", "x😁z", "left")).toEqual([1, 3]);
    expect(changedSpan("same", "same", "right")).toEqual([4, 4]);
  });
});
