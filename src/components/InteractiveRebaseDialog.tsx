import { useEffect, useState } from "react";
import type { InteractiveRebaseAction, InteractiveRebaseTodo } from "@/lib/api";
import {
  executeInteractiveRebase,
  formatAppError,
  planInteractiveRebase,
} from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { GripVertical } from "lucide-react";
import { cn } from "@/lib/utils";

const ACTIONS: InteractiveRebaseAction[] = [
  "pick",
  "reword",
  "edit",
  "squash",
  "fixup",
  "drop",
];

interface InteractiveRebaseDialogProps {
  open: boolean;
  workspaceId: string;
  upstream: string;
  onClose: () => void;
  onDone: (notice: string) => void;
}

export function InteractiveRebaseDialog({
  open,
  workspaceId,
  upstream,
  onClose,
  onDone,
}: InteractiveRebaseDialogProps): React.JSX.Element {
  const [todos, setTodos] = useState<InteractiveRebaseTodo[]>([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [dragIndex, setDragIndex] = useState<number | null>(null);

  useEffect(() => {
    if (!open) return;
    setLoading(true);
    setError(null);
    planInteractiveRebase(workspaceId, upstream)
      .then(setTodos)
      .catch((e) => setError(formatAppError(e)))
      .finally(() => setLoading(false));
  }, [open, workspaceId, upstream]);

  const move = (from: number, to: number) => {
    if (to < 0 || to >= todos.length || from === to) return;
    setTodos((prev) => {
      const next = [...prev];
      const [item] = next.splice(from, 1);
      if (!item) return prev;
      next.splice(to, 0, item);
      return next;
    });
  };

  const updateAction = (index: number, action: InteractiveRebaseAction) => {
    setTodos((prev) =>
      prev.map((t, i) => (i === index ? { ...t, action, message: action === "reword" ? t.message : t.message } : t)),
    );
  };

  const updateMessage = (index: number, message: string) => {
    setTodos((prev) => prev.map((t, i) => (i === index ? { ...t, message } : t)));
  };

  const run = () => {
    void (async () => {
      setBusy(true);
      setError(null);
      try {
        const result = await executeInteractiveRebase(workspaceId, upstream, todos);
        if (result.kind === "conflicts") {
          setError(result.conflicts.join("; ") || "Rebase conflicts");
          return;
        }
        if (result.kind === "paused_for_edit") {
          onDone(
            `Paused for edit after applying a commit onto ${upstream}. Amend via Working Copy, then Continue from Branches.`,
          );
          onClose();
          return;
        }
        onDone(`Interactive rebase onto ${upstream} (${result.kind.replace(/_/g, " ")})`);
        onClose();
      } catch (e) {
        setError(formatAppError(e));
      } finally {
        setBusy(false);
      }
    })();
  };

  return (
    <Modal
      open={open}
      onOpenChange={(v) => {
        if (!v) onClose();
      }}
      title={`Interactive rebase onto ${upstream}`}
      size="lg"
    >
      <div className="flex flex-col gap-3 max-h-[60vh]">
        <p className="text-xs text-text-muted">
          Drag to reorder. Actions: pick / reword / edit / squash / fixup / drop. Does not auto-push.
        </p>
        {error ? <p className="text-xs text-danger">{error}</p> : null}
        {loading ? (
          <p className="text-sm text-text-muted">Loading commits…</p>
        ) : todos.length === 0 ? (
          <p className="text-sm text-text-muted">Already up to date with {upstream}.</p>
        ) : (
          <ul className="flex flex-col gap-1 overflow-auto border border-border-subtle rounded-md p-1">
            {todos.map((todo, index) => (
              <li
                key={todo.oid}
                draggable
                onDragStart={() => setDragIndex(index)}
                onDragOver={(e) => e.preventDefault()}
                onDrop={() => {
                  if (dragIndex !== null) move(dragIndex, index);
                  setDragIndex(null);
                }}
                className={cn(
                  "flex flex-col gap-1 rounded px-2 py-1.5 bg-bg-secondary",
                  dragIndex === index && "opacity-60",
                )}
              >
                <div className="flex items-center gap-2">
                  <GripVertical size={14} className="text-text-muted shrink-0 cursor-grab" />
                  <select
                    className="text-xs bg-bg-primary border border-border-subtle rounded px-1 py-0.5 text-text-primary"
                    value={todo.action}
                    disabled={busy}
                    onChange={(e) =>
                      updateAction(index, e.target.value as InteractiveRebaseAction)
                    }
                  >
                    {ACTIONS.map((a) => (
                      <option key={a} value={a}>
                        {a}
                      </option>
                    ))}
                  </select>
                  <span className="font-mono text-[11px] text-text-muted shrink-0">
                    {todo.oid.slice(0, 7)}
                  </span>
                  <span className="text-xs text-text-primary truncate flex-1">{todo.summary}</span>
                </div>
                {todo.action === "reword" || todo.action === "squash" ? (
                  <textarea
                    className="w-full text-xs font-mono bg-bg-primary border border-border-subtle rounded p-1.5 text-text-primary resize-y min-h-[48px]"
                    placeholder={todo.action === "reword" ? "New commit message" : "Combined message (optional)"}
                    value={todo.message ?? ""}
                    disabled={busy}
                    onChange={(e) => updateMessage(index, e.target.value)}
                  />
                ) : null}
              </li>
            ))}
          </ul>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="secondary" size="sm" disabled={busy} onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="primary"
            size="sm"
            disabled={busy || loading || todos.length === 0}
            onClick={run}
          >
            Start rebase
          </Button>
        </div>
      </div>
    </Modal>
  );
}
