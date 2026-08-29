import { describe, expect, it } from "vitest";

import { applyOrder, arrayMove, computeTargetIndex } from "./useTabDragReorder";

describe("arrayMove", () => {
  it("moves an item forward", () => {
    expect(arrayMove(["a", "b", "c", "d"], 0, 2)).toEqual(["b", "c", "a", "d"]);
  });

  it("moves an item backward", () => {
    expect(arrayMove(["a", "b", "c", "d"], 3, 1)).toEqual(["a", "d", "b", "c"]);
  });

  it("does not mutate the input", () => {
    const source = ["a", "b", "c"];
    arrayMove(source, 0, 2);
    expect(source).toEqual(["a", "b", "c"]);
  });
});

describe("computeTargetIndex", () => {
  const rects = [
    { left: 0, width: 100 },
    { left: 100, width: 100 },
    { left: 200, width: 100 },
  ];

  it("targets the tab whose midpoint the pointer passed", () => {
    expect(computeTargetIndex(40, rects)).toBe(0);
    expect(computeTargetIndex(149, rects)).toBe(1);
  });

  it("passes to the next tab once its midpoint is crossed", () => {
    expect(computeTargetIndex(160, rects)).toBe(2);
  });

  it("targets the last tab past its midpoint", () => {
    expect(computeTargetIndex(299, rects)).toBe(2);
    expect(computeTargetIndex(10_000, rects)).toBe(2);
  });
});

describe("applyOrder", () => {
  const repos = [
    { id: "a", path: "/a" },
    { id: "b", path: "/b" },
    { id: "c", path: "/c" },
  ];

  it("reorders items to match the id order", () => {
    expect(applyOrder(repos, ["c", "a", "b"]).map((r) => r.id)).toEqual(["c", "a", "b"]);
  });

  it("appends items missing from the order, keeping relative order", () => {
    const withNew = [...repos, { id: "d", path: "/d" }];
    expect(applyOrder(withNew, ["c", "a"]).map((r) => r.id)).toEqual(["c", "a", "b", "d"]);
  });

  it("falls back to input order for an empty order", () => {
    expect(applyOrder(repos, []).map((r) => r.id)).toEqual(["a", "b", "c"]);
  });
});
