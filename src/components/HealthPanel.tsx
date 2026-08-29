import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Activity, Loader2, Sparkles } from "lucide-react";

import { explainHealth, formatAppError, getHealth, type HealthReport } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/Toast";
import { cn } from "@/lib/utils";

function mb(bytes: number): string {
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function MetricRow({
  label,
  value,
  warn,
  detail,
}: {
  label: string;
  value: string;
  warn?: boolean;
  detail?: string;
}): React.JSX.Element {
  return (
    <div className="flex min-w-0 items-baseline gap-2 px-2 py-1">
      <span className={cn("shrink-0 text-[11px]", warn ? "text-warning" : "text-text-muted")}>
        {label}
      </span>
      <span
        className={cn(
          "min-w-0 flex-1 truncate text-right text-[11px]",
          warn ? "font-medium text-warning" : "text-text-secondary",
        )}
        title={detail ?? value}
      >
        {value}
      </span>
    </div>
  );
}

function renderReport(report: HealthReport): React.JSX.Element {
  const large = report.large_files.map((f) => `${f.path} (${mb(f.size_bytes)})`).join(", ");
  return (
    <>
      <MetricRow
        label="Unpushed"
        value={
          report.unpushed === null
            ? "no upstream"
            : report.unpushed === 0
              ? "in sync"
              : `${report.unpushed} commits`
        }
        warn={report.unpushed !== null && report.unpushed > 0}
      />
      <MetricRow
        label="Conflict residue"
        value={report.conflict_residue.length ? report.conflict_residue.join(", ") : "none"}
        warn={report.conflict_residue.length > 0}
        detail="Leftover MERGE_HEAD / REVERT_HEAD / CHERRY_PICK_HEAD state files"
      />
      <MetricRow
        label="Dirty files"
        value={String(report.dirty_files)}
        warn={report.dirty_files > 0}
      />
      <MetricRow
        label="Stale branches"
        value={report.stale_branches.length ? report.stale_branches.join(", ") : "none (30 days)"}
        warn={report.stale_branches.length > 0}
      />
      <MetricRow
        label="Large files"
        value={large || "none (>1 MB)"}
        warn={report.large_files.length > 0}
      />
      <MetricRow label="Branches / tags" value={`${report.branch_count} / ${report.tag_count}`} />
    </>
  );
}

/**
 * M3 repo health dashboard: deterministic local metrics with an optional
 * AI summary (advice only). The numbers never depend on AI availability.
 */
export function HealthPanel(): React.JSX.Element {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const { toast } = useToast();

  const {
    data: report,
    isLoading,
    error,
  } = useQuery({
    queryKey: ["health", workspaceId, repoId],
    queryFn: () => getHealth(workspaceId!),
    enabled: Boolean(workspaceId && repoId),
  });

  const [aiText, setAiText] = useState<string | null>(null);
  const [aiBusy, setAiBusy] = useState(false);

  if (!workspaceId || !repoId) return <></>;
  if (error) {
    return <p className="px-2 py-1 text-xs text-danger">{formatAppError(error)}</p>;
  }

  const runSummary = (): void => {
    if (aiBusy) return;
    setAiBusy(true);
    explainHealth(workspaceId)
      .then(setAiText)
      .catch((e) => toast({ title: formatAppError(e), variant: "danger" }))
      .finally(() => setAiBusy(false));
  };

  return (
    <div className="flex flex-col gap-1 px-1 pb-1">
      {isLoading ? (
        <p className="px-2 py-1 text-xs text-text-muted italic">Measuring…</p>
      ) : report ? (
        <div className="flex flex-col">
          <div className="flex items-center gap-2 px-2 pt-1">
            <Activity size={12} className="text-text-muted" />
            <span className="text-[11px] font-medium text-text-secondary">Repo health</span>
          </div>
          {renderReport(report)}
        </div>
      ) : null}

      <div className="flex flex-col gap-1">
        <Button
          variant="secondary"
          size="sm"
          className="self-start px-2 text-[11px]"
          disabled={aiBusy || !report}
          onClick={runSummary}
        >
          {aiBusy ? (
            <Loader2 size={12} className="animate-spin" />
          ) : (
            <Sparkles size={12} />
          )}
          {aiBusy ? "Summarizing…" : "AI summary"}
        </Button>
      </div>

      {/* The AI summary lands in a dialog — the sidebar card stays compact. */}
      <Modal
        open={aiText !== null}
        onOpenChange={(o) => !o && setAiText(null)}
        title="AI summary"
        size="md"
      >
        <p className="text-xs leading-5 whitespace-pre-wrap text-text-secondary">{aiText}</p>
      </Modal>
    </div>
  );
}
