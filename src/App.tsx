import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";

import { Workbench } from "@/components/workbench/Workbench";
import { EMPTY_SESSION, useWorkbenchStore, workbenchKey } from "@/stores/workbenchStore";
import { AuthPromptDialog } from "@/components/AuthPromptDialog";
import { ActionBar } from "@/components/ActionBar";
import { WorkspaceRepoTabs } from "@/components/WorkspaceRepoTabs";
import { SyncStatusArea } from "@/components/SyncStatusArea";
import { useWorkspaceUiStore, readLastActive } from "@/stores/workspaceStore";
import { useLayoutStore } from "@/stores/layoutStore";
import { EmptyState } from "@/components/ui/EmptyState";
import { SidebarSection } from "@/components/ui/SidebarSection";
import { FolderOpen } from "lucide-react";
import { useTheme } from "@/hooks/useTheme";
import { useAutoRefreshLoop } from "@/hooks/useAutoRefresh";
import { useStartupUpdateCheck } from "@/hooks/useUpdater";
import { UpdateModal } from "@/components/UpdateModal";
import type { BranchInfo, RepoRef } from "@/lib/api";
import { listRepos, listWorkspaces } from "@/lib/api";
import { pickRestoredRepo } from "@/lib/repoSelection";
import type { LocateRequest } from "@/lib/commitLocate";
import { BranchList } from "@/components/BranchList";
import { StashPanel } from "@/components/StashPanel";
import { WorktreePanel } from "@/components/WorktreePanel";
import { SubmodulesPanel } from "@/components/SubmodulesPanel";
import { TagsPanel } from "@/components/TagsPanel";
import { RemotesPanel } from "@/components/RemotesPanel";
import { ReflogPanel } from "@/components/ReflogPanel";
import { HealthPanel } from "@/components/HealthPanel";
import { ConflictPanel } from "@/components/ConflictPanel";
import { MergeBanner } from "@/components/MergeBanner";
import { useMergeConflicts } from "@/hooks/useMergeConflicts";
import { CommandPalette } from "@/components/CommandPalette";
import { useTitlebarActivation } from "@/hooks/useTitlebar";
import { useQuitGuard, cancelQuit, confirmQuit } from "@/hooks/useQuitGuard";
import { QuitConfirmModal } from "@/components/QuitConfirmModal";

function App(): React.JSX.Element {
  const { t } = useTranslation();
  /** One-shot request for CommitGraph to center a commit; `seq` re-fires on repeated clicks. */
  const [locateRequest, setLocateRequest] = useState<LocateRequest | null>(null);
  const locateSeq = useRef(0);
  /** Conflict panel is opened on demand (Merge banner's Resolve button). */
  const [conflictPanelOpen, setConflictPanelOpen] = useState(false);
  const mergeConflicts = useMergeConflicts();

  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const scope = workbenchKey(activeWorkspaceId, activeRepoId);
  const session = useWorkbenchStore((s) => s.sessions[scope] ?? EMPTY_SESSION);
  const updateSession = useWorkbenchStore((s) => s.update);
  const setInspectorMaximized = useLayoutStore((s) => s.setInspectorMaximized);
  const titlebarMode = useTitlebarActivation();
  useTheme();
  useAutoRefreshLoop();
  useStartupUpdateCheck();
  const quitGuard = useQuitGuard();

  useEffect(() => {
    setInspectorMaximized(false);
  }, [activeWorkspaceId, activeRepoId, setInspectorMaximized]);

  // PM 1.4 restart restore: land on the last active workspace/repo once the
  // persisted workspace list is in. Skips stale ids (deleted workspace/repo)
  // and repos whose path no longer opens — restoring onto a missing repo
  // would fail every panel query and raise blocking error dialogs on the
  // very first render.
  const selectWorkspace = useWorkspaceUiStore((s) => s.selectWorkspace);
  const queryClient = useQueryClient();
  const restoredRef = useRef(false);
  useEffect(() => {
    if (restoredRef.current) return;
    restoredRef.current = true;
    const { workspaceId, repoId } = readLastActive();
    if (!workspaceId) return;
    listWorkspaces()
      .then(async (workspaces) => {
        const ws = workspaces.find((w) => w.id === workspaceId);
        if (!ws) return;
        const target = repoId ?? ws.last_active_repo_id;
        // Warm the ["repos", …] cache the tab strip reads anyway, so the
        // validity check below costs no extra round trip. Fail fast: default
        // retry(3) would stall restore for seconds on a backend hiccup, and
        // the tab strip's own query still surfaces load errors.
        const repos = await queryClient
          .fetchQuery({
            queryKey: ["repos", ws.id],
            queryFn: () => listRepos(ws.id),
            retry: 0,
          })
          .catch(() => [] as RepoRef[]);
        selectWorkspace(ws.id, pickRestoredRepo(repos, target));
      })
      .catch(() => {
        // First run with no persisted workspaces — nothing to restore.
      });
  }, [selectWorkspace, queryClient]);

  const selectedCommitOid = session.commit;
  const handleCommitSelect = useCallback(
    (sha: string): void => {
      if (!activeRepoId) return;
      updateSession(scope, { commit: sha, mode: "history" });
    },
    [activeRepoId, scope, updateSession],
  );

  const handleBranchSelect = (branch: BranchInfo): void => {
    if (!activeRepoId || !branch.last_commit_sha) return;
    handleCommitSelect(branch.last_commit_sha);
    locateSeq.current += 1;
    setLocateRequest({
      repoId: activeRepoId,
      sha: branch.last_commit_sha,
      seq: locateSeq.current,
    });
  };

  const handleTagSelect = (sha: string): void => {
    if (!activeRepoId) return;
    handleCommitSelect(sha);
    locateSeq.current += 1;
    setLocateRequest({ repoId: activeRepoId, sha, seq: locateSeq.current });
  };

  /** Command palette "locate_commit": same one-shot locate flow as the sidebar. */
  const handlePaletteLocate = (sha: string): void => {
    if (!activeRepoId || !sha) return;
    handleCommitSelect(sha);
    locateSeq.current += 1;
    setLocateRequest({ repoId: activeRepoId, sha, seq: locateSeq.current });
  };

  return (
    <div
      className="flex flex-col h-full w-full min-h-0 overflow-hidden bg-bg-primary"
      data-titlebar-mode={titlebarMode === "pending" ? undefined : titlebarMode}
    >
      <ActionBar
        onOpenChanges={() => {
          updateSession(scope, { mode: "changes" });
          setInspectorMaximized(false);
        }}
        conflictCount={mergeConflicts.files.length}
      />

      <MergeBanner merge={mergeConflicts} onResolve={() => setConflictPanelOpen(true)} />

      <AuthPromptDialog />

      <WorkspaceRepoTabs />

      <div className="flex-1 min-h-0 min-w-0 overflow-hidden">
        <Workbench
          scope={scope}
          workspaceId={activeWorkspaceId}
          repoId={activeRepoId}
          onCommitSelect={handleCommitSelect}
          locateRequest={locateRequest}
          conflictCount={mergeConflicts.files.length}
          onResolveConflicts={() => setConflictPanelOpen(true)}
          sidebar={
            <aside className="flex flex-col h-full gap-2 py-2 bg-bg-primary overflow-y-auto select-none">
              {activeWorkspaceId ? (
                <>
                  <HealthPanel key={`${activeWorkspaceId}:${activeRepoId}`} />
                  <BranchList onBranchSelect={handleBranchSelect} />
                  <StashPanel />
                  <TagsPanel onSelect={handleTagSelect} selectedSha={selectedCommitOid} />
                  <RemotesPanel />
                  <WorktreePanel compact />
                  <SubmodulesPanel />
                  <SidebarSection title={t("app.sidebar.recovery")} defaultOpen={false}>
                    <ReflogPanel />
                  </SidebarSection>
                </>
              ) : (
                <EmptyState
                  icon={<FolderOpen size={22} />}
                  title={t("app.emptyState.noWorkspaceTitle")}
                  description={t("app.emptyState.noWorkspaceDescription")}
                  className="flex-1"
                />
              )}
            </aside>
          }
        />
      </div>

      <footer className="flex h-8 shrink-0 min-w-0 items-center border-t border-border-subtle bg-bg-primary px-2">
        <SyncStatusArea />
      </footer>

      <ConflictPanel
        open={conflictPanelOpen}
        onClose={() => setConflictPanelOpen(false)}
        merge={mergeConflicts}
      />

      <CommandPalette requestLocate={handlePaletteLocate} />

      <QuitConfirmModal
        open={quitGuard.open}
        dirtyRepos={quitGuard.dirtyRepos}
        onCancel={cancelQuit}
        onConfirm={confirmQuit}
      />

      <UpdateModal />
    </div>
  );
}

export default App;
