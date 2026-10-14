import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";

/** Visual divider width in CSS px; the grab zone is the overlay in
 * ResizeHandle (±5px), independent of this value. */
const HANDLE_PX = 0.5;

interface ThreePaneLayoutProps {
  sidebar: ReactNode;
  main: ReactNode;
  inspector: ReactNode;
  /** Hide the center pane and expand inspector (inspector maximize). */
  inspectorMaximized?: boolean;
  initialSidebarWidth?: number;
  sidebarMin?: number;
  sidebarMax?: number;
  initialInspectorWidth?: number;
  inspectorMin?: number;
  inspectorMax?: number;
  mainMin?: number;
  inspectorClassName?: string;
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}

function useColumnDrag(onDelta: (delta: number) => void): (e: React.MouseEvent) => void {
  const cleanup = useRef<(() => void) | null>(null);
  useEffect(() => () => cleanup.current?.(), []);
  return useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      cleanup.current?.();
      const startX = e.clientX;
      const previousCursor = document.body.style.cursor;
      const previousUserSelect = document.body.style.userSelect;

      const onMove = (moveEvent: MouseEvent) => {
        // The callback captures the width at drag start. Use cumulative
        // movement so React batching cannot drop intermediate deltas.
        onDelta(moveEvent.clientX - startX);
      };

      const onUp = () => {
        document.removeEventListener("mousemove", onMove);
        document.removeEventListener("mouseup", onUp);
        document.body.style.cursor = previousCursor;
        document.body.style.userSelect = previousUserSelect;
        cleanup.current = null;
      };

      document.addEventListener("mousemove", onMove);
      document.addEventListener("mouseup", onUp);
      cleanup.current = onUp;
      document.body.style.cursor = "col-resize";
      document.body.style.userSelect = "none";
    },
    [onDelta],
  );
}

function ResizeHandle({
  onMouseDown,
  onDoubleClick,
  onDelta,
  label,
  value,
  min,
  max,
  className,
}: {
  onMouseDown: (e: React.MouseEvent) => void;
  onDoubleClick?: () => void;
  onDelta: (delta: number) => void;
  label: string;
  value: number;
  min: number;
  max: number;
  className?: string;
}): React.JSX.Element {
  return (
    // Fork-style split handle: the 1px line is decoration only; an invisible
    // overlay extending into both panes is the actual grab zone (Fitts's law
    // — a bare 1px target is effectively ungrabbable). Events on the overlay
    // bubble to this div.
    <div
      onMouseDown={onMouseDown}
      onDoubleClick={onDoubleClick}
      onKeyDown={(event) => {
        if (event.key === "ArrowLeft" || event.key === "ArrowRight") {
          event.preventDefault();
          onDelta((event.key === "ArrowLeft" ? -1 : 1) * (event.shiftKey ? 40 : 10));
        } else if (event.key === "Home") {
          event.preventDefault();
          onDoubleClick?.();
        }
      }}
      className={cn(
        // Invisible grab zone: the visible hairline is painted by the panes
        // themselves (pane-edge-left / pane-edge-right inset shadows), so
        // this div must stay transparent — anything painted here sits UNDER
        // the pane-edge shadows and never shows.
        "relative z-10 cursor-col-resize focus-visible:outline-2 focus-visible:outline-accent",
        className,
      )}
      style={{ width: HANDLE_PX }}
      role="separator"
      aria-orientation="vertical"
      aria-label={label}
      aria-valuenow={Math.round(value)}
      aria-valuemin={min}
      aria-valuemax={Math.round(max)}
      tabIndex={0}
    >
      <div aria-hidden className="absolute inset-y-0 -left-[5px] -right-[5px] cursor-col-resize" />
    </div>
  );
}

/**
 * Three-column shell: fixed sidebar + fluid center + fixed inspector.
 * Center column uses `1fr` so window maximize always fills without a gap.
 */
export function ThreePaneLayout({
  sidebar,
  main,
  inspector,
  inspectorMaximized = false,
  initialSidebarWidth = 240,
  sidebarMin = 200,
  sidebarMax = 480,
  initialInspectorWidth = 360,
  inspectorMin = 300,
  inspectorMax = 720,
  mainMin = 360,
  inspectorClassName,
}: ThreePaneLayoutProps): React.JSX.Element {
  const { t } = useTranslation();
  const containerRef = useRef<HTMLDivElement>(null);
  const [containerWidth, setContainerWidth] = useState<number | null>(null);
  const [preferredSidebarWidth, setSidebarWidth] = useState(initialSidebarWidth);
  const [preferredInspectorWidth, setInspectorWidth] = useState(initialInspectorWidth);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    const observer = new ResizeObserver(([entry]) => {
      if (entry && entry.contentRect.width > 0) setContainerWidth(entry.contentRect.width);
    });
    observer.observe(container);
    return () => observer.disconnect();
  }, []);

  // Preserve a useful history width even after dragging both side columns
  // wide or shrinking the window. Preferences survive a temporary resize.
  const availableWidth =
    containerWidth ?? preferredSidebarWidth + preferredInspectorWidth + mainMin + 2 * HANDLE_PX;
  const sidebarLimit = Math.max(
    sidebarMin,
    Math.min(
      sidebarMax,
      availableWidth - (inspectorMaximized ? inspectorMin : inspectorMin + mainMin + 2 * HANDLE_PX),
    ),
  );
  const sidebarWidth = clamp(preferredSidebarWidth, sidebarMin, sidebarLimit);
  const inspectorLimit = Math.max(
    inspectorMin,
    Math.min(inspectorMax, availableWidth - sidebarWidth - mainMin - 2 * HANDLE_PX),
  );
  const inspectorWidth = clamp(preferredInspectorWidth, inspectorMin, inspectorLimit);
  const dragSidebarOrigin = useCallback(
    (delta: number) => {
      const limit = Math.max(
        sidebarMin,
        Math.min(sidebarMax, availableWidth - inspectorWidth - mainMin - 2 * HANDLE_PX),
      );
      setSidebarWidth(clamp(sidebarWidth + delta, sidebarMin, limit));
    },
    [availableWidth, inspectorWidth, mainMin, sidebarMax, sidebarMin, sidebarWidth],
  );
  const dragInspectorOrigin = useCallback(
    (delta: number) => {
      setInspectorWidth(clamp(inspectorWidth - delta, inspectorMin, inspectorLimit));
    },
    [inspectorLimit, inspectorMin, inspectorWidth],
  );

  const onSidebarHandleDown = useColumnDrag(dragSidebarOrigin);
  const onInspectorHandleDown = useColumnDrag(dragInspectorOrigin);

  const gridTemplateColumns = inspectorMaximized
    ? `${sidebarWidth}px 1fr`
    : `${sidebarWidth}px ${HANDLE_PX}px minmax(${mainMin}px, 1fr) ${HANDLE_PX}px ${inspectorWidth}px`;

  return (
    <div
      ref={containerRef}
      className="grid h-full w-full min-w-0 overflow-hidden"
      style={{ gridTemplateColumns }}
    >
      <div className="min-w-0 min-h-0 h-full overflow-hidden">{sidebar}</div>

      {inspectorMaximized ? null : (
        <>
          <ResizeHandle
            onMouseDown={onSidebarHandleDown}
            onDoubleClick={() => setSidebarWidth(initialSidebarWidth)}
            onDelta={dragSidebarOrigin}
            label={t("app.layout.resizeSidebar")}
            value={sidebarWidth}
            min={sidebarMin}
            max={sidebarLimit}
          />
          <div className="min-w-0 min-h-0 h-full overflow-hidden">{main}</div>
          <ResizeHandle
            onMouseDown={onInspectorHandleDown}
            onDoubleClick={() => setInspectorWidth(initialInspectorWidth)}
            onDelta={dragInspectorOrigin}
            label={t("app.layout.resizeInspector")}
            value={inspectorWidth}
            min={inspectorMin}
            max={inspectorLimit}
          />
        </>
      )}

      <div
        className={cn(
          "min-w-0 min-h-0 h-full overflow-hidden transition-[box-shadow] duration-base",
          inspectorClassName,
        )}
      >
        {inspector}
      </div>
    </div>
  );
}
