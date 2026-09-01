// ActionBar center status area — the single sync/operation status surface.
// Priority: in-flight op (label + indeterminate progress bar + cancel
// button for backend-cancellable ops) > last operation result (persists
// until overwritten) > current branch name. The bottom bar is always
// visible and takes the state's color: gray when idle, accent while
// syncing, success/danger/info for the last result.

import { useEffect, useState } from "react";
import { ProgressBar } from "@heroui/react";
import { Card as HeroCard } from "@heroui/react";
import { X } from "lucide-react";

import { cancelSync } from "@/lib/api";
import { useWorkingCopy } from "@/hooks/useWorkingCopy";
import { cn } from "@/lib/utils";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
// The `duration-150` transitions below must stay in sync with the sync
// store's OP_FADE_MS clear window.
import { useSyncStore, operationLabel, type ActiveOperation } from "@/stores/syncStore";
import { useStatusAreaStore } from "@/stores/statusAreaStore";

type AreaState = "sync" | "success" | "danger" | "info" | "idle";

/** Ops the backend can abort via the sync cancel flag: the network syncs
 * with progress. UI operations (checkout, stash, …) finish immediately;
 * the branch-delete flow mixes local and remote steps in one run, so it
 * stays outside for now (its network leg is still timeout-bounded). */
function isCancellableOp(op: ActiveOperation | null): boolean {
  return op === "fetch" || op === "pull" || op === "push";
}

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
  const activeRemote = useSyncStore((s) => s.activeRemote);
  const fading = useSyncStore((s) => s.fading);
  const status = useStatusAreaStore((s) => s.status);
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const wc = useWorkingCopy();
  const [cancelRequested, setCancelRequested] = useState(false);

  // A new operation resets the cancel button: the previous request died
  // with the operation it targeted.
  useEffect(() => {
    setCancelRequested(false);
  }, [activeOp, activeRemote]);

  const handleCancel = () => {
    setCancelRequested(true);
    if (workspaceId) {
      // The op itself reports the outcome; a failed cancel request only
      // means the 180s timeout backstop still applies.
      void cancelSync(workspaceId).catch((e) => console.warn("cancelSync failed:", e));
    }
  };

  // Stay mounted while fading so the 150ms opacity transition can play out
  // before the result state swaps in.
  const syncing = activeOp !== null;
  const cancellable = syncing && !fading && isCancellableOp(activeOp) && !cancelRequested;
  const state: AreaState = syncing ? "sync" : status ? status.variant : "idle";
  const label = syncing ? operationLabel(activeOp, activeRemote) : null;
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
          "flex h-full items-center justify-center gap-2 text-xs",
          "transition-opacity duration-150",
          TEXT_COLOR[state],
          fadeClass,
          // Padding keeps text off the card edges; the cancellable variant
          // swaps the right inset for the cancel button's reserved space.
          cancellable ? "pl-3 pr-8" : "px-3",
        )}
      >
        {/* Long labels (interpolated remote names) wrap to at most two
            lines, ellipsis beyond. min-w-0 lets the flex item shrink —
            without it the raw overflow is clipped, not ellipsized. */}
        <span className="min-w-0 line-clamp-2">{text}</span>
      </div>
      {cancellable ? (
        <button
          type="button"
          aria-label={label ? `${label} — cancel` : "Cancel operation"}
          onClick={handleCancel}
          className={cn(
            "absolute right-1 top-1/2 grid h-6 w-6 -translate-y-1/2 place-items-center rounded",
            "text-text-muted transition-colors hover:bg-danger/10 hover:text-danger",
          )}
        >
          <X className="h-3.5 w-3.5" aria-hidden />
        </button>
      ) : null}
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
