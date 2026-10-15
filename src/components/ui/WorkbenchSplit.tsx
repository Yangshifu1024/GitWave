import { type ReactNode, useRef, useState } from "react";

interface WorkbenchSplitProps {
  id: string;
  label: string;
  direction?: "horizontal" | "vertical";
  initial?: number;
  min?: number;
  max?: number;
  collapsed?: boolean;
  first: ReactNode;
  second: ReactNode;
}

/** Two-pane desktop split. Pointer capture also cleans up interrupted drags on unmount. */
export function WorkbenchSplit({
  id,
  label,
  direction = "horizontal",
  initial = 25,
  min = 15,
  max = 60,
  collapsed = false,
  first,
  second,
}: WorkbenchSplitProps): React.JSX.Element {
  const container = useRef<HTMLDivElement>(null);
  const clamp = (value: number) => Math.max(min, Math.min(max, value));
  const [ratio, setRatio] = useState(() => {
    try {
      const value = Number(localStorage.getItem(`gitwave.split.${id}`) ?? initial);
      return Number.isFinite(value) ? clamp(value) : initial;
    } catch {
      return initial;
    }
  });
  const resize = (value: number) => {
    const next = clamp(value);
    setRatio(next);
    try {
      localStorage.setItem(`gitwave.split.${id}`, String(next));
    } catch {
      /* Session only. */
    }
  };
  const horizontal = direction === "horizontal";
  const template = collapsed
    ? "minmax(0, 1fr)"
    : `minmax(0, ${ratio}fr) 5px minmax(0, ${100 - ratio}fr)`;
  return (
    <div
      ref={container}
      className="grid h-full min-h-0 min-w-0 overflow-hidden"
      style={horizontal ? { gridTemplateColumns: template } : { gridTemplateRows: template }}
    >
      <div hidden={collapsed} className="min-h-0 min-w-0 overflow-hidden">
        {first}
      </div>
      {!collapsed && (
        <div
          role="separator"
          tabIndex={0}
          aria-label={label}
          aria-orientation={horizontal ? "vertical" : "horizontal"}
          aria-valuemin={min}
          aria-valuemax={max}
          aria-valuenow={Math.round(ratio)}
          className={`workbench-separator ${horizontal ? "cursor-col-resize" : "cursor-row-resize"}`}
          onDoubleClick={() => resize(initial)}
          onKeyDown={(event) => {
            const previous = horizontal ? "ArrowLeft" : "ArrowUp";
            const next = horizontal ? "ArrowRight" : "ArrowDown";
            if (![previous, next, "Home", "End"].includes(event.key)) return;
            event.preventDefault();
            resize(
              event.key === "Home"
                ? initial
                : event.key === "End"
                  ? max
                  : ratio + (event.key === previous ? -1 : 1) * (event.shiftKey ? 10 : 2),
            );
          }}
          onPointerDown={(event) => {
            event.preventDefault();
            event.currentTarget.setPointerCapture(event.pointerId);
          }}
          onPointerMove={(event) => {
            if (!event.currentTarget.hasPointerCapture(event.pointerId)) return;
            const rect = container.current?.getBoundingClientRect();
            if (!rect) return;
            const size = horizontal ? rect.width : rect.height;
            if (size > 0)
              resize(
                ((horizontal ? event.clientX - rect.left : event.clientY - rect.top) / size) * 100,
              );
          }}
          onPointerUp={(event) => {
            if (event.currentTarget.hasPointerCapture(event.pointerId))
              event.currentTarget.releasePointerCapture(event.pointerId);
          }}
        />
      )}
      <div className="min-h-0 min-w-0 overflow-hidden">{second}</div>
    </div>
  );
}
