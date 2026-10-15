import { type ReactNode, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  FileDiff,
  History,
  Maximize2,
  Minimize2,
  PanelLeftClose,
  PanelLeftOpen,
  Search,
} from "lucide-react";
import { Button } from "@/components/ui/Button";
import { BranchIndicator } from "@/components/ui/BranchIndicator";
import { EmptyState } from "@/components/ui/EmptyState";
import { WorkbenchSplit } from "@/components/ui/WorkbenchSplit";
import { CommitGraph } from "@/components/CommitGraph";
import { HistoryDetails } from "./HistoryDetails";
import { ChangesWorkspace } from "./ChangesWorkspace";
import { useWorkingCopy } from "@/hooks/useWorkingCopy";
import { useLayoutStore } from "@/stores/layoutStore";
import { useUiStore } from "@/stores/uiStore";
import { EMPTY_SESSION, useWorkbenchStore, type WorkbenchMode } from "@/stores/workbenchStore";
import type { LocateRequest } from "@/lib/commitLocate";
import { cn } from "@/lib/utils";

export function Workbench({
  scope,
  workspaceId,
  repoId,
  sidebar,
  onCommitSelect,
  locateRequest,
  conflictCount,
  onResolveConflicts,
}: {
  scope: string;
  workspaceId: string | null;
  repoId: string | null;
  sidebar: ReactNode;
  onCommitSelect: (sha: string) => void;
  locateRequest: LocateRequest | null;
  conflictCount: number;
  onResolveConflicts: () => void;
}): React.JSX.Element {
  const { t } = useTranslation();
  const session = useWorkbenchStore((s) => s.sessions[scope] ?? EMPTY_SESSION);
  const update = useWorkbenchStore((s) => s.update);
  const focused = useLayoutStore((s) => s.inspectorMaximized);
  const setFocused = useLayoutStore((s) => s.setInspectorMaximized);
  const [sidebarHidden, setSidebarHidden] = useState(false);
  const wc = useWorkingCopy();
  const modes = [
    { id: "history" as const, icon: History, label: t("app.workbench.history") },
    { id: "changes" as const, icon: FileDiff, label: t("app.workbench.changes") },
  ];
  const navigate = (mode: WorkbenchMode) => {
    update(scope, { mode });
    setFocused(false);
  };
  const navigation = (
    <nav
      aria-label={t("app.workbench.navigation")}
      className="flex shrink-0 flex-col gap-1 border-b border-border-subtle py-2"
    >
      {modes.map(({ id, icon: Icon, label }) => (
        <button
          key={id}
          type="button"
          disabled={!repoId}
          aria-current={session.mode === id ? "page" : undefined}
          onClick={() => navigate(id)}
          className={cn("workbench-nav-item", session.mode === id && "workbench-nav-selected")}
        >
          <Icon size={16} />
          <span className="flex-1 text-left">{label}</span>
          {id === "changes" && wc.data && (
            <span className="workbench-count">
              {new Set(wc.data.files.map((file) => file.path)).size}
            </span>
          )}
        </button>
      ))}
    </nav>
  );
  return (
    <WorkbenchSplit
      id="navigator"
      label={t("app.layout.resizeSidebar")}
      initial={19}
      min={15}
      max={32}
      collapsed={sidebarHidden}
      first={
        <div className="flex h-full min-h-0 flex-col bg-bg-primary">
          {navigation}
          <div className="min-h-0 flex-1 overflow-hidden">{sidebar}</div>
        </div>
      }
      second={
        <main className="flex h-full min-h-0 min-w-0 flex-col bg-bg-panel">
          <header className="workbench-toolbar">
            <Button
              size="sm"
              variant="ghost"
              aria-label={t("app.workbench.toggleSidebar")}
              aria-pressed={!sidebarHidden}
              onClick={() => setSidebarHidden(!sidebarHidden)}
            >
              {sidebarHidden ? <PanelLeftOpen size={16} /> : <PanelLeftClose size={16} />}
            </Button>
            {sidebarHidden ? (
              <div className="flex items-center gap-1">
                {modes.map(({ id, label }) => (
                  <Button
                    key={id}
                    size="sm"
                    variant={session.mode === id ? "secondary" : "ghost"}
                    disabled={!repoId}
                    aria-pressed={session.mode === id}
                    onClick={() => navigate(id)}
                  >
                    {label}
                  </Button>
                ))}
              </div>
            ) : (
              <h1 className="shrink-0 text-sm font-semibold">
                {t(`app.workbench.${session.mode}`)}
              </h1>
            )}
            {wc.data && (
              <div className="min-w-0 flex-1 overflow-hidden border-l border-border-subtle pl-3 text-xs">
                <BranchIndicator
                  branch={wc.data.branch}
                  sha={wc.data.branch === "(detached)" ? wc.data.sha : null}
                  upstream={wc.data.upstream}
                  ahead={wc.data.ahead}
                  behind={wc.data.behind}
                />
              </div>
            )}
            <div className="ml-auto flex shrink-0 gap-1">
              {session.mode === "history" && (
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={!repoId}
                  onClick={() => useUiStore.getState().setPaletteOpen(true)}
                >
                  <Search size={14} />
                  {t("app.workbench.search")}
                </Button>
              )}
              <Button
                size="sm"
                variant="ghost"
                disabled={!repoId}
                aria-pressed={focused}
                onClick={() => setFocused(!focused)}
              >
                {focused ? <Minimize2 size={14} /> : <Maximize2 size={14} />}
                {t(focused ? "app.workbench.restore" : "app.workbench.focus")}
              </Button>
            </div>
          </header>
          {!workspaceId || !repoId ? (
            <EmptyState
              title={t(
                workspaceId ? "app.emptyState.noRepoTitle" : "app.emptyState.selectWorkspaceTitle",
              )}
              description={t(
                workspaceId
                  ? "app.emptyState.noRepoDescription"
                  : "app.emptyState.selectWorkspaceDescription",
              )}
              className="flex-1"
            />
          ) : (
            <div key={scope} className="min-h-0 flex-1">
              <section
                hidden={session.mode !== "history"}
                className="h-full min-h-0"
                aria-label={t("app.workbench.history")}
              >
                <WorkbenchSplit
                  id="history-details"
                  label={t("app.workbench.resizeHistory")}
                  direction="vertical"
                  initial={42}
                  min={22}
                  max={72}
                  collapsed={focused}
                  first={
                    <CommitGraph
                      selectedSha={session.commit}
                      onCommitSelect={onCommitSelect}
                      locateRequest={locateRequest}
                    />
                  }
                  second={
                    <HistoryDetails
                      workspaceId={workspaceId}
                      repoId={repoId}
                      scope={scope}
                      sha={session.commit}
                    />
                  }
                />
              </section>
              <section
                hidden={session.mode !== "changes"}
                className="h-full min-h-0"
                aria-label={t("app.workbench.changes")}
              >
                <ChangesWorkspace
                  scope={scope}
                  active={session.mode === "changes"}
                  conflictCount={conflictCount}
                  onResolveConflicts={onResolveConflicts}
                />
              </section>
            </div>
          )}
        </main>
      }
    />
  );
}
