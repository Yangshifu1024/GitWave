import { useCallback, useEffect, useState } from "react";
import type { ConflictFile, ConflictSides } from "@/lib/api";
import {
  abortMerge,
  explainConflict,
  formatAppError,
  getConflictSides,
  listConflicts,
  mergeInProgress,
  resolveConflict,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { Button } from "@/components/ui/Button";
import { ListItem } from "@/components/ui/ListItem";
import { ErrorAlert } from "@/components/ui/ErrorAlert";
import { InputGroup, TextField } from "@heroui/react";
import { AlertTriangle, Sparkles, XCircle } from "lucide-react";

export function ConflictPanel(): React.JSX.Element | null {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);

  const [active, setActive] = useState(false);
  const [files, setFiles] = useState<ConflictFile[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [sides, setSides] = useState<ConflictSides | null>(null);
  const [editor, setEditor] = useState("");
  const [explain, setExplain] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = useCallback(async () => {
    if (!workspaceId) return;
    const inProgress = await mergeInProgress(workspaceId);
    setActive(inProgress);
    if (!inProgress) {
      setFiles([]);
      setSelected(null);
      setSides(null);
      return;
    }
    const list = await listConflicts(workspaceId);
    setFiles(list);
  }, [workspaceId]);

  useEffect(() => {
    if (!workspaceId || !repoId) {
      setActive(false);
      setFiles([]);
      return;
    }
    refresh().catch((e) => setError(formatAppError(e)));
    const t = window.setInterval(() => {
      void refresh().catch(() => undefined);
    }, 3000);
    return () => window.clearInterval(t);
  }, [workspaceId, repoId, refresh]);

  const openFile = async (path: string) => {
    if (!workspaceId) return;
    setSelected(path);
    setExplain(null);
    setError(null);
    try {
      const s = await getConflictSides(workspaceId, path);
      setSides(s);
      setEditor(s.working ?? s.ours ?? s.theirs ?? "");
    } catch (e) {
      setError(formatAppError(e));
    }
  };

  if (!workspaceId || !repoId || !active) return null;

  return (
    <div className="fixed inset-0 z-modal flex items-center justify-center">
      <div className="fixed inset-0 bg-bg-overlay backdrop-blur-sm" />
      <div className="relative z-10 w-[min(1100px,95vw)] h-[min(720px,90vh)] rounded-xl bg-bg-elevated shadow-modal flex flex-col overflow-hidden">
        <div className="shrink-0 flex items-center justify-between px-4 py-3 border-b border-border-subtle">
          <div className="flex items-center gap-2">
            <AlertTriangle size={16} className="text-warning" />
            <h2 className="text-sm font-semibold text-text-primary">
              Merge conflicts ({files.length})
            </h2>
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="danger"
              size="sm"
              disabled={busy}
              onClick={() => {
                void (async () => {
                  setBusy(true);
                  try {
                    await abortMerge(workspaceId);
                    bumpHistory();
                    await refresh();
                  } catch (e) {
                    setError(formatAppError(e));
                  } finally {
                    setBusy(false);
                  }
                })();
              }}
            >
              <XCircle size={14} />
              Abort merge
            </Button>
          </div>
        </div>

        {error ? <ErrorAlert message={error} onDismiss={() => setError(null)} /> : null}

        <div className="flex flex-1 min-h-0 overflow-hidden">
          <div className="w-64 shrink-0 border-r border-border-subtle overflow-auto">
            {files.length === 0 ? (
              <p className="p-3 text-xs text-text-muted">
                No conflicted paths left. Commit the merge from Working Copy when ready.
              </p>
            ) : (
              files.map((f) => (
                <ListItem
                  key={f.path}
                  selected={selected === f.path}
                  onClick={() => void openFile(f.path)}
                >
                  <span className="text-sm font-mono truncate">{f.path}</span>
                </ListItem>
              ))
            )}
          </div>

          <div className="flex-1 min-w-0 flex flex-col overflow-hidden">
            {!sides ? (
              <div className="flex-1 flex items-center justify-center text-sm text-text-muted">
                Select a conflicted file
              </div>
            ) : (
              <>
                <div className="shrink-0 flex items-center gap-2 px-3 py-2 border-b border-border-subtle">
                  <span className="text-xs font-mono text-text-secondary truncate flex-1">
                    {sides.path}
                  </span>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={busy}
                    onClick={() => setEditor(sides.ours ?? "")}
                  >
                    Use ours
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={busy}
                    onClick={() => setEditor(sides.theirs ?? "")}
                  >
                    Use theirs
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={busy}
                    onClick={() => {
                      void (async () => {
                        setBusy(true);
                        setExplain(null);
                        try {
                          setExplain(await explainConflict(workspaceId, sides.path));
                        } catch (e) {
                          setError(formatAppError(e));
                        } finally {
                          setBusy(false);
                        }
                      })();
                    }}
                    title="AI explain (advice only)"
                  >
                    <Sparkles size={14} />
                    Explain
                  </Button>
                  <Button
                    variant="primary"
                    size="sm"
                    disabled={busy}
                    onClick={() => {
                      void (async () => {
                        setBusy(true);
                        try {
                          await resolveConflict(workspaceId, sides.path, editor);
                          bumpHistory();
                          await refresh();
                          setSelected(null);
                          setSides(null);
                        } catch (e) {
                          setError(formatAppError(e));
                        } finally {
                          setBusy(false);
                        }
                      })();
                    }}
                  >
                    Mark resolved
                  </Button>
                </div>

                <div className="grid grid-cols-3 gap-px bg-border-subtle shrink-0 max-h-36 overflow-hidden border-b border-border-subtle">
                  {(
                    [
                      ["Base", sides.base],
                      ["Ours", sides.ours],
                      ["Theirs", sides.theirs],
                    ] as const
                  ).map(([label, text]) => (
                    <div key={label} className="bg-bg-secondary overflow-auto p-2">
                      <p className="text-[10px] uppercase tracking-wide text-text-muted mb-1">
                        {label}
                      </p>
                      <pre className="text-[10px] font-mono text-text-secondary whitespace-pre-wrap">
                        {text ?? "(missing)"}
                      </pre>
                    </div>
                  ))}
                </div>

                <div className="flex flex-1 min-h-0 overflow-hidden">
                  <TextField
                    value={editor}
                    onChange={setEditor}
                    className="flex-1 min-w-0 flex flex-col"
                    aria-label="Resolved file content"
                  >
                    <InputGroup
                      fullWidth
                      className="flex-1 min-h-0 border-0 shadow-none bg-transparent"
                    >
                      <InputGroup.TextArea
                        className="flex-1 min-w-0 h-full p-3 font-mono text-xs bg-bg-primary text-text-primary border-0 shadow-none resize-none"
                        spellCheck={false}
                      />
                    </InputGroup>
                  </TextField>
                  {explain ? (
                    <div className="w-80 shrink-0 border-l border-border-subtle overflow-auto p-3 bg-bg-secondary">
                      <p className="text-xs font-medium text-text-secondary mb-2">
                        AI explanation (reference only — not applied)
                      </p>
                      <p className="text-xs text-text-primary whitespace-pre-wrap">{explain}</p>
                    </div>
                  ) : null}
                </div>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
