import { useCallback, useMemo, useSyncExternalStore } from "react";

/** Fixed-height lists expose immutable render data instead of a mutable
 * virtualizer instance, so both manual and compiler memoization stay valid. */
export function useFixedVirtualizer({
  count,
  rowHeight,
  overscan,
  element,
}: {
  count: number;
  rowHeight: number;
  overscan: number;
  element: HTMLDivElement | null;
}) {
  const subscribe = useCallback(
    (notify: () => void) => {
      if (!element) return () => undefined;
      element.addEventListener("scroll", notify, { passive: true });
      const observer = new ResizeObserver(notify);
      observer.observe(element);
      return () => {
        element.removeEventListener("scroll", notify);
        observer.disconnect();
      };
    },
    [element],
  );
  const getSnapshot = useCallback(
    () => `${element?.scrollTop ?? 0}:${element?.clientHeight ?? 0}`,
    [element],
  );
  const viewport = useSyncExternalStore(subscribe, getSnapshot, () => "0:0");
  const [offset = 0, height = 0] = viewport.split(":").map(Number);
  const totalSize = count * rowHeight;
  const items = useMemo(() => {
    if (height <= 0 || count === 0) return [];
    const first = Math.max(0, Math.min(count - 1, Math.floor(offset / rowHeight)) - overscan);
    const end = Math.min(count, Math.ceil((offset + height) / rowHeight) + overscan);
    return Array.from({ length: Math.max(0, end - first) }, (_, i) => ({
      index: first + i,
      key: first + i,
      start: (first + i) * rowHeight,
      size: rowHeight,
    }));
  }, [count, rowHeight, overscan, offset, height]);
  const scrollToIndex = useCallback(
    (index: number, { align = "start" }: { align?: "start" | "center" } = {}) => {
      if (!element || count === 0) return;
      const target = Math.max(0, Math.min(count - 1, index)) * rowHeight;
      const top = align === "center" ? target - (element.clientHeight - rowHeight) / 2 : target;
      element.scrollTo({ top: Math.max(0, Math.min(top, totalSize - element.clientHeight)) });
    },
    [element, count, rowHeight, totalSize],
  );
  return { items, totalSize, scrollToIndex };
}
