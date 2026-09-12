/**
 * Parser for git merge-conflict markers in a file's text content.
 *
 * The backend (`ConflictSides`) returns whole-file contents without hunk
 * positions, so the conflict panel derives them here — the same way editors
 * like VSCode do, by scanning for line-start markers:
 *
 *   `<<<<<<<` (ours) … optional `|||||||` (base, diff3 style) …
 *   `=======` … `>>>>>>>` (theirs)
 */

/** A conflict region as 0-based line indices, both inclusive. */
export interface ConflictRegion {
  /** Line index of the `<<<<<<<` marker. */
  start: number;
  /**
   * Line index of the `>>>>>>>` marker — or, when the region is unterminated
   * (user mid-edit), the last line of the text.
   */
  end: number;
  /** True when a closing `>>>>>>>` marker was found for this region. */
  closed: boolean;
}

/**
 * Exactly 7 marker chars followed by whitespace or end-of-line. Longer runs
 * (`>>>>>>>>>>` in C++ sources) are content, not markers — libgit2 only
 * recognizes 7. `\s` (not a literal space) keeps bare markers working in
 * CRLF files, whose lines end with `\r` after splitting on `\n`.
 */
const OURS_MARKER = /^<{7}(?:\s|$)/;
const THEIRS_MARKER = /^>{7}(?:\s|$)/;
/** diff3-style base separator between ours and base content. */
const BASE_MARKER = /^\|{7}(?:\s|$)/;
/** `\r?` so the separator still classifies in CRLF files (split on "\n"). */
const SEPARATOR = /^={7}\r?$/;

/** Which conflict marker (if any) this line starts with. */
export type ConflictMarkerKind = "ours" | "separator" | "base" | "theirs";

export function classifyConflictLine(line: string): ConflictMarkerKind | null {
  if (OURS_MARKER.test(line)) return "ours";
  if (THEIRS_MARKER.test(line)) return "theirs";
  if (SEPARATOR.test(line)) return "separator";
  if (BASE_MARKER.test(line)) return "base";
  return null;
}

/**
 * Find all conflict regions in `text`. Markers only count at the start of a
 * line (indented occurrences are content). An unclosed `<<<<<<<` region runs
 * to the end of the text so it still counts and highlights while editing.
 */
export function findConflictRegions(text: string): ConflictRegion[] {
  const lines = text.split("\n");
  const regions: ConflictRegion[] = [];
  let start: number | null = null;

  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i];
    if (line === undefined) break;
    const kind = classifyConflictLine(line);
    if (start === null) {
      if (kind === "ours") start = i;
    } else if (kind === "theirs") {
      regions.push({ start, end: i, closed: true });
      start = null;
    }
  }

  if (start !== null) {
    regions.push({ start, end: lines.length - 1, closed: false });
  }
  return regions;
}

/**
 * Character offset of every line's start in `text` (plus a trailing
 * `text.length` sentinel). Compute once and index per region instead of
 * calling {@link lineStartOffset} per region (O(regions × lines)).
 */
export function lineStartOffsets(text: string): number[] {
  const lines = text.split("\n");
  const offsets: number[] = new Array<number>(lines.length + 1);
  let offset = 0;
  for (let i = 0; i < lines.length; i += 1) {
    offsets[i] = offset;
    offset += (lines[i]?.length ?? 0) + 1; // +1 for the "\n"
  }
  // A line at/after EOF sits at the end of the text (no trailing newline).
  offsets[lines.length] = text.length;
  return offsets;
}

/** 0-based line index → character offset of that line's start in `text`. */
export function lineStartOffset(text: string, line: number): number {
  const starts = lineStartOffsets(text);
  return starts[Math.min(line, starts.length - 1)] ?? text.length;
}
