// Keeps the active repository tab inside the visible strip (F017).
//
// The strip hides its scrollbar, so a selection that lands off-screen (many
// repos, a workspace switch, the last repo of a multi-add batch becoming
// active) is otherwise invisible. Scrolls only the strip's own scroller —
// never `scrollIntoView`, which would also drag ancestor scroll containers.

import { useEffect, type RefObject } from "react";

import { findTabStripScroller } from "@/hooks/tabStripScroller";
import { clampScrollLeft, scrollLeftToReveal } from "@/lib/horizontalWheel";

export interface RevealActiveTabOptions {
  /** The strip wrapper (same node the wheel hook takes). */
  containerRef: RefObject<HTMLElement | null>;
  activeWorkspaceId: string | null;
  activeRepoId: string | null;
  /** Rendered tab count: the reveal must also run when a late-arriving repo
   * list renders tabs for an already-restored selection. */
  tabCount: number;
}

export function useRevealActiveTab({
  containerRef,
  activeWorkspaceId,
  activeRepoId,
  tabCount,
}: RevealActiveTabOptions): void {
  useEffect(() => {
    if (!activeWorkspaceId || !activeRepoId) return;
    const root = containerRef.current;
    const scroller = findTabStripScroller(root);
    if (!root || !scroller) return;
    // The selected tab is React Aria's own output (`data-selected` is what the
    // tab's styling keys off), so no per-tab DOM attribute is needed here.
    const tab = root.querySelector<HTMLElement>('[role="tab"][data-selected="true"]');
    if (!tab) return;

    const scrollerRect = scroller.getBoundingClientRect();
    const tabRect = tab.getBoundingClientRect();
    // Offsets relative to the scrolled content, not to the viewport.
    const itemLeft = tabRect.left - scrollerRect.left + scroller.scrollLeft;
    const target = scrollLeftToReveal(
      scroller.scrollLeft,
      scroller.clientWidth,
      itemLeft,
      itemLeft + tabRect.width,
    );
    if (target === scroller.scrollLeft) return;

    const max = scroller.scrollWidth - scroller.clientWidth;
    scroller.scrollTo({
      left: clampScrollLeft(scroller.scrollLeft, target - scroller.scrollLeft, max),
      behavior: "smooth",
    });
  }, [containerRef, activeWorkspaceId, activeRepoId, tabCount]);
}
