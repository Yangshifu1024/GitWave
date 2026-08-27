import { invoke } from "@tauri-apps/api/core";
import { getCurrentWindow } from "@tauri-apps/api/window";
import { useEffect, type RefObject } from "react";

import { isMacOS } from "@/lib/platform";

/**
 * macOS titlebar: manual drag + instant zoom.
 *
 * Avoid `data-tauri-drag-region` on macOS — Tauri injects drag.js which calls
 * `internal_toggle_maximize` (animated AppKit zoom) on double-click and races
 * with our instant frame toggle (tauri#13898).
 */
export function useMacTitlebarWindow(dragZoneRef: RefObject<HTMLElement | null>): void {
  useEffect(() => {
    if (!isMacOS()) return;

    const zone = dragZoneRef.current;
    if (!zone) return;

    let initialX = 0;
    let initialY = 0;

    const onMouseDown = (event: MouseEvent): void => {
      if (event.button !== 0) return;

      if (event.detail === 2) {
        initialX = event.clientX;
        initialY = event.clientY;
        return;
      }

      if (event.detail === 1) {
        event.preventDefault();
        void getCurrentWindow()
          .startDragging()
          .catch(() => undefined);
      }
    };

    const onMouseUp = (event: MouseEvent): void => {
      if (event.button !== 0 || event.detail !== 2) return;
      if (event.clientX !== initialX || event.clientY !== initialY) return;

      event.preventDefault();
      void invoke("toggle_instant_zoom").catch(() => undefined);
    };

    zone.addEventListener("mousedown", onMouseDown);
    zone.addEventListener("mouseup", onMouseUp);
    return () => {
      zone.removeEventListener("mousedown", onMouseDown);
      zone.removeEventListener("mouseup", onMouseUp);
    };
  }, [dragZoneRef]);
}
