import { describe, expect, it } from "vitest";

import { mergeUniquePaths } from "./paths";

describe("mergeUniquePaths", () => {
  it("appends incoming paths in order", () => {
    expect(mergeUniquePaths([], ["/a", "/b"])).toEqual(["/a", "/b"]);
  });

  it("keeps first-seen order and drops duplicates against existing and within incoming", () => {
    expect(mergeUniquePaths(["/a"], ["/b", "/a", "/b", "/c"])).toEqual(["/a", "/b", "/c"]);
  });

  it("trims and drops empty entries", () => {
    expect(mergeUniquePaths([], ["  /a  ", "", "   "])).toEqual(["/a"]);
  });

  it("does not mutate the input array", () => {
    const existing = ["/a"];
    const result = mergeUniquePaths(existing, ["/b"]);
    expect(existing).toEqual(["/a"]);
    expect(result).toEqual(["/a", "/b"]);
  });
});
