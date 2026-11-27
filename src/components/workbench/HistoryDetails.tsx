import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { CommitInfoHeader } from "@/components/CommitInfoHeader";
import { DiffViewer } from "@/components/DiffViewer";
import { EmptyState } from "@/components/ui/EmptyState";
import { WorkbenchSplit } from "@/components/ui/WorkbenchSplit";
import { HistoryFileTree } from "./HistoryFileTree";
import { buildChangedFileTree } from "@/lib/changedFileTree";
import { formatAppError, getDiffPreview } from "@/lib/api";
import { EMPTY_SESSION, useWorkbenchStore } from "@/stores/workbenchStore";
import { useLayoutStore } from "@/stores/layoutStore";

export function HistoryDetails({
  workspaceId,
  repoId,
  scope,
  sha,
}: {
  workspaceId: string;
  repoId: string;
  scope: string;
  sha: string | null;
}): React.JSX.Element {
  const { t } = useTranslation();
  const session = useWorkbenchStore((s) => s.sessions[scope] ?? EMPTY_SESSION);
  const update = useWorkbenchStore((s) => s.update);
  const focused = useLayoutStore((s) => s.inspectorMaximized);
  const request = { commit_oid: sha ?? undefined, expanded: false };
  const summary = useQuery({
    queryKey: ["diff-preview", workspaceId, repoId, request],
    queryFn: () => getDiffPreview(workspaceId, repoId, request),
    enabled: !!sha,
    retry: false,
  });
  const files = summary.data?.diff.files;
  const tree = useMemo(() => buildChangedFileTree(files ?? []), [files]);
  if (!sha)
    return (
      <EmptyState
        title={t("app.emptyState.noCommitTitle")}
        description={t("app.emptyState.noCommitDescription")}
        className="h-full"
      />
    );
  const remembered = session.historyFile?.commit === sha ? session.historyFile.path : null;
  const path = remembered && tree.filePaths.has(remembered) ? remembered : tree.firstPath;
  return (
    <div className="flex h-full min-h-0 flex-col bg-bg-panel">
      <CommitInfoHeader workspaceId={workspaceId} sha={sha} />
      <div className="min-h-0 flex-1">
        <WorkbenchSplit
          id="history-files"
          label={t("app.workbench.resizeFiles")}
          initial={24}
          min={16}
          max={48}
          collapsed={focused}
          first={
            files && (
              <HistoryFileTree
                key={JSON.stringify([workspaceId, repoId, sha])}
                tree={tree}
                selected={path}
                onSelect={(path) => update(scope, { historyFile: { commit: sha, path } })}
              />
            )
          }
          second={
            summary.isPending ? (
              <EmptyState title={t("diff.preview.loading")} className="h-full" />
            ) : summary.isError ? (
              <div className="p-4 text-xs text-danger" role="alert">
                {formatAppError(summary.error)}{" "}
                <button className="underline" onClick={() => void summary.refetch()}>
                  {t("common.retry")}
                </button>
              </div>
            ) : path ? (
              <DiffViewer compact commitOid={sha} path={path} hideMaximize />
            ) : (
              <EmptyState title={t("app.workbench.noFiles")} className="h-full" />
            )
          }
        />
      </div>
    </div>
  );
}
