import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import type { InteractiveRebaseAction, InteractiveRebaseTodo } from "@/lib/api";
import { executeInteractiveRebase, formatAppError, planInteractiveRebase } from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { Select } from "@/components/ui/Select";
import { InputGroup, TextField } from "@heroui/react";
import { GripVertical } from "lucide-react";
import { cn } from "@/lib/utils";

const ACTIONS: InteractiveRebaseAction[] = ["pick", "reword", "edit", "squash", "fixup", "drop"];

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
  const { t } = useTranslation();
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
      prev.map((t, i) =>
        i === index ? { ...t, action, message: action === "reword" ? t.message : t.message } : t,
      ),
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
          setError(result.conflicts.join("; ") || t("branches.irebase.conflictError"));
          return;
        }
        if (result.kind === "paused_for_edit") {
          onDone(t("branches.irebase.paused", { name: upstream }));
          onClose();
          return;
        }
        onDone(
          t("branches.irebase.done", { name: upstream, kind: result.kind.replace(/_/g, " ") }),
        );
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
      title={t("branches.irebase.title", { name: upstream })}
      size="lg"
      footer={
        <>
          <Button variant="secondary" size="sm" disabled={busy} onClick={onClose}>
            {t("branches.cancel")}
          </Button>
          <Button
            variant="primary"
            size="sm"
            disabled={busy || loading || todos.length === 0}
            onClick={run}
          >
            {t("branches.irebase.start")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3 max-h-[60vh]">
        <p className="text-xs text-text-muted">{t("branches.irebase.hint")}</p>
        {error ? <p className="text-xs text-danger">{error}</p> : null}
        {loading ? (
          <p className="text-sm text-text-muted">{t("branches.irebase.loading")}</p>
        ) : todos.length === 0 ? (
          <p className="text-sm text-text-muted">
            {t("branches.irebase.upToDate", { name: upstream })}
          </p>
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
                  <Select
                    aria-label={t("branches.irebase.actionAria")}
                    className="h-auto w-auto flex-none px-1 py-0.5 text-xs"
                    value={todo.action}
                    disabled={busy}
                    onChange={(action) => updateAction(index, action as InteractiveRebaseAction)}
                    options={ACTIONS.map((a) => ({ value: a, label: a }))}
                  />
                  <span className="font-mono text-[11px] text-text-muted shrink-0">
                    {todo.oid.slice(0, 7)}
                  </span>
                  <span className="text-xs text-text-primary truncate flex-1">{todo.summary}</span>
                </div>
                {todo.action === "reword" || todo.action === "squash" ? (
                  <TextField
                    value={todo.message ?? ""}
                    onChange={(message) => updateMessage(index, message)}
                    isDisabled={busy}
                    className="w-full"
                  >
                    <InputGroup fullWidth className="rounded">
                      <InputGroup.TextArea
                        className="w-full min-h-[48px] resize-y p-1.5 font-mono text-xs text-text-primary"
                        placeholder={
                          todo.action === "reword"
                            ? t("branches.irebase.rewordPlaceholder")
                            : t("branches.irebase.squashPlaceholder")
                        }
                      />
                    </InputGroup>
                  </TextField>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </div>
    </Modal>
  );
}
