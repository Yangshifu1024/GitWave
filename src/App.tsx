import { useState } from "react";

import { Split, Pane, ResizeHandle } from "@/components/ui/Split";
import { Toolbar } from "@/components/Toolbar";
import { RepoList } from "@/components/RepoList";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { EmptyState } from "@/components/ui/EmptyState";
import { SidebarSection } from "@/components/ui/SidebarSection";
import { WorkingCopyBar } from "@/components/ui/WorkingCopyBar";
import { FolderOpen } from "lucide-react";
import { useTheme } from "@/hooks/useTheme";
import { CommitGraph } from "@/components/CommitGraph";
import { DiffViewer } from "@/components/DiffViewer";
import { BranchList } from "@/components/BranchList";
import { StashPanel } from "@/components/StashPanel";
import { WorktreePanel } from "@/components/WorktreePanel";
import { ConflictPanel } from "@/components/ConflictPanel";
import { useTitlebarActivation } from "@/hooks/useTitlebar";

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

  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const titlebarMode = useTitlebarActivation();
  useTheme();

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

  const handleWorkdirFileSelect = (path: string, staged: boolean): void => {
    if (!activeRepoId) return;
    setCommitSelection(null);
    setWorkdirSelection({ repoId: activeRepoId, path, staged });
  };

  return (
    <div
      className="flex flex-col h-screen w-screen overflow-hidden bg-bg-primary"
      data-titlebar-mode={titlebarMode === "pending" ? undefined : titlebarMode}
    >
      <Toolbar />

      <div className="flex-1 overflow-hidden">
        <Split direction="horizontal">
          <Pane initialSize={220} minSize={180} maxSize={360}>
            <aside className="flex flex-col h-full bg-bg-secondary overflow-x-hidden overflow-y-auto select-none">
              {activeWorkspaceId ? (
                <>
                  <RepoList workspaceId={activeWorkspaceId} />
                  <BranchList />
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
                  description="Choose or create a workspace in the toolbar."
                  className="flex-1"
                />
              )}
            </aside>
          </Pane>

          <ResizeHandle />

          <Pane initialSize="50%" minSize={280} grow>
            <section className="flex flex-col h-full bg-bg-primary overflow-hidden">
              <CommitGraph selectedSha={selectedCommitOid} onCommitSelect={handleCommitSelect} />
            </section>
          </Pane>

          <ResizeHandle />

          <Pane initialSize={360} minSize={240}>
            <MainContent
              selectedCommitOid={selectedCommitOid}
              selectedWorkdirPath={selectedWorkdirPath}
              selectedWorkdirStaged={selectedWorkdirStaged}
            />
          </Pane>
        </Split>
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
      <main className="flex flex-col h-full min-h-0 items-center justify-center bg-bg-elevated">
        <EmptyState
          icon={<FolderOpen size={28} />}
          title="Select a workspace"
          description="Create or choose a workspace in the toolbar to open repositories."
          className="py-16"
        />
      </main>
    );
  }

  if (!activeRepoId) {
    return (
      <main className="flex flex-col h-full min-h-0 items-center justify-center bg-bg-elevated">
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
    <main className="flex flex-col h-full min-h-0 bg-bg-elevated overflow-hidden">
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
