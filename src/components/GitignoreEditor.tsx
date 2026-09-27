import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import { formatAppError, getGitignore, writeGitignore } from "@/lib/api";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { Textarea } from "@/components/ui/Textarea";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

export interface GitignoreEditorProps {
  open: boolean;
  onClose: () => void;
}

/**
 * Modal editor for the repo-root `.gitignore` (S2): load on open, edit as
 * plain text (mono font), save back. The backend normalizes the trailing
 * newline so the per-file "Add to .gitignore" append keeps working.
 */
export function GitignoreEditor(
  props: React.ComponentProps<typeof GitignoreEditorContent>,
): React.JSX.Element {
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  return (
    <GitignoreEditorContent key={JSON.stringify([props.open, workspaceId, repoId])} {...props} />
  );
}

function GitignoreEditorContent({ open, onClose }: GitignoreEditorProps): React.JSX.Element | null {
  const { t } = useTranslation();
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const setStatus = useStatusAreaStore((s) => s.setStatus);
  const [content, setContent] = useState("");
  const [loading, setLoading] = useState(open && !!workspaceId);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open || !workspaceId) return;
    let cancelled = false;
    getGitignore(workspaceId)
      .then((text) => {
        if (!cancelled) setContent(text);
      })
      .catch((e) => {
        if (!cancelled) setStatus(formatAppError(e), "danger");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [open, workspaceId, setStatus]);

  if (!open) return null;

  const save = (): void => {
    if (!workspaceId || saving) return;
    setSaving(true);
    writeGitignore(workspaceId, content)
      .then(() => {
        setStatus(t("repo.gitignore.saved"));
        onClose();
      })
      .catch((e) => setStatus(formatAppError(e), "danger"))
      .finally(() => setSaving(false));
  };

  return (
    <Modal
      open
      onOpenChange={(o) => !o && onClose()}
      title={t("repo.gitignore.title")}
      description={t("repo.gitignore.description")}
      size="md"
      footer={
        <>
          <Button variant="secondary" size="sm" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button variant="primary" size="sm" disabled={saving || loading} onClick={save}>
            {saving ? t("repo.gitignore.saving") : t("common.save")}
          </Button>
        </>
      }
    >
      <div className="rounded-xl bg-bg-primary p-3">
        <Textarea
          value={loading ? t("common.loading") : content}
          disabled={loading}
          onChange={setContent}
          rows={14}
          spellCheck={false}
          className="rounded-md px-2.5 py-2 font-mono leading-5"
        />
      </div>
    </Modal>
  );
}
