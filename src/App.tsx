import { useEffect, useState } from "react";

import { Split, Pane, ResizeHandle } from "@/components/ui/Split";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/Tabs";
import { ThemeToggle } from "@/components/ThemeToggle";
import { RepoList } from "@/components/RepoList";
import { SshKeyManager } from "@/components/SshKeyManager";
import { WorkingCopyBar } from "@/components/ui/WorkingCopyBar";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { getAppVersion } from "@/lib/api";
import { useTheme } from "@/hooks/useTheme";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { FolderOpen, HelpCircle } from "lucide-react";

function App(): React.JSX.Element {
  const [version, setVersion] = useState<string>("…");

  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);

  // Initialize theme immediately so the <html> class is correct on first render
  useTheme();

  useEffect(() => {
    getAppVersion()
      .then(setVersion)
      .catch(() => {
        setVersion("?.?.?");
      });
  }, []);

  const showWorkingCopy = activeWorkspaceId !== null;

  return (
    <div className="flex flex-col h-screen w-screen overflow-hidden bg-bg-primary">
      {/* ── Topbar ──────────────────────────────────────────────────────── */}
      <header className="flex items-center shrink-0 h-12 px-4 gap-4 bg-bg-secondary border-b border-border-subtle">
        {/* Left: Workspace switcher */}
        <div className="w-48 shrink-0">
          <WorkspaceSwitcherInTopbar />
        </div>

        {/* Center: nothing yet (branch indicator will go here Sprint 4) */}
        <div className="flex-1" />

        {/* Right: theme, SSH, help, version */}
        <div className="flex items-center gap-1">
          <ThemeToggle />
          <SshKeyManagerPopover />
          <Button variant="ghost" size="sm" className="p-1" aria-label="Help">
            <HelpCircle size={16} />
          </Button>
          <span className="text-xs text-text-muted pl-2">v{version}</span>
        </div>
      </header>

      {/* ── 3-pane body ────────────────────────────────────────────────── */}
      <div className="flex-1 overflow-hidden">
        <Split direction="horizontal">
          {/* Sidebar: repo list */}
          <Pane initialSize={240} minSize={180} maxSize={360}>
            <aside className="flex flex-col h-full bg-bg-secondary border-r border-border-subtle overflow-auto">
              {activeWorkspaceId ? (
                <RepoList workspaceId={activeWorkspaceId} />
              ) : (
                <EmptyState
                  icon={<FolderOpen size={24} />}
                  title="No workspace selected"
                  description="Select or create a workspace to see repos."
                  className="flex-1"
                />
              )}
            </aside>
          </Pane>

          <ResizeHandle />

          {/* Feature Nav: tabs */}
          <Pane initialSize={280} minSize={200} maxSize={400}>
            <FeatureNav />
          </Pane>

          <ResizeHandle />

          {/* Main content */}
          <Pane initialSize={400} minSize={300}>
            <MainContent />
          </Pane>
        </Split>
      </div>

      {/* ── Working Copy Bar ─────────────────────────────────────────────── */}
      {showWorkingCopy && <WorkingCopyBar repoId={null} initialHeight={120} />}
    </div>
  );
}

// ─── Sub-components ──────────────────────────────────────────────────────────

/** Compact workspace switcher for the topbar — no header, just the active ws name */
function WorkspaceSwitcherInTopbar(): React.JSX.Element {
  const activeId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const setActiveId = useWorkspaceUiStore((s) => s.setActiveWorkspaceId);
  // We'll just show the active workspace name with a dropdown indicator
  // The full switcher is rendered in the sidebar for now (Sprint 1-2 pattern)
  // For topbar we keep it minimal
  return (
    <button
      type="button"
      onClick={() => {
        // In Sprint 2+ this would open a dropdown; for now just indicate selection
        void setActiveId;
      }}
      className="flex items-center gap-1 text-sm font-medium text-text-primary hover:text-accent transition-colors"
    >
      <span className="truncate max-w-[160px]">
        {activeId ? "Workspace" : "Select workspace"}
      </span>
    </button>
  );
}

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
      {open && (
        <SshKeyManagerModalWrapper onClose={() => setOpen(false)} />
      )}
    </>
  );
}

function SshKeyManagerModalWrapper({
  onClose,
}: {
  onClose: () => void;
}): React.JSX.Element {
  return (
    <div className="fixed inset-0 z-modal flex items-center justify-center">
      <div
        className="fixed inset-0 bg-bg-overlay backdrop-blur-sm"
        onClick={onClose}
      />
      <div
        className="relative z-10 w-full max-w-md rounded-xl bg-bg-elevated shadow-modal p-6"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-md font-semibold text-text-primary">SSH Keys</h2>
          <Button variant="ghost" size="sm" onClick={onClose} className="p-1">
            ✕
          </Button>
        </div>
        <SshKeyManager />
      </div>
    </div>
  );
}

/** Feature Nav: tab bar + content area (Sprint 3+ full content) */
function FeatureNav(): React.JSX.Element {
  const [activeTab, setActiveTab] = useState("history");

  return (
    <div className="flex flex-col h-full bg-bg-secondary border-r border-border-subtle overflow-hidden">
      <Tabs value={activeTab} onValueChange={setActiveTab} className="flex flex-col h-full">
        <TabsList className="shrink-0 px-2">
          <TabsTrigger value="history">History</TabsTrigger>
          <TabsTrigger value="branches">Branches</TabsTrigger>
          <TabsTrigger value="stash">Stash</TabsTrigger>
          <TabsTrigger value="tags">Tags</TabsTrigger>
          <TabsTrigger value="remotes">Remotes</TabsTrigger>
          <TabsTrigger value="worktrees">Worktrees</TabsTrigger>
        </TabsList>

        <TabsContent value="history" className="flex-1 overflow-auto p-4">
          <EmptyState
            icon={<span className="text-2xl">⏱</span>}
            title="History"
            description="Commit graph and details coming in Sprint 3."
            className="py-8"
          />
        </TabsContent>

        <TabsContent value="branches" className="flex-1 overflow-auto p-4">
          <EmptyState
            icon={<span className="text-2xl">⑂</span>}
            title="Branches"
            description="Branch list and tree view coming in Sprint 3."
            className="py-8"
          />
        </TabsContent>

        <TabsContent value="stash" className="flex-1 overflow-auto p-4">
          <EmptyState
            icon={<span className="text-2xl">◐</span>}
            title="Stash"
            description="Stash list coming in Sprint 5."
            className="py-8"
          />
        </TabsContent>

        <TabsContent value="tags" className="flex-1 overflow-auto p-4">
          <EmptyState
            icon={<span className="text-2xl">🏷</span>}
            title="Tags"
            description="Tag list coming in Sprint 5."
            className="py-8"
          />
        </TabsContent>

        <TabsContent value="remotes" className="flex-1 overflow-auto p-4">
          <EmptyState
            icon={<span className="text-2xl">☁</span>}
            title="Remotes"
            description="Remote management coming in Sprint 3."
            className="py-8"
          />
        </TabsContent>

        <TabsContent value="worktrees" className="flex-1 overflow-auto p-4">
          <EmptyState
            icon={<span className="text-2xl">⑂</span>}
            title="Worktrees"
            description="Worktree list coming in Sprint 5."
            className="py-8"
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}

/** Main content area — shows the active workspace/repo context */
function MainContent(): React.JSX.Element {
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);

  if (!activeWorkspaceId) {
    return (
      <main className="flex flex-col flex-1 items-center justify-center bg-bg-primary">
        <EmptyState
          icon={<span className="text-4xl">◈</span>}
          title="Welcome to GitWave"
          description="Select a workspace from the sidebar to get started."
          className="py-16"
        />
      </main>
    );
  }

  return (
    <main className="flex flex-col flex-1 bg-bg-primary overflow-auto">
      <EmptyState
        icon={<span className="text-4xl">◈</span>}
        title="GitWave"
        description="Select a tab in the navigation pane to explore this repo."
        className="py-16"
      />
    </main>
  );
}

export default App;
