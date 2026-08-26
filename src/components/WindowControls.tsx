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

import { Minus, Square, X } from "lucide-react";
import { getCurrentWindow } from "@tauri-apps/api/window";

import { Button } from "@/components/ui/Button";
import { isMacOS } from "@/lib/platform";

export function WindowControls(): React.JSX.Element | null {
  if (isMacOS()) {
    return null;
  }

  const win = getCurrentWindow();

  function minimize(): void {
    void win.minimize();
  }
  function toggleMaximize(): void {
    void win.toggleMaximize();
  }
  function close(): void {
    void win.close();
  }

  return (
    <div className="flex items-center gap-0.5" aria-label="Window controls">
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={minimize}
        aria-label="Minimize"
        className="p-1 h-7 w-8"
      >
        <Minus size={14} />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        onClick={toggleMaximize}
        aria-label="Maximize"
        className="p-1 h-7 w-8"
      >
        <Square size={12} />
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