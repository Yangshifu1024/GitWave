import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import {
  discardAiPreview,
  formatAppError,
  previewAi,
  type AiPreview,
  type AiTask,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";

interface PreviewProps {
  workspaceId: string;
  task: AiTask;
  disabled?: boolean;
  sendDisabled?: boolean;
  initiallyOpen?: boolean;
  onSend: (id: number) => void;
}

export function AiPreviewButton(props: PreviewProps) {
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const scope = JSON.stringify([props.workspaceId, repoId, props.task]);
  return <ScopedPreview key={scope} repoId={repoId} {...props} />;
}

type ScopedProps = PreviewProps & { repoId: string | null };

function ScopedPreview({
  repoId,
  workspaceId,
  task,
  disabled,
  sendDisabled = disabled,
  initiallyOpen,
  onSend,
}: ScopedProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(Boolean(initiallyOpen));
  return (
    <>
      <Button variant="ghost" size="sm" disabled={disabled} onClick={() => setOpen(true)}>
        {t("ai.preview.open")}
      </Button>
      {open && (
        <PreviewDialog
          repoId={repoId}
          workspaceId={workspaceId}
          task={task}
          onClose={() => setOpen(false)}
          onSend={onSend}
          disabled={sendDisabled}
        />
      )}
    </>
  );
}

function PreviewDialog({
  repoId,
  workspaceId,
  task,
  onClose,
  onSend,
  disabled,
}: ScopedProps & { onClose: () => void }) {
  const { t } = useTranslation();
  const [preview, setPreview] = useState<AiPreview | null>(null);
  const [error, setError] = useState<string | null>(null);
  const handle = useRef<number | null>(null);
  // The parent keys this dialog by its full scope. Capture that request
  // once so unrelated parent renders cannot replace the reviewed text.
  const [request] = useState(() => ({ workspaceId, task, repoId }));
  useEffect(() => {
    let current = true;
    void previewAi(request.workspaceId, request.task, request.repoId)
      .then((result) => {
        if (!current) {
          void discardAiPreview(result.id);
          return;
        }
        handle.current = result.id;
        setPreview(result);
      })
      .catch((e) => {
        if (current) setError(formatAppError(e));
      });
    return () => {
      current = false;
      if (handle.current !== null) void discardAiPreview(handle.current);
      handle.current = null;
    };
  }, [request]);
  return (
    <Modal
      open
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      title={t("ai.preview.open")}
      description={t("ai.preview.description")}
      size="lg"
      footer={
        <>
          <Button variant="secondary" size="sm" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            variant="primary"
            size="sm"
            disabled={!preview || disabled}
            onClick={() => {
              const id = handle.current;
              if (id === null) return;
              handle.current = null;
              onSend(id);
              onClose();
            }}
          >
            {t("ai.preview.send")}
          </Button>
        </>
      }
    >
      {preview ? (
        <>
          <p className="text-xs text-text-secondary">
            {t("ai.preview.counts", preview.prompt.counts)}
          </p>
          <div className="max-h-[52vh] overflow-auto">
            {(["system", "user"] as const).map((role) => (
              <section key={role} className="mb-3">
                <h3 className="text-xs font-medium text-text-secondary">
                  {t(`ai.preview.${role}`)}
                </h3>
                <pre className="mt-1 whitespace-pre-wrap break-words rounded-md bg-bg-primary p-2 text-xs select-text">
                  {preview.prompt[role]}
                </pre>
              </section>
            ))}
          </div>
        </>
      ) : (
        <p role="status" className={error ? "text-xs text-danger" : "text-xs text-text-muted"}>
          {error ?? t("ai.preview.loading")}
        </p>
      )}
    </Modal>
  );
}
