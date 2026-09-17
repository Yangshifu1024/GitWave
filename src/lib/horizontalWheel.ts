// Wheel-to-horizontal arithmetic behind the repository tab strip
// (docs/tasks/feat-repo-tab-wheel-scroll/plan.md). Pure on purpose: the DOM
// wiring lives in useHorizontalWheelScroll / useRevealActiveTab, so every
// decision here is unit-tested without a DOM.

/** Fallback line height for `deltaMode === 1` wheel events. Firefox reports
 * line-based deltas; Chromium (WebView2 included) reports pixels, and pages
 * only appear on some trackpad drivers. */
export const WHEEL_LINE_PX = 16;

export interface WheelLike {
  deltaX: number;
  deltaY: number;
  deltaMode: number;
}

/**
 * Horizontal scroll delta in pixels for one wheel event.
 *
 * Returns 0 when the gesture already carries a horizontal component — a
 * trackpad two-finger slide must stay with the browser's native (and
 * momentum-aware) horizontal scrolling instead of being applied twice.
 */
export function wheelScrollDelta(event: WheelLike, viewportWidth: number): number {
  if (event.deltaX !== 0) return 0;
  switch (event.deltaMode) {
    case 1:
      return event.deltaY * WHEEL_LINE_PX;
    case 2:
      return event.deltaY * viewportWidth;
    default:
      return event.deltaY;
  }
}

/** `current + delta` clamped into the scroll range [0, max]; a non-positive
 * range collapses to 0 (nothing to scroll). */
export function clampScrollLeft(current: number, delta: number, max: number): number {
  const upper = Math.max(0, max);
  return Math.min(upper, Math.max(0, current + delta));
}

/**
 * Scroll position that brings the item spanning `[itemLeft, itemRight)` fully
 * into the viewport `[current, current + viewportWidth)`.
 *
 * Returns `current` when the item is already visible, so activating a tab that
 * is on screen never nudges the strip. An item that starts left of the
 * viewport aligns its left edge; one that runs past the right edge aligns its
 * right edge (for an item wider than the viewport the left edge wins, matching
 * "show me the start of the thing I picked").
 */
export function scrollLeftToReveal(
  current: number,
  viewportWidth: number,
  itemLeft: number,
  itemRight: number,
): number {
  if (viewportWidth <= 0) return current;
  if (itemLeft < current) return itemLeft;
  if (itemRight > current + viewportWidth) return itemRight - viewportWidth;
  return current;
}
