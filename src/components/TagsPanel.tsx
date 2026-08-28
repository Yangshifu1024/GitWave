import { useQuery } from "@tanstack/react-query";
import { Tag as TagIcon } from "lucide-react";

import { formatAppError, listTags } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

/**
 * Sidebar section body: all tags in the active repo, click to jump to the
 * tagged commit in History.
 */
export function TagsPanel({ onSelect }: { onSelect?: (sha: string) => void }): React.JSX.Element {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);

  const {
    data: tags = [],
    isLoading,
    error,
  } = useQuery({
    queryKey: ["tags", workspaceId],
    queryFn: () => listTags(workspaceId!),
    enabled: Boolean(workspaceId && repoId),
  });

  if (!workspaceId || !repoId) return <></>;
  if (error) {
    return <p className="px-3 py-1.5 text-xs text-danger">{formatAppError(error)}</p>;
  }
  if (isLoading) {
    return <p className="px-3 py-1.5 text-xs text-text-muted italic">Loading…</p>;
  }
  if (tags.length === 0) {
    return <p className="px-3 py-1.5 text-xs text-text-muted">No tags yet</p>;
  }

  return (
    <div className="flex flex-col px-1 pb-1">
      {tags.map((t) => (
        <button
          key={t.name}
          type="button"
          onClick={() => t.sha && onSelect?.(t.sha)}
          title={t.annotation ?? t.name}
          className="flex min-w-0 items-center gap-2 rounded-md px-2 py-1 text-left hover:bg-bg-elevated"
        >
          <TagIcon size={12} className="shrink-0 text-text-muted" />
          <span className="min-w-0 flex-1 truncate text-xs text-text-secondary">{t.name}</span>
          <span className="shrink-0 font-mono text-[11px] text-text-muted tabular-nums">
            {t.sha.slice(0, 7)}
          </span>
        </button>
      ))}
    </div>
  );
}
