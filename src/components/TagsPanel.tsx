import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Tag as TagIcon } from "lucide-react";

import { formatAppError, listTags } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { SidebarSection } from "@/components/ui/SidebarSection";

/**
 * Sidebar card: all tags in the active repo, click to jump to the tagged
 * commit in History. An empty tag list collapses the card to a static
 * header (nothing to expand).
 */
export function TagsPanel({ onSelect }: { onSelect?: (sha: string) => void }): React.JSX.Element {
  const { t } = useTranslation();
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

  const collapsible = !isLoading && !error && tags.length > 0;

  return (
    <SidebarSection title={t("repo.tags.title")} collapsible={collapsible}>
      {error ? (
        <p className="px-3 py-1.5 text-xs text-danger">{formatAppError(error)}</p>
      ) : isLoading ? null : tags.length === 0 ? (
        <p className="px-3 py-1.5 text-xs text-text-muted">{t("repo.tags.empty")}</p>
      ) : (
        <div className="flex flex-col px-1 pb-1">
          {tags.map((tag) => (
            <button
              key={tag.name}
              type="button"
              onClick={() => tag.sha && onSelect?.(tag.sha)}
              title={tag.annotation ?? tag.name}
              className="flex min-w-0 items-center gap-2 rounded-md px-2 py-1 text-left hover:bg-bg-elevated"
            >
              <TagIcon size={12} className="shrink-0 text-text-muted" />
              <span className="min-w-0 flex-1 truncate text-xs text-text-secondary">
                {tag.name}
              </span>
              <span className="shrink-0 font-mono text-[11px] text-text-muted tabular-nums">
                {tag.sha.slice(0, 7)}
              </span>
            </button>
          ))}
        </div>
      )}
    </SidebarSection>
  );
}
