import { useCallback, useState, type ReactNode } from "react";
import { cn } from "@/lib/utils";

const HANDLE_PX = 1;

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
  return useCallback(
    (e: React.MouseEvent) => {
      e.preventDefault();
      let lastX = e.clientX;

      const onMove = (moveEvent: MouseEvent) => {
        const delta = moveEvent.clientX - lastX;
        lastX = moveEvent.clientX;
        onDelta(delta);
      };

      const onUp = () => {
        document.removeEventListener("mousemove", onMove);
        document.removeEventListener("mouseup", onUp);
        document.body.style.cursor = "";
        document.body.style.userSelect = "";
      };

      document.addEventListener("mousemove", onMove);
      document.addEventListener("mouseup", onUp);
      document.body.style.cursor = "col-resize";
      document.body.style.userSelect = "none";
    },
    [onDelta],
  );
}

function ResizeHandle({
  onMouseDown,
  onDoubleClick,
  className,
}: {
  onMouseDown: (e: React.MouseEvent) => void;
  onDoubleClick?: () => void;
  className?: string;
}): React.JSX.Element {
  return (
    <div
      onMouseDown={onMouseDown}
      onDoubleClick={onDoubleClick}
      className={cn("bg-border-subtle cursor-col-resize", className)}
      style={{ width: HANDLE_PX }}
      role="separator"
      aria-orientation="vertical"
      tabIndex={0}
    />
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
  initialSidebarWidth = 320,
  sidebarMin = 320,
  sidebarMax = 480,
  initialInspectorWidth = 500,
  inspectorMin = 360,
  inspectorMax = 720,
  mainMin = 280,
  inspectorClassName,
}: ThreePaneLayoutProps): React.JSX.Element {
  const [sidebarWidth, setSidebarWidth] = useState(initialSidebarWidth);
  const [inspectorWidth, setInspectorWidth] = useState(initialInspectorWidth);
  const dragSidebarOrigin = useCallback(
    (delta: number) => {
      setSidebarWidth((width) => clamp(width + delta, sidebarMin, sidebarMax));
    },
    [sidebarMax, sidebarMin],
  );
  const dragInspectorOrigin = useCallback(
    (delta: number) => {
      setInspectorWidth((width) => clamp(width - delta, inspectorMin, inspectorMax));
    },
    [inspectorMax, inspectorMin],
  );

  const onSidebarHandleDown = useColumnDrag(dragSidebarOrigin);
  const onInspectorHandleDown = useColumnDrag(dragInspectorOrigin);

  const gridTemplateColumns = inspectorMaximized
    ? `${sidebarWidth}px 1fr`
    : `${sidebarWidth}px ${HANDLE_PX}px minmax(${mainMin}px, 1fr) ${HANDLE_PX}px ${inspectorWidth}px`;

  return (
    <div className="grid h-full w-full min-w-0 overflow-hidden" style={{ gridTemplateColumns }}>
      <div className="min-w-0 min-h-0 h-full overflow-hidden">{sidebar}</div>

      {inspectorMaximized ? null : (
        <>
          <ResizeHandle
            onMouseDown={onSidebarHandleDown}
            onDoubleClick={() => setSidebarWidth(initialSidebarWidth)}
          />
          <div className="min-w-0 min-h-0 h-full overflow-hidden">{main}</div>
          <ResizeHandle
            onMouseDown={onInspectorHandleDown}
            onDoubleClick={() => setInspectorWidth(initialInspectorWidth)}
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
