import { useEffect, useRef, useState } from "react";

import { ThreePaneLayout } from "@/components/ui/ThreePaneLayout";
import { Toolbar } from "@/components/Toolbar";
import { WorkspaceList } from "@/components/WorkspaceList";
import { RepoList } from "@/components/RepoList";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { useLayoutStore } from "@/stores/layoutStore";
import { EmptyState } from "@/components/ui/EmptyState";
import { SidebarSection } from "@/components/ui/SidebarSection";
import { WorkingCopyBar } from "@/components/ui/WorkingCopyBar";
import { FolderOpen } from "lucide-react";
import { useTheme } from "@/hooks/useTheme";
import type { BranchInfo } from "@/lib/api";
import type { LocateRequest } from "@/lib/commitLocate";
import { CommitGraph } from "@/components/CommitGraph";
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
  const [workdirSelection, setWorkdirSelection] = useState<{
    repoId: string;
    path: string;
    staged: boolean;
  } | null>(null);
  /** One-shot request for CommitGraph to center a commit; `seq` re-fires on repeated clicks. */
  const [locateRequest, setLocateRequest] = useState<LocateRequest | null>(null);
  const locateSeq = useRef(0);

  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const inspectorMaximized = useLayoutStore((s) => s.inspectorMaximized);
  const setInspectorMaximized = useLayoutStore((s) => s.setInspectorMaximized);
  const setWcBarCollapsed = useLayoutStore((s) => s.setWcBarCollapsed);
  const setWcBarMaximized = useLayoutStore((s) => s.setWcBarMaximized);
  const titlebarMode = useTitlebarActivation();
  useTheme();

  useEffect(() => {
    setInspectorMaximized(false);
    setWcBarCollapsed(false);
    setWcBarMaximized(false);
  }, [
    activeWorkspaceId,
    activeRepoId,
    setInspectorMaximized,
    setWcBarCollapsed,
    setWcBarMaximized,
  ]);

  const selectedCommitOid =
    commitSelection && commitSelection.repoId === activeRepoId ? commitSelection.sha : null;
  const selectedWorkdirPath =
    workdirSelection && workdirSelection.repoId === activeRepoId ? workdirSelection.path : null;
  const selectedWorkdirStaged =
    workdirSelection && workdirSelection.repoId === activeRepoId ? workdirSelection.staged : null;

  const handleCommitSelect = (sha: string): void => {
    if (!activeRepoId) return;
    setWorkdirSelection(null);
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

  const handleWorkdirFileSelect = (path: string, staged: boolean): void => {
    if (!activeRepoId) return;
    setCommitSelection(null);
    setWorkdirSelection({ repoId: activeRepoId, path, staged });
  };

  return (
    <div
      className="flex flex-col h-full w-full min-h-0 overflow-hidden bg-bg-primary"
      data-titlebar-mode={titlebarMode === "pending" ? undefined : titlebarMode}
    >
      <Toolbar />

      <div className="flex-1 min-h-0 min-w-0 overflow-hidden">
        <ThreePaneLayout
          inspectorMaximized={inspectorMaximized}
          inspectorClassName={cn(
            inspectorMaximized &&
              "relative z-20 shadow-[inset_1px_0_0_var(--color-border-subtle),-12px_0_32px_color-mix(in_srgb,var(--color-text-primary)_12%,transparent)]",
          )}
          sidebar={
            <aside className="flex flex-col h-full bg-bg-primary overflow-x-hidden overflow-y-auto no-scrollbar select-none pane-edge-right">
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
          inspector={
            <MainContent
              selectedCommitOid={selectedCommitOid}
              selectedWorkdirPath={selectedWorkdirPath}
              selectedWorkdirStaged={selectedWorkdirStaged}
            />
          }
        />
      </div>

      <WorkingCopyBar
        repoId={activeRepoId}
        selectedPath={selectedWorkdirPath}
        selectedStaged={selectedWorkdirStaged}
        onSelectFile={handleWorkdirFileSelect}
      />
      <ConflictPanel />
    </div>
  );
}

function MainContent({
  selectedCommitOid,
  selectedWorkdirPath,
  selectedWorkdirStaged,
}: {
  selectedCommitOid: string | null;
  selectedWorkdirPath: string | null;
  selectedWorkdirStaged: boolean | null;
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
        <DiffViewer key={activeRepoId} commitOid={selectedCommitOid} />
      ) : (
        <DiffViewer
          key={activeRepoId}
          workdir={true}
          path={selectedWorkdirPath ?? undefined}
          staged={selectedWorkdirStaged}
        />
      )}
    </main>
  );
}

export default App;
