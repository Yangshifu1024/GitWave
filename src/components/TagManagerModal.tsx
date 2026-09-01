// Create (lightweight or annotated) and delete tags on a commit. Shared by
// the inspector header (CommitInfoHeader) and the commit context menu (F011).

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Tag as TagIcon, Trash2 } from "lucide-react";

import { createTag, deleteTag, formatAppError } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { Textarea } from "@/components/ui/Textarea";

export function TagManagerModal({
  workspaceId,
  sha,
  tags,
  onClose,
  onChanged,
  onError,
  onCreated,
}: {
  workspaceId: string;
  sha: string;
  tags: { name: string; sha: string; annotation: string | null }[];
  onClose: () => void;
  onChanged: () => void;
  onError: (message: string) => void;
  onCreated: (name: string) => void;
}): React.JSX.Element {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  /** Tag pending deletion — destructive, so it confirms first (P1). */
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);

  const here = tags.filter((tag) => tag.sha === sha);

  const create = (): void => {
    if (!name.trim() || busy) return;
    setBusy(true);
    createTag(workspaceId, name.trim(), sha, message.trim() || null)
      .then(() => {
        onCreated(name.trim());
        setName("");
        setMessage("");
        onChanged();
        onClose();
      })
      .catch((e) => onError(formatAppError(e)))
      .finally(() => setBusy(false));
  };

  const remove = (tagName: string): void => {
    if (busy) return;
    setBusy(true);
    deleteTag(workspaceId, tagName)
      .then(() => {
        setDeleteTarget(null);
        onChanged();
      })
      .catch((e) => onError(formatAppError(e)))
      .finally(() => setBusy(false));
  };

  return (
    <Modal
      open
      onOpenChange={(open) => !open && onClose()}
      title={t("commits.tag.managerTitle", { sha: sha.slice(0, 7) })}
      size="sm"
      footer={
        <>
          <Button variant="secondary" size="sm" className="min-w-0 flex-[3]" onClick={onClose}>
            {t("commits.action.close")}
          </Button>
          <Button
            variant="primary"
            size="sm"
            className="min-w-0 flex-[7]"
            disabled={busy || !name.trim()}
            onClick={create}
          >
            {t("commits.tag.createButton")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-1.5 rounded-xl bg-bg-primary p-3">
        {here.length === 0 ? (
          <p className="text-xs text-text-muted italic">{t("commits.tag.none")}</p>
        ) : (
          here.map((tag) => (
            <div
              key={tag.name}
              className="flex items-center gap-2 rounded-md border border-border-subtle px-2 py-1.5"
            >
              <TagIcon size={13} className="shrink-0 text-text-muted" />
              <span className="min-w-0 flex-1 truncate text-xs font-medium text-text-primary">
                {tag.name}
                {tag.annotation ? (
                  <span className="ml-1 font-normal text-text-muted" title={tag.annotation}>
                    {t("commits.tag.annotated")}
                  </span>
                ) : null}
              </span>
              <Button
                variant="ghost"
                size="sm"
                className="p-1 text-text-muted hover:text-danger"
                aria-label={t("commits.tag.deleteAria", { name: tag.name })}
                title={t("commits.tag.deleteTitle", { name: tag.name })}
                disabled={busy}
                onClick={() => setDeleteTarget(tag.name)}
              >
                <Trash2 size={13} />
              </Button>
            </div>
          ))
        )}
      </div>
      <div className="flex flex-col gap-2 rounded-xl bg-bg-primary p-3">
        <Input
          value={name}
          onChange={setName}
          onKeyDown={(e) => {
            if (e.key === "Enter") create();
          }}
          placeholder={t("commits.tag.namePlaceholder")}
          autoFocus
        />
        <Textarea
          value={message}
          onChange={setMessage}
          placeholder={t("commits.tag.annotationPlaceholder")}
          rows={2}
        />
      </div>

      {deleteTarget ? (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setDeleteTarget(null);
          }}
          title={t("repo.tags.deleteDialog.title")}
          description={t("repo.tags.deleteDialog.description", { name: deleteTarget })}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setDeleteTarget(null)}
              >
                {t("commits.action.cancel")}
              </Button>
              <Button
                variant="danger"
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={busy}
                onClick={() => remove(deleteTarget)}
              >
                {t("commits.action.delete")}
              </Button>
            </>
          }
        />
      ) : null}
    </Modal>
  );
}
