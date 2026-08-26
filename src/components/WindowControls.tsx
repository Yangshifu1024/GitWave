// WindowControls — minimize / maximize / close buttons backed by
// Tauri 2's getCurrentWindow() API.
//
// Cross-platform behavior:
//   - macOS: returns null. The OS provides traffic lights in the
//     top-left; we leave a corresponding pl-20 on the topbar so the
//     Workspace selector is not obscured. Adding custom buttons here
//     would violate macOS HIG.
//   - Windows / Linux: renders three ghost buttons on the right side
//     of the topbar (no native window controls).

import { useEffect, useState } from "react";
import { Copy, Minus, Square, X } from "lucide-react";
import { getCurrentWindow } from "@tauri-apps/api/window";

import { Button } from "@/components/ui/Button";
import { isMacOS } from "@/lib/platform";
import { cn } from "@/lib/utils";

/** Hover that contrasts with topbar `bg-bg-secondary`. */
const CONTROL_HOVER = "hover:bg-bg-elevated";

export function WindowControls(): React.JSX.Element | null {
  if (isMacOS()) {
    return null;
  }

  return <WinLinuxControls />;
}

function WinLinuxControls(): React.JSX.Element {
  const [maximized, setMaximized] = useState(false);

  useEffect(() => {
    const win = getCurrentWindow();
    let cancelled = false;
    let unlisten: (() => void) | undefined;

    void win.isMaximized().then((value) => {
      if (!cancelled) setMaximized(value);
    });

    void win
      .onResized(() => {
        void win.isMaximized().then((value) => {
          if (!cancelled) setMaximized(value);
        });
      })
      .then((fn) => {
        if (cancelled) {
          fn();
          return;
        }
        unlisten = fn;
      });

    return () => {
      cancelled = true;
      unlisten?.();
    };
  }, []);

  function minimize(): void {
    void getCurrentWindow().minimize();
  }
  function toggleMaximize(): void {
    const win = getCurrentWindow();
    void win.toggleMaximize().then(() => void win.isMaximized().then(setMaximized));
  }
  function close(): void {
    void getCurrentWindow().close();
  }

  return (
    <div className="flex items-center gap-0.5" aria-label="Window controls">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={minimize}
        aria-label="Minimize"
        className={cn("p-1 h-7 w-8", CONTROL_HOVER)}
      >
        <Minus size={14} />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={toggleMaximize}
        aria-label={maximized ? "Restore" : "Maximize"}
        className={cn("p-1 h-7 w-8", CONTROL_HOVER)}
      >
        {maximized ? <Copy size={12} className="-scale-x-100" /> : <Square size={12} />}
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={close}
        aria-label="Close"
        className="p-1 h-7 w-8 hover:bg-danger hover:text-text-inverse"
      >
        <X size={14} />
      </Button>
    </div>
  );
}
