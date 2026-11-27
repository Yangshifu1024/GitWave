import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { invoke } from "@tauri-apps/api/core";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { formatAppError } from "@/lib/api";

type Props = { workspaceId: string; repoId: string; path: string; staged: boolean };
export function PartialStaging(props: Props) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const [selected, setSelected] = useState<{ token: string; hunk: number; lines: number[] } | null>(
    null,
  );
  const client = useQueryClient();
  const preview = useQuery({
    queryKey: ["partial-preview", props],
    queryFn: () => invoke<{ token: string; hunks: string[][] }>("cmd_partial_preview", props),
    enabled: open,
    retry: false,
  });
  const write = useMutation({
    mutationFn: (selection: NonNullable<typeof selected>) =>
      invoke<void>("cmd_partial_apply", { ...props, selection }),
    onSuccess: async () => {
      setSelected(null);
      await Promise.all([
        preview.refetch(),
        client.invalidateQueries({ queryKey: ["working-copy", props.workspaceId, props.repoId] }),
        client.invalidateQueries({ queryKey: ["diff-preview", props.workspaceId, props.repoId] }),
      ]);
    },
  });
  const changeOpen = (next: boolean) => {
    if (write.isPending) return;
    setOpen(next);
    setSelected(null);
    write.reset();
  };
  return (
    <>
      <Button size="sm" variant="ghost" onClick={() => changeOpen(true)}>
        {t(props.staged ? "changes.partial.unstage" : "changes.partial.stage")}
      </Button>
      <Modal
        open={open}
        onOpenChange={changeOpen}
        title={t(props.staged ? "changes.partial.unstage" : "changes.partial.stage")}
      >
        <p className="truncate text-xs">{props.path}</p>
        {preview.isFetching && <p role="status">{t("changes.loading")}</p>}
        {(preview.error || write.error) && (
          <div role="alert">
            {formatAppError(preview.error ?? write.error)}{" "}
            <Button
              onClick={() => {
                setSelected(null);
                write.reset();
                void preview.refetch();
              }}
            >
              {t("common.retry")}
            </Button>
          </div>
        )}
        <div className="max-h-[55vh] overflow-auto">
          {!preview.error &&
            preview.data?.hunks.map((hunk, index) => {
              const token = preview.data.token;
              const all = hunk.flatMap((line, i) => (i > 0 && /^[+-]/.test(line) ? [i] : []));
              const lines =
                selected?.token === token && selected.hunk === index ? selected.lines : [];
              const disabled = write.isPending || preview.isFetching || !!write.error;
              return (
                <section key={index} className="mb-2">
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      disabled={disabled}
                      onClick={() => write.mutate({ hunk: index, token, lines: all })}
                    >
                      {t("changes.partial.hunk")}
                    </Button>
                    <Button
                      size="sm"
                      disabled={disabled || !lines.length}
                      onClick={() => write.mutate({ hunk: index, token, lines })}
                    >
                      {t("changes.partial.lines", { count: lines.length })}
                    </Button>
                  </div>
                  {hunk.map((line, i) => (
                    <label key={i} className="block whitespace-pre font-mono text-xs">
                      {i > 0 && /^[+-]/.test(line) && (
                        <input
                          type="checkbox"
                          disabled={disabled}
                          checked={lines.includes(i)}
                          onChange={(e) =>
                            setSelected({
                              token,
                              hunk: index,
                              lines: e.target.checked
                                ? [...lines, i]
                                : lines.filter((n) => n !== i),
                            })
                          }
                        />
                      )}{" "}
                      {line.replace(/\n$/, "")}
                    </label>
                  ))}
                </section>
              );
            })}
        </div>
      </Modal>
    </>
  );
}
