// ActionBar center status area — the single sync/operation status surface.
// Priority: in-flight op (label + indeterminate progress bar) > last
// operation result (persists until overwritten) > current branch name.
// The bottom bar is always visible and takes the state's color: gray when
// idle, accent while syncing, success/danger/info for the last result.

import { ProgressBar } from "@heroui/react";
import { Card as HeroCard } from "@heroui/react";

import { useWorkingCopy } from "@/hooks/useWorkingCopy";
import { cn } from "@/lib/utils";
// The `duration-150` transitions below must stay in sync with the sync
// store's OP_FADE_MS clear window.
import { useSyncStore, operationLabel } from "@/stores/syncStore";
import { useStatusAreaStore } from "@/stores/statusAreaStore";

type AreaState = "sync" | "success" | "danger" | "info" | "idle";

const TEXT_COLOR: Record<AreaState, string> = {
  sync: "text-accent",
  success: "text-success",
  danger: "text-danger",
  info: "text-accent",
  idle: "text-text-muted",
};

const BAR_COLOR: Record<AreaState, string> = {
  sync: "bg-accent",
  success: "bg-success",
  danger: "bg-danger",
  info: "bg-accent",
  idle: "bg-border-subtle",
};

export function SyncStatusArea(): React.JSX.Element {
  const activeOp = useSyncStore((s) => s.activeOp);
  const fading = useSyncStore((s) => s.fading);
  const status = useStatusAreaStore((s) => s.status);
  const wc = useWorkingCopy();

  // Stay mounted while fading so the 150ms opacity transition can play out
  // before the result state swaps in.
  const syncing = activeOp !== null;
  const state: AreaState = syncing ? "sync" : status ? status.variant : "idle";
  const label = syncing ? operationLabel(activeOp) : null;
  const text = syncing ? label : (status?.text ?? wc.data?.branch ?? "No repository selected");
  const fadeClass = fading ? "opacity-0" : "opacity-100";

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
          "transition-opacity duration-150",
          TEXT_COLOR[state],
          fadeClass,
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
          className={cn(
            "absolute bottom-0 inset-x-0 h-1 transition-opacity duration-150",
            fadeClass,
          )}
        >
          {/* No width utility on the Fill: HeroUI's built-in indeterminate
              animation owns both the 40% width and the translate sweep —
              overriding the width makes the full-width fill slide out of the
              clipped track and the bar vanish most of the cycle. */}
          <ProgressBar.Track className="h-1 rounded-none bg-accent/25 overflow-hidden">
            <ProgressBar.Fill className="h-full rounded-none bg-accent" />
          </ProgressBar.Track>
        </ProgressBar>
      ) : (
        <div
          aria-hidden
          className={cn(
            "absolute bottom-0 inset-x-0 h-1 transition-opacity duration-150",
            fadeClass,
          )}
        >
          <div className={cn("h-full w-full", BAR_COLOR[state])} />
        </div>
      )}
    </HeroCard>
  );
}
