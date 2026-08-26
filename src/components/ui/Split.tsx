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

interface PaneContextValue {
  paneId: string;
  initialSize: number | string;
  minSize: number;
  maxSize: number;
}

const PaneContext = createContext<PaneContextValue | null>(null);

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
    <PaneContext.Provider value={{ paneId, initialSize, minSize, maxSize }}>
      <div
        ref={ref}
        data-pane-id={paneId}
        className={cn("overflow-hidden min-w-0 min-h-0", className)}
        style={{ flexBasis, flexGrow: 0, flexShrink: 1 }}
      >
        {children}
      </div>
    </PaneContext.Provider>
  );
}

/**
 * A draggable resize handle between panes.
 * Double-click resets to initial size.
 */
export function ResizeHandle({ className }: ResizeHandleProps): React.JSX.Element {
  const ctx = useContext(SplitContext);
  const handleId = useRef(generateId()).current;
  const paneContext = useContext(PaneContext);
  const containerRef = useRef<HTMLDivElement | null>(null);

  const handleMouseDown = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      e.preventDefault();
      ctx?.startDrag(handleId);

      const startX = e.clientX;
      const startY = e.clientY;

      const paneEl = e.currentTarget.previousElementSibling as HTMLDivElement | null;
      if (!paneEl) return;

      const startWidth = paneEl.offsetWidth;
      const startHeight = paneEl.offsetHeight;

      const onMouseMove = (moveEvent: MouseEvent) => {
        const dx = moveEvent.clientX - startX;
        const dy = moveEvent.clientY - startY;

        // Determine orientation from parent split
        const splitEl = paneEl.closest("[data-split-direction]");
        const isHorizontal = splitEl?.getAttribute("data-split-direction") !== "vertical";

        if (isHorizontal) {
          const newWidth = Math.max(
            paneContext?.minSize ?? 100,
            Math.min(paneContext?.maxSize ?? Infinity, startWidth + dx),
          );
          paneEl.style.flexBasis = `${newWidth}px`;
          paneEl.style.flexShrink = "0";
          paneEl.style.flexGrow = "0";
        } else {
          const newHeight = Math.max(
            paneContext?.minSize ?? 100,
            Math.min(paneContext?.maxSize ?? Infinity, startHeight + dy),
          );
          paneEl.style.flexBasis = `${newHeight}px`;
          paneEl.style.flexShrink = "0";
          paneEl.style.flexGrow = "0";
        }
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
      document.body.style.cursor = "col-resize";
      document.body.style.userSelect = "none";
    },
    [ctx, handleId, paneContext],
  );

  const handleDoubleClick = useCallback(() => {
    const paneEl = document.querySelector<HTMLDivElement>(
      `[data-pane-id="${paneContext?.paneId}"]`,
    );
    if (!paneEl || paneContext == null) return;
    const basis =
      typeof paneContext.initialSize === "number"
        ? `${paneContext.initialSize}px`
        : paneContext.initialSize;
    paneEl.style.flexBasis = basis;
    paneEl.style.flexShrink = "1";
    paneEl.style.flexGrow = "0";
  }, [paneContext]);

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
      className={cn(
        "shrink-0 bg-border-subtle transition-colors",
        "hover:bg-accent",
        ctx?.isDragging && ctx?.dragHandleId === handleId && "bg-accent",
        className,
      )}
      style={{
        width: direction === "horizontal" ? 4 : undefined,
        height: direction === "vertical" ? 4 : undefined,
        cursor: "col-resize",
      }}
      role="separator"
      aria-orientation={direction === "horizontal" ? "vertical" : "horizontal"}
      tabIndex={0}
    />
  );
}
