import { useEffect, useRef, useState } from "react";
import { CircleHelp, KeyRound, MoreHorizontal } from "lucide-react";

import { ThemeToggle } from "@/components/ThemeToggle";
import { SshKeyManager } from "@/components/SshKeyManager";
import { ToolbarContextTitle } from "@/components/ToolbarContextTitle";
import { SyncProgressBar } from "@/components/SyncProgressBar";
import { Button } from "@/components/ui/Button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/DropdownMenu";
import { KeyHint } from "@/components/ui/KeyHint";
import { useMacTitlebarWindow } from "@/hooks/useMacTitlebarWindow";
import { getAppVersion } from "@/lib/api";
import { isMacOS } from "@/lib/platform";
import { cn } from "@/lib/utils";

export function Toolbar(): React.JSX.Element {
  const [sshOpen, setSshOpen] = useState(false);
  const [version, setVersion] = useState("…");
  const dragZoneRef = useRef<HTMLDivElement>(null);
  useMacTitlebarWindow(dragZoneRef);

  useEffect(() => {
    getAppVersion()
      .then(setVersion)
      .catch(() => setVersion("?.?.?"));
  }, []);

  return (
    <header
      className={cn(
        "app-toolbar relative z-20 flex items-center shrink-0 h-10 gap-1.5",
        "bg-bg-secondary border-b border-border-subtle",
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
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" size="sm" className="p-1" aria-label="More">
                <MoreHorizontal size={16} />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuLabel>GitWave v{version}</DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem onSelect={() => setSshOpen(true)}>
                <KeyRound size={14} />
                SSH Keys
              </DropdownMenuItem>
              <DropdownMenuItem disabled>
                <CircleHelp size={14} />
                Keyboard shortcuts
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </div>

      <SyncProgressBar />

      {sshOpen ? <SshKeyManagerModal onClose={() => setSshOpen(false)} /> : null}
    </header>
  );
}

function SshKeyManagerModal({ onClose }: { onClose: () => void }): React.JSX.Element {
  return (
    <div className="fixed inset-0 z-modal flex items-center justify-center">
      <div className="fixed inset-0 bg-bg-overlay backdrop-blur-sm" onClick={onClose} />
      <div
        className="relative z-10 w-full max-w-md rounded-xl bg-bg-elevated shadow-modal p-6"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-md font-semibold text-text-primary">SSH Keys</h2>
          <Button variant="ghost" size="sm" onClick={onClose} className="p-1">
            <span aria-hidden="true">&#x2715;</span>
          </Button>
        </div>
        <SshKeyManager />
      </div>
    </div>
  );
}
