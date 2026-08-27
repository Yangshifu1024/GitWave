import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { CircleHelp, KeyRound, MoreHorizontal } from "lucide-react";

import { ThemeToggle } from "@/components/ThemeToggle";
import { WorkspaceSwitcherDropdown } from "@/components/WorkspaceSwitcherDropdown";
import { SshKeyManager } from "@/components/SshKeyManager";
import { BranchIndicator } from "@/components/ui/BranchIndicator";
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
import { SyncButtons } from "@/components/ui/SyncButtons";
import { useWorkingCopy } from "@/hooks/useWorkingCopy";
import { getAppVersion, listRepos } from "@/lib/api";
import { isMacOS } from "@/lib/platform";
import { cn } from "@/lib/utils";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

function basename(path: string): string {
  const idx = Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\"));
  return idx >= 0 ? path.slice(idx + 1) : path;
}

export function Toolbar(): React.JSX.Element {
  const [sshOpen, setSshOpen] = useState(false);
  const [version, setVersion] = useState("…");
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const wc = useWorkingCopy();

  const { data: repos = [] } = useQuery({
    queryKey: ["repos", workspaceId],
    queryFn: () => listRepos(workspaceId!),
    enabled: Boolean(workspaceId),
  });
  const activeRepo = repos.find((repo) => repo.id === repoId);
  const repoLabel = activeRepo ? (activeRepo.nickname ?? basename(activeRepo.path)) : null;
  const snapshot = wc.data ?? null;
  const canSync = Boolean(workspaceId && repoId);

  useEffect(() => {
    getAppVersion()
      .then(setVersion)
      .catch(() => setVersion("?.?.?"));
  }, []);

  return (
    <header
      data-tauri-drag-region
      className={cn(
        "app-toolbar relative z-20 flex items-center shrink-0 h-10 gap-1.5",
        "bg-bg-secondary border-b border-border-subtle",
        isMacOS() && "app-toolbar--macos",
      )}
    >
      <WorkspaceSwitcherDropdown variant="toolbar" />

      {repoLabel ? (
        <>
          <span className="text-text-muted/50 text-xs" aria-hidden>
            ·
          </span>
          <span
            className="text-xs font-medium text-text-primary truncate max-w-[160px]"
            title={repoLabel}
          >
            {repoLabel}
          </span>
        </>
      ) : null}

      {snapshot ? (
        <BranchIndicator
          branch={snapshot.branch}
          sha={snapshot.branch === "(detached)" ? snapshot.sha : null}
          upstream={snapshot.upstream}
          ahead={snapshot.ahead}
          behind={snapshot.behind}
          className="text-xs"
        />
      ) : null}

      <SyncButtons
        ahead={snapshot?.ahead ?? 0}
        behind={snapshot?.behind ?? 0}
        onFetch={canSync ? wc.fetch : undefined}
        onPull={canSync ? wc.pull : undefined}
        onPush={canSync ? wc.push : undefined}
        fetchDisabled={!canSync}
        pullDisabled={!canSync || (snapshot?.behind ?? 0) === 0}
        pushDisabled={!canSync || (snapshot?.ahead ?? 0) === 0}
        inProgress={wc.syncPending}
      />

      <div className="ml-auto flex items-center gap-0.5">
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
