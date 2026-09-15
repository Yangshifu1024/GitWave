// Custom-drawn window titlebar (tauri-plugin-decoration): the only top row.
//
// It hosts the app menu (in-app AppMenuBar on Windows / Linux, the native
// system menu on macOS) plus whatever operation controls the app puts on the
// bar — ActionBar passes them as `children`. Keeping the shell presentational
// lets ActionBar own its operation state/dialogs without a second top row.
//
// Platform chrome avoidance:
// - macOS native traffic lights are floated over the bar; `.app-toolbar--macos`
//   reserves their width via padding-left.
// - Windows / Linux window controls are HTML controls the plugin overlays at
//   the bar's trailing edge; the plugin publishes a right clearance that
//   `.app-toolbar` pads by. Both clearances collapse in fullscreen.
// The drag surface lives *behind* the content (z-0): the content layer is
// pointer-events-none so gaps stay draggable, and every interactive group
// opts back in with pointer-events-auto.

import { useEffect, useRef, useState } from "react";

import { AppMenuBar } from "@/components/AppMenuBar";
import { SettingsModal } from "@/components/SettingsModal";
import { AboutModal } from "@/components/AboutModal";
import { useRefreshRepo } from "@/hooks/useAutoRefresh";
import { useMacTitlebarWindow } from "@/hooks/useMacTitlebarWindow";
import { useNativeAppMenu } from "@/hooks/useNativeAppMenu";
import { isMacOS } from "@/lib/platform";
import { cn } from "@/lib/utils";
import { useUiStore } from "@/stores/uiStore";

// macOS-only headless bridge: mounting it conditionally keeps
// useNativeAppMenu — and the gating subscriptions it holds — off
// Windows / Linux, where the in-app AppMenuBar owns the menus.
function NativeAppMenu({ onAbout }: { onAbout: () => void }): null {
  useNativeAppMenu({ onAbout });
  return null;
}

export function TitleBar({
  children,
  overlay,
}: {
  /** Operation controls (ActionBar) laid out after the menu. */
  children: React.ReactNode;
  /** Absolutely positioned layer centered on the window (sync status area). */
  overlay?: React.ReactNode;
}): React.JSX.Element {
  // Settings state lives in the ui store so global shortcuts (Cmd+,) and
  // the command palette can open it from anywhere.
  const settingsOpen = useUiStore((s) => s.settingsOpen);
  const setSettingsOpen = useUiStore((s) => s.setSettingsOpen);
  const [aboutOpen, setAboutOpen] = useState(false);
  const dragZoneRef = useRef<HTMLDivElement>(null);
  useMacTitlebarWindow(dragZoneRef);

  // App-standard "open settings" shortcut (same cross-platform modifier
  // check as CommitMessageBox). On macOS the native app menu carries the
  // ⌘, accelerator and the system consumes the key before the webview, so
  // this listener normally never fires there — it stays registered on all
  // platforms as the fallback for the (logged) case where the native menu
  // failed to install. setSettingsOpen(true) is idempotent, so even if both
  // paths ever fired, Settings just opens once.
  //
  // ⌘R / Ctrl+R refreshes repository data (same action as auto refresh;
  // surfaced in the status area). Safe to claim: the native app menu that
  // used to reload the webview is gone.
  const refreshRepo = useRefreshRepo();
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent): void => {
      if ((e.metaKey || e.ctrlKey) && e.key === ",") {
        e.preventDefault();
        setSettingsOpen(true);
      } else if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "r") {
        if (e.repeat) return;
        e.preventDefault();
        refreshRepo();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [setSettingsOpen, refreshRepo]);

  return (
    <header
      className={cn(
        "app-toolbar relative z-20 flex items-center shrink-0 h-10",
        "bg-bg-primary border-b border-border-subtle select-none",
        isMacOS() && "app-toolbar--macos",
      )}
    >
      <div
        ref={dragZoneRef}
        className="absolute inset-0 z-0"
        {...(!isMacOS() ? { "data-tauri-drag-region": true } : {})}
      />

      {/* `@container`: children can collapse secondary labels via `@6xl:` —
          the bar is the width authority, not the viewport. */}
      <div className="@container relative z-10 flex flex-1 min-w-0 items-center gap-3 pointer-events-none">
        {!isMacOS() && <AppMenuBar onAbout={() => setAboutOpen(true)} />}
        {isMacOS() && <NativeAppMenu onAbout={() => setAboutOpen(true)} />}
        {children}
      </div>

      {overlay}

      <SettingsModal open={settingsOpen} onOpenChange={setSettingsOpen} />
      <AboutModal open={aboutOpen} onOpenChange={setAboutOpen} />
    </header>
  );
}
