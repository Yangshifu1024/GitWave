import { type ReactNode, createContext, useCallback, useContext, useRef, useState } from "react";
import { cn } from "@/lib/utils";

interface SplitContextValue {
  isDragging: boolean;
  dragHandleId: string | null;
  registerHandle: (id: string) => void;
  unregisterHandle: () => void;
  startDrag: (handleId: string) => void;
  stopDrag: () => void;
}

const SplitContext = createContext<SplitContextValue | null>(null);

interface PaneProps {
  /** Pixel width/height, or a CSS size such as `"20%"`. */
  initialSize: number | string;
  minSize?: number;
  maxSize?: number;
  children: ReactNode;
  className?: string;
}

interface SplitProps {
  direction?: "horizontal" | "vertical";
  children: ReactNode;
}

interface ResizeHandleProps {
  className?: string;
}

function generateId(): string {
  return `pane-${Math.random().toString(36).slice(2, 9)}`;
}

/**
 * Horizontal or vertical Split container.
 * Contains Pane and ResizeHandle children.
 */
export function Split({ direction = "horizontal", children }: SplitProps): React.JSX.Element {
  const [isDragging, setIsDragging] = useState(false);
  const [activeHandleId, setActiveHandleId] = useState<string | null>(null);
  const handleIdRef = useRef<string | null>(null);

  const registerHandle = useCallback((id: string) => {
    handleIdRef.current = id;
  }, []);

  const unregisterHandle = useCallback(() => {
    handleIdRef.current = null;
  }, []);

  const startDrag = useCallback((id: string) => {
    setIsDragging(true);
    setActiveHandleId(id);
  }, []);

  const stopDrag = useCallback(() => {
    setIsDragging(false);
    setActiveHandleId(null);
  }, []);

  return (
    <SplitContext.Provider
      value={{
        isDragging,
        dragHandleId: activeHandleId,
        registerHandle,
        unregisterHandle,
        startDrag,
        stopDrag,
      }}
    >
      <div
        className={cn("flex h-full w-full", direction === "vertical" && "flex-col")}
        data-split-direction={direction}
      >
        {children}
      </div>
    </SplitContext.Provider>
  );
}

/**
 * A resizable pane within a Split.
 */
export function Pane({
  initialSize,
  minSize = 100,
  maxSize = Infinity,
  children,
  className,
}: PaneProps): React.JSX.Element {
  const paneId = useRef(generateId()).current;
  const ref = useRef<HTMLDivElement>(null);
  const flexBasis = typeof initialSize === "number" ? `${initialSize}px` : initialSize;

  return (
      <div
        ref={ref}
        data-pane-id={paneId}
        data-pane-initial={flexBasis}
        data-pane-min={minSize}
        data-pane-max={Number.isFinite(maxSize) ? maxSize : undefined}
        className={cn("overflow-hidden min-w-0 min-h-0 h-full", className)}
        style={{ flexBasis, flexGrow: 0, flexShrink: 1 }}
      >
        {children}
      </div>
  );
}

/**
 * A draggable resize handle between panes.
 * Double-click resets to initial size.
 */
export function ResizeHandle({ className }: ResizeHandleProps): React.JSX.Element {
  const ctx = useContext(SplitContext);
  const handleId = useRef(generateId()).current;
  const containerRef = useRef<HTMLDivElement | null>(null);

  const handleMouseDown = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      e.preventDefault();
      ctx?.startDrag(handleId);

      const startX = e.clientX;
      const startY = e.clientY;

      // ResizeHandle is a sibling of Pane (not a child) — walk DOM, not PaneContext.
      const prevEl = e.currentTarget.previousElementSibling as HTMLDivElement | null;
      const nextEl = e.currentTarget.nextElementSibling as HTMLDivElement | null;
      if (!prevEl || !nextEl) return;

      const splitEl = prevEl.closest("[data-split-direction]");
      const isHorizontal = splitEl?.getAttribute("data-split-direction") !== "vertical";

      const prevStart = isHorizontal ? prevEl.offsetWidth : prevEl.offsetHeight;
      const nextStart = isHorizontal ? nextEl.offsetWidth : nextEl.offsetHeight;
      const prevMin = Number(prevEl.dataset.paneMin ?? 100);
      const nextMin = Number(nextEl.dataset.paneMin ?? 100);
      const prevMax = prevEl.dataset.paneMax ? Number(prevEl.dataset.paneMax) : Infinity;
      const nextMax = nextEl.dataset.paneMax ? Number(nextEl.dataset.paneMax) : Infinity;

      const onMouseMove = (moveEvent: MouseEvent) => {
        const delta = isHorizontal ? moveEvent.clientX - startX : moveEvent.clientY - startY;
        // Grow/shrink the pair together so free space is never left empty (grow:0 trap).
        let prevSize = prevStart + delta;
        let nextSize = nextStart - delta;
        prevSize = Math.max(prevMin, Math.min(prevMax, prevSize));
        nextSize = prevStart + nextStart - prevSize;
        if (nextSize < nextMin) {
          nextSize = nextMin;
          prevSize = prevStart + nextStart - nextSize;
        } else if (nextSize > nextMax) {
          nextSize = nextMax;
          prevSize = prevStart + nextStart - nextSize;
        }
        prevSize = Math.max(prevMin, Math.min(prevMax, prevSize));

        prevEl.style.flexBasis = `${prevSize}px`;
        nextEl.style.flexBasis = `${nextSize}px`;
        prevEl.style.flexShrink = "0";
        nextEl.style.flexShrink = "0";
        prevEl.style.flexGrow = "0";
        nextEl.style.flexGrow = "0";
      };

      const onMouseUp = () => {
        ctx?.stopDrag();
        document.removeEventListener("mousemove", onMouseMove);
        document.removeEventListener("mouseup", onMouseUp);
        document.body.style.cursor = "";
        document.body.style.userSelect = "";
      };

      document.addEventListener("mousemove", onMouseMove);
      document.addEventListener("mouseup", onMouseUp);
      document.body.style.cursor = isHorizontal ? "col-resize" : "row-resize";
      document.body.style.userSelect = "none";
    },
    [ctx, handleId],
  );

  const handleDoubleClick = useCallback((e: React.MouseEvent<HTMLDivElement>) => {
    const resetPane = (el: Element | null) => {
      if (!(el instanceof HTMLDivElement) || !el.dataset.paneId) return;
      const initial = el.dataset.paneInitial;
      if (!initial) return;
      el.style.flexBasis = initial;
      el.style.flexShrink = "1";
      el.style.flexGrow = "0";
    };
    resetPane(e.currentTarget.previousElementSibling);
    resetPane(e.currentTarget.nextElementSibling);
  }, []);

  // Determine direction from closest Split context
  const splitEl = containerRef.current?.closest("[data-split-direction]");
  const direction =
    (splitEl?.getAttribute("data-split-direction") as "horizontal" | "vertical") ?? "horizontal";

  return (
    <div
      ref={containerRef}
      data-handle-id={handleId}
      onMouseDown={handleMouseDown}
      onDoubleClick={handleDoubleClick}
      className={cn("shrink-0 bg-border-subtle", className)}
      style={{
        width: direction === "horizontal" ? 2 : undefined,
        height: direction === "vertical" ? 2 : undefined,
        cursor: direction === "horizontal" ? "col-resize" : "row-resize",
      }}
      role="separator"
      aria-orientation={direction === "horizontal" ? "vertical" : "horizontal"}
      tabIndex={0}
    />
  );
}
