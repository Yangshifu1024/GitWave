// AI PR description modal — generates a copy-ready title + markdown body
// for the active branch vs the default base branch. Nothing is pushed or
// created remotely (P1): the user copies the text into their PR tool.

import { useEffect, useRef, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Copy, RefreshCw } from "lucide-react";
import { useTranslation } from "react-i18next";
import { formatAppError, generatePrDescription } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { Textarea } from "@/components/ui/Textarea";
import { useStatusAreaStore } from "@/stores/statusAreaStore";

export function PrDescriptionModal({
  workspaceId,
  open,
  onClose,
}: {
  workspaceId: string;
  open: boolean;
  onClose: () => void;
}): React.JSX.Element {
  const { t } = useTranslation();
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [provider, setProvider] = useState<string | null>(null);
  const setStatus = useStatusAreaStore((s) => s.setStatus);

  // Generation session: reopening (or regenerating) while a previous
  // request is in flight must not let the stale response overwrite fresh
  // content. The session rides the mutation context.
  const sessionRef = useRef(0);
  const genMut = useMutation({
    mutationFn: () => generatePrDescription(workspaceId),
    onMutate: () => ({ session: sessionRef.current }),
    onSuccess: (res, _variables, context) => {
      if (context && context.session !== sessionRef.current) return;
      setTitle(res.title);
      setBody(res.body);
      setProvider(res.provider_used);
      setError(null);
      if (res.used_fallback) {
        setStatus(t("commits.ai.fallbackNotice", { provider: res.provider_used }), "info");
      }
    },
    onError: (e, _variables, context) => {
      if (context && context.session !== sessionRef.current) return;
      setError(formatAppError(e));
    },
  });

  // Fresh generation each time the modal opens; the user can regenerate.
  useEffect(() => {
    if (!open) return;
    sessionRef.current += 1;
    setTitle("");
    setBody("");
    setError(null);
    setProvider(null);
    genMut.mutate();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- run once per open
  }, [open, workspaceId]);

  const copy = async (text: string, label: string) => {
    try {
      await navigator.clipboard.writeText(text);
      setStatus(t("commits.pr.copied", { label }));
    } catch {
      setStatus(t("commits.pr.clipboardUnavailable"), "danger");
    }
  };

  return (
    <Modal
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      title={t("commits.pr.title")}
      description={t("commits.pr.description")}
      size="lg"
      footer={
        <>
          <Button
            variant="ghost"
            size="sm"
            disabled={genMut.isPending}
            onClick={() => {
              sessionRef.current += 1;
              genMut.mutate();
            }}
          >
            <RefreshCw size={13} />
            {t("commits.action.regenerate")}
          </Button>
          <Button
            variant="secondary"
            size="sm"
            disabled={!title || genMut.isPending}
            onClick={() => void copy(title, t("commits.pr.titleLabel"))}
          >
            <Copy size={13} />
            {t("commits.action.copyTitle")}
          </Button>
          <Button
            variant="secondary"
            size="sm"
            disabled={!body || genMut.isPending}
            onClick={() => void copy(body, t("commits.pr.descriptionLabel"))}
          >
            <Copy size={13} />
            {t("commits.action.copyDescription")}
          </Button>
          <Button
            variant="primary"
            size="sm"
            disabled={(!title && !body) || genMut.isPending}
            onClick={() => void copy(`${title}\n\n${body}`.trim(), t("commits.pr.allLabel"))}
          >
            <Copy size={13} />
            {t("commits.action.copyAll")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        <div className="flex flex-col gap-2 rounded-xl bg-bg-primary p-3">
          <label className="flex flex-col gap-1 text-xs text-text-secondary">
            {t("commits.pr.titleLabel")}
            <Input
              value={title}
              onChange={setTitle}
              placeholder={t("commits.pr.titlePlaceholder")}
              disabled={genMut.isPending}
            />
          </label>
          <label className="flex flex-col gap-1 text-xs text-text-secondary">
            {t("commits.pr.descriptionMarkdown")}
            <Textarea
              value={body}
              onChange={setBody}
              rows={14}
              spellCheck={false}
              disabled={genMut.isPending}
              placeholder={t("commits.pr.bodyPlaceholder")}
              className="rounded-md px-2 py-1.5 font-mono text-[11px] leading-5"
            />
          </label>
        </div>

        {genMut.isPending ? (
          <p className="text-xs text-text-muted">{t("commits.pr.generating")}</p>
        ) : error ? (
          <p className="text-xs text-danger">{error}</p>
        ) : provider ? (
          <p className="text-[11px] text-text-muted">
            {t("commits.explain.generatedBy", { provider })}
          </p>
        ) : null}
      </div>
    </Modal>
  );
}
