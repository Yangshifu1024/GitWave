import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { ChangesPanel } from "@/components/ChangesPanel";
import { DiffViewer } from "@/components/DiffViewer";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { WorkbenchSplit } from "@/components/ui/WorkbenchSplit";
import { useWorkingCopy } from "@/hooks/useWorkingCopy";
import {
  EMPTY_SESSION,
  reconcileChangeSelection,
  useWorkbenchStore,
} from "@/stores/workbenchStore";
import { useLayoutStore } from "@/stores/layoutStore";

export function ChangesWorkspace({
  scope,
  active,
  onViewCommit,
  conflictCount,
  onResolveConflicts,
}: {
  scope: string;
  active: boolean;
  onViewCommit: (oid: string) => void;
  conflictCount: number;
  onResolveConflicts: () => void;
}): React.JSX.Element {
  const { t } = useTranslation();
  const wc = useWorkingCopy();
  const selected = useWorkbenchStore((s) => (s.sessions[scope] ?? EMPTY_SESSION).change);
  const update = useWorkbenchStore((s) => s.update);
  const focused = useLayoutStore((s) => s.inspectorMaximized);
  const files = wc.data?.files;
  const [lists, setLists] = useState<{
    scope: string;
    files: typeof files;
    previous?: typeof files;
  }>({
    scope,
    files,
  });
  if (lists.scope !== scope || lists.files !== files) {
    setLists({ scope, files, previous: lists.scope === scope ? lists.files : undefined });
  }
  const current = files ? reconcileChangeSelection(selected, files, lists.previous) : null;
  useEffect(() => {
    if (wc.data && (selected?.path !== current?.path || selected?.staged !== current?.staged))
      update(scope, { change: current ? { path: current.path, staged: current.staged } : null });
  }, [wc.data, selected, current, scope, update]);
  const kind = wc.data?.files.find(
    (file) => file.path === current?.path && file.staged === current?.staged,
  )?.kind;
  return (
    <div className="flex h-full min-h-0 flex-col bg-bg-panel">
      {conflictCount > 0 && (
        <div className="flex shrink-0 items-center justify-between gap-2 border-b border-warning/30 bg-warning/5 px-3 py-1 text-xs">
          <span>{t("app.workbench.conflicts", { count: conflictCount })}</span>
          <Button size="sm" variant="secondary" onClick={onResolveConflicts}>
            {t("app.workbench.resolve")}
          </Button>
        </div>
      )}
      <div className="min-h-0 flex-1">
        <WorkbenchSplit
          id="changes-files"
          label={t("app.workbench.resizeFiles")}
          initial={36}
          min={28}
          max={58}
          collapsed={focused}
          first={
            <ChangesPanel
              selectedPath={current?.path ?? null}
              selectedStaged={current?.staged ?? null}
              onSelectFile={(path, staged) => update(scope, { change: { path, staged } })}
              layout="modal"
              onViewCommit={onViewCommit}
            />
          }
          second={
            active && current ? (
              <DiffViewer
                compact
                workdir
                path={current.path}
                staged={current.staged}
                workdirKind={kind}
                hideMaximize
              />
            ) : (
              <EmptyState
                title={t("changes.panel.noFileSelected")}
                description={t("changes.panel.noFileSelectedDescription")}
                className="h-full"
              />
            )
          }
        />
      </div>
    </div>
  );
}
