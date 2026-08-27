import { useQuery } from "@tanstack/react-query";

import { listRepos, listWorkspaces } from "@/lib/api";
import { useWorkingCopy } from "@/hooks/useWorkingCopy";
import { useSyncStore } from "@/stores/syncStore";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { syncOperationLabel } from "@/components/SyncProgressBar";
import { cn } from "@/lib/utils";

function basename(path: string): string {
  const idx = Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\"));
  return idx >= 0 ? path.slice(idx + 1) : path;
}

export function ToolbarContextTitle({ className }: { className?: string }): React.JSX.Element {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const activeOp = useSyncStore((s) => s.activeOp);
  const fading = useSyncStore((s) => s.fading);
  const { data: snapshot } = useWorkingCopy();

  const { data: workspaces = [] } = useQuery({
    queryKey: ["workspaces"],
    queryFn: listWorkspaces,
  });

  const { data: repos = [] } = useQuery({
    queryKey: ["repos", workspaceId],
    queryFn: () => listRepos(workspaceId!),
    enabled: Boolean(workspaceId),
  });

  const workspace = workspaces.find((ws) => ws.id === workspaceId);
  const repo = repos.find((r) => r.id === repoId);
  const repoLabel = repo ? (repo.nickname ?? basename(repo.path)) : null;
  const branch =
    snapshot?.branch && snapshot.branch !== "(detached)"
      ? snapshot.branch
      : snapshot?.branch === "(detached)"
        ? snapshot.sha.slice(0, 7)
        : null;

  const syncLabel = activeOp && !fading ? syncOperationLabel(activeOp) : null;

  const parts = [workspace?.name, repoLabel, branch].filter(Boolean);
  const contextTitle = parts.length > 0 ? parts.join(" - ") : "Select workspace";
  const display = syncLabel ?? contextTitle;

  return (
    <div
      className={cn(
        "absolute inset-x-0 flex justify-center items-center pointer-events-none px-32",
        className,
      )}
    >
      <span
        className={cn(
          "text-xs font-medium truncate max-w-[min(480px,60vw)]",
          syncLabel ? "text-text-secondary" : "text-text-primary",
        )}
        title={display}
      >
        {display}
      </span>
    </div>
  );
}
