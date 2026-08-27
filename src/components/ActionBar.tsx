// Operations toolbar below the top bar: grouped workspace / repository /
// branch actions on the left, the Local Changes modal trigger on the right.
// The dialogs and mutations for each group live here so the sidebar lists
// stay pure navigation. Named `ActionBar` (not `ToolBar`) because Windows
// filesystems are case-insensitive and `Toolbar.tsx` already exists.

import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { listen } from "@tauri-apps/api/event";
import {
  ArrowDown,
  ArrowDownUp,
  ArrowUp,
  Download,
  FileDiff,
  FolderGit2,
  FolderOpen,
  FolderPlus,
  GitBranch,
  Pencil,
  Sparkles,
  Trash2,
} from "lucide-react";

import {
  addLocalRepo,
  cloneRepo,
  createBranch,
  createWorkspace,
  deleteWorkspace,
  formatAppError,
  getBranches,
  initRepo,
  listRemotes,
  listWorkspaces,
  renameWorkspace,
  setActiveRepo,
  type CloneProgress,
} from "@/lib/api";
import { useWorkspaceUiStore } from "@/stores/workspaceStore";
import { useWorkingCopy } from "@/hooks/useWorkingCopy";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { Tooltip } from "@/components/ui/Tooltip";
import { PathInput } from "@/components/ui/PathInput";
import { ErrorAlert } from "@/components/ui/ErrorAlert";
import { AiProviderSettings } from "@/components/AiProviderSettings";
import { WorkingCopyModal } from "@/components/ui/WorkingCopyModal";

interface PullDialogState {
  remote: string;
  branch: string;
  rebase: boolean;
  stash: boolean;
}

type AddKind = "init" | "clone" | "local" | null;

function GroupDivider(): React.JSX.Element {
  return <span className="mx-1 h-8 w-px bg-border-subtle" aria-hidden />;
}

function ActionBarGroup({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}): React.JSX.Element {
  return (
    <div className="flex flex-col items-center gap-1">
      <span className="text-[10px] font-semibold uppercase tracking-wide text-text-muted">
        {label}
      </span>
      <div className="flex items-center gap-1">{children}</div>
    </div>
  );
}

function ActionBarButton({
  icon,
  label,
  title,
  onClick,
  disabled = false,
  danger = false,
}: {
  icon: React.ReactNode;
  /** Visible button text (short — the group header carries the context). */
  label: string;
  /** Full description for tooltip / aria-label; defaults to `label`. */
  title?: string;
  onClick: () => void;
  disabled?: boolean;
  danger?: boolean;
}): React.JSX.Element {
  const hint = title ?? label;
  return (
    <Tooltip content={hint}>
      <button
        type="button"
        disabled={disabled}
        aria-label={hint}
        onClick={onClick}
        className={cn(
          "flex items-center gap-1.5 h-7 px-2.5 rounded-md text-xs text-text-secondary",
          "hover:bg-bg-elevated hover:text-text-primary",
          "disabled:opacity-40 disabled:pointer-events-none",
          danger && "hover:text-danger",
        )}
      >
        {icon}
        {label}
      </button>
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

  // ── Repository state ───────────────────────────────────────────────────
  const [adding, setAdding] = useState<AddKind>(null);
  const [initPath, setInitPath] = useState("");
  const [cloneUrl, setCloneUrl] = useState("");
  const [cloneDest, setCloneDest] = useState("");
  const [cloneProgress, setCloneProgress] = useState<CloneProgress | null>(null);
  const [cloneFailed, setCloneFailed] = useState(false);
  const [localPath, setLocalPath] = useState("");
  const [actionError, setActionError] = useState<string | null>(null);

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

  const branchesQuery = useQuery({
    queryKey: ["branches", activeWorkspaceId],
    queryFn: () => getBranches(activeWorkspaceId!),
    enabled: pullDialog !== null && Boolean(activeWorkspaceId),
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

  // The fetched remote list wins over the seeded default once it arrives.
  useEffect(() => {
    if (!pullDialog || remotes.length === 0) return;
    if (!remotes.includes(pullDialog.remote)) {
      setPullDialog({ ...pullDialog, remote: remotes[0]! });
    }
  }, [pullDialog, remotes]);

  const remoteOptions = remotes.length > 0 ? remotes : [pullDialog?.remote ?? "origin"];
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

  const noWorkspace = !activeWorkspaceId;
  const noRepo = !activeRepoId;

  return (
    <>
      <div className="flex items-center gap-3 px-3 py-1.5 shrink-0 bg-bg-primary border-b border-border-subtle select-none">
        <button
          type="button"
          disabled={localChangesDisabled}
          onClick={() => setWcModalOpen(true)}
          className={cn(
            "flex items-center gap-1.5 h-7 px-2.5 rounded-md text-xs font-medium",
            "bg-accent/10 text-accent hover:bg-accent/20",
            "disabled:opacity-40 disabled:pointer-events-none",
          )}
          aria-label={`Local changes: ${changeCount}`}
        >
          <FileDiff size={14} />
          Local Changes({changeCount})
        </button>
        <div className="flex-1" />
        <ActionBarGroup label="Workspace">
          <ActionBarButton
            icon={<FolderPlus size={14} />}
            label="New"
            title="New workspace"
            onClick={() => {
              setCreateName("");
              setCreateError(null);
              setCreateOpen(true);
            }}
          />
          <ActionBarButton
            icon={<Pencil size={14} />}
            label="Rename"
            title="Rename workspace"
            disabled={noWorkspace}
            onClick={openRename}
          />
          <ActionBarButton
            icon={<Sparkles size={14} />}
            label="AI"
            title="AI provider"
            disabled={noWorkspace}
            onClick={() => setAiOpen(true)}
          />
          <ActionBarButton
            icon={<Trash2 size={14} />}
            label="Delete"
            title="Delete workspace"
            disabled={noWorkspace}
            danger
            onClick={() => setDeleteOpen(true)}
          />
        </ActionBarGroup>
        <GroupDivider />
        <ActionBarGroup label="Repository">
          <ActionBarButton
            icon={<FolderGit2 size={14} />}
            label="Init"
            title="Initialize new repo"
            disabled={noWorkspace}
            onClick={() => startAdd("init")}
          />
          <ActionBarButton
            icon={<Download size={14} />}
            label="Clone"
            title="Clone remote repo"
            disabled={noWorkspace}
            onClick={() => startAdd("clone")}
          />
          <ActionBarButton
            icon={<FolderOpen size={14} />}
            label="Add"
            title="Add existing local repo"
            disabled={noWorkspace}
            onClick={() => startAdd("local")}
          />
          <ActionBarButton
            icon={<ArrowDownUp size={14} />}
            label="Fetch"
            disabled={noRepo || wc.isSyncBusy}
            onClick={wc.fetch}
          />
        </ActionBarGroup>
        <GroupDivider />
        <ActionBarGroup label="Branch">
          <ActionBarButton
            icon={<GitBranch size={14} />}
            label="New"
            title="New branch"
            disabled={noRepo || detached || !wc.data?.sha}
            onClick={() => {
              setBranchName("");
              setBranchError(null);
              setBranchCreateOpen(true);
            }}
          />
          <ActionBarButton
            icon={<ArrowDown size={14} />}
            label="Pull"
            disabled={noRepo || wc.isSyncBusy || detached}
            onClick={openPullDialog}
          />
          <ActionBarButton
            icon={<ArrowUp size={14} />}
            label="Push"
            disabled={noRepo || wc.isSyncBusy || detached}
            onClick={() => setPushDialog({ tags: false, force: false })}
          />
        </ActionBarGroup>

        <div className="flex-1" />
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

      {/* Repository: init */}
      {adding === "init" && (
        <Modal
          open={true}
          onOpenChange={(open) => {
            if (!open) endAdd();
          }}
          title="Initialize new repo"
          size="sm"
        >
          <PathInput
            autoFocus
            directory
            value={initPath}
            onChange={setInitPath}
            onKeyDown={(e) => {
              if (e.key === "Enter" && initPath.trim()) initMut.mutate({ path: initPath.trim() });
            }}
            placeholder="Absolute path, e.g. /Users/me/projects/new"
            error={actionError}
          />
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
          size="sm"
        >
          <div className="flex flex-col gap-3">
            <div className="flex flex-col gap-1">
              <label className="text-xs font-medium text-text-secondary" htmlFor="clone-url">
                URL
              </label>
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
              <label className="text-xs font-medium text-text-secondary" htmlFor="clone-dest">
                Destination path
              </label>
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
                    className="h-full bg-accent transition-[width] duration-200"
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
          size="sm"
        >
          <PathInput
            autoFocus
            directory
            value={localPath}
            onChange={setLocalPath}
            onKeyDown={(e) => {
              if (e.key === "Enter" && localPath.trim())
                localMut.mutate({ path: localPath.trim() });
            }}
            placeholder="Absolute path to an existing git working tree"
            error={actionError}
          />
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
            <label className="flex items-center gap-2">
              <span className="w-16 shrink-0 text-right text-xs text-text-secondary">Remote</span>
              <select
                className="flex-1 min-w-0 bg-bg-primary border border-border-subtle rounded px-1.5 py-1 text-xs text-text-primary"
                value={pullDialog.remote}
                disabled={wc.isSyncBusy}
                onChange={(e) => setPullDialog({ ...pullDialog, remote: e.target.value })}
              >
                {remoteOptions.map((r) => (
                  <option key={r} value={r}>
                    {r}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex items-center gap-2">
              <span className="w-16 shrink-0 text-right text-xs text-text-secondary">Branch</span>
              <select
                className="flex-1 min-w-0 bg-bg-primary border border-border-subtle rounded px-1.5 py-1 text-xs text-text-primary"
                value={pullDialog.branch}
                disabled={wc.isSyncBusy}
                onChange={(e) => setPullDialog({ ...pullDialog, branch: e.target.value })}
              >
                {branchOptions.map((b) => (
                  <option key={b} value={b}>
                    {pullDialog.remote}/{b}
                  </option>
                ))}
              </select>
            </label>
            <div className="flex items-center gap-2">
              <span className="w-16 shrink-0 text-right text-xs text-text-secondary">Into</span>
              <span className="flex-1 min-w-0 truncate text-xs font-mono text-text-primary">
                {wc.data?.branch ?? "—"}
              </span>
            </div>
            <label className="flex items-center gap-2 text-xs text-text-primary">
              <input
                type="checkbox"
                className="accent-accent"
                checked={pullDialog.rebase}
                disabled={wc.isSyncBusy}
                onChange={(e) => setPullDialog({ ...pullDialog, rebase: e.target.checked })}
              />
              Rebase instead of merge
            </label>
            <label className="flex items-center gap-2 text-xs text-text-primary">
              <input
                type="checkbox"
                className="accent-accent"
                checked={pullDialog.stash}
                disabled={wc.isSyncBusy}
                onChange={(e) => setPullDialog({ ...pullDialog, stash: e.target.checked })}
              />
              Stash and reapply local changes
            </label>
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
        >
          <div className="flex flex-col gap-2">
            <div className="flex items-center gap-2">
              <span className="w-16 shrink-0 text-right text-xs text-text-secondary">Branch</span>
              <span className="flex-1 min-w-0 truncate text-xs font-mono text-text-primary">
                {wc.data?.branch ?? "—"}
              </span>
            </div>
            <div className="flex items-center gap-2">
              <span className="w-16 shrink-0 text-right text-xs text-text-secondary">To</span>
              <span className="flex-1 min-w-0 truncate text-xs font-mono text-text-primary">
                default ({wc.data?.upstream ?? `origin/${wc.data?.branch ?? ""}`})
              </span>
            </div>
            <label className="flex items-center gap-2 text-xs text-text-primary">
              <input
                type="checkbox"
                className="accent-accent"
                checked={pushDialog.tags}
                disabled={wc.isSyncBusy}
                onChange={(e) => setPushDialog({ ...pushDialog, tags: e.target.checked })}
              />
              Push all tags
            </label>
            <label className="flex items-center gap-2 text-xs text-text-primary">
              <input
                type="checkbox"
                className="accent-accent"
                checked={pushDialog.force}
                disabled={wc.isSyncBusy}
                onChange={(e) => setPushDialog({ ...pushDialog, force: e.target.checked })}
              />
              Force push
            </label>
          </div>
          <div className="mt-3 flex justify-end gap-2">
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
          </div>
        </Modal>
      ) : null}

      <WorkingCopyModal open={wcModalOpen} onOpenChange={setWcModalOpen} />
    </>
  );
}
