import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import type { BlameLine } from "@/lib/api";
import { formatAppError, getBlame } from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { formatAbsoluteTime } from "@/lib/commitTime";

function shortSha(sha: string): string {
  return sha.slice(0, 7);
}

function BlameGutter({ line }: { line: BlameLine }): React.JSX.Element {
  const { i18n } = useTranslation();
  const date = formatAbsoluteTime(line.time, "ymd", i18n.language);
  return (
    <div
      className="grid min-w-0 grid-cols-[7ch_minmax(0,1fr)] @lg/blame:sticky @lg/blame:left-0 @lg/blame:z-10 @lg/blame:grid-cols-[7ch_minmax(0,1fr)_auto] items-center gap-2 overflow-hidden border-r border-border-subtle bg-bg-panel px-2 text-xs"
      title={`${line.sha}\n${line.author} <${line.author_email}>\n${date}`}
    >
      <span className="font-mono text-accent">{shortSha(line.sha)}</span>
      <span className="min-w-0 truncate font-sans text-text-secondary">{line.author}</span>
      <span className="hidden whitespace-nowrap font-sans text-[10px] tabular-nums text-text-muted @lg/blame:block">
        {date}
      </span>
    </div>
  );
}

interface BlameLineRowProps {
  line: BlameLine;
  lineNo: number;
}

function BlameLineRow({ line, lineNo }: BlameLineRowProps): React.JSX.Element {
  return (
    <div className="grid min-w-full grid-cols-[12rem_3rem_minmax(0,1fr)] @lg/blame:grid-cols-[20rem_3rem_minmax(0,1fr)] text-xs font-mono leading-5 hover:bg-accent/5 transition-colors">
      <BlameGutter line={line} />
      <span className="@lg/blame:sticky @lg/blame:left-[20rem] @lg/blame:z-10 border-r border-border-subtle bg-bg-elevated pr-2 text-right text-text-muted select-none">
        {lineNo}
      </span>
      <span className="whitespace-pre px-3 text-text-primary select-text">{line.content}</span>
    </div>
  );
}

interface BlameViewProps {
  path: string;
  hideHeader?: boolean;
}

export function BlameView({ path, hideHeader = false }: BlameViewProps): React.JSX.Element {
  const { t } = useTranslation();
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const [lines, setLines] = useState<BlameLine[]>([]);
  const [loading, setLoading] = useState(Boolean(activeWorkspaceId && activeRepoId && path));
  const [error, setError] = useState<string | null>(null);

  const scopeKey = JSON.stringify([activeWorkspaceId, activeRepoId, path]);
  const [previousScope, setPreviousScope] = useState(scopeKey);
  if (previousScope !== scopeKey) {
    setPreviousScope(scopeKey);
    setLines([]);
    setError(null);
    setLoading(Boolean(activeWorkspaceId && activeRepoId && path));
  }
  useEffect(() => {
    if (!activeWorkspaceId || !activeRepoId || !path) return;
    let cancelled = false;
    getBlame(activeWorkspaceId, path, activeRepoId)
      .then((result) => {
        if (!cancelled) setLines(result);
      })
      .catch((e) => {
        if (!cancelled) setError(formatAppError(e));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [activeWorkspaceId, activeRepoId, path]);

  if (!activeWorkspaceId) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        {t("repo.blame.selectWorkspace")}
      </div>
    );
  }

  if (!activeRepoId) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        {t("repo.blame.selectRepo")}
      </div>
    );
  }

  if (!path) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        {t("repo.blame.selectFile")}
      </div>
    );
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        {t("repo.blame.loading")}
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center justify-center h-full text-danger text-sm px-4">
        {error}
      </div>
    );
  }

  if (lines.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-text-muted text-sm">
        {t("repo.blame.empty")}
      </div>
    );
  }

  return (
    <div className="@container/blame h-full min-h-0 overflow-auto">
      {/* File header */}
      {!hideHeader && (
        <div className="sticky top-0 z-20 px-4 py-2 bg-bg-elevated border-b border-border-subtle">
          <span className="text-sm font-medium text-text-primary">{path}</span>
        </div>
      )}

      {/* Lines */}
      <div className="w-max min-w-full font-mono">
        {lines.map((line) => (
          <BlameLineRow key={line.line_no} line={line} lineNo={line.line_no} />
        ))}
      </div>
    </div>
  );
}
