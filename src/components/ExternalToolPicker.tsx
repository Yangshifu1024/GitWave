import { useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { ChevronDown } from "lucide-react";

import { Button } from "@/components/ui/Button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem } from "@/components/ui/DropdownMenu";
import {
  formatAppError,
  listEditors,
  listTerminals,
  openInEditor,
  openInTerminal,
} from "@/lib/api";

const pickers = {
  terminal: { prefix: "workspace.openTerminal", list: listTerminals, launch: openInTerminal },
  editor: { prefix: "workspace.openEditor", list: listEditors, launch: openInEditor },
};

export function ExternalToolPicker({
  kind,
  repoPath,
  onError,
}: {
  kind: keyof typeof pickers;
  repoPath: string | null;
  onError: (message: string) => void;
}): React.JSX.Element {
  const { t } = useTranslation();
  const { prefix, list, launch } = pickers[kind];
  const storageKey = `gitwave.externalTool.${kind}`;
  const [preferredId, setPreferredId] = useState<string | null>(() => {
    try {
      return localStorage.getItem(storageKey);
    } catch {
      return null;
    }
  });
  const {
    data: choices = [],
    refetch,
    isPending,
    error,
  } = useQuery({
    queryKey: ["externalTools", kind],
    queryFn: list,
    enabled: !!repoPath,
  });
  const current = choices.find((tool) => tool.id === preferredId);
  const opening = useRef(false);
  const [busy, setBusy] = useState(false);
  const disabled = !repoPath || busy;

  async function open(id: string): Promise<void> {
    if (!repoPath || opening.current) return;
    opening.current = true;
    setBusy(true);
    try {
      await launch(repoPath, id);
      setPreferredId(id);
      try {
        localStorage.setItem(storageKey, id);
      } catch {
        // The in-session choice still works if storage is unavailable.
      }
    } catch (cause) {
      onError(formatAppError(cause));
      void refetch();
    } finally {
      opening.current = false;
      setBusy(false);
    }
  }

  return (
    <div
      role="group"
      aria-label={t(`${prefix}.label`)}
      className="flex min-w-0 items-center rounded-md border border-border-subtle"
    >
      {current && (
        <Button
          size="sm"
          variant="ghost"
          disabled={disabled}
          onClick={() => void open(current.id)}
          className="min-w-0 max-w-36 rounded-r-none"
        >
          <span className="truncate">{current.name}</span>
        </Button>
      )}
      <DropdownMenu
        onOpenChange={(shown) => {
          if (shown) void refetch();
        }}
      >
        <Button
          size="sm"
          variant="ghost"
          disabled={disabled}
          aria-label={current ? t(`${prefix}.choose`) : t(`${prefix}.label`)}
          className={
            current
              ? "min-w-5 shrink-0 rounded-l-none border-l border-border-subtle px-1"
              : "min-w-0 gap-1"
          }
        >
          {!current && <span className="truncate">{t(`${prefix}.label`)}</span>}
          <ChevronDown size={12} aria-hidden="true" className="shrink-0" />
        </Button>
        <DropdownMenuContent placement="bottom start" className="external-tool-menu">
          {choices.map(({ id, name }) => (
            <DropdownMenuItem key={id} id={id} textValue={name} onSelect={() => void open(id)}>
              <span className="min-w-0 [overflow-wrap:anywhere]">{name}</span>
              {id === current?.id && (
                <span className="ms-auto shrink-0 text-xs text-text-muted">
                  {t("workspace.toolCurrent")}
                </span>
              )}
            </DropdownMenuItem>
          ))}
          {!choices.length && (
            <DropdownMenuItem id="empty" disabled>
              {error
                ? t("workspace.loadFailed", { error: formatAppError(error) })
                : isPending
                  ? t("common.loading")
                  : t(`${prefix}.noneDetected`)}
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
