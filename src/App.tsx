import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";

import { ThreePaneLayout } from "@/components/ui/ThreePaneLayout";
import { AuthPromptDialog } from "@/components/AuthPromptDialog";
import { Toolbar } from "@/components/Toolbar";
import { ActionBar } from "@/components/ActionBar";
import { WorkspaceRepoTabs } from "@/components/WorkspaceRepoTabs";
import { useWorkspaceUiStore, readLastActive } from "@/stores/workspaceStore";
import { useLayoutStore } from "@/stores/layoutStore";
import { EmptyState } from "@/components/ui/EmptyState";
import { SidebarSection } from "@/components/ui/SidebarSection";
import { FolderOpen, GitCommitHorizontal } from "lucide-react";
import { useTheme } from "@/hooks/useTheme";
import { useAutoRefreshLoop } from "@/hooks/useAutoRefresh";
import { useStartupUpdateCheck } from "@/hooks/useUpdater";
import { UpdateModal } from "@/components/UpdateModal";
import type { BranchInfo, RepoRef } from "@/lib/api";
import { listRepos, listWorkspaces } from "@/lib/api";
import { pickRestoredRepo } from "@/lib/repoSelection";
import type { LocateRequest } from "@/lib/commitLocate";
import { CommitGraph } from "@/components/CommitGraph";
import { CommitInfoHeader } from "@/components/CommitInfoHeader";
import { DiffViewer } from "@/components/DiffViewer";
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
import { cn } from "@/lib/utils";

function App(): React.JSX.Element {
  const { t } = useTranslation();
  /** Commit selection scoped to the repo it was made in — avoids stale OID after switch. */
  const [commitSelection, setCommitSelection] = useState<{
    repoId: string;
    sha: string;
  } | null>(null);
  /** One-shot request for CommitGraph to center a commit; `seq` re-fires on repeated clicks. */
  const [locateRequest, setLocateRequest] = useState<LocateRequest | null>(null);
  const locateSeq = useRef(0);
  /** Conflict panel is opened on demand (Merge banner's Resolve button). */
  const [conflictPanelOpen, setConflictPanelOpen] = useState(false);
  const mergeConflicts = useMergeConflicts();

  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const inspectorMaximized = useLayoutStore((s) => s.inspectorMaximized);
  const setInspectorMaximized = useLayoutStore((s) => s.setInspectorMaximized);
  const titlebarMode = useTitlebarActivation();
  useTheme();
  useAutoRefreshLoop();
  useStartupUpdateCheck();

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

  const selectedCommitOid =
    commitSelection && commitSelection.repoId === activeRepoId ? commitSelection.sha : null;

  const handleCommitSelect = (sha: string): void => {
    if (!activeRepoId) return;
    setCommitSelection({ repoId: activeRepoId, sha });
  };

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
      <Toolbar />

      <MergeBanner merge={mergeConflicts} onResolve={() => setConflictPanelOpen(true)} />

      <ActionBar />

      <AuthPromptDialog />

      <WorkspaceRepoTabs />

      <div className="flex-1 min-h-0 min-w-0 overflow-hidden">
        <ThreePaneLayout
          inspectorMaximized={inspectorMaximized}
          inspectorClassName={cn(
            inspectorMaximized &&
              "relative z-20 shadow-[inset_1px_0_0_var(--color-border-subtle),-12px_0_32px_color-mix(in_srgb,var(--color-text-primary)_12%,transparent)]",
          )}
          sidebar={
            <aside className="flex flex-col h-full gap-1.5 px-2 py-2 bg-bg-panel overflow-hidden select-none pane-edge-right">
              {activeWorkspaceId ? (
                <>
                  <SidebarSection title={t("app.sidebar.health")} defaultOpen={false}>
                    <HealthPanel />
                  </SidebarSection>
                  <BranchList onBranchSelect={handleBranchSelect} />
                  <StashPanel compact />
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
          main={
            <section className="flex flex-col h-full bg-bg-panel overflow-hidden">
              <CommitGraph
                selectedSha={selectedCommitOid}
                onCommitSelect={handleCommitSelect}
                locateRequest={locateRequest}
              />
            </section>
          }
          inspector={<MainContent selectedCommitOid={selectedCommitOid} />}
        />
      </div>

      <ConflictPanel
        open={conflictPanelOpen}
        onClose={() => setConflictPanelOpen(false)}
        merge={mergeConflicts}
      />

      <CommandPalette requestLocate={handlePaletteLocate} />

      <UpdateModal />
    </div>
  );
}

function MainContent({
  selectedCommitOid,
}: {
  selectedCommitOid: string | null;
}): React.JSX.Element {
  const { t } = useTranslation();
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);

  if (!activeWorkspaceId) {
    return (
      <main className="flex flex-col h-full min-h-0 items-center justify-center bg-bg-panel pane-edge-left">
        <EmptyState
          icon={<FolderOpen size={28} />}
          title={t("app.emptyState.selectWorkspaceTitle")}
          description={t("app.emptyState.selectWorkspaceDescription")}
          className="py-16"
        />
      </main>
    );
  }

  if (!activeRepoId) {
    return (
      <main className="flex flex-col h-full min-h-0 items-center justify-center bg-bg-panel pane-edge-left">
        <EmptyState
          icon={<FolderOpen size={28} />}
          title={t("app.emptyState.noRepoTitle")}
          description={t("app.emptyState.noRepoDescription")}
          className="py-16"
        />
      </main>
    );
  }

  return (
    <main className="flex flex-col h-full min-h-0 bg-bg-panel pane-edge-left overflow-hidden">
      {selectedCommitOid ? (
        <div key={activeRepoId} className="flex h-full min-h-0 flex-col">
          <CommitInfoHeader workspaceId={activeWorkspaceId} sha={selectedCommitOid} />
          <div className="min-h-0 flex-1">
            <DiffViewer commitOid={selectedCommitOid} />
          </div>
        </div>
      ) : (
        <EmptyState
          icon={<GitCommitHorizontal size={28} />}
          title={t("app.emptyState.noCommitTitle")}
          description={t("app.emptyState.noCommitDescription")}
          className="h-full"
        />
      )}
    </main>
  );
}

export default App;
