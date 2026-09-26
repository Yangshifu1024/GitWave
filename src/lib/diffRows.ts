import type { DiffHunk, DiffLine } from "./api";

export type DiffRow =
  { header: string } | { left?: DiffLine; right?: DiffLine; side?: "left" | "right" };

/** Pair entire replacement blocks, including unequal additions/deletions. */
export function diffRows(hunks: DiffHunk[], split: boolean): DiffRow[] {
  const rows: DiffRow[] = [];
  for (const hunk of hunks) {
    rows.push({
      header: `@@ -${hunk.old_start},${hunk.old_lines} +${hunk.new_start},${hunk.new_lines} @@`,
    });
    for (let i = 0; i < hunk.lines.length;) {
      const line = hunk.lines[i]!;
      if (line.kind === "context") {
        rows.push({ left: line, right: line });
        i++;
        continue;
      }
      const removed: DiffLine[] = [],
        added: DiffLine[] = [];
      while (i < hunk.lines.length && hunk.lines[i]!.kind !== "context") {
        const next = hunk.lines[i++]!;
        (next.kind === "removed" ? removed : added).push(next);
      }
      if (split) {
        for (let j = 0; j < Math.max(removed.length, added.length); j++)
          rows.push({ left: removed[j], right: added[j] });
      } else {
        removed.forEach((left, j) => rows.push({ left, right: added[j], side: "left" }));
        added.forEach((right, j) => rows.push({ right, left: removed[j], side: "right" }));
      }
    }
  }
  return rows;
}

/** UTF-16 offsets used by JSX slicing; do not split surrogate pairs. */
export function changedSpan(
  before: string,
  after: string,
  side: "left" | "right",
): [number, number] {
  const a = Array.from(before),
    b = Array.from(after);
  let start = 0,
    endA = a.length,
    endB = b.length;
  while (start < endA && start < endB && a[start] === b[start]) start++;
  while (endA > start && endB > start && a[endA - 1] === b[endB - 1]) {
    endA--;
    endB--;
  }
  const parts = side === "left" ? a : b;
  return [
    parts.slice(0, start).join("").length,
    parts.slice(0, side === "left" ? endA : endB).join("").length,
  ];
}
