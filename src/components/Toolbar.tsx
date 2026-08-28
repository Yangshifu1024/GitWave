import { useEffect, useRef, useState } from "react";
import { Settings } from "lucide-react";

import { ThemeToggle } from "@/components/ThemeToggle";
import { SettingsModal } from "@/components/SettingsModal";
import { ToolbarContextTitle } from "@/components/ToolbarContextTitle";
import { SyncProgressBar } from "@/components/SyncProgressBar";
import { Button } from "@/components/ui/Button";
import { KeyHint } from "@/components/ui/KeyHint";
import { useMacTitlebarWindow } from "@/hooks/useMacTitlebarWindow";
import { isMacOS } from "@/lib/platform";
import { cn } from "@/lib/utils";

export function Toolbar(): React.JSX.Element {
  const [settingsOpen, setSettingsOpen] = useState(false);
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
  }, []);

  return (
    <header
      className={cn(
        "app-toolbar relative z-20 flex items-center shrink-0 h-10 gap-1.5",
        "bg-bg-primary border-b border-border-subtle",
        isMacOS() && "app-toolbar--macos",
      )}
    >
      <div
        ref={dragZoneRef}
        className="absolute inset-0 z-0"
        {...(!isMacOS() ? { "data-tauri-drag-region": true } : {})}
      />

      <div className="relative z-10 flex flex-1 min-w-0 items-center pointer-events-none">
        <ToolbarContextTitle />

        <div className="ml-auto flex items-center gap-0.5 pointer-events-auto">
          <KeyHint keys={["⌘", "K"]} className="mr-1 opacity-80" />
          <ThemeToggle />
          <Button
            variant="ghost"
            size="sm"
            className="p-1"
            aria-label="Settings"
            title={isMacOS() ? "Settings (⌘,)" : "Settings (Ctrl+,)"}
            onClick={() => setSettingsOpen(true)}
          >
            <Settings size={16} />
          </Button>
        </div>
      </div>

      <SyncProgressBar />

      <SettingsModal open={settingsOpen} onOpenChange={setSettingsOpen} />
    </header>
  );
}
