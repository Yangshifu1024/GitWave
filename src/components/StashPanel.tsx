import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import type { DiffSummary } from "@/lib/api";
import {
  applyStash,
  dropStash,
  formatAppError,
  getStashDiff,
  listStashes,
  popStash,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { ListItem } from "@/components/ui/ListItem";
import { SidebarSection } from "@/components/ui/SidebarSection";
import { ErrorAlert } from "@/components/ui/ErrorAlert";
import { cn } from "@/lib/utils";
import { Archive, Eye, Play, Trash2, Upload } from "lucide-react";

export function StashPanel({ compact = false }: { compact?: boolean }): React.JSX.Element {
  const { t } = useTranslation();
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const queryClient = useQueryClient();

  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedOid, setSelectedOid] = useState<string | null>(null);
  const [diff, setDiff] = useState<DiffSummary | null>(null);

  const { data: entries = [], error: listError } = useQuery({
    queryKey: ["stashes", workspaceId],
    queryFn: () => listStashes(workspaceId!),
    enabled: Boolean(workspaceId && repoId),
  });

  const refresh = (): void => {
    void queryClient.invalidateQueries({ queryKey: ["stashes", workspaceId] });
  };

  const run = async (fn: () => Promise<void>) => {
    if (!workspaceId || busy) return;
    setBusy(true);
    setError(null);
    try {
      await fn();
      refresh();
      bumpHistory();
    } catch (e) {
      setError(formatAppError(e));
    } finally {
      setBusy(false);
    }
  };

  const handleDiff = async (oid: string) => {
    if (!workspaceId) return;
    setSelectedOid(oid);
    setError(null);
    try {
      setDiff(await getStashDiff(workspaceId, oid));
    } catch (e) {
      setDiff(null);
      setError(formatAppError(e));
    }
  };

  // Empty dataset = static header (nothing to expand); with entries the
  // section is expandable. Load errors stay expandable so they are readable.
  const hasEntries = entries.length > 0;
  const collapsible = hasEntries || Boolean(listError);

  return (
    <SidebarSection title={t("changes.stash.title")} collapsible={collapsible}>
      {listError ? (
        <p className="px-3 py-1.5 text-xs text-danger">{formatAppError(listError)}</p>
      ) : null}

      <ErrorAlert message={error} onDismiss={() => setError(null)} />

      <div className={cn("min-h-0", !compact && "flex flex-col overflow-hidden")}>
        {entries.length === 0 ? (
          <p
            className={cn(
              "text-text-muted",
              compact ? "px-3 py-1.5 text-xs" : "px-4 py-2 text-center text-sm",
            )}
          >
            {t("changes.stash.noStashes")}
          </p>
        ) : (
          entries.map((e) => (
            <ListItem
              key={`${e.index}-${e.oid}`}
              selected={selectedOid === e.oid}
              onClick={() => {
                void handleDiff(e.oid);
              }}
              leading={<Archive size={14} className="text-accent shrink-0" />}
              trailing={
                <div className="flex items-center gap-0.5">
                  {!compact ? (
                    <Button
                      variant="ghost"
                      size="sm"
                      className="p-1"
                      disabled={busy}
                      title={t("changes.stash.viewDiff")}
                      onClick={(ev) => {
                        ev.stopPropagation();
                        void handleDiff(e.oid);
                      }}
                    >
                      <Eye size={12} />
                    </Button>
                  ) : null}
                  <Button
                    variant="ghost"
                    size="sm"
                    className="p-1"
                    disabled={busy}
                    title={t("changes.stash.apply")}
                    onClick={(ev) => {
                      ev.stopPropagation();
                      void run(async () => applyStash(workspaceId!, e.index));
                    }}
                  >
                    <Play size={12} />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="p-1"
                    disabled={busy}
                    title={t("changes.stash.pop")}
                    onClick={(ev) => {
                      ev.stopPropagation();
                      void run(async () => popStash(workspaceId!, e.index));
                    }}
                  >
                    <Upload size={12} />
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="p-1 text-danger hover:bg-danger/10"
                    disabled={busy}
                    title={t("changes.stash.drop")}
                    onClick={(ev) => {
                      ev.stopPropagation();
                      void run(async () => dropStash(workspaceId!, e.index));
                    }}
                  >
                    <Trash2 size={12} />
                  </Button>
                </div>
              }
            >
              <div className="flex flex-col min-w-0">
                <span className={cn("text-text-primary truncate", compact ? "text-xs" : "text-sm")}>
                  {`stash@{${e.index}}`} · {e.message || t("changes.stash.noMessage")}
                </span>
                <span className="text-[10px] text-text-muted font-mono">{e.oid.slice(0, 7)}</span>
              </div>
            </ListItem>
          ))
        )}

        {!compact ? (
          <div className="w-80 shrink-0 overflow-auto p-3">
            {!diff ? (
              <p className="text-xs text-text-muted">{t("changes.stash.selectToPreview")}</p>
            ) : (
              <div className="space-y-2">
                <p className="text-xs text-text-secondary">
                  {t("changes.stash.diffSummary", {
                    files: diff.files.length,
                    additions: diff.total_additions,
                    deletions: diff.total_deletions,
                  })}
                </p>
                {diff.files.map((f) => (
                  <div
                    key={f.path}
                    className="rounded border border-border-subtle px-2 py-1.5 text-xs font-mono"
                  >
                    <div className="text-text-primary truncate">{f.path}</div>
                    <div className="text-text-muted">
                      <span className="text-success">+{f.additions}</span>{" "}
                      <span className="text-danger">−{f.deletions}</span>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        ) : null}
      </div>
    </SidebarSection>
  );
}
