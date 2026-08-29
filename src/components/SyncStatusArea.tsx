// ActionBar center status area — the single sync/operation status surface.
// Priority: in-flight sync op (label + indeterminate shimmer bar) > last
// operation result (persists until overwritten) > current branch name.
// The bottom bar is always visible and takes the state's color: gray when
// idle, accent while syncing, success/danger for the last result.

import { ProgressBar } from "@heroui/react";
import { Card as HeroCard } from "@heroui/react";

import { useWorkingCopy } from "@/hooks/useWorkingCopy";
import { cn } from "@/lib/utils";
import { useSyncStore, syncOperationLabel } from "@/stores/syncStore";
import { useStatusAreaStore } from "@/stores/statusAreaStore";

type AreaState = "sync" | "success" | "danger" | "idle";

const TEXT_COLOR: Record<AreaState, string> = {
  sync: "text-accent",
  success: "text-success",
  danger: "text-danger",
  idle: "text-text-muted",
};

const BAR_COLOR: Record<AreaState, string> = {
  sync: "bg-accent",
  success: "bg-success",
  danger: "bg-danger",
  idle: "bg-border-subtle",
};

export function SyncStatusArea(): React.JSX.Element {
  const activeOp = useSyncStore((s) => s.activeOp);
  const fading = useSyncStore((s) => s.fading);
  const status = useStatusAreaStore((s) => s.status);
  const wc = useWorkingCopy();

  const syncing = activeOp !== null && !fading;
  const state: AreaState = syncing ? "sync" : status ? status.variant : "idle";
  const label = syncing ? syncOperationLabel(activeOp) : null;
  const text = syncing ? label : (status?.text ?? wc.data?.branch ?? "No repository selected");

  return (
    <HeroCard
      aria-live="polite"
      className={cn(
        // 48px trial height (user comparing proportions vs the 28px buttons).
        "relative h-12 w-72 rounded-md border border-border-subtle bg-bg-elevated",
        "shadow-none p-0 gap-0 overflow-hidden",
      )}
    >
      <div
        className={cn(
          "flex h-full items-center justify-center gap-2 text-xs truncate",
          TEXT_COLOR[state],
        )}
      >
        <span className="truncate">{text}</span>
      </div>
      {state === "sync" ? (
        <ProgressBar
          aria-label={label ?? "Syncing"}
          minValue={0}
          maxValue={100}
          value={0}
          isIndeterminate
          className="absolute bottom-0 inset-x-0 h-1"
        >
          <ProgressBar.Track className="h-1 rounded-none bg-accent/25 overflow-hidden">
            <ProgressBar.Fill className="relative h-full w-full rounded-none bg-accent sync-progress-indeterminate" />
          </ProgressBar.Track>
        </ProgressBar>
      ) : (
        <div aria-hidden className="absolute bottom-0 inset-x-0 h-1">
          <div className={cn("h-full w-full", BAR_COLOR[state])} />
        </div>
      )}
    </HeroCard>
  );
}
