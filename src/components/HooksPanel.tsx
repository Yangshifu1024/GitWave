// Git hooks editor — list known client-side hooks, edit scripts. GitWave
// only edits hook files; it never executes them (P1). Saving marks the
// file executable on unix, mirroring `git init` samples.

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { formatAppError, getHook, listHooks, saveHook, type HookInfo } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { Textarea } from "@/components/ui/Textarea";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { cn } from "@/lib/utils";

const SAMPLE_PRE_COMMIT = `#!/bin/sh
# GitWave sample pre-commit hook. Non-zero exit aborts the commit.
# Example: block commits to main with TODO markers
# if git diff --cached | grep -q TODO; then
#   echo "TODO markers found in staged changes" >&2
#   exit 1
# fi
`;

export function HooksPanel({
  workspaceId,
  open,
  onClose,
}: {
  workspaceId: string;
  open: boolean;
  onClose: () => void;
}): React.JSX.Element {
  const { t } = useTranslation();
  const [selected, setSelected] = useState<string | null>(null);
  const [content, setContent] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [dirty, setDirty] = useState(false);
  const setStatus = useStatusAreaStore((s) => s.setStatus);
  const queryClient = useQueryClient();

  const { data: hooks = [] } = useQuery({
    queryKey: ["hooks", workspaceId],
    queryFn: () => listHooks(workspaceId),
    enabled: open,
  });

  const pick = async (hook: HookInfo): Promise<void> => {
    setError(null);
    setDirty(false);
    setSelected(hook.name);
    try {
      setContent(await getHook(workspaceId, hook.name));
    } catch (e) {
      setError(formatAppError(e));
    }
  };

  const saveMut = useMutation({
    mutationFn: () => saveHook(workspaceId, selected ?? "", content),
    onSuccess: () => {
      setDirty(false);
      setStatus(t("repo.hooks.saved", { name: selected }));
      void queryClient.invalidateQueries({ queryKey: ["hooks", workspaceId] });
    },
    onError: (e) => setError(formatAppError(e)),
  });

  return (
    <Modal
      open={open}
      onOpenChange={(next) => {
        if (!next) onClose();
      }}
      title={t("repo.hooks.title")}
      description={t("repo.hooks.description")}
      size="lg"
      footer={
        <Button
          variant="primary"
          size="sm"
          disabled={!selected || saveMut.isPending || (!dirty && !error)}
          onClick={() => saveMut.mutate()}
        >
          {t("repo.hooks.saveName", { name: selected ?? "" })}
        </Button>
      }
    >
      <div className="flex gap-3">
        <div className="w-44 shrink-0 flex flex-col gap-0.5">
          {hooks.map((hook) => (
            <button
              key={hook.name}
              type="button"
              onClick={() => void pick(hook)}
              className={cn(
                "flex items-center gap-2 rounded-md px-2 py-1 text-left text-xs",
                selected === hook.name
                  ? "bg-bg-secondary text-text-primary"
                  : "text-text-secondary hover:bg-bg-secondary",
              )}
            >
              <span
                aria-hidden="true"
                className={cn(
                  "size-1.5 shrink-0 rounded-full",
                  hook.exists ? "bg-success" : "bg-transparent border border-border-subtle",
                )}
              />
              <span className="truncate font-mono">{hook.name}</span>
            </button>
          ))}
        </div>

        <div className="flex min-w-0 flex-1 flex-col gap-2">
          {selected ? (
            <>
              <Textarea
                value={content}
                onChange={(v) => {
                  setContent(v);
                  setDirty(true);
                }}
                rows={14}
                spellCheck={false}
                placeholder={t("repo.hooks.scriptPlaceholder", { name: selected })}
                className="rounded-md px-2 py-1.5 font-mono text-[11px] leading-5"
              />
              {!content.trim() ? (
                <Button
                  variant="ghost"
                  size="sm"
                  className="self-start"
                  onClick={() => {
                    if (selected === "pre-commit") {
                      setContent(SAMPLE_PRE_COMMIT);
                      setDirty(true);
                    }
                  }}
                  title={t("repo.hooks.insertSampleTitle")}
                >
                  {t("repo.hooks.insertSample")}
                </Button>
              ) : null}
            </>
          ) : (
            <p className="text-xs text-text-muted">{t("repo.hooks.selectHint")}</p>
          )}

          {error ? <p className="text-xs text-danger">{error}</p> : null}
        </div>
      </div>
    </Modal>
  );
}
