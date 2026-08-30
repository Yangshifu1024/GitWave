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
import { useTranslation } from "react-i18next";

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
  const { t } = useTranslation();
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
        .then(() => done(t("commits.workspace.exported", { path })))
        .catch((e) => setTransferError(formatAppError(e)));
    } else {
      importWorkspace(path, null)
        .then((ws) => {
          done(t("commits.workspace.imported", { name: ws.name }));
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
      setBranchError(t("commits.branch.noTipError"));
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
      setStatus(t("changes.stash.saved"), "success");
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
      setStatus(t("commits.worktree.created", { name }), "success");
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
            label={
              changeCount > 0
                ? t("changes.actionBar.changesWithCount", { total: changeCount })
                : t("changes.actionBar.changes")
            }
            title={t("changes.actionBar.localChangesTitle")}
            tone={changeCount > 0 ? "warning" : "success"}
            disabled={localChangesDisabled}
            onClick={() => setWcModalOpen(true)}
          />
          <ActionBarButton
            icon={<Archive size={14} />}
            label={t("changes.stash.title")}
            title={t("changes.stash.buttonTitle")}
            disabled={localChangesDisabled}
            onClick={() => setStashOpen(true)}
          />
        </div>
        <Separator orientation="vertical" className="mx-1 h-8 w-px self-center bg-border-subtle" />
        <div className="flex items-center gap-1">
          <ActionBarButton
            icon={<ArrowDownUp size={14} />}
            label={t("commits.sync.fetch")}
            disabled={noRepo || wc.isSyncBusy}
            onClick={wc.fetch}
          />
          <ActionBarButton
            icon={<ArrowDown size={14} />}
            label={
              wc.data && wc.data.behind > 0
                ? t("commits.sync.pullWithCount", { total: wc.data.behind })
                : t("commits.sync.pull")
            }
            title={t("commits.sync.pullTitle")}
            disabled={noRepo || wc.isSyncBusy || detached}
            onClick={openPullDialog}
          />
          <ActionBarButton
            icon={<ArrowUp size={14} />}
            label={
              wc.data && wc.data.ahead > 0
                ? t("commits.sync.pushWithCount", { total: wc.data.ahead })
                : t("commits.sync.push")
            }
            title={t("commits.sync.pushTitle")}
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
        title={t("commits.workspace.createTitle")}
        size="sm"
        footer={
          <>
            <Button
              variant="secondary"
              size="sm"
              className="min-w-0 flex-[3]"
              onClick={() => setCreateOpen(false)}
            >
              {t("commits.action.cancel")}
            </Button>
            <Button
              variant="primary"
              size="sm"
              className="min-w-0 flex-[7]"
              onClick={submitCreate}
              disabled={!createName.trim() || createMut.isPending}
            >
              {t("commits.action.create")}
            </Button>
          </>
        }
      >
        <div className="rounded-xl bg-bg-primary p-3">
          <Input
            autoFocus
            value={createName}
            onChange={setCreateName}
            onKeyDown={(e) => {
              if (e.key === "Enter") submitCreate();
            }}
            placeholder={t("commits.workspace.namePlaceholder")}
            error={createError}
          />
        </div>
      </Modal>

      {/* Workspace: export / import transfer file */}
      <Modal
        open={transferOpen !== null}
        onOpenChange={(open) => {
          if (!open) setTransferOpen(null);
        }}
        title={
          transferOpen === "export"
            ? t("commits.workspace.exportTitle")
            : t("commits.workspace.importTitle")
        }
        description={
          transferOpen === "export"
            ? t("commits.workspace.exportDescription")
            : t("commits.workspace.importDescription")
        }
        size="sm"
        footer={
          <>
            <Button
              variant="secondary"
              size="sm"
              className="min-w-0 flex-[3]"
              onClick={() => setTransferOpen(null)}
            >
              {t("commits.action.cancel")}
            </Button>
            <Button
              variant="primary"
              size="sm"
              className="min-w-0 flex-[7]"
              disabled={!transferPath.trim()}
              onClick={submitTransfer}
            >
              {transferOpen === "export" ? t("commits.action.export") : t("commits.action.import")}
            </Button>
          </>
        }
      >
        <div className="rounded-xl bg-bg-primary p-3">
          <Input
            autoFocus
            value={transferPath}
            onChange={setTransferPath}
            onKeyDown={(e) => {
              if (e.key === "Enter") submitTransfer();
            }}
            placeholder={
              transferOpen === "export"
                ? t("commits.workspace.exportPathPlaceholder")
                : t("commits.workspace.importPathPlaceholder")
            }
            error={transferError}
          />
        </div>
      </Modal>

      {/* Workspace: rename */}
      <Modal
        open={renameOpen}
        onOpenChange={(open) => {
          if (!open) setRenameOpen(false);
        }}
        title={
          activeWorkspace
            ? t("commits.workspace.renameTitle", { name: activeWorkspace.name })
            : t("commits.workspace.renameFallback")
        }
        size="sm"
        footer={
          <>
            <Button
              variant="secondary"
              size="sm"
              className="min-w-0 flex-[3]"
              onClick={() => setRenameOpen(false)}
            >
              {t("commits.action.cancel")}
            </Button>
            <Button
              variant="primary"
              size="sm"
              className="min-w-0 flex-[7]"
              onClick={submitRename}
              disabled={!renameValue.trim() || renameMut.isPending}
            >
              {t("commits.action.save")}
            </Button>
          </>
        }
      >
        <div className="rounded-xl bg-bg-primary p-3">
          <Input
            autoFocus
            value={renameValue}
            onChange={setRenameValue}
            onKeyDown={(e) => {
              if (e.key === "Enter") submitRename();
            }}
            error={renameError}
          />
        </div>
      </Modal>

      {/* Workspace: delete */}
      <Modal
        open={deleteOpen}
        onOpenChange={(open) => {
          if (!open) setDeleteOpen(false);
        }}
        title={
          activeWorkspace
            ? t("commits.workspace.deleteTitle", { name: activeWorkspace.name })
            : t("commits.workspace.deleteFallback")
        }
        description={t("commits.workspace.deleteDescription")}
        size="sm"
        footer={
          <>
            <Button
              variant="secondary"
              size="sm"
              className="min-w-0 flex-[3]"
              onClick={() => setDeleteOpen(false)}
            >
              {t("commits.action.cancel")}
            </Button>
            <Button
              variant="danger"
              size="sm"
              className="min-w-0 flex-[7]"
              onClick={() => deleteMut.mutate(activeWorkspaceId!)}
              disabled={deleteMut.isPending}
            >
              {t("commits.action.delete")}
            </Button>
          </>
        }
      />

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
          title={t("commits.repo.initTitle")}
          description={t("commits.repo.initDescription")}
          size="sm"
          footer={
            <>
              <Button variant="secondary" size="sm" className="min-w-0 flex-[3]" onClick={endAdd}>
                {t("commits.action.cancel")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="min-w-0 flex-[7]"
                onClick={() => initMut.mutate({ path: initPath.trim() })}
                disabled={!initPath.trim() || initMut.isPending}
              >
                {t("commits.action.create")}
              </Button>
            </>
          }
        >
          <div className="flex flex-col gap-1 rounded-xl bg-bg-primary p-3">
            <Label className="text-sm font-medium text-text-secondary" htmlFor="init-path">
              {t("commits.repo.location")}
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
        </Modal>
      )}

      {/* Repository: clone */}
      {adding === "clone" && (
        <Modal
          open={true}
          onOpenChange={(open) => {
            if (!open) endAdd();
          }}
          title={t("commits.repo.cloneTitle")}
          description={t("commits.repo.cloneDescription")}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={endAdd}
                disabled={cloneMut.isPending}
              >
                {t("commits.action.cancel")}
              </Button>
              {cloneFailed ? (
                <Button
                  variant="primary"
                  size="sm"
                  className="min-w-0 flex-[7]"
                  onClick={() =>
                    cloneMut.mutate({
                      url: cloneUrl.trim(),
                      dest: cloneDest.trim(),
                      replaceDest: true,
                    })
                  }
                  disabled={!cloneUrl.trim() || !cloneDest.trim() || cloneMut.isPending}
                >
                  {t("commits.action.retry")}
                </Button>
              ) : (
                <Button
                  variant="primary"
                  size="sm"
                  className="min-w-0 flex-[7]"
                  onClick={() => cloneMut.mutate({ url: cloneUrl.trim(), dest: cloneDest.trim() })}
                  disabled={!cloneUrl.trim() || !cloneDest.trim() || cloneMut.isPending}
                >
                  {cloneMut.isPending ? t("commits.repo.cloning") : t("commits.action.clone")}
                </Button>
              )}
            </>
          }
        >
          <div className="flex flex-col gap-2 rounded-xl bg-bg-primary p-3">
            <div className="flex flex-col gap-1">
              <Label className="text-sm font-medium text-text-secondary" htmlFor="clone-url">
                {t("commits.repo.urlLabel")}
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
                placeholder={t("commits.repo.cloneUrlPlaceholder")}
              />
              <p className="text-xs text-text-muted">
                {t("commits.repo.detectedProtocol")}{" "}
                {cloneUrl
                  ? cloneUrl.startsWith("git@") || cloneUrl.startsWith("ssh://")
                    ? "ssh"
                    : "https"
                  : "—"}
              </p>
            </div>
            <div className="flex flex-col gap-1">
              <Label className="text-sm font-medium text-text-secondary" htmlFor="clone-dest">
                {t("commits.repo.destinationPath")}
              </Label>
              <PathInput
                id="clone-dest"
                directory
                value={cloneDest}
                onChange={setCloneDest}
                placeholder="./repo"
              />
            </div>
          </div>
          {actionError && <p className="text-xs text-danger">{actionError}</p>}
          {cloneMut.isPending || cloneProgress ? (
            <div className="flex flex-col gap-1">
              <div className="h-1.5 overflow-hidden rounded bg-bg-secondary">
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
              <p className="font-mono text-[11px] text-text-muted">
                {cloneProgress
                  ? t("commits.repo.cloneProgress", {
                      received: cloneProgress.receivedObjects,
                      total: cloneProgress.totalObjects || "?",
                      deltas: cloneProgress.indexedDeltas,
                      totalDeltas: cloneProgress.totalDeltas || "?",
                      kib: Math.round(cloneProgress.receivedBytes / 1024),
                    })
                  : t("commits.repo.startingClone")}
              </p>
            </div>
          ) : null}
        </Modal>
      )}

      {/* Repository: add local */}
      {adding === "local" && (
        <Modal
          open={true}
          onOpenChange={(open) => {
            if (!open) endAdd();
          }}
          title={t("commits.repo.addLocalTitle")}
          description={t("commits.repo.addLocalDescription")}
          size="sm"
          footer={
            <>
              <Button variant="secondary" size="sm" className="min-w-0 flex-[3]" onClick={endAdd}>
                {t("commits.action.cancel")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="min-w-0 flex-[7]"
                onClick={() => localMut.mutate({ path: localPath.trim() })}
                disabled={!localPath.trim() || localMut.isPending}
              >
                {t("commits.action.add")}
              </Button>
            </>
          }
        >
          <div className="flex flex-col gap-1 rounded-xl bg-bg-primary p-3">
            <Label className="text-sm font-medium text-text-secondary" htmlFor="add-local-path">
              {t("commits.repo.location")}
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
        </Modal>
      )}

      {/* Branch: create */}
      <Modal
        open={branchCreateOpen}
        onOpenChange={(open) => {
          if (!open) setBranchCreateOpen(false);
        }}
        title={t("commits.branch.createTitle", { branch: wc.data?.branch ?? "?" })}
        description={t("commits.branch.createDescription")}
        size="sm"
        footer={
          <>
            <Button
              variant="secondary"
              size="sm"
              className="min-w-0 flex-[3]"
              onClick={() => setBranchCreateOpen(false)}
            >
              {t("commits.action.cancel")}
            </Button>
            <Button
              variant="primary"
              size="sm"
              className="min-w-0 flex-[7]"
              onClick={submitBranchCreate}
              disabled={!branchName.trim() || branchCreateMut.isPending}
            >
              {t("commits.action.create")}
            </Button>
          </>
        }
      >
        <div className="rounded-xl bg-bg-primary p-3">
          <Input
            autoFocus
            value={branchName}
            onChange={setBranchName}
            onKeyDown={(e) => {
              if (e.key === "Enter") submitBranchCreate();
            }}
            placeholder={t("commits.branch.namePlaceholder")}
            error={branchError}
          />
        </div>
      </Modal>

      {/* Branch: pull (Fork-style) */}
      {pullDialog ? (
        <Modal
          open
          onOpenChange={(open) => {
            if (!open) setPullDialog(null);
          }}
          title={t("commits.sync.pullTitle")}
          description={t("commits.sync.pullDescription")}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setPullDialog(null)}
              >
                {t("commits.action.cancel")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="min-w-0 flex-[7]"
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
                {t("commits.sync.pull")}
              </Button>
            </>
          }
        >
          <div className="flex flex-col gap-2 rounded-xl bg-bg-primary p-3">
            <Label className="flex items-center gap-2">
              <span className="w-16 shrink-0 text-sm text-text-secondary">
                {t("commits.sync.remote")}
              </span>
              <Select
                aria-label={t("commits.sync.remote")}
                className="h-auto min-w-0 flex-1 bg-bg-primary border-border-subtle px-1.5 py-1.5 text-sm"
                value={pullDialog.remote}
                disabled={wc.isSyncBusy}
                onChange={(remote) => setPullDialog({ ...pullDialog, remote })}
                options={remoteOptions.map((r) => ({ value: r, label: r }))}
              />
            </Label>
            <Label className="flex items-center gap-2">
              <span className="w-16 shrink-0 text-sm text-text-secondary">
                {t("commits.sync.branch")}
              </span>
              <Select
                aria-label={t("commits.sync.branch")}
                className="h-auto min-w-0 flex-1 bg-bg-primary border-border-subtle px-1.5 py-1.5 text-sm"
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
              <span className="w-16 shrink-0 text-sm text-text-secondary">
                {t("commits.sync.into")}
              </span>
              <span className="min-w-0 flex-1 truncate text-sm text-text-primary">
                {wc.data?.branch ?? "—"}
              </span>
            </div>
          </div>
          <div className="flex flex-col gap-2 rounded-xl bg-bg-primary p-3">
            <Checkbox
              checked={pullDialog.rebase}
              disabled={wc.isSyncBusy}
              onChange={(rebase) => setPullDialog({ ...pullDialog, rebase })}
            >
              {t("commits.sync.rebaseInstead")}
            </Checkbox>
            <Checkbox
              checked={pullDialog.stash}
              disabled={wc.isSyncBusy}
              onChange={(stash) => setPullDialog({ ...pullDialog, stash })}
            >
              {t("commits.sync.stashAndReapply")}
            </Checkbox>
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
          title={t("commits.sync.pushTitle")}
          description={t("commits.sync.pushDescription")}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setPushDialog(null)}
              >
                {t("commits.action.cancel")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="min-w-0 flex-[7]"
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
                {t("commits.sync.push")}
              </Button>
            </>
          }
        >
          <div className="flex flex-col gap-2 rounded-xl bg-bg-primary p-3">
            <div className="flex items-center gap-2">
              <span className="w-16 shrink-0 text-sm text-text-secondary">
                {t("commits.sync.branch")}
              </span>
              <span className="min-w-0 flex-1 truncate text-sm text-text-primary">
                {wc.data?.branch ?? "—"}
              </span>
            </div>
            <div className="flex items-center gap-2">
              <span className="w-16 shrink-0 text-sm text-text-secondary">
                {t("commits.sync.to")}
              </span>
              <span className="min-w-0 flex-1 truncate text-sm text-text-primary">
                {t("commits.sync.defaultTarget", {
                  target: wc.data?.upstream ?? `origin/${wc.data?.branch ?? ""}`,
                })}
              </span>
            </div>
          </div>
          <div className="flex flex-col gap-2 rounded-xl bg-bg-primary p-3">
            <Checkbox
              checked={pushDialog.tags}
              disabled={wc.isSyncBusy}
              onChange={(tags) => setPushDialog({ ...pushDialog, tags })}
            >
              {t("commits.sync.pushAllTags")}
            </Checkbox>
            <Checkbox
              checked={pushDialog.force}
              disabled={wc.isSyncBusy}
              onChange={(force) => setPushDialog({ ...pushDialog, force })}
            >
              {t("commits.sync.forcePush")}
            </Checkbox>
          </div>
        </Modal>
      ) : null}

      {stashOpen ? (
        <Modal
          open
          onOpenChange={(o) => !o && setStashOpen(false)}
          title={t("changes.stash.saveTitle")}
          description={t("changes.stash.saveDescription")}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setStashOpen(false)}
              >
                {t("changes.action.cancel")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={stashSaving}
                onClick={() => void handleSaveStash()}
              >
                {t("changes.stash.saveButton")}
              </Button>
            </>
          }
        >
          <div className="rounded-xl bg-bg-primary p-3">
            <Input
              value={stashMessage}
              onChange={setStashMessage}
              placeholder={t("changes.stash.messagePlaceholder")}
              autoFocus
              onKeyDown={(e) => {
                if (e.key === "Enter" && !stashSaving) void handleSaveStash();
              }}
            />
          </div>
          <div className="rounded-xl bg-bg-primary p-3">
            <Checkbox
              checked={stashStage}
              disabled={stashSaving}
              onChange={(v) => setStashStage(v)}
              className="items-start text-text-primary"
            >
              <span className="flex flex-col">
                <span>{t("changes.stash.stageNewFiles")}</span>
                <span className="text-xs font-normal text-text-muted">
                  {t("changes.stash.stageNewFilesHint")}
                </span>
              </span>
            </Checkbox>
          </div>
        </Modal>
      ) : null}

      {worktreeCreateOpen ? (
        <Modal
          open
          onOpenChange={(o) => !o && setWorktreeCreateOpen(false)}
          title={t("commits.worktree.createTitle")}
          description={t("commits.worktree.createDescription")}
          size="sm"
          footer={
            <>
              <Button
                variant="secondary"
                size="sm"
                className="min-w-0 flex-[3]"
                onClick={() => setWorktreeCreateOpen(false)}
                disabled={wtBusy}
              >
                {t("commits.action.cancel")}
              </Button>
              <Button
                variant="primary"
                size="sm"
                className="min-w-0 flex-[7]"
                disabled={wtBusy || !wtName.trim() || !wtLocation.trim()}
                onClick={() => void handleCreateWorktree()}
              >
                {t("commits.action.create")}
              </Button>
            </>
          }
        >
          <div className="flex flex-col gap-2 rounded-xl bg-bg-primary p-3">
            <Label>{t("commits.worktree.startFrom")}</Label>
            <Select
              aria-label={t("commits.worktree.startFrom")}
              className="h-8 bg-bg-primary border-border-subtle px-1.5 text-xs"
              value={wtStart}
              disabled={wtBusy}
              onChange={(v) => setWtStart(v)}
              options={wtBranchOptions.map((b) => ({ value: b, label: b }))}
            />
            <Label>{t("commits.worktree.branchName")}</Label>
            <Input
              value={wtName}
              onChange={setWtName}
              placeholder={t("commits.worktree.branchNamePlaceholder")}
              disabled={wtBusy}
            />
            <Label>{t("commits.repo.location")}</Label>
            <PathInput
              directory
              value={wtLocation}
              onChange={setWtLocation}
              placeholder={t("commits.worktree.locationPlaceholder")}
              disabled={wtBusy}
            />
          </div>
        </Modal>
      ) : null}

      <WorkingCopyModal open={wcModalOpen} onOpenChange={setWcModalOpen} />
    </>
  );
}
