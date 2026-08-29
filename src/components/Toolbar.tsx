import { useEffect, useRef, useState } from "react";

import { AppMenuBar } from "@/components/AppMenuBar";
import { SettingsModal } from "@/components/SettingsModal";
import { AboutModal } from "@/components/AboutModal";
import { ToolbarAppTitle } from "@/components/ToolbarAppTitle";
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

export function Toolbar(): React.JSX.Element {
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
        "app-toolbar relative z-20 flex items-center shrink-0 h-10 gap-1.5",
        "bg-bg-primary",
        isMacOS() && "app-toolbar--macos",
      )}
    >
      <div
        ref={dragZoneRef}
        className="absolute inset-0 z-0"
        {...(!isMacOS() ? { "data-tauri-drag-region": true } : {})}
      />

      <div className="relative z-10 flex flex-1 min-w-0 items-center pointer-events-none">
        {!isMacOS() && <AppMenuBar onAbout={() => setAboutOpen(true)} />}
        {isMacOS() && <NativeAppMenu onAbout={() => setAboutOpen(true)} />}

        <ToolbarAppTitle />
      </div>

      <SettingsModal open={settingsOpen} onOpenChange={setSettingsOpen} />
      <AboutModal open={aboutOpen} onOpenChange={setAboutOpen} />
    </header>
  );
}
