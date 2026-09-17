// Mouse-wheel horizontal scrolling for the repository tab strip (F017).
//
// A vertical wheel carries no horizontal component, so an overflowing strip
// never moves on a plain mouse wheel — only trackpad slides, Shift+wheel and
// HeroUI's chevrons do. This maps the vertical gesture to the strip's
// horizontal scroller.

import { useEffect, type RefObject } from "react";

import { findTabStripScroller } from "@/hooks/tabStripScroller";
import { clampScrollLeft, wheelScrollDelta } from "@/lib/horizontalWheel";

/**
 * `containerRef` is the strip wrapper (the element that holds the tabs), not
 * the scrolling node — the scroller is resolved per event.
 *
 * A native listener, deliberately not React's `onWheel`: React registers wheel
 * handlers as passive, which makes `preventDefault()` a no-op (and logs a
 * console warning).
 *
 * While the pointer is over the strip the gesture is always consumed, ends
 * included — the strip owns the wheel there by design (F017 decision), so
 * reaching an edge does not hand the scroll back to whatever sits behind.
 */
export function useHorizontalWheelScroll(
  containerRef: RefObject<HTMLElement | null>,
  enabled: boolean,
): void {
  useEffect(() => {
    const root = containerRef.current;
    if (!root || !enabled) return;

    const onWheel = (event: WheelEvent): void => {
      const scroller = findTabStripScroller(root);
      if (!scroller) return;
      // A trackpad's own horizontal gesture keeps native momentum; a zero
      // delta has nothing to apply.
      const delta = wheelScrollDelta(event, scroller.clientWidth);
      if (delta === 0) return;
      event.preventDefault();
      const max = scroller.scrollWidth - scroller.clientWidth;
      scroller.scrollLeft = clampScrollLeft(scroller.scrollLeft, delta, max);
    };

    root.addEventListener("wheel", onWheel, { passive: false });
    return () => root.removeEventListener("wheel", onWheel);
  }, [containerRef, enabled]);
}
