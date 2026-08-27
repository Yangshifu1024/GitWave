// Sidebar workspace list — pure navigation (select). Operations live in the
// ActionBar below the top bar.

import { useQuery } from "@tanstack/react-query";

import { listWorkspaces } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { ListItem } from "@/components/ui/ListItem";
import { SidebarSection } from "@/components/ui/SidebarSection";

export function WorkspaceList(): React.JSX.Element {
  const activeId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const selectWorkspace = useWorkspaceUiStore((s) => s.selectWorkspace);

  const {
    data: workspaces = [],
    isLoading,
    error,
  } = useQuery({
    queryKey: ["workspaces"],
    queryFn: listWorkspaces,
  });

  const workspaceBody = isLoading ? (
    <p className="px-3 py-2 text-sm text-text-muted">Loading…</p>
  ) : error ? (
    <p className="px-3 py-2 text-sm text-text-muted">Failed to load workspaces.</p>
  ) : workspaces.length === 0 ? (
    <p className="px-3 py-2 text-sm text-text-muted italic">No workspaces yet</p>
  ) : (
    <ul className="pb-1">
      {workspaces.map((ws) => (
        <li key={ws.id}>
          <ListItem
            selected={ws.id === activeId}
            onClick={() => selectWorkspace(ws.id, ws.last_active_repo_id)}
          >
            <span className="truncate">{ws.name}</span>
          </ListItem>
        </li>
      ))}
    </ul>
  );

  return <SidebarSection title="Workspaces">{workspaceBody}</SidebarSection>;
}
