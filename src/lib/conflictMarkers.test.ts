import { describe, expect, it } from "vitest";
import { classifyConflictLine, findConflictRegions, lineStartOffset } from "./conflictMarkers";

describe("classifyConflictLine", () => {
  it("classifies the four marker kinds", () => {
    expect(classifyConflictLine("<<<<<<< HEAD")).toBe("ours");
    expect(classifyConflictLine("=======")).toBe("separator");
    expect(classifyConflictLine("||||||| base")).toBe("base");
    expect(classifyConflictLine(">>>>>>> branch")).toBe("theirs");
  });

  it("accepts a CR terminator on the separator (CRLF files)", () => {
    expect(classifyConflictLine("=======\r")).toBe("separator");
  });

  it("returns null for content and indented markers", () => {
    expect(classifyConflictLine("plain content")).toBeNull();
    expect(classifyConflictLine("  <<<<<<< indented")).toBeNull();
    expect(classifyConflictLine("======= underline")).toBeNull();
  });
});

describe("findConflictRegions", () => {
  it("returns empty for text without markers", () => {
    expect(findConflictRegions("")).toEqual([]);
    expect(findConflictRegions("just\nplain\nlines")).toEqual([]);
  });

  it("finds a single region", () => {
    const text = "a\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\nb\n";
    expect(findConflictRegions(text)).toEqual([{ start: 1, end: 5, closed: true }]);
  });

  it("finds multiple regions", () => {
    const text = [
      "<<<<<<< HEAD",
      "o1",
      "=======",
      "t1",
      ">>>>>>> x",
      "middle",
      "<<<<<<< HEAD",
      "o2",
      "=======",
      "t2",
      ">>>>>>> y",
    ].join("\n");
    expect(findConflictRegions(text)).toEqual([
      { start: 0, end: 4, closed: true },
      { start: 6, end: 10, closed: true },
    ]);
  });

  it("counts an unterminated region up to the last line", () => {
    const text = "a\n<<<<<<< HEAD\nours\nstill ours";
    expect(findConflictRegions(text)).toEqual([{ start: 1, end: 3, closed: false }]);
  });

  it("keeps diff3 base separators inside the region", () => {
    const text = "<<<<<<< HEAD\nours\n||||||| base\nbase content\n=======\ntheirs\n>>>>>>> x";
    expect(findConflictRegions(text)).toEqual([{ start: 0, end: 6, closed: true }]);
  });

  it("ignores indented markers", () => {
    const text = "code\n  <<<<<<< not a marker\n >>>>>>> also not\n<<<<<<< HEAD\nreal\n>>>>>>> x";
    expect(findConflictRegions(text)).toEqual([{ start: 3, end: 5, closed: true }]);
  });

  it("ignores markers inside a region until the real closing one", () => {
    const text = "<<<<<<< HEAD\nours\n>>>>>>\ntheirs\n>>>>>>> x";
    // `>>>>>>` (6 chars) is not a marker; region ends at the 7-char one.
    expect(findConflictRegions(text)).toEqual([{ start: 0, end: 4, closed: true }]);
  });
});

describe("lineStartOffset", () => {
  it("computes character offsets for line indices", () => {
    const text = "ab\ncd\nef";
    expect(lineStartOffset(text, 0)).toBe(0);
    expect(lineStartOffset(text, 1)).toBe(3);
    expect(lineStartOffset(text, 2)).toBe(6);
  });

  it("clamps to text length for out-of-range lines", () => {
    expect(lineStartOffset("ab", 5)).toBe(2);
  });
});
