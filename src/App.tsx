import { useEffect, useRef, useState } from "react";

import { ThreePaneLayout } from "@/components/ui/ThreePaneLayout";
import { Toolbar } from "@/components/Toolbar";
import { ActionBar } from "@/components/ActionBar";
import { WorkspaceList } from "@/components/WorkspaceList";
import { RepoList } from "@/components/RepoList";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { useLayoutStore } from "@/stores/layoutStore";
import { EmptyState } from "@/components/ui/EmptyState";
import { SidebarSection } from "@/components/ui/SidebarSection";
import { FolderOpen, GitCommitHorizontal } from "lucide-react";
import { useTheme } from "@/hooks/useTheme";
import type { BranchInfo } from "@/lib/api";
import type { LocateRequest } from "@/lib/commitLocate";
import { CommitGraph } from "@/components/CommitGraph";
import { CommitInfoHeader } from "@/components/CommitInfoHeader";
import { DiffViewer } from "@/components/DiffViewer";
import { BranchList } from "@/components/BranchList";
import { StashPanel } from "@/components/StashPanel";
import { WorktreePanel } from "@/components/WorktreePanel";
import { ConflictPanel } from "@/components/ConflictPanel";
import { useTitlebarActivation } from "@/hooks/useTitlebar";
import { cn } from "@/lib/utils";

function App(): React.JSX.Element {
  /** Commit selection scoped to the repo it was made in — avoids stale OID after switch. */
  const [commitSelection, setCommitSelection] = useState<{
    repoId: string;
    sha: string;
  } | null>(null);
  /** One-shot request for CommitGraph to center a commit; `seq` re-fires on repeated clicks. */
  const [locateRequest, setLocateRequest] = useState<LocateRequest | null>(null);
  const locateSeq = useRef(0);

  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const inspectorMaximized = useLayoutStore((s) => s.inspectorMaximized);
  const setInspectorMaximized = useLayoutStore((s) => s.setInspectorMaximized);
  const titlebarMode = useTitlebarActivation();
  useTheme();

  useEffect(() => {
    setInspectorMaximized(false);
  }, [activeWorkspaceId, activeRepoId, setInspectorMaximized]);

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

  return (
    <div
      className="flex flex-col h-full w-full min-h-0 overflow-hidden bg-bg-primary"
      data-titlebar-mode={titlebarMode === "pending" ? undefined : titlebarMode}
    >
      <Toolbar />

      <ActionBar />

      <div className="flex-1 min-h-0 min-w-0 overflow-hidden">
        <ThreePaneLayout
          inspectorMaximized={inspectorMaximized}
          inspectorClassName={cn(
            inspectorMaximized &&
              "relative z-20 shadow-[inset_1px_0_0_var(--color-border-subtle),-12px_0_32px_color-mix(in_srgb,var(--color-text-primary)_12%,transparent)]",
          )}
          sidebar={
            <aside className="flex flex-col h-full bg-bg-panel overflow-x-hidden overflow-y-auto no-scrollbar select-none pane-edge-right">
              <WorkspaceList />
              {activeWorkspaceId ? (
                <>
                  <RepoList workspaceId={activeWorkspaceId} />
                  <BranchList onBranchSelect={handleBranchSelect} />
                  <SidebarSection title="Stash" defaultOpen={false}>
                    <StashPanel compact />
                  </SidebarSection>
                  <SidebarSection title="Tags" defaultOpen={false}>
                    <p className="px-3 py-1.5 text-xs text-text-muted">No tags yet</p>
                  </SidebarSection>
                  <SidebarSection title="Remotes" defaultOpen={false}>
                    <p className="px-3 py-1.5 text-xs text-text-muted">Remote list coming later</p>
                  </SidebarSection>
                  <SidebarSection title="Worktrees" defaultOpen={false}>
                    <WorktreePanel compact />
                  </SidebarSection>
                </>
              ) : (
                <EmptyState
                  icon={<FolderOpen size={22} />}
                  title="No workspace selected"
                  description="Choose or create a workspace in the sidebar."
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

      <ConflictPanel />
    </div>
  );
}

function MainContent({
  selectedCommitOid,
}: {
  selectedCommitOid: string | null;
}): React.JSX.Element {
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);

  if (!activeWorkspaceId) {
    return (
      <main className="flex flex-col h-full min-h-0 items-center justify-center bg-bg-panel pane-edge-left">
        <EmptyState
          icon={<FolderOpen size={28} />}
          title="Select a workspace"
          description="Choose or create a workspace in the sidebar to open repositories."
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
          title="No repository selected"
          description="Select a repository from the sidebar to view history and diffs."
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
          title="No commit selected"
          description="Click a commit in the history graph to see its diff."
          className="h-full"
        />
      )}
    </main>
  );
}

export default App;
