import { useEffect, useRef, useState } from "react";

import { AppMenuBar } from "@/components/AppMenuBar";
import { SettingsModal } from "@/components/SettingsModal";
import { AboutModal } from "@/components/AboutModal";
import { ToolbarAppTitle } from "@/components/ToolbarAppTitle";
import { useMacTitlebarWindow } from "@/hooks/useMacTitlebarWindow";
import { isMacOS } from "@/lib/platform";
import { cn } from "@/lib/utils";
import { useUiStore } from "@/stores/uiStore";

export function Toolbar(): React.JSX.Element {
  // Settings state lives in the ui store so global shortcuts (Cmd+,) and
  // the command palette can open it from anywhere.
  const settingsOpen = useUiStore((s) => s.settingsOpen);
  const setSettingsOpen = useUiStore((s) => s.setSettingsOpen);
  const [aboutOpen, setAboutOpen] = useState(false);
  const dragZoneRef = useRef<HTMLDivElement>(null);
  useMacTitlebarWindow(dragZoneRef);

  // App-standard "open settings" shortcut: Cmd+, on macOS / Ctrl+, elsewhere
  // (same cross-platform modifier check as CommitMessageBox).
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent): void => {
      if ((e.metaKey || e.ctrlKey) && e.key === ",") {
        e.preventDefault();
        setSettingsOpen(true);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [setSettingsOpen]);

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
        <AppMenuBar onAbout={() => setAboutOpen(true)} />

        <ToolbarAppTitle />
      </div>

      <SettingsModal open={settingsOpen} onOpenChange={setSettingsOpen} />
      <AboutModal open={aboutOpen} onOpenChange={setAboutOpen} />
    </header>
  );
}
