import { Fragment, useDeferredValue, useEffect, useMemo, useRef, useState } from "react";
import type { ConflictSides } from "@/lib/api";
import { explainConflict, formatAppError, getConflictSides, resolveConflict } from "@/lib/api";
import { classifyConflictLine, findConflictRegions, lineStartOffset } from "@/lib/conflictMarkers";
import type { MergeConflictsState } from "@/hooks/useMergeConflicts";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/Button";
import { ErrorAlert } from "@/components/ui/ErrorAlert";
import { Modal } from "@/components/ui/Modal";
import { useToast } from "@/components/ui/Toast";
import { AlertTriangle, ChevronDown, ChevronUp, Sparkles, X, XCircle } from "lucide-react";

/** text-xs + leading-5 — editor and backdrop must agree for hunk jumps. */
const LINE_HEIGHT_PX = 20;
/** py-2 on the editor — hunk scroll aims one padding above the target line. */
const EDITOR_PAD_Y = 8;

interface ConflictPanelProps {
  /** Controlled by App (Merge banner's Resolve button). */
  open: boolean;
  onClose: () => void;
  merge: MergeConflictsState;
}

export function ConflictPanel({
  open,
  onClose,
  merge,
}: ConflictPanelProps): React.JSX.Element | null {
  const workspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const repoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const bumpHistory = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const { active, files, refresh, abort } = merge;

  const [selected, setSelected] = useState<string | null>(null);
  const [sides, setSides] = useState<ConflictSides | null>(null);
  const [editor, setEditor] = useState("");
  const [explain, setExplain] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hunkIndex, setHunkIndex] = useState(0);
  const [discardPrompt, setDiscardPrompt] = useState(false);
  const editorRef = useRef<HTMLTextAreaElement>(null);
  const backdropRef = useRef<HTMLPreElement>(null);
  /** Editor content the current buffer was seeded from — dirty check on close. */
  const seedRef = useRef("");
  const openRef = useRef(open);
  const { toast } = useToast();

  // The highlight backdrop derives from a deferred copy of the editor so a
  // keystroke never blocks on re-highlighting a large file (lockfile-sized
  // conflicts). The conflict count follows one beat behind while typing.
  const resolved = useDeferredValue(editor);
  const regions = useMemo(() => findConflictRegions(resolved), [resolved]);
  const lines = useMemo(() => resolved.split("\n"), [resolved]);

  const currentHunk = Math.min(hunkIndex, Math.max(0, regions.length - 1));

  useEffect(() => {
    openRef.current = open;
  }, [open]);

  const openFile = async (path: string) => {
    if (!workspaceId) return;
    setSelected(path);
    setExplain(null);
    setError(null);
    try {
      const s = await getConflictSides(workspaceId, path);
      if (!openRef.current) return; // panel closed while loading
      setSides(s);
      const seed = s.working ?? s.ours ?? s.theirs ?? "";
      seedRef.current = seed;
      setEditor(seed);
    } catch (e) {
      if (openRef.current) setError(formatAppError(e));
    }
  };

  // Reset hunk navigation when switching files.
  useEffect(() => {
    setHunkIndex(0);
  }, [selected]);

  // Clear in-progress state while hidden so reopening starts fresh.
  useEffect(() => {
    if (!open) {
      setSelected(null);
      setSides(null);
      setExplain(null);
      setEditor("");
      setError(null);
      setDiscardPrompt(false);
    }
  }, [open]);

  // Resolving the last conflict returns to the main window; a merge that
  // ends underneath us (e.g. committed in a terminal) does the same, so a
  // stale editor can never be carried into the next merge.
  useEffect(() => {
    if (open && active && files.length === 0) onClose();
    if (open && !active) onClose();
  }, [open, active, files.length, onClose]);

  const requestClose = () => {
    if (editor !== seedRef.current) setDiscardPrompt(true);
    else onClose();
  };
  // Keep the Escape listener free of per-render re-subscription.
  const requestCloseRef = useRef(requestClose);
  requestCloseRef.current = requestClose;

  // Escape closes back to the main window (with a dirty check).
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") requestCloseRef.current();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);

  // Per-line highlight classes for the backdrop: markers stand out, region
  // bodies get a light tint. Regions are sorted, so one forward pointer
  // classifies every line in O(n).
  const highlighted = useMemo(() => {
    let ri = 0;
    return lines.map((line, i) => {
      while (ri < regions.length && i > (regions[ri]?.end ?? -1)) ri += 1;
      const region = regions[ri];
      const inRegion = region !== undefined && i >= region.start;
      const kind = classifyConflictLine(line);
      if (kind === "ours" || kind === "theirs") {
        return { cls: "bg-conflict-marker-bg font-semibold text-danger" };
      }
      if (kind) {
        return { cls: "bg-conflict-marker-bg font-semibold text-text-secondary" };
      }
      return { cls: inRegion ? "bg-conflict-region-bg" : "" };
    });
  }, [lines, regions]);

  if (!open || !active || !workspaceId || !repoId) return null;

  const gotoHunk = (index: number) => {
    if (regions.length === 0) return;
    const target = Math.max(0, Math.min(index, regions.length - 1));
    setHunkIndex(target);
    const ta = editorRef.current;
    const region = regions[target];
    if (!ta || !region) return;
    const startOffset = lineStartOffset(resolved, region.start);
    const endOffset = region.closed
      ? lineStartOffset(resolved, region.end) + (lines[region.end]?.length ?? 0)
      : startOffset;
    // Selection first, scroll last: focusing a selection scrolls its active
    // end into view, and our explicit top-alignment must win.
    ta.focus();
    ta.setSelectionRange(startOffset, endOffset);
    // wrap="off" keeps logical lines == visual lines, so line × height is exact.
    ta.scrollTop = Math.max(0, region.start * LINE_HEIGHT_PX - EDITOR_PAD_Y);
  };

  const syncScroll = () => {
    const ta = editorRef.current;
    const pre = backdropRef.current;
    if (ta && pre) {
      pre.scrollTop = ta.scrollTop;
      pre.scrollLeft = ta.scrollLeft;
    }
  };

  const handleAbort = () => {
    void (async () => {
      setBusy(true);
      try {
        await abort();
        bumpHistory();
      } catch (e) {
        if (openRef.current) setError(formatAppError(e));
      } finally {
        setBusy(false);
      }
    })();
  };

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
            <Button variant="danger" size="sm" disabled={busy} onClick={handleAbort}>
              <XCircle size={14} />
              Abort merge
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="p-1"
              aria-label="Close"
              title="Close"
              onClick={requestClose}
            >
              <X size={14} />
            </Button>
          </div>
        </div>

        {error ? <ErrorAlert message={error} onDismiss={() => setError(null)} /> : null}

        <div className="flex flex-1 min-h-0 overflow-hidden">
          <div
            role="listbox"
            aria-label="Conflicted files"
            className="w-64 shrink-0 border-r border-border-subtle overflow-auto select-none px-1 py-1"
          >
            {files.length === 0 ? (
              // Transient only — the auto-close effect hides the panel right
              // after the list empties; kept as a defensive empty state.
              <p className="p-3 text-xs text-text-muted">
                No conflicted paths left. Commit the merge from Working Copy when ready.
              </p>
            ) : (
              files.map((f) => {
                // Row style mirrors the commit modal's unstaged list
                // (ui/FileListItem): the name is a label, not code.
                const name = f.path.split("/").pop() ?? f.path;
                return (
                  <div
                    key={f.path}
                    role="option"
                    aria-selected={selected === f.path}
                    tabIndex={0}
                    title={f.path}
                    onClick={() => void openFile(f.path)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") void openFile(f.path);
                    }}
                    className={cn(
                      "flex items-center px-3 py-1.5 rounded-md text-xs cursor-pointer transition-colors duration-fast",
                      selected === f.path ? "bg-accent/10" : "hover:bg-bg-secondary",
                    )}
                  >
                    <span className="flex-1 min-w-0 truncate text-text-primary">{name}</span>
                  </div>
                );
              })
            )}
          </div>

          <div className="flex-1 min-w-0 flex flex-col overflow-hidden">
            {!sides ? (
              <div className="flex-1 flex items-center justify-center text-sm text-text-muted">
                Select a conflicted file
              </div>
            ) : (
              <>
                {/* Row 1: path + conflict-hunk navigation (count is live). */}
                <div className="shrink-0 flex items-center gap-2 px-3 py-2 border-b border-border-subtle">
                  <span
                    className="text-xs font-mono text-text-secondary truncate flex-1"
                    title={sides.path}
                  >
                    {sides.path}
                  </span>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="p-1"
                    aria-label="Previous conflict"
                    title="Previous conflict"
                    disabled={regions.length === 0 || currentHunk <= 0}
                    onClick={() => gotoHunk(currentHunk - 1)}
                  >
                    <ChevronUp size={14} />
                  </Button>
                  <span
                    className={cn(
                      "shrink-0 text-xs tabular-nums",
                      regions.length === 0 ? "text-text-muted" : "text-text-secondary",
                    )}
                  >
                    {regions.length === 0
                      ? "No conflicts"
                      : `${regions.length} conflict${regions.length === 1 ? "" : "s"}`}
                  </span>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="p-1"
                    aria-label="Next conflict"
                    title="Next conflict"
                    disabled={regions.length === 0 || currentHunk >= regions.length - 1}
                    onClick={() => gotoHunk(currentHunk + 1)}
                  >
                    <ChevronDown size={14} />
                  </Button>
                </div>

                {/* Row 2: per-file actions. */}
                <div className="shrink-0 flex items-center gap-2 px-3 py-1.5 border-b border-border-subtle">
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={busy}
                    onClick={() => {
                      seedRef.current = sides.ours ?? "";
                      setEditor(sides.ours ?? "");
                    }}
                  >
                    Use ours
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={busy}
                    onClick={() => {
                      seedRef.current = sides.theirs ?? "";
                      setEditor(sides.theirs ?? "");
                    }}
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
                          const res = await explainConflict(workspaceId, sides.path);
                          if (!openRef.current) return;
                          setExplain(res.text);
                          if (res.used_fallback) {
                            toast({
                              title: `Primary AI provider failed — response served by ${res.provider_used}`,
                              variant: "info",
                            });
                          }
                        } catch (e) {
                          if (openRef.current) setError(formatAppError(e));
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
                          if (!openRef.current) return;
                          setSelected(null);
                          setSides(null);
                        } catch (e) {
                          if (openRef.current) setError(formatAppError(e));
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
                      ["Ours", sides.ours],
                      ["Base", sides.base],
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
                  {/* Highlight-within-textarea: a transparent-text textarea on
                      top of an identically styled <pre>, scroll-synced. */}
                  <div className="relative flex-1 min-w-0 bg-bg-primary">
                    <pre
                      ref={backdropRef}
                      aria-hidden
                      className="absolute inset-0 overflow-hidden px-3 py-2 font-mono text-xs leading-5 text-text-primary whitespace-pre select-none pointer-events-none"
                    >
                      {highlighted.map((h, i) => (
                        <Fragment key={i}>
                          <span className={h.cls}>{lines[i]}</span>
                          {"\n"}
                        </Fragment>
                      ))}
                    </pre>
                    <textarea
                      ref={editorRef}
                      value={editor}
                      onChange={(e) => setEditor(e.target.value)}
                      onScroll={syncScroll}
                      wrap="off"
                      spellCheck={false}
                      aria-label="Resolved file content"
                      className="absolute inset-0 h-full w-full resize-none overflow-auto bg-transparent px-3 py-2 font-mono text-xs leading-5 text-transparent caret-text-primary outline-none border-0"
                    />
                  </div>
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

      <Modal
        open={discardPrompt}
        onOpenChange={(o) => !o && setDiscardPrompt(false)}
        title="Discard unresolved edits?"
        description="The editor content differs from the version it was seeded with. Closing now discards manual edits — use Mark resolved to keep them."
        size="sm"
      >
        <div className="flex justify-end gap-2">
          <Button variant="secondary" size="sm" onClick={() => setDiscardPrompt(false)}>
            Keep editing
          </Button>
          <Button
            variant="danger"
            size="sm"
            onClick={() => {
              setDiscardPrompt(false);
              onClose();
            }}
          >
            Discard edits
          </Button>
        </div>
      </Modal>
    </div>
  );
}
