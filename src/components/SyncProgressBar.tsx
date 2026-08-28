import { ProgressBar } from "@heroui/react";
import { useSyncStore } from "@/stores/syncStore";
import { cn } from "@/lib/utils";
import type { SyncOperation } from "@/lib/api";

const OP_LABELS: Record<SyncOperation, string> = {
  fetch: "Fetching",
  pull: "Pulling",
  push: "Pushing",
};

export function SyncProgressBar(): React.JSX.Element | null {
  const activeOp = useSyncStore((s) => s.activeOp);
  const receivedObjects = useSyncStore((s) => s.receivedObjects);
  const totalObjects = useSyncStore((s) => s.totalObjects);
  const fading = useSyncStore((s) => s.fading);

  if (!activeOp) return null;

  const determinate = totalObjects > 0;

  return (
    <ProgressBar
      aria-label={OP_LABELS[activeOp]}
      minValue={0}
      maxValue={determinate ? totalObjects : 100}
      value={determinate ? receivedObjects : 0}
      isIndeterminate={!determinate}
      className={cn(
        "absolute bottom-0 inset-x-0 z-30",
        "transition-opacity duration-fast",
        fading && "opacity-0",
      )}
    >
      <ProgressBar.Track className="h-0.5 rounded-none bg-accent/15 overflow-hidden">
        <ProgressBar.Fill
          className={cn(
            "h-full rounded-none bg-accent",
            !determinate && "w-full sync-progress-indeterminate",
          )}
        />
      </ProgressBar.Track>
    </ProgressBar>
  );
}

export function syncOperationLabel(op: SyncOperation | null): string | null {
  if (!op) return null;
  switch (op) {
    case "fetch":
      return "Fetching from origin…";
    case "pull":
      return "Pulling changes…";
    case "push":
      return "Pushing to origin…";
  }
}
