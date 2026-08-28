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
  const percent = determinate ? Math.min(100, (receivedObjects / totalObjects) * 100) : 0;

  return (
    <div
      className={cn(
        "absolute bottom-0 inset-x-0 z-30 h-0.5 overflow-hidden bg-accent/15",
        "transition-opacity duration-fast",
        fading && "opacity-0",
      )}
      role="progressbar"
      aria-label={OP_LABELS[activeOp]}
      aria-valuemin={0}
      aria-valuemax={determinate ? totalObjects : undefined}
      aria-valuenow={determinate ? receivedObjects : undefined}
    >
      <div
        className={cn(
          "relative h-full bg-accent transition-[width] duration-fast ease-out",
          !determinate && "w-full sync-progress-indeterminate",
        )}
        style={determinate ? { width: `${percent}%` } : undefined}
      />
    </div>
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
