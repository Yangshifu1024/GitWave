// Operations toolbar below the top bar: the Local Changes trigger plus the
// sync actions that deserve persistent buttons (Fetch / Hooks / Pull / Push).
// Workspace / repository / branch management moved to the Toolbar menu bar
// (AppMenuBar), which dispatches `AppMenuAction` requests that this component
// consumes (see uiStore.menuAction) — every dialog and mutation still lives
// here, so the menu bar and the remaining buttons share identical handlers.
// Named `ActionBar` (not `ToolBar`) because Windows filesystems are
// case-insensitive and `Toolbar.tsx` already exists.

import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { listen } from "@tauri-apps/api/event";
import { ArrowDown, ArrowDownUp, ArrowUp, Archive, FileDiff } from "lucide-react";

import {
  addLocalRepo,
  addWorktree,
  cloneRepo,
  createBranch,
  createWorkspace,
  deleteWorkspace,
  exportWorkspace,
  formatAppError,
  importWorkspace,
  getBranches,
  initRepo,
  listRemotes,
  listWorkspaces,
  renameWorkspace,
  saveStash,
  setActiveRepo,
  type CloneProgress,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { useStatusAreaStore } from "@/stores/statusAreaStore";
import { useSyncStore } from "@/stores/syncStore";
import { useUiStore, type AppMenuAction } from "@/stores/uiStore";
import { useWorkingCopy } from "@/hooks/useWorkingCopy";
import { cn } from "@/lib/utils";
import { Separator } from "@heroui/react";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { Tooltip } from "@/components/ui/Tooltip";
import { PathInput } from "@/components/ui/PathInput";
import { ErrorAlert } from "@/components/ui/ErrorAlert";
import { Select } from "@/components/ui/Select";
import { Checkbox } from "@/components/ui/Checkbox";
import { Label } from "@/components/ui/Label";
import { AiProviderSettings } from "@/components/AiProviderSettings";
import { PrDescriptionModal } from "@/components/PrDescriptionModal";
import { LfsPanel } from "@/components/LfsPanel";
import { HooksPanel } from "@/components/HooksPanel";
import { WorkingCopyModal } from "@/components/ui/WorkingCopyModal";
import { WorkspaceDropdown } from "@/components/WorkspaceDropdown";
import { SyncStatusArea } from "@/components/SyncStatusArea";

interface PullDialogState {
  remote: string;
  branch: string;
  rebase: boolean;
  stash: boolean;
}

type AddKind = "init" | "clone" | "local" | null;

function ActionBarButton({
  icon,
  label,
  title,
  onClick,
  disabled = false,
  danger = false,
  tone,
}: {
  icon: React.ReactNode;
  /** Visible button text (short — the tooltip carries the full description). */
  label: string;
  /** Full description for tooltip / aria-label; defaults to `label`. */
  title?: string;
  onClick: () => void;
  disabled?: boolean;
  danger?: boolean;
  /** Text color override for stateful actions (e.g. clean vs dirty). */
  tone?: "success" | "warning";
}): React.JSX.Element {
  const hint = title ?? label;
  const toneClass =
    tone === "success"
      ? "text-success hover:bg-[color-mix(in_srgb,var(--color-success)_15%,transparent)]"
      : tone === "warning"
        ? "text-warning hover:bg-[color-mix(in_srgb,var(--color-warning)_15%,transparent)]"
        : danger
          ? "hover:text-danger"
          : "text-text-secondary hover:bg-bg-elevated hover:text-text-primary";
  return (
    <Tooltip content={hint}>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        disabled={disabled}
        aria-label={hint}
        onClick={onClick}
        className={cn(
          "flex items-center gap-1.5 h-7 px-2.5 rounded-md text-xs",
          toneClass,
          "disabled:opacity-40 disabled:pointer-events-none",
        )}
      >
        {icon}
        {label}
      </Button>
    </Tooltip>
  );
}

function deriveDestName(url: string): string {
  const trimmed = url.trim();
  if (trimmed.startsWith("git@")) {
    const after = trimmed.slice(trimmed.indexOf(":") + 1).replace(/\.git$/, "");
    return after.split("/").pop() ?? "repo";
  }
  const noProto = trimmed.replace(/^https?:\/\//, "").replace(/^ssh:\/\//, "");
  return (noProto.split("/").pop() ?? "repo").replace(/\.git$/, "");
}

export function ActionBar(): React.JSX.Element {
  const queryClient = useQueryClient();
  const setStatus = useStatusAreaStore((s) => s.setStatus);
  const startOp = useSyncStore((s) => s.startOp);
  const endOp = useSyncStore((s) => s.endOp);
  const activeWorkspaceId = useWorkspaceUiStore((s) => s.activeWorkspaceId);
  const activeRepoId = useWorkspaceUiStore((s) => s.activeRepoId);
  const selectWorkspace = useWorkspaceUiStore((s) => s.selectWorkspace);
  const setActiveRepoId = useWorkspaceUiStore((s) => s.setActiveRepoId);
  const bumpHistoryEpoch = useWorkspaceUiStore((s) => s.bumpHistoryEpoch);
  const wc = useWorkingCopy();

  // ── Workspace state ────────────────────────────────────────────────────
  const [createOpen, setCreateOpen] = useState(false);
  const [createName, setCreateName] = useState("");
  const [createError, setCreateError] = useState<string | null>(null);
  const [renameOpen, setRenameOpen] = useState(false);
  const [renameValue, setRenameValue] = useState("");
  const [renameError, setRenameError] = useState<string | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [aiOpen, setAiOpen] = useState(false);
  const [transferOpen, setTransferOpen] = useState<"export" | "import" | null>(null);
  const [transferPath, setTransferPath] = useState("");
  const [transferError, setTransferError] = useState<string | null>(null);

  const { data: workspaces = [] } = useQuery({
    queryKey: ["workspaces"],
    queryFn: listWorkspaces,
  });
  const activeWorkspace = workspaces.find((w) => w.id === activeWorkspaceId) ?? null;

  const refreshWorkspaces = (): void => {
    void queryClient.invalidateQueries({ queryKey: ["workspaces"] });
  };

  const createMut = useMutation({
    mutationFn: (name: string) => createWorkspace(name),
    onSuccess: () => {
      refreshWorkspaces();
      setCreateName("");
      setCreateError(null);
      setCreateOpen(false);
    },
    onError: (e: unknown) => setCreateError(formatAppError(e)),
  });

  const renameMut = useMutation({
    mutationFn: ({ id, newName }: { id: string; newName: string }) => renameWorkspace(id, newName),
    onSuccess: () => {
      refreshWorkspaces();
      setRenameOpen(false);
    },
    onError: (e: unknown) => setRenameError(formatAppError(e)),
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => deleteWorkspace(id),
    onSuccess: (_void, id) => {
      refreshWorkspaces();
      if (activeWorkspaceId === id) selectWorkspace(null);
      setDeleteOpen(false);
    },
  });

  function submitCreate(): void {
    const name = createName.trim();
    if (!name) return;
    setCreateError(null);
    createMut.mutate(name);
  }

  function openCreateWorkspace(): void {
    setCreateName("");
    setCreateError(null);
    setCreateOpen(true);
  }

  function openRename(): void {
    if (!activeWorkspace) return;
    setRenameValue(activeWorkspace.name);
    setRenameError(null);
    setRenameOpen(true);
  }

  function submitRename(): void {
    if (!activeWorkspace) return;
    const name = renameValue.trim();
    if (!name) return;
    setRenameError(null);
    renameMut.mutate({ id: activeWorkspace.id, newName: name });
  }

  function openExport(): void {
    setTransferPath(`${activeWorkspace?.name ?? "workspace"}.gitwave-workspace.json`);
    setTransferError(null);
    setTransferOpen("export");
  }

  function openImport(): void {
    setTransferPath("");
    setTransferError(null);
    setTransferOpen("import");
  }

  function submitTransfer(): void {
    const mode = transferOpen;
    if (!mode || !transferPath.trim()) return;
    const path = transferPath.trim();
    const done = (message: string): void => {
      setStatus(message);
      setTransferOpen(null);
      void queryClient.invalidateQueries({ queryKey: ["workspaces"] });
    };
    if (mode === "export") {
      if (!activeWorkspaceId) return;
      exportWorkspace(activeWorkspaceId, path)
        .then(() => done(`Workspace exported to ${path}`))
        .catch((e) => setTransferError(formatAppError(e)));
    } else {
      importWorkspace(path, null)
        .then((ws) => {
          done(`Workspace "${ws.name}" imported`);
          selectWorkspace(ws.id, ws.last_active_repo_id);
        })
        .catch((e) => setTransferError(formatAppError(e)));
    }
  }

  // ── Repository state ───────────────────────────────────────────────────
  const [adding, setAdding] = useState<AddKind>(null);
  const [initPath, setInitPath] = useState("");
  const [cloneUrl, setCloneUrl] = useState("");
  const [cloneDest, setCloneDest] = useState("");
  const [cloneProgress, setCloneProgress] = useState<CloneProgress | null>(null);
  const [cloneFailed, setCloneFailed] = useState(false);
  const [localPath, setLocalPath] = useState("");
  const [actionError, setActionError] = useState<string | null>(null);
  const [stashOpen, setStashOpen] = useState(false);
  const [stashMessage, setStashMessage] = useState("");
  const [stashStage, setStashStage] = useState(true);
  const [stashSaving, setStashSaving] = useState(false);
  const [worktreeCreateOpen, setWorktreeCreateOpen] = useState(false);
  const [wtStart, setWtStart] = useState("");
  const [wtName, setWtName] = useState("");
  const [wtLocation, setWtLocation] = useState("");
  const [wtBusy, setWtBusy] = useState(false);

  const refreshRepos = (): void => {
    void queryClient.invalidateQueries({ queryKey: ["repos", activeWorkspaceId] });
    refreshWorkspaces();
  };

  async function activateRepo(repoId: string): Promise<void> {
    if (!activeWorkspaceId) return;
    await setActiveRepo(activeWorkspaceId, repoId);
    setActiveRepoId(repoId);
    refreshWorkspaces();
  }

  function startAdd(kind: Exclude<AddKind, null>): void {
    setAdding(kind);
    setActionError(null);
    if (kind === "clone" && cloneDest === "") {
      setCloneDest(`./${deriveDestName(cloneUrl)}`);
    }
  }

  function endAdd(): void {
    setAdding(null);
    setActionError(null);
    setCloneProgress(null);
    setCloneFailed(false);
  }

  useEffect(() => {
    if (adding !== "clone") return;
    let unlisten: (() => void) | undefined;
    void listen<CloneProgress>("clone-progress", (event) => {
      setCloneProgress(event.payload);
    }).then((fn) => {
      unlisten = fn;
    });
    return () => {
      unlisten?.();
    };
  }, [adding]);

  const initMut = useMutation({
    mutationFn: ({ path }: { path: string }) => initRepo(activeWorkspaceId!, path),
    onSuccess: (repo) => {
      refreshRepos();
      setInitPath("");
      endAdd();
      void activateRepo(repo.id);
    },
    onError: (e: unknown) => setActionError(formatAppError(e)),
  });

  const cloneMut = useMutation({
    mutationFn: ({
      url,
      dest,
      replaceDest,
    }: {
      url: string;
      dest: string;
      replaceDest?: boolean;
    }) => cloneRepo(activeWorkspaceId!, url, dest, replaceDest ?? false),
    onMutate: () => {
      setCloneFailed(false);
      setCloneProgress(null);
      setActionError(null);
    },
    onSuccess: (repo) => {
      refreshRepos();
      setCloneUrl("");
      setCloneDest("");
      endAdd();
      void activateRepo(repo.id);
    },
    onError: (e: unknown) => {
      setActionError(formatAppError(e));
      setCloneFailed(true);
    },
  });

  const localMut = useMutation({
    mutationFn: ({ path }: { path: string }) => addLocalRepo(activeWorkspaceId!, path),
    onSuccess: (repo) => {
      refreshRepos();
      setLocalPath("");
      endAdd();
      void activateRepo(repo.id);
    },
    onError: (e: unknown) => setActionError(formatAppError(e)),
  });

  // ── Branch state ───────────────────────────────────────────────────────
  const [branchCreateOpen, setBranchCreateOpen] = useState(false);
  const [branchName, setBranchName] = useState("");
  const [branchError, setBranchError] = useState<string | null>(null);
  const [pullDialog, setPullDialog] = useState<PullDialogState | null>(null);
  const [pushDialog, setPushDialog] = useState<{ tags: boolean; force: boolean } | null>(null);
  const [prOpen, setPrOpen] = useState(false);
  const [lfsOpen, setLfsOpen] = useState(false);
  const [hooksOpen, setHooksOpen] = useState(false);

  const branchesQuery = useQuery({
    queryKey: ["branches", activeWorkspaceId],
    queryFn: () => getBranches(activeWorkspaceId!),
    enabled: (pullDialog !== null || worktreeCreateOpen) && Boolean(activeWorkspaceId),
  });

  const branchCreateMut = useMutation({
    mutationFn: ({ name, fromSha }: { name: string; fromSha: string }) =>
      createBranch(activeWorkspaceId!, name, fromSha),
    onSuccess: () => {
      bumpHistoryEpoch();
      setBranchName("");
      setBranchError(null);
      setBranchCreateOpen(false);
    },
    onError: (e: unknown) => setBranchError(formatAppError(e)),
  });

  function submitBranchCreate(): void {
    const name = branchName.trim();
    // On a branch, HEAD tip == current branch tip (detached is gated on the button).
    const fromSha = wc.data?.sha;
    if (!name) return;
    if (!fromSha) {
      setBranchError("No current branch tip to branch from");
      return;
    }
    setBranchError(null);
    branchCreateMut.mutate({ name, fromSha });
  }

  const detached = wc.data?.branch === "(detached)";

  function openBranchCreate(): void {
    setBranchName("");
    setBranchError(null);
    setBranchCreateOpen(true);
  }

  function openPushDialog(): void {
    setPushDialog({ tags: false, force: false });
  }

  const remotesQuery = useQuery({
    queryKey: ["remotes", activeWorkspaceId],
    queryFn: () => listRemotes(activeWorkspaceId!),
    enabled: pullDialog !== null && Boolean(activeWorkspaceId),
  });
  const remotes = useMemo(() => remotesQuery.data ?? [], [remotesQuery.data]);

  const openPullDialog = () => {
    const d = wc.data;
    const upstream = d?.upstream ?? "";
    const slash = upstream.indexOf("/");
    setPullDialog({
      remote: slash > 0 ? upstream.slice(0, slash) : "origin",
      branch: slash > 0 ? upstream.slice(slash + 1) : (d?.branch ?? "main"),
      rebase: false,
      stash: false,
    });
  };

  const handleSaveStash = async (): Promise<void> => {
    if (!activeWorkspaceId || stashSaving) return;
    setStashSaving(true);
    wc.setActionError(null);
    startOp("stash");
    try {
      await saveStash(activeWorkspaceId, stashMessage.trim() || undefined, stashStage);
      setStatus("Saved stash", "success");
      void queryClient.invalidateQueries({ queryKey: ["stashes", activeWorkspaceId] });
      void queryClient.invalidateQueries({
        queryKey: ["working-copy", activeWorkspaceId, activeRepoId],
      });
      setStashOpen(false);
      setStashMessage("");
    } catch (e) {
      wc.setActionError(formatAppError(e));
    } finally {
      setStashSaving(false);
      endOp("stash");
    }
  };

  const openWorktreeCreate = (): void => {
    setWtStart(wc.data?.branch ?? "");
    setWtName("");
    setWtLocation("");
    setWorktreeCreateOpen(true);
  };

  const handleCreateWorktree = async (): Promise<void> => {
    if (!activeWorkspaceId || wtBusy) return;
    const name = wtName.trim();
    const location = wtLocation.trim();
    if (!name || !location) return;
    setWtBusy(true);
    wc.setActionError(null);
    startOp("worktree");
    try {
      await addWorktree(activeWorkspaceId, name, location, name, true, wtStart || undefined);
      setStatus(`Created worktree ${name}`, "success");
      void queryClient.invalidateQueries({ queryKey: ["repos", activeWorkspaceId] });
      setWorktreeCreateOpen(false);
      setWtName("");
      setWtLocation("");
    } catch (e) {
      wc.setActionError(formatAppError(e));
    } finally {
      setWtBusy(false);
      endOp("worktree");
    }
  };

  // The fetched remote list wins over the seeded default once it arrives.
  useEffect(() => {
    if (!pullDialog || remotes.length === 0) return;
    if (!remotes.includes(pullDialog.remote)) {
      setPullDialog({ ...pullDialog, remote: remotes[0]! });
    }
  }, [pullDialog, remotes]);

  const remoteOptions = remotes.length > 0 ? remotes : [pullDialog?.remote ?? "origin"];

  // "Start from" choices for the Create Worktree dialog: local branches.
  const wtBranchOptions = worktreeCreateOpen
    ? (branchesQuery.data ?? []).filter((b) => b.kind === "local").map((b) => b.name)
    : [];
  const branchOptions = (() => {
    if (!pullDialog) return [];
    const prefix = `${pullDialog.remote}/`;
    const names = (branchesQuery.data ?? [])
      .filter((b) => b.kind === "remote" && b.name.startsWith(prefix))
      .map((b) => b.name.slice(prefix.length));
    if (!names.includes(pullDialog.branch)) names.unshift(pullDialog.branch);
    return names;
  })();

  // ── Local Changes ──────────────────────────────────────────────────────
  const [wcModalOpen, setWcModalOpen] = useState(false);
  const changeCount = wc.data?.files.length ?? 0;
  const localChangesDisabled = !activeRepoId || changeCount === 0;

  const noRepo = !activeRepoId;

  // ── Menu bar requests ──────────────────────────────────────────────────
  // AppMenuBar dispatches AppMenuAction requests; route each one to the same
  // handler its (former) ActionBar button used, so menu and button behavior
  // stay identical by construction. The router lives in a ref to keep the
  // effect deps noise-free while always seeing fresh state.
  const menuAction = useUiStore((s) => s.menuAction);
  const clearMenuAction = useUiStore((s) => s.clearMenuAction);

  const runMenuActionRef = useRef<(action: AppMenuAction) => void>(() => undefined);
  runMenuActionRef.current = (action) => {
    switch (action) {
      case "workspace:new":
        openCreateWorkspace();
        break;
      case "workspace:rename":
        openRename();
        break;
      case "workspace:ai":
        setAiOpen(true);
        break;
      case "workspace:export":
        openExport();
        break;
      case "workspace:import":
        openImport();
        break;
      case "workspace:delete":
        setDeleteOpen(true);
        break;
      case "repo:init":
        startAdd("init");
        break;
      case "repo:clone":
        startAdd("clone");
        break;
      case "repo:add":
        startAdd("local");
        break;
      case "repo:fetch":
        wc.fetch();
        break;
      case "repo:lfs":
        setLfsOpen(true);
        break;
      case "repo:hooks":
        setHooksOpen(true);
        break;
      case "repo:worktree-new":
        openWorktreeCreate();
        break;
      case "branch:new":
        openBranchCreate();
        break;
      case "branch:pull":
        openPullDialog();
        break;
      case "branch:push":
        openPushDialog();
        break;
      case "branch:pr":
        setPrOpen(true);
        break;
      default: {
        // Compile error when a new AppMenuAction member misses its routing.
        const exhaustive: never = action;
        throw new Error(`Unhandled menu action: ${String(exhaustive)}`);
      }
    }
  };

  useEffect(() => {
    if (!menuAction) return;
    runMenuActionRef.current(menuAction.action);
    clearMenuAction(menuAction.id);
  }, [menuAction, clearMenuAction]);

  return (
    <>
      <div className="relative flex items-center gap-3 px-3 py-1.5 shrink-0 bg-bg-primary border-b border-border-subtle select-none">
        <WorkspaceDropdown />

        {/* Reserved middle zone between the selector and the ops. */}
        <div className="flex-1" />

        <div className="flex items-center gap-1">
          <ActionBarButton
            icon={<FileDiff size={14} />}
            label={changeCount > 0 ? `Changes(${changeCount})` : "Changes"}
            title="Local changes"
            tone={changeCount > 0 ? "warning" : "success"}
            disabled={localChangesDisabled}
            onClick={() => setWcModalOpen(true)}
          />
          <ActionBarButton
            icon={<Archive size={14} />}
            label="Stash"
            title="Stash local changes"
            disabled={localChangesDisabled}
            onClick={() => setStashOpen(true)}
          />
        </div>
        <Separator orientation="vertical" className="mx-1 h-8 w-px self-center bg-border-subtle" />
        <div className="flex items-center gap-1">
          <ActionBarButton
            icon={<ArrowDownUp size={14} />}
            label="Fetch"
            disabled={noRepo || wc.isSyncBusy}
            onClick={wc.fetch}
          />
          <ActionBarButton
            icon={<ArrowDown size={14} />}
            label={wc.data && wc.data.behind > 0 ? `Pull (${wc.data.behind})` : "Pull"}
            title="Pull"
            disabled={noRepo || wc.isSyncBusy || detached}
            onClick={openPullDialog}
          />
          <ActionBarButton
            icon={<ArrowUp size={14} />}
            label={wc.data && wc.data.ahead > 0 ? `Push (${wc.data.ahead})` : "Push"}
            title="Push"
            disabled={noRepo || wc.isSyncBusy || detached}
            onClick={openPushDialog}
          />
        </div>

        {/* Status area: absolutely centered in the whole bar so it stays on
            the window's center axis regardless of the selector / buttons
            widths. Non-interactive, so pointer-events-none keeps the buttons
            clickable even if a narrow window overlaps them. */}
        <div className="absolute inset-x-0 flex justify-center pointer-events-none">
          <SyncStatusArea />
        </div>
      </div>

      <ErrorAlert message={wc.actionError} onDismiss={() => wc.setActionError(null)} />

      {/* Workspace: create */}
      <Modal
        open={createOpen}
        onOpenChange={(open) => {
          setCreateOpen(open);
          if (!open) {
            setCreateName("");
            setCreateError(null);
          }
        }}
        title="New Workspace"
        size="sm"
      >
        <Input
          autoFocus
          value={createName}
          onChange={setCreateName}
          onKeyDown={(e) => {
            if (e.key === "Enter") submitCreate();
          }}
          placeholder="Workspace name"
          error={createError}
        />
        <div className="mt-3 flex justify-end gap-2">
          <Button variant="secondary" size="sm" onClick={() => setCreateOpen(false)}>
            Cancel
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={submitCreate}
            disabled={!createName.trim() || createMut.isPending}
          >
            Create
          </Button>
        </div>
      </Modal>

      {/* Workspace: export / import transfer file */}
      <Modal
        open={transferOpen !== null}
        onOpenChange={(open) => {
          if (!open) setTransferOpen(null);
        }}
        title={transferOpen === "export" ? "Export workspace" : "Import workspace"}
        description={
          transferOpen === "export"
            ? "Writes a .gitwave-workspace.json with the workspace name and repo paths. API keys are never included."
            : "Reads a .gitwave-workspace.json, creates a new workspace and re-adds repos that exist on disk."
        }
        size="sm"
      >
        <Input
          autoFocus
          value={transferPath}
          onChange={setTransferPath}
          onKeyDown={(e) => {
            if (e.key === "Enter") submitTransfer();
          }}
          placeholder={
            transferOpen === "export"
              ? "destination .json path"
              : "source .gitwave-workspace.json path"
          }
          error={transferError}
        />
        <div className="mt-3 flex justify-end gap-2">
          <Button variant="secondary" size="sm" onClick={() => setTransferOpen(null)}>
            Cancel
          </Button>
          <Button
            variant="primary"
            size="sm"
            disabled={!transferPath.trim()}
            onClick={submitTransfer}
          >
            {transferOpen === "export" ? "Export" : "Import"}
          </Button>
        </div>
      </Modal>

      {/* Workspace: rename */}
      <Modal
        open={renameOpen}
        onOpenChange={(open) => {
          if (!open) setRenameOpen(false);
        }}
        title={activeWorkspace ? `Rename "${activeWorkspace.name}"` : "Rename workspace"}
        size="sm"
      >
        <Input
          autoFocus
          value={renameValue}
          onChange={setRenameValue}
          onKeyDown={(e) => {
            if (e.key === "Enter") submitRename();
          }}
          error={renameError}
        />
        <div className="mt-3 flex justify-end gap-2">
          <Button variant="secondary" size="sm" onClick={() => setRenameOpen(false)}>
            Cancel
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={submitRename}
            disabled={!renameValue.trim() || renameMut.isPending}
          >
            Save
          </Button>
        </div>
      </Modal>

      {/* Workspace: delete */}
      <Modal
        open={deleteOpen}
        onOpenChange={(open) => {
          if (!open) setDeleteOpen(false);
        }}
        title={activeWorkspace ? `Delete "${activeWorkspace.name}"?` : "Delete workspace?"}
        description="This removes the workspace and its repo references from GitWave. It does not delete the local repositories themselves."
        size="sm"
      >
        <div className="mt-3 flex justify-end gap-2">
          <Button variant="secondary" size="sm" onClick={() => setDeleteOpen(false)}>
            Cancel
          </Button>
          <Button
            variant="danger"
            size="sm"
            onClick={() => deleteMut.mutate(activeWorkspaceId!)}
            disabled={deleteMut.isPending}
          >
            Delete
          </Button>
        </div>
      </Modal>

      <AiProviderSettings
        workspaceId={activeWorkspaceId}
        workspaceName={activeWorkspace?.name}
        open={aiOpen}
        onOpenChange={setAiOpen}
      />

      {prOpen && activeWorkspaceId ? (
        <PrDescriptionModal workspaceId={activeWorkspaceId} open onClose={() => setPrOpen(false)} />
      ) : null}

      {lfsOpen && activeWorkspaceId ? (
        <LfsPanel workspaceId={activeWorkspaceId} open onClose={() => setLfsOpen(false)} />
      ) : null}

      {hooksOpen && activeWorkspaceId ? (
        <HooksPanel workspaceId={activeWorkspaceId} open onClose={() => setHooksOpen(false)} />
      ) : null}

      {/* Repository: init */}
      {adding === "init" && (
        <Modal
          open={true}
          onOpenChange={(open) => {
            if (!open) endAdd();
          }}
          title="Initialize new repo"
          description="Create a fresh Git repository in an empty folder."
          size="sm"
        >
          <div className="flex flex-col gap-1">
            <Label className="text-xs font-medium text-text-secondary" htmlFor="init-path">
              Location
            </Label>
            <PathInput
              id="init-path"
              autoFocus
              directory
              value={initPath}
              onChange={setInitPath}
              onKeyDown={(e) => {
                if (e.key === "Enter" && initPath.trim()) initMut.mutate({ path: initPath.trim() });
              }}
              placeholder="/Users/me/projects/new"
              error={actionError}
            />
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={endAdd}>
              Cancel
            </Button>
            <Button
              variant="primary"
              size="sm"
              onClick={() => initMut.mutate({ path: initPath.trim() })}
              disabled={!initPath.trim() || initMut.isPending}
            >
              Create
            </Button>
          </div>
        </Modal>
      )}

      {/* Repository: clone */}
      {adding === "clone" && (
        <Modal
          open={true}
          onOpenChange={(open) => {
            if (!open) endAdd();
          }}
          title="Clone remote repo"
          description="Copy a remote repository into a local folder."
          size="sm"
        >
          <div className="flex flex-col gap-3">
            <div className="flex flex-col gap-1">
              <Label className="text-xs font-medium text-text-secondary" htmlFor="clone-url">
                URL
              </Label>
              <Input
                id="clone-url"
                autoFocus
                value={cloneUrl}
                onChange={setCloneUrl}
                onKeyDown={(e) => {
                  if (e.key === "Enter" && cloneUrl.trim() && cloneDest.trim()) {
                    cloneMut.mutate({ url: cloneUrl.trim(), dest: cloneDest.trim() });
                  }
                }}
                placeholder="https://github.com/user/repo.git or git@github.com:user/repo.git"
              />
              <p className="text-xs text-text-muted">
                Detected protocol:{" "}
                {cloneUrl
                  ? cloneUrl.startsWith("git@") || cloneUrl.startsWith("ssh://")
                    ? "ssh"
                    : "https"
                  : "—"}
              </p>
            </div>
            <div className="flex flex-col gap-1">
              <Label className="text-xs font-medium text-text-secondary" htmlFor="clone-dest">
                Destination path
              </Label>
              <PathInput
                id="clone-dest"
                directory
                value={cloneDest}
                onChange={setCloneDest}
                placeholder="./repo"
              />
            </div>
            {actionError && <p className="text-xs text-danger">{actionError}</p>}
            {cloneMut.isPending || cloneProgress ? (
              <div className="flex flex-col gap-1">
                <div className="h-1.5 rounded bg-bg-secondary overflow-hidden">
                  <div
                    className="h-full bg-accent transition-[width] duration-base"
                    style={{
                      width: `${
                        cloneProgress && cloneProgress.totalObjects > 0
                          ? Math.min(
                              100,
                              Math.round(
                                (100 * cloneProgress.receivedObjects) / cloneProgress.totalObjects,
                              ),
                            )
                          : cloneMut.isPending
                            ? 8
                            : 0
                      }%`,
                    }}
                  />
                </div>
                <p className="text-[11px] text-text-muted font-mono">
                  {cloneProgress
                    ? `objects ${cloneProgress.receivedObjects}/${cloneProgress.totalObjects || "?"} · deltas ${cloneProgress.indexedDeltas}/${cloneProgress.totalDeltas || "?"} · ${Math.round(cloneProgress.receivedBytes / 1024)} KiB`
                    : "Starting clone…"}
                </p>
              </div>
            ) : null}
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={endAdd} disabled={cloneMut.isPending}>
              Cancel
            </Button>
            {cloneFailed ? (
              <Button
                variant="primary"
                size="sm"
                onClick={() =>
                  cloneMut.mutate({
                    url: cloneUrl.trim(),
                    dest: cloneDest.trim(),
                    replaceDest: true,
                  })
                }
                disabled={!cloneUrl.trim() || !cloneDest.trim() || cloneMut.isPending}
              >
                Retry
              </Button>
            ) : (
              <Button
                variant="primary"
                size="sm"
                onClick={() => cloneMut.mutate({ url: cloneUrl.trim(), dest: cloneDest.trim() })}
                disabled={!cloneUrl.trim() || !cloneDest.trim() || cloneMut.isPending}
              >
                {cloneMut.isPending ? "Cloning…" : "Clone"}
              </Button>
            )}
          </div>
        </Modal>
      )}

      {/* Repository: add local */}
      {adding === "local" && (
        <Modal
          open={true}
          onOpenChange={(open) => {
            if (!open) endAdd();
          }}
          title="Add existing local repo"
          description="Register an existing Git working tree with this workspace."
          size="sm"
        >
          <div className="flex flex-col gap-1">
            <Label className="text-xs font-medium text-text-secondary" htmlFor="add-local-path">
              Location
            </Label>
            <PathInput
              id="add-local-path"
              autoFocus
              directory
              value={localPath}
              onChange={setLocalPath}
              onKeyDown={(e) => {
                if (e.key === "Enter" && localPath.trim())
                  localMut.mutate({ path: localPath.trim() });
              }}
              placeholder="/Users/me/projects/existing"
              error={actionError}
            />
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={endAdd}>
              Cancel
            </Button>
            <Button
              variant="primary"
              size="sm"
              onClick={() => localMut.mutate({ path: localPath.trim() })}
              disabled={!localPath.trim() || localMut.isPending}
            >
              Add
            </Button>
          </div>
        </Modal>
      )}

      {/* Branch: create */}
      <Modal
        open={branchCreateOpen}
        onOpenChange={(open) => {
          if (!open) setBranchCreateOpen(false);
        }}
        title={`New branch from "${wc.data?.branch ?? "?"}"`}
        description="Creates a local branch at the current tip; the current branch stays checked out."
        size="sm"
      >
        <Input
          autoFocus
          value={branchName}
          onChange={setBranchName}
          onKeyDown={(e) => {
            if (e.key === "Enter") submitBranchCreate();
          }}
          placeholder="feature/my-branch"
          error={branchError}
        />
        <div className="mt-3 flex justify-end gap-2">
          <Button variant="secondary" size="sm" onClick={() => setBranchCreateOpen(false)}>
            Cancel
          </Button>
          <Button
            variant="primary"
            size="sm"
            onClick={submitBranchCreate}
            disabled={!branchName.trim() || branchCreateMut.isPending}
          >
            Create
          </Button>
        </div>
      </Modal>

      {/* Branch: pull (Fork-style) */}
      {pullDialog ? (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setPullDialog(null);
          }}
          title="Pull"
          description="Pull remote branches and merge them into your local branch"
          size="sm"
        >
          <div className="flex flex-col gap-2">
            <Label className="flex items-center gap-2">
              <span className="w-16 shrink-0 text-right text-xs text-text-secondary">Remote</span>
              <Select
                aria-label="Remote"
                className="h-auto flex-1 min-w-0 bg-bg-primary border-border-subtle px-1.5 py-1 text-xs"
                value={pullDialog.remote}
                disabled={wc.isSyncBusy}
                onChange={(remote) => setPullDialog({ ...pullDialog, remote })}
                options={remoteOptions.map((r) => ({ value: r, label: r }))}
              />
            </Label>
            <Label className="flex items-center gap-2">
              <span className="w-16 shrink-0 text-right text-xs text-text-secondary">Branch</span>
              <Select
                aria-label="Branch"
                className="h-auto flex-1 min-w-0 bg-bg-primary border-border-subtle px-1.5 py-1 text-xs"
                value={pullDialog.branch}
                disabled={wc.isSyncBusy}
                onChange={(branch) => setPullDialog({ ...pullDialog, branch })}
                options={branchOptions.map((b) => ({
                  value: b,
                  label: `${pullDialog.remote}/${b}`,
                }))}
              />
            </Label>
            <div className="flex items-center gap-2">
              <span className="w-16 shrink-0 text-right text-xs text-text-secondary">Into</span>
              <span className="flex-1 min-w-0 truncate text-xs font-mono text-text-primary">
                {wc.data?.branch ?? "—"}
              </span>
            </div>
            <Checkbox
              checked={pullDialog.rebase}
              disabled={wc.isSyncBusy}
              onChange={(rebase) => setPullDialog({ ...pullDialog, rebase })}
            >
              Rebase instead of merge
            </Checkbox>
            <Checkbox
              checked={pullDialog.stash}
              disabled={wc.isSyncBusy}
              onChange={(stash) => setPullDialog({ ...pullDialog, stash })}
            >
              Stash and reapply local changes
            </Checkbox>
          </div>
          <div className="mt-3 flex justify-end gap-2">
            <Button variant="secondary" size="sm" onClick={() => setPullDialog(null)}>
              Cancel
            </Button>
            <Button
              variant="primary"
              size="sm"
              disabled={wc.isSyncBusy}
              onClick={() => {
                const options = {
                  remote: pullDialog.remote,
                  branch: pullDialog.branch,
                  rebase: pullDialog.rebase,
                  stash: pullDialog.stash,
                };
                setPullDialog(null);
                wc.pull(options);
              }}
            >
              Pull
            </Button>
          </div>
        </Modal>
      ) : null}

      {/* Branch: push (Fork-style confirm) */}
      {pushDialog ? (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setPushDialog(null);
          }}
          title="Push"
          description="Push your local changes to remote repository"
          size="sm"
          footer={
            <>
              <Button variant="secondary" size="sm" onClick={() => setPushDialog(null)}>
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                disabled={wc.isSyncBusy}
                onClick={() => {
                  const options = {
                    remote: "origin",
                    tags: pushDialog.tags,
                    force: pushDialog.force,
                  };
                  setPushDialog(null);
                  wc.push(options);
                }}
              >
                Push
              </Button>
            </>
          }
        >
          <div className="flex items-center gap-2">
            <span className="w-16 shrink-0 text-sm text-text-secondary">Branch</span>
            <span className="min-w-0 flex-1 truncate font-mono text-sm text-text-primary">
              {wc.data?.branch ?? "—"}
            </span>
          </div>
          <div className="flex items-center gap-2">
            <span className="w-16 shrink-0 text-sm text-text-secondary">To</span>
            <span className="min-w-0 flex-1 truncate font-mono text-sm text-text-primary">
              default ({wc.data?.upstream ?? `origin/${wc.data?.branch ?? ""}`})
            </span>
          </div>
          <Checkbox
            checked={pushDialog.tags}
            disabled={wc.isSyncBusy}
            onChange={(tags) => setPushDialog({ ...pushDialog, tags })}
          >
            Push all tags
          </Checkbox>
          <Checkbox
            checked={pushDialog.force}
            disabled={wc.isSyncBusy}
            onChange={(force) => setPushDialog({ ...pushDialog, force })}
          >
            Force push
          </Checkbox>
        </Modal>
      ) : null}

      {stashOpen ? (
        <Modal
          open
          onOpenChange={(o) => !o && setStashOpen(false)}
          title="Save stash"
          description="Save your local changes to a new stash"
          size="sm"
        >
          <div className="flex flex-col gap-2">
            <Input
              value={stashMessage}
              onChange={setStashMessage}
              placeholder="Stash message (optional)"
              autoFocus
              onKeyDown={(e) => {
                if (e.key === "Enter" && !stashSaving) void handleSaveStash();
              }}
            />
            <Checkbox
              checked={stashStage}
              disabled={stashSaving}
              onChange={(v) => setStashStage(v)}
              className="items-start text-text-primary"
            >
              <span className="flex flex-col">
                <span>Stage new files</span>
                <span className="text-xs font-normal text-text-muted">
                  By default stash ignores new files until you stage them
                </span>
              </span>
            </Checkbox>
            <div className="flex justify-end gap-2">
              <Button variant="secondary" size="sm" onClick={() => setStashOpen(false)}>
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                disabled={stashSaving}
                onClick={() => void handleSaveStash()}
              >
                Save Stash
              </Button>
            </div>
          </div>
        </Modal>
      ) : null}

      {worktreeCreateOpen ? (
        <Modal
          open
          onOpenChange={(o) => !o && setWorktreeCreateOpen(false)}
          title="Create Worktree"
          description="Create branch and check out it in a separate worktree"
          size="sm"
        >
          <div className="flex flex-col gap-2">
            <Label>Start from</Label>
            <Select
              aria-label="Start from"
              className="h-8 bg-bg-primary border-border-subtle px-1.5 text-xs"
              value={wtStart}
              disabled={wtBusy}
              onChange={(v) => setWtStart(v)}
              options={wtBranchOptions.map((b) => ({ value: b, label: b }))}
            />
            <Label>Branch name</Label>
            <Input
              value={wtName}
              onChange={setWtName}
              placeholder="Enter branch name"
              disabled={wtBusy}
            />
            <Label>Location</Label>
            <PathInput
              directory
              value={wtLocation}
              onChange={setWtLocation}
              placeholder="Path for the new worktree"
              disabled={wtBusy}
            />
            <div className="mt-1 flex justify-end gap-2">
              <Button
                variant="secondary"
                size="sm"
                onClick={() => setWorktreeCreateOpen(false)}
                disabled={wtBusy}
              >
                Cancel
              </Button>
              <Button
                variant="primary"
                size="sm"
                disabled={wtBusy || !wtName.trim() || !wtLocation.trim()}
                onClick={() => void handleCreateWorktree()}
              >
                Create
              </Button>
            </div>
          </div>
        </Modal>
      ) : null}

      <WorkingCopyModal open={wcModalOpen} onOpenChange={setWcModalOpen} />
    </>
  );
}
