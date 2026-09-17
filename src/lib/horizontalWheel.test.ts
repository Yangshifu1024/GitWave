import { describe, expect, it } from "vitest";

import {
  WHEEL_LINE_PX,
  clampScrollLeft,
  scrollLeftToReveal,
  wheelScrollDelta,
} from "@/lib/horizontalWheel";

describe("wheelScrollDelta", () => {
  it("passes pixel deltas through unchanged", () => {
    expect(wheelScrollDelta({ deltaX: 0, deltaY: 120, deltaMode: 0 }, 800)).toBe(120);
    expect(wheelScrollDelta({ deltaX: 0, deltaY: -120, deltaMode: 0 }, 800)).toBe(-120);
  });

  it("scales line deltas (Firefox) by the fallback line height", () => {
    expect(wheelScrollDelta({ deltaX: 0, deltaY: 3, deltaMode: 1 }, 800)).toBe(3 * WHEEL_LINE_PX);
  });

  it("scales page deltas by the viewport width", () => {
    expect(wheelScrollDelta({ deltaX: 0, deltaY: 1, deltaMode: 2 }, 800)).toBe(800);
  });

  it("leaves trackpad horizontal gestures to the browser", () => {
    expect(wheelScrollDelta({ deltaX: 40, deltaY: 90, deltaMode: 0 }, 800)).toBe(0);
    expect(wheelScrollDelta({ deltaX: -40, deltaY: 0, deltaMode: 0 }, 800)).toBe(0);
  });

  it("returns 0 for a gesture with no vertical component", () => {
    expect(wheelScrollDelta({ deltaX: 0, deltaY: 0, deltaMode: 0 }, 800)).toBe(0);
  });
});

describe("clampScrollLeft", () => {
  it("adds the delta while inside the range", () => {
    expect(clampScrollLeft(100, 50, 400)).toBe(150);
  });

  it("clamps at the start and at the end", () => {
    expect(clampScrollLeft(20, -100, 400)).toBe(0);
    expect(clampScrollLeft(380, 100, 400)).toBe(400);
  });

  it("stays put on an exact bound", () => {
    expect(clampScrollLeft(0, 0, 400)).toBe(0);
    expect(clampScrollLeft(400, 0, 400)).toBe(400);
  });

  it("collapses to 0 when there is nothing to scroll", () => {
    expect(clampScrollLeft(0, 120, 0)).toBe(0);
    expect(clampScrollLeft(0, -120, -50)).toBe(0);
  });
});

describe("scrollLeftToReveal", () => {
  it("keeps the current position when the item is already visible", () => {
    expect(scrollLeftToReveal(100, 400, 200, 260)).toBe(100);
  });

  it("keeps the current position when the item sits exactly on both edges", () => {
    expect(scrollLeftToReveal(100, 400, 100, 500)).toBe(100);
  });

  it("aligns the left edge of an item that starts before the viewport", () => {
    expect(scrollLeftToReveal(300, 400, 120, 180)).toBe(120);
  });

  it("aligns the right edge of an item that runs past the viewport", () => {
    expect(scrollLeftToReveal(100, 400, 460, 520)).toBe(120);
  });

  it("shows the start of an item wider than the viewport", () => {
    expect(scrollLeftToReveal(300, 200, 250, 700)).toBe(250);
  });

  it("no-ops on a degenerate viewport", () => {
    expect(scrollLeftToReveal(150, 0, 10, 20)).toBe(150);
  });
});
