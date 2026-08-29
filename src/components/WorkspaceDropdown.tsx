// Workspace switcher dropdown at the ActionBar's left edge: shows the active
// workspace, lists all workspaces to switch (each selection restores that
// workspace's last active repo). Create / rename / delete stay in the
// Workspace menu.

import { useQuery } from "@tanstack/react-query";
import { Check, ChevronDown, FolderGit2 } from "lucide-react";

import { listWorkspaces } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
} from "@/components/ui/DropdownMenu";

export function WorkspaceDropdown(): React.JSX.Element {
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const selectWorkspace = useWorkspaceUiStore((s) => s.selectWorkspace);
  const { data: workspaces = [] } = useQuery({
    queryKey: ["workspaces"],
    queryFn: listWorkspaces,
  });
  const active = workspaces.find((w) => w.id === activeWorkspaceId);

  return (
    <DropdownMenu>
      <Button
        variant="ghost"
        size="sm"
        className="h-7 gap-1.5 border border-border-default px-2.5 text-xs text-text-secondary hover:bg-bg-elevated hover:text-text-primary"
      >
        <FolderGit2 size={14} className="shrink-0" />
        <span className="max-w-[180px] truncate">{active ? active.name : "Workspace"}</span>
        <ChevronDown size={12} className="shrink-0 opacity-70" />
      </Button>
      <DropdownMenuContent placement="bottom start" className="min-w-[200px]">
        <DropdownMenuLabel>Workspaces</DropdownMenuLabel>
        {workspaces.map((w) => (
          <DropdownMenuItem
            key={w.id}
            id={w.id}
            textValue={w.name}
            onSelect={() => selectWorkspace(w.id, w.last_active_repo_id)}
          >
            <span className="flex w-3.5 shrink-0">
              {w.id === activeWorkspaceId ? <Check size={13} /> : null}
            </span>
            <span className="truncate">{w.name}</span>
          </DropdownMenuItem>
        ))}
        {workspaces.length === 0 ? (
          <DropdownMenuItem disabled textValue="none">
            No workspaces yet
          </DropdownMenuItem>
        ) : null}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
