import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { MergeConflictsState } from "@/hooks/useMergeConflicts";
import { formatAppError } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { AlertTriangle, XCircle } from "lucide-react";

interface MergeBannerProps {
  merge: MergeConflictsState;
  /** Opens the conflict panel (Resolve). */
  onResolve: () => void;
}

/**
 * In-flow notice between the top bars while a merge is in progress. Replaces
 * the conflict panel's automatic full-screen takeover: the main window stays
 * usable and Resolve opens the panel on demand.
 */
export function MergeBanner({ merge, onResolve }: MergeBannerProps): React.JSX.Element | null {
  const { active, mergeInProgress, files, abort } = merge;
  const { t } = useTranslation();
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const [busy, setBusy] = useState(false);
  const setStatus = useStatusAreaStore((s) => s.setStatus);

  if (!active) return null;
  const allResolved = files.length === 0;

  const handleAbort = () => {
    void (async () => {
      setBusy(true);
      try {
        await abort();
        bumpHistory();
      } catch (e) {
        setStatus(formatAppError(e), "danger");
      } finally {
        setBusy(false);
      }
    })();
  };

  return (
    <div className="shrink-0 flex items-center justify-between gap-3 border-b border-border-subtle bg-bg-secondary px-4 py-1.5">
      <p className="flex min-w-0 items-center gap-2 text-xs text-text-secondary">
        <AlertTriangle size={14} className="shrink-0 text-warning" />
        <span className="truncate">
          {mergeInProgress
            ? allResolved
              ? t("conflicts.banner.mergeResolved")
              : t("conflicts.banner.mergeCount", { count: files.length })
            : t("conflicts.banner.indexCount", { count: files.length })}
        </span>
      </p>
      <span className="flex shrink-0 items-center gap-2">
        <Button variant="primary" size="sm" disabled={busy || allResolved} onClick={onResolve}>
          {t("conflicts.banner.resolve")}
        </Button>
        {mergeInProgress ? (
          <Button variant="danger" size="sm" disabled={busy} onClick={handleAbort}>
            <XCircle size={14} />
            {t("conflicts.abortMerge")}
          </Button>
        ) : null}
      </span>
    </div>
  );
}
