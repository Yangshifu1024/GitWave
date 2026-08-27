import { describe, expect, it } from "vitest";
import { nextFileSelection, pathsInRange } from "@/lib/fileSelection";

const ordered = ["a.ts", "b.ts", "c.ts", "d.ts"];

describe("pathsInRange", () => {
  it("returns an inclusive range in either direction", () => {
    expect(pathsInRange(ordered, "b.ts", "d.ts")).toEqual(["b.ts", "c.ts", "d.ts"]);
    expect(pathsInRange(ordered, "d.ts", "b.ts")).toEqual(["b.ts", "c.ts", "d.ts"]);
  });

  it("returns a single path when anchor and target are the same", () => {
    expect(pathsInRange(ordered, "c.ts", "c.ts")).toEqual(["c.ts"]);
  });
});

describe("nextFileSelection", () => {
  it("replace selects only the clicked path and moves the anchor", () => {
    expect(
      nextFileSelection(ordered, new Set(["a.ts", "b.ts"]), "c.ts", "replace", "a.ts"),
    ).toEqual({ selected: new Set(["c.ts"]), anchor: "c.ts" });
  });

  it("toggle adds or removes a path without moving the anchor", () => {
    expect(nextFileSelection(ordered, new Set(["a.ts"]), "c.ts", "toggle", "a.ts")).toEqual({
      selected: new Set(["a.ts", "c.ts"]),
      anchor: "a.ts",
    });
    expect(nextFileSelection(ordered, new Set(["a.ts", "c.ts"]), "c.ts", "toggle", "a.ts")).toEqual(
      { selected: new Set(["a.ts"]), anchor: "a.ts" },
    );
  });

  it("range selects from the anchor to the clicked path", () => {
    expect(nextFileSelection(ordered, new Set(["a.ts"]), "c.ts", "range", "a.ts")).toEqual({
      selected: new Set(["a.ts", "b.ts", "c.ts"]),
      anchor: "a.ts",
    });
  });
});
