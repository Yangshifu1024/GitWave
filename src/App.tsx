import { useEffect, useState } from "react";

import { Split, Pane, ResizeHandle } from "@/components/ui/Split";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/Tabs";
import { ThemeToggle } from "@/components/ThemeToggle";
import { RepoList } from "@/components/RepoList";
import { SshKeyManager } from "@/components/SshKeyManager";
import { WindowControls } from "@/components/WindowControls";
import { WorkspaceSwitcherDropdown } from "@/components/WorkspaceSwitcherDropdown";
import { WorkingCopyBar } from "@/components/ui/WorkingCopyBar";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { getAppVersion } from "@/lib/api";
import { isMacOS } from "@/lib/platform";
import { useTheme } from "@/hooks/useTheme";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { FolderOpen, HelpCircle } from "lucide-react";
import { CommitGraph } from "@/components/CommitGraph";
import { DiffViewer } from "@/components/DiffViewer";
import { BranchList } from "@/components/BranchList";
import { StashPanel } from "@/components/StashPanel";
import { WorktreePanel } from "@/components/WorktreePanel";
import { ConflictPanel } from "@/components/ConflictPanel";
import { ChangesPanel } from "@/components/ChangesPanel";

function App(): React.JSX.Element {
  const [version, setVersion] = useState<string>("…");
  /** Commit selection scoped to the repo it was made in — avoids stale OID after switch. */
  const [commitSelection, setCommitSelection] = useState<{
    repoId: string;
    sha: string;
  } | null>(null);
  const [workdirSelection, setWorkdirSelection] = useState<{
    repoId: string;
    path: string;
  } | null>(null);

  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);

  const selectedCommitOid =
    commitSelection && commitSelection.repoId === activeRepoId ? commitSelection.sha : null;
  const selectedWorkdirPath =
    workdirSelection && workdirSelection.repoId === activeRepoId ? workdirSelection.path : null;

  const handleCommitSelect = (sha: string): void => {
    if (!activeRepoId) return;
    setWorkdirSelection(null);
    setCommitSelection({ repoId: activeRepoId, sha });
  };

  const handleWorkdirFileSelect = (path: string): void => {
    if (!activeRepoId) return;
    setCommitSelection(null);
    setWorkdirSelection({ repoId: activeRepoId, path });
  };

  // Initialize theme immediately so the <html> class is correct on first render
  useTheme();

  useEffect(() => {
    getAppVersion()
      .then(setVersion)
      .catch(() => {
        setVersion("?.?.?");
      });
  }, []);

  const showWorkingCopy = activeWorkspaceId !== null && activeRepoId !== null;

  return (
    <div className="flex flex-col h-screen w-screen overflow-hidden bg-bg-primary">
      {/* ── Topbar ──────────────────────────────────────────────────────── */}
      <header
        data-tauri-drag-region
        className={`relative z-20 flex items-center shrink-0 h-12 ${
          isMacOS() ? "pl-20 pr-4" : "px-4"
        } gap-4 bg-bg-secondary border-b border-border-subtle`}
      >
        {/* Center: app title (absolutely centered regardless of sibling widths) */}
        <h1 className="absolute left-1/2 -translate-x-1/2 text-sm font-semibold text-text-primary pointer-events-none">
          GitWave
        </h1>

        {/* Right: theme, SSH, help, version (+ window controls on non-macOS).
            drag.js auto-blocks <button> children, so no explicit
            no-drag-region needed on the wrapper. */}
        <div className="ml-auto flex items-center gap-1">
          <ThemeToggle />
          <SshKeyManagerPopover />
          <Button variant="ghost" size="sm" className="p-1" aria-label="Help">
            <HelpCircle size={16} />
          </Button>
          <span className="text-xs text-text-muted pl-2">v{version}</span>
          {!isMacOS() ? (
            <>
              <span className="mx-2 h-4 w-px bg-border-subtle" aria-hidden />
              <WindowControls />
            </>
          ) : null}
        </div>
      </header>

      {/* ── 3-pane body ────────────────────────────────────────────────── */}
      <div className="flex-1 overflow-hidden">
        <Split direction="horizontal">
          {/* Sidebar: 20% */}
          <Pane initialSize="20%" minSize={180} maxSize={480}>
            <aside className="flex flex-col h-full bg-bg-secondary border-r border-border-subtle overflow-hidden">
              <div className="shrink-0 border-b border-border-subtle">
                <WorkspaceSwitcherDropdown />
              </div>
              <div className="flex-1 min-h-0 overflow-auto">
                {activeWorkspaceId ? (
                  <RepoList workspaceId={activeWorkspaceId} />
                ) : (
                  <EmptyState
                    icon={<FolderOpen size={24} />}
                    title="No workspace selected"
                    description="Select or create a workspace above to see repos."
                    className="flex-1"
                  />
                )}
              </div>
            </aside>
          </Pane>

          <ResizeHandle />

          {/* Feature Nav: 50% */}
          <Pane initialSize="50%" minSize={200} maxSize={900}>
            <FeatureNav
              selectedCommitOid={selectedCommitOid}
              onCommitSelect={handleCommitSelect}
              selectedWorkdirPath={selectedWorkdirPath}
              onWorkdirFileSelect={handleWorkdirFileSelect}
            />
          </Pane>

          <ResizeHandle />

          {/* Main: 30% */}
          <Pane initialSize="30%" minSize={240}>
            <MainContent
              selectedCommitOid={selectedCommitOid}
              selectedWorkdirPath={selectedWorkdirPath}
            />
          </Pane>
        </Split>
      </div>

      {/* ── Working Copy Bar ─────────────────────────────────────────────── */}
      {showWorkingCopy && <WorkingCopyBar repoId={activeRepoId} />}
      <ConflictPanel />
    </div>
  );
}

// ─── Sub-components ──────────────────────────────────────────────────────────

/** SshKeyManager wrapped in a ghost button that opens its own modal */
function SshKeyManagerPopover(): React.JSX.Element {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button
        variant="ghost"
        size="sm"
        className="p-1"
        onClick={() => setOpen(true)}
        aria-label="SSH Keys"
      >
        <span className="font-mono text-xs">SSH</span>
      </Button>
      {open && <SshKeyManagerModalWrapper onClose={() => setOpen(false)} />}
    </>
  );
}

function SshKeyManagerModalWrapper({ onClose }: { onClose: () => void }): React.JSX.Element {
  return (
    <div className="fixed inset-0 z-modal flex items-center justify-center">
      <div className="fixed inset-0 bg-bg-overlay backdrop-blur-sm" onClick={onClose} />
      <div
        className="relative z-10 w-full max-w-md rounded-xl bg-bg-elevated shadow-modal p-6"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-md font-semibold text-text-primary">SSH Keys</h2>
          <Button variant="ghost" size="sm" onClick={onClose} className="p-1">
            <span aria-hidden="true">&#x2715;</span>
          </Button>
        </div>
        <SshKeyManager />
      </div>
    </div>
  );
}

/** Feature Nav: tab bar + content area (Sprint 3+ full content) */
function FeatureNav({
  selectedCommitOid,
  onCommitSelect,
  selectedWorkdirPath,
  onWorkdirFileSelect,
}: {
  selectedCommitOid: string | null;
  onCommitSelect: (sha: string) => void;
  selectedWorkdirPath: string | null;
  onWorkdirFileSelect: (path: string) => void;
}): React.JSX.Element {
  const [activeTab, setActiveTab] = useState("changes");

  return (
    <div className="flex flex-col h-full bg-bg-secondary border-r border-border-subtle overflow-hidden">
      <Tabs value={activeTab} onValueChange={setActiveTab} className="flex flex-col h-full min-h-0">
        <TabsList className="shrink-0 px-2">
          <TabsTrigger value="changes">Changes</TabsTrigger>
          <TabsTrigger value="history">History</TabsTrigger>
          <TabsTrigger value="branches">Branches</TabsTrigger>
          <TabsTrigger value="stash">Stash</TabsTrigger>
          <TabsTrigger value="tags">Tags</TabsTrigger>
          <TabsTrigger value="remotes">Remotes</TabsTrigger>
          <TabsTrigger value="worktrees">Worktrees</TabsTrigger>
        </TabsList>

        <TabsContent value="changes" className="flex-1 min-h-0 overflow-hidden p-0">
          <ChangesPanel selectedPath={selectedWorkdirPath} onSelectFile={onWorkdirFileSelect} />
        </TabsContent>

        <TabsContent value="history" className="flex-1 min-h-0 overflow-hidden p-0">
          <CommitGraph selectedSha={selectedCommitOid} onCommitSelect={onCommitSelect} />
        </TabsContent>

        <TabsContent value="branches" className="flex-1 min-h-0 overflow-hidden p-0">
          <BranchList />
        </TabsContent>

        <TabsContent value="stash" className="flex-1 min-h-0 overflow-hidden p-0">
          <StashPanel />
        </TabsContent>

        <TabsContent value="tags" className="flex-1 min-h-0 overflow-auto p-4">
          <EmptyState
            icon={
              <span className="text-2xl" aria-hidden="true">
                &#x1F3F7;
              </span>
            }
            title="Tags"
            description="Tag list coming in Sprint 5."
            className="py-8"
          />
        </TabsContent>

        <TabsContent value="remotes" className="flex-1 min-h-0 overflow-auto p-4">
          <EmptyState
            icon={
              <span className="text-2xl" aria-hidden="true">
                &#x2601;
              </span>
            }
            title="Remotes"
            description="Remote management coming in Sprint 3."
            className="py-8"
          />
        </TabsContent>

        <TabsContent value="worktrees" className="flex-1 min-h-0 overflow-hidden p-0">
          <WorktreePanel />
        </TabsContent>
      </Tabs>
    </div>
  );
}

/** Main content area — shows diff for selected commit or working copy */
function MainContent({
  selectedCommitOid,
  selectedWorkdirPath,
}: {
  selectedCommitOid: string | null;
  selectedWorkdirPath: string | null;
}): React.JSX.Element {
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);

  if (!activeWorkspaceId) {
    return (
      <main className="flex flex-col h-full min-h-0 items-center justify-center bg-bg-primary">
        <EmptyState
          icon={
            <span className="text-4xl" aria-hidden="true">
              &#x25C8;
            </span>
          }
          title="Welcome to GitWave"
          description="Select a workspace from the sidebar to get started."
          className="py-16"
        />
      </main>
    );
  }

  if (!activeRepoId) {
    return (
      <main className="flex flex-col h-full min-h-0 items-center justify-center bg-bg-primary">
        <EmptyState
          icon={<FolderOpen size={32} />}
          title="No repository selected"
          description="Select a repository from the sidebar to view history and diffs."
          className="py-16"
        />
      </main>
    );
  }

  return (
    <main className="flex flex-col h-full min-h-0 bg-bg-primary overflow-hidden">
      {selectedCommitOid ? (
        <DiffViewer key={activeRepoId} commitOid={selectedCommitOid} />
      ) : (
        <DiffViewer
          key={activeRepoId}
          workdir={true}
          path={selectedWorkdirPath ?? undefined}
        />
      )}
    </main>
  );
}

export default App;
