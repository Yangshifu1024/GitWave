// Workspace switcher dropdown at the ActionBar's left edge: shows the active
// workspace, lists all workspaces to switch (each selection restores that
// workspace's last active repo), and carries a "new workspace" entry point so
// a first-run user with no workspaces is not stuck. Rename / delete stay in the
// Workspace menu; the create dialog itself is owned by ActionBar.

import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Check, ChevronDown, FolderGit2, FolderPlus } from "lucide-react";

import { listWorkspaces } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { useUiStore } from "@/stores/uiStore";
import { useValidatedWorkspaceSwitch } from "@/hooks/useValidatedWorkspaceSwitch";
import { Button } from "@/components/ui/Button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@/components/ui/DropdownMenu";

export function WorkspaceDropdown(): React.JSX.Element {
  const { t } = useTranslation();
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const switchWorkspace = useValidatedWorkspaceSwitch();
  // Creating a workspace stays owned by ActionBar's create dialog; the entry
  // point here just requests the same action the Workspace menu fires.
  const requestMenuAction = useUiStore((s) => s.requestMenuAction);
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
        <span className="max-w-[180px] truncate">
          {active ? active.name : t("workspace.selectorFallback")}
        </span>
        <ChevronDown size={12} className="shrink-0 opacity-70" />
      </Button>
      <DropdownMenuContent placement="bottom start" className="min-w-[200px]">
        <DropdownMenuLabel>{t("workspace.title")}</DropdownMenuLabel>
        {workspaces.map((w) => (
          <DropdownMenuItem
            key={w.id}
            id={w.id}
            textValue={w.name}
            onSelect={() => switchWorkspace(w.id, w.last_active_repo_id)}
          >
            <span className="flex w-3.5 shrink-0">
              {w.id === activeWorkspaceId ? <Check size={13} /> : null}
            </span>
            <span className="truncate">{w.name}</span>
          </DropdownMenuItem>
        ))}
        {workspaces.length === 0 ? (
          <DropdownMenuItem disabled textValue={t("workspace.empty.title")}>
            {t("workspace.empty.title")}
          </DropdownMenuItem>
        ) : null}
        <DropdownMenuSeparator />
        <DropdownMenuItem
          textValue={t("workspace.new")}
          onSelect={() => requestMenuAction("workspace:new")}
        >
          <span className="flex w-3.5 shrink-0">
            <FolderPlus size={13} />
          </span>
          <span className="truncate">{t("workspace.new")}</span>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
