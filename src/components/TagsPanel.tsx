// Sidebar tags card: click a tag to jump to its commit in History; the row
// whose commit is currently selected stays highlighted. Right-click a row for
// Delete (confirm modal), matching the sidebar branch-delete convention.

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Tag as TagIcon, Trash2 } from "lucide-react";

import { deleteTag, formatAppError, listTags, type TagInfo } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { SidebarSection } from "@/components/ui/SidebarSection";
import { ListItem } from "@/components/ui/ListItem";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { ErrorAlert } from "@/components/ui/ErrorAlert";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuLabel,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/ContextMenu";

export function TagsPanel({
  onSelect,
  selectedSha,
}: {
  onSelect?: (sha: string) => void;
  /** Commit currently selected in History — highlights the tag pointing at it. */
  selectedSha?: string | null;
}): React.JSX.Element {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const setStatus = useStatusAreaStore((s) => s.setStatus);

  const [deleteTarget, setDeleteTarget] = useState<TagInfo | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

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

  const confirmDelete = (): void => {
    if (!deleteTarget || !workspaceId || deleting) return;
    setDeleting(true);
    deleteTag(workspaceId, deleteTarget.name)
      .then(() => {
        setStatus(t("repo.tags.deleteDialog.deleted", { name: deleteTarget.name }));
        setDeleteTarget(null);
      })
      .catch((e) => setActionError(formatAppError(e)))
      .finally(() => {
        setDeleting(false);
        void queryClient.invalidateQueries({ queryKey: ["tags", workspaceId] });
      });
  };

  return (
    <>
      <SidebarSection title={t("repo.tags.title")} collapsible={collapsible}>
        {error ? (
          <p className="px-3 py-1.5 text-xs text-danger">{formatAppError(error)}</p>
        ) : isLoading ? null : tags.length === 0 ? (
          <p className="px-3 py-1.5 text-xs text-text-muted">{t("repo.tags.empty")}</p>
        ) : (
          <div className="flex flex-col px-1 pb-1">
            {tags.map((tag) => (
              <ContextMenu key={tag.name}>
                <ContextMenuTrigger asChild>
                  <div>
                    <ListItem
                      selected={Boolean(selectedSha && tag.sha && selectedSha === tag.sha)}
                      onClick={() => tag.sha && onSelect?.(tag.sha)}
                      leading={<TagIcon size={12} />}
                      className="px-2"
                    >
                      <span
                        className="min-w-0 flex-1 truncate text-xs text-text-secondary"
                        title={tag.annotation ?? tag.name}
                      >
                        {tag.name}
                      </span>
                      <span className="shrink-0 font-mono text-[11px] text-text-muted tabular-nums">
                        {tag.sha.slice(0, 7)}
                      </span>
                    </ListItem>
                  </div>
                </ContextMenuTrigger>
                <ContextMenuContent className="max-w-[240px]">
                  <ContextMenuLabel title={tag.name}>{tag.name}</ContextMenuLabel>
                  <ContextMenuSeparator />
                  <ContextMenuItem destructive onSelect={() => setDeleteTarget(tag)}>
                    <Trash2 size={14} />
                    {t("repo.tags.menu.delete")}
                  </ContextMenuItem>
                </ContextMenuContent>
              </ContextMenu>
            ))}
          </div>
        )}
      </SidebarSection>

      <ErrorAlert message={actionError} onDismiss={() => setActionError(null)} />

      {deleteTarget ? (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setDeleteTarget(null);
          }}
          title={t("repo.tags.deleteDialog.title")}
          description={t("repo.tags.deleteDialog.description", { name: deleteTarget.name })}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setDeleteTarget(null)}
              >
                {t("common.cancel")}
              </Button>
              <Button
                variant="danger"
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={deleting}
                onClick={confirmDelete}
              >
                {t("common.delete")}
              </Button>
            </>
          }
        >
          <div className="flex items-center gap-2 rounded-xl bg-bg-primary p-3">
            <span className="w-16 shrink-0 text-sm text-text-secondary">
              {t("repo.tags.title")}
            </span>
            <span className="flex min-w-0 items-center gap-1 text-sm text-text-primary">
              <TagIcon size={12} className="shrink-0 text-text-muted" />
              <span className="truncate" title={deleteTarget.name}>
                {deleteTarget.name}
              </span>
            </span>
          </div>
        </Modal>
      ) : null}
    </>
  );
}
