// Locates the element that actually scrolls a HeroUI tab strip (F017).
//
// The strip nests three levels deep — strip > ListContainer > ScrollShadow >
// [role=tablist] — and only the ScrollShadow carries the horizontal overflow;
// the container our TabsList styles is not scrollable. HeroUI tags the
// scroller with data-slot="scroll-shadow" (scroll-shadow.js in
// @heroui/react), so that attribute is the handle.
//
// The lookup is intentionally soft: a renamed slot degrades to "wheel
// scrolling and reveal-on-activate stop working" instead of throwing at
// runtime. Re-check this after any HeroUI upgrade.

/** Scrolling element inside the tab strip rooted at `root`, or null when the
 * strip is not mounted / HeroUI no longer exposes the slot. */
export function findTabStripScroller(root: HTMLElement | null): HTMLElement | null {
  if (!root) return null;
  return root.querySelector<HTMLElement>('[data-slot="scroll-shadow"]');
}
