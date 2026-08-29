// Pointer-based drag-to-reorder for a flat tab strip (F005).
//
// No dnd library: the Repository Tab strip is a single horizontal row, so a
// small state machine covers it — press → move past threshold → live order
// preview → release commits. Pointer events (not HTML5 DnD) because React
// Aria owns the tab's press/selection behavior; a DRAG_THRESHOLD_PX offset
// keeps click-to-switch and right-click intact, and `suppressClickRef` lets
// the owner swallow the selection a drag release would otherwise trigger.

import { useRef, useState } from "react";

/** Pointer must move this far (px) before a press becomes a drag. */
export const DRAG_THRESHOLD_PX = 6;

/** Move `from` to index `to`; returns a new array, input untouched. */
export function arrayMove<T>(list: readonly T[], from: number, to: number): T[] {
  const next = [...list];
  const [item] = next.splice(from, 1);
  if (item !== undefined) next.splice(to, 0, item);
  return next;
}

/** Index the item under pointer `x` should move to, judged by tab midpoints
 * across the full row (the dragged tab included). */
export function computeTargetIndex(
  x: number,
  rects: readonly { left: number; width: number }[],
): number {
  for (let i = 0; i < rects.length; i++) {
    const rect = rects[i];
    if (rect && x < rect.left + rect.width / 2) return i;
  }
  return rects.length - 1;
}

/** Order `items` by `order` (id list). Ids missing from `order` keep their
 * relative input order at the end — safe when a refetch lands mid-drag. */
export function applyOrder<T extends { id: string }>(
  items: readonly T[],
  order: readonly string[],
): T[] {
  const rank = new Map(order.map((id, i) => [id, i]));
  return [...items].sort((a, b) => {
    const ra = rank.get(a.id);
    const rb = rank.get(b.id);
    if (ra === undefined && rb === undefined) return 0;
    if (ra === undefined) return 1;
    if (rb === undefined) return -1;
    return ra - rb;
  });
}

interface PressState {
  pointerId: number;
  id: string;
  startX: number;
  startY: number;
  /** Id order at press time — a release back onto it commits nothing. */
  originOrder: string[];
}

export interface TabDragApi {
  /** Attach to every reorderable tab. */
  handlePointerDown: (event: React.PointerEvent<HTMLElement>, id: string) => void;
  /** Id of the tab currently dragged, for visual treatment. */
  draggingId: string | null;
  /** True from drag engage until after React Aria's press handler for the
   * release has run — check it in tab selection to ignore the pick a drag
   * release would otherwise cause. */
  suppressClickRef: React.RefObject<boolean>;
}

export function useTabDragReorder(options: {
  /** Element whose `[role="tab"]` descendants form the reorderable strip. */
  containerRef: React.RefObject<HTMLElement | null>;
  /** Ids in the currently rendered order (preview included). */
  ids: readonly string[];
  /** Live preview: called with the new id order whenever the target slot
   * changes. */
  onPreview: (ids: string[]) => void;
  /** Drag released onto a different order — commit the last previewed one. */
  onCommit: (ids: string[]) => void;
  /** Drag ended without a commit (pointercancel, or released back on the
   * original order): the owner should drop any preview it is rendering. */
  onAbort?: () => void;
}): TabDragApi {
  const { containerRef, ids, onPreview, onCommit, onAbort } = options;
  const [draggingId, setDraggingId] = useState<string | null>(null);
  const pressRef = useRef<PressState | null>(null);
  const orderRef = useRef(ids);
  orderRef.current = ids;
  const suppressClickRef = useRef(false);

  const sameOrder = (a: readonly string[], b: readonly string[]): boolean =>
    a.length === b.length && a.every((id, i) => id === b[i]);

  const handlePointerDown = (event: React.PointerEvent<HTMLElement>, id: string): void => {
    // One drag at a time: a second pointerdown (multi-touch, touchpad +
    // mouse) must not steal an in-flight press's listeners.
    if (pressRef.current) return;
    if (event.button !== 0) return;
    const press: PressState = {
      pointerId: event.pointerId,
      id,
      startX: event.clientX,
      startY: event.clientY,
      originOrder: [...orderRef.current],
    };
    pressRef.current = press;
    let engaged = false;

    const releaseSuppression = (): void => {
      // React Aria's press-select for this release runs synchronously in the
      // same event dispatch; clear only after it had its chance.
      setTimeout(() => {
        suppressClickRef.current = false;
      }, 0);
    };

    const onMove = (ev: PointerEvent): void => {
      if (ev.pointerId !== press.pointerId) return;
      if (!engaged) {
        const dist = Math.hypot(ev.clientX - press.startX, ev.clientY - press.startY);
        if (dist < DRAG_THRESHOLD_PX) return;
        engaged = true;
        setDraggingId(press.id);
        suppressClickRef.current = true;
      }
      const container = containerRef.current;
      if (!container) return;
      const rects = Array.from(container.querySelectorAll<HTMLElement>('[role="tab"]')).map((el) =>
        el.getBoundingClientRect(),
      );
      const order = orderRef.current;
      // Rects and ids must describe the same row; skip a frame if a render
      // is still in flight.
      if (rects.length !== order.length) return;
      const from = order.indexOf(press.id);
      if (from < 0) return;
      const to = computeTargetIndex(ev.clientX, rects);
      if (to === from) return;
      const next = arrayMove(order, from, to);
      orderRef.current = next;
      onPreview(next);
    };

    const onUp = (ev: PointerEvent): void => {
      if (ev.pointerId !== press.pointerId) return;
      cleanup();
      if (engaged) {
        // Dragged out and back onto the original order: nothing to persist.
        if (sameOrder(orderRef.current, press.originOrder)) onAbort?.();
        else onCommit([...orderRef.current]);
      }
      releaseSuppression();
    };

    const onCancel = (ev: PointerEvent): void => {
      if (ev.pointerId !== press.pointerId) return;
      cleanup();
      onAbort?.();
      releaseSuppression();
    };

    const cleanup = (): void => {
      pressRef.current = null;
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onUp);
      window.removeEventListener("pointercancel", onCancel);
      setDraggingId(null);
    };

    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onUp);
    window.addEventListener("pointercancel", onCancel);
  };

  return { handlePointerDown, draggingId, suppressClickRef };
}
