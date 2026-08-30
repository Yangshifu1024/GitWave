import { useEffect, useState } from "react";

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
export function GitignoreEditor({ open, onClose }: GitignoreEditorProps): React.JSX.Element | null {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const setStatus = useStatusAreaStore((s) => s.setStatus);
  const [content, setContent] = useState("");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open || !workspaceId) return;
    setLoading(true);
    getGitignore(workspaceId)
      .then(setContent)
      .catch((e) => setStatus(formatAppError(e), "danger"))
      .finally(() => setLoading(false));
  }, [open, workspaceId, setStatus]);

  if (!open) return null;

  const save = (): void => {
    if (!workspaceId || saving) return;
    setSaving(true);
    writeGitignore(workspaceId, content)
      .then(() => {
        setStatus(".gitignore saved");
        onClose();
      })
      .catch((e) => setStatus(formatAppError(e), "danger"))
      .finally(() => setSaving(false));
  };

  return (
    <Modal
      open
      onOpenChange={(o) => !o && onClose()}
      title="Edit .gitignore"
      description="One pattern per line, relative to the repository root."
      size="md"
      footer={
        <>
          <Button variant="secondary" size="sm" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" size="sm" disabled={saving || loading} onClick={save}>
            {saving ? "Saving…" : "Save"}
          </Button>
        </>
      }
    >
      <div className="rounded-xl bg-bg-primary p-3">
        <Textarea
          value={loading ? "Loading…" : content}
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
